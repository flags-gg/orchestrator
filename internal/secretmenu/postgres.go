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

type ResetButton struct {
	Position string `json:"position"`
	Top      string `json:"top"`
	Left     string `json:"left"`
	Color    string `json:"color"`
}
type CloseButton struct {
	Position string `json:"position"`
	Top      string `json:"top"`
	Right    string `json:"right"`
	Color    string `json:"color"`
}
type ButtonEnabled struct {
	Background   string `json:"background"`
	Padding      string `json:"padding"`
	BorderRadius string `json:"borderRadius"`
	Color        string `json:"color"`
}
type ButtonDisabled struct {
	Background   string `json:"background"`
	Padding      string `json:"padding"`
	BorderRadius string `json:"borderRadius"`
	Color        string `json:"color"`
}
type Header struct {
	FontWeight  int    `json:"fontWeight"`
	Color       string `json:"color"`
	Top         string `json:"top"`
	Position    string `json:"position"`
	MarginRight string `json:"marginRight"`
	MarginLeft  string `json:"marginLeft"`
	Width       string `json:"width"`
}
type Container struct {
	Position        string `json:"position"`
	BackgroundColor string `json:"backgroundColor"`
	Color           string `json:"color"`
	BorderRadius    string `json:"borderRadius"`
	BorderStyle     string `json:"borderStyle"`
	BorderColor     string `json:"borderColor"`
	BorderWidth     string `json:"borderWidth"`
	Padding         string `json:"padding"`
}

type Flag struct {
	Display         string `json:"display"`
	JustifyContent  string `json:"justifyContent"`
	AlignItems      string `json:"alignItems"`
	Padding         string `json:"padding"`
	BackgroundColor string `json:"backgroundColor"`
	Margin          string `json:"margin"`
	Color           string `json:"color"`
	MinWidth        string `json:"minWidth"`
}

type MenuStyle struct {
	Id    string `json:"style_id,omitempty"`
	SQLId sql.NullString

	CloseButton    CloseButton `json:"close_button,omitempty"`
	SQLCloseButton sql.NullString

	Container    Container `json:"container,omitempty"`
	SQLContainer sql.NullString

	ResetButton    ResetButton `json:"reset_button,omitempty"`
	SQLResetButton sql.NullString

	Flag    Flag `json:"flag,omitempty"`
	SQLFlag sql.NullString

	ButtonEnabled    ButtonEnabled `json:"button_enabled,omitempty"`
	SQLButtonEnabled sql.NullString

	ButtonDisabled    ButtonDisabled `json:"button_disabled,omitempty"`
	SQLButtonDisabled sql.NullString

	Header    Header `json:"header,omitempty"`
	SQLHeader sql.NullString
}

type SecretMenu struct {
	Id          string             `json:"menu_id"`
	Enabled     bool               `json:"enabled"`
	EnvDetails  EnvironmentDetails `json:"environment_details,omitempty"`
	Sequence    []string           `json:"sequence,omitempty"`
	CustomStyle MenuStyle          `json:"custom_style,omitempty"`
}

type EnvironmentDetails struct {
	EnvironmentID string `json:"environment_id"`
	Name          string `json:"name"`
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
		if strings.Contains(err.Error(), "operation was canceled") {
			return secretMenu, nil
		}
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
        close_button,
        container,
        reset_button,
        flag,
        button_enabled,
        button_disabled,
        header,
        style_id
    FROM public.environment_secret_menu
        LEFT JOIN public.secret_menu_style ON secret_menu_style.secret_menu_id = environment_secret_menu.id
        JOIN public.agent_environment ON agent_environment.id = environment_secret_menu.environment_id
    WHERE agent_environment.env_id = $1`, environmentId).Scan(
		&secretMenu.Id,
		&secretMenu.Enabled,
		&sequence,
		&menuStyle.SQLCloseButton,
		&menuStyle.SQLContainer,
		&menuStyle.SQLResetButton,
		&menuStyle.SQLFlag,
		&menuStyle.SQLButtonEnabled,
		&menuStyle.SQLButtonDisabled,
		&menuStyle.SQLHeader,
		&menuStyle.SQLId); err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) || errors.Is(err, pgx.ErrNoRows) {
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

func (s *System) GetSecretMenuFromDB(menuId string) (SecretMenu, error) {
	var secretMenu SecretMenu
	var menuStyle MenuStyle
	var envDetails EnvironmentDetails

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return secretMenu, nil
		}
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
        secret_menu.enabled,
        code,
        close_button,
        container,
        reset_button,
        flag,
        button_enabled,
        button_disabled,
        header,
        style_id,
        agent_environment.env_id,
        agent_environment.name
    FROM public.environment_secret_menu AS secret_menu
        LEFT JOIN public.secret_menu_style ON secret_menu_style.secret_menu_id = secret_menu.id
        LEFT JOIN public.agent_environment ON agent_environment.id = secret_menu.environment_id
    WHERE secret_menu.menu_id = $1`, menuId).Scan(
		&secretMenu.Id,
		&secretMenu.Enabled,
		&sequence,
		&menuStyle.SQLCloseButton,
		&menuStyle.SQLContainer,
		&menuStyle.SQLResetButton,
		&menuStyle.SQLFlag,
		&menuStyle.SQLButtonEnabled,
		&menuStyle.SQLButtonDisabled,
		&menuStyle.SQLHeader,
		&menuStyle.SQLId,
		&envDetails.EnvironmentID,
		&envDetails.Name); err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
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
	secretMenu.EnvDetails = envDetails

	return secretMenu, nil
}

