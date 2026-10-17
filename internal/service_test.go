package internal

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/secretmenu"
	"github.com/flags-gg/orchestrator/internal/testdb"
	"github.com/golang-jwt/jwt/v5"
	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAuth struct {
	key *rsa.PrivateKey
}

// newTestService builds a production-mode service whose Clerk JWTs are verified against a local test key
// and whose Clerk user lookup is stubbed: emails maps user id -> primary email ("" = unverified).
func newTestService(t *testing.T, emails map[string]string) (*Service, *testAuth) {
	t.Helper()
	c := ConfigBuilder.NewConfigNoVault()
	require.NoError(t, c.Build(ConfigBuilder.Database, ConfigBuilder.Bugfixes))
	c.Local.Development = false
	c.ProjectProperties = map[string]interface{}{"stripe_webhook_secret": "", "jwt_signing_key": "test-signing-key"}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	s := New(c)
	s.jwtOptions = append(s.jwtOptions, clerkhttp.JSONWebKey(pemKey))

	emailCacheMu.Lock()
	emailCache = map[string]emailStatus{}
	emailCacheMu.Unlock()
	orig := getClerkUser
	getClerkUser = func(_ context.Context, id string) (*clerk.User, error) {
		email, ok := emails[id]
		if !ok {
			return nil, io.EOF
		}
		primary := "idn_1"
		status := "verified"
		if email == "" {
			email, status = id+"@unverified.example", "unverified"
		}
		return &clerk.User{
			ID:                    id,
			PrimaryEmailAddressID: &primary,
			EmailAddresses: []*clerk.EmailAddress{
				{ID: primary, EmailAddress: email, Verification: &clerk.Verification{Status: status}},
			},
		}, nil
	}
	t.Cleanup(func() { getClerkUser = orig })

	return s, &testAuth{key: key}
}

func (a *testAuth) token(t *testing.T, sub string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "https://clerk.flags.test",
		"sub": sub,
		"sid": "sess_1",
		"iat": time.Now().Add(-time.Minute).Unix(),
		"nbf": time.Now().Add(-time.Minute).Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString(a.key)
	require.NoError(t, err)
	return signed
}

func do(h http.Handler, method, path, token, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestUserRoutesRequireVerifiedJWT(t *testing.T) {
	s, a := newTestService(t, map[string]string{"user-ok": "ok@a.example", "user-unverified": ""})
	h := s.routes()

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	forged := (&testAuth{key: other}).token(t, "user-ok")

	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/projects", "", "").Code, "no token")
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/projects", "", "", "x-user-subject", "user-ok").Code, "x-user-subject is not trusted")
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/projects", "garbage", "").Code, "malformed token")
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/projects", forged, "").Code, "token signed by another key")
	assert.Equal(t, http.StatusForbidden, do(h, http.MethodGet, "/projects", a.token(t, "user-unverified"), "").Code, "unverified email")
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/projects", a.token(t, "user-unknown"), "").Code, "unknown clerk user")
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPost, "/uploadthing", "", "").Code, "uploadthing requires auth")

	echo := s.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.UserID(r)
		_, _ = w.Write([]byte(id + "|" + auth.EmailDomain(r)))
	}))
	w := do(echo, http.MethodGet, "/", a.token(t, "user-ok"), "", "x-user-subject", "someone-else")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "user-ok|a.example", w.Body.String())
}

func TestDevelopmentBypassUsesDevUser(t *testing.T) {
	s, _ := newTestService(t, nil)
	s.Config.Local.Development = true
	s.Config.Clerk.DevUser = "dev-user"
	echo := s.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.UserID(r)
		_, _ = w.Write([]byte(id))
	}))
	w := do(echo, http.MethodGet, "/", "", "")
	assert.Equal(t, "dev-user", w.Body.String())
}

func TestSDKAndPublicRoutes(t *testing.T) {
	s, _ := newTestService(t, nil)
	h := s.routes()

	for _, path := range []string{"/flags", "/v1/flags"} {
		w := do(h, http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusOK, w.Code, path)
		assert.JSONEq(t, `{"intervalAllowed":900, "flags": []}`, w.Body.String(), path)
	}
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPost, "/ofrep/v1/evaluate/flags", "", "{}").Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodPost, "/ofrep/v1/evaluate/flags/x", "", "{}").Code)

	assert.Equal(t, http.StatusOK, do(h, http.MethodGet, "/health", "", "").Code)
	assert.Equal(t, http.StatusServiceUnavailable, do(h, http.MethodPost, "/webhooks/stripe", "", "{}").Code, "no webhook secret configured")
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodPost, "/events/keycloak", "", "").Code)
}

