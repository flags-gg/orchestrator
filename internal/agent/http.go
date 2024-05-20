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

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
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

func (s *System) GetAgent(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func (s *System) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func (s *System) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func (s *System) CreateAgent(w http.ResponseWriter, r *http.Request) {

	_, _ = w.Write([]byte(`{"agent": {}}`))
	return
}

func (s *System) GetSecretMenu(w http.ResponseWriter, r *http.Request) {

}

func (s *System) CreateSecretMenu(w http.ResponseWriter, r *http.Request) {

}

func (s *System) UpdateSecretMenu(w http.ResponseWriter, r *http.Request) {

}

func (s *System) GetAgentEnvironments(w http.ResponseWriter, r *http.Request) {

}

func (s *System) CreateAgentEnvironment(w http.ResponseWriter, r *http.Request) {

}

func (s *System) UpdateAgentEnvironment(w http.ResponseWriter, r *http.Request) {

}

func (s *System) DeleteAgentEnvironment(w http.ResponseWriter, r *http.Request) {

}
