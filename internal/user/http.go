package user

import (
	"context"
	"encoding/json"
	"github.com/Nerzal/gocloak/v13"
	"github.com/bugfixes/go-bugfixes/logs"
	ConfigBuilder "github.com/keloran/go-config"
	"net/http"
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

func (s *System) ValidateUser(ctx context.Context, subject string) bool {
	if subject == "" {
		return false
	}

	user, err := s.GetKeycloakDetails(ctx, subject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak details: %v", err)
		return false
	}
	if user == nil {
		return false
	}

	return true
}

func (s *System) GetKeycloakDetails(ctx context.Context, subject string) (*gocloak.User, error) {
	client, token, err := s.Config.Keycloak.GetClient(ctx)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak client: %v", err)
	}

	user, err := client.GetUserByID(ctx, token.AccessToken, s.Config.Keycloak.Realm, subject)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get user by id: %v", err)
	}

	return user, nil
}

func (s *System) CreateUser(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	userSubject := r.Header.Get("x-user-subject")
	if userSubject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No user subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	cloakDetails, err := s.GetKeycloakDetails(s.Context, userSubject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, err := s.RetrieveUserDetails(userSubject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// User already exists
	if user != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Create user
	if err := s.CreateUserDetails(userSubject, *cloakDetails.Email); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to create user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) GetUser(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	user, err := s.RetrieveUserDetails(subject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if user == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	type UserDetails struct {
		KnownAs string `json:"knownAs"`
		Email   string `json:"emailAddress"`
	}

	if err := json.NewEncoder(w).Encode(&UserDetails{
		KnownAs: *user.KnownAs,
		Email:   *user.Email,
	}); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user details: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateUser(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	type formData struct {
		KnownAs string `json:"knownAs"`
		Email   string `json:"emailAddress"`
	}
	fd := formData{}
	if err := json.NewDecoder(r.Body).Decode(&fd); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to decode form data: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	logs.Logf("knownAs: %s, email: %s, subject: %s", fd.KnownAs, fd.Email, subject)
}

func (s *System) GetUserNotifications(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	n := &Notifications{}

	notifications, err := s.RetrieveUserNotifications(subject)
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to retrieve user notifications: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if notifications == nil {
		if err := json.NewEncoder(w).Encode(n); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user notifications: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		return
	}

	n.Notifications = notifications
	if err := json.NewEncoder(w).Encode(n); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to encode user notifications: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) UpdateUserNotification(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.MarkNotificationAsRead(subject, notificationId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *System) DeleteUserNotification(w http.ResponseWriter, r *http.Request) {
	s.Context = r.Context()

	subject := r.Header.Get("x-user-subject")
	if subject == "" {
		_ = s.Config.Bugfixes.Logger.Errorf("No subject provided")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	notificationId := r.PathValue("notificationId")
	if err := s.DeleteUserNotificationInDB(subject, notificationId); err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to update user notification: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
