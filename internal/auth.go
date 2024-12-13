package internal

import (
	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/user"
	"net/http"
)

func (s *Service) ValidateUser(w http.ResponseWriter, r *http.Request) bool {
	_ = w
	userSubject := r.Header.Get("x-user-subject")
	userAccessToken := r.Header.Get("x-user-access-token")

	// Skip the check and just accept what is passed from bruno
	if s.Config.Local.Development {
		return true
	}

	if userSubject != "" && userAccessToken != "" {
		validateUser := user.NewSystem(s.Config).ValidateUser(r.Context(), userSubject)
		if !validateUser {
			return false
		}
	}

	return true
}

func (s *Service) ValidateAgent(w http.ResponseWriter, r *http.Request) bool {
	_ = w
	agentId := r.Header.Get("x-agent-id")
	projectId := r.Header.Get("x-project-id")
	environmentId := r.Header.Get("x-environment-id")
	validAgent := false

	// Skip the check and just accept what is passed from bruno
	if s.Config.Local.Development {
		return true
	}

	// skip this check since this isn't the agent asking for flags
	if r.URL.Path != "/flags" {
		return true
	}

	if agentId != "" && projectId != "" {
		// validate agent
		if environmentId != "" {
			// validate environment
			v, err := agent.NewSystem(s.Config).ValidateAgentWithEnvironment(r.Context(), agentId, projectId, environmentId)
			if err != nil {
				return false
			}
			validAgent = v
		}

		v, err := agent.NewSystem(s.Config).ValidateAgentWithoutEnvironment(r.Context(), agentId, projectId)
		if err != nil {
			return validAgent
		}
		validAgent = v
	}

	return validAgent
}

func (s *Service) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate User
		if s.ValidateUser(w, r) {
			next.ServeHTTP(w, r)
			return
		}

		// it's not a user so check if it's an agent
		if s.ValidateAgent(w, r) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"intervalAllowed":900, "flags": []}`))
		return
	})
}
