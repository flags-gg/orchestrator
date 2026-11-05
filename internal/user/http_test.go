package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/testdb"
	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptr(s string) *string { return &s }

func TestFirstLoginProvisioning(t *testing.T) {
	db := testdb.Start(t)
	cfg := ConfigBuilder.NewConfigNoVault()
	require.NoError(t, cfg.Build(ConfigBuilder.Database, ConfigBuilder.Bugfixes))
	s := NewSystem(cfg)
	original := getClerkUser
	t.Cleanup(func() { getClerkUser = original })
	getClerkUser = func(_ context.Context, id string) (*clerk.User, error) {
		if id == "clerk-unavailable" {
			return nil, errors.New("upstream unavailable")
		}
		return &clerk.User{ID: id, PrimaryEmailAddressID: ptr("primary"), EmailAddresses: []*clerk.EmailAddress{
			{ID: "other", EmailAddress: "other@example.com"},
			{ID: "primary", EmailAddress: "new@example.com"},
		}}, nil
	}
	request := func(subject string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/user", nil)
		r = r.WithContext(auth.WithUser(r.Context(), auth.User{ID: subject, Email: "new@example.com"}))
		w := httptest.NewRecorder()
		s.GetUser(w, r)
		return w
	}

	t.Run("creates a database user with onboarding incomplete", func(t *testing.T) {
		w := request("new-user")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var user User
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
		assert.True(t, user.Created)
		assert.False(t, user.Onboarded)
		require.NotNil(t, user.Subject)
		assert.Equal(t, "new-user", *user.Subject)
		assert.Equal(t, "new@example.com", *user.Email)
		assert.Equal(t, "new", *user.KnownAs)
		assert.Contains(t, w.Body.String(), `"onboarded":false`)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM public."user" WHERE subject = 'new-user'`).Scan(&count))
		assert.Equal(t, 1, count)
	})

	t.Run("repeat login preserves the saved profile", func(t *testing.T) {
		require.NoError(t, s.CreateUserDetails(context.Background(), "new-user", "Custom name", "custom@example.com", "First", "Last", "London", 1))
		w := request("new-user")
		require.Equal(t, http.StatusOK, w.Code)
		var user User
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &user))
		assert.Equal(t, "Custom name", *user.KnownAs)
		assert.Equal(t, "custom@example.com", *user.Email)
		require.NoError(t, s.createUserIfMissing(context.Background(), "new-user", "Overwrite", "wrong@example.com", "", ""))
		saved, err := s.RetrieveUserDetailsDB(context.Background(), "new-user")
		require.NoError(t, err)
		assert.Equal(t, "Custom name", *saved.KnownAs)
	})

	t.Run("parallel first logins create one row", func(t *testing.T) {
		var wg sync.WaitGroup
		responses := make(chan *httptest.ResponseRecorder, 4)
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); responses <- request("parallel-user") }()
		}
		wg.Wait()
		close(responses)
		for w := range responses {
			assert.Equal(t, http.StatusOK, w.Code)
		}
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM public."user" WHERE subject = 'parallel-user'`).Scan(&count))
		assert.Equal(t, 1, count)
	})

	t.Run("upstream failure is not a missing account or successful creation", func(t *testing.T) {
		assert.Equal(t, http.StatusBadGateway, request("clerk-unavailable").Code)
		var count int
		require.NoError(t, db.QueryRow(`SELECT count(*) FROM public."user" WHERE subject = 'clerk-unavailable'`).Scan(&count))
		assert.Zero(t, count)
	})

	t.Run("unauthenticated requests never provision an account", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.GetUser(w, httptest.NewRequest(http.MethodGet, "/user", nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestLoginProfileNames(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		username, first, last *string
		want                  string
	}{
		{"username", ptr("alice"), ptr("Alice"), ptr("Smith"), "alice"},
		{"full name", nil, ptr("Alice"), ptr("Smith"), "Alice Smith"},
		{"email-only login", nil, nil, nil, "alice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := profileFromClerk(&clerk.User{Username: tc.username, FirstName: tc.first, LastName: tc.last,
				PrimaryEmailAddressID: ptr("email"), EmailAddresses: []*clerk.EmailAddress{{ID: "email", EmailAddress: "alice@example.com"}}})
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.knownAs)
		})
	}
	_, err := profileFromClerk(&clerk.User{})
	assert.Error(t, err)
}