func TestCORSByRouteGroup(t *testing.T) {
	s, _ := newTestService(t, nil)
	h := s.routes()

	w := do(h, http.MethodOptions, "/projects", "", "", "Origin", "https://evil.example")
	assert.Equal(t, http.StatusForbidden, w.Code)

	w = do(h, http.MethodOptions, "/projects", "", "", "Origin", "https://dashboard.flags.gg")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://dashboard.flags.gg", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "authorization")
	assert.NotContains(t, w.Header().Get("Access-Control-Allow-Headers"), "x-user-subject")

	w = do(h, http.MethodOptions, "/flags", "", "", "Origin", "https://customer-site.example")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestLimitBody(t *testing.T) {
	h := LimitBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
	}))
	assert.Equal(t, http.StatusRequestEntityTooLarge, do(h, http.MethodPost, "/flag", "", strings.Repeat("a", 2<<20)).Code)
	assert.Equal(t, http.StatusOK, do(h, http.MethodPost, "/flag", "", "{}").Code)
}

func count(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(query, args...).Scan(&n))
	return n
}

func TestCrossCompanyIsolation(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-a": "a@a.example", "user-b": "b@b.example", "user-c": "c@a.example", "user-d": "d@d.example"})
	h := s.routes()
	flagA := testdb.FlagID(t, db, "a")
	b := a.token(t, "user-b")

	attacks := []struct{ method, path, body string }{
		{http.MethodGet, "/project/project-a", ""},
		{http.MethodPut, "/project/project-a", `{"name":"pwned","enabled":false}`},
		{http.MethodPut, "/project/project-a/image", `{"image":"x"}`},
		{http.MethodGet, "/project/project-a/limits", ""},
		{http.MethodDelete, "/project/project-a", ""},
		{http.MethodPost, "/project/project-a/agent", `{"name":"x"}`},
		{http.MethodPut, "/agent/agent-a", `{"agent_id":"agent-a","name":"pwned"}`},
		{http.MethodDelete, "/agent/agent-a", ""},
		{http.MethodPost, "/agent/agent-a/environment", `{"name":"x"}`},
		{http.MethodPost, "/agent/agent-a/env-a", `{"name":"x"}`},
		{http.MethodPost, "/agent/agent-b/env-a", `{"name":"x"}`},
		{http.MethodPut, "/environment/env-a", `{"environment_id":"env-a","name":"pwned"}`},
		{http.MethodDelete, "/environment/env-a", ""},
		{http.MethodGet, "/environment/env-a/flags", ""},
		{http.MethodPost, "/flag", `{"name":"x","agentId":"agent-a","environmentId":"env-a"}`},
		{http.MethodPost, "/flag", `{"name":"x","agentId":"agent-b","environmentId":"env-a"}`},
		{http.MethodPatch, "/flag/" + flagA, `{"enabled":true,"name":"flag-a"}`},
		{http.MethodPut, "/flag/" + flagA, `{"name":"pwned"}`},
		{http.MethodPost, "/flag/" + flagA + "/promote", ""},
		{http.MethodDelete, "/flag/" + flagA, ""},
		{http.MethodGet, "/secret-menu/menu-a", ""},
		{http.MethodPost, "/secret-menu/env-a", `{"sequence":["x"]}`},
		{http.MethodPut, "/secret-menu/menu-a/state", ""},
		{http.MethodPut, "/secret-menu/menu-a/sequence", `{"sequence":["x"]}`},
		{http.MethodPost, "/api-key/generate", `{"project_id":"project-a","agent_id":"agent-a"}`},
		{http.MethodPost, "/api-key/generate", `{"project_id":"project-b","agent_id":"agent-b","environment_id":"env-a"}`},
	}
	for _, at := range attacks {
		w := do(h, at.method, at.path, b, at.body)
		assert.Equal(t, http.StatusNotFound, w.Code, "%s %s", at.method, at.path)
	}

	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.project WHERE project_id = 'project-a' AND name = 'project a' AND logo IS NULL`))
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.agent WHERE agent_id = 'agent-a' AND name = 'agent'`))
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.agent WHERE project_id = (SELECT id FROM public.project WHERE project_id = 'project-a')`))
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.environment WHERE agent_id = (SELECT id FROM public.agent WHERE agent_id = 'agent-a')`))
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.flag WHERE name = 'flag-a' AND enabled = false`))
	assert.Equal(t, 0, count(t, db, `SELECT COUNT(*) FROM public.flag WHERE name IN ('x', 'pwned')`))
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.secret_menu WHERE menu_id = 'menu-a' AND code = 'up,down' AND enabled = false`))
	assert.Equal(t, 2, count(t, db, `SELECT COUNT(*) FROM public.secret_menu`))
	assert.Equal(t, 0, count(t, db, `SELECT COUNT(*) FROM public.api_key_audit`))

	// the owner still can
	own := a.token(t, "user-a")
	assert.Equal(t, http.StatusOK, do(h, http.MethodGet, "/project/project-a", own, "").Code)
	assert.Equal(t, http.StatusOK, do(h, http.MethodPatch, "/flag/"+flagA, own, `{"enabled":true,"name":"flag-a"}`).Code)
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.flag WHERE name = 'flag-a' AND enabled = true`))
	assert.Equal(t, http.StatusOK, do(h, http.MethodPost, "/api-key/generate", own, `{"project_id":"project-a","agent_id":"agent-a","environment_id":"env-a"}`).Code)

	// a style id from another menu can't be updated through this menu
	sm := secretmenu.NewSystem(s.Config)
	update := secretmenu.SecretMenu{CustomStyle: secretmenu.MenuStyle{Id: "style-a", SQLHeader: sql.NullString{String: "pwned", Valid: true}}}
	require.NoError(t, sm.UpdateSecretMenuStyleInDB(context.Background(), "menu-b", update))
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM public.secret_menu_style WHERE style_id = 'style-a' AND header = 'original'`))
}

