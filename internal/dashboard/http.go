package dashboard

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/flags-gg/orchestrator/internal/environment"
	"github.com/flags-gg/orchestrator/internal/flags"
	"github.com/flags-gg/orchestrator/internal/project"
	"github.com/flags-gg/orchestrator/internal/stats"
	ConfigBuilder "github.com/keloran/go-config"
)

type System struct {
	Config *ConfigBuilder.Config
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{Config: cfg}
}

type EnvironmentCoverage struct {
	Id            string `json:"id"`
	Name          string `json:"name"`
	EnvironmentId string `json:"environment_id"`
	AgentId       string `json:"agent_id"`
	AgentName     string `json:"agent_name,omitempty"`
	ProjectName   string `json:"project_name,omitempty"`
	TotalFlags    int    `json:"totalFlags"`
	EnabledFlags  int    `json:"enabledFlags"`
}

type Summary struct {
	Projects            []project.Project          `json:"projects"`
	Agents              []*agent.Agent             `json:"agents"`
	Environments        []*environment.Environment `json:"environments"`
	AllFlags            []flags.CompanyFlagEntry   `json:"allFlags"`
	NewestProject       *project.Project           `json:"newestProject,omitempty"`
	NewestFlag          *flags.CompanyFlagEntry    `json:"newestFlag,omitempty"`
	RecentFlagChanges   []flags.CompanyFlagEntry   `json:"recentFlagChanges"`
	EnvironmentCoverage []EnvironmentCoverage      `json:"environmentCoverage"`
	Stats               stats.CompanyOverview      `json:"stats"`
}

func parseTimestamp(value string) int64 {
	if value == "" {
		return 0
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.Unix()
	}

	parsed, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return parsed.Unix()
	}

	return 0
}

func (s *System) GetSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	w.Header().Set("Content-Type", "application/json")

	userSubject := r.Header.Get("x-user-subject")
	if userSubject == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userSubject)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	projects, err := project.NewSystem(s.Config).GetProjectsFromDB(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	agents, err := agent.NewSystem(s.Config).GetAgents(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	environments, err := environment.NewSystem(s.Config).GetEnvironmentsFromDB(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	allFlags, err := flags.NewSystem(s.Config).GetCompanyFlagsFromDB(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	overview, err := stats.NewSystem(s.Config).GetCompanyOverview(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if overview == nil {
		overview = &stats.CompanyOverview{}
	}

	coverageMap := make(map[string]*EnvironmentCoverage, len(environments))
	for _, env := range environments {
		coverageMap[env.EnvironmentId] = &EnvironmentCoverage{
			Id:            env.Id,
			Name:          env.Name,
			EnvironmentId: env.EnvironmentId,
			AgentId:       env.AgentId,
			AgentName:     env.AgentName,
			ProjectName:   env.ProjectName,
		}
	}

	for _, entry := range allFlags {
		coverage, ok := coverageMap[entry.Environment.EnvironmentId]
		if !ok {
			coverage = &EnvironmentCoverage{
				Id:            entry.Environment.Id,
				Name:          entry.Environment.Name,
				EnvironmentId: entry.Environment.EnvironmentId,
				AgentId:       entry.Environment.AgentId,
				AgentName:     entry.Environment.AgentName,
				ProjectName:   entry.Environment.ProjectName,
			}
			coverageMap[entry.Environment.EnvironmentId] = coverage
		}

		coverage.TotalFlags++
		if entry.Flag.Enabled {
			coverage.EnabledFlags++
		}
	}

	coverage := make([]EnvironmentCoverage, 0, len(coverageMap))
	for _, item := range coverageMap {
		coverage = append(coverage, *item)
	}
	sort.Slice(coverage, func(i, j int) bool {
		if coverage[i].TotalFlags == coverage[j].TotalFlags {
			return coverage[i].EnvironmentId < coverage[j].EnvironmentId
		}
		return coverage[i].TotalFlags > coverage[j].TotalFlags
	})
	if len(coverage) > 6 {
		coverage = coverage[:6]
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ID > projects[j].ID
	})

	newestProject := (*project.Project)(nil)
	if len(projects) > 0 {
		newestProject = &projects[0]
	}

	allFlagsSorted := append([]flags.CompanyFlagEntry(nil), allFlags...)
	sort.Slice(allFlagsSorted, func(i, j int) bool {
		return allFlagsSorted[i].Flag.Details.ID > allFlagsSorted[j].Flag.Details.ID
	})

	newestFlag := (*flags.CompanyFlagEntry)(nil)
	if len(allFlagsSorted) > 0 {
		newestFlag = &allFlagsSorted[0]
	}

	recentFlagChanges := make([]flags.CompanyFlagEntry, 0, len(allFlags))
	for _, entry := range allFlags {
		if parseTimestamp(entry.Flag.Details.LastChanged) > 0 {
			recentFlagChanges = append(recentFlagChanges, entry)
		}
	}
	sort.Slice(recentFlagChanges, func(i, j int) bool {
		return parseTimestamp(recentFlagChanges[i].Flag.Details.LastChanged) > parseTimestamp(recentFlagChanges[j].Flag.Details.LastChanged)
	})
	if len(recentFlagChanges) > 5 {
		recentFlagChanges = recentFlagChanges[:5]
	}

	response := Summary{
		Projects:            projects,
		Agents:              agents,
		Environments:        environments,
		AllFlags:            allFlags,
		NewestProject:       newestProject,
		NewestFlag:          newestFlag,
		RecentFlagChanges:   recentFlagChanges,
		EnvironmentCoverage: coverage,
		Stats:               *overview,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
