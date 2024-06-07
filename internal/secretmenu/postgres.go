package secretmenu

import (
	"context"
	"database/sql"
	"github.com/bugfixes/go-bugfixes/logs"
	ConfigBuilder "github.com/keloran/go-config"
	"strings"
)

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

type MenuStyle struct {
	Id          sql.NullString `json:"id"`
	CloseButton sql.NullString `json:"close_button"`
	Container   sql.NullString `json:"container"`
	Button      sql.NullString `json:"button"`
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
        agent_secret_menu.enabled,
        code,
        closebutton,
        container,
        button,
        style_id
    FROM public.agent_secret_menu
        LEFT JOIN public.secret_menu_style ON secret_menu_style.secret_menu_id = agent_secret_menu.id
        JOIN public.agent_environment ON agent_environment.id = agent_secret_menu.environment_id
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
		return secretMenu, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	if sequence.Valid {
		secretMenu.Sequence = strings.Split(sequence.String, ",")
	}
	secretMenu.CustomStyle = menuStyle

	return secretMenu, nil
}
