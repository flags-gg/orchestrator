package flags

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type flagCreate struct {
	Name          string `json:"name"`
	EnvironmentId string `json:"environmentId"`
	AgentId       string `json:"agentId"`
}

func (s *System) GetClientFlagsFromDB(ctx context.Context, environmentId string) ([]Flag, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}()

	var flags []Flag
	rows, err := client.Query(ctx, `
    SELECT
        flags.id,
        flags.name,
        flags.enabled,
        COALESCE(flags.updated_at::text, ''),
        COALESCE(
          EXISTS (
            SELECT 1
            FROM public.environment_chain ec
            JOIN public.flag f2 ON f2.agent_id = flags.agent_id
                                AND f2.name = flags.name
                                AND f2.environment_id = ec.child_environment_id
            WHERE ec.agent_id = flags.agent_id
              AND ec.parent_environment_id = flags.environment_id
          ), false
        ) AS promoted
    FROM public.agent
        LEFT JOIN public.flag AS flags ON agent.id = flags.agent_id
        LEFT JOIN public.environment AS env ON env.id = flags.environment_id
        LEFT JOIN public.project ON project.id = agent.project_id
    WHERE env.env_id = $1`, environmentId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return flags, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("failed to get flags: %v", err)
	}
	defer rows.Close()
	if rows.Err() != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("failed to get flags: %v", err)
	}
	for rows.Next() {
		flag := Flag{}
		details := Details{}
		err := rows.Scan(&details.ID, &details.Name, &flag.Enabled, &details.LastChanged, &details.Promoted)
		if err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("failed to scan row: %v", err)
		}
		flag.Details = details
		flags = append(flags, flag)
	}

	return flags, nil
}

func (s *System) UpdateFlagInDB(ctx context.Context, flag Flag) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(ctx, `
    UPDATE public.flag
    SET
      enabled = $1,
      name = $3,
      updated_at = CASE
        WHEN enabled != $1 THEN now()
        ELSE updated_at
      END
    WHERE id = $2`, flag.Enabled, flag.Details.ID, flag.Details.Name)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to update flag: %v", err)
	}

	return nil
}

func (s *System) EditFlagInDB(ctx context.Context, cr FlagNameChangeRequest) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(ctx, `
    UPDATE public.flag
    SET
      name=$2
    WHERE id = $1`, cr.ID, cr.Name)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to update flag: %v", err)
	}

	return nil
}

func (s *System) DeleteFlagFromDB(ctx context.Context, flag Flag) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(ctx, `DELETE FROM public.flag WHERE id = $1`, flag.Details.ID)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to delete flag: %v", err)
	}

	return nil
}

func (s *System) DeleteAllFlagsForEnv(ctx context.Context, envId string) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}()

	var envIdInt int
	err = client.QueryRow(ctx, `SELECT id FROM public.environment WHERE env_id = $1`, envId).Scan(&envIdInt)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to get environment id: %v", err)
	}

	_, err = client.Exec(ctx, `DELETE FROM public.flag WHERE environment_id = $1`, envIdInt)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to delete flags: %v", err)
	}
	return nil
}

func (s *System) PromoteFlagInDB(ctx context.Context, flagId string) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}()

	// 1) Get source flag info
	var (
		flagName            string
		enabled             bool
		agentIdInt          int
		sourceEnvironmentId int
	)
	err = client.QueryRow(ctx, `
		SELECT f.name, f.enabled, f.agent_id, f.environment_id
		FROM public.flag f
		WHERE f.id = $1`, flagId).Scan(&flagName, &enabled, &agentIdInt, &sourceEnvironmentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to load flag for promotion: %v", err)
	}

	// 2) Find the next child environment in the chain
	var childEnvId int
	err = client.QueryRow(ctx, `
		SELECT ec.child_environment_id
		FROM public.environment_chain ec
		WHERE ec.agent_id = $1 AND ec.parent_environment_id = $2`, agentIdInt, sourceEnvironmentId).Scan(&childEnvId)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.Config.Bugfixes.Logger.Errorf("no child environment to promote to")
		}
		return s.Config.Bugfixes.Logger.Errorf("failed to find child environment: %v", err)
	}

	// 3) Create a NEW flag in the child environment (do not rely on name uniqueness)
	_, err = client.Exec(ctx, `
		INSERT INTO public.flag (name, agent_id, environment_id, enabled)
		VALUES ($1, $2, $3, $4)`,
		flagName, agentIdInt, childEnvId, enabled,
	)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to insert promoted flag: %v", err)
	}

	return nil
}

func (s *System) CreateFlagInDB(ctx context.Context, flag flagCreate) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(ctx, `
        INSERT INTO public.flag (
          name,
          agent_id,
          environment_id
        ) VALUES (
          $1,
          (SELECT id FROM public.agent WHERE agent_id = $2),
          (SELECT id FROM public.environment WHERE env_id = $3))`, flag.Name, flag.AgentId, flag.EnvironmentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("failed to create flag: %v", err)
	}

	return nil
}
