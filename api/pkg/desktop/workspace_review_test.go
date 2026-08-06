package desktop

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useReviewTestWorkspace(t *testing.T, repoDir string) string {
	t.Helper()
	workspace := filepath.Base(repoDir)
	original := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == workspace {
			return repoDir
		}
		return ""
	}
	t.Cleanup(func() { findWorkspaceByNameFunc = original })
	return workspace
}

func runReviewTestGit(t *testing.T, repoDir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
	return string(out)
}

func TestWorkspaceReviewSeparatesAllBranchAndWorkingTreeChanges(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	runReviewTestGit(t, repoDir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "committed.txt"), []byte("committed\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", "committed.txt")
	runReviewTestGit(t, repoDir, "commit", "-m", "add committed file")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "staged.txt"), []byte("staged\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", "staged.txt")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test\nworking\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("untracked\n"), 0o644))

	req := httptest.NewRequest(http.MethodGet, "/workspace/review?workspace="+workspace+"&base=main", nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceReview(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response types.WorkspaceReviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Sources, 3)
	byID := make(map[string]types.WorkspaceReviewSource)
	for _, source := range response.Sources {
		byID[source.ID] = source
	}
	assert.ElementsMatch(t, []string{"README.md", "committed.txt", "staged.txt", "untracked.txt"}, codeChangePaths(byID[types.WorkspaceReviewSourceAll].Files))
	assert.Equal(t, []string{"committed.txt"}, codeChangePaths(byID[types.WorkspaceReviewSourceBranch].Files))
	assert.ElementsMatch(t, []string{"README.md", "staged.txt", "untracked.txt"}, codeChangePaths(byID[types.WorkspaceReviewSourceWorkingTree].Files))
	assert.Contains(t, byID[types.WorkspaceReviewSourceAll].Patch, "+untracked")
}

func TestWorkspaceReviewRejectsUnknownExplicitBase(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/workspace/review?workspace="+workspace+"&base=does-not-exist", nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceReview(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWorkspaceCheckpointCaptureDoesNotModifyUserGitState(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Test\nbefore\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", "README.md")

	headBefore := runReviewTestGit(t, repoDir, "rev-parse", "HEAD")
	statusBefore := runReviewTestGit(t, repoDir, "status", "--porcelain=v1")
	indexBefore := runReviewTestGit(t, repoDir, "write-tree")

	beforeRef := "refs/helix/checkpoints/session/interaction/before"
	captureCheckpointRequest(t, server, workspace, beforeRef)
	assert.Equal(t, headBefore, runReviewTestGit(t, repoDir, "rev-parse", "HEAD"))
	assert.Equal(t, statusBefore, runReviewTestGit(t, repoDir, "status", "--porcelain=v1"))
	assert.Equal(t, indexBefore, runReviewTestGit(t, repoDir, "write-tree"))

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "after.txt"), []byte("after\n"), 0o644))
	afterRef := "refs/helix/checkpoints/session/interaction/after"
	captureCheckpointRequest(t, server, workspace, afterRef)

	body, err := json.Marshal(types.WorkspaceCheckpointDiffRequest{
		Workspace: workspace, BeforeRef: beforeRef, AfterRef: afterRef,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/workspace/checkpoints/diff", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleWorkspaceCheckpointDiff(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var diff types.WorkspaceCheckpointDiffResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &diff))
	require.Len(t, diff.Files, 1)
	assert.Equal(t, "after.txt", diff.Files[0].Path)
	assert.Equal(t, "added", diff.Files[0].Kind)
	assert.Contains(t, diff.Patch, "+after")
}

func TestWorkspaceFileRejectsSymlinkEscape(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)
	outsideDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outsideDir, "secret.txt"), filepath.Join(repoDir, "escape.txt")))

	req := httptest.NewRequest(http.MethodGet, "/workspace/file?workspace="+workspace+"&path=escape.txt", nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceFile(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotContains(t, w.Body.String(), "secret")
}

func captureCheckpointRequest(t *testing.T, server *Server, workspace, ref string) {
	t.Helper()
	body, err := json.Marshal(types.WorkspaceCheckpointCaptureRequest{Workspace: workspace, Ref: ref})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/workspace/checkpoints/capture", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleWorkspaceCheckpointCapture(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func codeChangePaths(files []types.InteractionCodeChangeFile) []string {
	paths := make([]string, len(files))
	for index, file := range files {
		paths[index] = file.Path
	}
	return paths
}
