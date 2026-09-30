package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A task started from develop must be told to merge develop, not the repo default.
func TestBuildImplementationPushInstruction_UsesTaskBaseBranch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	repo := &types.GitRepository{ID: "repo_primary", Name: "app", DefaultBranch: "main"}
	task := &types.SpecTask{
		ID:         "spt_test",
		ProjectID:  "prj_test",
		Name:       "Add feature",
		BranchName: "feature/001-add-feature",
		BaseBranch: "develop",
	}
	mockStore.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{repo}, nil)

	s := &HelixAPIServer{Store: mockStore}
	message, err := s.buildImplementationPushInstruction(context.Background(), task, repo, repo.ID)
	require.NoError(t, err)

	require.Contains(t, message, "git fetch origin develop && git merge origin/develop")
	require.NotContains(t, message, "origin/main")
}
