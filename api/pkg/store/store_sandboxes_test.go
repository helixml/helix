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

func newSandboxTestStore(t *testing.T) *PostgresStore {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Sandbox{}))

	return &PostgresStore{gdb: db, Store: orgstore.New(db)}
}

func TestSetSandboxContainerIgnoresDeletedRows(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_deleted_container",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Status:         types.SandboxStatusPending,
	})
	require.NoError(t, err)
	require.NoError(t, store.DeleteSandbox(ctx, sb.ID))

	err = store.SetSandboxContainer(ctx, sb.ID, "host-a", "ctr-a")
	require.ErrorIs(t, err, ErrNotFound)

	var persisted types.Sandbox
	require.NoError(t, store.gdb.WithContext(ctx).Unscoped().Where("id = ?", sb.ID).First(&persisted).Error)
	require.Empty(t, persisted.HostDeviceID)
	require.Empty(t, persisted.ContainerID)
}

func TestSetSandboxStatusIgnoresDeletedRows(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_deleted_status",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Status:         types.SandboxStatusPending,
	})
	require.NoError(t, err)
	require.NoError(t, store.DeleteSandbox(ctx, sb.ID))

	err = store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, "")
	require.ErrorIs(t, err, ErrNotFound)

	var persisted types.Sandbox
	require.NoError(t, store.gdb.WithContext(ctx).Unscoped().Where("id = ?", sb.ID).First(&persisted).Error)
	require.Equal(t, types.SandboxStatusStopped, persisted.Status)
	require.Nil(t, persisted.StartedAt)
}

// Regression: the column behind types.Sandbox.VCPUs is `v_cpus`, not `vcpus` —
// GORM's naming strategy splits the leading acronym. A raw-map update with the
// wrong key fails at the database, which mock-store unit tests cannot see.
func TestSetSandboxResourcesPersistsNewAllocation(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_resize",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
		VCPUs:          1,
		MemoryMB:       2048,
	})
	require.NoError(t, err)

	require.NoError(t, store.SetSandboxResources(ctx, sb.ID, 8, 16384))

	updated, err := store.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.Equal(t, 8, updated.VCPUs)
	require.Equal(t, 16384, updated.MemoryMB)
}

func TestSetSandboxResourcesRejectsNonPositiveValues(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	_, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_resize_bad",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
	})
	require.NoError(t, err)

	require.Error(t, store.SetSandboxResources(ctx, "sbx_resize_bad", 0, 8192))
	require.ErrorIs(t, store.SetSandboxResources(ctx, "sbx_missing", 4, 8192), ErrNotFound)
}

func TestGetSandboxBySessionSkipsDeletedRows(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_session",
		OrganizationID: "org_1",
		Owner:          "user_1",
		SessionID:      "ses_1",
		SpecTaskID:     "spt_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
	})
	require.NoError(t, err)

	found, err := store.GetSandboxBySession(ctx, "ses_1")
	require.NoError(t, err)
	require.Equal(t, sb.ID, found.ID)
	require.True(t, found.SessionBacked())
	require.Equal(t, "ses_1", found.HydraOpsID())

	// After teardown the session must look unmetered again, so a resumed task
	// opens a fresh row rather than reusing a deleted one.
	require.NoError(t, store.DeleteSandbox(ctx, sb.ID))
	_, err = store.GetSandboxBySession(ctx, "ses_1")
	require.ErrorIs(t, err, ErrNotFound)
}

// Session-backed rows are created with a negative TTL so expires_at stays NULL
// and the TTL reaper never tears down a desktop the task still owns.
func TestNegativeTimeoutLeavesSandboxOutOfTheExpiryReaper(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_never_expires",
		OrganizationID: "org_1",
		Owner:          "user_1",
		SessionID:      "ses_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
		TimeoutSeconds: -1,
	})
	require.NoError(t, err)
	require.Nil(t, sb.ExpiresAt)

	expired, err := store.ListExpiredSandboxes(ctx, time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, expired)
}
