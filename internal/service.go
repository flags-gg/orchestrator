package internal

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/environment"
	"github.com/flags-gg/orchestrator/internal/general"
	"github.com/flags-gg/orchestrator/internal/pricing"
	"github.com/flags-gg/orchestrator/internal/project"
	"github.com/flags-gg/orchestrator/internal/secretmenu"
	ConfigBuilder "github.com/keloran/go-config"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/bugfixes/go-bugfixes/middleware"
	"github.com/keloran/go-healthcheck"
	"github.com/keloran/go-probe"

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

	// Projects
	mux.HandleFunc("GET /projects", project.NewSystem(s.Config).GetProjects)
	mux.HandleFunc("POST /project", project.NewSystem(s.Config).CreateProject)
	mux.HandleFunc("GET /project/{projectId}", project.NewSystem(s.Config).GetProject)
	mux.HandleFunc("PUT /project/{projectId}", project.NewSystem(s.Config).UpdateProject)
	mux.HandleFunc("PUT /project/{projectId}/image", project.NewSystem(s.Config).UpdateProjectImage)
	mux.HandleFunc("GET /project/{projectId}/limits", project.NewSystem(s.Config).GetLimits)
	mux.HandleFunc("DELETE /project/{projectId}", project.NewSystem(s.Config).DeleteProject)

	// Agents
	mux.HandleFunc("GET /project/{projectId}/agents", agent.NewSystem(s.Config).GetProjectAgents)
	mux.HandleFunc("POST /project/{projectId}/agent", agent.NewSystem(s.Config).CreateAgent)
	mux.HandleFunc("GET /agents", agent.NewSystem(s.Config).GetAgentsRequest)
	mux.HandleFunc("GET /agent/{agentId}", agent.NewSystem(s.Config).GetAgent)
	mux.HandleFunc("PUT /agent/{agentId}", agent.NewSystem(s.Config).UpdateAgent)
	mux.HandleFunc("DELETE /agent/{agentId}", agent.NewSystem(s.Config).DeleteAgent)

	// Environments
	mux.HandleFunc("GET /agent/{agentId}/environments", environment.NewSystem(s.Config).GetAgentEnvironments)
	mux.HandleFunc("POST /agent/{agentId}/environment", environment.NewSystem(s.Config).CreateAgentEnvironment)
	mux.HandleFunc("POST /agent/{agentId}/{environmentId}", environment.NewSystem(s.Config).CloneAgentEnvironment)
	mux.HandleFunc("GET /environment/{environmentId}", environment.NewSystem(s.Config).GetEnvironment)
	mux.HandleFunc("PUT /environment/{environmentId}", environment.NewSystem(s.Config).UpdateEnvironment)
	mux.HandleFunc("DELETE /environment/{environmentId}", environment.NewSystem(s.Config).DeleteEnvironment)
	mux.HandleFunc("GET /environments", environment.NewSystem(s.Config).GetEnvironments)

	// Flags
	mux.HandleFunc("GET /v1/flags", flags.NewSystem(s.Config).GetAgentFlags)                           // temp whilst apis update
	mux.HandleFunc("GET /flags", flags.NewSystem(s.Config).GetAgentFlags)                              // used by the library
	mux.HandleFunc("GET /environment/{environmentId}/flags", flags.NewSystem(s.Config).GetClientFlags) // used by the frontend
	mux.HandleFunc("POST /flag", flags.NewSystem(s.Config).CreateFlags)
	mux.HandleFunc("PATCH /flag/{flagId}", flags.NewSystem(s.Config).UpdateFlags)
	mux.HandleFunc("PUT /flag/{flagId}", flags.NewSystem(s.Config).EditFlag)
	mux.HandleFunc("DELETE /flag/{flagId}", flags.NewSystem(s.Config).DeleteFlags)
	mux.HandleFunc("POST /flag/{flagId}/promote", flags.NewSystem(s.Config).PromoteFlag)

	// OFREP (OpenFeature Remote Evaluation Protocol)
	mux.HandleFunc("POST /ofrep/v1/evaluate/flags/{key}", flags.NewOFREPSystem(s.Config).EvaluateSingleFlag)
	mux.HandleFunc("POST /ofrep/v1/evaluate/flags", flags.NewOFREPSystem(s.Config).EvaluateBulkFlags)

	// API Key Management
	mux.HandleFunc("POST /api-key/generate", flags.NewAPIKeyHTTPSystem(s.Config).GenerateAPIKeyHandler)

	// Secret Menu
	mux.HandleFunc("GET /secret-menu/{menuId}", secretmenu.NewSystem(s.Config).GetSecretMenu)
	mux.HandleFunc("POST /secret-menu/{environmentId}", secretmenu.NewSystem(s.Config).CreateSecretMenu)
	mux.HandleFunc("PUT /secret-menu/{menuId}/sequence", secretmenu.NewSystem(s.Config).UpdateSecretMenuSequence)
	mux.HandleFunc("PUT /secret-menu/{menuId}/state", secretmenu.NewSystem(s.Config).UpdateSecretMenuState)
	mux.HandleFunc("PUT /secret-menu/{menuId}/style", secretmenu.NewSystem(s.Config).UpdateSecretMenuStyle)
	mux.HandleFunc("GET /secret-menu/{menuId}/style", secretmenu.NewSystem(s.Config).GetSecretMenuStyle)

	// Stats
	mux.HandleFunc("GET /stats/company", stats.NewSystem(s.Config).GetCompanyStats)
	mux.HandleFunc("GET /stats/agent/{agentId}/environment/{environmentId}", stats.NewSystem(s.Config).GetEnvironmentStats)
	mux.HandleFunc("GET /stats/project/{projectId}", stats.NewSystem(s.Config).GetProjectStats)
	mux.HandleFunc("GET /stats/agent/{agentId}", stats.NewSystem(s.Config).GetAgentStats)

	// User
	mux.HandleFunc("POST /user", user.NewSystem(s.Config).CreateUser)
	mux.HandleFunc("PUT /user", user.NewSystem(s.Config).UpdateUser)
	mux.HandleFunc("GET /user", user.NewSystem(s.Config).GetUser)
	mux.HandleFunc("DELETE /user", user.NewSystem(s.Config).DeleteUser)
	mux.HandleFunc("PUT /user/image", user.NewSystem(s.Config).UpdateUserImage)

	// Notifications
	mux.HandleFunc("GET /user/notifications", user.NewSystem(s.Config).GetUserNotifications)
	mux.HandleFunc("PATCH /user/notification/{notificationId}", user.NewSystem(s.Config).UpdateUserNotification)
	mux.HandleFunc("DELETE /user/notification/{notificationId}", user.NewSystem(s.Config).DeleteUserNotification)

	// Company
	mux.HandleFunc("GET /company", company.NewSystem(s.Config).GetCompany)
	mux.HandleFunc("PUT /company", company.NewSystem(s.Config).UpdateCompany)
	mux.HandleFunc("POST /company", company.NewSystem(s.Config).CreateCompany)
	mux.HandleFunc("GET /company/limits", company.NewSystem(s.Config).GetCompanyLimits)
	mux.HandleFunc("GET /company/pricing", pricing.NewSystem(s.Config).GetCompanyPricing)
	mux.HandleFunc("PUT /company/user", company.NewSystem(s.Config).AttachUserToCompany)
	mux.HandleFunc("GET /company/users", company.NewSystem(s.Config).GetCompanyUsers)
	mux.HandleFunc("PUT /company/image", company.NewSystem(s.Config).UpdateCompanyImage)
	mux.HandleFunc("POST /company/invite", company.NewSystem(s.Config).InviteUserToCompany)
	mux.HandleFunc("PUT /company/upgrade", company.NewSystem(s.Config).UpgradeCompany)

	// General
	mux.HandleFunc(fmt.Sprintf("%s /health", http.MethodGet), healthcheck.HTTP)
	mux.HandleFunc(fmt.Sprintf("%s /probe", http.MethodGet), probe.HTTP)
	mux.HandleFunc("GET /pricing", pricing.NewSystem(s.Config).GetGeneralPricing)
	mux.HandleFunc("/uploadthing", user.NewSystem(s.Config).UploadThing)
	mux.HandleFunc("/events/keycloak", general.NewSystem(s.Config).KeycloakEvents)

	// General Webhooks
	mux.HandleFunc("/webhooks/stripe", general.NewSystem(s.Config).StripeEvents)
	mux.HandleFunc("/v1/webhooks/stripe", general.NewSystem(s.Config).StripeEvents)

	// middlewares
	mw := middleware.NewMiddleware(context.Background())
	mw.AddMiddleware(middleware.SetupLogger(middleware.Error).Logger)
	mw.AddMiddleware(middleware.RequestID)
	mw.AddMiddleware(middleware.Recoverer)
	mw.AddMiddleware(s.Auth)
	mw.AddMiddleware(mw.CORS)
	mw.AddMiddleware(middleware.LowerCaseHeaders)
	mw.AddAllowedHeaders(
		"x-agent-id",
		"x-company-id",
		"x-project-id",
		"x-environment-id",
		"x-user-subject",
		"x-flags-timestamp",
	)
	mw.AddAllowedMethods(http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodPatch)
	mw.AddAllowedOrigins("https://www.flags.gg", "https://flags.gg", "*")
	if s.Config.Local.Development {
		mw.AddAllowedOrigins("http://localhost:3000", "http://localhost:5173", "*")
	}

	port := s.Config.Local.HTTPPort
	if s.Config.ProjectProperties["railway_port"].(string) != "" && s.Config.ProjectProperties["on_railway"].(bool) {
		i, err := strconv.Atoi(s.Config.ProjectProperties["railway_port"].(string))
		if err != nil {
			_ = logs.Errorf("Failed to parse port: %v", err)
			return
		}
		port = i
	}

	logs.Logf("Starting HTTP on %d", s.Config.Local.HTTPPort)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mw.Handler(mux),
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}
	errChan <- server.ListenAndServe()
}
