package service

import (
	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/user"
	"net/http"
)

func (s *Service) ValidateUser(w http.ResponseWriter, r *http.Request) bool {
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

func (s *Service) ValidateAgent(w http.ResponseWriter, r *http.Request) bool {
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

func (s *Service) Auth(next http.Handler) http.Handler {
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
