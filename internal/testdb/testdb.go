// Package testdb starts a throwaway Postgres with the real k8s/schema migrations, for integration tests.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Start returns an open *sql.DB on a migrated database and points the RDS_* env (used by go-config) at it.
func Start(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:16-alpine",
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor: wait.ForSQL("5432/tcp", "postgres", func(host string, port network.Port) string {
				return fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())
			}).WithStartupTimeout(2 * time.Minute),
			Env: map[string]string{"POSTGRES_DB": "testdb", "POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test"},
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}
	t.Setenv("RDS_HOSTNAME", host)
	t.Setenv("RDS_PORT", port.Port())
	t.Setenv("RDS_USERNAME", "test")
	t.Setenv("RDS_PASSWORD", "test")
	t.Setenv("RDS_DB", "testdb")

	db, err := sql.Open("postgres", fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port()))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, file, _, _ := runtime.Caller(0)
	schema := filepath.Join(filepath.Dir(file), "..", "..", "k8s", "schema")
	if _, err := os.Stat(schema); err != nil {
		t.Fatalf("schema dir: %v", err)
	}
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		t.Fatalf("migrate driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+schema, "postgres", driver)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := m.Up(); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	return db
}

// Seed creates two companies (a, b) each with a user, project, agent, environment, flag and secret menu,
// plus two unattached users (user-c with an a.example email, user-d) and plans 1 (free) and 2 (paid).
func Seed(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`
	INSERT INTO public.payment_plans (id, name, price, stripe_id, stripe_id_dev) VALUES
	  (1, 'Free', 0, NULL, NULL),
	  (2, 'Startup', 10, 'price_live', 'price_dev');

	INSERT INTO public."user" (subject, email_address) VALUES
	  ('user-a', 'a@a.example'), ('user-b', 'b@b.example'), ('user-c', 'c@a.example'), ('user-d', 'd@d.example');

	INSERT INTO public.company (name, company_id, invite_code, domain) VALUES
	  ('A', 'company-a', 'invite-a', 'a.example'), ('B', 'company-b', 'invite-b', 'b.example');

	INSERT INTO public.company_user (company_id, user_id)
	SELECT c.id, u.id FROM public.company c JOIN public."user" u ON u.subject = 'user-' || lower(c.name);

	INSERT INTO public.project (name, company_id, project_id, enabled)
	SELECT 'project ' || lower(name), id, 'project-' || lower(name), true FROM public.company;

	INSERT INTO public.agent (name, project_id, agent_id)
	SELECT 'agent', p.id, replace(p.project_id, 'project-', 'agent-') FROM public.project p;

	INSERT INTO public.environment (name, env_id, agent_id, level, "default")
	SELECT 'dev', replace(a.agent_id, 'agent-', 'env-'), a.id, 0, true FROM public.agent a;

	INSERT INTO public.flag (name, agent_id, environment_id, enabled)
	SELECT 'flag-' || replace(e.env_id, 'env-', ''), e.agent_id, e.id, false FROM public.environment e;

	INSERT INTO public.secret_menu (menu_id, environment_id, agent_id, code, enabled)
	SELECT replace(e.env_id, 'env-', 'menu-'), e.id, e.agent_id, 'up,down', false FROM public.environment e;

	INSERT INTO public.secret_menu_style (secret_menu_id, style_id, header)
	SELECT sm.id, replace(sm.menu_id, 'menu-', 'style-'), 'original' FROM public.secret_menu sm;
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// FlagID returns the numeric id of the seeded flag for company "a" or "b".
func FlagID(t *testing.T, db *sql.DB, suffix string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id::text FROM public.flag WHERE name = $1`, "flag-"+suffix).Scan(&id); err != nil {
		t.Fatalf("flag id: %v", err)
	}
	return id
}
