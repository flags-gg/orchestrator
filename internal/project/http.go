package project

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/auth"
	"github.com/flags-gg/orchestrator/internal/company"
	ConfigBuilder "github.com/keloran/go-config"
)

type System struct {
	Config *ConfigBuilder.Config
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) GetProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	type Projects struct {
		Projects []Project `json:"projects"`
	}
	project := Projects{}

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pro, err := s.GetProjectsFromDB(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	project.Projects = pro

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&project); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) GetProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceProject, r.PathValue("projectId"))
	if !ok {
		return
	}

	proj, err := s.GetProjectFromDB(ctx, companyId, r.PathValue("projectId"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&proj); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) CreateProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r)
	if !ok {
		return
	}

	type ProjCreate struct {
		Name string `json:"name"`
	}
	proj := ProjCreate{}
	if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	createdProject, err := s.CreateProjectInDB(ctx, companyId, proj.Name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	projDetails, err := s.GetProjectFromDB(ctx, companyId, createdProject.ProjectID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&projDetails); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) UpdateProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	projectId := r.PathValue("projectId")

	type ProjEdit struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}

	if _, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceProject, projectId); !ok {
		return
	}

	proj := ProjEdit{}
	if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := s.UpdateProjectInDB(ctx, projectId, proj.Name, proj.Enabled); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	//type ProjEdit struct {
	//	Name      string `json:"name"`
	//	ProjectID string `json:"project_id"`
	//}

	projectId := r.PathValue("projectId")

	if _, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceProject, projectId); !ok {
		return
	}

	if err := agent.NewSystem(s.Config).DeleteAllAgentsForProject(ctx, projectId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.DeleteProjectInDB(ctx, projectId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateProjectImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	projectId := r.PathValue("projectId")

	imageChange := struct {
		Image string `json:"image"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&imageChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceProject, projectId); !ok {
		return
	}

	if err := s.UpdateProjectImageInDB(ctx, projectId, imageChange.Image); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceProject, r.PathValue("projectId"))
	if !ok {
		return
	}

	projectId := r.PathValue("projectId")

	limits, err := s.GetLimitsFromDB(ctx, companyId, projectId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&limits); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}
