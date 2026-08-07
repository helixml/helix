package server

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func failedTask(justDoIt bool) *types.SpecTask {
	return &types.SpecTask{
		ID:           "spt_1",
		Status:       types.TaskStatusBacklog,
		JustDoItMode: justDoIt,
		Metadata: map[string]interface{}{
			"error":                    "no active Claude subscription is available",
			"error_timestamp":          "2026-08-07T10:07:04Z",
			types.TaskErrorCodeKey:     types.TaskErrorSubscriptionRequired,
			types.TaskErrorProviderKey: types.SubscriptionProviderClaude,
			"unrelated":                "keep me",
		},
	}
}

func specTaskSession() *types.Session {
	return &types.Session{ID: "ses_1", Metadata: types.SessionMetadata{SpecTaskID: "spt_1"}}
}

// The state this exists to prevent: an agent working for 40 minutes while the
// task still reads backlog with the launch failure that has since been fixed.
func TestReconcileSpecTaskAfterTurn_ReleasesLatchedFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	task := failedTask(true)

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(task, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated *types.SpecTask) error {
			require.Equal(t, types.TaskStatusImplementation, updated.Status)
			require.NotContains(t, updated.Metadata, "error")
			require.NotContains(t, updated.Metadata, "error_timestamp")
			require.NotContains(t, updated.Metadata, types.TaskErrorCodeKey)
			require.NotContains(t, updated.Metadata, types.TaskErrorProviderKey)
			require.Equal(t, "keep me", updated.Metadata["unrelated"], "unrelated metadata must survive")
			require.NotNil(t, updated.StatusUpdatedAt)
			return nil
		})

	server.reconcileSpecTaskAfterTurn(context.Background(), specTaskSession())
}

// A planning task must not be dropped into implementation.
func TestReconcileSpecTaskAfterTurn_UsesThePhaseTheTaskWouldHaveStartedIn(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(failedTask(false), nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated *types.SpecTask) error {
			require.Equal(t, types.TaskStatusSpecGeneration, updated.Status)
			return nil
		})

	server.reconcileSpecTaskAfterTurn(context.Background(), specTaskSession())
}

// Reconciliation must not reopen a phase the orchestrator owns, nor a terminal
// one: a turn completing on a merged task must not drag it back to work.
func TestReconcileSpecTaskAfterTurn_LeavesNonBacklogStatusAlone(t *testing.T) {
	for _, status := range []types.SpecTaskStatus{
		types.TaskStatusImplementation,
		types.TaskStatusImplementationReview,
		types.TaskStatusDone,
		types.TaskStatusQueuedImplementation,
	} {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		server := &HelixAPIServer{Store: mockStore}
		task := failedTask(true)
		task.Status = status

		mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(task, nil)
		// The stale error is still released — it is false either way — but the
		// status must be left exactly as the orchestrator set it.
		mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, updated *types.SpecTask) error {
				require.Equal(t, status, updated.Status)
				require.NotContains(t, updated.Metadata, "error")
				return nil
			})

		server.reconcileSpecTaskAfterTurn(context.Background(), specTaskSession())
		ctrl.Finish()
	}
}

func TestReconcileSpecTaskAfterTurn_NoWriteWhenNothingToReconcile(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	// Healthy task mid-implementation: no latch, no status change, no write.
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{
		ID: "spt_1", Status: types.TaskStatusImplementation,
	}, nil)

	server.reconcileSpecTaskAfterTurn(context.Background(), specTaskSession())
}

func TestReconcileSpecTaskAfterTurn_IgnoresPlainChatSessions(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	// No SpecTaskID: the store must not be touched at all.
	server.reconcileSpecTaskAfterTurn(context.Background(), &types.Session{ID: "ses_1"})
	server.reconcileSpecTaskAfterTurn(context.Background(), nil)
}

// The gap the turn path alone leaves: an agent connects, does work, and the
// sandbox idles out without a turn ever completing. The launch error is false
// the moment the agent is on the wire, so connection must release it.
func TestReconcileSpecTaskLaunchFailure_ReleasesLatchWithoutAdvancingStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	task := failedTask(true)

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(specTaskSession(), nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(task, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated *types.SpecTask) error {
			require.NotContains(t, updated.Metadata, "error")
			require.NotContains(t, updated.Metadata, types.TaskErrorCodeKey)
			require.NotContains(t, updated.Metadata, types.TaskErrorProviderKey)
			// Connecting is not evidence of finished work; the turn path owns status.
			require.Equal(t, types.TaskStatusBacklog, updated.Status)
			return nil
		})

	server.reconcileSpecTaskLaunchFailure(context.Background(), "ses_1")
}

func TestReconcileSpecTaskLaunchFailure_NoWriteWhenNoLatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(specTaskSession(), nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_1").Return(&types.SpecTask{
		ID: "spt_1", Status: types.TaskStatusBacklog,
	}, nil)

	// A backlog task with no latched error is a task that simply has not been
	// started; connecting an agent must not rewrite it.
	server.reconcileSpecTaskLaunchFailure(context.Background(), "ses_1")
}

func TestReconcileSpecTaskLaunchFailure_IgnoresUnknownSessions(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_missing").Return(nil, errors.New("not found"))
	server.reconcileSpecTaskLaunchFailure(context.Background(), "ses_missing")

	// Empty id must not even reach the store.
	server.reconcileSpecTaskLaunchFailure(context.Background(), "")
}
