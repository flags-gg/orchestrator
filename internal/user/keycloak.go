package user

import (
	"github.com/Nerzal/gocloak/v13"
	"github.com/bugfixes/go-bugfixes/logs"
	"strings"
)

func (s *System) GetKeycloakDetails(subject string) (*gocloak.User, error) {
	client, token, err := s.Config.Keycloak.GetClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "ingress.local") {
			logs.Fatalf("DNS error killing process: %v", err)
			return nil, nil
		}

		if strings.Contains(err.Error(), "context canceled") {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak client: %v", err)
	}

	user, err := client.GetUserByID(s.Context, token.AccessToken, s.Config.Keycloak.Realm, subject)
	if err != nil {
		if strings.Contains(err.Error(), "context canceled") {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get user by id: %v", err)
	}

	return user, nil
}

func (s *System) DeleteUserInKeycloak(subject string) error {
	client, token, err := s.Config.Keycloak.GetClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "ingress.local") {
			logs.Fatalf("DNS error killing process: %v", err)
			return nil
		}

		if strings.Contains(err.Error(), "context canceled") {
			return nil
		}

		return s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak client: %v", err)
	}

	err = client.DeleteUser(s.Context, token.AccessToken, s.Config.Keycloak.Realm, subject)
	if err != nil {
		if strings.Contains(err.Error(), "context canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete user: %v", err)
	}

	return nil
}
