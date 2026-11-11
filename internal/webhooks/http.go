// Package webhooks manages company-scoped subscriptions and a durable signed delivery queue (FLA-104).
package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	ConfigBuilder "github.com/keloran/go-config"
)

type System struct {
	Config *ConfigBuilder.Config
	Client *http.Client
}

func New(cfg *ConfigBuilder.Config) *System { return &System{Config: cfg, Client: newClient()} }

var eventTypes = map[string]bool{"flag.created": true, "flag.state_changed": true, "flag.archived": true}

func validEvents(events []string) bool {
	if len(events) == 0 || len(events) > 3 {
		return false
	}
	seen := map[string]bool{}
	for _, e := range events {
		if !eventTypes[e] || seen[e] {
			return false
		}
		seen[e] = true
	}
	return true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	logs.Errorf("webhooks: %v", err)
	writeJSON(w, 500, map[string]string{"error": "Webhook operation failed"})
}
func (s *System) conn(ctx context.Context) (*pgx.Conn, error) {
	return s.Config.Database.GetPGXClient(ctx)
}
func (s *System) owner(w http.ResponseWriter, r *http.Request) (string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	return company.NewSystem(s.Config).CallerOwns(w, r)
}

func (s *System) List(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.owner(w, r)
	if !ok {
		return
	}
	conn, err := s.conn(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer conn.Close(r.Context())
	var data json.RawMessage
	err = conn.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('id',e.id,'name',e.name,'url',e.url,'enabled',e.enabled,'event_types',e.event_types,'created_at',e.created_at) ORDER BY e.created_at DESC),'[]'::jsonb) FROM public.webhook_endpoint e JOIN public.company c ON c.id=e.company_id WHERE c.company_id=$1`, owner).Scan(&data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"endpoints": data})
}

func (s *System) Create(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.owner(w, r)
	if !ok {
		return
	}
	var req struct {
		Name   string   `json:"name"`
		URL    string   `json:"url"`
		Events []string `json:"event_types"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid JSON"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) < 1 || len(req.Name) > 100 || !validEvents(req.Events) {
		writeJSON(w, 400, map[string]string{"error": "Provide a name and at least one supported event type"})
		return
	}
	if _, err := validURL(req.URL); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fail(w, err)
		return
	}
	secret := "whsec_" + hex.EncodeToString(raw)
	conn, err := s.conn(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer conn.Close(r.Context())
	// Serialize the quota check per company so concurrent requests cannot exceed the limit.
	tx, err := conn.Begin(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer tx.Rollback(r.Context())
	var companyID int
	err = tx.QueryRow(r.Context(), `SELECT id FROM public.company WHERE company_id=$1 FOR UPDATE`, owner).Scan(&companyID)
	if err != nil {
		fail(w, err)
		return
	}
	var count int
	err = tx.QueryRow(r.Context(), `SELECT count(*) FROM public.webhook_endpoint WHERE company_id=$1`, companyID).Scan(&count)
	if err != nil {
		fail(w, err)
		return
	}
	if count >= 20 {
		writeJSON(w, 409, map[string]string{"error": "Maximum of 20 webhook endpoints per company"})
		return
	}
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO public.webhook_endpoint(company_id,name,url,secret,event_types) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, companyID, req.Name, req.URL, secret, req.Events).Scan(&id)
	if err != nil {
		fail(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "secret": secret})
}

func (s *System) Update(w http.ResponseWriter, r *http.Request) { s.mutate(w, r, false) }
func (s *System) Delete(w http.ResponseWriter, r *http.Request) { s.mutate(w, r, true) }
func (s *System) mutate(w http.ResponseWriter, r *http.Request, remove bool) {
	owner, ok := s.owner(w, r)
	if !ok {
		return
	}
	id := r.PathValue("endpointId")
	if _, err := uuid.Parse(id); err != nil {
		http.NotFound(w, r)
		return
	}
	var req struct {
		Enabled *bool     `json:"enabled"`
		Events  *[]string `json:"event_types"`
	}
	if !remove {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Enabled == nil && req.Events == nil) || (req.Events != nil && !validEvents(*req.Events)) {
			writeJSON(w, 400, map[string]string{"error": "Provide enabled or supported event types"})
			return
		}
	}
	conn, err := s.conn(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer conn.Close(r.Context())
	sql := `UPDATE public.webhook_endpoint e SET enabled=COALESCE($3,e.enabled), event_types=COALESCE($4,e.event_types) FROM public.company c WHERE c.id=e.company_id AND c.company_id=$1 AND e.id=$2`
	args := []any{owner, id, req.Enabled, req.Events}
	if remove {
		sql = `DELETE FROM public.webhook_endpoint e USING public.company c WHERE c.id=e.company_id AND c.company_id=$1 AND e.id=$2`
		args = args[:2]
	}
	result, err := conn.Exec(r.Context(), sql, args...)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(204)
}

func (s *System) Deliveries(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.owner(w, r)
	if !ok {
		return
	}
	before := int64(9223372036854775807)
	if value := r.URL.Query().Get("before"); value != "" {
		var err error
		before, err = strconv.ParseInt(value, 10, 64)
		if err != nil || before <= 0 {
			writeJSON(w, 400, map[string]string{"error": "Invalid cursor"})
			return
		}
	}
	endpoint := r.URL.Query().Get("endpoint")
	if endpoint != "" {
		if _, err := uuid.Parse(endpoint); err != nil {
			writeJSON(w, 400, map[string]string{"error": "Invalid endpoint"})
			return
		}
	}
	conn, err := s.conn(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer conn.Close(r.Context())
	var data json.RawMessage
	err = conn.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(to_jsonb(log) ORDER BY log.id::bigint DESC),'[]'::jsonb) FROM (SELECT d.id::text,d.event_id,d.event_type,d.payload,d.created_at,d.status,d.attempts,d.next_attempt_at,d.last_attempt_at,d.response_status,d.last_error,d.delivered_at,e.id endpoint_id,e.name endpoint_name FROM public.webhook_delivery d JOIN public.webhook_endpoint e ON e.id=d.endpoint_id JOIN public.company c ON c.id=e.company_id WHERE c.company_id=$1 AND d.id<$2 AND ($3='' OR e.id::text=$3) ORDER BY d.id DESC LIMIT 50) log`, owner, before, endpoint).Scan(&data)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"deliveries": data})
}

func (s *System) Retry(w http.ResponseWriter, r *http.Request) {
	owner, ok := s.owner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("deliveryId"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	conn, err := s.conn(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	defer conn.Close(r.Context())
	result, err := conn.Exec(r.Context(), `UPDATE public.webhook_delivery d SET status='pending',attempts=0,next_attempt_at=now(),locked_until=NULL,last_error=NULL FROM public.webhook_endpoint e,public.company c WHERE d.endpoint_id=e.id AND e.company_id=c.id AND c.company_id=$1 AND d.id=$2 AND d.status='failed' AND e.enabled`, owner, id)
	if err != nil {
		fail(w, err)
		return
	}
	if result.RowsAffected() == 0 {
		writeJSON(w, 409, map[string]string{"error": "Only failed deliveries on enabled endpoints can be retried"})
		return
	}
	w.WriteHeader(204)
}
