package flags

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	ConfigBuilder "github.com/keloran/go-config"
)

// OFREP (OpenFeature Remote Evaluation Protocol) implementation
// Spec: https://github.com/open-feature/protocol

// EvaluationContext represents the context for flag evaluation
type EvaluationContext struct {
	TargetingKey string                 `json:"targetingKey,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
}

// EvaluationRequest is the request body for flag evaluation
type EvaluationRequest struct {
	Context EvaluationContext `json:"context,omitempty"`
}

// BulkEvaluationRequest is for evaluating multiple flags
type BulkEvaluationRequest struct {
	Context EvaluationContext `json:"context,omitempty"`
}

// ResolutionReason represents why a flag was resolved to its value
type ResolutionReason string

const (
	ReasonStatic         ResolutionReason = "STATIC"
	ReasonTargetingMatch ResolutionReason = "TARGETING_MATCH"
	ReasonDefault        ResolutionReason = "DEFAULT"
	ReasonDisabled       ResolutionReason = "DISABLED"
	ReasonError          ResolutionReason = "ERROR"
	ReasonUnknown        ResolutionReason = "UNKNOWN"
)

// ErrorCode represents OFREP error codes
type ErrorCode string

const (
	ErrorParseError          ErrorCode = "PARSE_ERROR"
	ErrorTargetingKeyMissing ErrorCode = "TARGETING_KEY_MISSING"
	ErrorInvalidContext      ErrorCode = "INVALID_CONTEXT"
	ErrorFlagNotFound        ErrorCode = "FLAG_NOT_FOUND"
	ErrorTypeMismatch        ErrorCode = "TYPE_MISMATCH"
	ErrorGeneral             ErrorCode = "GENERAL"
)

// SuccessEvaluationResponse represents a successful flag evaluation
type SuccessEvaluationResponse struct {
	Key      string                 `json:"key"`
	Reason   ResolutionReason       `json:"reason"`
	Variant  string                 `json:"variant,omitempty"`
	Value    interface{}            `json:"value"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ErrorEvaluationResponse represents a failed flag evaluation
type ErrorEvaluationResponse struct {
	Key          string                 `json:"key"`
	ErrorCode    ErrorCode              `json:"errorCode"`
	ErrorDetails string                 `json:"errorDetails,omitempty"`
	Reason       ResolutionReason       `json:"reason"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// BulkEvaluationResponse wraps multiple flag evaluations
type BulkEvaluationResponse struct {
	Flags []interface{} `json:"flags"`
}

// OFREPSystem handles OFREP endpoints
type OFREPSystem struct {
	Config *ConfigBuilder.Config
}

func NewOFREPSystem(cfg *ConfigBuilder.Config) *OFREPSystem {
	return &OFREPSystem{
		Config: cfg,
	}
}

// extractCredentials gets credentials from X-API-Key (JWT) or individual headers
// Priority: X-API-Key (JWT) > individual headers (x-project-id, x-agent-id, x-environment-id)
func (s *OFREPSystem) extractCredentials(r *http.Request) (projectId, agentId, environmentId string) {
	// Check X-API-Key header first (OFREP standard)
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "" {
		// X-API-Key contains a JWT with project_id, agent_id, environment_id
		apiKeySystem := NewAPIKeySystem(s.Config)
		claims, err := apiKeySystem.ValidateAPIKey(apiKey)
		if err == nil && claims != nil {
			return claims.ProjectID, claims.AgentID, claims.EnvironmentID
		}
		// If JWT validation fails, silently continue to fallback
	}

	// Fall back to individual headers (existing flags.gg method)
	projectId = r.Header.Get("x-project-id")
	agentId = r.Header.Get("x-agent-id")
	environmentId = r.Header.Get("x-environment-id")

	return projectId, agentId, environmentId
}

// EvaluateSingleFlag handles POST /ofrep/v1/evaluate/flags/{key}
func (s *OFREPSystem) EvaluateSingleFlag(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	flagKey := r.PathValue("key")
	if flagKey == "" {
		s.sendErrorResponse(w, "", ErrorFlagNotFound, "Flag key is required", http.StatusBadRequest)
		return
	}

	var req EvaluationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendErrorResponse(w, flagKey, ErrorParseError, "Invalid request body", http.StatusBadRequest)
		return
	}

	projectId, agentId, environmentId := s.extractCredentials(r)

	if projectId == "" || agentId == "" {
		s.sendErrorResponse(w, flagKey, ErrorInvalidContext, "Missing project-id or agent-id headers", http.StatusBadRequest)
		return
	}

	flag, err := s.GetSingleFlagFromDB(ctx, projectId, agentId, environmentId, flagKey)
	if err != nil {
		s.sendErrorResponse(w, flagKey, ErrorGeneral, "Failed to retrieve flag", http.StatusInternalServerError)
		return
	}

	if flag == nil {
		s.sendErrorResponse(w, flagKey, ErrorFlagNotFound, "Flag not found", http.StatusNotFound)
		return
	}

	response := SuccessEvaluationResponse{
		Key:    flagKey,
		Reason: ReasonStatic,
		Value:  flag.Enabled,
		Variant: func() string {
			if flag.Enabled {
				return "enabled"
			}
			return "disabled"
		}(),
		Metadata: map[string]interface{}{
			"flagId": flag.Details.ID,
		},
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
}

// EvaluateBulkFlags handles POST /ofrep/v1/evaluate/flags
func (s *OFREPSystem) EvaluateBulkFlags(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	var req BulkEvaluationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BulkEvaluationResponse{
			Flags: []interface{}{},
		})
		return
	}

	projectId, agentId, environmentId := s.extractCredentials(r)

	if projectId == "" || agentId == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(BulkEvaluationResponse{
			Flags: []interface{}{},
		})
		return
	}

	flags, err := NewSystem(s.Config).GetAgentFlagsFromDB(ctx, projectId, agentId, environmentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(BulkEvaluationResponse{
			Flags: []interface{}{},
		})
		return
	}

	var responses []interface{}
	if flags != nil {
		for _, flag := range flags.Flags {
			response := SuccessEvaluationResponse{
				Key:    flag.Details.Name,
				Reason: ReasonStatic,
				Value:  flag.Enabled,
				Variant: func() string {
					if flag.Enabled {
						return "enabled"
					}
					return "disabled"
				}(),
				Metadata: map[string]interface{}{
					"flagId": flag.Details.ID,
				},
			}
			responses = append(responses, response)
		}
	}

	bulkResponse := BulkEvaluationResponse{
		Flags: responses,
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(bulkResponse); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
}

func (s *OFREPSystem) sendErrorResponse(w http.ResponseWriter, key string, code ErrorCode, details string, statusCode int) {
	w.WriteHeader(statusCode)
	response := ErrorEvaluationResponse{
		Key:          key,
		ErrorCode:    code,
		ErrorDetails: details,
		Reason:       ReasonError,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode error response: %v", err)
	}
}
