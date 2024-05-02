package agent

import (
	"errors"
	"fmt"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/jackc/pgx/v5"
)

type Agent struct {
	Id           string         `json:"id"`
	Name         string         `json:"name"`
	RequestLimit int            `json:"request_limit"`
	AgentId      string         `json:"agent_id"`
	Environment  []*Environment `json:"environments"`
}

func (s *System) AddAgent(name, companyId string) (string, error) {

	return "bob", nil
}

func (s *System) GetAgentDetails(agentId, companyId string) (*Agent, error) {
	client, err := pgx.Connect(s.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", s.Config.Database.User, s.Config.Database.Password, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName))
	if err != nil {
		return nil, logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	agent := &Agent{}
	if err := client.QueryRow(s.Context, "SELECT agent.id, agent.name AS AgentName, agent.access_limit, company.name AS CompanyName FROM public.agent AS agent LEFT JOIN public.company AS company ON agent.company_id = company.id WHERE agent.agent_id = $1 AND company.company_id = $2", agentId, companyId).Scan(&agent.Id, &agent.Name, &agent.RequestLimit); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, logs.Errorf("Failed to query database: %v", err)
	}

	return agent, nil
}

func (s *System) GetAgents(companyId string) ([]*Agent, error) {
	client, err := pgx.Connect(s.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", s.Config.Database.User, s.Config.Database.Password, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName))
	if err != nil {
		return nil, logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(s.Context, "SELECT agent.id, agent.name AS AgentName, agent.access_limit,agent.agent_id FROM public.agent AS agent LEFT JOIN public.company AS company ON agent.company_id = company.id WHERE company.company_id = $1", companyId)
	if err != nil {
		return nil, logs.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		agent := &Agent{}
		if err := rows.Scan(&agent.Id, &agent.Name, &agent.RequestLimit, &agent.AgentId); err != nil {
			return nil, logs.Errorf("Failed to scan database rows: %v", err)
		}

		envs, err := s.GetAgentEnvironments(agent.AgentId)
		if err != nil {
			return nil, logs.Errorf("Failed to get agent environments: %v", err)
		}
		agent.Environment = envs

		agents = append(agents, agent)
	}

	return agents, nil
}

func (s *System) GetAgentEnvironments(agentId string) ([]*Environment, error) {
	client, err := pgx.Connect(s.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", s.Config.Database.User, s.Config.Database.Password, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName))
	if err != nil {
		return nil, logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(s.Context, "SELECT env.id, env.name, env.env_id FROM agent_environment AS env JOIN agent ON env.agent_id = agent.id WHERE agent.agent_id = $1", agentId)
	if err != nil {
		return nil, logs.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var environments []*Environment
	for rows.Next() {
		environment := &Environment{}
		if err := rows.Scan(&environment.Id, &environment.Name, &environment.EnvironmentId); err != nil {
			return nil, logs.Errorf("Failed to scan database rows: %v", err)
		}

		environments = append(environments, environment)
	}

	return environments, nil
}

func (s *System) GetCompanyId(userSubject string) (string, error) {
	client, err := pgx.Connect(s.Context, fmt.Sprintf("postgres://%s:%s@%s:%d/%s", s.Config.Database.User, s.Config.Database.Password, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName))
	if err != nil {
		return "", logs.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var companyId string
	if err := client.QueryRow(s.Context, "SELECT public.company.company_id FROM public.company LEFT JOIN public.company_user ON public.company_user.company_id = public.company.id LEFT JOIN public.user ON public.user.id = public.company_user.user_id WHERE public.user.subject = $1", userSubject).Scan(&companyId); err != nil {
		return "", logs.Errorf("Failed to query database: %v", err)
	}

	return companyId, nil
}
