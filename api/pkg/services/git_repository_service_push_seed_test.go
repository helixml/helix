package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// seedTestRepos returns a Helix bare mirror and an upstream bare repo whose
// pre-receive hook appends every ref it receives to the returned log file, so
// tests can see the order branches reached the upstream.
func seedTestRepos(t *testing.T) (service *GitRepositoryService, repo *types.GitRepository, upstream, refLog string) {
	t.Helper()
	ctx := context.Background()
	upstream, refLog = emptyUpstream(t)
	local := filepath.Join(t.TempDir(), "local.git")
	require.NoError(t, giteagit.InitRepository(ctx, local, true, "sha1"))

	mockStore := store.NewMockStore(gomock.NewController(t))
	service = NewGitRepositoryService(mockStore, t.TempDir(), "http://localhost:8080", "test", "test@example.com")
	repo = &types.GitRepository{
		ID: "repo-seed", Name: "module-example", LocalPath: local, IsExternal: true,
		ExternalURL: "file://" + upstream, ExternalType: types.ExternalRepositoryTypeGitHub,
		DefaultBranch: "main",
	}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), repo.ID).Return(repo, nil).AnyTimes()
	mockStore.EXPECT().MarkGitRepositorySyncStarted(gomock.Any(), repo.ID, gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().MarkGitRepositorySyncFinished(gomock.Any(), repo.ID, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	return service, repo, upstream, refLog
}

func receivedRefs(t *testing.T, refLog string) []string {
	t.Helper()
	b, err := os.ReadFile(refLog)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Fields(string(b))
}

func commitFile(t *testing.T, repoPath, branch, file, content string) {
	t.Helper()
	_, _, err := CommitFileToBareBranch(context.Background(), repoPath, branch, file, []byte(content), "Ada Lovelace", "ada@example.com", "update "+file)
	require.NoError(t, err)
}

// The startup script is saved to helix-specs and pushed before anything else
// exists. On an empty upstream, main must still arrive first.
func TestHelixSpecsWriteToEmptyUpstreamPushesMainFirst(t *testing.T) {
	ctx := context.Background()
	service, repo, upstream, refLog := seedTestRepos(t)

	err := service.WithExternalRepoWrite(ctx, repo, ExternalRepoWriteOptions{Branch: SpecsBranchName, FailOnPushError: true}, func() error {
		_, _, err := CommitFileToBareBranch(ctx, repo.LocalPath, SpecsBranchName, ".helix/startup.sh", []byte("#!/bin/bash\n"), "Ada Lovelace", "ada@example.com", "Update startup script")
		return err
	})
	require.NoError(t, err)

	require.Equal(t, []string{"refs/heads/main", "refs/heads/helix-specs"}, receivedRefs(t, refLog))

	readme, _, err := gitcmd.NewCommand("show", "refs/heads/main:README.md").RunStdString(ctx, &gitcmd.RunOpts{Dir: upstream})
	require.NoError(t, err)
	require.Equal(t, "# module-example\n\nThis repository was initialized by Helix.\n", readme)
	author, _, err := gitcmd.NewCommand("log", "-1", "--format=%an <%ae>", "refs/heads/main").RunStdString(ctx, &gitcmd.RunOpts{Dir: upstream})
	require.NoError(t, err)
	require.Equal(t, "Ada Lovelace <ada@example.com>", strings.TrimSpace(author))

	// Once the upstream has branches, later pushes go straight through.
	commitFile(t, repo.LocalPath, SpecsBranchName, "design/notes.md", "notes")
	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))
	require.Equal(t, []string{"refs/heads/main", "refs/heads/helix-specs", "refs/heads/helix-specs"}, receivedRefs(t, refLog))
}

// A default branch that already exists locally is pushed as it is, not replaced
// with a README commit.
func TestFeaturePushToEmptyUpstreamPushesExistingDefaultBranchFirst(t *testing.T) {
	ctx := context.Background()
	service, repo, upstream, refLog := seedTestRepos(t)
	repo.DefaultBranch = "trunk"
	commitFile(t, repo.LocalPath, "trunk", "main.go", "package main\n")
	trunk, err := branchHead(ctx, repo.LocalPath, "trunk")
	require.NoError(t, err)
	commitFile(t, repo.LocalPath, "feature/x", "x.go", "package x\n")

	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, "feature/x", false))

	require.Equal(t, []string{"refs/heads/trunk", "refs/heads/feature/x"}, receivedRefs(t, refLog))
	upstreamTrunk, err := branchHead(ctx, upstream, "trunk")
	require.NoError(t, err)
	require.Equal(t, trunk, upstreamTrunk)
}

