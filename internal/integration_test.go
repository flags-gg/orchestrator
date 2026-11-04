package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flags-gg/orchestrator/internal/integration"
	"github.com/flags-gg/orchestrator/internal/testdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ticket lifecycle Tracklines drives: on board -> dev (off), toggle, testing -> staging, dev complete -> production, archive.
func TestTicketFlagLifecycle(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	_, err := db.Exec(`
	INSERT INTO public.environment (name, env_id, agent_id, level)
	SELECT n, e, a.id, l FROM public.agent a, (VALUES ('staging', 'env-a-staging', 1), ('prod', 'env-a-prod', 2)) v(n, e, l)
	WHERE a.agent_id = 'agent-a'`)
	require.NoError(t, err)

	s, a := newTestService(t, map[string]string{"user-a": "a@a.example"})
	h := s.routes()
	ctx := context.Background()
	tokA, err := integration.NewSystem(s.Config).CreateToken(ctx, "company-a", "tracklines")
	require.NoError(t, err)
	tokB, err := integration.NewSystem(s.Config).CreateToken(ctx, "company-b", "tracklines")
	require.NoError(t, err)

	state := func(w interface{ Result() *http.Response }) map[string][2]bool {
		var tf integration.TicketFlag
		require.NoError(t, json.NewDecoder(w.Result().Body).Decode(&tf))
		out := map[string][2]bool{} // env -> {present, enabled}
		for _, e := range tf.Environments {
			out[e.Name] = [2]bool{e.Present, e.Enabled}
		}
		return out
	}
	const flag = "/integration/tickets/TL-1/flag"

	// auth: missing, unknown, and a dashboard Clerk session are all rejected
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, flag, "", "").Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, flag, "flg_nope", "").Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, flag, a.token(t, "user-a"), "").Code)

	// on board: created off in dev only; retry is a 200 no-op
	w := do(h, http.MethodPut, flag, tokA, `{"agent_id":"agent-a","name":"checkout-v2"}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, map[string][2]bool{"dev": {true, false}, "staging": {false, false}, "prod": {false, false}}, state(w))
	assert.Equal(t, http.StatusOK, do(h, http.MethodPut, flag, tokA, `{"agent_id":"agent-a","name":"checkout-v2"}`).Code)

	// other company sees nothing and cannot use our agent
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, flag, tokB, "").Code)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodPut, "/integration/tickets/TL-9/flag", tokB, `{"agent_id":"agent-a","name":"x"}`).Code)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodPatch, flag, tokB, `{"environment_id":"env-a","enabled":true}`).Code)

	// a name already used in dev is a conflict, not a silent adopt
	assert.Equal(t, http.StatusConflict, do(h, http.MethodPut, "/integration/tickets/TL-2/flag", tokA, `{"agent_id":"agent-a","name":"flag-a"}`).Code)

	// in progress: toggle in dev
	w = do(h, http.MethodPatch, flag, tokA, `{"environment_id":"env-a","enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, [2]bool{true, true}, state(w)["dev"])
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodPatch, flag, tokA, `{"environment_id":"env-a-prod","enabled":true}`).Code)

	// testing: one step to staging, arrives off
	w = do(h, http.MethodPost, flag+"/promote", tokA, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, map[string][2]bool{"dev": {true, true}, "staging": {true, false}, "prod": {false, false}}, state(w))

	// dev complete: production, off; again is a no-op
	for range 2 {
		w = do(h, http.MethodPost, flag+"/promote?to=production", tokA, "")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, [2]bool{true, false}, state(w)["prod"])
	}

	// SDK path still sees it as an ordinary flag
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM public.flag f JOIN public.environment e ON e.id = f.environment_id WHERE e.env_id = 'env-a-prod' AND f.name = 'checkout-v2'`))

	// decided: archived everywhere, retry is fine
	assert.Equal(t, http.StatusNoContent, do(h, http.MethodDelete, flag, tokA, "").Code)
	assert.Equal(t, http.StatusNoContent, do(h, http.MethodDelete, flag, tokA, "").Code)
	assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM public.flag WHERE ticket_ref = 'TL-1'`))
	assert.Equal(t, 3, count(t, db, `SELECT count(*) FROM public.flag_archive WHERE ticket_ref = 'TL-1'`))
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, flag, tokA, "").Code)
}

