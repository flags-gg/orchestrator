package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/flags-gg/orchestrator/internal/integration"
	"github.com/flags-gg/orchestrator/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FLA-110: environments made before levels existed are all level 0. The migration renumbers each agent
// whose levels collide (its chain first, else local/dev → others → prod, then age), leaves distinct
// levels alone, and adds the one-per-level constraint that older databases never got.
func TestEnvironmentLevelsMigration(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)

	// an old database: no constraint, everything at level 0
	_, err := db.Exec(`ALTER TABLE public.environment DROP CONSTRAINT environment_agent_level_unique;
	-- agent-a (seeded with "dev"): named environments, no chain
	INSERT INTO public.environment (name, env_id, agent_id, level)
	SELECT n, e, a.id, 0 FROM public.agent a, (VALUES ('Production', 'a-prod'), ('Local', 'a-local'), ('Staging', 'a-staging')) v(n, e)
	WHERE a.agent_id = 'agent-a';
	-- agent-b (seeded with "dev"): a chain that disagrees with the names
	INSERT INTO public.environment (name, env_id, agent_id, level)
	SELECT n, e, a.id, 0 FROM public.agent a, (VALUES ('staging', 'b-staging'), ('local but last', 'b-last')) v(n, e)
	WHERE a.agent_id = 'agent-b';
	INSERT INTO public.environment_chain (agent_id, parent_environment_id, child_environment_id)
	SELECT a.id, p.id, c.id FROM public.agent a
	  JOIN public.environment p ON p.agent_id = a.id JOIN public.environment c ON c.agent_id = a.id
	WHERE a.agent_id = 'agent-b' AND ((p.env_id = 'env-b' AND c.env_id = 'b-staging') OR (p.env_id = 'b-staging' AND c.env_id = 'b-last'));
	-- agent-d: a chain loop (no first environment), which the chain walk never reaches
	INSERT INTO public.agent (name, project_id, agent_id) SELECT 'd', p.id, 'agent-d' FROM public.project p WHERE p.project_id = 'project-a';
	INSERT INTO public.environment (name, env_id, agent_id, level)
	SELECT n, e, a.id, 0 FROM public.agent a, (VALUES ('one', 'd-1'), ('two', 'd-2')) v(n, e) WHERE a.agent_id = 'agent-d';
	INSERT INTO public.environment_chain (agent_id, parent_environment_id, child_environment_id)
	SELECT a.id, p.id, c.id FROM public.agent a
	  JOIN public.environment p ON p.agent_id = a.id JOIN public.environment c ON c.agent_id = a.id
	WHERE a.agent_id = 'agent-d' AND p.id <> c.id;
	-- agent-c: levels already distinct, even if the names would sort them differently
	INSERT INTO public.agent (name, project_id, agent_id) SELECT 'c', p.id, 'agent-c' FROM public.project p WHERE p.project_id = 'project-a';
	INSERT INTO public.environment (name, env_id, agent_id, level)
	SELECT n, e, a.id, l FROM public.agent a, (VALUES ('prod', 'c-prod', 0), ('dev', 'c-dev', 5)) v(n, e, l)
	WHERE a.agent_id = 'agent-c';`)
	require.NoError(t, err)

	_, file, _, _ := runtime.Caller(0)
	up, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "k8s", "schema", "0010_environment_levels.up.sql"))
	require.NoError(t, err)
	for range 2 { // and again: nothing left to change, the constraint already there
		_, err = db.Exec(string(up))
		require.NoError(t, err)
	}

	levels := func(agent string) map[string]int {
		rows, err := db.Query(`SELECT e.env_id, e.level FROM public.environment e JOIN public.agent a ON a.id = e.agent_id WHERE a.agent_id = $1`, agent)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]int{}
		for rows.Next() {
			var id string
			var l int
			require.NoError(t, rows.Scan(&id, &l))
			out[id] = l
		}
		return out
	}
	assert.Equal(t, map[string]int{"env-a": 0, "a-local": 1, "a-staging": 2, "a-prod": 3}, levels("agent-a"), "local/dev first, prod last, ties by age")
	assert.Equal(t, map[string]int{"env-b": 0, "b-staging": 1, "b-last": 2}, levels("agent-b"), "the chain decides over names")
	assert.Equal(t, map[string]int{"c-prod": 0, "c-dev": 5}, levels("agent-c"), "distinct levels untouched")
	assert.Equal(t, map[string]int{"d-1": 0, "d-2": 1}, levels("agent-d"), "a loop still gets distinct levels")

	// the constraint is back: a second environment at an existing level is refused
	_, err = db.Exec(`INSERT INTO public.environment (name, env_id, agent_id, level) SELECT 'dup', 'a-dup', id, 0 FROM public.agent WHERE agent_id = 'agent-a'`)
	assert.Error(t, err)

	// the Tracklines integration now sees one production environment, the last one
	s, _ := newTestService(t, map[string]string{"user-a": "a@a.example"})
	tok, err := integration.NewSystem(s.Config).CreateToken(context.Background(), "company-a", "tracklines")
	require.NoError(t, err)
	w := do(s.routes(), http.MethodPut, "/integration/tickets/RB-2/flag", tok, `{"agent_id":"agent-a","name":"rb-2"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var tf integration.TicketFlag
	require.NoError(t, json.NewDecoder(w.Body).Decode(&tf))
	var prod []string
	for _, e := range tf.Environments {
		if e.Production {
			prod = append(prod, e.Name)
		}
	}
	assert.Equal(t, []string{"Production"}, prod)
	assert.Equal(t, "dev", tf.Environments[0].Name, "a new ticket flag starts in the first environment")
	assert.True(t, tf.Environments[0].Present)
}
