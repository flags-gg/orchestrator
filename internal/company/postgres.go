package company

import (
	"context"
	"database/sql"
	"errors"
	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/checkout/session"
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
	Details      PlanDetails `json:"details,omitempty"`
	Agents       Agents      `json:"agents,omitempty"`
	Environments int         `json:"environments,omitempty"`
	Projects     Projects    `json:"projects,omitempty"`
	Users        Users       `json:"users,omitempty"`
}

type PlanDetails struct {
	Price   string `json:"price"`
	PriceDB *sql.NullString

	Name   string `json:"name"`
	NameDB *sql.NullString

	Custom   bool `json:"custom"`
	CustomDB *sql.NullBool

	TeamMembers  int `json:"team_members"`
	Projects     int `json:"projects"`
	Agents       int `json:"agents"`
	Environments int `json:"environments"`
}

type Details struct {
	Company     *Company    `json:"company,omitempty"`
	PaymentPlan PlanDetails `json:"payment_plan,omitempty"`
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
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
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
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return u, nil
}

func (s *System) GetAgentLimits(companyId string) (*Agents, error) {
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
		SELECT 
			c.id AS company_id, 
			pp.agents AS agents
	  	FROM public.company c
			JOIN public.company_user cu ON cu.company_id = c.id
			JOIN public.user u ON u.id = cu.user_id
			JOIN public.payment_plans pp ON pp.id = c.payment_plan_id
		WHERE c.company_id = $1
		LIMIT 1
	)
	SELECT
		uc.agents,
		p.project_id AS project_id,
		COUNT(a.id) AS agents_used
	FROM user_company uc
		JOIN public.project p ON p.company_id = uc.company_id
		LEFT JOIN public.agent a ON a.project_id = p.id
	GROUP BY uc.agents, p.id`, companyId)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
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
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
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
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
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

	details := &Details{}
	company := &Company{}
	paymentPlan := &PlanDetails{}

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
		c.company_id,
  		c.name AS companyName,
  		c.domain AS companyDomain,
  		c.invite_code,
  		c.logo,
        pp.name as paymentPlanName,
        pp.price as paymentPlanPrice,
        pp.custom as paymentPlanCustom,
        pp.team_members as paymentPlanTeamMembers,
        pp.projects as paymentPlanProjects,
        pp.agents as paymentPlanAgents,
        pp.environments as paymentPlanEnvironments,
        pp.price as paymentPlanPrice
	FROM company AS c
	  JOIN payment_plans pp ON pp.id = c.payment_plan_id
	WHERE c.company_id = $1`, companyId).Scan(
		&company.ID,
		&company.Name,
		&company.Domain,
		&company.InviteCode,
		&company.LogoDB,
		&paymentPlan.NameDB,
		&paymentPlan.PriceDB,
		&paymentPlan.CustomDB,
		&paymentPlan.TeamMembers,
		&paymentPlan.Projects,
		&paymentPlan.Agents,
		&paymentPlan.Environments,
		&paymentPlan.Price); err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) || errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}
	details.Company = company
	details.PaymentPlan = *paymentPlan

	// Convert the nullable strings to strings
	if details.Company.LogoDB != nil && details.Company.LogoDB.Valid {
		details.Company.Logo = details.Company.LogoDB.String
	}
	if details.PaymentPlan.NameDB != nil && details.PaymentPlan.NameDB.Valid {
		details.PaymentPlan.Name = details.PaymentPlan.NameDB.String
	}
	if details.PaymentPlan.PriceDB != nil && details.PaymentPlan.PriceDB.Valid {
		details.PaymentPlan.Price = details.PaymentPlan.PriceDB.String
	}
	if details.PaymentPlan.CustomDB != nil && details.PaymentPlan.CustomDB.Valid {
		details.PaymentPlan.Custom = details.PaymentPlan.CustomDB.Bool
	}

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
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
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

