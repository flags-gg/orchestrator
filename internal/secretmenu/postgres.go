package secretmenu

import (
	"context"
	"database/sql"
	"errors"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	ConfigBuilder "github.com/keloran/go-config"
	"strings"
)

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

type MenuStyle struct {
	Id          sql.NullString `json:"style_id,omitempty"`
	CloseButton sql.NullString `json:"close_button,omitempty"`
	Container   sql.NullString `json:"container,omitempty"`
	Button      sql.NullString `json:"button,omitempty"`
}

type SecretMenu struct {
	Id          string    `json:"menu_id"`
	Enabled     bool      `json:"enabled"`
	Sequence    []string  `json:"sequence,omitempty"`
	CustomStyle MenuStyle `json:"custom_style,omitempty"`
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

func (s *System) GetEnvironmentSecretMenu(environmentId string) (SecretMenu, error) {
	var secretMenu SecretMenu
	var menuStyle MenuStyle

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return secretMenu, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var sequence sql.NullString

	if err := client.QueryRow(s.Context, `
    SELECT
        menu_id,
        environment_secret_menu.enabled,
        code,
        closebutton,
        container,
        button,
        style_id
    FROM public.environment_secret_menu
        LEFT JOIN public.secret_menu_style ON secret_menu_style.secret_menu_id = environment_secret_menu.id
        JOIN public.agent_environment ON agent_environment.id = environment_secret_menu.environment_id
    WHERE env_id = $1`, environmentId).Scan(
		&secretMenu.Id,
		&secretMenu.Enabled,
		&sequence,
		&menuStyle.CloseButton,
		&menuStyle.Container,
		&menuStyle.Button,
		&menuStyle.Id); err != nil {
		if err.Error() == "context canceled" {
			return secretMenu, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return secretMenu, nil
		}
		return secretMenu, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	if sequence.Valid {
		secretMenu.Sequence = strings.Split(sequence.String, ",")
	}
	secretMenu.CustomStyle = menuStyle

	return secretMenu, nil
}

func (s *System) UpdateSecretMenuInDB(menuId string, secretMenu SecretMenu) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	sequence := strings.Join(secretMenu.Sequence, ",")
	if _, err := client.Exec(s.Context, `
    UPDATE public.environment_secret_menu
    SET enabled = $1, code = $2
    WHERE menu_id = $3`, secretMenu.Enabled, sequence, menuId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update database: %v", err)
	}

	if secretMenu.CustomStyle.Id.String == "" {
		uu, err := uuid.NewRandom()
		if err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to generate UUID: %v", err)
		}
		styleId := uu.String()

		var secretMenuId int
		if err := client.QueryRow(s.Context, `SELECT id FROM public.environment_secret_menu WHERE menu_id = $1`, menuId).Scan(&secretMenuId); err != nil {
			if err.Error() == "context canceled" {
				return nil
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}

			return s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
		}

		if _, err := client.Exec(s.Context, `
      INSERT INTO public.secret_menu_style (secret_menu_id, closebutton, container, button, style_id)
      VALUES ($1, $2, $3, $4, $5)`, secretMenuId, secretMenu.CustomStyle.CloseButton, secretMenu.CustomStyle.Container, secretMenu.CustomStyle.Button, styleId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to insert into database: %v", err)
		}
		return nil
	}

	if _, err := client.Exec(s.Context, `
    UPDATE public.secret_menu_style
    SET closebutton = $1, container = $2, button = $3
    WHERE style_id = $4`, secretMenu.CustomStyle.CloseButton, secretMenu.CustomStyle.Container, secretMenu.CustomStyle.Button, secretMenu.CustomStyle.Id); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update database: %v", err)
	}

	return nil
}

func (s *System) CreateSecretMenuInDB(environmentId string, secretMenu SecretMenu) (string, string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	sequence := strings.Join(secretMenu.Sequence, ",")
	menuId, err := uuid.NewRandom()
	if err != nil {
		return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to generate UUID: %v", err)
	}

	var env_id int
	var agent_id int
	if err := client.QueryRow(s.Context, `SELECT agent_id, id FROM public.agent_environment WHERE env_id = $1`, environmentId).Scan(&agent_id, &env_id); err != nil {
		if err.Error() == "context canceled" {
			return "", "", nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}

		return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	if _, err := client.Exec(s.Context, `INSERT INTO public.environment_secret_menu (menu_id, environment_id, enabled, code, agent_id) VALUES ($1, $2, $3, $4, $5)`, menuId.String(), env_id, secretMenu.Enabled, sequence, agent_id); err != nil {
		return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to insert into database: %v", err)
	}

	if secretMenu.CustomStyle.Id.String == "" {
		uu, err := uuid.NewRandom()
		if err != nil {
			return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to generate UUID: %v", err)
		}
		styleId := uu.String()

		var secretMenuId int
		if err := client.QueryRow(s.Context, `SELECT id FROM public.environment_secret_menu WHERE menu_id = $1`, menuId.String()).Scan(&secretMenuId); err != nil {
			if err.Error() == "context canceled" {
				return "", "", nil
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return "", "", nil
			}

			return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
		}

		if _, err := client.Exec(s.Context, `
      INSERT INTO public.secret_menu_style (secret_menu_id, closebutton, container, button, style_id)
      VALUES ($1, $2, $3, $4, $5)`, secretMenuId, secretMenu.CustomStyle.CloseButton, secretMenu.CustomStyle.Container, secretMenu.CustomStyle.Button, styleId); err != nil {
			return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to insert into database: %v", err)
		}
		return menuId.String(), uu.String(), nil
	}

	return menuId.String(), "", nil
}