func (s *System) UpdateSecretMenuSequenceInDB(menuId string, secretMenu SecretMenu) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
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
    SET code = $1
    WHERE menu_id = $2`, sequence, menuId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update database: %v", err)
	}

	return nil
}

func (s *System) UpdateSecretMenuStateInDB(menuId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `
    UPDATE public.environment_secret_menu
    SET enabled = NOT enabled
    WHERE menu_id = $1`, menuId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update database: %v", err)
	}

	return nil
}

func (s *System) UpdateSecretMenuStyleInDB(menuId string, secretMenu SecretMenu) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if secretMenu.CustomStyle.Id == "" {
		uu, err := uuid.NewRandom()
		if err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to generate UUID: %v", err)
		}
		styleId := uu.String()

		var secretMenuId int
		if err := client.QueryRow(s.Context, `SELECT id FROM public.environment_secret_menu WHERE menu_id = $1`, menuId).Scan(&secretMenuId); err != nil {
			if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
				return nil
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}

			return s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
		}

		if _, err := client.Exec(s.Context, `
      INSERT INTO public.secret_menu_style (secret_menu_id, close_button, container, reset_button, flag, button_enabled, button_disabled, header, style_id)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			secretMenuId,
			secretMenu.CustomStyle.SQLCloseButton,
			secretMenu.CustomStyle.SQLContainer,
			secretMenu.CustomStyle.SQLResetButton,
			secretMenu.CustomStyle.SQLFlag,
			secretMenu.CustomStyle.SQLButtonEnabled,
			secretMenu.CustomStyle.SQLButtonDisabled,
			secretMenu.CustomStyle.SQLHeader, styleId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to insert into database: %v", err)
		}
		return nil
	}

	if _, err := client.Exec(s.Context, `
    	UPDATE public.secret_menu_style
    	SET close_button = $1, container = $2, reset_button = $3, flag = $4, button_enabled = $5, button_disabled = $6, header = $7
    	WHERE style_id = $8`,
		secretMenu.CustomStyle.SQLCloseButton,
		secretMenu.CustomStyle.SQLContainer,
		secretMenu.CustomStyle.SQLResetButton,
		secretMenu.CustomStyle.SQLFlag,
		secretMenu.CustomStyle.SQLButtonEnabled,
		secretMenu.CustomStyle.SQLButtonDisabled,
		secretMenu.CustomStyle.SQLHeader,
		secretMenu.CustomStyle.Id); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update database: %v", err)
	}

	return nil
}

