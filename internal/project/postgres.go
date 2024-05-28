package project

import (
	"github.com/flags-gg/orchestrator/internal/agent"
	"github.com/google/uuid"
)

type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ProjectID  string `json:"project_id"`
	AgentLimit int    `json:"agent_limit"`
}

func (s *System) GetProjectsFromDB(userId string) ([]Project, error) {
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
      project.allowed_agents
    FROM public.project
      JOIN public.company ON company.id = project.company_id
      JOIN public.company_user ON company_user.company_id = company.id
      JOIN public.user AS u ON u.id = company_user.user_id
    WHERE u.subject = $1`, userId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.ProjectID, &project.Name, &project.AgentLimit); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
		}
		projects = append(projects, project)
	}

	return projects, nil
}

func (s *System) GetProjectFromDB(userId, projectId string) (*Project, error) {
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
	if err := client.QueryRow(s.Context, `
        SELECT project.name,
               project.id,
               project.project_id,
               project.allowed_agents
        FROM public.project
          JOIN public.company ON company.id = project.company_id
          JOIN public.company_user ON company_user.company_id = company.id
          JOIN public.user AS u ON u.id = company_user.user_id
        WHERE project_id = $2
          AND u.subject = $1`, userId, projectId).Scan(&project.Name, &project.ID, &project.ProjectID, &project.AgentLimit); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
	}

	return &project, nil
}

func (s *System) CreateProjectInDB(userSubject, projectName string) (*Project, error) {
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
          name,
          allowed_agents
      ) VALUES ((
        SELECT
          company.id
        FROM public.company
          JOIN public.company_user ON company_user.company_id = company.id
          JOIN public.user AS u ON u.id = company_user.user_id
        WHERE u.subject = $1
      ), $2, $3, (
        SELECT allowed_agents_per_project
        FROM public.company
            JOIN public.company_user ON company_user.company_id = company.id
            JOIN public.user AS u ON u.id = company_user.user_id
        WHERE u.subject = $1
      ))
      RETURNING project.id`, userSubject, projectId, projectName).Scan(&insertedProjectId); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to insert project into database: %v", err)
	}

	// create the default agent
	agentDetails, err := agent.NewSystem(s.Config).CreateAgentInDB("Default Agent", projectId, userSubject)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to create default agent: %v", err)
	}

	_, err = agent.NewSystem(s.Config).CreateEnvironmentInDB("Default Env", agentDetails.AgentId, userSubject)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to create default environment: %v", err)
	}

	return &Project{
		ID:   projectId,
		Name: projectName,
	}, nil
}