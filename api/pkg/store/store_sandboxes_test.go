package store

import (
	"context"
	"fmt"
	"testing"

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

	return &PostgresStore{gdb: db}
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
