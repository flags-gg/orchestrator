package stats

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/flags-gg/orchestrator/internal/company"
	"github.com/jackc/pgx/v5"
)

var ErrAgentNotFound = errors.New("agent not found")

type EnvironmentMetrics struct {
	EnvironmentId string `json:"environment_id"`
	Name          string `json:"name"`
	Level         int    `json:"level"`
	Users         []int  `json:"users"`    // distinct targeting keys per day
	Requests      []int  `json:"requests"` // SDK/OFREP flag requests per day
	// response time of the flags endpoint over the whole range; the bucket's upper bound, 1001 means >1s
	P50Ms int `json:"p50_ms"`
	P95Ms int `json:"p95_ms"`
}

type FlagMetrics struct {
	Name          string `json:"name"`
	EnvironmentId string `json:"environment_id"`
	ServedOn      []int  `json:"served_on"`
	ServedOff     []int  `json:"served_off"`
	RuleHits      []int  `json:"rule_hits"`
}

type AgentMetrics struct {
	Days         []string             `json:"days"`
	Environments []EnvironmentMetrics `json:"environments"`
	Flags        []FlagMetrics        `json:"flags"`
}

// percentile returns the upper bound (ms) of the histogram bucket holding the p-th request.
func percentile(buckets []int, p float64) int {
	total := 0
	for _, n := range buckets {
		total += n
	}
	if total == 0 {
		return 0
	}
	seen := 0
	for i, n := range buckets {
		seen += n
		if float64(seen) >= p*float64(total) {
			if i < len(LatencyBucketsMs) {
				return LatencyBucketsMs[i]
			}
			break
		}
	}
	return LatencyBucketsMs[len(LatencyBucketsMs)-1] + 1
}

// GetAgentMetrics returns daily usage for the agent's environments and flags over the last `days` days.
func (s *System) GetAgentMetrics(ctx context.Context, companyId, agentId string, days int) (*AgentMetrics, error) {
	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close(ctx) }()

	out := &AgentMetrics{Days: []string{}, Environments: []EnvironmentMetrics{}, Flags: []FlagMetrics{}}
	rows, err := client.Query(ctx, `SELECT to_char(d, 'YYYY-MM-DD') FROM generate_series(current_date - ($1::int - 1), current_date, '1 day') d`, days)
	if err != nil {
		return nil, err
	}
	if out.Days, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return nil, err
	}
	dayIdx := map[string]int{}
	for i, d := range out.Days {
		dayIdx[d] = i
	}
	series := func() []int { return make([]int, len(out.Days)) }

	rows, err = client.Query(ctx, `
    SELECT e.id, e.env_id, COALESCE(e.name, ''), e.level
    FROM public.environment e
      JOIN public.agent a ON a.id = e.agent_id
      JOIN public.project p ON p.id = a.project_id
      JOIN public.company c ON c.id = p.company_id
    WHERE a.agent_id = $1 AND c.company_id = $2
    ORDER BY e.level`, agentId, companyId)
	if err != nil {
		return nil, err
	}
	envs := map[int]int{}           // db id -> index in out.Environments
	envByPublic := map[string]int{} // env_id -> index
	var ids []int
	for rows.Next() {
		var id int
		em := EnvironmentMetrics{}
		if err := rows.Scan(&id, &em.EnvironmentId, &em.Name, &em.Level); err != nil {
			rows.Close()
			return nil, err
		}
		em.Users, em.Requests = series(), series()
		envs[id], envByPublic[em.EnvironmentId] = len(out.Environments), len(out.Environments)
		ids = append(ids, id)
		out.Environments = append(out.Environments, em)
	}
	rows.Close()
	if len(ids) == 0 {
		// no environments: tell an unknown/foreign agent apart from an empty one
		var one int
		if err := client.QueryRow(ctx, `
      SELECT 1 FROM public.agent a JOIN public.project p ON p.id = a.project_id JOIN public.company c ON c.id = p.company_id
      WHERE a.agent_id = $1 AND c.company_id = $2`, agentId, companyId).Scan(&one); err != nil {
			return nil, ErrAgentNotFound
		}
		return out, nil
	}

	latency := make([][]int, len(out.Environments))
	rows, err = client.Query(ctx, `
    SELECT environment_id, to_char(day, 'YYYY-MM-DD'), users, latency_buckets FROM public.environment_usage_daily
    WHERE environment_id = ANY($1) AND day > current_date - $2::int`, ids, days)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, users int
		var day string
		var buckets []int
		if err := rows.Scan(&id, &day, &users, &buckets); err != nil {
			rows.Close()
			return nil, err
		}
		i := envs[id]
		out.Environments[i].Users[dayIdx[day]] = users
		if latency[i] == nil {
			latency[i] = make([]int, len(buckets))
		}
		for b, n := range buckets {
			if b < len(latency[i]) {
				latency[i][b] += n
			}
		}
	}
	rows.Close()
	for i := range out.Environments {
		out.Environments[i].P50Ms, out.Environments[i].P95Ms = percentile(latency[i], 0.5), percentile(latency[i], 0.95)
	}

	publicIds := make([]string, 0, len(envByPublic))
	for id := range envByPublic {
		publicIds = append(publicIds, id)
	}
	rows, err = client.Query(ctx, `
    SELECT environment_id, to_char(created_at::date, 'YYYY-MM-DD'), count(*)::int FROM public.environment_request_audit
    WHERE environment_id = ANY($1) AND created_at::date > current_date - $2::int
    GROUP BY 1, 2`, publicIds, days)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var env, day string
		var n int
		if err := rows.Scan(&env, &day, &n); err != nil {
			rows.Close()
			return nil, err
		}
		out.Environments[envByPublic[env]].Requests[dayIdx[day]] = n
	}
	rows.Close()

	rows, err = client.Query(ctx, `
    SELECT environment_id, flag_name, to_char(day, 'YYYY-MM-DD'), served_on, served_off, rule_hits FROM public.flag_exposure_daily
    WHERE environment_id = ANY($1) AND day > current_date - $2::int
    ORDER BY flag_name, environment_id`, ids, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	flagIdx := map[string]int{}
	for rows.Next() {
		var id, on, off, hits int
		var name, day string
		if err := rows.Scan(&id, &name, &day, &on, &off, &hits); err != nil {
			return nil, err
		}
		env := out.Environments[envs[id]].EnvironmentId
		k := env + "\x00" + name
		fi, ok := flagIdx[k]
		if !ok {
			fi = len(out.Flags)
			flagIdx[k] = fi
			out.Flags = append(out.Flags, FlagMetrics{Name: name, EnvironmentId: env, ServedOn: series(), ServedOff: series(), RuleHits: series()})
		}
		d := dayIdx[day]
		out.Flags[fi].ServedOn[d], out.Flags[fi].ServedOff[d], out.Flags[fi].RuleHits[d] = on, off, hits
	}
	return out, rows.Err()
}

// GetAgentMetricsHandler: GET /agent/{agentId}/metrics?days=14 (1-90)
func (s *System) GetAgentMetricsHandler(w http.ResponseWriter, r *http.Request) {
	agentId := r.PathValue("agentId")
	companyId, ok := company.NewSystem(s.Config).CallerOwns(w, r, company.ResourceAgent, agentId)
	if !ok {
		return
	}
	days := 14
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 90 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		days = n
	}
	m, err := s.GetAgentMetrics(r.Context(), companyId, agentId, days)
	if errors.Is(err, ErrAgentNotFound) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		_ = s.Config.Bugfixes.Logger.Errorf("Failed to get agent metrics: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m)
}
