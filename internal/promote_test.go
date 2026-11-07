package internal

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/flags-gg/orchestrator/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FLA-111: the dashboard promotes to the chain's child where there is one, else the next level up, so
// environments that were never cloned (no chain link) can promote too. can_promote and promoted agree.
func TestDashboardPromoteFollowsChainElseLevel(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	_, err := db.Exec(`
	-- agent-a: dev (seeded, level 0) → staging → prod, by level only: never cloned, no chain
	INSERT INTO public.environment (name, env_id, agent_id, level)
	SELECT n, e, a.id, l FROM public.agent a, (VALUES ('staging', 'a-staging', 1), ('prod', 'a-prod', 2)) v(n, e, l)
	WHERE a.agent_id = 'agent-a';
	-- agent-b: dev → live by its chain, skipping qa (level 1)
	INSERT INTO public.environment (name, env_id, agent_id, level)
	SELECT n, e, a.id, l FROM public.agent a, (VALUES ('qa', 'b-qa', 1), ('live', 'b-live', 2)) v(n, e, l)
	WHERE a.agent_id = 'agent-b';
	INSERT INTO public.environment_chain (agent_id, parent_environment_id, child_environment_id)
	SELECT a.id, p.id, c.id FROM public.agent a
	  JOIN public.environment p ON p.agent_id = a.id AND p.env_id = 'env-b'
	  JOIN public.environment c ON c.agent_id = a.id AND c.env_id = 'b-live'
	WHERE a.agent_id = 'agent-b';`)
	require.NoError(t, err)

	s, a := newTestService(t, map[string]string{"user-a": "a@a.example", "user-b": "b@b.example"})
	h := s.routes()
	in := func(flag, env string) int {
		return count(t, db, `SELECT count(*) FROM public.flag f JOIN public.environment e ON e.id = f.environment_id WHERE f.name = $1 AND e.env_id = $2`, flag, env)
	}
	flagIn := func(flag, env string) string {
		var id string
		require.NoError(t, db.QueryRow(`SELECT f.id::text FROM public.flag f JOIN public.environment e ON e.id = f.environment_id WHERE f.name = $1 AND e.env_id = $2`, flag, env).Scan(&id))
		return id
	}
	canPromote := func(token, agent string) map[string]bool {
		w := do(h, http.MethodGet, "/agent/"+agent+"/environments", token, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var out struct {
			Environments []struct {
				EnvironmentId string `json:"environment_id"`
				CanPromote    bool   `json:"canPromote"`
			} `json:"environments"`
		}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&out))
		m := map[string]bool{}
		for _, e := range out.Environments {
			m[e.EnvironmentId] = e.CanPromote
		}
		return m
	}
	promote := func(token, flagID string) int {
		return do(h, http.MethodPost, "/flag/"+flagID+"/promote", token, "").Code
	}

	ua := a.token(t, "user-a")
	assert.Equal(t, map[string]bool{"env-a": true, "a-staging": true, "a-prod": false}, canPromote(ua, "agent-a"), "every environment but the last can promote")

	// by level: dev → staging → prod, then nowhere
	require.Equal(t, http.StatusOK, promote(ua, flagIn("flag-a", "env-a")))
	assert.Equal(t, 1, in("flag-a", "a-staging"))
	assert.Equal(t, 0, in("flag-a", "a-prod"), "one step at a time")
	require.Equal(t, http.StatusOK, promote(ua, flagIn("flag-a", "a-staging")))
	assert.Equal(t, 1, in("flag-a", "a-prod"))
	assert.Equal(t, http.StatusBadRequest, promote(ua, flagIn("flag-a", "a-prod")), "the last environment has nowhere to go")
	require.Equal(t, http.StatusOK, promote(ua, flagIn("flag-a", "env-a")), "promoting again is a no-op")
	assert.Equal(t, 1, in("flag-a", "a-staging"))

	// promoted: the flag is already in the next environment
	w := do(h, http.MethodGet, "/environment/env-a/flags", ua, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"promoted":true`)

	// by chain: dev → live, skipping qa's level
	ub := a.token(t, "user-b")
	assert.Equal(t, map[string]bool{"env-b": true, "b-qa": true, "b-live": false}, canPromote(ub, "agent-b"))
	require.Equal(t, http.StatusOK, promote(ub, flagIn("flag-b", "env-b")))
	assert.Equal(t, 1, in("flag-b", "b-live"))
	assert.Equal(t, 0, in("flag-b", "b-qa"))
}
