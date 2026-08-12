package services

import (
	"context"
	"path/filepath"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// botMergeRepo builds a repo with a feature branch and returns its path plus the
// resolved default branch name (git version dependent: main vs master).
// When ffPossible is true the feature branch has the default branch merged into
// it, so the default branch is an ancestor and a fast-forward will succeed.
func botMergeRepo(t *testing.T, ctx context.Context, ffPossible bool) (string, string) {
	t.Helper()
	repoPath := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, giteagit.InitRepository(ctx, repoPath, false, "sha1"))
	commit(t, ctx, repoPath, "README.md", "# initial", "c0 initial")

	_, _, err := gitcmd.NewCommand().AddArguments("checkout", "-b", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	commit(t, ctx, repoPath, "feature.md", "agent work", "feature commit")

	if _, _, err := gitcmd.NewCommand().AddArguments("checkout", "main").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath}); err != nil {
		_, _, err = gitcmd.NewCommand().AddArguments("checkout", "master").
			RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
		require.NoError(t, err)
	}
	defaultBranch, _, err := gitcmd.NewCommand().AddArguments("rev-parse", "--abbrev-ref", "HEAD").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	defaultBranch = trimNewline(defaultBranch)

	// Advance the default branch so the branches diverge, mirroring a second
	// operator landing their run first.
	commit(t, ctx, repoPath, "other.md", "another operator's run", "c1 diverge default")

	_, _, err = gitcmd.NewCommand().AddArguments("checkout", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)

	if ffPossible {
		// What the agent is instructed to do before pushing.
		_, _, err = gitcmd.NewCommand().
			AddConfig("user.name", "Test Author").
			AddConfig("user.email", "test@example.com").
			AddArguments("merge", "--no-ff", "-m", "merge base into feature").
			AddDynamicArguments(defaultBranch).
			RunStdString(ctx, &gitcmd.RunOpts{
				Dir: repoPath,
				Env: []string{
					"GIT_AUTHOR_DATE=2026-05-08T08:00:00+00:00",
					"GIT_COMMITTER_DATE=2026-05-08T08:00:00+00:00",
				},
			})
		require.NoError(t, err)
	}
	return repoPath, defaultBranch
}

// A bot_run on an internal repo has no human to click Accept and no PR path, so
// the merge must be driven for it. Critically it must NOT be marked done: bots are
// long-running (one persistent spec task each), so completing the task would stop
// the bot.
func TestTryAutoMergeBotRun_InternalRepoMergesWithoutCompletingTask(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath, defaultBranch := botMergeRepo(t, ctx, true)

	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "spt_bot",
		ProjectID:  "prj_test",
		Type:       specTaskTypeBotRun,
		Status:     types.TaskStatusImplementation,
		BranchName: "feature/x",
	}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_bot").Return(task, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).Return(nil)

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeBotRun(ctx, "spt_bot", &types.GitRepository{ID: "repo_test", LocalPath: repoPath, DefaultBranch: defaultBranch})

	require.True(t, task.MergedToMain, "the branch should have been merged")
	require.NotNil(t, task.MergedAt)
	require.Equal(t, types.TaskStatusImplementation, task.Status,
		"a long-running bot task must NOT be completed by the merge, or the bot stops")

	head, _, err := gitcmd.NewCommand().AddArguments("rev-parse").AddDynamicArguments(defaultBranch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	feature, _, err := gitcmd.NewCommand().AddArguments("rev-parse", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	require.Equal(t, trimNewline(feature), trimNewline(head),
		"default branch should now point at the feature branch tip")
}

// If the agent skipped its pre-push merge the branch has diverged. Nothing should
// be merged or recorded; the agent has to merge the base in and push again.
func TestTryAutoMergeBotRun_DivergedDoesNotMerge(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath, defaultBranch := botMergeRepo(t, ctx, false)

	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "spt_bot",
		ProjectID:  "prj_test",
		Type:       specTaskTypeBotRun,
		Status:     types.TaskStatusImplementation,
		BranchName: "feature/x",
	}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_bot").Return(task, nil)
	// No UpdateSpecTask: nothing was merged.

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeBotRun(ctx, "spt_bot", &types.GitRepository{ID: "repo_test", LocalPath: repoPath, DefaultBranch: defaultBranch})

	require.False(t, task.MergedToMain, "a diverged branch must not be recorded as merged")
	require.Nil(t, task.MergedAt)
}

