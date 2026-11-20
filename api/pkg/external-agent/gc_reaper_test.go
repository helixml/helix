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
	mockStore.EXPECT().
		ListActiveDesktopSessionIDs(gomock.Any()).
		Return([]string{"ses_live1"}, nil)

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
	mockStore.EXPECT().ListActiveDesktopSessionIDs(gomock.Any()).Return([]string{}, nil)
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
		}, nil)

	ids, err := liveSpecTaskIDsForReaper(context.Background(), mockStore, nil)
	assert.NoError(t, err)

	assert.Contains(t, ids, "spt_keepalive_done",
		"Keep Alive must protect a done task's workspace — deleting it breaks the running agent's shell")
	assert.Contains(t, ids, "spt_keepalive_archived",
		"Keep Alive is explicit user intent and must outrank archived+terminal too")
	assert.Contains(t, ids, "spt_running", "a non-terminal task is live")
	assert.NotContains(t, ids, "spt_done_stale",
		"an unpinned, terminal, stale task must still be reapable, or the GC stops reclaiming anything")
}

// A done or archived task whose desktop is still running keeps its workspace:
// it is the agent's cwd and holds its transcript. Liveness used to come only
// from the task's own status and timestamp, so a task finished more than the
// grace period ago lost its workspace under a running agent.
func TestLiveSpecTaskIDsForReaper_LiveSessionKeepsFinishedTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	now := time.Now()
	stale := now.Add(-60 * 24 * time.Hour)
	mockStore.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{
		{ID: "spt_done_in_use", Status: types.TaskStatusDone, PlanningSessionID: "ses_running", UpdatedAt: stale},
		{ID: "spt_archived_in_use", Status: types.TaskStatusDone, Archived: true, PlanningSessionID: "ses_running_2", UpdatedAt: stale},
		{ID: "spt_done_idle", Status: types.TaskStatusDone, PlanningSessionID: "ses_gone", UpdatedAt: stale},
	}, nil)

	ids, err := liveSpecTaskIDsForReaper(context.Background(), mockStore, []string{"ses_running", "ses_running_2"})
	assert.NoError(t, err)

	assert.ElementsMatch(t, []string{"spt_done_in_use", "spt_archived_in_use"}, ids)
}

// Regression: a finished or archived task's workspace is retained one grace
// period after it stopped being live, not two.
//
// The live set used to include every task updated within the grace window, and
// hydra writes a liveness mark for each id on every tick it is live, then waits
// a full grace from the last mark. So a task archived at T was marked live
// until T+grace and reaped at T+2*grace: ~48h with a 24h grace, ~60 days with
// the 720h default. Recency now comes only from hydra's mark (or, for an id it
// never saw live, the workspace's own mtimes), so these tasks must be absent.
func TestLiveSpecTaskIDsForReaper_RecentlyFinishedTaskIsNotPaddedLive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	now := time.Now()
	mockStore.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{
		// Archived the moment it finished; its desktop was stopped (or
		// destroyed) by the archive, so its session is recent but not active.
		{ID: "spt_archived_now", Status: types.TaskStatusDone, Archived: true, PlanningSessionID: "ses_just_stopped", UpdatedAt: now},
		// Finished an hour ago, never archived, desktop stopped.
		{ID: "spt_done_1h", Status: types.TaskStatusDone, PlanningSessionID: "ses_stopped", UpdatedAt: now.Add(-time.Hour)},
		// Archived mid-flight: archived alone ends liveness too.
		{ID: "spt_archived_in_review", Status: types.TaskStatusSpecReview, Archived: true, UpdatedAt: now},
		{ID: "spt_failed_now", Status: types.TaskStatusImplementationFailed, UpdatedAt: now},
		// Controls: still working, and finished but with a desktop booting.
		{ID: "spt_working", Status: types.TaskStatusImplementation, UpdatedAt: now},
		{ID: "spt_done_restarting", Status: types.TaskStatusDone, PlanningSessionID: "ses_restarting", UpdatedAt: now.Add(-90 * 24 * time.Hour)},
	}, nil)

	ids, err := liveSpecTaskIDsForReaper(context.Background(), mockStore, []string{"ses_restarting"})
	assert.NoError(t, err)

	assert.ElementsMatch(t, []string{"spt_working", "spt_done_restarting"}, ids,
		"a terminal or archived task is live only while pinned or while its desktop is active; recency is hydra's job")
}

// The reaper hands liveSpecTaskIDsForReaper the active-desktop set, not the
// recency-padded session set it sends hydra for session resources: a stopped
// desktop whose session row changed recently must not keep its task live.
func TestReapOrphanResources_TaskLivenessUsesActiveDesktopsOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockExec := NewMockExecutor(ctrl)
	now := time.Now()

	// ses_stopped is in the padded live set (recently updated) but not active.
	mockStore.EXPECT().ListExternalAgentSessionIDs(gomock.Any(), gomock.Any()).
		Return([]string{"ses_stopped", "ses_running"}, nil)
	mockStore.EXPECT().ListActiveDesktopSessionIDs(gomock.Any()).
		Return([]string{"ses_running"}, nil)
	mockStore.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{
		{ID: "spt_archived", Status: types.TaskStatusDone, Archived: true, PlanningSessionID: "ses_stopped", UpdatedAt: now},
		{ID: "spt_in_use", Status: types.TaskStatusDone, PlanningSessionID: "ses_running", UpdatedAt: now},
	}, nil)
	mockStore.EXPECT().ListSandboxInstances(gomock.Any()).
		Return([]*types.SandboxInstance{{ID: "sbox", LastSeen: now}}, nil)
	mockExec.EXPECT().ReconcileSandboxResources(gomock.Any(), "sbox", gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, req *hydra.GCReconcileRequest) (*hydra.GCReconcileResponse, error) {
			assert.Equal(t, []string{"ses_stopped", "ses_running"}, req.LiveSessionIDs,
				"session resources keep their recency padding")
			assert.Equal(t, []string{"spt_in_use"}, req.LiveSpecTaskIDs)
			return &hydra.GCReconcileResponse{}, nil
		})

	reapOrphanResources(context.Background(), mockExec, mockStore, 24*time.Hour, false)
}
