package agent

import (
	"errors"
	"fmt"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/jackc/pgx/v5"
)

type Agent struct {
	Id           string `json:"id"`
	Name         string `json:"name"`
	RequestLimit int    `json:"request_limit"`
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
			_ = logs.Errorf("Failed to close database connection: %v", err)
		}
	}()

	agent := &Agent{}
	if err := client.QueryRow(s.Context, "SELECT agent.id, agent.name AS AgentName, agent.access_limit, company.name AS CompanyName FROM public.agent AS agent LEFT JOIN public.company AS company ON agent.company_id = company.id WHERE agent.agent_id = $1 AND company.company_id = $2", agentId, companyId).Scan(&agent.Id, &agent.Id, &agent.RequestLimit); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, logs.Errorf("Failed to query database: %v", err)
	}

	return agent, nil
}
