package project

import (
	"context"
	"encoding/json"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
	"strconv"
	"time"
)

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

type Project struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	CompanyID string `json:"company_id"`
}

func NewSystem(cfg *ConfigBuilder.Config) *System {
	return &System{
		Config: cfg,
	}
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
}
