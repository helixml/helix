package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type contentsRepo struct {
	t      *testing.T
	dir    string
	bare   string
	work   string
	svc    *GitRepositoryService
	repoID string
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Seed", "GIT_AUTHOR_EMAIL=seed@example.com",
		"GIT_COMMITTER_NAME=Seed", "GIT_COMMITTER_EMAIL=seed@example.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}

// newContentsRepo creates a bare repo whose master branch holds the files
// that seed writes into a scratch work tree.
func newContentsRepo(t *testing.T, seed func(work string)) *contentsRepo {
	t.Helper()
	dir := t.TempDir()
	r := &contentsRepo{
		t:      t,
		dir:    dir,
		bare:   filepath.Join(dir, "repo.git"),
		work:   filepath.Join(dir, "seed"),
		repoID: "repo",
	}
	gitRun(t, dir, "init", "-q", "--bare", "-b", "master", r.bare)
	gitRun(t, dir, "init", "-q", "-b", "master", r.work)
	require.NoError(t, os.WriteFile(filepath.Join(r.work, "README.md"), []byte("# repo\n"), 0o644))
	if seed != nil {
		seed(r.work)
	}
	gitRun(t, r.work, "add", "-A")
	gitRun(t, r.work, "commit", "-q", "-m", "seed")
	gitRun(t, r.work, "push", "-q", r.bare, "master:master")

	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetGitRepository(gomock.Any(), r.repoID).Return(&types.GitRepository{
		ID: r.repoID, LocalPath: r.bare, DefaultBranch: "master",
	}, nil).AnyTimes()
	r.svc = NewGitRepositoryService(mockStore, dir, "http://localhost:8080", "Helix", "helix@example.com")
	return r
}

func (r *contentsRepo) head(branch string) string {
	return gitRun(r.t, r.bare, "rev-parse", "refs/heads/"+branch)
}

func (r *contentsRepo) write(path, branch, content string) (string, error) {
	return r.svc.CreateOrUpdateFileContents(context.Background(), r.repoID, path, branch, []byte(content), "msg", "Alice", "alice@example.com")
}

// checkoutTree is the tree a working-copy commit of the same write would
// record: clone, write the file, git add, write-tree.
func (r *contentsRepo) checkoutTree(branch, path string, content string) string {
	clone := filepath.Join(r.t.TempDir(), "clone")
	gitRun(r.t, r.dir, "clone", "-q", "-b", branch, r.bare, clone)
	full := filepath.Join(clone, filepath.FromSlash(path))
	require.NoError(r.t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(r.t, os.WriteFile(full, []byte(content), 0o644))
	gitRun(r.t, clone, "add", "--", path)
	return gitRun(r.t, clone, "write-tree")
}

func TestCreateOrUpdateFileContentsMatchesWorkingCopyCommit(t *testing.T) {
	r := newContentsRepo(t, func(work string) {
		require.NoError(t, os.MkdirAll(filepath.Join(work, "docs"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(work, "docs", "guide.md"), []byte("old\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(work, "run.sh"), []byte("#!/bin/sh\n"), 0o755))
	})
	gitRun(t, r.bare, "branch", "feature", "master")

	for _, tc := range []struct{ name, branch, path, content string }{
		{"create at root", "master", "new.txt", "hello\n"},
		{"update existing", "master", "docs/guide.md", "new\n"},
		{"create nested dirs", "master", "a/b/c/deep.md", "deep\n"},
		{"create in existing dir", "master", "docs/other.md", "other\n"},
		{"update executable keeps mode", "master", "run.sh", "#!/bin/sh\necho hi\n"},
		{"write on another branch", "feature", "docs/guide.md", "feature\n"},
		{"unicode name", "master", "docs/ž ünï.md", "u\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := r.head(tc.branch)
			wantTree := r.checkoutTree(tc.branch, tc.path, tc.content)

			commit, err := r.write(tc.path, tc.branch, tc.content)
			require.NoError(t, err)

			assert.Equal(t, commit, r.head(tc.branch))
			assert.Equal(t, wantTree, gitRun(t, r.bare, "rev-parse", commit+"^{tree}"))
			assert.Equal(t, parent, gitRun(t, r.bare, "rev-parse", commit+"^"))
			assert.Equal(t, "Alice <alice@example.com>|Alice <alice@example.com>|msg",
				gitRun(t, r.bare, "show", "-s", "--format=%an <%ae>|%cn <%ce>|%s", commit))
			assert.Equal(t, strings.TrimSpace(tc.content), gitRun(t, r.bare, "show", commit+":"+tc.path))
		})
	}

	mode := gitRun(t, r.bare, "ls-tree", "master", "run.sh")
	assert.True(t, strings.HasPrefix(mode, "100755 "), mode)
	assert.Equal(t, "README.md", gitRun(t, r.bare, "ls-tree", "--name-only", "master", "README.md"))
}

func TestCreateOrUpdateFileContentsUnchangedReturnsHead(t *testing.T) {
	r := newContentsRepo(t, nil)
	head := r.head("master")
	commit, err := r.write("README.md", "master", "# repo\n")
	require.NoError(t, err)
	assert.Equal(t, head, commit)
	assert.Equal(t, head, r.head("master"))
}

func TestCreateOrUpdateFileContentsMissingBranchFails(t *testing.T) {
	r := newContentsRepo(t, nil)
	_, err := r.write("x.md", "nope", "x")
	require.Error(t, err)
	assert.Empty(t, gitRun(t, r.bare, "branch", "--list", "nope"))
}

func TestCreateOrUpdateFileContentsRejectsBadBranchNames(t *testing.T) {
	r := newContentsRepo(t, nil)
	for _, branch := range []string{"", "-x", "a..b", "*", "a b", "master:other", "refs/../x", "@", "a\nb"} {
		_, err := r.write("x.md", branch, "x")
		assert.ErrorIs(t, err, ErrInvalidBranchName, "branch %q", branch)
	}
}

// A branch can hold a symlink to .git (or a submodule) under a name the
// caller then writes through. Writing must never follow it: the commit is
// refused and no git command runs against attacker-chosen config.
func TestCreateOrUpdateFileContentsRefusesSymlinkAndSubmodulePaths(t *testing.T) {
	r := newContentsRepo(t, func(work string) {
		require.NoError(t, os.Symlink(".git/config", filepath.Join(work, "cfg")))
		require.NoError(t, os.Symlink(".git", filepath.Join(work, "d")))
		require.NoError(t, os.Symlink("/etc", filepath.Join(work, "abs")))
		require.NoError(t, os.MkdirAll(filepath.Join(work, "docs"), 0o755))
		require.NoError(t, os.Symlink("../.git/hooks", filepath.Join(work, "docs", "hooks")))
	})
	gitRun(t, r.work, "update-index", "--add", "--cacheinfo", "160000,"+r.head("master")+",sub")
	gitRun(t, r.work, "commit", "-q", "-m", "submodule")
	gitRun(t, r.work, "push", "-q", r.bare, "master:master")

	marker := filepath.Join(r.dir, "PWNED")
	poison := "[core]\n\tfsmonitor = touch " + marker + "\n"
	configBefore, err := os.ReadFile(filepath.Join(r.bare, "config"))
	require.NoError(t, err)
	head := r.head("master")

	for _, path := range []string{"cfg", "d/config", "D/config", "abs/passwd", "docs/hooks/pre-commit", "sub", "sub/x", "README.md/x", "docs"} {
		t.Run(path, func(t *testing.T) {
			_, err := r.write(path, "master", poison)
			assert.ErrorIs(t, err, ErrRepoPathBlocked)
			assert.Equal(t, head, r.head("master"))
		})
	}

	configAfter, err := os.ReadFile(filepath.Join(r.bare, "config"))
	require.NoError(t, err)
	assert.Equal(t, string(configBefore), string(configAfter))
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "fsmonitor command ran")
}

func TestCreateOrUpdateFileContentsConcurrentWritesNeverLoseUpdates(t *testing.T) {
	r := newContentsRepo(t, nil)
	const writers = 8
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = r.write(fmt.Sprintf("f%d.txt", i), "master", fmt.Sprintf("%d", i))
		}()
	}
	wg.Wait()

	files := gitRun(t, r.bare, "ls-tree", "--name-only", "master")
	for i, err := range errs {
		name := fmt.Sprintf("f%d.txt", i)
		if err != nil {
			assert.ErrorIs(t, err, ErrBranchMoved)
			continue
		}
		assert.Contains(t, files, name, "a successful write was lost")
	}
}

func TestCommitFileOnParentRefusesStaleParent(t *testing.T) {
	r := newContentsRepo(t, nil)
	stale := r.head("master")
	moved, err := r.write("moved.txt", "master", "moved")
	require.NoError(t, err)

	_, _, err = commitFileOnParent(context.Background(), r.bare, "master", stale, "late.txt", []byte("late"), "A", "a@example.com", "late")
	assert.ErrorIs(t, err, ErrBranchMoved)
	assert.Equal(t, moved, r.head("master"))

	_, _, err = commitFileOnParent(context.Background(), r.bare, "master", "", "late.txt", []byte("late"), "A", "a@example.com", "late")
	assert.ErrorIs(t, err, ErrBranchMoved, "creating an existing branch must not overwrite it")
	assert.Equal(t, moved, r.head("master"))
}

func TestCommitFileToBareBranchCreatesMissingBranch(t *testing.T) {
	r := newContentsRepo(t, nil)
	commit, changed, err := CommitFileToBareBranch(context.Background(), r.bare, "fresh", "a/b.md", []byte("b"), "A", "a@example.com", "m")
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, commit, r.head("fresh"))
	assert.Equal(t, "a/b.md", gitRun(t, r.bare, "ls-tree", "-r", "--name-only", "fresh"))
}

// mktree reads one entry per line unless -z is set, so a name holding a
// newline used to smuggle a second, attacker-chosen entry into the tree.
func TestWriteTreeKeepsNewlineNamesAsOneEntry(t *testing.T) {
	r := newContentsRepo(t, nil)
	ctx := context.Background()
	blob, err := WriteBlob(ctx, r.bare, []byte("x"))
	require.NoError(t, err)
	name := "a.png\n100644 blob " + blob + "\tinjected"

	tree, err := WriteTree(ctx, r.bare, []TreeEntry{{Mode: "100644", Type: "blob", SHA: blob, Name: name}})
	require.NoError(t, err)
	entries, err := ListTreeEntries(ctx, r.bare, tree)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, name, entries[0].Name)

	for _, bad := range []string{"", ".", "..", "a/b", "a\x00b"} {
		_, err := WriteTree(ctx, r.bare, []TreeEntry{{Mode: "100644", Type: "blob", SHA: blob, Name: bad}})
		assert.Error(t, err, "name %q", bad)
	}
}
