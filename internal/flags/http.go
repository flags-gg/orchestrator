package flags

import (
	"context"
	"encoding/json"
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

func (s *System) GetFlags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	w.Header().Set("Content-Type", "application/json")
	s.Context = r.Context()

	isAgent := false
	isClient := false

	if r.Header.Get("x-project-id") != "" && r.Header.Get("x-agent-id") != "" {
		isAgent = true
	}
	if r.Header.Get("x-user-subject") != "" && r.Header.Get("x-user-access-token") != "" {
		isClient = true
	}

	if !isAgent && !isClient {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// get the flags for agent
	if isAgent {
		s.GetFlagsAgent(w, r)
		return
	}

	// get the flags for client
	s.GetFlagsClient(w, r)
}

func (s *System) GetFlagsAgent(w http.ResponseWriter, r *http.Request) {
	responseObj := AgentResponse{}

	projectId := r.Header.Get("x-project-id")
	agentId := r.Header.Get("x-agent-id")
	environmentId := r.Header.Get("x-environment-id")

	res, err := s.GetAgentFlags(projectId, agentId, environmentId)
	if err != nil {
		responseObj = AgentResponse{
			IntervalAllowed: 600,
			Flags:           []Flag{},
		}
		s.Config.Bugfixes.Logger.Fatalf("Failed to get flags: %v", err)
	}
	responseObj = *res

	if err := json.NewEncoder(w).Encode(responseObj); err != nil {
		_, _ = w.Write([]byte(`{"error": "failed to encode response"}`))
		stats.NewSystem(s.Config).AddAgentError(r.Header.Get("x-project-id"), r.Header.Get("x-agent-id"), r.Header.Get("x-environment-id"))
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
	stats.NewSystem(s.Config).AddAgentSuccess(r.Header.Get("x-project-id"), r.Header.Get("x-agent-id"), r.Header.Get("x-environment-id"))
}

func (s *System) GetFlagsClient(w http.ResponseWriter, r *http.Request) {
	responseObj := Response{}
	res, err := s.GetClientFlags(r.Header.Get("x-user-subject"), r.Header.Get("x-user-access-token"))
	if err != nil {
		responseObj = Response{
			Flags: []Flag{},
		}
		s.Config.Bugfixes.Logger.Fatalf("Failed to get flags: %v", err)
	}
	responseObj = res

	if err := json.NewEncoder(w).Encode(responseObj); err != nil {
		_, _ = w.Write([]byte(`{"error": "failed to encode response"}`))
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
}

func (s *System) CreateFlags(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) UpdateFlags(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) DeleteFlags(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}
