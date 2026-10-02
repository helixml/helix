package services

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHandleFeatureBranchPushRecordsCompletedTaskPush(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "spt_test",
		ProjectID:  "prj_test",
		Status:     types.TaskStatusDone,
		BranchName: "feature/follow-up",
	}
	repo := &types.GitRepository{ID: "repo_test"}

	mockStore.EXPECT().GetProjectsForRepository(gomock.Any(), repo.ID).Return([]string{task.ProjectID}, nil)
	mockStore.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{task}, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).Return(nil)

	server := &GitHTTPServer{store: mockStore}
	server.handleFeatureBranchPush(context.Background(), repo, task.BranchName, "new-commit", "", nil)

	require.Equal(t, "new-commit", task.LastPushCommitHash)
	require.NotNil(t, task.LastPushAt)
}
