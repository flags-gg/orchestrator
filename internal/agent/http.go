package agent

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/environment"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
)

type Details struct {
	Id           string                    `json:"id"`
	Name         string                    `json:"name"`
	Environments []environment.Environment `json:"environments"`
	Enabled      bool                      `json:"enabled"`
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

func (s *System) GetAgentsRequest(w http.ResponseWriter, r *http.Request) {
	type Agents struct {
		Agents []*Agent `json:"agents"`
	}
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(usr.ID)
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

func (s *System) GetProjectAgents(w http.ResponseWriter, r *http.Request) {
	type Agents struct {
		Agents []*Agent `json:"agents"`
	}
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(usr.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	projectId := r.PathValue("projectId")
	agents, err := s.GetAgentsForProject(companyId, projectId)
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
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(usr.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	agentId := r.PathValue("agentId")
	details, err := s.GetAgentDetails(agentId, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
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
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(usr.ID)
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

	if err := s.UpdateAgentDetails(agent); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(usr.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	agentId := r.PathValue("agentId")
	if err := environment.NewSystem(s.Config).SetContext(s.Context).DeleteAllEnvironmentsForAgent(agentId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.DeleteAgentFromDB(agentId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) CreateAgent(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(usr.ID)
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

	agentId, err := s.CreateAgentForProject(agent.Name, projectId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, err = environment.NewSystem(s.Config).SetContext(s.Context).CreateEnvironmentInDB("Default Env", agentId)
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