// External repos keep their existing PR flow; the bot auto-merge must not touch
// them, or it would bypass review and skip the upstream push.
func TestTryAutoMergeBotRun_ExternalRepoSkipped(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath, defaultBranch := botMergeRepo(t, ctx, true)

	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "spt_bot",
		ProjectID:  "prj_test",
		Type:       specTaskTypeBotRun,
		Status:     types.TaskStatusImplementation,
		BranchName: "feature/x",
	}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_bot").Return(task, nil)

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeBotRun(ctx, "spt_bot", &types.GitRepository{
		ID: "repo_test", LocalPath: repoPath, DefaultBranch: defaultBranch,
		IsExternal: true, ExternalURL: "https://github.com/example/repo",
	})

	require.False(t, task.MergedToMain, "external repos must keep the PR flow")
}

// A non-bot task in implementation is still driven by a human clicking Accept.
func TestTryAutoMergeBotRun_NonBotTaskSkipped(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "spt_human",
		ProjectID:  "prj_test",
		Type:       "feature",
		Status:     types.TaskStatusImplementation,
		BranchName: "feature/x",
	}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_human").Return(task, nil)
	// Returns before touching the project or repo.

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeBotRun(ctx, "spt_human", &types.GitRepository{ID: "repo_test"})

	require.False(t, task.MergedToMain)
}

// The regression that stranded a fortnight of playbook commits: a project whose
// DEFAULT repo is external (GitHub) but which also contains an internal repo. The
// bot pushes to the internal one; auto-merge used to re-resolve the project
// default, trip its own IsExternal guard on that unrelated repo, and return
// without merging anything. Nothing ever landed, and no error was logged.
//
// Passing the pushed repo is the fix, so pin that an internal push still merges
// even though the project default is external.
func TestTryAutoMergeBotRun_InternalRepoMergesWhenProjectDefaultIsExternal(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath, defaultBranch := botMergeRepo(t, ctx, true)

	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "spt_bot",
		ProjectID:  "prj_mixed",
		Type:       specTaskTypeBotRun,
		Status:     types.TaskStatusImplementation,
		BranchName: "feature/x",
	}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_bot").Return(task, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).Return(nil)
	// Deliberately NO GetProject/GetGitRepository expectations: resolving the
	// project default is exactly the behaviour that caused the bug, so if it comes
	// back gomock fails on the unexpected call.

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeBotRun(ctx, "spt_bot", &types.GitRepository{
		ID: "repo_internal_playbook", LocalPath: repoPath, DefaultBranch: defaultBranch,
	})

	require.True(t, task.MergedToMain,
		"a bot push to an internal repo must merge even when the project default repo is external")

	head, _, err := gitcmd.NewCommand().AddArguments("rev-parse").AddDynamicArguments(defaultBranch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	feature, _, err := gitcmd.NewCommand().AddArguments("rev-parse", "feature/x").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, err)
	require.Equal(t, trimNewline(feature), trimNewline(head))
}

// HelixOS dispatches its unattended runs under three different Type values from
// one helper: bot_run, hypothesis and candidate_search. Matching only "bot_run"
// silently excluded the hypothesis bots — the ones doing daily outreach and
// writing back to the shared playbook repo — from auto-merge entirely.
func TestIsAutonomousRunCoversEveryUnattendedType(t *testing.T) {
	for _, typ := range []string{"bot_run", "hypothesis", "candidate_search"} {
		require.True(t, isAutonomousRun(typ), "%s is an unattended HelixOS run and must auto-merge", typ)
	}
	for _, typ := range []string{"feature", "bug", ""} {
		require.False(t, isAutonomousRun(typ), "%q is human-driven and must wait for Accept", typ)
	}
}

// End-to-end on the merge itself: a hypothesis run pushing to an internal repo
// must land, exactly as a bot_run does.
func TestTryAutoMergeBotRun_HypothesisRunMerges(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repoPath, defaultBranch := botMergeRepo(t, ctx, true)

	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "spt_hyp",
		ProjectID:  "prj_test",
		Type:       specTaskTypeHypothesis,
		Status:     types.TaskStatusImplementation,
		BranchName: "feature/x",
	}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_hyp").Return(task, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).Return(nil)

	srv := &GitHTTPServer{store: mockStore}
	srv.tryAutoMergeBotRun(ctx, "spt_hyp", &types.GitRepository{
		ID: "repo_playbook", LocalPath: repoPath, DefaultBranch: defaultBranch,
	})

	require.True(t, task.MergedToMain, "a hypothesis run must auto-merge like any other unattended run")
}
