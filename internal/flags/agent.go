package flags

import (
	"errors"
	"fmt"
	"github.com/flags-gg/orchestrator/internal/stats"
	"github.com/jackc/pgx/v5"
	"math/rand"
	"strings"
)

func (s *System) GetAgentFlags(companyId, agentId, environmentId string) (*AgentResponse, error) {
	res := &AgentResponse{
		IntervalAllowed: 60,
	}

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		stats.NewSystem(s.Config).AddAgentError(companyId, agentId, environmentId)
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			stats.NewSystem(s.Config).AddAgentError(companyId, agentId, environmentId)
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var flags []Flag
	var menuEnabled bool
	var menuCode string
	var menuCloseButton string
	var menuContainer string
	var menuButton string

	rows, err := client.Query(s.Context, "SELECT flags.name AS FlagName, flags.enabled AS FlagEnabled, secretMenu.enabled AS MenuEnabled, secretMenu.code AS MenuCode, menuStyle.closebutton AS MenuCloseButton, menuStyle.container AS MenuContainer, menuStyle.button AS MenuButton FROM public.agent AS agent LEFT JOIN public.agent_flag AS flags ON agent.id = flags.agent_id LEFT JOIN public.agent_environment AS env ON env.id = flags.environment_id LEFT JOIN public.company AS company ON company.id = agent.company_id LEFT JOIN public.agent_secret_menu AS secretMenu ON secretMenu.agent_id = agent.id LEFT JOIN public.secret_menu_style AS menuStyle ON menuStyle.secret_menu_id = secretMenu.id WHERE env.env_id = $1 AND agent.agent_id = $2 AND company.company_id = $3", environmentId, agentId, companyId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		stats.NewSystem(s.Config).AddAgentError(companyId, agentId, environmentId)
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
			&menuButton,
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

	if menuEnabled {
		sm := &SecretMenu{
			Sequence: strings.Split(menuCode, ","),
		}

		if menuCloseButton != "" {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "closeButton",
				Value: menuCloseButton,
			})
		}

		if menuContainer != "" {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "container",
				Value: menuContainer,
			})
		}

		if menuButton != "" {
			sm.Styles = append(sm.Styles, SecretMenuStyle{
				Name:  "button",
				Value: menuButton,
			})
		}
		res.SecretMenu = *sm
	}

	stats.NewSystem(s.Config).AddAgentSuccess(companyId, agentId, environmentId)
	return res, nil
}
