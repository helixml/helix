package external_agent

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestStartDesktopRejectsMissingEligibleSandboxRunner(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)
	session := &types.Session{ID: "ses_1"}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(session, nil).Times(2)
	mockStore.EXPECT().FindAvailableSandboxInstance(gomock.Any(), "ubuntu", false).Return(nil, nil)

	_, err := executor.StartDesktop(context.Background(), &types.DesktopAgent{
		SessionID:   "ses_1",
		DesktopType: "headless",
	})
	require.EqualError(t, err, `no eligible sandbox runner available for container type "headless" (required image "ubuntu"); check runner status, heartbeat, and advertised desktop image versions`)
}

// A start that fails after the boot was marked (here: Hydra unreachable over
// RevDial, as when a deploy recreates the sandbox) must leave the session
// "stopped" with the reason — the state the UI renders as a stopped desktop
// with a Start button. It used to write "", which the spec-task page read as
// "never launched" and showed a boot spinner forever.
func TestStartDesktopFailureLeavesSessionStoppedWithReason(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	executor := newTestExecutor(mockStore)
	executor.connman = failingConnman{err: errors.New("no connection")}
	executor.workspaceBasePathForCloning = t.TempDir()

	stored := types.Session{ID: "ses_1", Metadata: types.SessionMetadata{
		DevContainerID:      "old-container",
		ExternalAgentStatus: "stopped",
		StatusMessage:       "Desktop failed to start: earlier failure",
	}}
	var statuses []string
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").DoAndReturn(
		func(context.Context, string) (*types.Session, error) {
			copy := stored
			return &copy, nil
		}).AnyTimes()
	mockStore.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, s types.Session) (*types.Session, error) {
			stored = s
			statuses = append(statuses, s.Metadata.ExternalAgentStatus+"|"+s.Metadata.StatusMessage)
			return &s, nil
		}).AnyTimes()

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_1"}).
		Return(&types.User{ID: "usr_1", Email: "dev@example.com", FullName: "Dev"}, nil).AnyTimes()
	mockStore.EXPECT().GetSandboxInstance(gomock.Any(), "sbx_1").
		Return(&types.SandboxInstance{ID: "sbx_1", MaxSandboxes: 1}, nil)

	_, err := executor.StartDesktop(context.Background(), &types.DesktopAgent{
		SessionID:   "ses_1",
		UserID:      "usr_1",
		SandboxID:   "sbx_1",
		DesktopType: "headless",
		CustomImage: "registry.example/desktop:1",
	})

	require.ErrorContains(t, err, "no connection")
	require.NotEmpty(t, statuses)
	require.Equal(t, "starting|", statuses[0], "a new attempt must clear the previous failure reason")
	require.Equal(t, "stopped", stored.Metadata.ExternalAgentStatus)
	require.Equal(t, "Desktop failed to start: failed to create dev container via Hydra: failed to dial Hydra via RevDial: no connection", stored.Metadata.StatusMessage)
}