func (s *System) CreateSecretMenuInDB(environmentId string, secretMenu SecretMenu) (string, string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return "", "", nil
		}
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

	var envId int
	var agentId int
	if err := client.QueryRow(s.Context, `SELECT agent_id, id FROM public.agent_environment WHERE env_id = $1`, environmentId).Scan(&agentId, &envId); err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return "", "", nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}

		return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	if _, err := client.Exec(s.Context, `INSERT INTO public.environment_secret_menu (menu_id, environment_id, enabled, code, agent_id) VALUES ($1, $2, $3, $4, $5)`, menuId.String(), envId, secretMenu.Enabled, sequence, agentId); err != nil {
		return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to insert into database: %v", err)
	}

	if secretMenu.CustomStyle.Id == "" {
		uu, err := uuid.NewRandom()
		if err != nil {
			return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to generate UUID: %v", err)
		}
		styleId := uu.String()

		var secretMenuId int
		if err := client.QueryRow(s.Context, `SELECT id FROM public.environment_secret_menu WHERE menu_id = $1`, menuId.String()).Scan(&secretMenuId); err != nil {
			if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
				return "", "", nil
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return "", "", nil
			}

			return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
		}

		if _, err := client.Exec(s.Context, `
      INSERT INTO public.secret_menu_style (secret_menu_id, close_button, container, reset_button, flag, button_enabled, button_disabled, header, style_id)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			secretMenuId,
			secretMenu.CustomStyle.SQLCloseButton,
			secretMenu.CustomStyle.SQLContainer,
			secretMenu.CustomStyle.SQLResetButton,
			secretMenu.CustomStyle.SQLFlag,
			secretMenu.CustomStyle.SQLButtonEnabled,
			secretMenu.CustomStyle.SQLButtonDisabled,
			secretMenu.CustomStyle.SQLHeader, styleId); err != nil {
			return "", "", s.Config.Bugfixes.Logger.Errorf("Failed to insert into database: %v", err)
		}
		return menuId.String(), uu.String(), nil
	}

	return menuId.String(), "", nil
}

func (s *System) DeleteSecretMenuForEnv(envId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `
    DELETE FROM public.environment_secret_menu
           WHERE environment_id = (
             SELECT id
              FROM public.agent_environment
              WHERE env_id = $1
              LIMIT 1
           )`, envId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete from database: %v", err)
	}

	return nil
}

func (s *System) GetSecretMenuStyleFromDB(menuId string) (StyleMenu, error) {
	var menuStyle MenuStyle
	var styleMenu StyleMenu
	var styleId sql.NullString

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return styleMenu, nil
		}
		return styleMenu, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if err := client.QueryRow(s.Context, `
    SELECT
        style_id,
        close_button,
        container,
        reset_button,
        flag,
        button_enabled,
        button_disabled,
        header,
        style_id
    FROM public.environment_secret_menu
        LEFT JOIN public.secret_menu_style ON secret_menu_style.secret_menu_id = environment_secret_menu.id
        JOIN public.agent_environment ON agent_environment.id = environment_secret_menu.environment_id
    WHERE environment_secret_menu.menu_id = $1`, menuId).Scan(
		&styleId,
		&menuStyle.SQLCloseButton,
		&menuStyle.SQLContainer,
		&menuStyle.SQLResetButton,
		&menuStyle.SQLFlag,
		&menuStyle.SQLButtonEnabled,
		&menuStyle.SQLButtonDisabled,
		&menuStyle.SQLHeader,
		&menuStyle.SQLId); err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return styleMenu, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return styleMenu, nil
		}
		return styleMenu, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	if styleId.Valid {
		styleMenu.Id = styleId.String
	}
	if menuStyle.SQLCloseButton.Valid {
		style := Style{}
		style.Name = "closeButton"
		style.Value = menuStyle.SQLCloseButton.String
		styleMenu.Styles = append(styleMenu.Styles, style)
	}
	if menuStyle.SQLContainer.Valid {
		style := Style{}
		style.Name = "container"
		style.Value = menuStyle.SQLContainer.String
		styleMenu.Styles = append(styleMenu.Styles, style)
	}
	if menuStyle.SQLResetButton.Valid {
		style := Style{}
		style.Name = "resetButton"
		style.Value = menuStyle.SQLResetButton.String
		styleMenu.Styles = append(styleMenu.Styles, style)
	}
	if menuStyle.SQLFlag.Valid {
		style := Style{}
		style.Name = "flag"
		style.Value = menuStyle.SQLFlag.String
		styleMenu.Styles = append(styleMenu.Styles, style)
	}
	if menuStyle.SQLButtonEnabled.Valid {
		style := Style{}
		style.Name = "buttonEnabled"
		style.Value = menuStyle.SQLButtonEnabled.String
		styleMenu.Styles = append(styleMenu.Styles, style)
	}
	if menuStyle.SQLButtonDisabled.Valid {
		style := Style{}
		style.Name = "buttonDisabled"
		style.Value = menuStyle.SQLButtonDisabled.String
		styleMenu.Styles = append(styleMenu.Styles, style)
	}
	if menuStyle.SQLHeader.Valid {
		style := Style{}
		style.Name = "header"
		style.Value = menuStyle.SQLHeader.String
		styleMenu.Styles = append(styleMenu.Styles, style)
	}
	if menuStyle.SQLId.Valid {
		styleMenu.Id = menuStyle.SQLId.String
	}

	return styleMenu, nil
}
