package main

import (
	"github.com/bugfixes/go-bugfixes/logs"
	env "github.com/caarlos0/env/v8"
	"github.com/flags-gg/orchestrator/internal"
	vaulthelper "github.com/keloran/vault-helper"
	"os"

	ConfigBuilder "github.com/keloran/go-config"
	ConfigVault "github.com/keloran/go-config/vault"
)

var (
	BuildVersion = "0.0.1"
	BuildHash    = "unknown"
	ServiceName  = "service"
)

type ProjectConfig struct{}

func (pc ProjectConfig) Build(cfg *ConfigBuilder.Config) error {
	type FlagsService struct {
		ProjectID     string `env:"FLAGS_PROJECT_ID" envDefault:"flags-gg"`
		AgentID       string `env:"FLAGS_AGENT_ID" envDefault:"orchestrator"`
		EnvironmentID string `env:"FLAGS_ENVIRONMENT_ID" envDefault:"orchestrator"`
	}

	type PC struct {
		//ResendKey   string `env:"RESEND_KEY" envDefault:"flags-gg-resend-key"`
		StripeLocal string `env:"STRIPE_LOCAL" envDefault:"stripe_local"`
		//ClerkKey    string `env:"CLERK_SECRET_KEY" envDefault:"clerk_key"`
		Flags FlagsService
	}
	p := PC{}

	if err := env.Parse(&p); err != nil {
		return logs.Errorf("Failed to parse services: %v", err)
	}
	if cfg.ProjectProperties == nil {
		cfg.ProjectProperties = make(map[string]interface{})
	}
	//cfg.ProjectProperties["stripeLocal"] = p.StripeLocal
	//cfg.ProjectProperties["clerkKey"] = p.ClerkKey

	cfg.ProjectProperties["flags_agent"] = p.Flags.AgentID
	cfg.ProjectProperties["flags_environment"] = p.Flags.EnvironmentID
	cfg.ProjectProperties["flags_project"] = p.Flags.ProjectID

	// get the resend key out of the vault
	vh := *cfg.VaultHelper
	if vh.Secrets() == nil {
		return logs.Error("no secrets found")
	}

	//if cfg.ProjectProperties["resendKey"] == "" {
	//	secret, err := vh.GetSecret("resend_key")
	//	if err != nil {
	//		return logs.Errorf("failed to get resend key: %v", err)
	//	}
	//	cfg.ProjectProperties["resendKey"] = secret
	//}
	//
	//// get the clerkKey
	//if cfg.ProjectProperties["clerkKey"] == "" {
	//	secret, err := vh.GetSecret("clerk_key")
	//	if err != nil {
	//		return logs.Errorf("failed to get clerk key: %v", err)
	//	}
	//	cfg.ProjectProperties["clerkKey"] = secret
	//}

	return nil
}

func main() {
	logs.Logf("Starting %s version %s (build %s)", ServiceName, BuildVersion, BuildHash)

	kvPath := "kv/data/flags-gg/orchestrator"
	if localPath, ok := os.LookupEnv("LOCAL_VAULT_PATH"); ok {
		kvPath = localPath
	}
	vh := vaulthelper.NewVault("", "")
	c := ConfigBuilder.NewConfig(vh)
	c.VaultPaths = ConfigVault.Paths{
		Database: ConfigVault.Path{
			Credentials: "database/creds/flags_database",
			Details:     kvPath,
		},
		Keycloak: ConfigVault.Path{
			Details: kvPath,
		},
		Influx: ConfigVault.Path{
			Details: kvPath,
		},
		BugFixes: ConfigVault.Path{
			Details: kvPath,
		},
		Clerk: ConfigVault.Path{
			Details: kvPath,
		},
		Resend: ConfigVault.Path{
			Details: kvPath,
		},
	}

	err := c.Build(
		ConfigBuilder.Vault,
		ConfigBuilder.Local,
		ConfigBuilder.Database,
		ConfigBuilder.Keycloak,
		ConfigBuilder.Influx,
		ConfigBuilder.Bugfixes,
		ConfigBuilder.Clerk,
		ConfigBuilder.Resend,
		ConfigBuilder.WithProjectConfigurator(ProjectConfig{}))
	if err != nil {
		logs.Fatalf("Failed to build config: %v", err)
	}

	if err := internal.New(c).Start(); err != nil {
		logs.Fatalf("Failed to start service: %v", err)
	}
}
