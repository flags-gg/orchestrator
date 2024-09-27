package environment

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/flags"
	"github.com/flags-gg/orchestrator/internal/secretmenu"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
)

type Environment struct {
	Id            string                `json:"id"`
	Name          string                `json:"name"`
	EnvironmentId string                `json:"environment_id"`
	Enabled       bool                  `json:"enabled"`
	SecretMenu    secretmenu.SecretMenu `json:"secret_menu,omitempty"`
	Flags         []flags.Flag          `json:"flags,omitempty"`
	ProjectName   string                `json:"project_name,omitempty"`
	AgentName     string                `json:"agent_name,omitempty"`
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

	type envCreate struct {
		Name string `json:"name"`
	}

	agentId := r.PathValue("agentId")
	var env envCreate
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = s.CreateEnvironmentInDB(env.Name, agentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *System) UpdateEnvironment(w http.ResponseWriter, r *http.Request) {
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
	var env Environment
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if env.EnvironmentId != environmentId {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.UpdateEnvironmentInDB(env); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update environment: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteEnvironment(w http.ResponseWriter, r *http.Request) {
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
	if err := flags.NewSystem(s.Config).SetContext(r.Context()).DeleteAllFlagsForEnv(environmentId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to delete flags: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.DeleteEnvironmentFromDB(environmentId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to delete environment: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
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
