package agent

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/config"
	"net/http"
)

type Environment struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type AgentDetails struct {
	Id           string        `json:"id"`
	Name         string        `json:"name"`
	Environments []Environment `json:"environments"`
}

type System struct {
	Config  *config.Config
	Context context.Context
}

func NewAgentSystem(cfg *config.Config) *System {
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

func GetAgents(w http.ResponseWriter, r *http.Request) {
	type Agents struct {
		Agents []AgentDetails `json:"agents"`
	}

	agents := Agents{
		Agents: []AgentDetails{
			{
				Id:   "bob",
				Name: "Agent 123",
				Environments: []Environment{
					{
						Id:   "321",
						Name: "Development",
					},
					{
						Id:   "3211",
						Name: "Production",
					},
				},
			},
			{
				Id:   "bill",
				Name: "Agent 456",
				Environments: []Environment{
					{
						Id:   "654",
						Name: "Development",
					},
					{
						Id:   "6541",
						Name: "Production",
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&agents); err != nil {
		_ = logs.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func GetAgent(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"agent": {}}`))
	return
}

func UpdateAgent(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"agent": {}}`))
	return
}

func DeleteAgent(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(`{"agent": {}}`))
	return
}

func (s *System) CreateAgent(w http.ResponseWriter, r *http.Request) {

	w.Write([]byte(`{"agent": {}}`))
	return
}

func GetSecretMenu(w http.ResponseWriter, r *http.Request) {

}

func CreateSecretMenu(w http.ResponseWriter, r *http.Request) {

}

func UpdateSecretMenu(w http.ResponseWriter, r *http.Request) {

}
