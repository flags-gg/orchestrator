package config

import (
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/caarlos0/env/v8"
	ConfigBuilder "github.com/keloran/go-config"
)

type Config struct {
	Services
	ConfigBuilder.Config
}

type Services struct {
	FlagsService string `env:"FLAGS_SERVICE" envDefault:"flags-service.flags-gg:3000"`
}

func BuildServices(cfg *Config) error {
	services := &Services{}
	if err := env.Parse(services); err != nil {
		return logs.Errorf("Failed to parse services: %v", err)
	}
	cfg.Services = *services
	return nil
}

func Build() (*Config, error) {
	cfg := &Config{}
	gcc, err := ConfigBuilder.Build(ConfigBuilder.Local, ConfigBuilder.Vault, ConfigBuilder.Database, ConfigBuilder.Keycloak)
	if err != nil {
		return nil, logs.Errorf("Failed to build config: %v", err)
	}
	cfg.Config = *gcc
	if err := BuildServices(cfg); err != nil {
		return nil, logs.Errorf("Failed to build services: %v", err)
	}

	return cfg, nil
}
