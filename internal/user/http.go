package user

import (
	"context"
	"github.com/Nerzal/gocloak/v13"
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

	subject := r.URL.Query().Get("subject")
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

	w.WriteHeader(http.StatusOK)
}
