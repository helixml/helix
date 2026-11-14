package services

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDefaultBranchTestRepo creates a bare repo at the service's storage path
// with the given branches, HEAD pointing at headBranch.
func newDefaultBranchTestRepo(t *testing.T, filestoreBase, repoID, headBranch string, branches ...string) string {
	t.Helper()
	work := t.TempDir()
	runGitForBranchTest(t, work, "init", "-b", headBranch)
	runGitForBranchTest(t, work, "config", "user.email", "test@test.com")
	runGitForBranchTest(t, work, "config", "user.name", "test")
	writeBranchTestFile(t, work, "README.md", "hello")
	runGitForBranchTest(t, work, "add", "README.md")
	runGitForBranchTest(t, work, "commit", "-m", "init")
	for _, b := range branches {
		runGitForBranchTest(t, work, "branch", b)
	}
	repoPath := filepath.Join(filestoreBase, "git-repositories", repoID)
	require.NoError(t, os.MkdirAll(filepath.Dir(repoPath), 0o755))
	runGitForBranchTest(t, work, "clone", "--bare", work, repoPath)
	return repoPath
}

func TestDefaultBranch_ExternalUserChoiceSurvivesGetRepository(t *testing.T) {
	filestoreBase := t.TempDir()
	repoPath := newDefaultBranchTestRepo(t, filestoreBase, "repo-ext", "main", "develop")
	st := &fakeStore{repo: &types.GitRepository{
		ID: "repo-ext", ExternalURL: "https://gitlab.example.com/org/repo.git",
		IsExternal: true, LocalPath: repoPath, DefaultBranch: "main",
	}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	updated, err := svc.UpdateRepository(t.Context(), "repo-ext", &types.GitRepositoryUpdateRequest{DefaultBranch: "develop"}, "")
	require.NoError(t, err)
	assert.Equal(t, "develop", updated.DefaultBranch)
	assert.Equal(t, "develop", runGitForBranchTest(t, repoPath, "symbolic-ref", "--short", "HEAD"))

	// Upstream HEAD in the mirror is reset back to main; the stored choice must still win.
	runGitForBranchTest(t, repoPath, "symbolic-ref", "HEAD", "refs/heads/main")
	repo, err := svc.GetRepository(t.Context(), "repo-ext")
	require.NoError(t, err)
	assert.Equal(t, "develop", repo.DefaultBranch)
	assert.Equal(t, "develop", st.repo.DefaultBranch)
}

func TestDefaultBranch_ExternalStaleBranchIsCorrected(t *testing.T) {
	filestoreBase := t.TempDir()
	repoPath := newDefaultBranchTestRepo(t, filestoreBase, "repo-master", "master")
	st := &fakeStore{repo: &types.GitRepository{
		ID: "repo-master", ExternalURL: "https://github.com/org/repo",
		IsExternal: true, LocalPath: repoPath, DefaultBranch: "main",
	}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	repo, err := svc.GetRepository(t.Context(), "repo-master")
	require.NoError(t, err)
	assert.Equal(t, "master", repo.DefaultBranch)
	assert.Equal(t, "master", st.repo.DefaultBranch)
}

func TestDefaultBranch_EmptyIsDetected(t *testing.T) {
	filestoreBase := t.TempDir()
	repoPath := newDefaultBranchTestRepo(t, filestoreBase, "repo-empty", "trunk")
	st := &fakeStore{repo: &types.GitRepository{ID: "repo-empty", LocalPath: repoPath}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	repo, err := svc.GetRepository(t.Context(), "repo-empty")
	require.NoError(t, err)
	assert.Equal(t, "trunk", repo.DefaultBranch)
}

func TestDefaultBranch_UpdateRejectsMissingBranch(t *testing.T) {
	filestoreBase := t.TempDir()
	repoPath := newDefaultBranchTestRepo(t, filestoreBase, "repo-local", "main")
	st := &fakeStore{repo: &types.GitRepository{ID: "repo-local", LocalPath: repoPath, DefaultBranch: "main"}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	_, err := svc.UpdateRepository(t.Context(), "repo-local", &types.GitRepositoryUpdateRequest{DefaultBranch: "nope"}, "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDefaultBranchNotFound))
	assert.Equal(t, "main", st.repo.DefaultBranch)
	assert.Equal(t, "main", runGitForBranchTest(t, repoPath, "symbolic-ref", "--short", "HEAD"))
}
