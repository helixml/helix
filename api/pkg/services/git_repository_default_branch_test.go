package services

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBareTestRepo creates a bare repo at repoPath with one commit on
// headBranch (HEAD) and the given extra branches.
func newBareTestRepo(t *testing.T, repoPath, headBranch string, branches ...string) {
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
	require.NoError(t, os.MkdirAll(filepath.Dir(repoPath), 0o755))
	runGitForBranchTest(t, work, "clone", "--bare", work, repoPath)
}

// newDefaultBranchTestRepo creates a bare repo at the service's storage path.
func newDefaultBranchTestRepo(t *testing.T, filestoreBase, repoID, headBranch string, branches ...string) string {
	t.Helper()
	repoPath := filepath.Join(filestoreBase, "git-repositories", repoID)
	newBareTestRepo(t, repoPath, headBranch, branches...)
	return repoPath
}

func headBranch(t *testing.T, repoPath string) string {
	t.Helper()
	return runGitForBranchTest(t, repoPath, "symbolic-ref", "--short", "HEAD")
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
	assert.Equal(t, "develop", headBranch(t, repoPath))

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
	assert.Equal(t, "main", headBranch(t, repoPath))
}

// An external repo that is not cloned yet is validated upstream, and the
// clone on first read points HEAD at the chosen branch.
func TestDefaultBranch_ExternalNotOnDisk(t *testing.T) {
	filestoreBase := t.TempDir()
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	newBareTestRepo(t, upstream, "main", "develop")
	st := &fakeStore{repo: &types.GitRepository{
		ID: "repo-uncloned", ExternalURL: upstream, IsExternal: true,
	}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	_, err := svc.UpdateRepository(t.Context(), "repo-uncloned", &types.GitRepositoryUpdateRequest{DefaultBranch: "nope"}, "")
	require.ErrorIs(t, err, ErrDefaultBranchNotFound)

	_, err = svc.UpdateRepository(t.Context(), "repo-uncloned", &types.GitRepositoryUpdateRequest{DefaultBranch: "develop"}, "")
	require.NoError(t, err)
	assert.Equal(t, "develop", st.repo.DefaultBranch)

	repo, err := svc.GetRepository(t.Context(), "repo-uncloned")
	require.NoError(t, err)
	assert.Equal(t, "develop", repo.DefaultBranch)
	assert.Equal(t, "develop", headBranch(t, filepath.Join(filestoreBase, "git-repositories", "repo-uncloned")))
}

// A branch that exists upstream but has not been mirrored yet is accepted and fetched.
func TestDefaultBranch_ExternalMirrorLagsUpstream(t *testing.T) {
	filestoreBase := t.TempDir()
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	newBareTestRepo(t, upstream, "main")
	repoPath := filepath.Join(filestoreBase, "git-repositories", "repo-lag")
	runGitForBranchTest(t, t.TempDir(), "clone", "--mirror", upstream, repoPath)
	runGitForBranchTest(t, upstream, "branch", "feature", "main")

	st := &fakeStore{repo: &types.GitRepository{
		ID: "repo-lag", ExternalURL: upstream, IsExternal: true, LocalPath: repoPath, DefaultBranch: "main",
	}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	updated, err := svc.UpdateRepository(t.Context(), "repo-lag", &types.GitRepositoryUpdateRequest{DefaultBranch: "feature"}, "")
	require.NoError(t, err)
	assert.Equal(t, "feature", updated.DefaultBranch)
	assert.Equal(t, "feature", headBranch(t, repoPath))

	repo, err := svc.GetRepository(t.Context(), "repo-lag")
	require.NoError(t, err)
	assert.Equal(t, "feature", repo.DefaultBranch)
}

// A HEAD pointing at a deleted branch must not be written back as the default.
func TestDefaultBranch_DanglingHeadNotPersisted(t *testing.T) {
	filestoreBase := t.TempDir()
	repoPath := newDefaultBranchTestRepo(t, filestoreBase, "repo-dangling", "main", "develop")
	runGitForBranchTest(t, repoPath, "symbolic-ref", "HEAD", "refs/heads/deleted")
	st := &fakeStore{repo: &types.GitRepository{
		ID: "repo-dangling", ExternalURL: "https://github.com/org/repo", IsExternal: true,
		LocalPath: repoPath, DefaultBranch: "gone",
	}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	repo, err := svc.GetRepository(t.Context(), "repo-dangling")
	require.NoError(t, err)
	assert.Equal(t, "gone", repo.DefaultBranch)
	assert.Equal(t, "gone", st.repo.DefaultBranch)
}

// A caller-set default branch survives the async clone and HEAD follows it.
func TestDefaultBranch_CloneRepositoryAsyncKeepsCallerBranch(t *testing.T) {
	filestoreBase := t.TempDir()
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	newBareTestRepo(t, upstream, "main", "develop")
	st := &cloneTestStore{repo: &types.GitRepository{
		ID: "repo-async", ExternalURL: upstream, IsExternal: true,
		Status: types.GitRepositoryStatusCloning, DefaultBranch: "develop",
	}}
	svc := NewGitRepositoryService(st, filestoreBase, "http://localhost:8080", "test", "test@test.com")

	done := make(chan string, 1)
	svc.CloneRepositoryAsync(st.repo, func(path string) { done <- path })
	var repoPath string
	select {
	case repoPath = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for clone")
	}

	repo, err := st.GetGitRepository(t.Context(), "repo-async")
	require.NoError(t, err)
	assert.Equal(t, "develop", repo.DefaultBranch)
	assert.Equal(t, "develop", headBranch(t, repoPath))
}

// A branch saved while detection was running must not be overwritten by the detected value.
func TestDefaultBranch_DetectionDoesNotOverwriteConcurrentSave(t *testing.T) {
	st := &fakeStore{repo: &types.GitRepository{ID: "repo-race", DefaultBranch: "develop"}}
	svc := NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@test.com")

	// Detection started when the stored branch was empty, then a save set "develop".
	effective := svc.persistDetectedDefaultBranch(t.Context(), "repo-race", "", "main")
	assert.Equal(t, "develop", effective)
	assert.Equal(t, "develop", st.repo.DefaultBranch)

	// With no concurrent change, the detected branch is saved.
	st.repo.DefaultBranch = ""
	effective = svc.persistDetectedDefaultBranch(t.Context(), "repo-race", "", "main")
	assert.Equal(t, "main", effective)
	assert.Equal(t, "main", st.repo.DefaultBranch)
}
