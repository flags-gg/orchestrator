package internal

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"

	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/dashboard"
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
	Config     *ConfigBuilder.Config
	jwtOptions []clerkhttp.AuthorizationOption
}

func New(cfg *ConfigBuilder.Config) *Service {
	clerk.SetKey(cfg.Clerk.Key)
	return &Service{
		Config:     cfg,
		jwtOptions: []clerkhttp.AuthorizationOption{clerkhttp.AuthorizedPartyMatches(dashboardOrigins(cfg)...)},
	}
}

// dashboardOrigins are the browser origins allowed on dashboard/user routes (CORS and the JWT azp claim).
func dashboardOrigins(cfg *ConfigBuilder.Config) []string {
	origins := []string{"https://dashboard.flags.gg", "https://flags.gg", "https://www.flags.gg"}
	if cfg.Local.Development {
		origins = append(origins, "http://localhost:3000", "http://localhost:5173")
	}
	return origins
}

// isSDKPath reports whether a path is served to SDKs (agent auth, any CORS origin).
func isSDKPath(path string) bool {
	return path == "/flags" || path == "/v1/flags" || strings.HasPrefix(path, "/ofrep/")
}

func (s *Service) Start() error {
	errChan := make(chan error)

	go s.startHTTP(errChan)

	return <-errChan
}

