package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/testdb"
	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/require"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCompanyWebhooksAndDelivery(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	cfg := ConfigBuilder.NewConfigNoVault()
	require.NoError(t, cfg.Build(ConfigBuilder.Database))
	s := New(cfg)
	call := func(user, method, path, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /company/webhooks", s.List)
		mux.HandleFunc("POST /company/webhooks", s.Create)
		mux.HandleFunc("PATCH /company/webhooks/{endpointId}", s.Update)
		mux.HandleFunc("DELETE /company/webhooks/{endpointId}", s.Delete)
		mux.HandleFunc("GET /company/webhook-deliveries", s.Deliveries)
		mux.HandleFunc("POST /company/webhook-deliveries/{deliveryId}/retry", s.Retry)
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if user != "" {
			r = r.WithContext(auth.WithUser(r.Context(), auth.User{ID: user}))
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 401, call("", "GET", "/company/webhooks", "").Code)
	create := call("user-a", "POST", "/company/webhooks", `{"name":"TrackLines","url":"https://hooks.example.com/flags","event_types":["flag.created","flag.state_changed","flag.archived"]}`)
	require.Equal(t, 201, create.Code, create.Body.String())
	var endpoint map[string]string
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &endpoint))
	secret := endpoint["secret"]
	id := endpoint["id"]
	require.NotEmpty(t, secret)
	require.NotContains(t, call("user-a", "GET", "/company/webhooks", "").Body.String(), secret)
	require.NotContains(t, call("user-b", "GET", "/company/webhooks", "").Body.String(), id)
	require.Equal(t, 404, call("user-b", "PATCH", "/company/webhooks/"+id, `{"enabled":false}`).Code)
	require.Equal(t, 404, call("user-b", "DELETE", "/company/webhooks/"+id, "").Code)
	// A second subscription sees only archive events; another company sees none of A's changes.
	archive := call("user-a", "POST", "/company/webhooks", `{"name":"archive","url":"https://hooks.example.com/archive","event_types":["flag.archived"]}`)
	require.Equal(t, 201, archive.Code)
	require.Equal(t, 201, call("user-b", "POST", "/company/webhooks", `{"name":"other company","url":"https://hooks.example.com/b","event_types":["flag.state_changed"]}`).Code)
	count := func(sql string, args ...any) int {
		var n int
		require.NoError(t, db.QueryRow(sql, args...).Scan(&n))
		return n
	}
	exec := func(sql string, args ...any) { _, err := db.Exec(sql, args...); require.NoError(t, err) }
	require.Equal(t, 0, count(`SELECT count(*) FROM public.webhook_delivery`))
	tx, err := db.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(`UPDATE public.flag SET enabled=true WHERE name='flag-a'`)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.Equal(t, 0, count(`SELECT count(*) FROM public.webhook_delivery`), "rolled-back changes must not send")
	exec(`UPDATE public.flag SET enabled=true WHERE name='flag-a'`)
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery`), "only matching subscription in company A")
	exec(`UPDATE public.flag SET enabled=true,updated_at=now() WHERE name='flag-a'`)
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery`), "idempotent calls emit nothing")

	var payload json.RawMessage
	require.NoError(t, db.QueryRow(`SELECT payload FROM public.webhook_delivery LIMIT 1`).Scan(&payload))
	var event struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Company string `json:"company_id"`
		Data    struct {
			Environment string `json:"environment_id"`
			Enabled     bool   `json:"enabled"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(payload, &event))
	require.Equal(t, "flag.state_changed", event.Type)
	require.Equal(t, "company-a", event.Company)
	require.Equal(t, "env-a", event.Data.Environment)
	require.True(t, event.Data.Enabled)
	attempts := 0
	s.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		attempts++
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, Signature(secret, r.Header.Get("X-Flags-Timestamp"), body), r.Header.Get("X-Flags-Signature"))
		require.Equal(t, event.ID, r.Header.Get("X-Flags-Event-ID"))
		stamp, err := strconv.ParseInt(r.Header.Get("X-Flags-Timestamp"), 10, 64)
		require.NoError(t, err)
		require.Less(t, time.Since(time.Unix(stamp, 0)), 5*time.Second)
		status := 503
		if attempts > 1 {
			status = 204
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	ctx := context.Background()
	worked, err := s.deliverOne(ctx)
	require.NoError(t, err)
	require.True(t, worked)
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery WHERE status='pending' AND attempts=1 AND response_status=503 AND next_attempt_at > now()+interval '50 seconds'`))
	worked, err = s.deliverOne(ctx)
	require.NoError(t, err)
	require.False(t, worked, "backoff must be honored")
	exec(`UPDATE public.webhook_delivery SET next_attempt_at=now()`)
	worked, err = s.deliverOne(ctx)
	require.NoError(t, err)
	require.True(t, worked)
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery WHERE status='delivered' AND attempts=2 AND response_status=204 AND delivered_at IS NOT NULL`))
	require.NotContains(t, call("user-b", "GET", "/company/webhook-deliveries", "").Body.String(), event.ID)
	log := call("user-a", "GET", "/company/webhook-deliveries", "")
	require.Equal(t, 200, log.Code, log.Body.String())
	require.Contains(t, log.Body.String(), event.ID)
	require.NotContains(t, log.Body.String(), secret)

	// Paused subscriptions don't enqueue; event-type edits take effect for future changes.
	require.Equal(t, 204, call("user-a", "PATCH", "/company/webhooks/"+id, `{"enabled":false}`).Code)
	exec(`UPDATE public.flag SET enabled=false WHERE name='flag-a'`)
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery`))
	require.Equal(t, 204, call("user-a", "PATCH", "/company/webhooks/"+id, `{"enabled":true,"event_types":["flag.created","flag.archived"]}`).Code)
	exec(`INSERT INTO public.flag(name,agent_id,environment_id,ticket_ref) SELECT 'new',agent_id,id,'FLA-104' FROM public.environment WHERE env_id='env-a'`)
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery WHERE event_type='flag.created'`))
	exec(`DELETE FROM public.flag WHERE name='new'`)
	require.Equal(t, 2, count(`SELECT count(*) FROM public.webhook_delivery WHERE event_type='flag.archived'`))
	require.Equal(t, 1, count(`SELECT count(DISTINCT event_id) FROM public.webhook_delivery WHERE event_type='flag.archived'`))

	// Final failures surface for manual retry; leases recover after a worker crash.
	exec(`UPDATE public.webhook_delivery SET next_attempt_at=now()+interval '1 day' WHERE status='pending'`)
	exec(`UPDATE public.webhook_delivery SET attempts=7,status='pending',next_attempt_at=now() WHERE event_type='flag.created'`)
	s.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("offline") })}
	worked, err = s.deliverOne(ctx)
	require.NoError(t, err)
	require.True(t, worked)
	var failedID string
	require.NoError(t, db.QueryRow(`SELECT id::text FROM public.webhook_delivery WHERE status='failed'`).Scan(&failedID))
	require.Equal(t, 409, call("user-b", "POST", "/company/webhook-deliveries/"+failedID+"/retry", "").Code)
	require.Equal(t, 204, call("user-a", "POST", "/company/webhook-deliveries/"+failedID+"/retry", "").Code)
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery WHERE id::text=$1 AND status='pending' AND attempts=0`, failedID))
	exec(`UPDATE public.webhook_delivery SET status='delivering',attempts=8,locked_until=now()-interval '1 minute' WHERE id::text=$1`, failedID)
	require.NoError(t, s.prune(ctx))
	require.Equal(t, 1, count(`SELECT count(*) FROM public.webhook_delivery WHERE id::text=$1 AND status='failed'`, failedID))
	require.Equal(t, 204, call("user-a", "DELETE", "/company/webhooks/"+id, "").Code)
	require.Equal(t, 0, count(`SELECT count(*) FROM public.webhook_delivery WHERE endpoint_id::text=$1`, id))
}
