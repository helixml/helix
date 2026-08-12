package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestWorkspaceReviewRepresentsDualStateFileCoherently is the acceptance
// criterion the whole review contract exists for: the previous endpoint asked
// for a file's committed patch first and only fell back to its working patch
// when that was empty, so a file changed in both states showed whichever side
// happened to be queried first. Each scope must now answer for itself, and the
// combined scope must carry the file exactly once with both changes folded in.
func TestWorkspaceReviewRepresentsDualStateFileCoherently(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "both.txt"), []byte("one\ntwo\nthree\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", "both.txt")
	runReviewTestGit(t, repoDir, "commit", "-m", "seed both.txt")
	runReviewTestGit(t, repoDir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "both.txt"), []byte("one\ntwo-branch\nthree\n"), 0o644))
	runReviewTestGit(t, repoDir, "commit", "-am", "branch edit")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "both.txt"), []byte("one\ntwo-branch\nthree-worktree\n"), 0o644))

	byID := requestReviewSources(t, server, workspace, "main")

	all := byID[types.WorkspaceReviewSourceAll]
	require.Len(t, all.Files, 1, "the file must appear once, not once per scope")
	assert.Equal(t, "both.txt", all.Files[0].Path)
	assert.Equal(t, 2, all.Files[0].Additions)
	assert.Equal(t, 2, all.Files[0].Deletions)

	branch := byID[types.WorkspaceReviewSourceBranch]
	require.Len(t, branch.Files, 1)
	assert.Equal(t, 1, branch.Files[0].Additions)

	working := byID[types.WorkspaceReviewSourceWorkingTree]
	require.Len(t, working.Files, 1)
	assert.Equal(t, 1, working.Files[0].Additions)

	assert.Contains(t, all.Patch, "+three-worktree")
	assert.NotContains(t, branch.Patch, "three-worktree")
}

// TestWorkspaceReviewClassifiesRenamesDeletesAndBinaries covers the change
// kinds a lossy per-file projection used to flatten into "modified".
func TestWorkspaceReviewClassifiesRenamesDeletesAndBinaries(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "old-name.txt"), []byte("keep\nthis\ncontent\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "doomed.txt"), []byte("delete me\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", ".")
	runReviewTestGit(t, repoDir, "commit", "-m", "seed")
	runReviewTestGit(t, repoDir, "checkout", "-b", "feature")
	runReviewTestGit(t, repoDir, "mv", "old-name.txt", "new-name.txt")
	runReviewTestGit(t, repoDir, "rm", "-q", "doomed.txt")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "blob.bin"), []byte{0x00, 0x01, 0x02, 0x00, 0xff}, 0o644))
	runReviewTestGit(t, repoDir, "add", ".")
	runReviewTestGit(t, repoDir, "commit", "-m", "rename, delete, binary")

	byID := requestReviewSources(t, server, workspace, "main")
	byPath := make(map[string]types.InteractionCodeChangeFile)
	for _, file := range byID[types.WorkspaceReviewSourceBranch].Files {
		byPath[file.Path] = file
	}

	require.Contains(t, byPath, "new-name.txt")
	assert.Equal(t, "renamed", byPath["new-name.txt"].Kind)
	assert.Equal(t, "old-name.txt", byPath["new-name.txt"].OldPath)
	require.Contains(t, byPath, "doomed.txt")
	assert.Equal(t, "deleted", byPath["doomed.txt"].Kind)
	require.Contains(t, byPath, "blob.bin")
	assert.Equal(t, "added", byPath["blob.bin"].Kind)
	assert.True(t, byPath["blob.bin"].Binary, "binary files must not be reported with line counts")
}

