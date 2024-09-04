package flags

import (
	"context"
	"encoding/json"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/stats"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
	"strconv"
	"time"
)

type SecretMenuStyle struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type SecretMenu struct {
	Sequence []string          `json:"sequence,omitempty"`
	Styles   []SecretMenuStyle `json:"styles,omitempty"`
}
type Details struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}
type Flag struct {
	Enabled bool    `json:"enabled"`
	Details Details `json:"details"`
}
type AgentResponse struct {
	IntervalAllowed int        `json:"intervalAllowed,omitempty"`
	SecretMenu      SecretMenu `json:"secretMenu,omitempty"`
	Flags           []Flag     `json:"flags,omitempty"`
}
type Response struct {
	Flags []Flag `json:"flags"`
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

func (s *System) SetContext(ctx context.Context) *System {
	s.Context = ctx
	return s
}

func (s *System) GetAgentFlags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	w.Header().Set("Content-Type", "application/json")
	s.Context = r.Context()

	if r.Header.Get("x-project-id") == "" || r.Header.Get("x-agent-id") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	responseObj := AgentResponse{}

	projectId := r.Header.Get("x-project-id")
	agentId := r.Header.Get("x-agent-id")
	environmentId := r.Header.Get("x-environment-id")

	res, err := s.GetAgentFlagsFromDB(projectId, agentId, environmentId)
	if err != nil {
		responseObj = AgentResponse{
			IntervalAllowed: 600,
			Flags:           []Flag{},
		}
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get flags: %v", err)
	}
	responseObj = *res

	if err := json.NewEncoder(w).Encode(responseObj); err != nil {
		_, _ = w.Write([]byte(`{"error": "failed to encode response"}`))
		stats.NewSystem(s.Config).AddAgentError(projectId, agentId, environmentId)
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
	stats.NewSystem(s.Config).AddAgentSuccess(projectId, agentId, environmentId)
}

func (s *System) GetClientFlags(w http.ResponseWriter, r *http.Request) {
	responseObj := []Flag{}
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

	res, err := s.GetClientFlagsFromDB(environmentId)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get flags: %v", err)
	}
	responseObj = res

	if err := json.NewEncoder(w).Encode(responseObj); err != nil {
		_, _ = w.Write([]byte(`{"error": "failed to encode response"}`))
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
}

func (s *System) CreateFlags(w http.ResponseWriter, r *http.Request) {
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

	flag := flagCreate{}
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.CreateFlagInDB(flag); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateFlags(w http.ResponseWriter, r *http.Request) {
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

	type changeRequest struct {
		Enabled bool   `json:"enabled"`
		Name    string `json:"name"`
	}

	cr := changeRequest{}
	if err := json.NewDecoder(r.Body).Decode(&cr); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	flagChange := Flag{
		Enabled: cr.Enabled,
		Details: Details{
			Name: cr.Name,
			ID:   r.PathValue("flagId"),
		},
	}
	if err := s.UpdateFlagInDB(flagChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return

	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteFlags(w http.ResponseWriter, r *http.Request) {
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

	f := Flag{
		Details: Details{
			ID: r.PathValue("flagId"),
		},
	}

	if err := s.DeleteFlagFromDB(f); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to delete flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
