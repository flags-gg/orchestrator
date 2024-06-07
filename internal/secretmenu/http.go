package secretmenu

import (
	"encoding/json"
	"github.com/flags-gg/orchestrator/internal/company"
	"net/http"
)

func (s *System) GetSecretMenu(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-access-token") == "" || r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	envId := r.PathValue("environmentId")

	secretMenu, err := s.GetEnvironmentSecretMenu(envId)
	if err != nil {
		s.Config.Bugfixes.Logger.Fatalf("Failed to get secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(secretMenu); err != nil {
		s.Config.Bugfixes.Logger.Fatalf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) CreateSecretMenu(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) UpdateSecretMenu(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
