package agent

import (
	"encoding/json"
	"net/http"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/environment"
	ConfigBuilder "github.com/keloran/go-config"
)

type Details struct {
	Id           string                    `json:"id"`
	Name         string                    `json:"name"`
	Environments []environment.Environment `json:"environments"`
	Enabled      bool                      `json:"enabled"`
}

type System struct {
	Config *ConfigBuilder.Config
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

// getUserId returns the user ID, using dev mode config if in development, otherwise Clerk
func (s *System) getUserId(r *http.Request) (string, error) {
	if s.Config.Local.Development && s.Config.Clerk.DevUser != "" {
		return s.Config.Clerk.DevUser, nil
	}

	// Production mode: use Clerk authentication
	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(r.Context(), r.Header.Get("x-user-subject"))
	if err != nil {
		return "", err
	}
	return usr.ID, nil
}

func (s *System) GetAgentsRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type Agents struct {
		Agents []*Agent `json:"agents"`
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	agents, err := s.GetAgents(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
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

func (s *System) GetProjectAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	type Agents struct {
		Agents []*Agent `json:"agents"`
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	projectId := r.PathValue("projectId")
	agents, err := s.GetAgentsForProject(ctx, companyId, projectId)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
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
	ctx := r.Context()

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	agentId := r.PathValue("agentId")
	details, err := s.GetAgentDetails(ctx, agentId, companyId)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(details); err != nil {
		_ = logs.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	agentId := r.PathValue("agentId")
	agent := Agent{}
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if agentId != agent.AgentId {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.UpdateAgentDetails(ctx, agent); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	agentId := r.PathValue("agentId")
	if err := environment.NewSystem(s.Config).DeleteAllEnvironmentsForAgent(ctx, agentId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.DeleteAgentFromDB(ctx, agentId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) CreateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	projectId := r.PathValue("projectId")
	agent := Agent{}
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	agentId, err := s.CreateAgentForProject(ctx, agent.Name, projectId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = environment.NewSystem(s.Config).CreateEnvironmentInDB(ctx, "Default Env", agentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	agent.AgentId = agentId
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(agent); err != nil {
		_ = logs.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
