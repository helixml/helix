package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// commit is a tiny test helper: write a file, stage it, commit, return the
// commit hash on the current branch HEAD.
func commit(t *testing.T, ctx context.Context, repoPath, file, content, msg string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, file), []byte(content), 0644))
	require.NoError(t, giteagit.AddChanges(ctx, repoPath, true))
	_, _, err := gitcmd.NewCommand().
		AddConfig("user.name", "Test Author").
		AddConfig("user.email", "test@example.com").
		AddArguments("commit").
		AddOptionFormat("--message=%s", msg).
		RunStdString(ctx, &gitcmd.RunOpts{
			Dir: repoPath,
			Env: []string{
				"GIT_AUTHOR_DATE=2026-05-08T08:00:00+00:00",
				"GIT_COMMITTER_DATE=2026-05-08T08:00:00+00:00",
			},
		})
	require.NoError(t, err)
}

// TestTryAutoMergeAfterRebase_InternalRepoSuccess covers the happy path:
// the agent has rebased its feature branch on top of main and pushed; the
// auto-retry called from handleFeatureBranchPush should FF-merge and mark
// the task done without any user click.
func TestTryAutoMergeAfterRebase_InternalRepoSuccess(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, giteagit.InitRepository(ctx, repoPath, false, "sha1"))

	// c0 on main
	commit(t, ctx, repoPath, "README.md", "# initial", "c0 initial")
	// Create feature branch from c0
	_, _, err := gitcmd.NewCommand().AddArguments("checkout", "-b", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	// Advance main with a divergent commit so plain FF would fail
	_, _, err = gitcmd.NewCommand().AddArguments("checkout", "main").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		// Some git versions still default to "master" on init
		_, _, err = gitcmd.NewCommand().AddArguments("checkout", "master").
			RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
		require.NoError(t, err)
	}
	// Determine the actual default branch name (main vs master)
	defaultBranch, _, err := gitcmd.NewCommand().AddArguments("rev-parse", "--abbrev-ref", "HEAD").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	defaultBranch = trimNewline(defaultBranch)

	commit(t, ctx, repoPath, "main.md", "diverged on main", "c1 diverge main")

	// Switch to feature, simulate agent rebase: bring in main's commit so FF works
	_, _, err = gitcmd.NewCommand().AddArguments("checkout", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	_, _, err = gitcmd.NewCommand().AddArguments("merge", "--no-ff", "-m", "merge main into feature").
		AddDynamicArguments(defaultBranch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)

	// At this point main is an ancestor of feature/x; FF would succeed.

	mockStore := store.NewMockStore(ctrl)
	project := &types.Project{ID: "prj_test", DefaultRepoID: "repo_test"}
	gitRepo := &types.GitRepository{
		ID:            "repo_test",
		LocalPath:     repoPath,
		DefaultBranch: defaultBranch,
		// IsExternal=false → no upstream sync/push, internal-repo path.
	}
	task := &types.SpecTask{
		ID:                "spt_test",
		ProjectID:         "prj_test",
		Status:            types.TaskStatusImplementationReview,
		BranchName:        "feature/x",
		RebaseRequestedAt: ptrTime("2026-05-08T08:13:00Z"),
	}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").Return(task, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_test").Return(project, nil)
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_test").Return(gitRepo, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).Return(nil)
	mockStore.EXPECT().DismissAttentionEventsForTask(gomock.Any(), "spt_test").Return(int64(0), nil)

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeAfterRebase(ctx, "spt_test")

	require.Equal(t, types.TaskStatusDone, task.Status, "task should be marked done after successful auto-merge")
	require.True(t, task.MergedToMain, "MergedToMain should be set")
	require.NotNil(t, task.MergedAt, "MergedAt should be set")
	require.NotNil(t, task.CompletedAt, "CompletedAt should be set")
	require.NotNil(t, task.ImplementationApprovedAt, "ImplementationApprovedAt should be set")
}

// TestTryAutoMergeAfterRebase_StillDivergentLeavesInReview covers the path
// where the agent's push didn't actually resolve the divergence (e.g. it
// pushed a new commit on the feature branch without merging main first).
// The helper should leave the task in implementation_review and NOT mark it
// done — the user can intervene from there.
func TestTryAutoMergeAfterRebase_StillDivergentLeavesInReview(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, giteagit.InitRepository(ctx, repoPath, false, "sha1"))

	commit(t, ctx, repoPath, "README.md", "# initial", "c0")

	_, _, err := gitcmd.NewCommand().AddArguments("checkout", "-b", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	commit(t, ctx, repoPath, "feature.md", "feature work", "feature commit")

	// Switch back to default branch and advance with a divergent commit.
	_, _, err = gitcmd.NewCommand().AddArguments("checkout", "main").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		_, _, err = gitcmd.NewCommand().AddArguments("checkout", "master").
			RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
		require.NoError(t, err)
	}
	defaultBranch, _, err := gitcmd.NewCommand().AddArguments("rev-parse", "--abbrev-ref", "HEAD").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	defaultBranch = trimNewline(defaultBranch)

	commit(t, ctx, repoPath, "main.md", "diverged on main", "c1 diverge main")
	// feature/x and default are siblings — FF impossible. Agent has NOT rebased.

	mockStore := store.NewMockStore(ctrl)
	project := &types.Project{ID: "prj_test", DefaultRepoID: "repo_test"}
	gitRepo := &types.GitRepository{
		ID:            "repo_test",
		LocalPath:     repoPath,
		DefaultBranch: defaultBranch,
	}
	task := &types.SpecTask{
		ID:                "spt_test",
		ProjectID:         "prj_test",
		Status:            types.TaskStatusImplementationReview,
		BranchName:        "feature/x",
		RebaseRequestedAt: ptrTime("2026-05-08T08:13:00Z"),
	}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").Return(task, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_test").Return(project, nil)
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_test").Return(gitRepo, nil)
	// No UpdateSpecTask expected — failure path leaves the row untouched.

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeAfterRebase(ctx, "spt_test")

	require.Equal(t, types.TaskStatusImplementationReview, task.Status, "task should stay in implementation_review when FF still fails")
	require.False(t, task.MergedToMain, "MergedToMain must remain unset")
}

// TestTryAutoMergeAfterRebase_SkipsIfStatusChanged covers the status guard:
// if the task is no longer in implementation_review by the time the goroutine
// runs (user moved it to backlog, archived it, another path already merged it),
// the helper must do nothing — no GetProject, no GetGitRepository, no
// UpdateSpecTask. Without this guard, the auto-merge would resurrect the task
// as `done` and overwrite the user's deliberate intervention.
func TestTryAutoMergeAfterRebase_SkipsIfStatusChanged(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:                "spt_test",
		ProjectID:         "prj_test",
		Status:            types.TaskStatusBacklog, // no longer implementation_review
		BranchName:        "feature/x",
		RebaseRequestedAt: ptrTime("2026-05-08T08:13:00Z"),
	}

	// Only GetSpecTask is expected — the status guard short-circuits everything else.
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").Return(task, nil)

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeAfterRebase(ctx, "spt_test")

	require.Equal(t, types.TaskStatusBacklog, task.Status, "task status must not change once moved away from implementation_review")
}

func trimNewline(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return s[:len(s)-1]
	}
	return s
}

func ptrTime(rfc3339 string) *time.Time {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic(err)
	}
	return &t
}
