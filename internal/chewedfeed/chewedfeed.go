// Package chewedfeed tells ChewedFeed HQ about flags.gg's customers: the standard product hook
// (POST /api/product-customers/sync with the X-Enquiry-Key header). A company is sent as free when
// it's created and again with its plan name whenever the plan changes; an hourly sweep sends every
// company, which backfills the ones from before this and anything a push missed.
// Off unless CHEWEDFEED_API_URL and CHEWEDFEED_ENQUIRY_KEY are set.
package chewedfeed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	ConfigBuilder "github.com/keloran/go-config"
)

type customer struct {
	ExternalCustomerID string    `json:"external_customer_id"`
	Name               string    `json:"name"`
	Email              string    `json:"email"`
	Plan               string    `json:"plan"`
	SignedUpAt         time.Time `json:"signed_up_at"`
}

// companies is every company ("" for $1) or one, with its plan name and its first member's email.
const companies = `SELECT c.company_id, c.name, c.created_at, COALESCE(pp.name, 'free'),
	COALESCE((SELECT u.email_address FROM public.company_user cu JOIN public.user u ON u.id = cu.user_id
		WHERE cu.company_id = c.id AND COALESCE(u.email_address, '') <> '' ORDER BY cu.created_at, cu.id LIMIT 1), '')
	FROM public.company c LEFT JOIN public.payment_plans pp ON pp.id = c.payment_plan_id
	WHERE $1 = '' OR c.company_id = $1`

var client = &http.Client{Timeout: 10 * time.Second}

// Push sends one company (or every company, for "") to ChewedFeed. Best effort: failures are
// logged and the hourly sweep sends it again, so callers never fail on ChewedFeed.
func Push(ctx context.Context, cfg *ConfigBuilder.Config, companyID string) {
	url, key := strings.TrimRight(os.Getenv("CHEWEDFEED_API_URL"), "/"), os.Getenv("CHEWEDFEED_ENQUIRY_KEY")
	if url == "" || key == "" {
		return
	}
	if err := push(ctx, cfg, url, key, companyID); err != nil {
		_ = cfg.Bugfixes.Logger.Errorf("chewedfeed: sync %q: %v", companyID, err)
	}
}

func push(ctx context.Context, cfg *ConfigBuilder.Config, url, key, companyID string) error {
	db, err := cfg.Database.GetPGXClient(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close(ctx) }()
	rows, err := db.Query(ctx, companies, companyID)
	if err != nil {
		return err
	}
	var list []customer
	for rows.Next() {
		var c customer
		if err := rows.Scan(&c.ExternalCustomerID, &c.Name, &c.SignedUpAt, &c.Plan, &c.Email); err != nil {
			rows.Close()
			return err
		}
		if c.Email != "" { // ChewedFeed needs one; a company with no member email waits for the sweep
			list = append(list, c)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range list {
		if err := send(ctx, url, key, c); err != nil {
			return fmt.Errorf("%s: %w", c.ExternalCustomerID, err)
		}
	}
	return nil
}

func send(ctx context.Context, url, key string, c customer) error {
	body, err := json.Marshal(c)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/product-customers/sync", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Enquiry-Key", key)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("chewedfeed returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Run sweeps every company to ChewedFeed now and then hourly, until ctx ends.
// ponytail: one POST per company per hour; a batch endpoint in ChewedFeed if companies reach the thousands
func Run(ctx context.Context, cfg *ConfigBuilder.Config) {
	for {
		Push(ctx, cfg, "")
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}
