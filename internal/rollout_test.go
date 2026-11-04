package internal

import (
	"net/http"
	"testing"

	"github.com/flags-gg/orchestrator/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlagRollout(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-a": "a@a.example", "user-b": "b@b.example"})
	h := s.routes()
	flagA := testdb.FlagID(t, db, "a")
	ua := a.token(t, "user-a")
	_, err := db.Exec(`UPDATE public.flag SET enabled = true WHERE id::text = $1`, flagA)
	require.NoError(t, err)

	// validation and ownership
	assert.Equal(t, http.StatusBadRequest, do(h, http.MethodPatch, "/flag/"+flagA+"/rollout", ua, `{"rollout":101}`).Code)
	assert.Equal(t, http.StatusBadRequest, do(h, http.MethodPatch, "/flag/"+flagA+"/rollout", ua, `{}`).Code)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodPatch, "/flag/"+flagA+"/rollout", a.token(t, "user-b"), `{"rollout":5}`).Code)
	require.Equal(t, http.StatusOK, do(h, http.MethodPatch, "/flag/"+flagA+"/rollout", ua, `{"rollout":25}`).Code)

	sdk := func(caps ...string) string {
		hdr := []string{"x-project-id", "project-a", "x-agent-id", "agent-a", "x-environment-id", "env-a"}
		if len(caps) > 0 {
			hdr = append(hdr, "x-flags-capabilities", caps[0])
		}
		w := do(h, http.MethodGet, "/flags", "", "", hdr...)
		require.Equal(t, http.StatusOK, w.Code)
		return w.Body.String()
	}
	// older SDKs would show a 25% flag to everyone, so they get it disabled
	assert.Contains(t, sdk(), `"enabled":false`)
	assert.NotContains(t, sdk(), `"rollout"`)
	// rollout-aware SDKs get the percentage to evaluate themselves
	assert.Contains(t, sdk("rollout"), `"enabled":true`)
	assert.Contains(t, sdk("rollout"), `"rollout":25`)

	// OFREP evaluates server-side: user-1 is bucket 63, so out at 25%
	w := do(h, http.MethodPost, "/ofrep/v1/evaluate/flags/flag-a", "", `{"context":{"targetingKey":"user-1"}}`,
		"x-project-id", "project-a", "x-agent-id", "agent-a", "x-environment-id", "env-a", "Content-Type", "application/json")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"reason":"SPLIT"`)
	assert.Contains(t, w.Body.String(), `"value":false`)

	// dashboard view shows it
	assert.Contains(t, do(h, http.MethodGet, "/agent/agent-a/flags", ua, "").Body.String(), `"rollout":25`)
}
