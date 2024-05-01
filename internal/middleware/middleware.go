package middleware

import (
	"net/http"
	"strings"

	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/config"
	"github.com/flags-gg/orchestrator/internal/user"
)

type System struct {
	Config      *config.Config
	Middlewares []func(handler http.Handler) http.Handler
}

func NewMiddleware(cfg *config.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		originalOrigin := r.Header.Get("Origin")

		allowedOrigins := []string{
			"https://www.flags.gg",
			"https://flags.gg",
		}
		if s.Config.Local.Development {
			allowedOrigins = append(allowedOrigins, "http://localhost:3000", "http://localhost:5173")
		}

		isAllowed := false
		//// force allow, so it can force the reset time to 10min later
		//if r.Header.Get("x-company-id") == "" || r.Header.Get("x-agent-id") == "" {
		//	isAllowed = true
		//}
		//if r.Header.Get("x-user-subject") != "" && r.Header.Get("x-user-access-token") != "" {
		//	isAllowed = true
		//}

		for _, origin := range allowedOrigins {
			if origin == originalOrigin {
				isAllowed = true
				break
			}
		}
		if !isAllowed {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", originalOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", getHeadersAllowed())
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
func getHeadersAllowed() string {
	standardAllowed := []string{
		"Accept",
		"Content-Type",
	}

	agentHeaders := []string{
		"x-agent-id",
		"x-company-id",
		"x-environment-id",
	}

	userHeaders := []string{
		"x-user-subject",
		"x-user-access-token",
	}

	allowedHeaders := append(standardAllowed, agentHeaders...)
	allowedHeaders = append(allowedHeaders, userHeaders...)

	return strings.Join(allowedHeaders, ", ")
}

func (s *System) ValidateUser(w http.ResponseWriter, r *http.Request) bool {
	userSubject := r.Header.Get("x-user-subject")
	userAccessToken := r.Header.Get("x-user-access-token")

	if userSubject != "" && userAccessToken != "" {
		validateUser := user.NewUserSystem(s.Config).ValidateUser(r.Context(), userSubject)
		if !validateUser {
			w.WriteHeader(http.StatusForbidden)
			return false
		}
	}

	return true
}

func (s *System) ValidateAgent(w http.ResponseWriter, r *http.Request) bool {
	agentId := r.Header.Get("x-agent-id")
	companyId := r.Header.Get("x-company-id")
	environmentId := r.Header.Get("x-environment-id")
	if agentId != "" && companyId != "" {
		validAgent := false

		// validate agent
		if environmentId != "" {
			// validate environment
			validAgent = agent.NewAgentSystem(s.Config).ValidateAgentWithEnvironment(r.Context(), agentId, companyId, environmentId)
		}

		validAgent = agent.NewAgentSystem(s.Config).ValidateAgentWithoutEnvironment(r.Context(), agentId, companyId)
		if !validAgent {
			_, _ = w.Write([]byte(`{"intervalAllowed":900, "flags": []}`))
			return false
		}
	}

	return true
}

func (s *System) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate User
		if valid := s.ValidateUser(w, r); !valid {
			return
		}

		// Validate Agent
		if valid := s.ValidateAgent(w, r); !valid {
			return
		}

		// Continue to processing
		next.ServeHTTP(w, r)
	})
}

func (s *System) AddMiddleware(middlewares ...func(handler http.Handler) http.Handler) {
	s.Middlewares = append(s.Middlewares, middlewares...)
}

func (s *System) Handler(h http.Handler) http.Handler {
	if len(s.Middlewares) == 0 {
		return h
	}

	for _, middleware := range s.Middlewares {
		h = middleware(h)
	}

	return h
}
