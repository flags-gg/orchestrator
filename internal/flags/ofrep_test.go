package flags

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/docker/go-connections/nat"
	ConfigBuilder "github.com/keloran/go-config"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type testContainer struct {
	container testcontainers.Container
	uri       string
}

func setupTestDatabase(c context.Context) (*testContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:14-alpine",
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections"),
			wait.ForSQL("5432/tcp", "postgres", func(host string, port nat.Port) string {
				return fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())
			}),
		).WithDeadline(time.Minute * 2),
		Env: map[string]string{
			"POSTGRES_DB":       "testdb",
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
		},
	}

	container, err := testcontainers.GenericContainer(c, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, err
	}

	mappedPort, err := container.MappedPort(c, "5432")
	if err != nil {
		return nil, err
	}

	hostIP, err := container.Host(c)
	if err != nil {
		return nil, err
	}

	uri := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", hostIP, mappedPort.Port())
	_ = os.Setenv("RDS_HOSTNAME", hostIP)
	_ = os.Setenv("RDS_PORT", mappedPort.Port())
	_ = os.Setenv("RDS_USERNAME", "test")
	_ = os.Setenv("RDS_PASSWORD", "test")
	_ = os.Setenv("RDS_DB", "testdb")

	db, err := sql.Open("postgres", uri)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			_ = logs.Errorf("Failed to close database connection: %v", err)
		}
	}()

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE public.project (
			id serial PRIMARY KEY,
			project_id varchar(255) NOT NULL,
			name varchar(255) NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			created_at timestamp NOT NULL DEFAULT now()
		);

		CREATE TABLE public.agent (
			id serial PRIMARY KEY,
			agent_id varchar(255) NOT NULL,
			project_id integer REFERENCES public.project(id),
			name varchar(255) NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			interval integer NOT NULL DEFAULT 60,
			created_at timestamp NOT NULL DEFAULT now()
		);

		CREATE TABLE public.environment (
			id serial PRIMARY KEY,
			env_id varchar(255) NOT NULL,
			agent_id integer REFERENCES public.agent(id),
			name varchar(255) NOT NULL,
			"default" boolean NOT NULL DEFAULT false,
			created_at timestamp NOT NULL DEFAULT now()
		);

		CREATE TABLE public.flag (
			id serial PRIMARY KEY,
			name varchar(255) NOT NULL,
			enabled boolean NOT NULL DEFAULT false,
			agent_id integer REFERENCES public.agent(id),
			environment_id integer REFERENCES public.environment(id),
			created_at timestamp NOT NULL DEFAULT now(),
		    updated_at timestamp NOT NULL DEFAULT now()
		);

		CREATE TABLE public.secret_menu (
			id serial PRIMARY KEY,
			agent_id integer REFERENCES public.agent(id),
			environment_id integer REFERENCES public.environment(id),
			code varchar(255),
			enabled boolean NOT NULL DEFAULT false,
			created_at timestamp NOT NULL DEFAULT now()
		);

		CREATE TABLE public.secret_menu_style (
			id serial PRIMARY KEY,
			secret_menu_id integer REFERENCES public.secret_menu(id),
			close_button text,
			container text,
			reset_button text,
			flag text,
			button_enabled text,
			button_disabled text,
			header text,
			created_at timestamp NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		return nil, err
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO public.project (project_id, name, enabled)
		VALUES ('test-project-1', 'Test Project', true);

		INSERT INTO public.agent (agent_id, project_id, name, enabled, interval)
		VALUES ('test-agent-1', 1, 'Test Agent', true, 60);

		INSERT INTO public.environment (env_id, agent_id, name, "default")
		VALUES ('test-env-1', 1, 'Test Environment', true);

		INSERT INTO public.flag (name, enabled, agent_id, environment_id)
		VALUES
			('feature-flag-1', true, 1, 1),
			('feature-flag-2', false, 1, 1),
			('feature-flag-3', true, 1, 1);
	`)
	if err != nil {
		return nil, err
	}

	return &testContainer{
		container: container,
		uri:       uri,
	}, nil
}

func setupTestSystem(t *testing.T) (*System, *OFREPSystem) {
	c := ConfigBuilder.NewConfigNoVault()
	if err := c.Build(ConfigBuilder.Database, ConfigBuilder.Bugfixes); err != nil {
		t.Fatalf("Failed to build config: %v", err)
	}

	return NewSystem(c), NewOFREPSystem(c)
}

func TestOFREPSingleFlagEvaluation(t *testing.T) {
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

	tests := []struct {
		name           string
		flagKey        string
		projectId      string
		agentId        string
		environmentId  string
		requestBody    EvaluationRequest
		expectedStatus int
		expectedValue  interface{}
		expectedError  ErrorCode
	}{
		{
			name:          "Success - enabled flag",
			flagKey:       "feature-flag-1",
			projectId:     "test-project-1",
			agentId:       "test-agent-1",
			environmentId: "test-env-1",
			requestBody: EvaluationRequest{
				Context: EvaluationContext{
					TargetingKey: "user-123",
				},
			},
			expectedStatus: http.StatusOK,
			expectedValue:  true,
		},
		{
			name:          "Success - disabled flag",
			flagKey:       "feature-flag-2",
			projectId:     "test-project-1",
			agentId:       "test-agent-1",
			environmentId: "test-env-1",
			requestBody: EvaluationRequest{
				Context: EvaluationContext{
					TargetingKey: "user-123",
				},
			},
			expectedStatus: http.StatusOK,
			expectedValue:  false,
		},
		{
			name:          "Flag not found",
			flagKey:       "non-existent-flag",
			projectId:     "test-project-1",
			agentId:       "test-agent-1",
			environmentId: "test-env-1",
			requestBody: EvaluationRequest{
				Context: EvaluationContext{
					TargetingKey: "user-123",
				},
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  ErrorFlagNotFound,
		},
		{
			name:           "Missing project-id",
			flagKey:        "feature-flag-1",
			projectId:      "",
			agentId:        "test-agent-1",
			environmentId:  "test-env-1",
			requestBody:    EvaluationRequest{},
			expectedStatus: http.StatusBadRequest,
			expectedError:  ErrorInvalidContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/ofrep/v1/evaluate/flags/"+tt.flagKey, bytes.NewReader(body))
			req.Header.Set("x-project-id", tt.projectId)
			req.Header.Set("x-agent-id", tt.agentId)
			req.Header.Set("x-environment-id", tt.environmentId)
			req.SetPathValue("key", tt.flagKey)

			w := httptest.NewRecorder()
			ofrepSystem.EvaluateSingleFlag(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				var response SuccessEvaluationResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tt.flagKey, response.Key)
				assert.Equal(t, tt.expectedValue, response.Value)
				assert.Equal(t, ReasonStatic, response.Reason)
				assert.NotEmpty(t, response.Variant)
				assert.NotNil(t, response.Metadata)
			} else {
				var response ErrorEvaluationResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedError, response.ErrorCode)
				assert.Equal(t, ReasonError, response.Reason)
			}
		})
	}
}

func TestOFREPBulkFlagEvaluation(t *testing.T) {
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

	tests := []struct {
		name           string
		projectId      string
		agentId        string
		environmentId  string
		requestBody    BulkEvaluationRequest
		expectedStatus int
		expectedCount  int
	}{
		{
			name:          "Success - all flags",
			projectId:     "test-project-1",
			agentId:       "test-agent-1",
			environmentId: "test-env-1",
			requestBody: BulkEvaluationRequest{
				Context: EvaluationContext{
					TargetingKey: "user-123",
				},
			},
			expectedStatus: http.StatusOK,
			expectedCount:  3,
		},
		{
			name:           "Missing agent-id",
			projectId:      "test-project-1",
			agentId:        "",
			environmentId:  "test-env-1",
			requestBody:    BulkEvaluationRequest{},
			expectedStatus: http.StatusBadRequest,
			expectedCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/ofrep/v1/evaluate/flags", bytes.NewReader(body))
			req.Header.Set("x-project-id", tt.projectId)
			req.Header.Set("x-agent-id", tt.agentId)
			req.Header.Set("x-environment-id", tt.environmentId)

			w := httptest.NewRecorder()
			ofrepSystem.EvaluateBulkFlags(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response BulkEvaluationResponse
			err := json.NewDecoder(w.Body).Decode(&response)
			assert.NoError(t, err)
			assert.Len(t, response.Flags, tt.expectedCount)

			if tt.expectedStatus == http.StatusOK && tt.expectedCount > 0 {
				for _, flag := range response.Flags {
					successFlag, ok := flag.(map[string]interface{})
					assert.True(t, ok)
					assert.NotEmpty(t, successFlag["key"])
					assert.NotNil(t, successFlag["value"])
					assert.Equal(t, "STATIC", successFlag["reason"])
				}
			}
		})
	}
}

func TestOFREPAndStandardEndpointsReturnSameFlags(t *testing.T) {
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

	standardSystem, ofrepSystem := setupTestSystem(t)

	projectId := "test-project-1"
	agentId := "test-agent-1"
	environmentId := "test-env-1"

	// Get flags from standard endpoint
	stdReq := httptest.NewRequest(http.MethodGet, "/flags", nil)
	stdReq.Header.Set("x-project-id", projectId)
	stdReq.Header.Set("x-agent-id", agentId)
	stdReq.Header.Set("x-environment-id", environmentId)

	stdW := httptest.NewRecorder()
	standardSystem.GetAgentFlags(stdW, stdReq)

	assert.Equal(t, http.StatusOK, stdW.Code)

	var stdResponse AgentResponse
	err = json.NewDecoder(stdW.Body).Decode(&stdResponse)
	assert.NoError(t, err)

	// Get flags from OFREP bulk endpoint
	ofrepBody, _ := json.Marshal(BulkEvaluationRequest{
		Context: EvaluationContext{
			TargetingKey: "user-123",
		},
	})
	ofrepReq := httptest.NewRequest(http.MethodPost, "/ofrep/v1/evaluate/flags", bytes.NewReader(ofrepBody))
	ofrepReq.Header.Set("x-project-id", projectId)
	ofrepReq.Header.Set("x-agent-id", agentId)
	ofrepReq.Header.Set("x-environment-id", environmentId)

	ofrepW := httptest.NewRecorder()
	ofrepSystem.EvaluateBulkFlags(ofrepW, ofrepReq)

	assert.Equal(t, http.StatusOK, ofrepW.Code)

	var ofrepResponse BulkEvaluationResponse
	err = json.NewDecoder(ofrepW.Body).Decode(&ofrepResponse)
	assert.NoError(t, err)

	// Compare: same number of flags
	assert.Equal(t, len(stdResponse.Flags), len(ofrepResponse.Flags), "Both endpoints should return the same number of flags")

	// Build a map of standard flags for comparison
	stdFlagsMap := make(map[string]bool)
	for _, flag := range stdResponse.Flags {
		stdFlagsMap[flag.Details.Name] = flag.Enabled
	}

	// Verify each OFREP flag matches standard flag
	for _, flag := range ofrepResponse.Flags {
		ofrepFlag, ok := flag.(map[string]interface{})
		assert.True(t, ok)

		flagName := ofrepFlag["key"].(string)
		flagValue := ofrepFlag["value"].(bool)

		stdValue, exists := stdFlagsMap[flagName]
		assert.True(t, exists, "Flag %s should exist in both endpoints", flagName)
		assert.Equal(t, stdValue, flagValue, "Flag %s should have the same value in both endpoints", flagName)
	}
}

func TestOFREPAPIKeyAuthentication(t *testing.T) {
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

	// Generate JWT API keys
	apiKeySystem := NewAPIKeySystem(ofrepSystem.Config)
	validAPIKeyWithEnv, err := apiKeySystem.GenerateAPIKey("test-project-1", "test-agent-1", "test-env-1")
	assert.NoError(t, err)
	validAPIKeyWithoutEnv, err := apiKeySystem.GenerateAPIKey("test-project-1", "test-agent-1", "")
	assert.NoError(t, err)

	tests := []struct {
		name           string
		apiKey         string
		flagKey        string
		expectedStatus int
		shouldSucceed  bool
	}{
		{
			name:           "Success with API key",
			apiKey:         validAPIKeyWithEnv,
			flagKey:        "feature-flag-1",
			expectedStatus: http.StatusOK,
			shouldSucceed:  true,
		},
		{
			name:           "Success with API key without environment",
			apiKey:         validAPIKeyWithoutEnv,
			flagKey:        "feature-flag-1",
			expectedStatus: http.StatusOK,
			shouldSucceed:  true,
		},
		{
			name:           "Invalid API key format",
			apiKey:         "invalid-key",
			flagKey:        "feature-flag-1",
			expectedStatus: http.StatusBadRequest,
			shouldSucceed:  false,
		},
		{
			name:           "Empty API key",
			apiKey:         "",
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
			if tt.apiKey != "" {
				req.Header.Set("X-API-Key", tt.apiKey)
			}
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
			} else {
				var response ErrorEvaluationResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.NotEmpty(t, response.ErrorCode)
			}
		})
	}
}

func TestOFREPCaseInsensitiveFlagLookup(t *testing.T) {
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

	tests := []struct {
		name        string
		flagKey     string
		shouldMatch bool
	}{
		{
			name:        "Exact match",
			flagKey:     "feature-flag-1",
			shouldMatch: true,
		},
		{
			name:        "Uppercase",
			flagKey:     "FEATURE-FLAG-1",
			shouldMatch: true,
		},
		{
			name:        "Mixed case",
			flagKey:     "Feature-Flag-1",
			shouldMatch: true,
		},
		{
			name:        "Different flag",
			flagKey:     "different-flag",
			shouldMatch: false,
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
			req.Header.Set("x-project-id", "test-project-1")
			req.Header.Set("x-agent-id", "test-agent-1")
			req.Header.Set("x-environment-id", "test-env-1")
			req.SetPathValue("key", tt.flagKey)

			w := httptest.NewRecorder()
			ofrepSystem.EvaluateSingleFlag(w, req)

			if tt.shouldMatch {
				assert.Equal(t, http.StatusOK, w.Code)
				var response SuccessEvaluationResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, tt.flagKey, response.Key)
			} else {
				assert.Equal(t, http.StatusNotFound, w.Code)
			}
		})
	}
}
