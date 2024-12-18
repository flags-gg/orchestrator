package project

import (
	"context"
	"database/sql"
	"errors"
	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/flags-gg/orchestrator/internal/environment"
	"github.com/google/uuid"
)

type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ProjectID  string `json:"project_id"`
	AgentLimit int    `json:"agent_limit"`
	AgentsUsed int    `json:"agents_used"`
	Logo       string `json:"logo"`
	Enabled    bool   `json:"enabled"`
}

func (s *System) GetProjectsFromDB(companyId string) ([]Project, error) {
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
      project.id,
      project.project_id,
      project.name,
      project.logo,
      project.enabled,
      payment_plans.agents,
      (
        SELECT COUNT(id)
        FROM public.agent
        WHERE project_id = project.id
      ) AS agents_used
    FROM public.project
      JOIN public.company ON company.id = project.company_id
      JOIN public.company_user ON company_user.company_id = company.id
      JOIN public.payment_plans ON payment_plans.id = company.payment_plan_id
    WHERE company.company_id = $1`, companyId)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		var projectLogo sql.NullString
		if err := rows.Scan(&project.ID, &project.ProjectID, &project.Name, &projectLogo, &project.Enabled, &project.AgentLimit, &project.AgentsUsed); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
		}

		if projectLogo.Valid {
			project.Logo = projectLogo.String
		}

		projects = append(projects, project)
	}

	return projects, nil
}

func (s *System) GetProjectFromDB(companyId, projectId string) (*Project, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var project Project
	var pLogo sql.NullString
	if err := client.QueryRow(s.Context, `
        SELECT
          project.name,
          project.id,
          project.project_id,
          payment_plans.agents,
          project.logo,
          project.enabled,
          (
              SELECT COUNT(id)
              FROM public.agent
              WHERE project_id = project.id
          ) as agents_used
        FROM public.project
          JOIN public.company ON company.id = project.company_id
          JOIN public.company_user ON company_user.company_id = company.id
          JOIN public.payment_plans ON payment_plans.id = company.payment_plan_id
        WHERE project_id = $2
          AND company.company_id = $1`, companyId, projectId).Scan(&project.Name, &project.ID, &project.ProjectID, &project.AgentLimit, &pLogo, &project.Enabled, &project.AgentsUsed); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
	}
	if pLogo.Valid {
		project.Logo = pLogo.String
	}

	return &project, nil
}

func (s *System) CreateProjectInDB(companyId, projectName string) (*Project, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	// create the project
	projectId := uuid.New().String()
	var insertedProjectId string
	if err := client.QueryRow(s.Context, `
      INSERT INTO public.project (
          company_id,
          project_id,
          name
      ) VALUES ((
        SELECT
          company.id
        FROM public.company
        WHERE company.company_id = $1
      ), $2, $3)
      RETURNING project.id`, companyId, projectId, projectName).Scan(&insertedProjectId); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to insert project into database: %v", err)
	}

	// create the default agent
	agentDetails, err := agent.NewSystem(s.Config).CreateAgentInDB("Default Agent", projectId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to create default agent: %v", err)
	}

	_, err = environment.NewSystem(s.Config).CreateEnvironmentInDB("Default Env", agentDetails.AgentId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to create default environment: %v", err)
	}

	return &Project{
		ID:        insertedProjectId,
		ProjectID: projectId,
		Name:      projectName,
	}, nil
}

func (s *System) UpdateProjectInDB(projectId, projectName string, enabled bool) (*Project, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `
      UPDATE public.project
      SET name = $1, enabled = $2
      WHERE project_id = $3`, projectName, enabled, projectId); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to update project in database: %v", err)
	}

	return &Project{
		ProjectID: projectId,
		Name:      projectName,
		Enabled:   enabled,
	}, nil
}

func (s *System) DeleteProjectInDB(projectId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `
      DELETE FROM public.project
      WHERE project_id = $1`, projectId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to delete project in database: %v", err)
	}

	return nil
}

func (s *System) UpdateProjectImageInDB(projectId, logo string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `
      UPDATE public.project
      SET logo = $1
      WHERE project_id = $2`, logo, projectId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update project in database: %v", err)
	}

	return nil
}

type AgentLimits struct {
	AgentsAllowed int `json:"allowed"`
	AgentsUsed    int `json:"used"`
}

func (s *System) GetLimitsFromDB(companyId, projectId string) (AgentLimits, error) {
	var limits AgentLimits

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return limits, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if err := client.QueryRow(s.Context, `
    SELECT
		pp.agents,
		(
	    	SELECT COUNT(a.id)
			FROM public.agent AS a
			  JOIN public.project AS p ON p.id = a.project_id
			WHERE p.project_id = $2
		) AS projects_used
	FROM public.payment_plans AS pp
		JOIN public.company AS c ON c.payment_plan_id = pp.id
	WHERE c.company_id = $1`, companyId, projectId).Scan(
		&limits.AgentsAllowed,
		&limits.AgentsUsed); err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return limits, nil
		}
		return limits, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return limits, nil
}
