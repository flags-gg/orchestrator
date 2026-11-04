package flags

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/flags-gg/orchestrator/internal/stats"
	ConfigBuilder "github.com/keloran/go-config"
)

// OFREP (OpenFeature Remote Evaluation Protocol) implementation
// Spec: https://github.com/open-feature/protocol

// EvaluationContext represents the context for flag evaluation. Per the OFREP spec, attributes sit
// flat next to targetingKey; a nested "context" object is also accepted for older callers.
type EvaluationContext struct {
	TargetingKey string                 `json:"targetingKey,omitempty"`
	Context      map[string]interface{} `json:"context,omitempty"`
}

func (e *EvaluationContext) UnmarshalJSON(b []byte) error {
	var all map[string]interface{}
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	e.Context = map[string]interface{}{}
	if nested, ok := all["context"].(map[string]interface{}); ok {
		for k, v := range nested {
			e.Context[k] = v
		}
	}
	for k, v := range all {
		switch k {
		case "targetingKey":
			e.TargetingKey, _ = v.(string)
		case "context":
		default:
			e.Context[k] = v
		}
	}
	return nil
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
	ReasonSplit          ResolutionReason = "SPLIT"
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
	if environmentId == "" {
		defaultEnvironmentId, err := NewSystem(s.Config).GetDefaultEnvironment(ctx, projectId, agentId)
		if err != nil {
			s.sendErrorResponse(w, flagKey, ErrorGeneral, "Failed to resolve environment", http.StatusInternalServerError)
			return
		}
		environmentId = defaultEnvironmentId
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

	if err := stats.NewSystem(s.Config).RecordEnvironmentRequest(
		ctx,
		projectId,
		agentId,
		environmentId,
		stats.RequestKindSingleFlag,
		stats.RequestSourceOFREPSingle,
	); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to record single flag request: %v", err)
	}

	response := evaluate(flagKey, *flag, req.Context)

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
	if environmentId == "" {
		defaultEnvironmentId, err := NewSystem(s.Config).GetDefaultEnvironment(ctx, projectId, agentId)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(BulkEvaluationResponse{
				Flags: []interface{}{},
			})
			return
		}
		environmentId = defaultEnvironmentId
	}

	flags, err := NewSystem(s.Config).GetAgentFlagsFromDB(ctx, projectId, agentId, environmentId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(BulkEvaluationResponse{
			Flags: []interface{}{},
		})
		return
	}

	if err := stats.NewSystem(s.Config).RecordEnvironmentRequest(
		ctx,
		projectId,
		agentId,
		environmentId,
		stats.RequestKindAllFlags,
		stats.RequestSourceOFREPBulk,
	); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to record bulk flag request: %v", err)
	}

	var responses []interface{}
	if flags != nil {
		for _, flag := range flags.Flags {
			responses = append(responses, evaluate(flag.Details.Name, flag, req.Context))
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

// evaluate resolves a boolean flag for one user: an enabled flag's first matching targeting rule decides it
// (TARGETING_MATCH); otherwise a partial rollout is a SPLIT on the targeting key.
func evaluate(key string, flag Flag, ec EvaluationContext) SuccessEvaluationResponse {
	value, reason := flag.Enabled, ReasonStatic
	metadata := map[string]interface{}{"flagId": flag.Details.ID}
	attrs := EvalContext{}
	for k, v := range ec.Context {
		attrs[k] = v
	}
	if ec.TargetingKey != "" {
		attrs["key"] = ec.TargetingKey
	}
	if serve, matched := MatchRules(flag.Rules, attrs); flag.Enabled && matched {
		value, reason = serve, ReasonTargetingMatch
	} else if flag.Enabled && flag.Rollout != nil && *flag.Rollout < 100 {
		value, reason = InRollout(flag.Details.Name, ec.TargetingKey, *flag.Rollout), ReasonSplit
		metadata["rollout"] = *flag.Rollout
	}
	variant := "disabled"
	if value {
		variant = "enabled"
	}
	return SuccessEvaluationResponse{Key: key, Reason: reason, Value: value, Variant: variant, Metadata: metadata}
}
