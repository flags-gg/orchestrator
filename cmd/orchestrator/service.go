package main

import (
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/caarlos0/env/v8"
	"github.com/flags-gg/orchestrator/internal/service"
	ConfigBuilder "github.com/keloran/go-config"
)

var (
	BuildVersion = "0.0.1"
	BuildHash    = "unknown"
	ServiceName  = "service"
)

type ProjectConfig struct{}

func (pc ProjectConfig) Build(cfg *ConfigBuilder.Config) error {
	type PC struct {
		FlagsService string `env:"FLAGS_SERVICE" envDefault:"flags-service.flags-gg:3000"`
	}
	p := PC{}

	if err := env.Parse(&p); err != nil {
		return logs.Errorf("Failed to parse services: %v", err)
	}
	if cfg.ProjectProperties == nil {
		cfg.ProjectProperties = make(map[string]interface{})
	}
	cfg.ProjectProperties["flagsService"] = p.FlagsService

	return nil
}

func main() {
	logs.Local().Infof("Starting %s version %s (build %s)", ServiceName, BuildVersion, BuildHash)

	cfg, err := ConfigBuilder.Build(
		ConfigBuilder.Local,
		ConfigBuilder.Vault,
		ConfigBuilder.Database,
		ConfigBuilder.Keycloak,
		ConfigBuilder.WithProjectConfigurator(ProjectConfig{}))
	if err != nil {
		logs.Local().Fatalf("Failed to build config: %v", err)
	}

	if err := service.New(cfg).Start(); err != nil {
		logs.Local().Fatalf("Failed to start service: %v", err)
	}
}
