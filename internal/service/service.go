package service

import (
	"context"
	"fmt"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/bugfixes/go-bugfixes/middleware"
	"github.com/keloran/go-healthcheck"
	"github.com/keloran/go-probe"

	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/flags"
	"github.com/flags-gg/orchestrator/internal/project"
	"github.com/flags-gg/orchestrator/internal/stats"
	"github.com/flags-gg/orchestrator/internal/user"
)

type Service struct {
	Config *ConfigBuilder.Config
}

func New(cfg *ConfigBuilder.Config) *Service {
	return &Service{
		Config: cfg,
	}
}

func (s *Service) Start() error {
	errChan := make(chan error)

	go s.startHTTP(errChan)

	return <-errChan
}

func (s *Service) startHTTP(errChan chan error) {
	mux := http.NewServeMux()
	// Flags
	mux.HandleFunc("GET /flags", flags.NewSystem(s.Config).GetFlags)
	mux.HandleFunc("POST /flag", flags.NewSystem(s.Config).CreateFlags)
	mux.HandleFunc("PUT /flag/{flagId}", flags.NewSystem(s.Config).UpdateFlags)
	mux.HandleFunc("DELETE /flag/{flagId}", flags.NewSystem(s.Config).DeleteFlags)

	// Agents
	mux.HandleFunc("GET /agents", agent.NewSystem(s.Config).GetAgentsRequest)
	mux.HandleFunc("POST /agent", agent.NewSystem(s.Config).CreateAgent)
	mux.HandleFunc("GET /agent/{agentId}", agent.NewSystem(s.Config).GetAgent)
	mux.HandleFunc("PUT /agent/{agentId}", agent.NewSystem(s.Config).UpdateAgent)
	mux.HandleFunc("DELETE /agent/{agentId}", agent.NewSystem(s.Config).DeleteAgent)
	mux.HandleFunc("GET /agent/{agentId}/environments", agent.NewSystem(s.Config).GetAgentEnvironments)
	mux.HandleFunc("POST /agent/{agentId}/environment", agent.NewSystem(s.Config).CreateAgentEnvironment)
	mux.HandleFunc("PUT /agent/{agentId}/environment/{environmentId}", agent.NewSystem(s.Config).UpdateAgentEnvironment)
	mux.HandleFunc("DELETE /agent/{agentId}/environment/{environmentId}", agent.NewSystem(s.Config).DeleteAgentEnvironment)

	// Projects
	mux.HandleFunc("GET /projects", project.NewSystem(s.Config).GetProjects)
	mux.HandleFunc("POST /project", project.NewSystem(s.Config).CreateProject)
	mux.HandleFunc("GET /project/{projectId}", project.NewSystem(s.Config).GetProject)
	mux.HandleFunc("PUT /project/{projectId}", project.NewSystem(s.Config).UpdateProject)
	mux.HandleFunc("DELETE /project/{projectId}", project.NewSystem(s.Config).DeleteProject)

	// Secret Menu
	mux.HandleFunc("GET /agent/{agentId}/secret-menu", agent.NewSystem(s.Config).GetSecretMenu)
	mux.HandleFunc("POST /agent/{agentId}/secret-menu", agent.NewSystem(s.Config).CreateSecretMenu)
	mux.HandleFunc("PUT /agent/{agentId}/secret-menu", agent.NewSystem(s.Config).UpdateSecretMenu)

	// Stats
	mux.HandleFunc("GET /stats/agent/environment/{agentId}", stats.NewSystem(s.Config).GetEnvironmentStats)
	mux.HandleFunc("GET /stats/agent/{agentId}", stats.NewSystem(s.Config).GetAgentStats)

	// User
	mux.HandleFunc("POST /user", user.NewSystem(s.Config).CreateUser)
	mux.HandleFunc("PUT /user/{userSubject}", user.NewSystem(s.Config).UpdateUser)
	mux.HandleFunc("GET /user/{userSubject}", user.NewSystem(s.Config).GetUser)

	// Company
	mux.HandleFunc("GET /company", company.NewSystem(s.Config).GetCompany)
	mux.HandleFunc("PUT /company", company.NewSystem(s.Config).UpdateCompany)
	mux.HandleFunc("POST /company", company.NewSystem(s.Config).CreateCompany)
	mux.HandleFunc("GET /company/limits", company.NewSystem(s.Config).GetCompanyLimits)

	// General
	mux.HandleFunc(fmt.Sprintf("%s /health", http.MethodGet), healthcheck.HTTP)
	mux.HandleFunc(fmt.Sprintf("%s /probe", http.MethodGet), probe.HTTP)

	// middlewares
	mw := middleware.NewMiddleware(context.Background())
	mw.AddMiddleware(middleware.SetupLogger(middleware.Error).Logger)
	mw.AddMiddleware(middleware.RequestID)
	mw.AddMiddleware(middleware.Recoverer)
	mw.AddMiddleware(s.Auth)
	mw.AddMiddleware(mw.CORS)
	mw.AddAllowedHeaders(
		"x-agent-id",
		"x-company-id",
		"x-project-id",
		"x-environment-id",
		"x-user-subject",
		"x-user-access-token",
	)
	mw.AddAllowedMethods(http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions)
	mw.AddAllowedOrigins("https://www.flags.gg", "https://flags.gg", "*")
	if s.Config.Local.Development {
		mw.AddAllowedOrigins("http://localhost:3000", "http://localhost:5173", "*")
	}

	logs.Logf("Starting HTTP on %d", s.Config.Local.HTTPPort)
	errChan <- http.ListenAndServe(fmt.Sprintf(":%d", s.Config.Local.HTTPPort), mw.Handler(mux))
}
