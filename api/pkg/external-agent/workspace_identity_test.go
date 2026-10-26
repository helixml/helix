package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestWorkspaceLostThread(t *testing.T) {
	withThread := func(workspaceID string) types.SessionMetadata {
		return types.SessionMetadata{ZedThreadID: "thread-1", WorkspaceID: workspaceID}
	}
	cases := []struct {
		name        string
		metadata    types.SessionMetadata
		workspaceID string
		created     bool
		lost        bool
	}{
		{"same workspace", withThread("ws-a"), "ws-a", false, false},
		{"workspace replaced", withThread("ws-a"), "ws-b", false, true},
		{"workspace replaced and created", withThread("ws-a"), "ws-b", true, true},
		// Sessions from before identity stamping: only a workspace created in
		// this start proves the thread store is new.
		{"legacy session, workspace created", withThread(""), "ws-b", true, true},
		{"legacy session, workspace survived", withThread(""), "ws-b", false, false},
		{"no thread to lose", types.SessionMetadata{WorkspaceID: "ws-a"}, "ws-b", true, false},
		{"hydra without identity", withThread("ws-a"), "", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.lost, workspaceLostThread(tc.metadata, tc.workspaceID, tc.created))
		})
	}
}

func TestReconcileWorkspaceIdentity_RecreatedWorkspaceFlagsReseed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := &HydraExecutor{store: mockStore}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Metadata: types.SessionMetadata{
		ZedThreadID: "ses_f569d9bb3ffew704MweP05X3p7", WorkspaceID: "ws-old", ZedAgentName: "opencode",
	}}, nil)
	mockStore.EXPECT().UpdateSessionMetadata(gomock.Any(), "ses_1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, metadata types.SessionMetadata) error {
			assert.Empty(t, metadata.ZedThreadID)
			assert.Equal(t, "ses_f569d9bb3ffew704MweP05X3p7", metadata.ThreadReseedFrom)
			assert.Equal(t, "ws-new", metadata.WorkspaceID)
			assert.Equal(t, "opencode", metadata.ZedAgentName, "unrelated metadata is preserved")
			return nil
		},
	)

	require.NoError(t, h.reconcileWorkspaceIdentity(context.Background(), "ses_1", &hydra.DevContainerResponse{WorkspaceID: "ws-new", WorkspaceCreated: true}))
}

func TestReconcileWorkspaceIdentity_SurvivingWorkspaceTouchesNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := &HydraExecutor{store: mockStore}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Metadata: types.SessionMetadata{
		ZedThreadID: "thread-1", WorkspaceID: "ws-a",
	}}, nil)
	mockStore.EXPECT().UpdateSessionMetadata(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	require.NoError(t, h.reconcileWorkspaceIdentity(context.Background(), "ses_1", &hydra.DevContainerResponse{WorkspaceID: "ws-a"}))
}
