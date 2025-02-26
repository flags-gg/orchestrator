package stats

import (
	"context"
	"errors"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *System) GetNamesForData(data *AgentStat) (*AgentStat, error) {
	agentName, err := s.GetAgentName(data.ID)
	if err != nil {
		return data, err
	}
	data.Name = agentName

	for j, env := range data.Environments {
		environmentName, err := s.GetEnvironmentName(env.Id)
		if err != nil {
			return data, err
		}

		data.Environments[j].Name = environmentName
	}

	return data, nil
}

func (s *System) GetAgentName(agentId string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var agentName string
	if err := client.QueryRow(s.Context, `
    SELECT agent.name AS AgentName
    FROM public.agent AS agent
    WHERE agent_id = $1`, agentId).Scan(&agentName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return "", nil
		}

		return "", logs.Errorf("Failed to query database: %v", err)
	}

	return agentName, nil
}

func (s *System) GetEnvironmentName(environmentId string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var envName string
	if err := client.QueryRow(s.Context, `
    SELECT env.name AS EnvName
    FROM public.agent_environment AS env
    WHERE env_id = $1`, environmentId).Scan(&envName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return "", nil
		}

		return "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return envName, nil
}
