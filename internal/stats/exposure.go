package stats

import (
	"context"
	"crypto/sha256"
	"math/rand/v2"
	"time"
)

// Exposure is what one flag was served to one user: the server's own decision.
type Exposure struct {
	Flag    string
	On      bool
	RuleHit bool
}

// LatencyBucketsMs are the upper bounds of the response-time histogram; a final bucket holds the rest.
var LatencyBucketsMs = []int{5, 10, 25, 50, 100, 250, 500, 1000}

func latencyBucket(d time.Duration) int {
	ms := int(d.Milliseconds())
	for i, le := range LatencyBucketsMs {
		if ms <= le {
			return i + 1 // postgres arrays are 1-based
		}
	}
	return len(LatencyBucketsMs) + 1
}

// RecordFlagsRequest stores the endpoint's response time and, the first time a targeting key is seen in the
// environment today, counts that user and what every flag was served to them. Later polls that day cost one insert.
func (s *System) RecordFlagsRequest(ctx context.Context, environmentId, targetingKey string, took time.Duration, exposures []Exposure) error {
	if environmentId == "" {
		return nil
	}
	ctx, cancel := auditContext(ctx)
	defer cancel()

	client, err := s.Config.Database.GetPGXClient(ctx)
	if err != nil {
		return s.Config.Bugfixes.Logger.Errorf("Failed to connect to database: %v", err)
	}
	defer func() { _ = client.Close(ctx) }()

	tx, err := client.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var envId int
	if err := tx.QueryRow(ctx, `SELECT id FROM public.environment WHERE env_id = $1`, environmentId).Scan(&envId); err != nil {
		return nil // unknown environment: nothing to record
	}

	newUser := false
	if targetingKey != "" {
		sum := sha256.Sum256([]byte(environmentId + ":" + targetingKey))
		tag, err := tx.Exec(ctx, `
      INSERT INTO public.environment_user_seen (environment_id, day, key_hash) VALUES ($1, current_date, $2)
      ON CONFLICT DO NOTHING`, envId, sum[:16])
		if err != nil {
			return err
		}
		newUser = tag.RowsAffected() == 1
	}

	users := 0
	if newUser {
		users = 1
	}
	b := latencyBucket(took)
	if _, err := tx.Exec(ctx, `
    INSERT INTO public.environment_usage_daily (environment_id, day, users, latency_buckets)
    VALUES ($1, current_date, $2, (SELECT array_agg(CASE WHEN i = $3 THEN 1 ELSE 0 END ORDER BY i) FROM generate_series(1, 9) i))
    ON CONFLICT (environment_id, day) DO UPDATE SET
      users = environment_usage_daily.users + EXCLUDED.users,
      latency_buckets[$3] = environment_usage_daily.latency_buckets[$3] + 1`, envId, users, b); err != nil {
		return err
	}

	if newUser && len(exposures) > 0 {
		names := make([]string, len(exposures))
		on := make([]bool, len(exposures))
		hit := make([]bool, len(exposures))
		for i, e := range exposures {
			names[i], on[i], hit[i] = e.Flag, e.On, e.RuleHit
		}
		if _, err := tx.Exec(ctx, `
      INSERT INTO public.flag_exposure_daily (environment_id, flag_name, day, served_on, served_off, rule_hits)
      SELECT $1, n, current_date, o::int, (NOT o)::int, h::int FROM unnest($2::text[], $3::bool[], $4::bool[]) AS t(n, o, h)
      ON CONFLICT (environment_id, flag_name, day) DO UPDATE SET
        served_on = flag_exposure_daily.served_on + EXCLUDED.served_on,
        served_off = flag_exposure_daily.served_off + EXCLUDED.served_off,
        rule_hits = flag_exposure_daily.rule_hits + EXCLUDED.rule_hits`, envId, names, on, hit); err != nil {
			return err
		}
	}

	// ponytail: dedupe rows are only needed for today; prune on ~1 in 500 writes instead of running a scheduler
	if rand.IntN(500) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM public.environment_user_seen WHERE day < current_date - 1`); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
