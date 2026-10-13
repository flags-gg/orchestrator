package flags

import (
	"encoding/json"
	"net/http"

	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/stats"
	ConfigBuilder "github.com/keloran/go-config"
)

type APIKeyHTTPSystem struct {
	Config *ConfigBuilder.Config
}

func NewAPIKeyHTTPSystem(cfg *ConfigBuilder.Config) *APIKeyHTTPSystem {
	return &APIKeyHTTPSystem{
		Config: cfg,
	}
}

type GenerateAPIKeyRequest struct {
	ProjectID     string `json:"project_id"`
	AgentID       string `json:"agent_id"`
	EnvironmentID string `json:"environment_id,omitempty"`
}

type GenerateAPIKeyResponse struct {
	APIKey    string `json:"api_key"`
	ExpiresAt string `json:"expires_at"`
}

// GenerateAPIKeyHandler handles POST /api-key/generate
func (s *APIKeyHTTPSystem) GenerateAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Parse request
	var req GenerateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	if req.ProjectID == "" || req.AgentID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "project_id and agent_id are required",
		})
		return
	}

	// Only issue keys for the caller's own project/agent/environment
	owned := []string{company.ResourceProject, req.ProjectID, company.ResourceAgent, req.AgentID}
	if req.EnvironmentID != "" {
		owned = append(owned, company.ResourceEnvironment, req.EnvironmentID)
	}
	if _, ok := company.NewSystem(s.Config).CallerOwns(w, r, owned...); !ok {
		return
	}

	// Generate API key
	apiKeySystem := NewAPIKeySystem(s.Config)
	apiKey, err := apiKeySystem.GenerateAPIKey(req.ProjectID, req.AgentID, req.EnvironmentID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to generate API key",
		})
		return
	}

	if err := stats.NewSystem(s.Config).RecordAPIKeyCreation(ctx, userId, req.ProjectID, req.AgentID, req.EnvironmentID); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to record api key creation: %v", err)
	}

	// Return response
	response := GenerateAPIKeyResponse{
		APIKey:    apiKey,
		ExpiresAt: "365 days from now", // TODO: Calculate actual expiry
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
}