// TestWorkspaceReviewReportsTruncationInsteadOfPartialSummary guards the
// failure mode where a bounded git listing was read but its truncation flag
// discarded, presenting a partial change set as the complete one.
func TestWorkspaceReviewReportsTruncationInsteadOfPartialSummary(t *testing.T) {
	repoDir := setupTestGitRepo(t)

	// A patch comfortably larger than the server's own patch bound.
	line := strings.Repeat("x", 120) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "huge.txt"),
		[]byte(strings.Repeat(line, (workspacePatchLimit/len(line))+64)), 0o644))

	env, release, err := untrackedIndexEnv(context.Background(), repoDir)
	require.NoError(t, err)
	defer release()

	source, err := buildReviewSource(context.Background(), repoDir, "main",
		reviewRange{id: types.WorkspaceReviewSourceWorkingTree, from: "HEAD", includeUntracked: true}, false, env)
	require.NoError(t, err)
	assert.True(t, source.Truncated, "an over-limit patch must be reported as truncated")
	assert.LessOrEqual(t, len(source.Patch), workspacePatchLimit)
	assert.Equal(t, []string{"huge.txt"}, codeChangePaths(source.Files),
		"the file summary stays complete even when the patch preview is cut")
}

// TestWorkspaceReviewHonoursIgnoreWhitespace covers the toolbar toggle, which
// changes the git invocation and had no coverage at all.
func TestWorkspaceReviewHonoursIgnoreWhitespace(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "spaced.txt"), []byte("alpha\nbeta\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", "spaced.txt")
	runReviewTestGit(t, repoDir, "commit", "-m", "seed")
	// Reindent only — no semantic change.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "spaced.txt"), []byte("    alpha\n    beta\n"), 0o644))

	included := requestReviewSources(t, server, workspace, "main")
	assert.Equal(t, []string{"spaced.txt"},
		codeChangePaths(included[types.WorkspaceReviewSourceWorkingTree].Files))
	assert.Contains(t, included[types.WorkspaceReviewSourceWorkingTree].Patch, "+    alpha")

	req := httptest.NewRequest(http.MethodGet,
		"/workspace/review?workspace="+workspace+"&base=main&ignore_whitespace=true", nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceReview(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response types.WorkspaceReviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	for _, source := range response.Sources {
		if source.ID == types.WorkspaceReviewSourceWorkingTree {
			assert.NotContains(t, source.Patch, "+    alpha",
				"a whitespace-only change must drop out of the patch when filtered")
		}
	}
}

// TestWorkspaceReviewRefreshReflectsLaterEdits is the "test the next
// operation" case: a review is polled, so the second read after a further
// workspace edit must report the new state, not a cached hash.
func TestWorkspaceReviewRefreshReflectsLaterEdits(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "moving.txt"), []byte("first\n"), 0o644))
	first := requestReviewSources(t, server, workspace, "main")[types.WorkspaceReviewSourceAll]

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "moving.txt"), []byte("first\nsecond\n"), 0o644))
	second := requestReviewSources(t, server, workspace, "main")[types.WorkspaceReviewSourceAll]

	assert.NotEqual(t, first.PatchHash, second.PatchHash, "the refreshed patch must hash differently")
	assert.Contains(t, second.Patch, "+second")
	assert.Equal(t, 2, second.Files[0].Additions)

	// And a file deleted after the previous read stops being reported as added.
	require.NoError(t, os.Remove(filepath.Join(repoDir, "moving.txt")))
	third := requestReviewSources(t, server, workspace, "main")[types.WorkspaceReviewSourceAll]
	assert.NotContains(t, codeChangePaths(third.Files), "moving.txt")
}

// TestWorkspaceReviewOnCleanRepositoryReportsNoChanges distinguishes "clean" —
// which must be an empty, successful result — from a failure.
func TestWorkspaceReviewOnCleanRepositoryReportsNoChanges(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	byID := requestReviewSources(t, server, workspace, "main")

	require.Len(t, byID, 3)
	for id, source := range byID {
		assert.Empty(t, source.Files, "scope %s", id)
		assert.Empty(t, strings.TrimSpace(source.Patch), "scope %s", id)
		assert.False(t, source.Truncated, "scope %s", id)
		assert.Equal(t, 0, source.TotalAdditions, "scope %s", id)
	}
}

