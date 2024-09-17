package secretmenu

import (
	"database/sql"
	"encoding/json"
	"github.com/flags-gg/orchestrator/internal/company"
	"net/http"
)

func (s *System) GetSecretMenu(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-access-token") == "" || r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	menuId := r.PathValue("menuId")
	secretMenu, err := s.GetSecretMenuFromDB(menuId)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(secretMenu); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) CreateSecretMenu(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-access-token") == "" || r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	envId := r.PathValue("environmentId")
	menuUpdate := SecretMenu{}
	if err := json.NewDecoder(r.Body).Decode(&menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	menuId, styleId, err := s.CreateSecretMenuInDB(envId, menuUpdate)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	menuUpdate.Id = menuId
	if styleId != "" {
		sid := sql.NullString{String: styleId, Valid: true}
		menuUpdate.CustomStyle.Id = sid
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("failed to encode menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) UpdateSecretMenuState(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-access-token") == "" || r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	menuUpdate := SecretMenu{}
	if err := json.NewDecoder(r.Body).Decode(&menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	menuId := r.PathValue("menuId")
	if err := s.UpdateSecretMenuStateInDB(menuId, menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) UpdateSecretMenuSequence(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-access-token") == "" || r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	menuUpdate := SecretMenu{}
	if err := json.NewDecoder(r.Body).Decode(&menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	menuId := r.PathValue("menuId")
	if err := s.UpdateSecretMenuSequenceInDB(menuId, menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) UpdateSecretMenuStyle(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-access-token") == "" || r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	menuUpdate := SecretMenu{}
	if err := json.NewDecoder(r.Body).Decode(&menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	menuId := r.PathValue("menuId")
	if err := s.UpdateSecretMenuStyleInDB(menuId, menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