// routes registers every endpoint in exactly one group. Anything not registered is a 404.
//   - authed: Clerk session JWT + verified email (dashboard / user)
//   - sdk: agent credentials
//   - mux.HandleFunc: public (health, pricing, signed Stripe webhook)
func (s *Service) routes() http.Handler {
	mux := http.NewServeMux()
	authed := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.RequireUser(h)) }
	sdk := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.RequireAgent(h)) }

	// Projects
	authed("GET /projects", project.NewSystem(s.Config).GetProjects)
	authed("POST /project", project.NewSystem(s.Config).CreateProject)
	authed("GET /project/{projectId}", project.NewSystem(s.Config).GetProject)
	authed("PUT /project/{projectId}", project.NewSystem(s.Config).UpdateProject)
	authed("PUT /project/{projectId}/image", project.NewSystem(s.Config).UpdateProjectImage)
	authed("GET /project/{projectId}/limits", project.NewSystem(s.Config).GetLimits)
	authed("DELETE /project/{projectId}", project.NewSystem(s.Config).DeleteProject)

	// Agents
	authed("GET /project/{projectId}/agents", agent.NewSystem(s.Config).GetProjectAgents)
	authed("POST /project/{projectId}/agent", agent.NewSystem(s.Config).CreateAgent)
	authed("GET /agents", agent.NewSystem(s.Config).GetAgentsRequest)
	authed("GET /agent/{agentId}", agent.NewSystem(s.Config).GetAgent)
	authed("PUT /agent/{agentId}", agent.NewSystem(s.Config).UpdateAgent)
	authed("DELETE /agent/{agentId}", agent.NewSystem(s.Config).DeleteAgent)

	// Environments
	authed("GET /agent/{agentId}/environments", environment.NewSystem(s.Config).GetAgentEnvironments)
	authed("POST /agent/{agentId}/environment", environment.NewSystem(s.Config).CreateAgentEnvironment)
	authed("POST /agent/{agentId}/{environmentId}", environment.NewSystem(s.Config).CloneAgentEnvironment)
	authed("GET /environment/{environmentId}", environment.NewSystem(s.Config).GetEnvironment)
	authed("PUT /environment/{environmentId}", environment.NewSystem(s.Config).UpdateEnvironment)
	authed("DELETE /environment/{environmentId}", environment.NewSystem(s.Config).DeleteEnvironment)
	authed("GET /environments", environment.NewSystem(s.Config).GetEnvironments)

	// Flags
	authed("GET /environment/{environmentId}/flags", flags.NewSystem(s.Config).GetClientFlags) // used by the frontend
	authed("POST /flag", flags.NewSystem(s.Config).CreateFlags)
	authed("PATCH /flag/{flagId}", flags.NewSystem(s.Config).UpdateFlags)
	authed("PUT /flag/{flagId}", flags.NewSystem(s.Config).EditFlag)
	authed("DELETE /flag/{flagId}", flags.NewSystem(s.Config).DeleteFlags)
	authed("POST /flag/{flagId}/promote", flags.NewSystem(s.Config).PromoteFlag)

	// Client
	sdk("POST /ofrep/v1/evaluate/flags/{key}", flags.NewOFREPSystem(s.Config).EvaluateSingleFlag)
	sdk("POST /ofrep/v1/evaluate/flags", flags.NewOFREPSystem(s.Config).EvaluateBulkFlags)
	sdk("GET /v1/flags", flags.NewSystem(s.Config).GetAgentFlags)
	sdk("GET /flags", flags.NewSystem(s.Config).GetAgentFlags)

	// API Key Management
	authed("POST /api-key/generate", flags.NewAPIKeyHTTPSystem(s.Config).GenerateAPIKeyHandler)

	// Secret Menu
	authed("GET /secret-menu/{menuId}", secretmenu.NewSystem(s.Config).GetSecretMenu)
	authed("POST /secret-menu/{environmentId}", secretmenu.NewSystem(s.Config).CreateSecretMenu)
	authed("PUT /secret-menu/{menuId}/sequence", secretmenu.NewSystem(s.Config).UpdateSecretMenuSequence)
	authed("PUT /secret-menu/{menuId}/state", secretmenu.NewSystem(s.Config).UpdateSecretMenuState)
	authed("PUT /secret-menu/{menuId}/style", secretmenu.NewSystem(s.Config).UpdateSecretMenuStyle)
	authed("GET /secret-menu/{menuId}/style", secretmenu.NewSystem(s.Config).GetSecretMenuStyle)

	// Stats
	authed("GET /stats/dashboard", dashboard.NewSystem(s.Config).GetSummary)
	authed("GET /stats/company", stats.NewSystem(s.Config).GetCompanyStats)
	authed("GET /stats/agent/{agentId}/environment/{environmentId}", stats.NewSystem(s.Config).GetEnvironmentStats)
	authed("GET /stats/project/{projectId}", stats.NewSystem(s.Config).GetProjectStats)
	authed("GET /stats/agent/{agentId}", stats.NewSystem(s.Config).GetAgentStats)

	// User
	authed("POST /user", user.NewSystem(s.Config).CreateUser)
	authed("PUT /user", user.NewSystem(s.Config).UpdateUser)
	authed("GET /user", user.NewSystem(s.Config).GetUser)
	authed("DELETE /user", user.NewSystem(s.Config).DeleteUser)
	authed("PUT /user/image", user.NewSystem(s.Config).UpdateUserImage)

	// Notifications
	authed("GET /user/notifications", user.NewSystem(s.Config).GetUserNotifications)
	authed("PATCH /user/notification/{notificationId}", user.NewSystem(s.Config).UpdateUserNotification)
	authed("DELETE /user/notification/{notificationId}", user.NewSystem(s.Config).DeleteUserNotification)

	// Company
	authed("GET /company", company.NewSystem(s.Config).GetCompany)
	authed("PUT /company", company.NewSystem(s.Config).UpdateCompany)
	authed("POST /company", company.NewSystem(s.Config).CreateCompany)
	authed("GET /company/limits", company.NewSystem(s.Config).GetCompanyLimits)
	authed("GET /company/pricing", pricing.NewSystem(s.Config).GetCompanyPricing)
	authed("PUT /company/user", company.NewSystem(s.Config).AttachUserToCompany)
	authed("GET /company/users", company.NewSystem(s.Config).GetCompanyUsers)
	authed("PUT /company/image", company.NewSystem(s.Config).UpdateCompanyImage)
	authed("POST /company/invite", company.NewSystem(s.Config).InviteUserToCompany)
	authed("PUT /company/upgrade", company.NewSystem(s.Config).UpgradeCompany)

	// General
	mux.HandleFunc(fmt.Sprintf("%s /health", http.MethodGet), healthcheck.HTTP)
	mux.HandleFunc(fmt.Sprintf("%s /probe", http.MethodGet), probe.HTTP)
	mux.HandleFunc("GET /pricing", pricing.NewSystem(s.Config).GetGeneralPricing)
	authed("POST /uploadthing", user.NewSystem(s.Config).UploadThing)

	// General Webhooks
	mux.HandleFunc("POST /webhooks/stripe", general.NewSystem(s.Config).StripeEvents)
	mux.HandleFunc("POST /v1/webhooks/stripe", general.NewSystem(s.Config).StripeEvents)

	// middlewares
	headers := []string{
		"authorization",
		"x-api-key",
		"x-agent-id",
		"x-company-id",
		"x-project-id",
		"x-environment-id",
		"x-flags-timestamp",
	}
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions, http.MethodPatch}
	sdkCORS := middleware.NewMiddleware()
	sdkCORS.AddAllowedHeaders(headers...)
	sdkCORS.AddAllowedMethods(methods...)
	sdkCORS.AddAllowedOrigins("*")
	dashboardCORS := middleware.NewMiddleware()
	dashboardCORS.AddAllowedHeaders(headers...)
	dashboardCORS.AddAllowedMethods(methods...)
	dashboardCORS.AddAllowedOrigins(dashboardOrigins(s.Config)...)

	mw := middleware.NewMiddleware()
	mw.AddMiddleware(middleware.SetupLogger(middleware.Error).Logger)
	mw.AddMiddleware(middleware.RequestID)
	mw.AddMiddleware(middleware.Recoverer)
	mw.AddMiddleware(func(next http.Handler) http.Handler {
		sdkNext, dashboardNext := sdkCORS.CORS(next), dashboardCORS.CORS(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSDKPath(r.URL.Path) {
				sdkNext.ServeHTTP(w, r)
				return
			}
			dashboardNext.ServeHTTP(w, r)
		})
	})
	mw.AddMiddleware(middleware.LowerCaseHeaders)
	mw.AddMiddleware(LimitBody)

	return mw.Handler(mux)
}

func (s *Service) startHTTP(errChan chan error) {
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
		Handler:           s.routes(),
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		TLSNextProto:      make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}
	errChan <- server.ListenAndServe()
}