func TestWorkspaceFileRejectsUnsafePaths(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)
	require.NoError(t, os.Mkdir(filepath.Join(repoDir, "adir"), 0o755))

	for name, pathValue := range map[string]string{
		"absolute":          "/etc/passwd",
		"parent traversal":  "../../../etc/passwd",
		"embedded parent":   "sub/../../escape.txt",
		"encoded traversal": "%2e%2e%2fetc%2fpasswd",
		"empty":             "",
		"directory":         "adir",
		"dot":               ".",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/workspace/file?workspace="+workspace+"&path="+url.QueryEscape(pathValue), nil)
			w := httptest.NewRecorder()
			server.handleWorkspaceFile(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

// TestWorkspaceCheckpointCaptureHandlesUnbornHEAD covers a workspace cloned or
// initialised without a commit: seeding the temporary index from HEAD fails
// there, and an empty starting index is the correct behaviour rather than a
// failed capture.
// TestWorkspaceFileRefusesGitInternalsAndIgnoredContent is a disclosure
// regression test. Real-path containment keeps reads inside the workspace, but
// `.git` is inside the workspace: `.git/config` carries whatever credentials a
// remote URL embeds, and session read access reaches org members who are not
// the owner. Ignored files are the same class — the tree never lists them, so
// the read endpoint must not serve them either.
func TestWorkspaceFileRefusesGitInternalsAndIgnoredContent(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	runReviewTestGit(t, repoDir, "remote", "add", "origin",
		"https://x-access-token:ghp_TOKENVALUE@github.com/owner/repo.git")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".git", "credentials"),
		[]byte("https://user:hunter2@github.com\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("secret.env\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "secret.env"), []byte("APIKEY=sk-live-123\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", ".gitignore")
	runReviewTestGit(t, repoDir, "commit", "-m", "ignore secrets")

	for _, blocked := range []string{".git/config", ".git/credentials", ".git/HEAD", "secret.env"} {
		req := httptest.NewRequest(http.MethodGet,
			"/workspace/file?workspace="+workspace+"&path="+url.QueryEscape(blocked), nil)
		w := httptest.NewRecorder()
		server.handleWorkspaceFile(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code, "%s must not be readable", blocked)
		body := w.Body.String()
		assert.NotContains(t, body, "ghp_TOKENVALUE")
		assert.NotContains(t, body, "hunter2")
		assert.NotContains(t, body, "sk-live-123")
	}

	// A tracked file in the same workspace still reads normally.
	req := httptest.NewRequest(http.MethodGet, "/workspace/file?workspace="+workspace+"&path=README.md", nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceFile(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// TestWorkspaceFileReadsBrowsableContent covers the happy path of the read
// endpoint, which every previous test skipped: they all asserted rejections, so
// the success path — and binary/truncation detection with it — never ran.
func TestWorkspaceFileReadsBrowsableContent(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "hello.txt"), []byte("hello\nworld\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "blob.bin"), []byte{0x00, 0x01, 0xff, 0x00}, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "big.txt"),
		[]byte(strings.Repeat("a", workspaceFileLimit+2048)), 0o644))

	text := readWorkspaceFile(t, server, workspace, "hello.txt")
	assert.Equal(t, "hello\nworld\n", text.Contents)
	assert.Equal(t, int64(12), text.ByteLength)
	assert.False(t, text.Binary)
	assert.False(t, text.Truncated)
	assert.NotEmpty(t, text.ContentHash)

	binary := readWorkspaceFile(t, server, workspace, "blob.bin")
	assert.True(t, binary.Binary)
	assert.Empty(t, binary.Contents, "binary bytes must not be sent as JSON text")

	large := readWorkspaceFile(t, server, workspace, "big.txt")
	assert.True(t, large.Truncated, "a file over the read cap must report truncation")
	assert.Len(t, large.Contents, workspaceFileLimit)

	// The next read after an edit reflects it, with a different content hash.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "hello.txt"), []byte("changed\n"), 0o644))
	updated := readWorkspaceFile(t, server, workspace, "hello.txt")
	assert.Equal(t, "changed\n", updated.Contents)
	assert.NotEqual(t, text.ContentHash, updated.ContentHash)
}

func TestWorkspaceFileWriteSavesAndSupportsTheNextEdit(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)
	pathValue := filepath.Join(repoDir, "editable.txt")
	require.NoError(t, os.WriteFile(pathValue, []byte("first\n"), 0o640))
	runReviewTestGit(t, repoDir, "add", "editable.txt")

	opened := readWorkspaceFile(t, server, workspace, "editable.txt")
	firstSave := writeWorkspaceFile(t, server, types.WorkspaceFileWriteRequest{
		Workspace: workspace, Path: "editable.txt", Contents: "second\n",
		ExpectedContentHash: opened.ContentHash,
	})
	assert.Equal(t, "second\n", firstSave.Contents)
	assert.NotEqual(t, opened.ContentHash, firstSave.ContentHash)

	secondSave := writeWorkspaceFile(t, server, types.WorkspaceFileWriteRequest{
		Workspace: workspace, Path: "editable.txt", Contents: "third\n",
		ExpectedContentHash: firstSave.ContentHash,
	})
	assert.Equal(t, "third\n", secondSave.Contents)
	assert.Equal(t, secondSave, readWorkspaceFile(t, server, workspace, "editable.txt"))
	info, err := os.Stat(pathValue)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

func TestWorkspaceFileWriteRejectsStaleContent(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)
	pathValue := filepath.Join(repoDir, "moving.txt")
	require.NoError(t, os.WriteFile(pathValue, []byte("opened\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", "moving.txt")
	opened := readWorkspaceFile(t, server, workspace, "moving.txt")
	require.NoError(t, os.WriteFile(pathValue, []byte("agent edit\n"), 0o644))

	body, err := json.Marshal(types.WorkspaceFileWriteRequest{
		Workspace: workspace, Path: "moving.txt", Contents: "browser edit\n",
		ExpectedContentHash: opened.ContentHash,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/workspace/file", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleWorkspaceFile(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
	contents, err := os.ReadFile(pathValue)
	require.NoError(t, err)
	assert.Equal(t, "agent edit\n", string(contents), "a stale browser must not overwrite the agent's edit")
}

func TestWorkspaceFileWriteRejectsUneditableContent(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("secret.env\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "secret.env"), []byte("secret\n"), 0o600))
	runReviewTestGit(t, repoDir, "add", ".gitignore")

	for name, request := range map[string]types.WorkspaceFileWriteRequest{
		"ignored": {
			Workspace: workspace, Path: "secret.env", Contents: "changed\n",
			ExpectedContentHash: hashString("secret\n"),
		},
		"oversized": {
			Workspace: workspace, Path: ".gitignore", Contents: strings.Repeat("x", workspaceFileLimit+1),
			ExpectedContentHash: hashString("secret.env\n"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(request)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPut, "/workspace/file", bytes.NewReader(body))
			w := httptest.NewRecorder()
			server.handleWorkspaceFile(w, req)
			assert.GreaterOrEqual(t, w.Code, 400)
		})
	}
}

// TestWorkspaceFilesListsTrackedAndUntrackedOnly covers the tree endpoint,
// which had no coverage at all.
func TestWorkspaceFilesListsTrackedAndUntrackedOnly(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "src", "deep"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "src", "deep", "mod.go"), []byte("package deep\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte("build/\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "build"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "build", "out.js"), []byte("//built\n"), 0o644))

	req := httptest.NewRequest(http.MethodGet, "/workspace/files?workspace="+workspace, nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceFiles(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var response types.WorkspaceFilesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	kinds := make(map[string]string, len(response.Entries))
	for _, entry := range response.Entries {
		kinds[entry.Path] = entry.Kind
	}

	assert.Equal(t, "file", kinds["src/deep/mod.go"])
	assert.Equal(t, "directory", kinds["src"], "intermediate directories must be derived")
	assert.Equal(t, "directory", kinds["src/deep"])
	assert.NotContains(t, kinds, "build/out.js", "ignored output is out of scope")
	assert.NotContains(t, kinds, ".git")
	assert.False(t, response.Truncated)
	assert.Equal(t, workspace, response.Workspace)
}

func TestListWorkspaceSkillsUsesProjectPrecedenceAndFollowsSymlinks(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()

	writeSkill := func(root, directory, name, description string) string {
		t.Helper()
		skillDir := filepath.Join(root, directory)
		require.NoError(t, os.MkdirAll(skillDir, 0o755))
		content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n# %s\n", name, description, name)
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))
		return skillDir
	}

	writeSkill(filepath.Join(homeDir, ".agents", "skills"), "review", "review", "Personal review")
	writeSkill(filepath.Join(workDir, ".agents", "skills"), "review", "review", "Project review")
	target := writeSkill(filepath.Join(homeDir, "skill-sources"), "deploy", "deploy", "Deploy safely")
	personalSkills := filepath.Join(homeDir, ".codex", "skills")
	require.NoError(t, os.MkdirAll(personalSkills, 0o755))
	require.NoError(t, os.Symlink(target, filepath.Join(personalSkills, "deploy")))

	skills, err := listWorkspaceSkills(workDir, homeDir)
	require.NoError(t, err)
	require.Len(t, skills, 2)
	assert.Equal(t, "deploy", skills[0].Name)
	assert.Equal(t, "personal", skills[0].Scope)
	assert.Equal(t, "review", skills[1].Name)
	assert.Equal(t, "Project review", skills[1].Description)
	assert.Equal(t, "project", skills[1].Scope)
}

func TestReadWorkspaceSkillRejectsInvalidFrontmatter(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "invalid")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Missing frontmatter\n"), 0o644))

	_, ok, err := readWorkspaceSkill(workspaceSkillRoot{path: root, scope: "project"}, filepath.Join(skillDir, "SKILL.md"))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestResolveReviewWorkspaceFallsBackAndRejectsUnknownNames(t *testing.T) {
	if _, _, err := resolveReviewWorkspace("definitely-not-a-workspace"); err == nil {
		t.Fatal("an unknown workspace name must not resolve")
	}
}

func readWorkspaceFile(t *testing.T, server *Server, workspace, path string) types.WorkspaceFileResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/workspace/file?workspace="+workspace+"&path="+url.QueryEscape(path), nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceFile(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response types.WorkspaceFileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
}

func writeWorkspaceFile(t *testing.T, server *Server, request types.WorkspaceFileWriteRequest) types.WorkspaceFileResponse {
	t.Helper()
	body, err := json.Marshal(request)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/workspace/file", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleWorkspaceFile(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response types.WorkspaceFileResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
}

func TestWorkspaceCheckpointCaptureHandlesUnbornHEAD(t *testing.T) {
	repoDir := t.TempDir()
	runReviewTestGit(t, repoDir, "init", "-q", "-b", "main")
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "first.txt"), []byte("first\n"), 0o644))

	captureCheckpointRequest(t, server, workspace, "refs/helix/checkpoints/ses/int/before")
	assert.Contains(t,
		runReviewTestGit(t, repoDir, "show", "--name-only", "--format=", "refs/helix/checkpoints/ses/int/before"),
		"first.txt")
}

