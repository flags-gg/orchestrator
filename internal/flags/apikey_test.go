package flags

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/assert"
)

func TestJWTAPIKeyGeneration(t *testing.T) {
	c := ConfigBuilder.NewConfigNoVault()
	if err := c.Build(ConfigBuilder.Bugfixes); err != nil {
		t.Fatalf("Failed to build config: %v", err)
	}

	apiKeySystem := NewAPIKeySystem(c)

	tests := []struct {
		name          string
		projectID     string
		agentID       string
		environmentID string
	}{
		{
			name:          "With environment",
			projectID:     "test-project-1",
			agentID:       "test-agent-1",
			environmentID: "test-env-1",
		},
		{
			name:          "Without environment",
			projectID:     "test-project-1",
			agentID:       "test-agent-1",
			environmentID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiKey, err := apiKeySystem.GenerateAPIKey(tt.projectID, tt.agentID, tt.environmentID)
			assert.NoError(t, err)
			assert.NotEmpty(t, apiKey)

			// Validate the generated key
			claims, err := apiKeySystem.ValidateAPIKey(apiKey)
			assert.NoError(t, err)
			assert.NotNil(t, claims)
			assert.Equal(t, tt.projectID, claims.ProjectID)
			assert.Equal(t, tt.agentID, claims.AgentID)
			assert.Equal(t, tt.environmentID, claims.EnvironmentID)
		})
	}
}

func TestJWTAPIKeyValidation(t *testing.T) {
	c := ConfigBuilder.NewConfigNoVault()
	if err := c.Build(ConfigBuilder.Bugfixes); err != nil {
		t.Fatalf("Failed to build config: %v", err)
	}

	apiKeySystem := NewAPIKeySystem(c)

	validKey, _ := apiKeySystem.GenerateAPIKey("test-project", "test-agent", "test-env")

	tests := []struct {
		name      string
		apiKey    string
		shouldErr bool
	}{
		{
			name:      "Valid JWT",
			apiKey:    validKey,
			shouldErr: false,
		},
		{
			name:      "Invalid JWT",
			apiKey:    "invalid.jwt.token",
			shouldErr: true,
		},
		{
			name:      "Empty string",
			apiKey:    "",
			shouldErr: true,
		},
		{
			name:      "Random string",
			apiKey:    "not-a-jwt",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := apiKeySystem.ValidateAPIKey(tt.apiKey)
			if tt.shouldErr {
				assert.Error(t, err)
				assert.Nil(t, claims)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
			}
		})
	}
}

func TestOFREPWithJWTAPIKey(t *testing.T) {
	ctx := context.Background()

	testDB, err := setupTestDatabase(ctx)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}
	defer func() {
		if err := testDB.container.Terminate(ctx); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	_, ofrepSystem := setupTestSystem(t)

	// Generate a JWT API key
	apiKeySystem := NewAPIKeySystem(ofrepSystem.Config)
	jwtAPIKey, err := apiKeySystem.GenerateAPIKey("test-project-1", "test-agent-1", "test-env-1")
	assert.NoError(t, err)

	tests := []struct {
		name           string
		apiKey         string
		flagKey        string
		expectedStatus int
		shouldSucceed  bool
	}{
		{
			name:           "Success with JWT API key",
			apiKey:         jwtAPIKey,
			flagKey:        "feature-flag-1",
			expectedStatus: http.StatusOK,
			shouldSucceed:  true,
		},
		{
			name:           "Invalid JWT",
			apiKey:         "invalid.jwt.token",
			flagKey:        "feature-flag-1",
			expectedStatus: http.StatusBadRequest,
			shouldSucceed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(EvaluationRequest{
				Context: EvaluationContext{
					TargetingKey: "user-123",
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/ofrep/v1/evaluate/flags/"+tt.flagKey, bytes.NewReader(body))
			req.Header.Set("X-API-Key", tt.apiKey)
			req.SetPathValue("key", tt.flagKey)

			w := httptest.NewRecorder()
			ofrepSystem.EvaluateSingleFlag(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.shouldSucceed {
				var response SuccessEvaluationResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tt.flagKey, response.Key)
				assert.NotNil(t, response.Value)
			}
		})
	}
}

func TestOFREPFallbackToIndividualHeaders(t *testing.T) {
	ctx := context.Background()

	testDB, err := setupTestDatabase(ctx)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}
	defer func() {
		if err := testDB.container.Terminate(ctx); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	_, ofrepSystem := setupTestSystem(t)

	// Test that individual headers still work
	body, _ := json.Marshal(EvaluationRequest{
		Context: EvaluationContext{
			TargetingKey: "user-123",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/ofrep/v1/evaluate/flags/feature-flag-1", bytes.NewReader(body))
	req.Header.Set("x-project-id", "test-project-1")
	req.Header.Set("x-agent-id", "test-agent-1")
	req.Header.Set("x-environment-id", "test-env-1")
	req.SetPathValue("key", "feature-flag-1")

	w := httptest.NewRecorder()
	ofrepSystem.EvaluateSingleFlag(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response SuccessEvaluationResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, "feature-flag-1", response.Key)
	assert.NotNil(t, response.Value)
}
