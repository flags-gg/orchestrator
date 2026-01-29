package company

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/clerk/clerk-sdk-go/v2"
	clerkUser "github.com/clerk/clerk-sdk-go/v2/user"
	ConfigBuilder "github.com/keloran/go-config"
	"github.com/resend/resend-go/v2"
)

type System struct {
	Config    *ConfigBuilder.Config
	CompanyID string
}

type Company struct {
	Name       string `json:"name"`
	ID         string `json:"id"`
	Domain     string `json:"domain"`
	InviteCode string `json:"invite_code"`
	Logo       string `json:"logo"`
	LogoDB     *sql.NullString
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

func (s *System) GetCompany(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	company, err := s.GetCompanyInfo(ctx, userId)
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
	ctx := r.Context()
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	if r.Header.Get("x-user-subject") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	type C struct {
		Name   string `json:"name"`
		Domain string `json:"domain"`
	}
	c := C{}
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if c.Name == "" || c.Domain == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.CreateCompanyDB(ctx, c.Name, c.Domain, userId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	_, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusNotImplemented)
}

func (s *System) GetCompanyLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	w.Header().Set("x-flags-timestamp", ts)

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	userId, err := s.getUserId(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	companyId, err := s.GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	limits, err := s.GetLimits(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&limits); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (s *System) AttachUserToCompany(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	if r.Header.Get("x-user-subject") == "" {
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

	company, err := s.GetCompanyBasedOnDomain(ctx, user.Domain, user.InviteCode)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if !company {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	clerk.SetKey(s.Config.Clerk.Key)
	usr, err := clerkUser.Get(ctx, r.Header.Get("x-user-subject"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := s.AttachUserToCompanyDB(ctx, usr.ID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetCompanyUsers(w http.ResponseWriter, r *http.Request) {
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

	companyId, err := s.GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	users, err := s.GetCompanyUsersFromDB(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(users); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *System) UpdateCompanyImage(w http.ResponseWriter, r *http.Request) {
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

	companyId, err := s.GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	imageChange := struct {
		Image string `json:"image"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&imageChange); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode body: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.UpdateCompanyImageInDB(ctx, companyId, imageChange.Image); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update project: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) InviteUserToCompany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	ctx := r.Context()

	if r.Header.Get("x-user-subject") == "" {
		if err := json.NewEncoder(w).Encode(&Company{}); err != nil {
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

	type Invite struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	var invite Invite
	if err := json.NewDecoder(r.Body).Decode(&invite); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	companyId, err := s.GetCompanyId(ctx, usr.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	inviteCode, err := s.GetInviteCodeFromDB(ctx, companyId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Create the invite
	client := resend.NewClient(s.Config.Resend.Key)
	params := &resend.SendEmailRequest{
		From:    "Flags.gg <support@flags.gg>",
		To:      []string{invite.Email},
		Subject: "Flags.gg Invite",
		Html:    fmt.Sprintf("<p>Hello: %s<br />You have been invited to join <a href=\"https://flags.gg\">Flags.gg</a></p><br /><p>The invite code is <strong>%s</strong></p>", invite.Name, inviteCode),
		ReplyTo: "support@flags.gg",
	}
	if _, err = client.Emails.Send(params); err != nil {
		logs.Logf("Failed to send invite: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpgradeCompany(w http.ResponseWriter, r *http.Request) {
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

	companyId, err := s.GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type upgrade struct {
		StripeSessionId string `json:"sessionId"`
	}
	var upgradeRequest upgrade
	if err := json.NewDecoder(r.Body).Decode(&upgradeRequest); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.UpgradeCompanyInDB(ctx, companyId, upgradeRequest.StripeSessionId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
