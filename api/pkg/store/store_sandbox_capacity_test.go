package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/orgstore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSandboxInstanceTestStore(t *testing.T) *PostgresStore {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.SandboxInstance{}))

	return &PostgresStore{gdb: db, Store: orgstore.New(db)}
}

func TestSandboxInstanceDefaultsToUnlimited(t *testing.T) {
	ctx := context.Background()
	store := newSandboxInstanceTestStore(t)
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "default", Status: "online"}))

	instance, err := store.GetSandboxInstance(ctx, "default")
	require.NoError(t, err)
	require.Equal(t, -1, instance.MaxSandboxes)
}

func TestHeartbeatPersistsRunnerReportedMaxSandboxes(t *testing.T) {
	ctx := context.Background()
	store := newSandboxInstanceTestStore(t)
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "reported", MaxSandboxes: 20, Status: "online"}))
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "unlimited", MaxSandboxes: 20, Status: "online"}))
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "silent", MaxSandboxes: 20, Status: "online"}))
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "legacy", MaxSandboxes: 20, Status: "online"}))

	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "reported", &types.SandboxHeartbeatRequest{MaxSandboxes: 1}, 20))
	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "unlimited", &types.SandboxHeartbeatRequest{MaxSandboxes: -1}, 20))
	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "silent", &types.SandboxHeartbeatRequest{}, -1))

	reported, err := store.GetSandboxInstance(ctx, "reported")
	require.NoError(t, err)
	require.Equal(t, 1, reported.MaxSandboxes)
	require.True(t, reported.MaxSandboxesFromRunner)
	unlimited, err := store.GetSandboxInstance(ctx, "unlimited")
	require.NoError(t, err)
	require.Equal(t, -1, unlimited.MaxSandboxes)
	require.True(t, unlimited.MaxSandboxesFromRunner)

	silent, err := store.GetSandboxInstance(ctx, "silent")
	require.NoError(t, err)
	require.Equal(t, -1, silent.MaxSandboxes)
	require.False(t, silent.MaxSandboxesFromRunner)

	// Boot-time backfill removes legacy fallback caps without changing
	// runner-reported values.
	rows, err := store.BackfillSandboxMaxSandboxes(ctx, -1)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	reported, err = store.GetSandboxInstance(ctx, "reported")
	require.NoError(t, err)
	require.Equal(t, 1, reported.MaxSandboxes)
	unlimited, err = store.GetSandboxInstance(ctx, "unlimited")
	require.NoError(t, err)
	require.Equal(t, -1, unlimited.MaxSandboxes)

	silent, err = store.GetSandboxInstance(ctx, "silent")
	require.NoError(t, err)
	require.Equal(t, -1, silent.MaxSandboxes)
	legacy, err := store.GetSandboxInstance(ctx, "legacy")
	require.NoError(t, err)
	require.Equal(t, -1, legacy.MaxSandboxes)

	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "reported", &types.SandboxHeartbeatRequest{}, 7))
	reported, err = store.GetSandboxInstance(ctx, "reported")
	require.NoError(t, err)
	require.Equal(t, 7, reported.MaxSandboxes)
	require.False(t, reported.MaxSandboxesFromRunner)

	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "unlimited", &types.SandboxHeartbeatRequest{}, 7))
	unlimited, err = store.GetSandboxInstance(ctx, "unlimited")
	require.NoError(t, err)
	require.Equal(t, 7, unlimited.MaxSandboxes)
	require.False(t, unlimited.MaxSandboxesFromRunner)
}

func TestHeartbeatRejectsInvalidGlobalMaxSandboxes(t *testing.T) {
	ctx := context.Background()
	store := newSandboxInstanceTestStore(t)
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "silent", MaxSandboxes: 20, Status: "online"}))

	err := store.UpdateSandboxHeartbeat(ctx, "silent", &types.SandboxHeartbeatRequest{}, 0)
	require.ErrorContains(t, err, "refusing invalid default max_sandboxes")

	err = store.UpdateSandboxHeartbeat(ctx, "silent", &types.SandboxHeartbeatRequest{}, -2)
	require.ErrorContains(t, err, "refusing invalid default max_sandboxes")

	err = store.UpdateSandboxHeartbeat(ctx, "silent", &types.SandboxHeartbeatRequest{MaxSandboxes: -2}, 20)
	require.ErrorContains(t, err, "refusing invalid max_sandboxes")

	for _, invalid := range []int{0, -2} {
		_, err = store.BackfillSandboxMaxSandboxes(ctx, invalid)
		require.ErrorContains(t, err, "refusing to write invalid value")
	}
}

func TestFindAvailableSandboxInstanceSkipsFullRunners(t *testing.T) {
	ctx := context.Background()
	store := newSandboxInstanceTestStore(t)

	register := func(id string, active, max int) {
		require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{
			ID:              id,
			Status:          "online",
			LastSeen:        time.Now(),
			ActiveSandboxes: active,
			MaxSandboxes:    max,
			GPUVendor:       "none",
			RenderNode:      "SOFTWARE",
			DesktopVersions: []byte(`{"ubuntu":"abc123"}`),
		}))
	}

	// "full" sorts first (fewer active) but is at its cap of 1.
	register("full", 1, 1)
	register("roomy", 2, 3)
	register("unlimited", 100, -1)

	host, err := store.FindAvailableSandboxInstance(ctx, "ubuntu", false)
	require.NoError(t, err)
	require.NotNil(t, host)
	require.Equal(t, "roomy", host.ID)

	// Once finite runners fill up, the busy unlimited runner still accepts work.
	require.NoError(t, store.SetSandboxContainerCount(ctx, "roomy", 3))
	host, err = store.FindAvailableSandboxInstance(ctx, "ubuntu", false)
	require.NoError(t, err)
	require.NotNil(t, host)
	require.Equal(t, "unlimited", host.ID)

	// Remove the unlimited runner's matching image so only full runners remain.
	require.NoError(t, store.gdb.WithContext(ctx).Model(&types.SandboxInstance{}).Where("id = ?", "unlimited").Update("desktop_versions", []byte(`{}`)).Error)
	host, err = store.FindAvailableSandboxInstance(ctx, "ubuntu", false)
	require.ErrorIs(t, err, ErrAllSandboxHostsFull)
	require.Nil(t, host)

	// A runner that doesn't advertise the image is "no runner", not "full".
	host, err = store.FindAvailableSandboxInstance(ctx, "sway", false)
	require.NoError(t, err)
	require.Nil(t, host)
}
