package agent

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
)

type Environment struct {
	Id            string `json:"id"`
	Name          string `json:"name"`
	EnvironmentId string `json:"environment_id"`
}

type AgentDetails struct {
	Id           string        `json:"id"`
	Name         string        `json:"name"`
	Environments []Environment `json:"environments"`
}

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

func NewAgentSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) ValidateAgentWithEnvironment(ctx context.Context, agentId, companyId, environmentId string) bool {
	s.Context = ctx

	return true
}

func (s *System) ValidateAgentWithoutEnvironment(ctx context.Context, agentId, companyId string) bool {
	s.Context = ctx

	return true
}

func (s *System) GetAgentsRequest(w http.ResponseWriter, r *http.Request) {
	type Agents struct {
		Agents []*Agent `json:"agents"`
	}
	s.Context = r.Context()

	if r.Header.Get("x-user-access-token") == "" || r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := s.GetCompanyId(r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	agents, err := s.GetAgents(companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&Agents{
		Agents: agents,
	}); err != nil {
		_ = logs.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func GetAgent(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func UpdateAgent(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func DeleteAgent(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func (s *System) CreateAgent(w http.ResponseWriter, r *http.Request) {

	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func GetSecretMenu(w http.ResponseWriter, r *http.Request) {

}

func CreateSecretMenu(w http.ResponseWriter, r *http.Request) {

}

func UpdateSecretMenu(w http.ResponseWriter, r *http.Request) {

}