// TestConsecutiveCheckpointsStayIndependent is the per-turn receipt guarantee:
// a later edit must not rewrite the patch stored under an earlier turn.
func TestConsecutiveCheckpointsStayIndependent(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	captureCheckpointRequest(t, server, workspace, "refs/helix/checkpoints/ses/int1/before")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "turn-one.txt"), []byte("one\n"), 0o644))
	captureCheckpointRequest(t, server, workspace, "refs/helix/checkpoints/ses/int1/after")

	captureCheckpointRequest(t, server, workspace, "refs/helix/checkpoints/ses/int2/before")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "turn-two.txt"), []byte("two\n"), 0o644))
	captureCheckpointRequest(t, server, workspace, "refs/helix/checkpoints/ses/int2/after")

	// A third edit lands after both turns finished; neither receipt may move.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "turn-three.txt"), []byte("three\n"), 0o644))

	first := checkpointDiff(t, server, workspace, "refs/helix/checkpoints/ses/int1/before", "refs/helix/checkpoints/ses/int1/after")
	second := checkpointDiff(t, server, workspace, "refs/helix/checkpoints/ses/int2/before", "refs/helix/checkpoints/ses/int2/after")

	assert.Equal(t, []string{"turn-one.txt"}, codeChangePaths(first.Files))
	assert.Equal(t, []string{"turn-two.txt"}, codeChangePaths(second.Files))
	assert.NotEqual(t, first.PatchHash, second.PatchHash)
}

