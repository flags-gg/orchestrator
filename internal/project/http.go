package project

import (
	"context"
	"encoding/json"
	"github.com/flags-gg/orchestrator/internal/agent"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
	"strconv"
	"time"
)

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
}

func (s *System) SetContext(ctx context.Context) *System {
	s.Context = ctx
	return s
}

func (s *System) GetProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	type Projects struct {
		Projects []Project `json:"projects"`
	}
	project := Projects{}

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&project); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	pro, err := s.GetProjectsFromDB(r.Header.Get("x-user-subject"))
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
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	proj, err := s.GetProjectFromDB(r.Header.Get("x-user-subject"), r.PathValue("projectId"))
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
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	userSubject := r.Header.Get("x-user-subject")

	type ProjCreate struct {
		Name string `json:"name"`
	}
	proj := ProjCreate{}
	if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	createdProject, err := s.CreateProjectInDB(userSubject, proj.Name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	projDetails, err := s.GetProjectFromDB(userSubject, createdProject.ProjectID)
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
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	projectId := r.PathValue("projectId")

	type ProjEdit struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}

	proj := ProjEdit{}
	if err := json.NewDecoder(r.Body).Decode(&proj); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := s.UpdateProjectInDB(projectId, proj.Name, proj.Enabled); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	type ProjEdit struct {
		Name      string `json:"name"`
		ProjectID string `json:"project_id"`
	}

	projectId := r.PathValue("projectId")

	if err := agent.NewSystem(s.Config).SetContext(r.Context()).DeleteAllAgentsForProject(projectId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	if err := s.DeleteProjectInDB(projectId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateProjectImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	projectId := r.PathValue("projectId")

	imageChange := struct {
		Image string `json:"image"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&imageChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.UpdateProjectImageInDB(projectId, imageChange.Image); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}
