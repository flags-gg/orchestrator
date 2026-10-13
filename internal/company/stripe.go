package company

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5"
	"github.com/stripe/stripe-go"
)

var ErrSessionUsed = errors.New("checkout session already used by another company")

// CheckoutSession holds the checkout fields we rely on. stripe-go v70's struct predates
// status/payment_status, so the session is decoded from the raw API/webhook JSON instead.
type CheckoutSession struct {
	ID                string            `json:"id"`
	Status            string            `json:"status"`
	PaymentStatus     string            `json:"payment_status"`
	ClientReferenceID string            `json:"client_reference_id"`
	Customer          string            `json:"customer"`
	Subscription      string            `json:"subscription"`
	Metadata          map[string]string `json:"metadata"`
}

// UserID is the Clerk user the dashboard created the session for.
func (cs *CheckoutSession) UserID() string {
	if cs.ClientReferenceID != "" {
		return cs.ClientReferenceID
	}
	return cs.Metadata["userId"]
}

// fetchCheckoutSession is a seam for tests.
var fetchCheckoutSession = func(key, id string) (*CheckoutSession, error) {
	cs := &CheckoutSession{}
	if err := stripe.GetBackend(stripe.APIBackend).Call(http.MethodGet, "/v1/checkout/sessions/"+url.PathEscape(id), key, nil, cs); err != nil {
		return nil, err
	}
	return cs, nil
}

func validateCheckoutSession(cs *CheckoutSession, userId string) error {
	switch {
	case cs == nil || cs.ID == "":
		return errors.New("missing checkout session")
	case cs.Status != "complete" || cs.PaymentStatus != "paid":
		return errors.New("checkout session not paid")
	case userId == "" || cs.UserID() != userId:
		return errors.New("checkout session belongs to another user")
	case cs.Metadata["priceId"] == "":
		return errors.New("checkout session has no priceId")
	}
	return nil
}

func (s *System) stripeKey() string {
	key, _ := s.Config.ProjectProperties["stripeKey"].(string)
	return key
}

// UpgradeCompanyInDB moves the company to the session's plan. Each session can only ever be applied
// to one company once; re-applying it to the same company is a no-op (webhook and PUT both call this).
func (s *System) UpgradeCompanyInDB(ctx context.Context, companyId string, cs *CheckoutSession) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	tx, err := client.Begin(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to begin transaction: %v", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	tag, err := tx.Exec(ctx, `
    INSERT INTO public.stripe_checkout_session (session_id, company_id)
    VALUES ($1, $2)
    ON CONFLICT (session_id) DO NOTHING`, cs.ID, companyId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to record checkout session: %v", err)
	}
	if tag.RowsAffected() == 0 {
		var usedBy string
		if err := tx.QueryRow(ctx, `SELECT company_id FROM public.stripe_checkout_session WHERE session_id = $1`, cs.ID).Scan(&usedBy); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to read checkout session: %v", err)
		}
		if usedBy == companyId {
			return nil
		}
		return ErrSessionUsed
	}

	// dev price ids only count in development
	var planId int
	if err := tx.QueryRow(ctx, `
    SELECT id
    FROM public.payment_plans
    WHERE stripe_id = $1
       OR ($2 AND stripe_id_dev = $1)
    LIMIT 1`, cs.Metadata["priceId"], s.Config.Local.Development).Scan(&planId); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.Config.Bugfixes.Logger.Errorf("Unknown price id: %s", cs.Metadata["priceId"])
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to find payment plan: %v", err)
	}

	tag, err = tx.Exec(ctx, `
    UPDATE public.company
    SET
      payment_plan_id = $1,
      stripe_customer_id = NULLIF($2, ''),
      stripe_subscription_id = NULLIF($3, '')
    WHERE company_id = $4`, planId, cs.Customer, cs.Subscription, companyId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update company: %v", err)
	}
	if tag.RowsAffected() == 0 {
		return s.Config.Bugfixes.Logger.Errorf("Company not found: %s", companyId)
	}

	return tx.Commit(ctx)
}

// DowngradeSubscription puts the company holding the subscription back on the default (free) plan.
func (s *System) DowngradeSubscription(ctx context.Context, subscriptionId string) error {
	if subscriptionId == "" {
		return nil
	}

	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(ctx, `
    UPDATE public.company
    SET payment_plan_id = DEFAULT, stripe_subscription_id = NULL
    WHERE stripe_subscription_id = $1`, subscriptionId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to downgrade company: %v", err)
	}
	return nil
}

// HandleStripeEvent applies an already signature-verified Stripe event. Other event types are ignored.
func (s *System) HandleStripeEvent(ctx context.Context, event stripe.Event) error {
	if event.Data == nil {
		return nil
	}

	switch event.Type {
	case "checkout.session.completed":
		cs := &CheckoutSession{}
		if err := json.Unmarshal(event.Data.Raw, cs); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to decode checkout session: %v", err)
		}
		if err := validateCheckoutSession(cs, cs.UserID()); err != nil {
			s.Config.Bugfixes.Logger.Infof("Ignoring checkout session %s: %v", cs.ID, err)
			return nil
		}
		companyId, err := s.GetCompanyId(ctx, cs.UserID())
		if err != nil {
			return err
		}
		if companyId == "" {
			s.Config.Bugfixes.Logger.Infof("Ignoring checkout session %s: user has no company", cs.ID)
			return nil
		}
		if err := s.UpgradeCompanyInDB(ctx, companyId, cs); err != nil && !errors.Is(err, ErrSessionUsed) {
			return err
		}
	case "customer.subscription.deleted":
		var sub struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to decode subscription: %v", err)
		}
		return s.DowngradeSubscription(ctx, sub.ID)
	}
	return nil
}