func TestPruneSessionCheckpointRefsBoundsRetention(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	workspace := useReviewTestWorkspace(t, repoDir)
	server := newTestServer(t)

	for index := 0; index < checkpointRefsPerSession+4; index++ {
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "churn.txt"),
			[]byte(fmt.Sprintf("turn %d\n", index)), 0o644))
		captureCheckpointRequest(t, server, workspace,
			fmt.Sprintf("refs/helix/checkpoints/ses/int%04d/after", index))
	}

	refs := strings.Fields(runReviewTestGit(t, repoDir, "for-each-ref", "--format=%(refname)", "refs/helix/checkpoints/ses"))
	assert.LessOrEqual(t, len(refs), checkpointRefsPerSession,
		"checkpoint refs must not accumulate without bound in the user's repository")
	assert.Contains(t, refs, fmt.Sprintf("refs/helix/checkpoints/ses/int%04d/after", checkpointRefsPerSession+3),
		"the newest checkpoint must survive pruning")

	// Refs for a different session are not collateral damage.
	captureCheckpointRequest(t, server, workspace, "refs/helix/checkpoints/other/int1/after")
	assert.Contains(t,
		runReviewTestGit(t, repoDir, "for-each-ref", "--format=%(refname)", "refs/helix/checkpoints/other"),
		"refs/helix/checkpoints/other/int1/after")
}

