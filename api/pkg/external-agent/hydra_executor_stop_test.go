package external_agent

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type failingConnman struct {
	err error
}

func (c failingConnman) Dial(context.Context, string) (net.Conn, error) {
	return nil, c.err
}

func TestHydraExecutorRejectsEmptySessionIDWithoutStoreAccess(t *testing.T) {
	mockStore := store.NewMockStore(gomock.NewController(t))
	h := &HydraExecutor{store: mockStore}

	assert.EqualError(t, h.StopDesktop(context.Background(), ""), "session ID is required to stop desktop")
	assert.EqualError(t, h.revokeSessionAPIKeys(context.Background(), ""), "session ID is required to revoke session keys")
}

func TestHydraExecutorStopFailurePreservesLiveSessionState(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.connman = failingConnman{err: errors.New("hydra disconnected")}
	tracked := &ZedSession{
		SessionID:   "ses_live",
		ContainerID: "container-live",
		SandboxID:   "runner-1",
		Status:      "running",
	}
	h.sessions["ses_live"] = tracked

	err := h.StopDesktop(context.Background(), "ses_live")

	assert.ErrorContains(t, err, "hydra disconnected")
	assert.Same(t, tracked, h.sessions["ses_live"], "failed stop must preserve in-memory tracking")
	assert.Equal(t, "container-live", h.sessions["ses_live"].ContainerID)
	// The strict mock has no store expectations: a failed delete must not revoke
	// API keys, close billing, decrement capacity, or clear session status.
}

// A stopped desktop must read as "stopped" — the UI's stopped state with a
// Start button — not "", which is indistinguishable from a session whose first
// launch has not begun.
func TestHydraExecutorStopMarksSessionStopped(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.connman = fakeConnman{}
	h.filestoreLocalPath = t.TempDir()
	h.sessions["ses_x"] = &ZedSession{SessionID: "ses_x", ContainerID: "c1", SandboxID: "local", Status: "running"}

	stored := types.Session{ID: "ses_x", Metadata: types.SessionMetadata{
		ExternalAgentStatus: "running",
		StatusMessage:       "Unpacking build cache (1.0/2.0 GB)",
		ContainerName:       "ubuntu-external-x",
	}}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_x").DoAndReturn(
		func(context.Context, string) (*types.Session, error) {
			copy := stored
			return &copy, nil
		}).AnyTimes()
	mockStore.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, s types.Session) (*types.Session, error) {
			stored = s
			return &s, nil
		}).AnyTimes()
	mockStore.EXPECT().DecrementSandboxContainerCount(gomock.Any(), "local").Return(nil)
	mockStore.EXPECT().ListAPIKeys(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	require.NoError(t, h.StopDesktop(context.Background(), "ses_x"))

	assert.Equal(t, "stopped", stored.Metadata.ExternalAgentStatus)
	assert.Empty(t, stored.Metadata.StatusMessage)
}

func TestHydraExecutorStaleLocalSessionDecrementsContainerCount(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.connman = fakeConnman{}
	h.sessions["ses_stale"] = &ZedSession{
		SessionID:   "ses_stale",
		ContainerID: "c1",
		SandboxID:   "local",
		Status:      "running",
	}

	mockStore.EXPECT().DecrementSandboxContainerCount(gomock.Any(), "local").Return(nil)

	assert.False(t, h.HasRunningContainer(context.Background(), "ses_stale"))
	_, stillTracked := h.sessions["ses_stale"]
	assert.False(t, stillTracked)
}

func TestHydraExecutorSandboxDisconnectPreservesRunnerAffinity(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	session := &types.Session{
		ID:        "ses_x",
		SandboxID: "runner-1",
		Metadata: types.SessionMetadata{
			ContainerName:       "ubuntu-external-ses_x",
			ContainerID:         "c1",
			ExternalAgentStatus: "running",
		},
	}

	mockStore.EXPECT().ListSessionsBySandbox(gomock.Any(), "runner-1").Return([]*types.Session{session}, nil)
	mockStore.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated types.Session) (*types.Session, error) {
			assert.Equal(t, "runner-1", updated.SandboxID)
			assert.Equal(t, "stopped", updated.Metadata.ExternalAgentStatus)
			assert.Empty(t, updated.Metadata.ContainerID)
			return &updated, nil
		},
	)

	h.OnSandboxDisconnected("runner-1")
}
