package services

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func runGit(t *testing.T, ctx context.Context, repoPath string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

// A task started from develop must land on develop, never on the repo default.
func TestTryAutoMergeAfterRebase_MergesIntoTaskBaseBranch(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, giteagit.InitRepository(ctx, repoPath, false, "sha1"))
	commit(t, ctx, repoPath, "README.md", "# initial", "c0 initial")
	defaultBranch := runGit(t, ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	defaultBefore := runGit(t, ctx, repoPath, "rev-parse", defaultBranch)

	runGit(t, ctx, repoPath, "checkout", "-b", "develop")
	commit(t, ctx, repoPath, "develop.md", "develop work", "c1 develop")
	runGit(t, ctx, repoPath, "checkout", "-b", "feature/x")
	commit(t, ctx, repoPath, "feature.md", "feature work", "c2 feature")
	featureHead := runGit(t, ctx, repoPath, "rev-parse", "feature/x")

	mockStore := store.NewMockStore(ctrl)
	project := &types.Project{ID: "prj_test", DefaultRepoID: "repo_test"}
	gitRepo := &types.GitRepository{ID: "repo_test", LocalPath: repoPath, DefaultBranch: defaultBranch}
	task := &types.SpecTask{
		ID:                "spt_test",
		ProjectID:         "prj_test",
		Status:            types.TaskStatusImplementationReview,
		BranchName:        "feature/x",
		BaseBranch:        "develop",
		RebaseRequestedAt: ptrTime("2026-05-08T08:13:00Z"),
	}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").Return(task, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_test").Return(project, nil)
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_test").Return(gitRepo, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).Return(nil)
	mockStore.EXPECT().DismissAttentionEventsForTask(gomock.Any(), "spt_test").Return(int64(0), nil)

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeAfterRebase(ctx, "spt_test")

	require.Equal(t, types.TaskStatusDone, task.Status)
	require.Equal(t, featureHead, runGit(t, ctx, repoPath, "rev-parse", "develop"), "feature must land on the task base branch")
	require.Equal(t, defaultBefore, runGit(t, ctx, repoPath, "rev-parse", defaultBranch), "repo default branch must be untouched")
}