// TestUntrackedIndexLeavesUserGitStateUntouched pins the invariant that makes
// the single-pass untracked diff safe: it runs entirely inside a throwaway
// index and must not disturb what the user (or Zed) has staged.
func TestUntrackedIndexLeavesUserGitStateUntouched(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "staged.txt"), []byte("staged\n"), 0o644))
	runReviewTestGit(t, repoDir, "add", "staged.txt")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("untracked\n"), 0o644))

	statusBefore := runReviewTestGit(t, repoDir, "status", "--porcelain=v1")
	indexBefore := runReviewTestGit(t, repoDir, "write-tree")

	env, release, err := untrackedIndexEnv(context.Background(), repoDir)
	require.NoError(t, err)
	files, _, err := diffFileSummary(context.Background(), repoDir, "HEAD", "", env)
	require.NoError(t, err)
	release()

	assert.Contains(t, codeChangePaths(files), "untracked.txt")
	assert.Contains(t, codeChangePaths(files), "staged.txt")
	assert.Equal(t, statusBefore, runReviewTestGit(t, repoDir, "status", "--porcelain=v1"))
	assert.Equal(t, indexBefore, runReviewTestGit(t, repoDir, "write-tree"))
}

func requestReviewSources(t *testing.T, server *Server, workspace, base string) map[string]types.WorkspaceReviewSource {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/workspace/review?workspace="+workspace+"&base="+base, nil)
	w := httptest.NewRecorder()
	server.handleWorkspaceReview(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response types.WorkspaceReviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	byID := make(map[string]types.WorkspaceReviewSource, len(response.Sources))
	for _, source := range response.Sources {
		byID[source.ID] = source
	}
	return byID
}

func checkpointDiff(t *testing.T, server *Server, workspace, beforeRef, afterRef string) types.WorkspaceCheckpointDiffResponse {
	t.Helper()
	body, err := json.Marshal(types.WorkspaceCheckpointDiffRequest{
		Workspace: workspace, BeforeRef: beforeRef, AfterRef: afterRef,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/workspace/checkpoints/diff", bytes.NewReader(body))
	w := httptest.NewRecorder()
	server.handleWorkspaceCheckpointDiff(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response types.WorkspaceCheckpointDiffResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
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
