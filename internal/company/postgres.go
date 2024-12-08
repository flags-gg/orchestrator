package company

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Agents struct {
	Allowed int          `json:"allowed"`
	Used    []AgentsUsed `json:"used,omitempty"`
}
type AgentsUsed struct {
	ProjectID  string `json:"project_id"`
	AgentsUsed int    `json:"used"`
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

type Details struct {
	Company     *Company       `json:"company,omitempty"`
	Avatar      sql.NullString `json:"avatar,omitempty"`
	PaymentPlan sql.NullString `json:"paymentPlan,omitempty"`
	Timezone    sql.NullString `json:"timezone,omitempty"`
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
    WITH user_company AS (
        SELECT c.id AS company_id, c.allowed_projects
        FROM public.company c
        JOIN public.company_user cu ON cu.company_id = c.id
        JOIN public.user u ON u.id = cu.user_id
        WHERE u.subject = $1
        LIMIT 1
    )
    SELECT
        uc.allowed_projects,
        (SELECT COUNT(*)
         FROM public.project p
         WHERE p.company_id = uc.company_id
        ) AS used_projects
    FROM user_company uc
    `, userSubject).Scan(&p.Allowed, &p.Used); err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return p, nil
}

func (s *System) GetUserLimits(userSubject string) (*Users, error) {
	u := &Users{
		Allowed:   1,
		Activated: 1,
	}

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
    WITH user_company AS (
        SELECT c.id AS company_id, c.allowed_members
        FROM public.company c
        JOIN public.company_user cu ON cu.company_id = c.id
        JOIN public.user u ON u.id = cu.user_id
        WHERE u.subject = $1
        LIMIT 1
    )
    SELECT
        uc.allowed_members,
        (SELECT COUNT(*)
         FROM public.company_user cu2
         WHERE cu2.company_id = uc.company_id
        ) AS activated_count
    FROM user_company uc
    `, userSubject).Scan(&u.Allowed, &u.Activated); err != nil {
		if err.Error() == "context canceled" {
			return u, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return u, nil
}

func (s *System) GetAgentLimits(userSubject string) (*Agents, error) {
	a := &Agents{}

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	// Prepare the query
	rows, err := client.Query(s.Context, `
    WITH user_company AS (
        SELECT c.id AS company_id, c.allowed_agents_per_project
        FROM public.company c
        JOIN public.company_user cu ON cu.company_id = c.id
        JOIN public.user u ON u.id = cu.user_id
        WHERE u.subject = $1
        LIMIT 1
    )
    SELECT
        uc.allowed_agents_per_project,
        p.project_id AS project_id,
        COUNT(a.id) AS agents_used
    FROM user_company uc
    JOIN public.project p ON p.company_id = uc.company_id
    LEFT JOIN public.agent a ON a.project_id = p.id
    GROUP BY uc.allowed_agents_per_project, p.id
    `, userSubject)
	if err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	defer rows.Close()
	a.Used = []AgentsUsed{}

	// Iterate over the result set
	for rows.Next() {
		var projectID string // Adjust the type if necessary
		var agentsUsed int
		// Since allowed_agents_per_project is the same for all rows, we can capture it once
		if err := rows.Scan(&a.Allowed, &projectID, &agentsUsed); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan row: %v", err)
		}
		// Append the project data to the Projects slice
		a.Used = append(a.Used, AgentsUsed{
			ProjectID:  projectID,
			AgentsUsed: agentsUsed,
		})
	}

	if err := rows.Err(); err != nil {
		if err.Error() == "context canceled" {
			return a, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Row iteration error: %v", err)
	}

	return a, nil
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
		if err.Error() == "no rows in result set" {
			return "", nil
		}
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return companyId, nil
}

func (s *System) GetCompanyInfo(userSubject string) (*Details, error) {
	companyId, err := s.GetCompanyId(userSubject)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to get company id: %v", err)
	}

	_ = fmt.Sprintf("Company ID: %s", companyId)
	details := &Details{}
	company := &Company{}
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
    SELECT
		company_id,
  		name AS companyName,
  		domain AS companyDomain,
  		invite_code
	FROM company
	WHERE company_id = $1`, companyId).Scan(&company.ID, &company.Name, &company.Domain, &company.InviteCode); err != nil {
		if err.Error() == "context canceled" {
			return nil, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	details.Company = company

	return details, nil
}

func (s *System) GetCompanyBasedOnDomain(domain, inviteCode string) (bool, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return false, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var companyId string
	if err := client.QueryRow(s.Context, `
    SELECT company_id
	FROM company
	WHERE domain = $1 OR invite_code = $2`, domain, inviteCode).Scan(&companyId); err != nil {
		if err.Error() == "context canceled" {
			return false, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	s.CompanyID = companyId

	return true, nil
}

func (s *System) AttachUserToCompanyDB(userSubject string) error {
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
    INSERT INTO public.company_user (
        company_id,
        user_id
    ) VALUES (
        (SELECT id FROM company WHERE company_id = $1),
        (SELECT id FROM public.user WHERE subject = $2)
    )`, s.CompanyID, userSubject)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to insert user into database: %v", err)
	}

	_, err = client.Exec(s.Context, `
		UPDATE public.user
		SET onboarded = true
		WHERE subject = $1`, userSubject)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update user onboarded status: %v", err)
	}

	return nil
}

func (s *System) CreateCompanyDB(name, domain, userSubject string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	companyId := uuid.New().String()
	inviteCode := uuid.New().String()
	apiKey := uuid.New().String()
	apiSecret := uuid.New().String()

	if _, err := client.Exec(s.Context, `
    INSERT INTO public.company (
        name,
        domain,
        company_id,
        invite_code,
        api_key,
        api_secret
    ) VALUES (
        $1,
        $2,
        $3,
        $4,
        $5,
        $6
    )`, name, domain, companyId, inviteCode, apiKey, apiSecret); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to insert company into database: %v", err)
	}

	if _, err := client.Exec(s.Context, `
    INSERT INTO public.company_user (
        company_id,
        user_id
    ) VALUES (
        (SELECT id FROM company WHERE company_id = $1),
        (SELECT id FROM public.user WHERE subject = $2)
    )`, companyId, userSubject); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to insert user into company_user: %v", err)
	}

	return nil
}
