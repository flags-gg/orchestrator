package flags

import (
	"errors"
	"github.com/jackc/pgx/v5"
)

type flagCreate struct {
	Name          string `json:"name"`
	EnvironmentId string `json:"environmentId"`
	AgentId       string `json:"agentId"`
}

func (s *System) GetClientFlagsFromDB(projectId, agentId, environmentId string) ([]Flag, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("failed to close database connection: %v", err)
		}
	}()

	flags := []Flag{}
	rows, err := client.Query(s.Context, `
    SELECT
	      flags.id,
        flags.name,
        flags.enabled
    FROM public.agent
        LEFT JOIN public.agent_flag AS flags ON agent.id = flags.agent_id
        LEFT JOIN public.agent_environment AS env ON env.id = flags.environment_id
        LEFT JOIN public.project ON project.id = agent.project_id
    WHERE env.env_id = $3
        AND agent.agent_id = $2
        AND project.project_id = $1`, projectId, agentId, environmentId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return flags, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("failed to get flags: %v", err)
	}
	for rows.Next() {
		flag := Flag{}
		details := Details{}
		err := rows.Scan(&details.ID, &details.Name, &flag.Enabled)
		if err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("failed to scan row: %v", err)
		}
		flag.Details = details
		flags = append(flags, flag)
	}

	return flags, nil
}

func (s *System) UpdateFlagInDB(flag Flag) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(s.Context, `
    UPDATE public.agent_flag
    SET
      enabled = $1,
      name=$3
    WHERE id = $2`, flag.Enabled, flag.Details.ID, flag.Details.Name)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to update flag: %v", err)
	}

	return nil
}

func (s *System) DeleteFlagFromDB(flag Flag) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(s.Context, `DELETE FROM public.agent_flag WHERE id = $1`, flag.Details.ID)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to delete flag: %v", err)
	}

	return nil
}

func (s *System) CreateFlagInDB(flag flagCreate) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(s.Context, `
        INSERT INTO public.agent_flag (
          name,
          agent_id,
          environment_id
        ) VALUES (
          $1,
          (SELECT id FROM public.agent WHERE agent_id = $2),
          (SELECT id FROM public.agent_environment WHERE env_id = $3))`, flag.Name, flag.AgentId, flag.EnvironmentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to delete flag: %v", err)
	}

	return nil
}
