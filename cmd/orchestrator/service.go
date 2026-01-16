package main

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/caarlos0/env/v8"
	"github.com/flags-gg/orchestrator/internal"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	ConfigBuilder "github.com/keloran/go-config"
	_ "github.com/lib/pq"
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
		StripeSecret string `env:"STRIPE_SECRET" envDefault:"stripe_secret"`
		RailwayPort  string `env:"PORT" envDefault:"3000"`
		OnRailway    bool   `env:"ON_RAILWAY" envDefault:"false"`
		Flags        FlagsService
	}
	p := PC{}

	if err := env.Parse(&p); err != nil {
		return logs.Errorf("Failed to parse services: %v", err)
	}
	if cfg.ProjectProperties == nil {
		cfg.ProjectProperties = make(map[string]interface{})
	}
	cfg.ProjectProperties["stripeKey"] = p.StripeSecret
	cfg.ProjectProperties["railway_port"] = p.RailwayPort
	cfg.ProjectProperties["on_railway"] = p.OnRailway

	cfg.ProjectProperties["flags_agent"] = p.Flags.AgentID
	cfg.ProjectProperties["flags_environment"] = p.Flags.EnvironmentID
	cfg.ProjectProperties["flags_project"] = p.Flags.ProjectID

	return nil
}

func main() {
	logs.Logf("Starting %s version %s (build %s)", ServiceName, BuildVersion, BuildHash)
	c := ConfigBuilder.NewConfigNoVault()

	err := c.Build(
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

	// do migration before starting app
	if err := migrateDB(c); err != nil {
		logs.Fatalf("Failed to migrate db: %v", err)
	}

	if err := internal.New(c).Start(); err != nil {
		logs.Fatalf("Failed to start service: %v", err)
	}
}

func migrateDB(config *ConfigBuilder.Config) error {
	db, err := sql.Open("postgres",
		fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
			config.Database.User,
			config.Database.Password,
			config.Database.Host,
			config.Database.Port,
			config.Database.DBName))
	if err != nil {
		return logs.Errorf("Failed to connect to database: %v", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return logs.Errorf("Failed to create postgres driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://k8s/schema", "postgres", driver)
	if err != nil {
		return logs.Errorf("Failed to create migration instance: %v", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return logs.Errorf("Failed to run migration: %v", err)
	}

	return nil
}
