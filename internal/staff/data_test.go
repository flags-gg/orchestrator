package staff

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/flags-gg/orchestrator/internal/testdb"
	ConfigBuilder "github.com/keloran/go-config"
)

func TestPlatformData(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	cfg := ConfigBuilder.NewConfigNoVault()
	if err := cfg.Build(ConfigBuilder.Database); err != nil {
		t.Fatal(err)
	}
	s := New(cfg)
	checks := []struct {
		path  string
		run   func(*httptest.ResponseRecorder)
		check func(map[string]any)
	}{
		{"/staff/stats", func(w *httptest.ResponseRecorder) { s.Stats(w, httptest.NewRequest("GET", "/staff/stats", nil)) }, func(data map[string]any) {
			if data["companies"].(map[string]any)["total"] != float64(2) {
				t.Fatal(data)
			}
		}},
		{"/staff/companies?q=a.example", func(w *httptest.ResponseRecorder) {
			s.Companies(w, httptest.NewRequest("GET", "/staff/companies?q=a.example", nil))
		}, func(data map[string]any) {
			if data["total"] != float64(1) {
				t.Fatal(data)
			}
		}},
		{"/staff/users", func(w *httptest.ResponseRecorder) { s.Users(w, httptest.NewRequest("GET", "/staff/users", nil)) }, func(data map[string]any) {
			if data["total"] != float64(4) {
				t.Fatal(data)
			}
		}},
		{"/staff/operations", func(w *httptest.ResponseRecorder) {
			s.Operations(w, httptest.NewRequest("GET", "/staff/operations", nil))
		}, func(data map[string]any) {
			if data["migration_dirty"] != false {
				t.Fatal(data)
			}
		}},
	}
	for _, tc := range checks {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.run(w)
			if w.Code != 200 {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
			var data map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			tc.check(data)
		})
	}
}
