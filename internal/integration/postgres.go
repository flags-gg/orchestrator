package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrNoEnvironment = errors.New("agent has no environments")
	ErrNameTaken     = errors.New("flag name already used in that environment")
)

type EnvironmentState struct {
	EnvironmentId string `json:"environment_id"`
	Name          string `json:"name"`
	Level         int    `json:"level"`
	Present       bool   `json:"present"`
	Enabled       bool   `json:"enabled"`
	Production    bool   `json:"production"`

	id int
}

type TicketFlag struct {
	Ticket       string             `json:"ticket"`
	Name         string             `json:"name"`
	AgentId      string             `json:"agent_id"`
	Environments []EnvironmentState `json:"environments"`

	agentId int
}

type Token struct {
	Id        int       `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *System) conn(ctx context.Context) (*pgx.Conn, func(), error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return nil, nil, s.Config.Bugfixes.Logger.Errorf("failed to connect to database: %v", err)
	}
	return client, func() {
		if err := client.Close(ctx); err != nil {
			_ = s.Config.Bugfixes.Logger.Errorf("failed to close database connection: %v", err)
		}
	}, nil
}

// queryer is satisfied by both *pgx.Conn and pgx.Tx.
type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ticketFlag loads the ticket's flag with its state in every environment of its agent, lowest level first.
func ticketFlag(ctx context.Context, q queryer, companyId, ticket string) (*TicketFlag, error) {
	rows, err := q.Query(ctx, `
    WITH tf AS (
      SELECT f.agent_id, f.name
      FROM public.flag f
        JOIN public.environment e ON e.id = f.environment_id
        JOIN public.agent a ON a.id = f.agent_id
        JOIN public.project p ON p.id = a.project_id
        JOIN public.company c ON c.id = p.company_id
      WHERE f.ticket_ref = $2 AND c.company_id = $1
      ORDER BY e.level
      LIMIT 1
    )
    SELECT tf.agent_id, a.agent_id, tf.name, e.id, e.env_id, COALESCE(e.name, ''), e.level,
           f.id IS NOT NULL, COALESCE(f.enabled, false),
           e.level = MAX(e.level) OVER ()
    FROM tf
      JOIN public.agent a ON a.id = tf.agent_id
      JOIN public.environment e ON e.agent_id = tf.agent_id
      LEFT JOIN public.flag f ON f.environment_id = e.id AND f.ticket_ref = $2
    ORDER BY e.level`, companyId, ticket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tf *TicketFlag
	for rows.Next() {
		if tf == nil {
			tf = &TicketFlag{Ticket: ticket}
		}
		var env EnvironmentState
		if err := rows.Scan(&tf.agentId, &tf.AgentId, &tf.Name, &env.id, &env.EnvironmentId, &env.Name, &env.Level,
			&env.Present, &env.Enabled, &env.Production); err != nil {
			return nil, err
		}
		tf.Environments = append(tf.Environments, env)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if tf == nil {
		return nil, ErrNotFound
	}
	return tf, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *System) GetTicketFlag(ctx context.Context, companyId, ticket string) (*TicketFlag, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	return ticketFlag(ctx, client, companyId, ticket)
}

// CreateTicketFlag creates the flag, off, in the agent's first environment. If the ticket already has a flag
// it is returned unchanged (created=false), so retries are safe.
func (s *System) CreateTicketFlag(ctx context.Context, companyId, ticket, agentId, name string) (*TicketFlag, bool, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return nil, false, err
	}
	defer done()

	if tf, err := ticketFlag(ctx, client, companyId, ticket); err == nil {
		return tf, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	var agentIdInt int
	var envId *int
	if err := client.QueryRow(ctx, `
    SELECT a.id, (SELECT e.id FROM public.environment e WHERE e.agent_id = a.id ORDER BY e.level LIMIT 1)
    FROM public.agent a
      JOIN public.project p ON p.id = a.project_id
      JOIN public.company c ON c.id = p.company_id
    WHERE a.agent_id = $1 AND c.company_id = $2`, agentId, companyId).Scan(&agentIdInt, &envId); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrNotFound
		}
		return nil, false, err
	}
	if envId == nil {
		return nil, false, ErrNoEnvironment
	}

	if _, err := client.Exec(ctx, `
    INSERT INTO public.flag (name, agent_id, environment_id, enabled, ticket_ref) VALUES ($1, $2, $3, false, $4)`,
		name, agentIdInt, *envId, ticket); err != nil {
		if isUniqueViolation(err) {
			return nil, false, ErrNameTaken
		}
		return nil, false, err
	}

	tf, err := ticketFlag(ctx, client, companyId, ticket)
	return tf, err == nil, err
}

func (s *System) SetTicketFlag(ctx context.Context, companyId, ticket, environmentId string, enabled bool) error {
	client, done, err := s.conn(ctx)
	if err != nil {
		return err
	}
	defer done()

	tag, err := client.Exec(ctx, `
    UPDATE public.flag f SET enabled = $4, updated_at = now()
    FROM public.environment e, public.agent a, public.project p, public.company c
    WHERE f.ticket_ref = $2 AND e.env_id = $3 AND f.environment_id = e.id
      AND a.id = f.agent_id AND p.id = a.project_id AND c.id = p.company_id AND c.company_id = $1`,
		companyId, ticket, environmentId, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// PromoteTicketFlag adds the flag, off, to the next environment after the deepest one it is in,
// or to every remaining environment when toProduction is set. Already at production is a no-op.
func (s *System) PromoteTicketFlag(ctx context.Context, companyId, ticket string, toProduction bool) (*TicketFlag, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer done()

	tx, err := client.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tf, err := ticketFlag(ctx, tx, companyId, ticket)
	if err != nil {
		return nil, err
	}

	deepest := -1
	for i, env := range tf.Environments {
		if env.Present {
			deepest = i
		}
	}
	targets := tf.Environments[deepest+1:]
	if !toProduction && len(targets) > 1 {
		targets = targets[:1]
	}
	for _, env := range targets {
		if _, err := tx.Exec(ctx, `
      INSERT INTO public.flag (name, agent_id, environment_id, enabled, ticket_ref) VALUES ($1, $2, $3, false, $4)`,
			tf.Name, tf.agentId, env.id, ticket); err != nil {
			if isUniqueViolation(err) {
				return nil, ErrNameTaken
			}
			return nil, err
		}
	}

	if tf, err = ticketFlag(ctx, tx, companyId, ticket); err != nil {
		return nil, err
	}
	return tf, tx.Commit(ctx)
}

// archiveFlags moves the matching flag rows (all environments) into flag_archive; where filters public.flag f
// and may use a (agent), p (project), c (company), $1 (company id) and $2.. (args).
func (s *System) archiveFlags(ctx context.Context, where, companyId string, args ...any) (int64, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return 0, err
	}
	defer done()

	tag, err := client.Exec(ctx, `
    WITH gone AS (
      DELETE FROM public.flag f
      USING public.agent a, public.project p, public.company c
      WHERE `+where+` AND a.id = f.agent_id AND p.id = a.project_id AND c.id = p.company_id AND c.company_id = $1
      RETURNING f.id, f.name, f.agent_id, f.environment_id, f.enabled, f.ticket_ref, f.created_at, f.updated_at, f.rollout
    )
    INSERT INTO public.flag_archive (flag_id, name, agent_id, environment_id, enabled, ticket_ref, created_at, updated_at, rollout)
    SELECT * FROM gone`, append([]any{companyId}, args...)...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ArchiveTicketFlag archives the ticket's flag in every environment; nothing to archive is not an error.
func (s *System) ArchiveTicketFlag(ctx context.Context, companyId, ticket string) error {
	_, err := s.archiveFlags(ctx, "f.ticket_ref = $2", companyId, ticket)
	return err
}

// ArchiveAgentFlag archives the agent's flag with this name in every environment.
func (s *System) ArchiveAgentFlag(ctx context.Context, companyId, agentId, name string) error {
	n, err := s.archiveFlags(ctx, "f.name = $2 AND a.agent_id = $3", companyId, name, agentId)
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

// CreateToken returns the raw token; only its hash is stored, so it cannot be shown again.
func (s *System) CreateToken(ctx context.Context, companyId, name string) (string, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	raw := "flg_" + hex.EncodeToString(b)

	client, done, err := s.conn(ctx)
	if err != nil {
		return "", err
	}
	defer done()

	if _, err := client.Exec(ctx, `
    INSERT INTO public.integration_token (company_id, name, token_hash)
    SELECT id, $2, $3 FROM public.company WHERE company_id = $1`, companyId, name, hashToken(raw)); err != nil {
		return "", err
	}
	return raw, nil
}

func (s *System) ListTokens(ctx context.Context, companyId string) ([]Token, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer done()

	rows, err := client.Query(ctx, `
    SELECT t.id, t.name, t.created_at FROM public.integration_token t
      JOIN public.company c ON c.id = t.company_id
    WHERE c.company_id = $1 ORDER BY t.created_at`, companyId)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Token, error) {
		var t Token
		return t, row.Scan(&t.Id, &t.Name, &t.CreatedAt)
	})
}

func (s *System) DeleteToken(ctx context.Context, companyId, tokenId string) error {
	client, done, err := s.conn(ctx)
	if err != nil {
		return err
	}
	defer done()

	tag, err := client.Exec(ctx, `
    DELETE FROM public.integration_token t USING public.company c
    WHERE t.company_id = c.id AND c.company_id = $1 AND t.id::text = $2`, companyId, tokenId)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CompanyForToken returns the public company id for a raw token, or "" when it is unknown.
func (s *System) CompanyForToken(ctx context.Context, raw string) (string, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return "", err
	}
	defer done()

	var companyId string
	if err := client.QueryRow(ctx, `
    SELECT c.company_id FROM public.integration_token t
      JOIN public.company c ON c.id = t.company_id
    WHERE t.token_hash = $1`, hashToken(raw)).Scan(&companyId); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return companyId, nil
}

type AgentEnvironment struct {
	EnvironmentId string `json:"environment_id"`
	Name          string `json:"name"`
	Level         int    `json:"level"`
	Production    bool   `json:"production"`
}

type AgentInfo struct {
	ProjectId    string             `json:"project_id"`
	ProjectName  string             `json:"project_name"`
	AgentId      string             `json:"agent_id"`
	AgentName    string             `json:"agent_name"`
	Environments []AgentEnvironment `json:"environments"`
}

// FlagState is one flag in one environment; FlagId is what the per-flag dashboard routes take.
type FlagState struct {
	EnvironmentId string `json:"environment_id"`
	FlagId        string `json:"flag_id"`
	Enabled       bool   `json:"enabled"`
	Rollout       int    `json:"rollout"`
}

// AgentFlag is one flag across the agent's environments; States only lists environments it is in.
type AgentFlag struct {
	Name   string      `json:"name"`
	Ticket string      `json:"ticket,omitempty"`
	States []FlagState `json:"states"`
}

type AgentFlags struct {
	AgentId      string             `json:"agent_id"`
	Environments []AgentEnvironment `json:"environments"`
	Flags        []AgentFlag        `json:"flags"`
}

func (s *System) ListAgents(ctx context.Context, companyId string) ([]AgentInfo, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer done()

	rows, err := client.Query(ctx, `
    SELECT COALESCE(p.project_id, ''), COALESCE(p.name, ''), a.agent_id, COALESCE(a.name, ''),
           e.env_id, COALESCE(e.name, ''), e.level, e.level = MAX(e.level) OVER (PARTITION BY a.id)
    FROM public.agent a
      JOIN public.project p ON p.id = a.project_id
      JOIN public.company c ON c.id = p.company_id
      LEFT JOIN public.environment e ON e.agent_id = a.id
    WHERE c.company_id = $1
    ORDER BY p.name, a.name, a.id, e.level`, companyId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentInfo
	for rows.Next() {
		var ai AgentInfo
		var envId, envName *string
		var level *int
		var prod *bool
		if err := rows.Scan(&ai.ProjectId, &ai.ProjectName, &ai.AgentId, &ai.AgentName, &envId, &envName, &level, &prod); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].AgentId != ai.AgentId {
			ai.Environments = []AgentEnvironment{}
			out = append(out, ai)
		}
		if envId != nil {
			last := &out[len(out)-1]
			last.Environments = append(last.Environments, AgentEnvironment{EnvironmentId: *envId, Name: *envName, Level: *level, Production: *prod})
		}
	}
	return out, rows.Err()
}

// GetAgentFlags returns every flag of the agent grouped by name, with its state per environment.
func (s *System) GetAgentFlags(ctx context.Context, companyId, agentId string) (*AgentFlags, error) {
	client, done, err := s.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer done()

	var agentIdInt int
	if err := client.QueryRow(ctx, `
    SELECT a.id FROM public.agent a
      JOIN public.project p ON p.id = a.project_id
      JOIN public.company c ON c.id = p.company_id
    WHERE a.agent_id = $1 AND c.company_id = $2`, agentId, companyId).Scan(&agentIdInt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	out := &AgentFlags{AgentId: agentId, Environments: []AgentEnvironment{}, Flags: []AgentFlag{}}
	rows, err := client.Query(ctx, `
    SELECT e.env_id, COALESCE(e.name, ''), e.level, e.level = MAX(e.level) OVER ()
    FROM public.environment e WHERE e.agent_id = $1 ORDER BY e.level`, agentIdInt)
	if err != nil {
		return nil, err
	}
	if out.Environments, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (AgentEnvironment, error) {
		var e AgentEnvironment
		return e, row.Scan(&e.EnvironmentId, &e.Name, &e.Level, &e.Production)
	}); err != nil {
		return nil, err
	}

	rows, err = client.Query(ctx, `
    SELECT COALESCE(f.name, ''), COALESCE(f.ticket_ref, ''), e.env_id, f.id::text, f.enabled, f.rollout
    FROM public.flag f JOIN public.environment e ON e.id = f.environment_id
    WHERE f.agent_id = $1
    ORDER BY lower(f.name), f.name, e.level`, agentIdInt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, ticket string
		var st FlagState
		if err := rows.Scan(&name, &ticket, &st.EnvironmentId, &st.FlagId, &st.Enabled, &st.Rollout); err != nil {
			return nil, err
		}
		if n := len(out.Flags); n == 0 || out.Flags[n-1].Name != name {
			out.Flags = append(out.Flags, AgentFlag{Name: name})
		}
		last := &out.Flags[len(out.Flags)-1]
		if last.Ticket == "" {
			last.Ticket = ticket
		}
		last.States = append(last.States, st)
	}
	return out, rows.Err()
}
