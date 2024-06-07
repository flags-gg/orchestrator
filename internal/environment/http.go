package environment

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/secretmenu"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
)

type Environment struct {
	Id            string                `json:"id"`
	Name          string                `json:"name"`
	EnvironmentId string                `json:"environment_id"`
	Enabled       bool                  `json:"enabled"`
	SecretMenu    secretmenu.SecretMenu `json:"secret_menu"`
}

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config:  cfg,
		Context: context.Background(),
	}
}

func (s *System) SetContext(ctx context.Context) *System {
	s.Context = ctx
	return s
}

func (s *System) GetAgentEnvironments(w http.ResponseWriter, r *http.Request) {
	type Environments struct {
		Environments []*Environment `json:"environments"`
	}
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

	agentId := r.PathValue("agentId")
	environments, err := s.GetAgentEnvironmentsFromDB(agentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&Environments{
		Environments: environments,
	}); err != nil {
		_ = logs.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) GetEnvironments(w http.ResponseWriter, r *http.Request) {
	type Environments struct {
		Environments []*Environment `json:"environments"`
	}
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

	agentId := r.PathValue("agentId")
	environments, err := s.GetAgentEnvironmentsFromDB(agentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&Environments{
		Environments: environments,
	}); err != nil {
		_ = logs.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) CreateAgentEnvironment(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) UpdateAgentEnvironment(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) DeleteAgentEnvironment(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) GetEnvironment(w http.ResponseWriter, r *http.Request) {
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

	environmentId := r.PathValue("environmentId")

	sm, err := secretmenu.NewSystem(s.Config).SetContext(s.Context).GetEnvironmentSecretMenu(environmentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	environment, err := s.GetEnvironmentFromDB(environmentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	environment.SecretMenu = sm
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(environment); err != nil {
		_ = logs.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
