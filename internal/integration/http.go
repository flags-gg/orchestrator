// Package integration is the machine API for ticket trackers (Tracklines): a flag per ticket that is created
// in the first environment, toggled, promoted along the environment chain, and archived when done.
// The tracker decides which board column triggers which call; every call here is idempotent.
package integration

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/flags"
	ConfigBuilder "github.com/keloran/go-config"
)

type System struct {
	Config *ConfigBuilder.Config
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{Config: cfg}
}

func validTicket(t string) bool {
	return t != "" && len(t) <= 255 && !strings.ContainsAny(t, " /\t\n")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *System) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	case errors.Is(err, ErrNoEnvironment), errors.Is(err, ErrNameTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		_ = s.Config.Bugfixes.Logger.Errorf("integration: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// ticket returns the caller's company and the validated ticket key, or writes the error.
func ticket(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	companyId, key := auth.CompanyID(r), r.PathValue("ticket")
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return "", "", false
	}
	if !validTicket(key) {
		w.WriteHeader(http.StatusBadRequest)
		return "", "", false
	}
	return companyId, key, true
}

// GetFlag: GET /integration/tickets/{ticket}/flag
func (s *System) GetFlag(w http.ResponseWriter, r *http.Request) {
	companyId, key, ok := ticket(w, r)
	if !ok {
		return
	}
	tf, err := s.GetTicketFlag(r.Context(), companyId, key)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tf)
}

// CreateFlag: PUT /integration/tickets/{ticket}/flag {"agent_id","name"}; 201 when created, 200 when it already existed.
func (s *System) CreateFlag(w http.ResponseWriter, r *http.Request) {
	companyId, key, ok := ticket(w, r)
	if !ok {
		return
	}
	var req struct {
		AgentId string `json:"agent_id"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AgentId == "" || !flags.ValidFlagName(req.Name) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	tf, created, err := s.CreateTicketFlag(r.Context(), companyId, key, req.AgentId, req.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, tf)
}

// SetFlag: PATCH /integration/tickets/{ticket}/flag {"environment_id", "enabled"?, "rollout"?, "rules"?}; at least one
// of the three. Rules replace the environment's rules ([] clears them).
func (s *System) SetFlag(w http.ResponseWriter, r *http.Request) {
	companyId, key, ok := ticket(w, r)
	if !ok {
		return
	}
	var req struct {
		EnvironmentId string        `json:"environment_id"`
		Enabled       *bool         `json:"enabled"`
		Rollout       *int          `json:"rollout"`
		Rules         *[]flags.Rule `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.EnvironmentId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c := FlagChange{Enabled: req.Enabled, Rollout: req.Rollout, Rules: req.Rules}
	if err := c.Valid(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.SetTicketFlag(r.Context(), companyId, key, req.EnvironmentId, c); err != nil {
		s.fail(w, err)
		return
	}
	s.GetFlag(w, r)
}

// PromoteFlag: POST /integration/tickets/{ticket}/flag/promote[?to=production]
func (s *System) PromoteFlag(w http.ResponseWriter, r *http.Request) {
	companyId, key, ok := ticket(w, r)
	if !ok {
		return
	}
	to := r.URL.Query().Get("to")
	if to != "" && to != "production" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	tf, err := s.PromoteTicketFlag(r.Context(), companyId, key, to == "production")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tf)
}

// ArchiveFlag: DELETE /integration/tickets/{ticket}/flag; 204 even when there was nothing to archive.
func (s *System) ArchiveFlag(w http.ResponseWriter, r *http.Request) {
	companyId, key, ok := ticket(w, r)
	if !ok {
		return
	}
	if err := s.ArchiveTicketFlag(r.Context(), companyId, key); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// CreateTokenHandler: POST /company/integration-token {"name"} (dashboard user); the token is only returned here.
func (s *System) CreateTokenHandler(w http.ResponseWriter, r *http.Request) {
	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || len(req.Name) > 255 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	raw, err := s.CreateToken(r.Context(), companyId, req.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name, "token": raw})
}

// ListTokensHandler: GET /company/integration-tokens
func (s *System) ListTokensHandler(w http.ResponseWriter, r *http.Request) {
	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r)
	if !ok {
		return
	}
	tokens, err := s.ListTokens(r.Context(), companyId)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

// DeleteTokenHandler: DELETE /company/integration-token/{tokenId}
func (s *System) DeleteTokenHandler(w http.ResponseWriter, r *http.Request) {
	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r)
	if !ok {
		return
	}
	if err := s.DeleteToken(r.Context(), companyId, r.PathValue("tokenId")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AgentFlagsHandler: GET /agent/{agentId}/flags (dashboard user), the flag-first view of an agent.
func (s *System) AgentFlagsHandler(w http.ResponseWriter, r *http.Request) {
	agentId := r.PathValue("agentId")
	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceAgent, agentId)
	if !ok {
		return
	}
	af, err := s.GetAgentFlags(r.Context(), companyId, agentId)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, af)
}

// ListAgentsHandler: GET /integration/agents, for a tracker's settings picker.
func (s *System) ListAgentsHandler(w http.ResponseWriter, r *http.Request) {
	companyId := auth.CompanyID(r)
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	agents, err := s.ListAgents(r.Context(), companyId)
	if err != nil {
		s.fail(w, err)
		return
	}
	if agents == nil {
		agents = []AgentInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// ArchiveAgentFlagHandler: DELETE /agent/{agentId}/flags/{name} (dashboard user), removes a flag from every environment.
func (s *System) ArchiveAgentFlagHandler(w http.ResponseWriter, r *http.Request) {
	agentId := r.PathValue("agentId")
	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceAgent, agentId)
	if !ok {
		return
	}
	if err := s.ArchiveAgentFlag(r.Context(), companyId, agentId, r.PathValue("name")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