func TestIntegrationTokens(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-a": "a@a.example", "user-b": "b@b.example"})
	h := s.routes()
	ua, ub := a.token(t, "user-a"), a.token(t, "user-b")

	w := do(h, http.MethodPost, "/company/integration-token", ua, `{"name":"tracklines"}`)
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct{ Token string }
	require.NoError(t, json.NewDecoder(w.Body).Decode(&created))
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, "/integration/tickets/TL-1/flag", created.Token, "").Code) // valid token, no flag

	assert.Equal(t, 0, count(t, db, `SELECT count(*) FROM public.integration_token WHERE token_hash = $1`, created.Token), "raw token must not be stored")

	var id string
	require.NoError(t, db.QueryRow(`SELECT id::text FROM public.integration_token`).Scan(&id))
	assert.Contains(t, do(h, http.MethodGet, "/company/integration-tokens", ua, "").Body.String(), `"tracklines"`)
	assert.NotContains(t, do(h, http.MethodGet, "/company/integration-tokens", ub, "").Body.String(), `"tracklines"`)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodDelete, "/company/integration-token/"+id, ub, "").Code)
	assert.Equal(t, http.StatusNoContent, do(h, http.MethodDelete, "/company/integration-token/"+id, ua, "").Code)
	assert.Equal(t, http.StatusUnauthorized, do(h, http.MethodGet, "/integration/tickets/TL-1/flag", created.Token, "").Code)
}

func TestMCPAndAgentFlags(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	s, a := newTestService(t, map[string]string{"user-a": "a@a.example", "user-b": "b@b.example"})
	h := s.routes()
	tok, err := integration.NewSystem(s.Config).CreateToken(context.Background(), "company-a", "agent")
	require.NoError(t, err)

	rpc := func(token, body string) *httptest.ResponseRecorder {
		return do(h, http.MethodPost, "/mcp", token, body, "Content-Type", "application/json", "Accept", "application/json, text/event-stream")
	}
	call := func(name, args string) string {
		w := rpc(tok, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return w.Body.String()
	}

	assert.Equal(t, http.StatusUnauthorized, rpc("", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`).Code)
	assert.Equal(t, http.StatusUnauthorized, rpc(a.token(t, "user-a"), `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`).Code)

	w := rpc(tok, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, tool := range []string{"list_agents", "list_flags", "get_flag", "create_flag", "set_flag", "promote_flag", "archive_flag"} {
		assert.Contains(t, w.Body.String(), `"`+tool+`"`)
	}

	assert.Contains(t, call("list_agents", `{}`), `agent-a`)
	w = do(h, http.MethodGet, "/integration/agents", tok, "")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"agent_id":"agent-a"`)
	assert.NotContains(t, w.Body.String(), `agent-b`)
	assert.NotContains(t, call("list_agents", `{}`), `agent-b`)
	assert.Contains(t, call("create_flag", `{"ticket":"TL-7","agent_id":"agent-a","name":"new-nav"}`), `"present":true`)
	assert.Contains(t, call("create_flag", `{"ticket":"TL-8","agent_id":"agent-b","name":"x"}`), `"isError":true`)
	assert.Contains(t, call("set_flag", `{"ticket":"TL-7","environment_id":"env-a","enabled":true}`), `"enabled":true`)
	assert.Contains(t, call("list_flags", `{"agent_id":"agent-a"}`), `"ticket":"TL-7"`)
	assert.Contains(t, call("archive_flag", `{"ticket":"TL-7"}`), `"archived":true`)
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM public.flag_archive WHERE ticket_ref = 'TL-7'`))

	// dashboard flag-first view: grouped by name, scoped to the caller's company
	w = do(h, http.MethodGet, "/agent/agent-a/flags", a.token(t, "user-a"), "")
	require.Equal(t, http.StatusOK, w.Code)
	var af integration.AgentFlags
	require.NoError(t, json.NewDecoder(w.Body).Decode(&af))
	require.Len(t, af.Flags, 1)
	assert.Equal(t, "flag-a", af.Flags[0].Name)
	assert.Equal(t, "env-a", af.Flags[0].States[0].EnvironmentId)
	assert.True(t, af.Environments[0].Production)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodGet, "/agent/agent-a/flags", a.token(t, "user-b"), "").Code)

	// remove by name archives every environment's row; other companies and unknown names 404
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodDelete, "/agent/agent-a/flags/flag-a", a.token(t, "user-b"), "").Code)
	assert.Equal(t, http.StatusNotFound, do(h, http.MethodDelete, "/agent/agent-a/flags/nope", a.token(t, "user-a"), "").Code)
	assert.Equal(t, http.StatusNoContent, do(h, http.MethodDelete, "/agent/agent-a/flags/flag-a", a.token(t, "user-a"), "").Code)
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM public.flag_archive WHERE name = 'flag-a'`))
	assert.Equal(t, 1, count(t, db, `SELECT count(*) FROM public.flag WHERE name = 'flag-b'`))
}
