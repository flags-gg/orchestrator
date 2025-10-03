package flags

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *OFREPSystem) GetSingleFlagFromDB(projectId, agentId, environmentId, flagKey string) (*Flag, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if client != nil {
			if err := client.Close(s.Context); err != nil {
				s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
			}
		}
	}()

	if environmentId == "" {
		flagSystem := NewSystem(s.Config).SetContext(s.Context)
		envId, err := flagSystem.GetDefaultEnvironment(projectId, agentId)
		if err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get default environment: %v", err)
		}
		environmentId = envId
	}

	var flagName string
	var flagEnabled bool
	var flagId string

	err = client.QueryRow(s.Context, `
    SELECT
      flags.id AS FlagId,
      flags.name AS FlagName,
      flags.enabled AS FlagEnabled
    FROM public.agent
      LEFT JOIN public.flag AS flags ON agent.id = flags.agent_id
      LEFT JOIN public.environment AS env ON env.id = flags.environment_id
      LEFT JOIN public.project ON project.id = agent.project_id
    WHERE env.env_id = $1
      AND agent.agent_id = $2
      AND project.project_id = $3
      AND LOWER(flags.name) = LOWER($4)
      AND agent.enabled = true
      AND project.enabled = true
    LIMIT 1`, environmentId, agentId, projectId, flagKey).Scan(&flagId, &flagName, &flagEnabled)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	flag := &Flag{
		Enabled: flagEnabled,
		Details: Details{
			Name: flagName,
			ID:   flagId,
		},
	}

	return flag, nil
}
