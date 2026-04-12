package stats

import (
	"context"
	"strings"
)

type RequestKind string

const (
	RequestKindSingleFlag RequestKind = "single_flag"
	RequestKindAllFlags   RequestKind = "all_flags"
)

type RequestSource string

const (
	RequestSourceOFREPSingle RequestSource = "ofrep_single"
	RequestSourceOFREPBulk   RequestSource = "ofrep_bulk"
	RequestSourceSDKAll      RequestSource = "sdk_all"
)

type EnvironmentRequestSummary struct {
	EnvironmentID      string `json:"environment_id"`
	EnvironmentName    string `json:"environment_name"`
	AgentID            string `json:"agent_id"`
	AgentName          string `json:"agent_name"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	SingleFlagRequests int    `json:"single_flag_requests"`
	AllFlagsRequests   int    `json:"all_flags_requests"`
	TotalRequests      int    `json:"total_requests"`
}

type RequestTotals struct {
	SingleFlagRequests int `json:"single_flag_requests"`
	AllFlagsRequests   int `json:"all_flags_requests"`
	TotalRequests      int `json:"total_requests"`
}

type APIKeyCreator struct {
	Subject         string `json:"subject"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	CreatedAt       string `json:"created_at"`
	ProjectID       string `json:"project_id"`
	ProjectName     string `json:"project_name"`
	AgentID         string `json:"agent_id"`
	AgentName       string `json:"agent_name"`
	EnvironmentID   string `json:"environment_id,omitempty"`
	EnvironmentName string `json:"environment_name,omitempty"`
}

type CompanyOverview struct {
	RequestTotals       RequestTotals               `json:"request_totals"`
	EnvironmentRequests []EnvironmentRequestSummary `json:"environment_requests"`
	LatestAPIKeyCreator *APIKeyCreator              `json:"latest_api_key_creator,omitempty"`
}

func (s *System) RecordEnvironmentRequest(ctx context.Context, projectId, agentId, environmentId string, requestKind RequestKind, requestSource RequestSource) error {
	if projectId == "" || agentId == "" || environmentId == "" {
		return nil
	}

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

	if _, err := client.Exec(ctx, `
		INSERT INTO public.environment_request_audit (
			project_id,
			agent_id,
			environment_id,
			request_kind,
			request_source
		) VALUES ($1, $2, $3, $4, $5)`,
		projectId,
		agentId,
		environmentId,
		requestKind,
		requestSource,
	); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to insert environment request audit: %v", err)
	}

	return nil
}

func (s *System) RecordAPIKeyCreation(ctx context.Context, userSubject, projectId, agentId, environmentId string) error {
	if userSubject == "" || projectId == "" || agentId == "" {
		return nil
	}

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

	if _, err := client.Exec(ctx, `
		INSERT INTO public.api_key_audit (
			project_id,
			agent_id,
			environment_id,
			created_by_subject
		) VALUES ($1, $2, NULLIF($3, ''), $4)`,
		projectId,
		agentId,
		environmentId,
		userSubject,
	); err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to insert api key audit: %v", err)
	}

	return nil
}

func (s *System) GetCompanyOverview(ctx context.Context, companyId string) (*CompanyOverview, error) {
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

	overview := &CompanyOverview{
		EnvironmentRequests: make([]EnvironmentRequestSummary, 0),
	}

	rows, err := client.Query(ctx, `
		SELECT
			era.environment_id,
			COALESCE(env.name, ''),
			era.agent_id,
			COALESCE(agent.name, ''),
			era.project_id,
			COALESCE(project.name, ''),
			COUNT(*) FILTER (WHERE era.request_kind = 'single_flag')::int AS single_flag_requests,
			COUNT(*) FILTER (WHERE era.request_kind = 'all_flags')::int AS all_flags_requests,
			COUNT(*)::int AS total_requests
		FROM public.environment_request_audit era
			JOIN public.project project ON project.project_id = era.project_id
			JOIN public.company company ON company.id = project.company_id
			LEFT JOIN public.agent agent ON agent.agent_id = era.agent_id
			LEFT JOIN public.environment env ON env.env_id = era.environment_id
		WHERE company.company_id = $1
		GROUP BY
			era.environment_id,
			env.name,
			era.agent_id,
			agent.name,
			era.project_id,
			project.name
		ORDER BY total_requests DESC, project.name ASC, agent.name ASC, env.name ASC`, companyId)
	if err != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to query environment request audit: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		summary := EnvironmentRequestSummary{}
		if err := rows.Scan(
			&summary.EnvironmentID,
			&summary.EnvironmentName,
			&summary.AgentID,
			&summary.AgentName,
			&summary.ProjectID,
			&summary.ProjectName,
			&summary.SingleFlagRequests,
			&summary.AllFlagsRequests,
			&summary.TotalRequests,
		); err != nil {
			return nil, s.Config.Bugfixes.Logger.Errorf("Failed to scan environment request audit row: %v", err)
		}

		overview.RequestTotals.SingleFlagRequests += summary.SingleFlagRequests
		overview.RequestTotals.AllFlagsRequests += summary.AllFlagsRequests
		overview.RequestTotals.TotalRequests += summary.TotalRequests
		overview.EnvironmentRequests = append(overview.EnvironmentRequests, summary)
	}

	if rows.Err() != nil {
		return nil, s.Config.Bugfixes.Logger.Errorf("Failed to iterate environment request audit rows: %v", rows.Err())
	}

	creator := &APIKeyCreator{}
	err = client.QueryRow(ctx, `
		SELECT
			aka.created_by_subject,
			COALESCE(
				NULLIF(u.known_as, ''),
				NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''),
				NULLIF(u.email_address, ''),
				aka.created_by_subject
			) AS display_name,
			COALESCE(u.email_address, ''),
			aka.created_at::text,
			aka.project_id,
			COALESCE(project.name, ''),
			aka.agent_id,
			COALESCE(agent.name, ''),
			COALESCE(aka.environment_id, ''),
			COALESCE(env.name, '')
		FROM public.api_key_audit aka
			JOIN public.project project ON project.project_id = aka.project_id
			JOIN public.company company ON company.id = project.company_id
			LEFT JOIN public.agent agent ON agent.agent_id = aka.agent_id
			LEFT JOIN public.environment env ON env.env_id = aka.environment_id
			LEFT JOIN public.user u ON u.subject = aka.created_by_subject
		WHERE company.company_id = $1
		ORDER BY aka.created_at DESC
		LIMIT 1`, companyId).Scan(
		&creator.Subject,
		&creator.Name,
		&creator.Email,
		&creator.CreatedAt,
		&creator.ProjectID,
		&creator.ProjectName,
		&creator.AgentID,
		&creator.AgentName,
		&creator.EnvironmentID,
		&creator.EnvironmentName,
	)
	if err == nil {
		overview.LatestAPIKeyCreator = creator
	}

	return overview, nil
}
