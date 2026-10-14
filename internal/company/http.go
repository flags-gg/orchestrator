package company

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/auth"
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

func (s *System) GetCompany(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	userId, err := auth.UserID(r)
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

	userId, err := auth.UserID(r)
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
		if errors.Is(err, ErrDomainTaken) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateCompany(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	_, err := auth.UserID(r)
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

	userId, err := auth.UserID(r)
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

// ponytail: hard-coded list, misses regional/smaller providers; swap for a maintained list if that matters
var freeEmailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "outlook.com": true, "hotmail.com": true, "live.com": true,
	"msn.com": true, "yahoo.com": true, "ymail.com": true, "icloud.com": true, "me.com": true, "mac.com": true,
	"aol.com": true, "proton.me": true, "protonmail.com": true, "pm.me": true, "gmx.com": true, "gmx.net": true,
	"mail.com": true, "zoho.com": true, "yandex.com": true, "fastmail.com": true, "hey.com": true, "tutanota.com": true,
}

func IsFreeEmailDomain(domain string) bool {
	return freeEmailDomains[strings.ToLower(domain)]
}

func (s *System) AttachUserToCompany(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("x-flags-timestamp", strconv.FormatInt(time.Now().Unix(), 10))

	type CompanyUser struct {
		Domain     string `json:"domain"`
		InviteCode string `json:"invite_code"`
	}
	user := CompanyUser{}
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Invite codes join as before; a domain only joins when it is the caller's verified email domain,
	// and never for free-mail providers (a gmail.com address proves nothing about a company)
	emailDomain := auth.EmailDomain(r)
	if IsFreeEmailDomain(emailDomain) || !strings.EqualFold(user.Domain, emailDomain) {
		user.Domain = ""
	}
	if user.Domain == "" && user.InviteCode == "" {
		w.WriteHeader(http.StatusForbidden)
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

	if err := s.AttachUserToCompanyDB(ctx, userId); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetCompanyUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userId, err := auth.UserID(r)
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

	userId, err := auth.UserID(r)
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

	userId, err := auth.UserID(r)
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
	if _, err := mail.ParseAddress(invite.Email); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// only members can invite to their own company
	companyId, err := s.GetCompanyId(ctx, userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if companyId == "" {
		w.WriteHeader(http.StatusForbidden)
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
		Html:    fmt.Sprintf("<p>Hello: %s<br />You have been invited to join <a href=\"https://flags.gg\">Flags.gg</a></p><br /><p>The invite code is <strong>%s</strong></p>", html.EscapeString(invite.Name), html.EscapeString(inviteCode)),
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

	userId, err := auth.UserID(r)
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

	if upgradeRequest.StripeSessionId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	session, err := fetchCheckoutSession(s.stripeKey(), upgradeRequest.StripeSessionId)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get stripe session: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := validateCheckoutSession(session, userId); err != nil {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	if err := s.UpgradeCompanyInDB(ctx, companyId, session); err != nil {
		if errors.Is(err, ErrSessionUsed) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// CallerOwns resolves the caller's company and checks they own every (kind, id) pair given.
// It writes the error response itself and returns the company id plus whether the handler may continue.
func (s *System) CallerOwns(w http.ResponseWriter, r *http.Request, kindIds ...string) (string, bool) {
	userId, err := auth.UserID(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return "", false
	}
	companyId, err := s.GetCompanyId(r.Context(), userId)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return "", false
	}
	if companyId == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return "", false
	}
	for i := 0; i+1 < len(kindIds); i += 2 {
		owns, err := s.OwnsResource(r.Context(), companyId, kindIds[i], kindIds[i+1])
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return "", false
		}
		if !owns {
			w.WriteHeader(http.StatusNotFound)
			return "", false
		}
	}
	return companyId, true
}
