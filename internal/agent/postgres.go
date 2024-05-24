package agent

import (
	"context"
	"errors"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/jackc/pgx/v5"
)

type Agent struct {
	Id           string         `json:"id"`
	Name         string         `json:"name"`
	RequestLimit int            `json:"request_limit"`
	AgentId      string         `json:"agent_id"`
	Environments []*Environment `json:"environments"`
}

func (s *System) AddAgent(name, projectId string) (string, error) {

	return "bob", nil
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

	agent := &Agent{}
	if err := client.QueryRow(s.Context, "SELECT agent.id,  agent.name AS AgentName,  agent.access_limit FROM public.agent AS agent JOIN public.project ON agent.project_id = project.id JOIN public.company ON company.id = project.company_id WHERE agent.agent_id = $1 AND company.company_id = $2", agentId, companyId).Scan(&agent.Id, &agent.Name, &agent.RequestLimit); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
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

	rows, err := client.Query(s.Context, "SELECT agent.id,  agent.name AS AgentName,  agent.access_limit,  agent.agent_id FROM public.agent JOIN public.project ON agent.project_id = project.id JOIN public.company ON project.company_id = company.id WHERE company.company_id = $1", companyId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		agent := &Agent{}
		if err := rows.Scan(&agent.Id, &agent.Name, &agent.RequestLimit, &agent.AgentId); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		envs, err := s.GetAgentEnvironmentsFromDB(agent.AgentId)
		if err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get agent environments: %v", err)
		}
		agent.Environments = envs

		agents = append(agents, agent)
	}

	return agents, nil
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

	rows, err := client.Query(s.Context, "SELECT env.id, env.name, env.env_id FROM agent_environment AS env JOIN agent ON env.agent_id = agent.id WHERE agent.agent_id = $1", agentId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var environments []*Environment
	for rows.Next() {
		environment := &Environment{}
		if err := rows.Scan(&environment.Id, &environment.Name, &environment.EnvironmentId); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database rows: %v", err)
		}

		environments = append(environments, environment)
	}

	return environments, nil
}

func (s *System) GetCompanyId(userSubject string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var companyId string
	if err := client.QueryRow(s.Context, "SELECT public.company.company_id FROM public.company LEFT JOIN public.company_user ON public.company_user.company_id = public.company.id LEFT JOIN public.user ON public.user.id = public.company_user.user_id WHERE public.user.subject = $1", userSubject).Scan(&companyId); err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return companyId, nil
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

	if err := client.QueryRow(ctx, "SELECT TRUE FROM public.agent JOIN public.agent_environment AS pa ON pa.agent_id = agent.id JOIN public.project ON project.id = agent.project_id WHERE agent.agent_id = $1 AND pa.env_id = $2 AND project.project_id = $3", agentId, environmentId, projectId).Scan(&valid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

	if err := client.QueryRow(ctx, "SELECT TRUE FROM public.agent JOIN project ON project.id = agent.project_id WHERE agent.agent_id = $1 AND project.project_id = $2", agentId, projectId).Scan(&valid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return valid, nil
		}
		return valid, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return valid, nil
}
