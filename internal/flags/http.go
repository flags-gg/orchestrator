package flags

import (
	"context"
	"encoding/json"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/flags-gg/orchestrator/internal/company"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
	"strconv"
	"time"
)

type SecretMenuStyle struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type SecretMenu struct {
	Sequence []string          `json:"sequence,omitempty"`
	Styles   []SecretMenuStyle `json:"styles,omitempty"`
}
type Details struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	LastChanged string `json:"lastChanged,omitempty"`
	Promoted    bool   `json:"promoted,omitempty"`
}
type Flag struct {
	Enabled bool    `json:"enabled"`
	Details Details `json:"details"`
}
type AgentResponse struct {
	IntervalAllowed int        `json:"intervalAllowed,omitempty"`
	SecretMenu      SecretMenu `json:"secretMenu,omitempty"`
	Flags           []Flag     `json:"flags,omitempty"`
}
type Response struct {
	Flags []Flag `json:"flags"`
}

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

type FlagNameChangeRequest struct {
	Name string `json:"name"`
	ID   string `json:"id"`
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

// getUserId returns the user ID, using dev mode config if in development, otherwise Clerk
func (s *System) getUserId(r *http.Request) (string, error) {
	if s.Config.Local.Development && s.Config.Clerk.DevUser != "" {
		return s.Config.Clerk.DevUser, nil
	}

	// Production mode: use Clerk authentication
	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(s.Context, r.Header.Get("x-user-subject"))
	if err != nil {
		return "", err
	}
	return usr.ID, nil
}

func (s *System) GetAgentFlags(w http.ResponseWriter, r *http.Request) {
	logs.Infof("Headers: %v", r.Header)

	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	w.Header().Set("Content-Type", "application/json")
	s.Context = r.Context()

	if r.Header.Get("x-project-id") == "" || r.Header.Get("x-agent-id") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var responseObj AgentResponse

	projectId := r.Header.Get("x-project-id")
	agentId := r.Header.Get("x-agent-id")
	environmentId := r.Header.Get("x-environment-id")

	res, err := s.GetAgentFlagsFromDB(projectId, agentId, environmentId)
	if err != nil {
		responseObj = AgentResponse{
			IntervalAllowed: 600,
			Flags:           []Flag{},
		}
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get flags: %v", err)
	}
	if res != nil {
		responseObj = *res
	}

	if err := json.NewEncoder(w).Encode(responseObj); err != nil {
		_, _ = w.Write([]byte(`{"error": "failed to encode response"}`))
		//stats.NewSystem(s.Config).AddAgentError(projectId, agentId, environmentId)
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
	//stats.NewSystem(s.Config).AddAgentSuccess(projectId, agentId, environmentId)
}

func (s *System) GetClientFlags(w http.ResponseWriter, r *http.Request) {
	var responseObj []Flag
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	environmentId := r.PathValue("environmentId")

	res, err := s.GetClientFlagsFromDB(environmentId)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get flags: %v", err)
	}
	responseObj = res

	if err := json.NewEncoder(w).Encode(responseObj); err != nil {
		_, _ = w.Write([]byte(`{"error": "failed to encode response"}`))
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
	}
}

func (s *System) CreateFlags(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	flag := flagCreate{}
	if err := json.NewDecoder(r.Body).Decode(&flag); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.CreateFlagInDB(flag); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateFlags(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type changeRequest struct {
		Enabled bool   `json:"enabled"`
		Name    string `json:"name"`
	}

	cr := changeRequest{}
	if err := json.NewDecoder(r.Body).Decode(&cr); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	flagChange := Flag{
		Enabled: cr.Enabled,
		Details: Details{
			Name: cr.Name,
			ID:   r.PathValue("flagId"),
		},
	}
	if err := s.UpdateFlagInDB(flagChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return

	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) PromoteFlag(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	flagId := r.PathValue("flagId")
	if flagId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.PromoteFlagInDB(flagId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to promote flag: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) EditFlag(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	flagId := r.PathValue("flagId")
	flagChange := FlagNameChangeRequest{}
	if err := json.NewDecoder(r.Body).Decode(&flagChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	flagChange.ID = flagId

	if err := s.EditFlagInDB(flagChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return

	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteFlags(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := company.NewSystem(s.Config).SetContext(s.Context).GetCompanyId(userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	f := Flag{
		Details: Details{
			ID: r.PathValue("flagId"),
		},
	}

	if err := s.DeleteFlagFromDB(f); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to delete flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
