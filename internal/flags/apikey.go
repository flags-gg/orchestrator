package flags

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	ConfigBuilder "github.com/keloran/go-config"
)

// APIKeyClaims represents the JWT claims for an API key
type APIKeyClaims struct {
	ProjectID     string `json:"project_id"`
	AgentID       string `json:"agent_id"`
	EnvironmentID string `json:"environment_id,omitempty"`
	jwt.RegisteredClaims
}

// APIKeySystem handles API key generation and validation
type APIKeySystem struct {
	Config *ConfigBuilder.Config
}

func NewAPIKeySystem(cfg *ConfigBuilder.Config) *APIKeySystem {
	return &APIKeySystem{
		Config: cfg,
	}
}

// GenerateAPIKey creates a JWT-based API key with project, agent, and environment info
func (s *APIKeySystem) GenerateAPIKey(projectID, agentID, environmentID string) (string, error) {
	claims := APIKeyClaims{
		ProjectID:     projectID,
		AgentID:       agentID,
		EnvironmentID: environmentID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(365 * 24 * time.Hour)), // 1 year expiry
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "flags.gg",
			Subject:   fmt.Sprintf("%s:%s", projectID, agentID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Get signing key from config or use a default for now
	signingKey := s.getSigningKey()

	return token.SignedString([]byte(signingKey))
}

// ValidateAPIKey parses and validates a JWT-based API key
func (s *APIKeySystem) ValidateAPIKey(tokenString string) (*APIKeyClaims, error) {
	signingKey := s.getSigningKey()

	token, err := jwt.ParseWithClaims(tokenString, &APIKeyClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(signingKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*APIKeyClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}

// getSigningKey retrieves the JWT signing key from config
// TODO: This should be stored in environment variables or secrets management
func (s *APIKeySystem) getSigningKey() string {
	// Check if there's a JWT_SIGNING_KEY in environment
	// For now, return a placeholder that should be configured
	if s.Config != nil && s.Config.ProjectProperties != nil {
		if key, ok := s.Config.ProjectProperties["jwt_signing_key"].(string); ok && key != "" {
			return key
		}
	}

	// Default key - should be overridden in production
	return "flags-gg-jwt-signing-key-change-in-production"
}
