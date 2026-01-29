package user

import (
	"context"
	"errors"
	"strings"

	"github.com/Nerzal/gocloak/v13"
	"github.com/bugfixes/go-bugfixes/logs"
)

func (s *System) GetKeycloakDetails(ctx context.Context, subject string) (*gocloak.User, error) {
	client, token, err := s.Config.Keycloak.GetClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "ingress.local") {
			logs.Fatalf("DNS error killing process: %v", err)
			return nil, nil
		}

		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak client: %v", err)
	}

	user, err := client.GetUserByID(ctx, token.AccessToken, s.Config.Keycloak.Realm, subject)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get user by id: %v", err)
	}

	return user, nil
}

func (s *System) DeleteUserInKeycloak(ctx context.Context, subject string) error {
	client, token, err := s.Config.Keycloak.GetClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "ingress.local") {
			logs.Fatalf("DNS error killing process: %v", err)
			return nil
		}

		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil
		}

		return s.Config.Bugfixes.Logger.Errorf("Failed to get keycloak client: %v", err)
	}

	err = client.DeleteUser(ctx, token.AccessToken, s.Config.Keycloak.Realm, subject)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete user: %v", err)
	}

	return nil
}
