package external_agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestReapOrphanResources_CallsRecentSandboxesWithCorrectRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExec := NewMockExecutor(ctrl)

	const grace = 6 * time.Hour
	now := time.Now()

	liveSessions := []string{"ses_live1", "ses_live2"}

	mockStore.EXPECT().
		ListExternalAgentSessionIDs(gomock.Any(), gomock.Any()).
		Return(liveSessions, nil)

	// Two spec tasks: one live (implementation), one terminal+old (done) → excluded.
	mockStore.EXPECT().
		ListSpecTasks(gomock.Any(), gomock.Any()).
		Return([]*types.SpecTask{
			{ID: "spt_live", Status: types.TaskStatusImplementation, UpdatedAt: now},
			{ID: "spt_done", Status: types.TaskStatusDone, UpdatedAt: now.Add(-30 * 24 * time.Hour)},
		}, nil)

	// One recent sandbox (reconciled) + one stale sandbox (skipped).
	mockStore.EXPECT().
		ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{
			{ID: "sbox_recent", LastSeen: now},
			{ID: "sbox_stale", LastSeen: now.Add(-24 * time.Hour)},
		}, nil)

	// Only the recent sandbox is reconciled, exactly once, with the correct request.
	mockExec.EXPECT().
		ReconcileSandboxResources(gomock.Any(), "sbox_recent", gomock.Any()).
		DoAndReturn(func(_ context.Context, sandboxID string, req *hydra.GCReconcileRequest) (*hydra.GCReconcileResponse, error) {
			assert.Equal(t, []string{"ses_live1", "ses_live2"}, req.LiveSessionIDs)
			assert.Equal(t, []string{"spt_live"}, req.LiveSpecTaskIDs)
			assert.Equal(t, int(grace.Seconds()), req.GracePeriodSeconds)
			assert.True(t, req.DryRun)
			return &hydra.GCReconcileResponse{ZvolsReaped: []string{"x"}, BytesFreed: 42}, nil
		})

	reapOrphanResources(context.Background(), mockExec, mockStore, grace, true /* dryRun */)
}

func TestReapOrphanResources_StoreErrorIsNotFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExec := NewMockExecutor(ctrl)

	// First store call fails → reaper bails early, no executor call, no panic.
	mockStore.EXPECT().
		ListExternalAgentSessionIDs(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("db down"))

	// No ListSpecTasks / ListSandboxInstances / ReconcileSandboxResources expected.
	assert.NotPanics(t, func() {
		reapOrphanResources(context.Background(), mockExec, mockStore, 6*time.Hour, false)
	})
}

func TestReapOrphanResources_SandboxReconcileErrorContinues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExec := NewMockExecutor(ctrl)

	now := time.Now()

	mockStore.EXPECT().ListExternalAgentSessionIDs(gomock.Any(), gomock.Any()).Return([]string{}, nil)
	mockStore.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{}, nil)
	mockStore.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{
		{ID: "sbox_a", LastSeen: now},
		{ID: "sbox_b", LastSeen: now},
	}, nil)

	// First sandbox errors; reaper must continue to the second.
	mockExec.EXPECT().ReconcileSandboxResources(gomock.Any(), "sbox_a", gomock.Any()).
		Return(nil, errors.New("revdial down"))
	mockExec.EXPECT().ReconcileSandboxResources(gomock.Any(), "sbox_b", gomock.Any()).
		Return(&hydra.GCReconcileResponse{}, nil)

	assert.NotPanics(t, func() {
		reapOrphanResources(context.Background(), mockExec, mockStore, 6*time.Hour, false)
	})
}
