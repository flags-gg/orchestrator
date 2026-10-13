package general

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/webhook"
)

func TestStripeEventsSignature(t *testing.T) {
	c := ConfigBuilder.NewConfigNoVault()
	require.NoError(t, c.Build(ConfigBuilder.Bugfixes))
	s := NewSystem(c)
	payload := []byte(`{"id":"evt_1","type":"invoice.paid","data":{"object":{}}}`)

	post := func(sig string) int {
		req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", bytes.NewReader(payload))
		req.Header.Set("Stripe-Signature", sig)
		w := httptest.NewRecorder()
		s.StripeEvents(w, req)
		return w.Code
	}
	sign := func(secret string) string {
		now := time.Now()
		return fmt.Sprintf("t=%d,v1=%x", now.Unix(), webhook.ComputeSignature(now, payload, secret))
	}

	c.ProjectProperties = map[string]interface{}{"stripe_webhook_secret": ""}
	assert.Equal(t, http.StatusServiceUnavailable, post(sign("")), "no secret configured rejects everything")

	c.ProjectProperties["stripe_webhook_secret"] = "whsec_test"
	assert.Equal(t, http.StatusBadRequest, post(""))
	assert.Equal(t, http.StatusBadRequest, post(sign("whsec_wrong")))
	assert.Equal(t, http.StatusOK, post(sign("whsec_test")), "valid signature, unhandled type")
}
