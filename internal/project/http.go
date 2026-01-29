package project

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/flags-gg/orchestrator/internal/agent"
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

// getUserId returns the user ID, using dev mode config if in development, otherwise Clerk
func (s *System) getUserId(r *http.Request) (string, error) {
	if s.Config.Local.Development && s.Config.Clerk.DevUser != "" {
		return s.Config.Clerk.DevUser, nil
	}

	// Production mode: use Clerk authentication
	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(r.Context(), r.Header.Get("x-user-subject"))
	if err != nil {
		return "", err
	}
	return usr.ID, nil
}

func (s *System) GetProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	type Projects struct {
		Projects []Project `json:"projects"`
	}
	project := Projects{}

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&project); err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}
		return
	}

	userId, err := s.getUserId(r)
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

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
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

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
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

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, usr.ID)
	if err != nil {
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

	if r.Header.Get("x-user-subject") == "" {
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

	clerk.SetKey(s.Config.Clerk.Key)
	_, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
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
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteProject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&Project{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	//type ProjEdit struct {
	//	Name      string `json:"name"`
	//	ProjectID string `json:"project_id"`
	//}

	projectId := r.PathValue("projectId")

	clerk.SetKey(s.Config.Clerk.Key)
	_, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := agent.NewSystem(s.Config).DeleteAllAgentsForProject(ctx, projectId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	if err := s.DeleteProjectInDB(ctx, projectId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateProjectImage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	if r.Header.Get("x-user-subject") == "" {
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

	clerk.SetKey(s.Config.Clerk.Key)
	_, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := s.UpdateProjectImageInDB(ctx, projectId, imageChange.Image); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
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
