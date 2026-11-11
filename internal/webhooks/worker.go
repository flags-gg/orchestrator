package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/bugfixes/go-bugfixes/logs"
	"github.com/jackc/pgx/v5"
)

const maxAttempts = 8

var delays = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 4 * time.Hour, 12 * time.Hour, 24 * time.Hour}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(delays) {
		attempt = len(delays)
	}
	return delays[attempt-1]
}

type delivery struct {
	ID                   int64
	EventID, URL, Secret string
	Payload              json.RawMessage
	Attempts             int
}

// Run resumes committed deliveries after a restart. Leases and SKIP LOCKED coordinate API replicas.
func (s *System) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	cleanup := time.NewTicker(time.Minute)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanup.C:
			if err := s.prune(ctx); err != nil && ctx.Err() == nil {
				logs.Errorf("webhook retention: %v", err)
			}
		case <-ticker.C:
			// Bound each pass so shutdown and retention checks get serviced under sustained load.
			for i := 0; i < 10 && ctx.Err() == nil; i++ {
				worked, err := s.deliverOne(ctx)
				if err != nil {
					if ctx.Err() == nil {
						logs.Errorf("webhook worker: %v", err)
					}
					break
				}
				if !worked {
					break
				}
			}
		}
	}
}

func (s *System) claim(ctx context.Context) (*delivery, error) {
	conn, err := s.conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	d := &delivery{}
	err = conn.QueryRow(ctx, `WITH next AS (
 SELECT d.id FROM public.webhook_delivery d JOIN public.webhook_endpoint e ON e.id=d.endpoint_id
 WHERE e.enabled AND d.attempts < $1 AND d.next_attempt_at<=now() AND
 (d.status='pending' OR (d.status='delivering' AND d.locked_until<now()))
 ORDER BY d.next_attempt_at,d.id FOR UPDATE OF d SKIP LOCKED LIMIT 1
 ), claimed AS (
 UPDATE public.webhook_delivery d SET status='delivering', attempts=d.attempts+1,
 last_attempt_at=now(),locked_until=now()+interval '1 minute'
 FROM next WHERE d.id=next.id RETURNING d.*)
 SELECT d.id,d.event_id::text,e.url,e.secret,d.payload,d.attempts
 FROM claimed d JOIN public.webhook_endpoint e ON e.id=d.endpoint_id`, maxAttempts).Scan(&d.ID, &d.EventID, &d.URL, &d.Secret, &d.Payload, &d.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return d, err
}

func (s *System) deliverOne(ctx context.Context) (bool, error) {
	d, err := s.claim(ctx)
	if err != nil || d == nil {
		return false, err
	}
	status := 0
	reason := ""
	if _, err := validURL(d.URL); err != nil {
		reason = "Endpoint URL is invalid"
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL, bytes.NewReader(d.Payload))
		if err != nil {
			return true, err
		}
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "Flags.gg-Webhooks/1.0")
		req.Header.Set("X-Flags-Event-ID", d.EventID)
		req.Header.Set("X-Flags-Timestamp", stamp)
		req.Header.Set("X-Flags-Signature", Signature(d.Secret, stamp, d.Payload))
		response, err := s.Client.Do(req)
		if err != nil {
			reason = "Connection failed or timed out"
		} else {
			status = response.StatusCode
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			if status < 200 || status >= 300 {
				reason = fmt.Sprintf("Endpoint returned HTTP %d", status)
			}
		}
	}
	// A canceled HTTP service leaves its lease for another worker instead of marking it successful.
	if ctx.Err() != nil {
		return true, ctx.Err()
	}
	return true, s.finish(ctx, d, status, reason)
}

func (s *System) finish(ctx context.Context, d *delivery, status int, reason string) error {
	conn, err := s.conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	state := "pending"
	if reason == "" {
		state = "delivered"
	} else if d.Attempts >= maxAttempts {
		state = "failed"
	}
	_, err = conn.Exec(ctx, `UPDATE public.webhook_delivery SET status=$3, response_status=NULLIF($4,0),last_error=NULLIF($5,''),
 next_attempt_at=now()+make_interval(secs=>$6),locked_until=NULL,delivered_at=CASE WHEN $3='delivered' THEN now() ELSE NULL END
 WHERE id=$1 AND attempts=$2 AND status='delivering'`, d.ID, d.Attempts, state, status, reason, retryDelay(d.Attempts).Seconds())
	return err
}

func (s *System) prune(ctx context.Context) error {
	conn, err := s.conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	// A crash during the final attempt has exhausted the budget too. Surface it in the log for retry.
	_, err = conn.Exec(ctx, `UPDATE public.webhook_delivery SET status='failed',locked_until=NULL,last_error='Worker stopped during the final attempt' WHERE status='delivering' AND locked_until<now() AND attempts >= $1`, maxAttempts)
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, `DELETE FROM public.webhook_delivery WHERE id IN (SELECT id FROM public.webhook_delivery WHERE status IN ('delivered','failed') AND created_at < now()-interval '30 days' ORDER BY id LIMIT 1000)`)
	return err
}
