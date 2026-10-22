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

func TestHeartbeatPersistsRunnerReportedMaxSandboxes(t *testing.T) {
	ctx := context.Background()
	store := newSandboxInstanceTestStore(t)
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "reported", MaxSandboxes: 20, Status: "online"}))
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "silent", MaxSandboxes: 20, Status: "online"}))

	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "reported", &types.SandboxHeartbeatRequest{MaxSandboxes: 1}, 20))
	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "silent", &types.SandboxHeartbeatRequest{}, 20))

	reported, err := store.GetSandboxInstance(ctx, "reported")
	require.NoError(t, err)
	require.Equal(t, 1, reported.MaxSandboxes)
	require.True(t, reported.MaxSandboxesFromRunner)

	silent, err := store.GetSandboxInstance(ctx, "silent")
	require.NoError(t, err)
	require.Equal(t, 20, silent.MaxSandboxes)
	require.False(t, silent.MaxSandboxesFromRunner)

	// Boot-time backfill must only touch rows that didn't report a cap.
	rows, err := store.BackfillSandboxMaxSandboxes(ctx, 5)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	reported, err = store.GetSandboxInstance(ctx, "reported")
	require.NoError(t, err)
	require.Equal(t, 1, reported.MaxSandboxes)

	silent, err = store.GetSandboxInstance(ctx, "silent")
	require.NoError(t, err)
	require.Equal(t, 5, silent.MaxSandboxes)

	require.NoError(t, store.UpdateSandboxHeartbeat(ctx, "reported", &types.SandboxHeartbeatRequest{}, 7))
	reported, err = store.GetSandboxInstance(ctx, "reported")
	require.NoError(t, err)
	require.Equal(t, 7, reported.MaxSandboxes)
	require.False(t, reported.MaxSandboxesFromRunner)
}

func TestHeartbeatRejectsInvalidGlobalMaxSandboxes(t *testing.T) {
	ctx := context.Background()
	store := newSandboxInstanceTestStore(t)
	require.NoError(t, store.RegisterSandboxInstance(ctx, &types.SandboxInstance{ID: "silent", MaxSandboxes: 20, Status: "online"}))

	err := store.UpdateSandboxHeartbeat(ctx, "silent", &types.SandboxHeartbeatRequest{}, 0)
	require.ErrorContains(t, err, "refusing non-positive default max_sandboxes")
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

	host, err := store.FindAvailableSandboxInstance(ctx, "ubuntu", false)
	require.NoError(t, err)
	require.NotNil(t, host)
	require.Equal(t, "roomy", host.ID)

	// Fill the other runner too: placement must report capacity, not "no runner".
	require.NoError(t, store.SetSandboxContainerCount(ctx, "roomy", 3))
	host, err = store.FindAvailableSandboxInstance(ctx, "ubuntu", false)
	require.ErrorIs(t, err, ErrAllSandboxHostsFull)
	require.Nil(t, host)

	// A runner that doesn't advertise the image is "no runner", not "full".
	host, err = store.FindAvailableSandboxInstance(ctx, "sway", false)
	require.NoError(t, err)
	require.Nil(t, host)
}
