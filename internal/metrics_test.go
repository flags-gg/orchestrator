package internal

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/flags-gg/orchestrator/internal/integration"
	"github.com/flags-gg/orchestrator/internal/stats"
	"github.com/flags-gg/orchestrator/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagMetrics(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-a": "a@a.example", "user-b": "b@b.example"})
	h := s.routes()
	ua := a.token(t, "user-a")
	flagA := testdb.FlagID(t, db, "a")

	// flag-a: on, 0% rollout, but beta testers get it
	_, err := db.Exec(`UPDATE public.flag SET enabled = true, rollout = 0,
		rules = '[{"attribute":"groups","operator":"in","values":["beta"],"serve":true}]' WHERE id::text = $1`, flagA)
	require.NoError(t, err)

	poll := func(ctx string) {
		hdr := []string{"x-project-id", "project-a", "x-agent-id", "agent-a", "x-environment-id", "env-a", "x-flags-capabilities", "rollout"}
		if ctx != "" {
			hdr = append(hdr, "x-flags-context", base64.StdEncoding.EncodeToString([]byte(ctx)))
		}
		require.Equal(t, http.StatusOK, do(h, http.MethodGet, "/flags", "", "", hdr...).Code)
	}
	poll(`{"key":"u1","groups":["beta"]}`)
	poll(`{"key":"u1","groups":["beta"]}`) // same user again today: counted once
	poll(`{"key":"u2"}`)
	poll(`{"key":"u3"}`)
	poll("") // an older SDK without a key: a request, not a user

	w := do(h, http.MethodGet, "/agent/agent-a/metrics?days=7", ua, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var m stats.AgentMetrics
	require.NoError(t, json.NewDecoder(w.Body).Decode(&m))
	require.Len(t, m.Days, 7)
	today := len(m.Days) - 1

	require.Len(t, m.Environments, 1)
	env := m.Environments[0]
	assert.Equal(t, 3, env.Users[today])
	assert.Equal(t, 5, env.Requests[today])
	assert.Greater(t, env.P95Ms, 0)

	require.Len(t, m.Flags, 1)
	f := m.Flags[0]
	assert.Equal(t, "flag-a", f.Name)
	assert.Equal(t, 1, f.ServedOn[today], "only the beta tester")
	assert.Equal(t, 2, f.ServedOff[today])
	assert.Equal(t, 1, f.RuleHits[today])

	// the flags view shows the 7-day totals under each state
	w = do(h, http.MethodGet, "/agent/agent-a/flags", ua, "")
	var af integration.AgentFlags
	require.NoError(t, json.NewDecoder(w.Body).Decode(&af))
	assert.Equal(t, 1, af.Flags[0].States[0].ServedOn)
	assert.Equal(t, 2, af.Flags[0].States[0].ServedOff)

	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, "/agent/agent-a/metrics", a.token(t, "user-b"), "").Code)
	assert.Equal(t, http.StatusBadRequest, do(h, http.MethodGet, "/agent/agent-a/metrics?days=0", ua, "").Code)
}
