package company

import (
	"context"
	"encoding/json"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
	"strconv"
	"time"
)

type System struct {
	Config    *ConfigBuilder.Config
	Context   context.Context
	CompanyID string
}

type Company struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	Domain     string `json:"domain"`
	InviteCode string `json:"invite_code"`
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

func (s *System) GetCompany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	userSubject := r.Header.Get("x-user-subject")
	company, err := s.GetCompanyInfo(userSubject)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(company); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) CreateCompany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) GetCompanyLimits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	userSubject := r.Header.Get("x-user-subject")

	projectLimits, err := s.GetProjectLimits(userSubject)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	userLimits, err := s.GetUserLimits(userSubject)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	agentLimits, err := s.GetAgentLimits(userSubject)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if userLimits == nil || projectLimits == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// This is a dummy response
	limits := Limits{
		Projects: *projectLimits,
		Users:    *userLimits,
		Agents:   *agentLimits,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&limits); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) AttachUserToCompany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" || r.Header.Get("x-user-access-token") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	type CompanyUser struct {
		Domain     string `json:"domain"`
		InviteCode string `json:"invite_code"`
	}
	user := CompanyUser{}
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	company, err := s.GetCompanyBasedOnDomain(user.Domain, user.InviteCode)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !company {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	userSubject := r.Header.Get("x-user-subject")
	if err := s.AttachUserToCompanyDB(userSubject); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
