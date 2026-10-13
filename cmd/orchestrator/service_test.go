package main

import (
	"testing"

	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/assert"
)

func TestCheckConfig(t *testing.T) {
	cfg := func(dev bool, stripeKey, signingKey string) *ConfigBuilder.Config {
		c := &ConfigBuilder.Config{ProjectProperties: map[string]interface{}{"stripeKey": stripeKey, "jwt_signing_key": signingKey}}
		c.Local.Development = dev
		return c
	}

	assert.Error(t, checkConfig(cfg(true, "sk_live_123", "")), "live stripe key in development")
	assert.Error(t, checkConfig(cfg(false, "sk_live_123", "")), "no signing key in production")
	assert.NoError(t, checkConfig(cfg(false, "sk_live_123", "secret")))
	assert.NoError(t, checkConfig(cfg(true, "sk_test_123", "")), "development may use a random signing key")
}
