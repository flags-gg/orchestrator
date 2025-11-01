package environment

import (
	"context"
	"errors"
	"fmt"
	"github.com/flags-gg/orchestrator/internal/flags"
	"github.com/flags-gg/orchestrator/internal/secretmenu"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *System) CreateEnvironmentInDB(name, agentId string) (*Environment, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
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
      WITH agent_row AS (
        SELECT id FROM public.agent WHERE agent_id = $1
      ), next_level AS (
        SELECT COALESCE(MAX(level) + 1, 0) AS lvl FROM public.environment WHERE agent_id = (SELECT id FROM agent_row)
      )
      INSERT INTO public.environment (
          agent_id,
          env_id,
          name,
          level
      ) VALUES (
        (SELECT id FROM agent_row),
        $2,
        $3,
        (SELECT lvl FROM next_level)
      )
      RETURNING environment.id`, agentId, envId, name).Scan(&insertedEnvId); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to insert environment into database: %v", err)
	}

	return &Environment{
		Id:            insertedEnvId,
		EnvironmentId: envId,
		Name:          name,
	}, nil
}

func (s *System) GetEnvironmentFromDB(envId, companyId string) (*Environment, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
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
      env.level,
      -- can promote: has a child link
      EXISTS (
        SELECT 1 FROM public.environment_chain ec
        WHERE ec.parent_environment_id = env.id AND ec.agent_id = env.agent_id
      ) AS can_promote,
      agent.name as AgentName,
      project.name as ProjectName
    FROM public.environment AS env
    	LEFT JOIN public.agent ON agent.id = env.agent_id
    	LEFT JOIN public.project ON project.id = agent.project_id
        JOIN public.company ON company.id = project.company_id
    WHERE env.env_id = $1
      AND company.company_id = $2`, envId, companyId).Scan(&environment.Id, &environment.Name, &environment.EnvironmentId, &environment.Enabled, &environment.Level, &environment.CanPromote, &environment.AgentName, &environment.ProjectName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return environment, nil
}

func (s *System) GetAgentEnvironmentsFromDB(agentId, companyId string) ([]*Environment, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
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
      env.enabled,
      env.level,
      EXISTS (
        SELECT 1 FROM public.environment_chain ec
        WHERE ec.parent_environment_id = env.id AND ec.agent_id = env.agent_id
      ) AS can_promote
    FROM environment AS env
      JOIN agent ON env.agent_id = agent.id
      JOIN project ON project.id = agent.project_id
      JOIN company ON company.id = project.company_id
    WHERE agent.agent_id = $1
      AND company.company_id = $2
    ORDER BY env.level ASC`, agentId, companyId)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var environments []*Environment
	for rows.Next() {
		environment := &Environment{}
		if err := rows.Scan(&environment.Id, &environment.Name, &environment.EnvironmentId, &environment.Enabled, &environment.Level, &environment.CanPromote); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		environments = append(environments, environment)
	}

	return environments, nil
}

func (s *System) GetEnvironmentsFromDB(companyId string) ([]*Environment, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(s.Context, `
    SELECT
		env.env_id AS EnvId,
  		env.name AS EnvName,
  		agent.name AS AgentName,
  		project.name AS ProjectName
	FROM environment AS env
		JOIN agent ON agent.id = env.agent_id
  		JOIN project ON project.id = agent.project_id
  		JOIN company ON company.id = project.company_id
	WHERE company.company_id = $1`, companyId)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var environments []*Environment
	for rows.Next() {
		environment := &Environment{}
		if err := rows.Scan(&environment.EnvironmentId, &environment.Name, &environment.AgentName, &environment.ProjectName); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		environments = append(environments, environment)
	}

	return environments, nil
}

func (s *System) UpdateEnvironmentInDB(env Environment) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(s.Context, `
    UPDATE public.environment
    SET name = $1, enabled = $3
    WHERE env_id = $2`, env.Name, env.EnvironmentId, env.Enabled)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update environment in database: %v", err)
	}

	return nil
}

func (s *System) CloneEnvironmentInDB(envId, newEnvId, agentId, name string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	flagsToClone, err := flags.NewSystem(s.Config).SetContext(s.Context).GetClientFlagsFromDB(envId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to get flags: %v", err)
	}

	var agentIdInt int
	if err := client.QueryRow(s.Context, `
    SELECT id FROM public.agent WHERE agent_id = $1`, agentId).Scan(&agentIdInt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.Config.Bugfixes.Logger.Errorf("Failed to get agent id: %v", err)
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	var envIdInt int
	if err := client.QueryRow(s.Context, `
    WITH next_level AS (
      SELECT COALESCE(MAX(level) + 1, 0) AS lvl FROM public.environment WHERE agent_id = $1
    )
    INSERT INTO public.environment (agent_id, env_id, name, level)
        VALUES ($1, $2, $3, (SELECT lvl FROM next_level))
        RETURNING environment.id`, agentIdInt, newEnvId, name).Scan(&envIdInt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.Config.Bugfixes.Logger.Errorf("Failed to insert environment into database: %v", err)
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	insertVars := ""

	for _, flag := range flagsToClone {
		bv := "false"
		if flag.Enabled {
			bv = "true"
		}

		insertVars += fmt.Sprintf(`('%s', %d, %d, %s),`, flag.Details.Name, agentIdInt, envIdInt, bv)
	}
	if insertVars != "" {
		insertVars = insertVars[:len(insertVars)-1] // Remove last comma
		_, err := client.Exec(s.Context, fmt.Sprintf(`INSERT INTO public.flag (name, agent_id, environment_id, enabled) VALUES %s`, insertVars))
		if err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to insert flags into database: %v", err)
		}
		return nil
	}

	return nil
}

func (s *System) LinkChildEnvironmentInDB(parentEnvId, childEnvId, agentId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	// Resolve IDs
	var (
		agentIdInt     int
		parentEnvIdInt int
		childEnvIdInt  int
	)
	if err := client.QueryRow(s.Context, `SELECT id FROM public.agent WHERE agent_id = $1`, agentId).Scan(&agentIdInt); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to resolve agent: %v", err)
	}
	if err := client.QueryRow(s.Context, `SELECT id FROM public.environment WHERE env_id = $1`, parentEnvId).Scan(&parentEnvIdInt); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to resolve parent environment: %v", err)
	}
	if err := client.QueryRow(s.Context, `SELECT id FROM public.environment WHERE env_id = $1`, childEnvId).Scan(&childEnvIdInt); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to resolve child environment: %v", err)
	}

	_, err = client.Exec(s.Context, `
		INSERT INTO public.environment_chain (agent_id, parent_environment_id, child_environment_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (agent_id, parent_environment_id) DO UPDATE SET child_environment_id = EXCLUDED.child_environment_id`, agentIdInt, parentEnvIdInt, childEnvIdInt)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to link child environment: %v", err)
	}
	return nil
}

func (s *System) DeleteEnvironmentFromDB(envId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
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
    DELETE FROM public.environment
    WHERE env_id = $1`, envId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete environment from database: %v", err)
	}

	return nil
}

func (s *System) DeleteAllEnvironmentsForAgent(agentId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
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
    FROM public.environment
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
