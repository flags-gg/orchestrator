package environment

import (
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *System) CreateEnvironmentInDB(name, agentId, userSubject string) (*Environment, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	envId := uuid.New().String()
	var insertedEnvId string

	if err := client.QueryRow(s.Context, `
      INSERT INTO public.agent_environment (
          agent_id,
          env_id,
          name
      ) VALUES ((
        SELECT agent.id
        FROM public.agent
        WHERE agent.agent_id = $1
      ), $2, $3)
      RETURNING agent_environment.id`, agentId, envId, name).Scan(&insertedEnvId); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to insert environment into database: %v", err)
	}

	return &Environment{
		Id:            insertedEnvId,
		EnvironmentId: envId,
		Name:          name,
	}, nil
}

func (s *System) GetEnvironmentFromDB(envId string) (*Environment, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	environment := &Environment{}
	if err := client.QueryRow(s.Context, `
    SELECT
      env.id,
      env.name,
      env.env_id,
      env.enabled
    FROM public.agent_environment AS env
    WHERE env.env_id = $1`, envId).Scan(&environment.Id, &environment.Name, &environment.EnvironmentId, &environment.Enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return environment, nil
}

func (s *System) GetAgentEnvironmentsFromDB(agentId string) ([]*Environment, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(s.Context, `
    SELECT
      env.id,
      env.name,
      env.env_id,
      env.enabled
    FROM agent_environment AS env
      JOIN agent ON env.agent_id = agent.id
    WHERE agent.agent_id = $1`, agentId)
	if err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var environments []*Environment
	for rows.Next() {
		environment := &Environment{}
		if err := rows.Scan(&environment.Id, &environment.Name, &environment.EnvironmentId, &environment.Enabled); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		environments = append(environments, environment)
	}

	return environments, nil
}
