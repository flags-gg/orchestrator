package company

type Agents struct {
	Allowed int `json:"allowed"`
	Used    int `json:"used"`
}
type Projects struct {
	Allowed int `json:"allowed"`
	Used    int `json:"used"`
}
type Users struct {
	Allowed   int `json:"allowed"`
	Activated int `json:"activated"`
}

type Limits struct {
	Agents   Agents   `json:"agents,omitempty"`
	Projects Projects `json:"projects,omitempty"`
	Users    Users    `json:"users,omitempty"`
}

func (s *System) GetProjectLimits(userSubject string) (*Projects, error) {
	p := &Projects{}

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if err := client.QueryRow(s.Context, `
    SELECT allowed_projects
    FROM public.company
      JOIN public.company_user ON company_user.company_id = company.id
      JOIN public.user AS u ON u.id = company_user.user_id
    WHERE u.subject = $1`, userSubject).Scan(&p.Allowed); err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	if err := client.QueryRow(s.Context, `
    SELECT COUNT(*)
    FROM public.project
      JOIN public.company ON company.id = project.company_id
      JOIN public.company_user ON company_user.company_id = company.id
      JOIN public.user AS u ON u.id = company_user.user_id
    WHERE u.subject = $1`, userSubject).Scan(&p.Used); err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return p, nil
}

func (s *System) GetUserLimits(userSubject string) (*Users, error) {
	u := &Users{}

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if err := client.QueryRow(s.Context, `
    SELECT allowed_members
    FROM public.company
      JOIN public.company_user ON company_user.company_id = company.id
      JOIN public.user AS u ON u.id = company_user.user_id
    WHERE u.subject = $1`, userSubject).Scan(&u.Allowed); err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	if err := client.QueryRow(s.Context, `
    SELECT COUNT(*)
    FROM public.company_user
      JOIN public.user AS u ON u.id = company_user.user_id
    WHERE u.subject = $1`, userSubject).Scan(&u.Activated); err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return u, nil
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
	if err := client.QueryRow(s.Context, `
    SELECT
      public.company.company_id
    FROM public.company
      LEFT JOIN public.company_user ON public.company_user.company_id = public.company.id
      LEFT JOIN public.user ON public.user.id = public.company_user.user_id
    WHERE public.user.subject = $1`, userSubject).Scan(&companyId); err != nil {
		if err.Error() == "context canceled" {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return companyId, nil
}
