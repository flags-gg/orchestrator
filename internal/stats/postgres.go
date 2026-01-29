package stats

import (
	"context"
	"errors"
	"strings"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/jackc/pgx/v5"
)

func (s *System) GetNamesForData(ctx context.Context, data *AgentStat) (*AgentStat, error) {
	agentName, err := s.GetAgentName(ctx, data.ID)
	if err != nil {
		return data, err
	}
	data.Name = agentName

	for j, env := range data.Environments {
		environmentName, err := s.GetEnvironmentName(ctx, env.Id)
		if err != nil {
			return data, err
		}

		data.Environments[j].Name = environmentName
	}

	return data, nil
}

func (s *System) GetAgentName(ctx context.Context, agentId string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var agentName string
	if err := client.QueryRow(ctx, `
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

func (s *System) GetEnvironmentName(ctx context.Context, environmentId string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var envName string
	if err := client.QueryRow(ctx, `
    SELECT env.name AS EnvName
    FROM public.environment AS env
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
