package environment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	ConfigBuilder "github.com/keloran/go-config"
	_ "github.com/lib/pq"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// setupCloneDB starts postgres with the real migrations applied and one agent with a "dev" environment holding two flags.
func setupCloneDB(t *testing.T) (*sql.DB, *System) {
	t.Helper()
	ctx := context.Background()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:14-alpine",
			ExposedPorts: []string{"5432/tcp"},
			WaitingFor: wait.ForAll(
				wait.ForLog("database system is ready to accept connections"),
				wait.ForSQL("5432/tcp", "postgres", func(host string, port network.Port) string {
					return fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())
				}),
			).WithDeadline(2 * time.Minute),
			Env: map[string]string{"POSTGRES_DB": "testdb", "POSTGRES_USER": "test", "POSTGRES_PASSWORD": "test"},
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, _ := container.Host(ctx)
	port, _ := container.MappedPort(ctx, "5432")
	_ = os.Setenv("RDS_HOSTNAME", host)
	_ = os.Setenv("RDS_PORT", port.Port())
	_ = os.Setenv("RDS_USERNAME", "test")
	_ = os.Setenv("RDS_PASSWORD", "test")
	_ = os.Setenv("RDS_DB", "testdb")

	db, err := sql.Open("postgres", fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port()))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migrations, _ := filepath.Glob("../../k8s/schema/*.up.sql")
	sort.Strings(migrations)
	for _, m := range migrations {
		body, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("apply %s: %v", m, err)
		}
	}

	if _, err := db.Exec(`
		INSERT INTO public.agent (name, project_id, agent_id) VALUES ('agent', 1, 'agent-1');
		INSERT INTO public.environment (name, env_id, agent_id, level) VALUES ('dev', 'env-dev', 1, 0);
		INSERT INTO public.flag (name, agent_id, environment_id, enabled) VALUES ('a', 1, 1, true), ('b', 1, 1, false);`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := ConfigBuilder.NewConfigNoVault()
	if err := c.Build(ConfigBuilder.Database); err != nil {
		t.Fatalf("build config: %v", err)
	}
	return db, NewSystem(c)
}

func TestCloneChildEnvironment(t *testing.T) {
	db, s := setupCloneDB(t)
	ctx := context.Background()

	if err := s.CloneChildEnvironmentInDB(ctx, "env-dev", "env-live", "agent-1", "live"); err != nil {
		t.Fatalf("clone: %v", err)
	}

	var flags int
	_ = db.QueryRow(`SELECT count(*) FROM public.flag f JOIN public.environment e ON e.id = f.environment_id WHERE e.env_id = 'env-live'`).Scan(&flags)
	if flags != 2 {
		t.Fatalf("child flags = %d, want 2", flags)
	}
	var child string
	_ = db.QueryRow(`SELECT c.env_id FROM public.environment_chain ec JOIN public.environment c ON c.id = ec.child_environment_id`).Scan(&child)
	if child != "env-live" {
		t.Fatalf("chain child = %q, want env-live", child)
	}

	// a parent only has one child: a second clone must fail and leave nothing behind
	if err := s.CloneChildEnvironmentInDB(ctx, "env-dev", "env-qa", "agent-1", "qa"); !errors.Is(err, ErrHasChild) {
		t.Fatalf("second clone err = %v, want ErrHasChild", err)
	}
	var envs int
	_ = db.QueryRow(`SELECT count(*) FROM public.environment`).Scan(&envs)
	if envs != 2 {
		t.Fatalf("environments = %d, want 2 (no orphan from the rejected clone)", envs)
	}
}
