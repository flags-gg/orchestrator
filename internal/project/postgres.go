package project

type Project struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
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

	rows, err := client.Query(s.Context, "SELECT project.id,  project.project_id,  project.name FROM public.project JOIN public.company ON company.id = project.company_id JOIN public.company_user ON company_user.company_id = company.id JOIN public.user AS u ON u.id = company_user.user_id WHERE u.subject = $1", userId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.ProjectID, &project.Name); err != nil {
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

	row := client.QueryRow(s.Context, "SELECT project.name,  project.id,  project.project_id FROM public.project JOIN public.company ON company.id = project.company_id JOIN public.company_user ON company_user.company_id = company.id JOIN public.user AS u ON u.id = company_user.user_id WHERE project_id = $2 AND u.subject = $1", userId, projectId)

	var project Project
	if err := row.Scan(&project.Name, &project.ID, &project.ProjectID); err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
	}

	return &project, nil
}
