package flags

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/flags-gg/orchestrator/internal/stats"
	"github.com/jackc/pgx/v5"
	"math/rand"
	"strings"
)

func (s *System) GetAgentFlagsFromDB(projectId, agentId, environmentId string) (*AgentResponse, error) {
	res := &AgentResponse{
		IntervalAllowed: 60,
	}

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		stats.NewSystem(s.Config).AddAgentError(projectId, agentId, environmentId)
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			stats.NewSystem(s.Config).AddAgentError(projectId, agentId, environmentId)
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if environmentId == "" {
		envId, err := s.GetDefaultEnvironment(projectId, agentId)
		if err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get default environment: %v", err)
		}
		environmentId = envId
	}

	var flags []Flag
	var menuEnabled sql.NullBool
	var menuCode sql.NullString
	var menuCloseButton sql.NullString
	var menuContainer sql.NullString
	var menuResetButton sql.NullString
	var menuFlag sql.NullString
	var menuButtonEnabled sql.NullString
	var menuButtonDisabled sql.NullString
	var menuHeader sql.NullString
	var intervalAllowed int

	rows, err := client.Query(s.Context, `
    SELECT
      flags.name AS FlagName,
      flags.enabled AS FlagEnabled,
      secretMenu.enabled AS MenuEnabled,
      secretMenu.code AS MenuCode,
      menuStyle.close_button AS MenuCloseButton,
      menuStyle.container AS MenuContainer,
      menuStyle.reset_button AS ResetButton,
      menuStyle.flag AS Flag,
      menuStyle.button_enabled AS ButtonEnabled,
      menuStyle.button_disabled AS ButtonDisabled,
      menuStyle.header AS Header,
      agent.interval
    FROM public.agent
      LEFT JOIN public.environment_flag AS flags ON agent.id = flags.agent_id
      LEFT JOIN public.agent_environment AS env ON env.id = flags.environment_id
      LEFT JOIN public.project ON project.id = agent.project_id
      LEFT JOIN public.environment_secret_menu AS secretMenu ON secretMenu.agent_id = agent.id
		AND secretMenu.environment_id = env.id
      LEFT JOIN public.secret_menu_style AS menuStyle ON menuStyle.secret_menu_id = secretMenu.id
    WHERE env.env_id = $1
      AND agent.agent_id = $2
      AND project.project_id = $3
      AND agent.enabled = true
      AND agent.enabled = true
      AND project.enabled = true`, environmentId, agentId, projectId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		stats.NewSystem(s.Config).AddAgentError(projectId, agentId, environmentId)
		if err.Error() == "context canceled" {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	for rows.Next() {
		var flagName string
		var flagEnabled bool

		if err = rows.Scan(
			&flagName,
			&flagEnabled,
			&menuEnabled,
			&menuCode,
			&menuCloseButton,
			&menuContainer,
			&menuResetButton,
			&menuFlag,
			&menuButtonEnabled,
			&menuButtonDisabled,
			&menuHeader,
			&intervalAllowed,
		); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan row: %v", err)
		}

		flag := Flag{
			Enabled: flagEnabled,
			Details: Details{
				Name: flagName,
				ID:   fmt.Sprintf("%d", rand.Intn(1000)),
			},
		}
		flags = append(flags, flag)
	}
	res.Flags = flags
	res.IntervalAllowed = intervalAllowed

	if menuEnabled.Bool {
		sm := &SecretMenu{
			Sequence: strings.Split(menuCode.String, ","),
		}

		if menuCloseButton.Valid {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "closeButton",
				Value: menuCloseButton.String,
			})
		}
		if menuContainer.Valid {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "container",
				Value: menuContainer.String,
			})
		}
		if menuResetButton.Valid {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "resetButton",
				Value: menuResetButton.String,
			})
		}
		if menuFlag.Valid {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "flag",
				Value: menuFlag.String,
			})
		}
		if menuButtonEnabled.Valid {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "buttonEnabled",
				Value: menuButtonEnabled.String,
			})
		}
		if menuButtonDisabled.Valid {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "buttonDisabled",
				Value: menuButtonDisabled.String,
			})
		}
		if menuHeader.Valid {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "header",
				Value: menuHeader.String,
			})
		}

		res.SecretMenu = *sm

	}

	stats.NewSystem(s.Config).AddAgentSuccess(projectId, agentId, environmentId)
	return res, nil
}

func (s *System) GetDefaultEnvironment(projectId, agentId string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var envId string
	err = client.QueryRow(s.Context, `
    SELECT env.env_id
    FROM public.agent_environment AS env
      JOIN public.agent ON env.agent_id = agent.id
      JOIN public.project ON agent.project_id = project.id
    WHERE agent.agent_id = $1
      AND project.project_id = $2
      AND env.default = true
    LIMIT 1`, agentId, projectId).Scan(&envId)
	if err != nil {
		if err.Error() == "context canceled" {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return envId, nil
}
