package agent

import (
	"context"
	"testing"

	"github.com/flags-gg/orchestrator/internal/testdb"
	ConfigBuilder "github.com/keloran/go-config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent's request limit is the plan's requests, not its agent count.
func TestAgentLimitsComeFromPlan(t *testing.T) {
	db := testdb.Start(t)
	testdb.Seed(t, db)
	_, err := db.Exec(`UPDATE public.payment_plans SET agents = 3, environments = 7, requests = 123456 WHERE id = 1`)
	require.NoError(t, err)

	c := ConfigBuilder.NewConfigNoVault()
	require.NoError(t, c.Build(ConfigBuilder.Database, ConfigBuilder.Bugfixes))
	s := NewSystem(c)
	ctx := context.Background()

	a, err := s.GetAgentDetails(ctx, "agent-a", "company-a")
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, 123456, a.RequestLimit)
	assert.Equal(t, 7, a.EnvironmentLimit)

	agents, err := s.GetAgents(ctx, "company-a")
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, 123456, agents[0].RequestLimit)
	assert.Equal(t, 7, agents[0].EnvironmentLimit)
}
