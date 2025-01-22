package secretmenu

import (
	"database/sql"
	"encoding/json"
	"github.com/flags-gg/orchestrator/internal/company"
	"net/http"

	flagsService "github.com/flags-gg/go-flags"
)

type Style struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type StyleMenu struct {
	Id     string  `json:"style_id"`
	Styles []Style `json:"styles"`
}

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
		menuUpdate.CustomStyle.SQLId = sql.NullString{String: styleId, Valid: true}
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

	menuId := r.PathValue("menuId")
	if err := s.UpdateSecretMenuStateInDB(menuId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

	flags := flagsService.NewClient(flagsService.WithAuth(flagsService.Auth{
		ProjectID:     s.Config.ProjectProperties["flags_project"].(string),
		AgentID:       s.Config.ProjectProperties["flags_agent"].(string),
		EnvironmentID: s.Config.ProjectProperties["flags_environment"].(string),
	}))
	if !flags.Is("menu style").Enabled() {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

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

	var menuUpdate SecretMenu
	if err := json.NewDecoder(r.Body).Decode(&menuUpdate); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// reset button
	b, err := json.Marshal(menuUpdate.CustomStyle.ResetButton)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	menuUpdate.CustomStyle.SQLResetButton = sql.NullString{String: string(b), Valid: true}

	// close button
	b, err = json.Marshal(menuUpdate.CustomStyle.CloseButton)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	menuUpdate.CustomStyle.SQLCloseButton = sql.NullString{String: string(b), Valid: true}

	// container
	b, err = json.Marshal(menuUpdate.CustomStyle.Container)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	menuUpdate.CustomStyle.SQLContainer = sql.NullString{String: string(b), Valid: true}

	// flag
	b, err = json.Marshal(menuUpdate.CustomStyle.Flag)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	menuUpdate.CustomStyle.SQLFlag = sql.NullString{String: string(b), Valid: true}

	// button enabled
	b, err = json.Marshal(menuUpdate.CustomStyle.ButtonEnabled)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	menuUpdate.CustomStyle.SQLButtonEnabled = sql.NullString{String: string(b), Valid: true}

	// button disabled
	b, err = json.Marshal(menuUpdate.CustomStyle.ButtonDisabled)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	menuUpdate.CustomStyle.SQLButtonDisabled = sql.NullString{String: string(b), Valid: true}

	// header
	b, err = json.Marshal(menuUpdate.CustomStyle.Header)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	menuUpdate.CustomStyle.SQLHeader = sql.NullString{String: string(b), Valid: true}

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

func (s *System) GetSecretMenuStyle(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	flags := flagsService.NewClient(flagsService.WithAuth(flagsService.Auth{
		ProjectID:     s.Config.ProjectProperties["flags_project"].(string),
		AgentID:       s.Config.ProjectProperties["flags_agent"].(string),
		EnvironmentID: s.Config.ProjectProperties["flags_environment"].(string),
	}))
	if !flags.Is("menu style").Enabled() {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

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
	secretMenu, err := s.GetSecretMenuStyleFromDB(menuId)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get secret menu: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if secretMenu.Id == "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(secretMenu); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}
