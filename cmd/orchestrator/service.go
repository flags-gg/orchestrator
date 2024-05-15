package main

import (
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/caarlos0/env/v8"
	vault_helper "github.com/keloran/vault-helper"

	ConfigBuilder "github.com/keloran/go-config"
	ConfigVault "github.com/keloran/go-config/vault"

	"github.com/flags-gg/orchestrator/internal/service"
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
	logs.Infof("Starting %s version %s (build %s)", ServiceName, BuildVersion, BuildHash)

	genericVaultPath := "kv/data/flags-gg/orchestrator"
	vh := vault_helper.NewVault("", "")
	c := ConfigBuilder.NewConfig(vh)
	c.VaultPaths = ConfigVault.Paths{
		Database: ConfigVault.Path{
			Credentials: "database/creds/flags_database",
			Details:     genericVaultPath,
		},
		Keycloak: ConfigVault.Path{
			Details: genericVaultPath,
		},
		Influx: ConfigVault.Path{
			Details: genericVaultPath,
		},
		BugFixes: ConfigVault.Path{
			Details: genericVaultPath,
		},
	}

	err := c.Build(
		ConfigBuilder.Vault,
		ConfigBuilder.Local,
		ConfigBuilder.Database,
		ConfigBuilder.Keycloak,
		ConfigBuilder.Influx,
		ConfigBuilder.Bugfixes,
		ConfigBuilder.WithProjectConfigurator(ProjectConfig{}))
	if err != nil {
		logs.Fatalf("Failed to build config: %v", err)
	}

	if err := service.New(c).Start(); err != nil {
		logs.Fatalf("Failed to start service: %v", err)
	}
}
