package agent

import (
	"context"
	"errors"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/flags-gg/orchestrator/internal/environment"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProjectInfo struct {
	Id   string `json:"project_id"`
	Name string `json:"name"`
}

type Agent struct {
	Id               string                     `json:"id"`
	Name             string                     `json:"name"`
	RequestLimit     int                        `json:"request_limit"`
	AgentId          string                     `json:"agent_id"`
	Environments     []*environment.Environment `json:"environments"`
	EnvironmentLimit int                        `json:"environment_limit"`
	Enabled          bool                       `json:"enabled"`
	ProjectInfo      *ProjectInfo               `json:"project_info"`
}

func (s *System) CreateAgentForProject(name, projectId, userSubject string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	agentId := uuid.New().String()

	if _, err := client.Exec(s.Context, `
    INSERT INTO public.agent (
        project_id,
        agent_id,
        name,
        allowed_environments,
        allowed_access_limit
    ) VALUES ((
      SELECT project.id
      FROM public.project
      WHERE project.project_id = $1
    ), $2, $3, (
      SELECT allowed_environments_per_agent
      FROM public.company
          JOIN public.company_user ON company_user.company_id = company.id
          JOIN public.user AS u ON u.id = company_user.user_id
      WHERE u.subject = $4
    ), (
      SELECT allowed_access_per_environment
      FROM public.company
          JOIN public.company_user ON company_user.company_id = company.id
          JOIN public.user AS u ON u.id = company_user.user_id
      WHERE u.subject = $4
    ))`, projectId, agentId, name, userSubject); err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to insert agent into database: %v", err)
	}

	return agentId, nil
}

func (s *System) GetAgentDetails(agentId, companyId string) (*Agent, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	agent := &Agent{
		AgentId:     agentId,
		ProjectInfo: &ProjectInfo{},
	}
	if err := client.QueryRow(s.Context, `
    SELECT
      agent.id,
      agent.name AS AgentName,
      agent.allowed_access_limit,
      agent.allowed_environments,
      agent.enabled,
      project.name,
      project.project_id
    FROM public.agent AS agent
      JOIN public.project ON agent.project_id = project.id
      JOIN public.company ON company.id = project.company_id
    WHERE agent.agent_id = $1
      AND company.company_id = $2`, agentId, companyId).Scan(
		&agent.Id,
		&agent.Name,
		&agent.RequestLimit,
		&agent.EnvironmentLimit,
		&agent.Enabled,
		&agent.ProjectInfo.Name,
		&agent.ProjectInfo.Id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err.Error() == "context canceled" {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	agent.Environments, err = environment.NewSystem(s.Config).SetContext(s.Context).GetAgentEnvironmentsFromDB(agentId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get agent environments: %v", err)
	}

	return agent, nil
}

func (s *System) GetAgents(companyId string) ([]*Agent, error) {
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
      agent.id,
      agent.name AS AgentName,
      agent.allowed_access_limit,
      agent.agent_id,
      agent.allowed_environments
    FROM public.agent
      JOIN public.project ON agent.project_id = project.id
      JOIN public.company ON project.company_id = company.id
    WHERE company.company_id = $1`, companyId)
	if err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		agent := &Agent{}
		if err := rows.Scan(&agent.Id, &agent.Name, &agent.RequestLimit, &agent.AgentId, &agent.EnvironmentLimit); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		envs, err := environment.NewSystem(s.Config).SetContext(s.Context).GetAgentEnvironmentsFromDB(agent.AgentId)
		if err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get agent environments: %v", err)
		}
		agent.Environments = envs

		agents = append(agents, agent)
	}

	return agents, nil
}

func (s *System) GetAgentsForProject(companyId, projectId string) ([]*Agent, error) {
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
      agent.id,
      agent.name AS AgentName,
      agent.allowed_access_limit,
      agent.agent_id,
      agent.allowed_environments
    FROM public.agent
      JOIN public.project ON agent.project_id = project.id
      JOIN public.company ON project.company_id = company.id
    WHERE company.company_id = $1
        AND project.project_id = $2`, companyId, projectId)
	if err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		agent := &Agent{}
		if err := rows.Scan(&agent.Id, &agent.Name, &agent.RequestLimit, &agent.AgentId, &agent.EnvironmentLimit); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		envs, err := environment.NewSystem(s.Config).SetContext(s.Context).GetAgentEnvironmentsFromDB(agent.AgentId)
		if err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get agent environments: %v", err)
		}
		agent.Environments = envs

		agents = append(agents, agent)
	}

	return agents, nil
}

