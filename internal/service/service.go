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
	mux.HandleFunc("GET /flags", flags.NewFlagsSystem(s.Config).GetFlags)
	mux.HandleFunc("POST /flag", flags.CreateFlags)
	mux.HandleFunc("PUT /flag/{flagId}", flags.UpdateFlags)
	mux.HandleFunc("DELETE /flag/{flagId}", flags.DeleteFlags)

	// Agents
	mux.HandleFunc("GET /agents", agent.NewAgentSystem(s.Config).GetAgentsRequest)
	mux.HandleFunc("POST /agent", agent.NewAgentSystem(s.Config).CreateAgent)
	mux.HandleFunc("GET /agent/{agentId}", agent.GetAgent)
	mux.HandleFunc("PUT /agent/{agentId}", agent.UpdateAgent)
	mux.HandleFunc("DELETE /agent/{agentId}", agent.DeleteAgent)

	// Secret Menu
	mux.HandleFunc("GET /agent/{agentId}/secret-menu", agent.GetSecretMenu)
	mux.HandleFunc("POST /agent/{agentId}/secret-menu", agent.CreateSecretMenu)
	mux.HandleFunc("PUT /agent/{agentId}/secret-menu", agent.UpdateSecretMenu)

	// Stats
	mux.HandleFunc("GET /stats/agent/environment/{agentId}", stats.NewStatsSystem(s.Config).GetEnvironmentStats)
	mux.HandleFunc("GET /stats/agent/{agentId}", stats.NewStatsSystem(s.Config).GetAgentStats)

	// User
	mux.HandleFunc("POST /user", user.NewUserSystem(s.Config).CreateUser)
	mux.HandleFunc("GET /user/{userSubject}", user.NewUserSystem(s.Config).GetUser)

	// Company
	mux.HandleFunc("GET /company", company.NewCompanySystem(s.Config).GetCompany)
	mux.HandleFunc("PUT /company", company.NewCompanySystem(s.Config).UpdateCompany)
	mux.HandleFunc("POST /company", company.NewCompanySystem(s.Config).CreateCompany)

	// General
	mux.HandleFunc(fmt.Sprintf("%s /health", http.MethodGet), healthcheck.HTTP)
	mux.HandleFunc(fmt.Sprintf("%s /probe", http.MethodGet), probe.HTTP)

	// middlewares
	mw := middleware.NewMiddleware(context.Background())
	mw.AddMiddleware(middleware.Logger)
	mw.AddMiddleware(middleware.Recoverer)
	mw.AddMiddleware(mw.CORS)
	mw.AddMiddleware(s.Auth)
	mw.AddAllowedHeaders("x-agent-id", "x-company-id", "x-environment-id", "x-user-subject", "x-user-access-token")
	mw.AddAllowedMethods(http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions)
	mw.AddAllowedOrigins("https://www.flags.gg", "https://flags.gg")
	if s.Config.Local.Development {
		mw.AddAllowedOrigins("http://localhost:3000", "http://localhost:5173", "*")
	}

	logs.Local().Infof("Starting HTTP on %d", s.Config.Local.HTTPPort)
	errChan <- http.ListenAndServe(fmt.Sprintf(":%d", s.Config.Local.HTTPPort), mw.Handler(mux))
}
