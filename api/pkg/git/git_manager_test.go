package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

// Commit IDs come from webhook payloads; an option-shaped value such as
// --output=<path> would make git write files on the API server.
func TestDiff_RejectsOptionShapedCommitIDs(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o600))
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "one")
	first := runGit(t, repo, "rev-parse", "HEAD")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o600))
	runGit(t, repo, "commit", "-q", "-am", "two")
	second := runGit(t, repo, "rev-parse", "HEAD")

	written := filepath.Join(t.TempDir(), "written-by-git")
	m := NewGitManager("", "")
	_, err := m.Diff(context.Background(), repo, "--output="+written, second)
	require.Error(t, err)
	require.NoFileExists(t, written)

	changes, err := m.Diff(context.Background(), repo, first, second)
	require.NoError(t, err)
	require.Len(t, changes, 1)
}

func TestCloneRepository_RefusesLocalRemotes(t *testing.T) {
	src := t.TempDir()
	runGit(t, src, "init", "-q", "--bare")

	for _, remote := range []string{src, "file://" + src, "ext::sh -c id"} {
		dest := filepath.Join(t.TempDir(), "clone")
		_, err := NewGitManager(remote, "").CloneRepository(context.Background(), dest)
		require.Error(t, err, remote)
		require.NoDirExists(t, filepath.Join(dest, ".git"), remote)
	}
}