func (s *System) ValidateAgentWithEnvironment(ctx context.Context, agentId, projectId, environmentId string) (bool, error) {
	valid := false
	s.Context = ctx

	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return false, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if err := client.QueryRow(ctx, `
    SELECT TRUE
    FROM public.agent
      JOIN public.agent_environment AS pa ON pa.agent_id = agent.id
      JOIN public.project ON project.id = agent.project_id
    WHERE agent.agent_id = $1
      AND pa.env_id = $2
      AND project.project_id = $3`, agentId, environmentId, projectId).Scan(&valid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return valid, nil
		}
		if err.Error() == "context canceled" {
			return valid, nil
		}
		return valid, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return valid, nil
}

func (s *System) ValidateAgentWithoutEnvironment(ctx context.Context, agentId, projectId string) (bool, error) {
	valid := false
	s.Context = ctx

	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return false, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if err := client.QueryRow(ctx, `
    SELECT TRUE
    FROM public.agent
      JOIN project ON project.id = agent.project_id
    WHERE agent.agent_id = $1
      AND project.project_id = $2`, agentId, projectId).Scan(&valid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return valid, nil
		}
		if err.Error() == "context canceled" {
			return valid, nil
		}
		return valid, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return valid, nil
}

func (s *System) CreateAgentInDB(name, projectId, userSubject string) (*Agent, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	agentId := uuid.New().String()
	var insertedAgentId string

	if err := client.QueryRow(s.Context, `
      INSERT INTO public.agent (
          project_id,
          agent_id,
          name,
          allowed_environments,
          allowed_access_limit
      ) VALUES ((
        SELECT project.id
        FROM public.project
        WHERE project.project_id = $1
      ), $2, $3, (
        SELECT allowed_environments_per_agent
        FROM public.company
            JOIN public.company_user ON company_user.company_id = company.id
            JOIN public.user AS u ON u.id = company_user.user_id
        WHERE u.subject = $4
      ), (
        SELECT allowed_access_per_environment
        FROM public.company
            JOIN public.company_user ON company_user.company_id = company.id
            JOIN public.user AS u ON u.id = company_user.user_id
        WHERE u.subject = $4
      ))
      RETURNING agent.id`, projectId, agentId, name, userSubject).Scan(&insertedAgentId); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to insert agent into database: %v", err)
	}

	return &Agent{
		Id:      insertedAgentId,
		AgentId: agentId,
		Name:    "Default Agent",
	}, nil
}

func (s *System) UpdateAgentDetails(agent Agent) error {
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
    UPDATE public.agent
    SET name = $1, enabled = $2
    WHERE agent_id = $3`, agent.Name, agent.Enabled, agent.AgentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update agent details: %v", err)
	}

	return nil
}

func (s *System) DeleteAgentFromDB(agentId string) error {
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
    DELETE FROM public.agent
    WHERE agent_id = $1`, agentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete agent: %v", err)
	}

	return nil
}

func (s *System) DeleteAllAgentsForProject(projectId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var agents []*Agent
	rows, err := client.Query(s.Context, `
    SELECT agent_id
    FROM public.agent
    WHERE project_id = (
      SELECT id
      FROM public.project
      WHERE project_id = $1
    )`, projectId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		agent := &Agent{}
		if err := rows.Scan(&agent.AgentId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}
		agents = append(agents, agent)
	}

	for _, agent := range agents {
		if err := environment.NewSystem(s.Config).SetContext(s.Context).DeleteAllEnvironmentsForAgent(agent.AgentId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to delete agent environments: %v", err)
		}

		if err := s.DeleteAgentFromDB(agent.AgentId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to delete agent: %v", err)
		}
	}

	return nil
}
