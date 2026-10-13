package company

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/testdb"
	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go"
)

func paidSession(id, user, price string) *CheckoutSession {
	return &CheckoutSession{
		ID: id, Status: "complete", PaymentStatus: "paid", ClientReferenceID: user,
		Customer: "cus_1", Subscription: "sub_" + id, Metadata: map[string]string{"priceId": price, "userId": user},
	}
}

func TestValidateCheckoutSession(t *testing.T) {
	ok := paidSession("cs_1", "user-a", "price_live")
	assert.NoError(t, validateCheckoutSession(ok, "user-a"))

	unpaid := *ok
	unpaid.PaymentStatus = "unpaid"
	assert.Error(t, validateCheckoutSession(&unpaid, "user-a"))

	open := *ok
	open.Status = "open"
	assert.Error(t, validateCheckoutSession(&open, "user-a"))

	assert.Error(t, validateCheckoutSession(ok, "user-b"), "someone else's session")
	assert.Error(t, validateCheckoutSession(ok, ""))

	metaOnly := *ok
	metaOnly.ClientReferenceID = ""
	assert.NoError(t, validateCheckoutSession(&metaOnly, "user-a"), "falls back to metadata userId")

	noPrice := paidSession("cs_2", "user-a", "")
	assert.Error(t, validateCheckoutSession(noPrice, "user-a"))
}

func plan(t *testing.T, db *sql.DB, companyId string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow(`SELECT payment_plan_id FROM public.company WHERE company_id = $1`, companyId).Scan(&id))
	return id
}

func TestUpgradeCompany(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	c := ConfigBuilder.NewConfigNoVault()
	require.NoError(t, c.Build(ConfigBuilder.Database, ConfigBuilder.Bugfixes))
	c.Local.Development = false
	c.ProjectProperties = map[string]interface{}{"stripeKey": "sk_test"}
	s := NewSystem(c)

	sessions := map[string]*CheckoutSession{
		"cs_paid":   paidSession("cs_paid", "user-a", "price_live"),
		"cs_dev":    paidSession("cs_dev", "user-a", "price_dev"),
		"cs_b":      paidSession("cs_b", "user-b", "price_live"),
		"cs_unpaid": {ID: "cs_unpaid", Status: "open", PaymentStatus: "unpaid", ClientReferenceID: "user-a", Metadata: map[string]string{"priceId": "price_live"}},
	}
	orig := fetchCheckoutSession
	fetchCheckoutSession = func(_, id string) (*CheckoutSession, error) {
		if cs, ok := sessions[id]; ok {
			return cs, nil
		}
		return nil, errors.New("no such session")
	}
	t.Cleanup(func() { fetchCheckoutSession = orig })

	upgrade := func(user, session string) int {
		body, _ := json.Marshal(map[string]string{"sessionId": session})
		req := httptest.NewRequest(http.MethodPut, "/company/upgrade", bytes.NewReader(body))
		req = req.WithContext(auth.WithUser(req.Context(), auth.User{ID: user}))
		w := httptest.NewRecorder()
		s.UpgradeCompany(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusForbidden, upgrade("user-a", "cs_unpaid"))
	assert.Equal(t, http.StatusForbidden, upgrade("user-a", "cs_b"), "another user's session")
	assert.Equal(t, http.StatusBadRequest, upgrade("user-a", "cs_missing"))
	assert.Equal(t, http.StatusInternalServerError, upgrade("user-a", "cs_dev"), "dev price outside development")
	assert.Equal(t, 1, plan(t, db, "company-a"))

	assert.Equal(t, http.StatusOK, upgrade("user-a", "cs_paid"))
	assert.Equal(t, 2, plan(t, db, "company-a"))
	assert.Equal(t, http.StatusOK, upgrade("user-a", "cs_paid"), "same company re-applying is a no-op")

	// the same session can't upgrade a second company
	assert.ErrorIs(t, s.UpgradeCompanyInDB(context.Background(), "company-b", sessions["cs_paid"]), ErrSessionUsed)
	assert.Equal(t, 1, plan(t, db, "company-b"))

	// subscription deleted -> back to the default plan, and replaying the old session doesn't re-upgrade
	raw, _ := json.Marshal(map[string]string{"id": "sub_cs_paid"})
	require.NoError(t, s.HandleStripeEvent(context.Background(), stripe.Event{Type: "customer.subscription.deleted", Data: &stripe.EventData{Raw: raw}}))
	assert.Equal(t, 1, plan(t, db, "company-a"))
	assert.Equal(t, http.StatusOK, upgrade("user-a", "cs_paid"))
	assert.Equal(t, 1, plan(t, db, "company-a"))

	// webhook checkout.session.completed upgrades the referenced user's company, idempotently
	raw, _ = json.Marshal(sessions["cs_b"])
	event := stripe.Event{Type: "checkout.session.completed", Data: &stripe.EventData{Raw: raw}}
	require.NoError(t, s.HandleStripeEvent(context.Background(), event))
	require.NoError(t, s.HandleStripeEvent(context.Background(), event))
	assert.Equal(t, 2, plan(t, db, "company-b"))

	// dev prices are accepted in development
	c.Local.Development = true
	assert.Equal(t, http.StatusOK, upgrade("user-a", "cs_dev"))
	assert.Equal(t, 2, plan(t, db, "company-a"))
}
