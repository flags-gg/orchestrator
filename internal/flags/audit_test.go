package flags

import (
	"context"
	"database/sql"
	"testing"

	"github.com/flags-gg/orchestrator/internal/stats"
)

func TestRecordEnvironmentRequestSurvivesCancelledRequest(t *testing.T) {
	ctx := context.Background()

	testDB, err := setupTestDatabase(ctx)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}
	defer func() { _ = testDB.container.Terminate(ctx) }()

	db, err := sql.Open("postgres", testDB.uri)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS public.environment_request_audit (
		project_id TEXT, agent_id TEXT, environment_id TEXT, request_kind TEXT, request_source TEXT)`); err != nil {
		t.Fatalf("Failed to create audit table: %v", err)
	}

	system, _ := setupTestSystem(t)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()

	if err := stats.NewSystem(system.Config).RecordEnvironmentRequest(cancelled, "project", "agent", "environment", stats.RequestKindAllFlags, stats.RequestSourceSDKAll); err != nil {
		t.Fatalf("RecordEnvironmentRequest returned error: %v", err)
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM public.environment_request_audit`).Scan(&rows); err != nil {
		t.Fatalf("Failed to count audit rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 audit row, got %d", rows)
	}
}
