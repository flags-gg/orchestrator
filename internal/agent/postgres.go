package agent

import (
	"context"
	"errors"
	"strings"

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

func (s *System) CreateAgentForProject(ctx context.Context, name, projectId string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = logs.Errorf("Failed to close database connection: %v", err)
		}
	}()

	agentId := uuid.New().String()

	if _, err := client.Exec(ctx, `
    INSERT INTO public.agent (
        project_id,
        agent_id,
        name
    ) VALUES ((
      SELECT project.id
      FROM public.project
      WHERE project.project_id = $1
    ), $2, $3)`, projectId, agentId, name); err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to insert agent into database: %v", err)
	}

	return agentId, nil
}

func (s *System) GetAgentDetails(ctx context.Context, agentId, companyId string) (*Agent, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = logs.Errorf("Failed to close database connection: %v", err)
		}
	}()

	agent := &Agent{
		AgentId:     agentId,
		ProjectInfo: &ProjectInfo{},
	}
	if err := client.QueryRow(ctx, `
    SELECT
      agent.id,
      agent.name AS AgentName,
      payment_plans.agents,
      payment_plans.environments,
      agent.enabled,
      project.name,
      project.project_id
    FROM public.agent AS agent
      JOIN public.project ON agent.project_id = project.id
      JOIN public.company ON company.id = project.company_id
      JOIN public.payment_plans ON payment_plans.id = company.payment_plan_id
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
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	agent.Environments, err = environment.NewSystem(s.Config).GetAgentEnvironmentsFromDB(ctx, agentId, companyId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get agent environments: %v", err)
	}

	return agent, nil
}

func (s *System) GetAgents(ctx context.Context, companyId string) ([]*Agent, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(ctx, `
    SELECT
      agent.id,
      agent.name AS AgentName,
      payment_plans.agents,
      agent.agent_id,
      payment_plans.environments,
      project.name AS ProjectName,
      project.project_id as ProjectId
    FROM public.agent
      JOIN public.project ON agent.project_id = project.id
      JOIN public.company ON project.company_id = company.id
      JOIN public.payment_plans ON payment_plans.id = company.payment_plan_id
    WHERE company.company_id = $1`, companyId)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		agent := &Agent{}
		projectInfo := &ProjectInfo{}
		if err := rows.Scan(&agent.Id, &agent.Name, &agent.RequestLimit, &agent.AgentId, &agent.EnvironmentLimit, &projectInfo.Name, &projectInfo.Id); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		//envs, err := environment.NewSystem(s.Config).SetContext(s.Context).GetAgentEnvironmentsFromDB(agent.AgentId, companyId)
		//if err != nil {
		//	return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get agent environments: %v", err)
		//}
		//agent.Environments = envs
		agent.ProjectInfo = projectInfo

		agents = append(agents, agent)
	}

	return agents, nil
}

func (s *System) GetAgentsForProject(ctx context.Context, companyId, projectId string) ([]*Agent, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(ctx, `
    SELECT
      agent.id,
      agent.name AS AgentName,
      payment_plans.requests,
      agent.agent_id,
      payment_plans.environments
    FROM public.agent
      JOIN public.project ON agent.project_id = project.id
      JOIN public.company ON project.company_id = company.id
      JOIN public.payment_plans ON payment_plans.id = company.payment_plan_id
    WHERE company.company_id = $1
        AND project.project_id = $2`, companyId, projectId)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
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

		envs, err := environment.NewSystem(s.Config).GetAgentEnvironmentsFromDB(ctx, agent.AgentId, companyId)
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

	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return false, nil
		}
		return false, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	if err := client.QueryRow(ctx, `
    SELECT TRUE
    FROM public.agent
      JOIN public.environment AS pa ON pa.agent_id = agent.id
      JOIN public.project ON project.id = agent.project_id
    WHERE agent.agent_id = $1
      AND pa.env_id = $2
      AND project.project_id = $3`, agentId, environmentId, projectId).Scan(&valid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return valid, nil
		}
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return valid, nil
		}
		return valid, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return valid, nil
}

func (s *System) ValidateAgentWithoutEnvironment(ctx context.Context, agentId, projectId string) (bool, error) {
	valid := false

	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return false, nil
		}
		return false, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
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
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return valid, nil
		}
		return valid, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return valid, nil
}

func (s *System) CreateAgentInDB(ctx context.Context, name, projectId string) (*Agent, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	agentId := uuid.New().String()
	var insertedAgentId string

	if err := client.QueryRow(ctx, `
      INSERT INTO public.agent (
          project_id,
          agent_id,
          name
      ) VALUES ((
        SELECT project.id
        FROM public.project
        WHERE project.project_id = $1
      ), $2, $3)
      RETURNING agent.id`, projectId, agentId, name).Scan(&insertedAgentId); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to insert agent into database: %v", err)
	}

	return &Agent{
		Id:      insertedAgentId,
		AgentId: agentId,
		Name:    "Default Agent",
	}, nil
}

func (s *System) UpdateAgentDetails(ctx context.Context, agent Agent) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(ctx, `
    UPDATE public.agent
    SET name = $1, enabled = $2
    WHERE agent_id = $3`, agent.Name, agent.Enabled, agent.AgentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update agent details: %v", err)
	}

	return nil
}

func (s *System) DeleteAgentFromDB(ctx context.Context, agentId string) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	_, err = client.Exec(ctx, `
    DELETE FROM public.agent
    WHERE agent_id = $1`, agentId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete agent: %v", err)
	}

	return nil
}

func (s *System) DeleteAllAgentsForProject(ctx context.Context, projectId string) error {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "operation was canceled") {
			return nil
		}
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("Failed to close database connection: %v", err)
		}
	}()

	var agents []*Agent
	rows, err := client.Query(ctx, `
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
		if err := environment.NewSystem(s.Config).DeleteAllEnvironmentsForAgent(ctx, agent.AgentId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to delete agent environments: %v", err)
		}

		if err := s.DeleteAgentFromDB(ctx, agent.AgentId); err != nil {
			return s.Config.Bugfixes.Logger.Errorf("Failed to delete agent: %v", err)
		}
	}

	return nil
}