func TestCloneEnvironmentWithQuotedFlagName(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-a": "a@a.example"})
	h := s.routes()

	_, err := db.Exec(`INSERT INTO public.flag (name, agent_id, environment_id, enabled)
		SELECT 'it''s on''); DROP TABLE public.flag; --', agent_id, id, true FROM public.environment WHERE env_id = 'env-a'`)
	require.NoError(t, err)

	w := do(h, http.MethodPost, "/agent/agent-a/env-a", a.token(t, "user-a"), `{"name":"clone"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 2, count(t, db, `
		SELECT COUNT(*) FROM public.flag f JOIN public.environment e ON e.id = f.environment_id
		WHERE e.name = 'clone' AND f.name IN ('flag-a', 'it''s on''); DROP TABLE public.flag; --')`))

	// names outside the allowed charset are rejected on create
	w = do(h, http.MethodPost, "/flag", a.token(t, "user-a"), `{"name":"bad'name","agentId":"agent-a","environmentId":"env-a"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestJoinCompanyRules(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-c": "c@a.example", "user-d": "d@d.example", "user-b": "b@b.example", "user-h": "h@gmail.com"})
	h := s.routes()
	members := `SELECT COUNT(*) FROM public.company_user cu JOIN public."user" u ON u.id = cu.user_id JOIN public.company c ON c.id = cu.company_id WHERE u.subject = $1 AND c.company_id = 'company-a'`

	// domain that isn't the caller's verified email domain
	assert.Equal(t, http.StatusForbidden, do(h, http.MethodPut, "/company/user", a.token(t, "user-d"), `{"domain":"a.example"}`).Code)
	assert.Equal(t, 0, count(t, db, members, "user-d"))

	// matching verified email domain joins
	assert.Equal(t, http.StatusOK, do(h, http.MethodPut, "/company/user", a.token(t, "user-c"), `{"domain":"A.example"}`).Code)
	assert.Equal(t, 1, count(t, db, members, "user-c"))

	// invite codes still work regardless of domain
	assert.Equal(t, http.StatusOK, do(h, http.MethodPut, "/company/user", a.token(t, "user-d"), `{"invite_code":"invite-a"}`).Code)
	assert.Equal(t, 1, count(t, db, members, "user-d"))

	// a second company can't claim an existing domain
	assert.Equal(t, http.StatusConflict, do(h, http.MethodPost, "/company", a.token(t, "user-b"), `{"name":"dup","domain":"A.EXAMPLE"}`).Code)

	// free-mail users can't domain-join a company claiming their provider's domain, only by invite
	_, err := db.Exec(`INSERT INTO public."user" (subject, email_address) VALUES ('user-h', 'h@gmail.com');
		INSERT INTO public.company (name, company_id, invite_code, domain) VALUES ('GM', 'company-gm', 'invite-gm', 'gmail.com')`)
	require.NoError(t, err)
	gmMembers := `SELECT COUNT(*) FROM public.company_user cu JOIN public."user" u ON u.id = cu.user_id JOIN public.company c ON c.id = cu.company_id WHERE u.subject = $1 AND c.domain = 'gmail.com'`
	assert.Equal(t, http.StatusForbidden, do(h, http.MethodPut, "/company/user", a.token(t, "user-h"), `{"domain":"gmail.com"}`).Code)
	assert.Equal(t, 0, count(t, db, gmMembers, "user-h"))
	assert.Equal(t, http.StatusOK, do(h, http.MethodPut, "/company/user", a.token(t, "user-h"), `{"invite_code":"invite-gm"}`).Code)
	assert.Equal(t, 1, count(t, db, gmMembers, "user-h"))
}

func TestGetEnvironmentIncludesAgent(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-a": "a@a.example"})

	// the dashboard creates flags with this agent_id, so it must come from the environment, not client state
	w := do(s.routes(), http.MethodGet, "/environment/env-a", a.token(t, "user-a"), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"agent_id":"agent-a"`)
}
