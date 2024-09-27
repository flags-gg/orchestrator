package environment

import (
	"errors"
	"github.com/flags-gg/orchestrator/internal/flags"
	"github.com/flags-gg/orchestrator/internal/secretmenu"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *System) CreateEnvironmentInDB(name, agentId string) (*Environment, error) {
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
      env.enabled,
      agent.name as AgentName,
      project.name as ProjectName
    FROM public.agent_environment AS env
    	LEFT JOIN public.agent ON agent.id = env.agent_id
    	LEFT JOIN public.project ON project.id = agent.project_id
    WHERE env.env_id = $1`, envId).Scan(&environment.Id, &environment.Name, &environment.EnvironmentId, &environment.Enabled, &environment.AgentName, &environment.ProjectName); err != nil {
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

func (s *System) UpdateEnvironmentInDB(env Environment) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(s.Context, `
    UPDATE public.agent_environment
    SET name = $1, enabled = $3
    WHERE env_id = $2`, env.Name, env.EnvironmentId, env.Enabled)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update environment in database: %v", err)
	}

	return nil
}

func (s *System) DeleteEnvironmentFromDB(envId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if err := flags.NewSystem(s.Config).SetContext(s.Context).DeleteAllFlagsForEnv(envId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete flags: %v", err)
	}
	if err := secretmenu.NewSystem(s.Config).SetContext(s.Context).DeleteSecretMenuForEnv(envId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete secret menus: %v", err)
	}

	_, err = client.Exec(s.Context, `
    DELETE FROM public.agent_environment
    WHERE env_id = $1`, envId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete environment from database: %v", err)
	}

	return nil
}

func (s *System) DeleteAllEnvironmentsForAgent(agentId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var environmentIds []string
	rows, err := client.Query(s.Context, `
    SELECT env_id
    FROM public.agent_environment
    WHERE agent_id = (
        SELECT id
        FROM public.agent
        WHERE agent_id = $1
    )`, agentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to get environments from database: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var envId string
		if err := rows.Scan(&envId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}
		environmentIds = append(environmentIds, envId)
	}

	for _, envId := range environmentIds {
		if err := flags.NewSystem(s.Config).SetContext(s.Context).DeleteAllFlagsForEnv(envId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to delete flags: %v", err)
		}

		if err := s.DeleteEnvironmentFromDB(envId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to delete environment from database: %v", err)
		}
	}

	return nil
}