func TestPushToNonEmptyUpstreamOnlyPushesTheBranch(t *testing.T) {
	ctx := context.Background()
	service, repo, upstream, refLog := seedTestRepos(t)
	commitFile(t, upstream, "develop", "README.md", "hello\n")
	commitFile(t, repo.LocalPath, SpecsBranchName, ".helix/startup.sh", "#!/bin/bash\n")

	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))

	require.Equal(t, []string{"refs/heads/helix-specs"}, receivedRefs(t, refLog))
	main, err := branchHead(ctx, repo.LocalPath, "main")
	require.NoError(t, err)
	require.Empty(t, main, "no README commit is made when the upstream already has branches")
}

func TestDefaultBranchPushToEmptyUpstreamIsUnchanged(t *testing.T) {
	ctx := context.Background()
	service, repo, _, refLog := seedTestRepos(t)
	commitFile(t, repo.LocalPath, "main", "README.md", "hello\n")

	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, "main", false))

	require.Equal(t, []string{"refs/heads/main"}, receivedRefs(t, refLog))
}

// A repository row with no default branch yet (an empty external repo whose
// HEAD could not be detected) seeds main.
func TestEmptyUpstreamWithUnsetDefaultBranchSeedsMain(t *testing.T) {
	ctx := context.Background()
	service, repo, _, refLog := seedTestRepos(t)
	repo.DefaultBranch = ""
	commitFile(t, repo.LocalPath, SpecsBranchName, ".helix/startup.sh", "#!/bin/bash\n")

	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))

	require.Equal(t, []string{"refs/heads/main", "refs/heads/helix-specs"}, receivedRefs(t, refLog))
}

// Repointing the repository at another, empty upstream seeds it again even
// though the previous upstream was already known to have branches.
func TestRepointedToEmptyUpstreamPushesMainFirst(t *testing.T) {
	ctx := context.Background()
	service, repo, _, refLog := seedTestRepos(t)
	commitFile(t, repo.LocalPath, SpecsBranchName, ".helix/startup.sh", "#!/bin/bash\n")
	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))
	require.Equal(t, []string{"refs/heads/main", "refs/heads/helix-specs"}, receivedRefs(t, refLog))

	other, otherLog := emptyUpstream(t)
	repo.ExternalURL = "file://" + other
	commitFile(t, repo.LocalPath, SpecsBranchName, "design/notes.md", "notes")
	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))

	require.Equal(t, []string{"refs/heads/main", "refs/heads/helix-specs"}, receivedRefs(t, otherLog))
}

// An upstream deleted and recreated at the same URL is seeded again once the
// cached check has expired, and not re-checked before then.
func TestRecreatedUpstreamAtSameURLPushesMainFirstAfterExpiry(t *testing.T) {
	ctx := context.Background()
	service, repo, upstream, refLog := seedTestRepos(t)
	commitFile(t, repo.LocalPath, SpecsBranchName, ".helix/startup.sh", "#!/bin/bash\n")
	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))

	hooks := filepath.Join(upstream, "hooks", "pre-receive")
	hook, err := os.ReadFile(hooks)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(upstream))
	require.NoError(t, giteagit.InitRepository(ctx, upstream, true, "sha1"))
	require.NoError(t, os.WriteFile(hooks, hook, 0o755))
	require.NoError(t, os.Remove(refLog))

	// Within the expiry the cached result stands.
	commitFile(t, repo.LocalPath, SpecsBranchName, "design/a.md", "a")
	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))
	require.Equal(t, []string{"refs/heads/helix-specs"}, receivedRefs(t, refLog))

	// Recreate again, and age the cached entry past the expiry.
	require.NoError(t, os.RemoveAll(upstream))
	require.NoError(t, giteagit.InitRepository(ctx, upstream, true, "sha1"))
	require.NoError(t, os.WriteFile(hooks, hook, 0o755))
	require.NoError(t, os.Remove(refLog))
	service.upstreamHasBranches.Store(repo.ID, upstreamBranchesSeen{url: repo.ExternalURL, at: time.Now().Add(-upstreamHasBranchesTTL - time.Second)})

	commitFile(t, repo.LocalPath, SpecsBranchName, "design/b.md", "b")
	require.NoError(t, service.PushBranchToRemote(ctx, repo.ID, SpecsBranchName, false))
	require.Equal(t, []string{"refs/heads/main", "refs/heads/helix-specs"}, receivedRefs(t, refLog))
}

func emptyUpstream(t *testing.T) (path, refLog string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "upstream.git")
	refLog = filepath.Join(dir, "received.log")
	require.NoError(t, giteagit.InitRepository(context.Background(), path, true, "sha1"))
	hook := "#!/bin/sh\nwhile read old new ref; do echo \"$ref\" >> '" + refLog + "'; done\n"
	require.NoError(t, os.WriteFile(filepath.Join(path, "hooks", "pre-receive"), []byte(hook), 0o755))
	return path, refLog
}
