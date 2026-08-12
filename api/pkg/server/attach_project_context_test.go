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

// Regression test for the worker-hire bug: StartExternalAgentSession must
// populate ProjectID, RepositoryIDs, and PrimaryRepositoryID on the
// DesktopAgent. Without it, HELIX_REPOSITORIES is empty in the container
// and helix-workspace-setup.sh aborts.
func TestAttachProjectContext_PopulatesAgentFromStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: "prj_x"}).
		Return([]*types.GitRepository{
			{ID: "repo_a"},
			{ID: "repo_b"},
		}, nil)
	mockStore.EXPECT().
		GetProject(gomock.Any(), "prj_x").
		Return(&types.Project{ID: "prj_x", DefaultRepoID: "repo_b"}, nil)

	s := &HelixAPIServer{Store: mockStore}
	agent := &types.DesktopAgent{}
	err := s.attachProjectContext(context.Background(), agent, "prj_x")
	require.NoError(t, err)
	require.Equal(t, "prj_x", agent.ProjectID)
	require.Equal(t, []string{"repo_a", "repo_b"}, agent.RepositoryIDs)
	require.Equal(t, "repo_b", agent.PrimaryRepositoryID)
}

func TestAttachProjectContext_EmptyProjectIDIsNoOp(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// No store calls expected.

	s := &HelixAPIServer{Store: mockStore}
	agent := &types.DesktopAgent{}
	err := s.attachProjectContext(context.Background(), agent, "")
	require.NoError(t, err)
	require.Empty(t, agent.ProjectID)
	require.Empty(t, agent.RepositoryIDs)
	require.Empty(t, agent.PrimaryRepositoryID)
}

func TestAttachProjectContext_NoReposLeavesRepoFieldsUnset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		ListGitRepositories(gomock.Any(), gomock.Any()).
		Return(nil, nil)
	// GetProject is not called when there are no repos (no work to do).

	s := &HelixAPIServer{Store: mockStore}
	agent := &types.DesktopAgent{}
	err := s.attachProjectContext(context.Background(), agent, "prj_empty")
	require.NoError(t, err)
	require.Equal(t, "prj_empty", agent.ProjectID)
	require.Empty(t, agent.RepositoryIDs)
	require.Empty(t, agent.PrimaryRepositoryID)
}

func TestAttachProjectContext_ListReposErrorIsReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		ListGitRepositories(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("db boom"))

	s := &HelixAPIServer{Store: mockStore}
	agent := &types.DesktopAgent{}
	err := s.attachProjectContext(context.Background(), agent, "prj_x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "db boom")
}

func TestAttachProjectContext_GetProjectErrorTolerated_FirstRepoWins(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// If GetProject fails (e.g., briefly racy lookup), we still want repos
	// attached — the worst case is PrimaryRepositoryID falls back to the
	// first repo. This matches the pre-refactor behaviour of the
	// startDevContainerForSession path.
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().
		ListGitRepositories(gomock.Any(), gomock.Any()).
		Return([]*types.GitRepository{{ID: "repo_first"}}, nil)
	mockStore.EXPECT().
		GetProject(gomock.Any(), "prj_x").
		Return(nil, errors.New("transient"))

	s := &HelixAPIServer{Store: mockStore}
	agent := &types.DesktopAgent{}
	err := s.attachProjectContext(context.Background(), agent, "prj_x")
	require.NoError(t, err)
	require.Equal(t, []string{"repo_first"}, agent.RepositoryIDs)
	require.Equal(t, "repo_first", agent.PrimaryRepositoryID)
}
