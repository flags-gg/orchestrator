// Package staff serves platform-wide information to the configured verified Clerk user.
package staff

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/auth"
	ConfigBuilder "github.com/keloran/go-config"
)

type System struct {
	Config *ConfigBuilder.Config
}

func New(cfg *ConfigBuilder.Config) *System {
	return &System{Config: cfg}
}

// Only must be wrapped in RequireUser: it never accepts a company role or SDK credential as staff access.
func (s *System) Only(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		staffID, _ := s.Config.ProjectProperties["staff_user_id"].(string)
		staffID = strings.TrimSpace(staffID)
		id, err := auth.UserID(r)
		if staffID == "" || err != nil || id != staffID {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (s *System) Me(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]bool{"staff": true})
}

func (s *System) query(w http.ResponseWriter, r *http.Request, sql string, args ...any) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	conn, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		logs.Errorf("staff database: %v", err)
		http.Error(w, "Unable to load staff data", 500)
		return
	}
	defer conn.Close(ctx)
	var data json.RawMessage
	if err := conn.QueryRow(ctx, sql, args...).Scan(&data); err != nil {
		logs.Errorf("staff query: %v", err)
		http.Error(w, "Unable to load staff data", 500)
		return
	}
	jsonResponse(w, data)
}

func (s *System) Stats(w http.ResponseWriter, r *http.Request) {
	// Table names are fixed application constants, never request input.
	tables := []struct{ key, table string }{{"companies", "company"}, {"users", `"user"`}, {"projects", "project"}, {"agents", "agent"}, {"environments", "environment"}, {"flags", "flag"}, {"integration_tokens", "integration_token"}}
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		parts = append(parts, `'`+t.key+`', (SELECT jsonb_build_object('total', count(*), 'day', count(*) FILTER (WHERE created_at >= now() - interval '1 day'), 'week', count(*) FILTER (WHERE created_at >= now() - interval '7 days')) FROM public.`+t.table+`)`)
	}
	s.query(w, r, `SELECT jsonb_build_object(`+strings.Join(parts, ",")+`, 'enabled_flags', (SELECT count(*) FROM public.flag WHERE enabled), 'plans', (SELECT COALESCE(jsonb_object_agg(name, n), '{}'::jsonb) FROM (SELECT COALESCE(pp.name, 'Unknown') name, count(*) n FROM public.company c LEFT JOIN public.payment_plans pp ON pp.id = c.payment_plan_id GROUP BY COALESCE(pp.name, 'Unknown')) p))`)
}

func paging(r *http.Request) (string, int) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 200 {
		q = q[:200]
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	return q, (page - 1) * 25
}

func (s *System) Companies(w http.ResponseWriter, r *http.Request) {
	q, offset := paging(r)
	s.query(w, r, `WITH matched AS (SELECT c.id, c.company_id, c.name, COALESCE(c.domain,'') domain, c.created_at, COALESCE(pp.name,'Unknown') plan,
 (SELECT count(*) FROM public.project p WHERE p.company_id=c.id) projects,
 (SELECT count(*) FROM public.company_user cu WHERE cu.company_id=c.id) users
 FROM public.company c LEFT JOIN public.payment_plans pp ON pp.id=c.payment_plan_id WHERE $1='' OR strpos(lower(c.name || ' ' || COALESCE(c.domain,'') || ' ' || c.company_id),lower($1))>0),
 page AS (SELECT * FROM matched ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET $2)
 SELECT jsonb_build_object('total',(SELECT count(*) FROM matched),'items',COALESCE((SELECT jsonb_agg(to_jsonb(page)-'id') FROM page),'[]'::jsonb))`, q, offset)
}

func (s *System) Users(w http.ResponseWriter, r *http.Request) {
	q, offset := paging(r)
	s.query(w, r, `WITH matched AS (SELECT u.id, u.subject, COALESCE(NULLIF(u.known_as,''),NULLIF(concat_ws(' ',u.first_name,u.last_name),''),'Unnamed user') name,
 COALESCE(u.email_address,'') email, u.onboarded, u.created_at,
 COALESCE((SELECT jsonb_agg(c.name ORDER BY c.name) FROM public.company c JOIN public.company_user cu ON cu.company_id=c.id WHERE cu.user_id=u.id),'[]'::jsonb) companies
 FROM public."user" u WHERE $1='' OR strpos(lower(concat_ws(' ',u.known_as,u.first_name,u.last_name,u.email_address,u.subject)),lower($1))>0),
 page AS (SELECT * FROM matched ORDER BY created_at DESC, id DESC LIMIT 25 OFFSET $2)
 SELECT jsonb_build_object('total',(SELECT count(*) FROM matched),'items',COALESCE((SELECT jsonb_agg(to_jsonb(page)-'id') FROM page),'[]'::jsonb))`, q, offset)
}

func (s *System) Operations(w http.ResponseWriter, r *http.Request) {
	s.query(w, r, `SELECT jsonb_build_object('database_bytes',pg_database_size(current_database()), 'database_time',now(), 'connections',(SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()), 'migration', (SELECT version FROM public.schema_migrations LIMIT 1), 'migration_dirty',(SELECT dirty FROM public.schema_migrations LIMIT 1), 'archived_flags',(SELECT count(*) FROM public.flag_archive), 'webhook_endpoints',(SELECT count(*) FROM public.webhook_endpoint), 'webhook_pending',(SELECT count(*) FROM public.webhook_delivery WHERE status IN ('pending','delivering')), 'webhook_failed',(SELECT count(*) FROM public.webhook_delivery WHERE status='failed'))`)
}
