package internal

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/flags"
)

// getClerkUser is a seam for tests; production uses the Clerk API.
var getClerkUser = clerkUser.Get

const emailCacheTTL = 5 * time.Minute

type emailStatus struct {
	email    string
	verified bool
	expires  time.Time
}

// ponytail: per-process map, entries are overwritten not evicted; bounded by user count.
var (
	emailCacheMu sync.Mutex
	emailCache   = map[string]emailStatus{}
)

// primaryEmail returns the user's primary email and whether it is verified, cached for emailCacheTTL.
func primaryEmail(ctx context.Context, userId string) (emailStatus, error) {
	emailCacheMu.Lock()
	cached, ok := emailCache[userId]
	emailCacheMu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached, nil
	}

	usr, err := getClerkUser(ctx, userId)
	if err != nil {
		return emailStatus{}, err
	}
	status := emailStatus{expires: time.Now().Add(emailCacheTTL)}
	for _, e := range usr.EmailAddresses {
		if e != nil && usr.PrimaryEmailAddressID != nil && e.ID == *usr.PrimaryEmailAddressID {
			status.email = e.EmailAddress
			status.verified = e.Verification != nil && e.Verification.Status == "verified"
		}
	}

	emailCacheMu.Lock()
	emailCache[userId] = status
	emailCacheMu.Unlock()
	return status, nil
}

// RequireUser verifies the Clerk session JWT from "Authorization: Bearer <token>" (JWKS cached locally
// by the Clerk SDK), requires a verified primary email, and puts the user into the request context.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	verified := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := clerk.SessionClaimsFromContext(r.Context())
		if !ok || claims == nil || claims.Subject == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		status, err := primaryEmail(r.Context(), claims.Subject)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if !status.verified {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: claims.Subject, Email: status.email})))
	})
	withJWT := clerkhttp.WithHeaderAuthorization(s.jwtOptions...)(verified)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Local development bypass, e.g. Bruno without a Clerk session
		if s.Config.Local.Development && s.Config.Clerk.DevUser != "" {
			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.User{ID: s.Config.Clerk.DevUser})))
			return
		}
		withJWT.ServeHTTP(w, r)
	})
}

// RequireAgent validates SDK credentials (X-API-Key JWT, or x-project-id/x-agent-id[/x-environment-id]).
func (s *Service) RequireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.ValidateAgent(r) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/ofrep/") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errorCode":"INVALID_CONTEXT","errorDetails":"invalid credentials"}`))
			return
		}
		// SDKs treat this as "no flags", keep the shape they already handle
		_, _ = w.Write([]byte(`{"intervalAllowed":900, "flags": []}`))
	})
}

func (s *Service) ValidateAgent(r *http.Request) bool {
	// Skip the check and just accept what is passed from bruno
	if s.Config.Local.Development {
		return true
	}

	projectId := r.Header.Get("x-project-id")
	agentId := r.Header.Get("x-agent-id")
	environmentId := r.Header.Get("x-environment-id")
	if apiKey := r.Header.Get("x-api-key"); apiKey != "" {
		if claims, err := flags.NewAPIKeySystem(s.Config).ValidateAPIKey(apiKey); err == nil {
			projectId, agentId, environmentId = claims.ProjectID, claims.AgentID, claims.EnvironmentID
		}
	}
	if agentId == "" || projectId == "" {
		return false
	}

	ctx := r.Context()
	var valid bool
	var err error
	if environmentId != "" {
		valid, err = agent.NewSystem(s.Config).ValidateAgentWithEnvironment(ctx, agentId, projectId, environmentId)
	} else {
		valid, err = agent.NewSystem(s.Config).ValidateAgentWithoutEnvironment(ctx, agentId, projectId)
	}
	return err == nil && valid
}

// LimitBody caps request bodies; uploads get more room for multipart files.
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := int64(1 << 20)
		if r.URL.Path == "/uploadthing" {
			limit = 32 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		next.ServeHTTP(w, r)
	})
}
