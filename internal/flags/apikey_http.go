package flags

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/flags-gg/orchestrator/internal/company"
	ConfigBuilder "github.com/keloran/go-config"
)

type APIKeyHTTPSystem struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

func NewAPIKeyHTTPSystem(cfg *ConfigBuilder.Config) *APIKeyHTTPSystem {
	return &APIKeyHTTPSystem{
		Config:  cfg,
		Context: context.Background(),
	}
}

func (s *APIKeyHTTPSystem) SetContext(ctx context.Context) *APIKeyHTTPSystem {
	s.Context = ctx
	return s
}

// getUserId returns the user ID, using dev mode config if in development, otherwise Clerk
func (s *APIKeyHTTPSystem) getUserId(r *http.Request) (string, error) {
	if s.Config.Local.Development && s.Config.Clerk.DevUser != "" {
		return s.Config.Clerk.DevUser, nil
	}

	// Production mode: use Clerk authentication
	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		return "", err
	}
	return usr.ID, nil
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
	s.Context = r.Context()

	// Authenticate user
	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
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
