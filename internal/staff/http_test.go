package staff

import (
	"net/http/httptest"
	"testing"

	"github.com/flags-gg/orchestrator/internal/auth"
	ConfigBuilder "github.com/keloran/go-config"
)

func TestStaffGuard(t *testing.T) {
	for _, tc := range []struct {
		name, configured, user, company string
		status                          int
	}{
		{name: "unconfigured", user: "user_staff", status: 404},
		{name: "whitespace config", configured: "  ", user: "user_staff", status: 404},
		{name: "anonymous", configured: "user_staff", status: 404},
		{name: "other user", configured: "user_staff", user: "user_other", status: 404},
		{name: "company token", configured: "user_staff", company: "user_staff", status: 404},
		{name: "case differs", configured: "user_staff", user: "user_STAFF", status: 404},
		{name: "staff", configured: "user_staff", user: "user_staff", status: 200},
		{name: "trim configured ID", configured: " user_staff ", user: "user_staff", status: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(&ConfigBuilder.Config{ProjectProperties: map[string]interface{}{"staff_user_id": tc.configured}})
			r := httptest.NewRequest("GET", "/staff/me", nil)
			// Client-supplied identity headers never grant staff access.
			r.Header.Set("x-user-subject", "user_staff")
			if tc.user != "" {
				r = r.WithContext(auth.WithUser(r.Context(), auth.User{ID: tc.user}))
			}
			if tc.company != "" {
				r = r.WithContext(auth.WithCompany(r.Context(), tc.company))
			}
			w := httptest.NewRecorder()
			s.Only(s.Me).ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d", w.Code, tc.status)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("staff response may be cached")
			}
		})
	}
}
