package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// gitCommit is a tiny real-git test helper: write a file, stage it, commit.
func gitCommit(t *testing.T, ctx context.Context, repoPath, file, content, msg string) {
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

// initRepoWithDefaultBranch inits a repo, makes an initial commit, and returns
// the repo path plus the resolved default branch name (main vs master varies by
// git version).
func initRepoWithDefaultBranch(t *testing.T, ctx context.Context) (repoPath, defaultBranch string) {
	t.Helper()
	repoPath = filepath.Join(t.TempDir(), "repo")
	require.NoError(t, giteagit.InitRepository(ctx, repoPath, false, "sha1"))
	gitCommit(t, ctx, repoPath, "README.md", "# initial", "c0 initial")
	out, _, err := gitcmd.NewCommand().AddArguments("rev-parse", "--abbrev-ref", "HEAD").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	return repoPath, strings.TrimSpace(out)
}

// TestMergeInternalRepoBranch_FastForwards verifies the happy path: a non-primary
// internal repo whose feature branch is ahead of its default branch is
// fast-forward merged server-side, advancing the default branch to the feature
// commit. This is the mixed-backing gap the helper closes — an internal secondary
// repo would otherwise be left unmerged after task approval.
func TestMergeInternalRepoBranch_FastForwards(t *testing.T) {
	ctx := context.Background()
	repoPath, defaultBranch := initRepoWithDefaultBranch(t, ctx)

	// Feature branch with one commit on top of the default branch — FF-able.
	_, _, coErr := gitcmd.NewCommand().AddArguments("checkout", "-b", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, coErr)
	gitCommit(t, ctx, repoPath, "feature.md", "work", "c1 feature")
	featureSHA, ferr := services.GetBranchCommitID(ctx, repoPath, "feature/x")
	require.NoError(t, ferr)

	repo := &types.GitRepository{ID: "repo_test", Name: "myrepo", LocalPath: repoPath, DefaultBranch: defaultBranch}
	task := &types.SpecTask{ID: "spt_test", BranchName: "feature/x"}

	srv := &HelixAPIServer{}
	require.NoError(t, srv.mergeInternalRepoBranch(ctx, repo, task))

	got, gerr := services.GetBranchCommitID(ctx, repoPath, defaultBranch)
	require.NoError(t, gerr)
	require.Equal(t, featureSHA, got, "default branch should be fast-forwarded to the feature commit")
}

// TestMergeInternalRepoBranch_NoFeatureBranchIsNoOp verifies that when the agent
// pushed no changes to this repo (feature branch absent), the helper is a no-op
// that returns nil rather than erroring — so a repo the task never touched can't
// fail the whole PR-ensuring pass.
func TestMergeInternalRepoBranch_NoFeatureBranchIsNoOp(t *testing.T) {
	ctx := context.Background()
	repoPath, defaultBranch := initRepoWithDefaultBranch(t, ctx)
	before, err := services.GetBranchCommitID(ctx, repoPath, defaultBranch)
	require.NoError(t, err)

	repo := &types.GitRepository{ID: "repo_test", Name: "myrepo", LocalPath: repoPath, DefaultBranch: defaultBranch}
	task := &types.SpecTask{ID: "spt_test", BranchName: "feature/never-pushed-here"}

	srv := &HelixAPIServer{}
	require.NoError(t, srv.mergeInternalRepoBranch(ctx, repo, task), "absent feature branch must be a no-op, not an error")

	after, err := services.GetBranchCommitID(ctx, repoPath, defaultBranch)
	require.NoError(t, err)
	require.Equal(t, before, after, "default branch must be unchanged when the feature branch is absent")
}
