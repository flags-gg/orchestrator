package service

import (
	"fmt"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/config"
	"github.com/flags-gg/orchestrator/internal/flags"
	"github.com/flags-gg/orchestrator/internal/middleware"
	"github.com/flags-gg/orchestrator/internal/stats"
	"github.com/flags-gg/orchestrator/internal/user"
	"github.com/keloran/go-healthcheck"
	"github.com/keloran/go-probe"
	"net/http"
)

type Service struct {
	Config *config.Config
}

func New(cfg *config.Config) *Service {
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
	mux.HandleFunc("GET /agents", agent.GetAgents)
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
	middleWare := middleware.NewMiddlewareSystem(s.Config).Middleware(mux)

	logs.Local().Infof("Starting HTTP on %d", s.Config.Local.HTTPPort)
	errChan <- http.ListenAndServe(fmt.Sprintf(":%d", s.Config.Local.HTTPPort), middleWare)
}
