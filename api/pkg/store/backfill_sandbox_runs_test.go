package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// TestMigrationsSeedSandboxRunsFromEarlierSandboxes covers the one-time
// backfill: a sandbox that ran before the run ledger existed gets exactly one
// run, and running the migrations again adds nothing.
func TestMigrationsSeedSandboxRunsFromEarlierSandboxes(t *testing.T) {
	db := GetTestDB()
	ctx := context.Background()
	started := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	stopped := started.Add(30 * time.Minute)
	running := started.Add(time.Hour)
	stoppedID, runningID := system.GenerateUUID(), system.GenerateUUID()
	for _, sb := range []*types.Sandbox{
		{ID: stoppedID, OrganizationID: "org_backfill", Runtime: types.SandboxRuntimeHeadlessUbuntu,
			Status: types.SandboxStatusStopped, VCPUs: 2, StartedAt: &started, StoppedAt: &stopped},
		{ID: runningID, OrganizationID: "org_backfill", Runtime: types.SandboxRuntimeHeadlessUbuntu,
			Status: types.SandboxStatusRunning, VCPUs: 0, StartedAt: &running},
	} {
		require.NoError(t, db.gdb.WithContext(ctx).Create(sb).Error)
	}
	t.Cleanup(func() {
		db.gdb.Exec("DELETE FROM sandbox_runs WHERE sandbox_id IN (?, ?)", stoppedID, runningID)
		db.gdb.Exec("DELETE FROM sandboxes WHERE id IN (?, ?)", stoppedID, runningID)
	})

	for range 2 {
		require.NoError(t, db.runMigrations())
	}

	var runs []types.SandboxRun
	require.NoError(t, db.gdb.Where("sandbox_id IN (?, ?)", stoppedID, runningID).Order("started_at").Find(&runs).Error)
	require.Len(t, runs, 2)
	require.Equal(t, stoppedID, runs[0].SandboxID)
	require.Equal(t, 2, runs[0].VCPUs)
	require.NotNil(t, runs[0].StoppedAt)
	require.True(t, runs[0].StoppedAt.Equal(stopped))
	require.Equal(t, runningID, runs[1].SandboxID)
	require.Equal(t, 1, runs[1].VCPUs, "vCPUs are at least 1")
	require.Nil(t, runs[1].StoppedAt, "a running sandbox's run is still open")
}
