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

// A task the user pinned with Keep Alive must never be reaped, even once it has
// reached a terminal status and aged well past the grace period.
//
// Regression test for the workspace of spt_01kz6r8evrdtpepqd59sjm0eev being
// os.RemoveAll'd 6h after its PR merged, while the container was still running
// and in use: the agent's shell broke (cwd deleted) and .claude-state went with
// it, so a later restart hit "Resource not found" on load_session and silently
// showed an empty "New Zed Agent Thread".
func TestLiveSpecTaskIDsForReaper_KeepAliveOutranksTerminalStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	now := time.Now()
	cutoff := now.Add(-6 * time.Hour)
	stale := now.Add(-30 * 24 * time.Hour) // long past any grace window

	mockStore.EXPECT().
		ListSpecTasks(gomock.Any(), gomock.Any()).
		Return([]*types.SpecTask{
			// The reported case: done, stale, but explicitly pinned.
			{ID: "spt_keepalive_done", Status: types.TaskStatusDone, KeepAlive: true, UpdatedAt: stale},
			// Pinned AND archived is still pinned — the flag is explicit intent.
			{ID: "spt_keepalive_archived", Status: types.TaskStatusDone, KeepAlive: true, Archived: true, UpdatedAt: stale},
			// Not pinned, terminal, stale -> reapable. This is what keeps the GC useful.
			{ID: "spt_done_stale", Status: types.TaskStatusDone, UpdatedAt: stale},
			// Not pinned but still working -> live on status alone.
			{ID: "spt_running", Status: types.TaskStatusImplementation, UpdatedAt: stale},
			// Not pinned, terminal, but touched inside the grace window -> live.
			{ID: "spt_done_recent", Status: types.TaskStatusDone, UpdatedAt: now},
		}, nil)

	ids, err := liveSpecTaskIDsForReaper(context.Background(), mockStore, cutoff)
	assert.NoError(t, err)

	assert.Contains(t, ids, "spt_keepalive_done",
		"Keep Alive must protect a done task's workspace — deleting it breaks the running agent's shell")
	assert.Contains(t, ids, "spt_keepalive_archived",
		"Keep Alive is explicit user intent and must outrank archived+terminal too")
	assert.Contains(t, ids, "spt_running", "a non-terminal task is live")
	assert.Contains(t, ids, "spt_done_recent", "a recently-touched task is live")
	assert.NotContains(t, ids, "spt_done_stale",
		"an unpinned, terminal, stale task must still be reapable, or the GC stops reclaiming anything")
}