type User struct {
	Subject   string `json:"subject"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	KnownAs   string `json:"known_as"`
}

func (s *System) GetCompanyUsersFromDB(companyId string) ([]User, error) {
	var users []User

	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return users, s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			logs.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	rows, err := client.Query(s.Context, `
    SELECT
        u.subject,
        u.first_name,
        u.last_name,
        u.known_as
    FROM public.user AS u
        JOIN public.company_user AS cu ON u.id = cu.user_id
        JOIN public.company AS c ON c.id = cu.company_id
    WHERE c.company_id = $1`, companyId)
	if err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return users, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return users, nil
		}
		return users, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	for rows.Next() {
		var user User
		err := rows.Scan(&user.Subject, &user.FirstName, &user.LastName, &user.KnownAs)
		if err != nil {
			return users, s.Config.Bugfixes.Logger.Errorf("Failed to scan row: %v", err)
		}
		users = append(users, user)
	}

	return users, nil
}

func (s *System) GetLimits(companyId string) (Limits, error) {
	var limits Limits

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
		pp.price,
		pp.name,
		pp.custom,
		pp.team_members,
		pp.projects,
		pp.agents,
		pp.environments,
		(
	    	SELECT COUNT(p.id)
			FROM public.project p
			WHERE p.company_id = c.id
		) AS projects_used,
		(
    		SELECT COUNT(*)
			FROM public.user u
				JOIN public.company_user cu ON cu.user_id = u.id
			WHERE cu.company_id = c.id
		) AS users_used
	FROM public.payment_plans AS pp
		JOIN public.company AS c ON c.payment_plan_id = pp.id
	WHERE c.company_id = $1`, companyId).Scan(
		&limits.Details.Price,
		&limits.Details.Name,
		&limits.Details.Custom,
		&limits.Agents.Allowed,
		&limits.Projects.Allowed,
		&limits.Agents.Allowed,
		&limits.Environments,
		&limits.Projects.Used,
		&limits.Users.Activated); err != nil {
		if err.Error() == "context canceled" || errors.Is(err, context.Canceled) {
			return limits, nil
		}
		return limits, s.Config.Bugfixes.Logger.Errorf("Failed to query database: %v", err)
	}

	return limits, nil
}

func (s *System) UpdateCompanyImageInDB(companyId, image string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	if _, err := client.Exec(s.Context, `UPDATE public.company
		SET logo = $1
		WHERE company_id = $2`, image, companyId); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update project in database: %v", err)
	}

	return nil
}

func (s *System) GetInviteCodeFromDB(companyId string) (string, error) {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	var inviteCode sql.NullString
	if err := client.QueryRow(s.Context, `
    SELECT
      invite_code
    FROM public.company
    WHERE company_id = $1`, companyId).Scan(&inviteCode); err != nil {
		return "", s.Config.Bugfixes.Logger.Errorf("Failed to scan database: %v", err)
	}

	return inviteCode.String, nil
}

func (s *System) UpgradeCompanyInDB(companyId, stripeSessionId string) error {
	client, err := s.Config.Database.GetPGXClient(s.Context)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := client.Close(s.Context); err != nil {
			s.Config.Bugfixes.Logger.Fatalf("Failed to close database connection: %v", err)
		}
	}()

	stripe.Key = s.Config.Local.GetValue("STRIPE_SECRET")
	params := &stripe.CheckoutSessionParams{}
	result, err := session.Get(stripeSessionId, params)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to get stripe session: %v", err)
	}

	priceId, exists := result.Metadata["priceId"]
	if !exists {
		return s.Config.Bugfixes.Logger.Errorf("Failed to get price id from metadata")
	}

	_, err = client.Exec(s.Context, `
    UPDATE public.company
    SET
      payment_plan_id = (
          SELECT id
          FROM public.payment_plans
          WHERE stripe_id = $1
            OR stripe_id_dev = $1
      )
    WHERE company_id = $2`, priceId, companyId)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to update company: %v", err)
	}

	return nil
}
