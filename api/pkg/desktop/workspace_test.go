package desktop

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestRepoWithRemote builds a temp workspace dir laid out the way
// the desktop expects (/home/retro/work, faked here via WORKSPACE_DIR)
// containing one repo named `repoName` with an initial commit, a bare
// `origin` remote in a sibling directory so push has somewhere to go,
// and (optionally) some uncommitted file changes on top.
//
// Returns the workspace root (set as WORKSPACE_DIR by the caller for
// the duration of the test) and the path to the cloned repo so the
// caller can poke at it directly.
func setupTestRepoWithRemote(t *testing.T, repoName string, dirty bool) (workspaceDir, repoDir, originDir string) {
	t.Helper()
	root := t.TempDir()
	workspaceDir = filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(workspaceDir, 0755))

	// Bare remote — `git push origin HEAD` lands here. Stored OUTSIDE
	// workspaceDir so findAllWorkspaces() doesn't see it as a workspace.
	originDir = filepath.Join(root, "origin.git")
	runIn(t, root, "git", "init", "--bare", originDir)

	repoDir = filepath.Join(workspaceDir, repoName)
	runIn(t, root, "git", "clone", originDir, repoDir)
	runIn(t, repoDir, "git", "config", "user.email", "test@test.com")
	runIn(t, repoDir, "git", "config", "user.name", "Test")
	// Seed with one commit so origin/HEAD resolves and rev-list works.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# init\n"), 0644))
	runIn(t, repoDir, "git", "add", "README.md")
	runIn(t, repoDir, "git", "commit", "-m", "Initial commit")
	runIn(t, repoDir, "git", "branch", "-M", "main")
	runIn(t, repoDir, "git", "push", "-u", "origin", "main")

	if dirty {
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("dirty\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker2.txt"), []byte("also dirty\n"), 0644))
	}
	return workspaceDir, repoDir, originDir
}

func runIn(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v failed: %s", name, args, string(out))
}

func newDesktopTestServer(t *testing.T, workspaceDir string) *Server {
	t.Helper()
	t.Setenv("WORKSPACE_DIR", workspaceDir)
	return &Server{}
}

// TestHandleWorkspaceStatus_Dirty covers the happy path of the modal-
// open check: workspace with one uncommitted file should return
// uncommitted_files=1 and is_dirty derivable on the caller side.
func TestHandleWorkspaceStatus_Dirty(t *testing.T) {
	workspaceDir, _, _ := setupTestRepoWithRemote(t, "testproj", true)
	s := newDesktopTestServer(t, workspaceDir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/workspace/status", nil)
	s.handleWorkspaceStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var resp WorkspaceStatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Repos, 1, "expected exactly one workspace: %+v", resp.Repos)
	r := resp.Repos[0]
	assert.Equal(t, "testproj", r.Name)
	assert.Equal(t, 2, r.UncommittedFiles, "two untracked files seeded")
	assert.Equal(t, 0, r.UnpushedCommits)
	assert.Equal(t, "main", r.Branch)
	assert.Empty(t, r.Error)
}

// TestHandleWorkspaceStatus_Clean asserts a freshly-cloned repo with
// no edits returns zero counts (the modal then suppresses its dirty
// panel and proceeds silently).
func TestHandleWorkspaceStatus_Clean(t *testing.T) {
	workspaceDir, _, _ := setupTestRepoWithRemote(t, "testproj", false)
	s := newDesktopTestServer(t, workspaceDir)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/workspace/status", nil)
	s.handleWorkspaceStatus(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp WorkspaceStatusResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Repos, 1)
	assert.Equal(t, 0, resp.Repos[0].UncommittedFiles)
	assert.Equal(t, 0, resp.Repos[0].UnpushedCommits)
}

// TestHandleWorkspaceCommitAndPush_Dirty is the core end-to-end test
// for the pre-fork safety net: seed a dirty repo, hit the endpoint
// with a commit message, verify a real commit landed AND was pushed
// to origin. This is the regression that catches the bug we hit in
// production (commit-msg hook rejecting non-conventional messages):
// if the message format ever stops working the assertion on origin's
// log will fail.
func TestHandleWorkspaceCommitAndPush_Dirty(t *testing.T) {
	workspaceDir, repoDir, originDir := setupTestRepoWithRemote(t, "testproj", true)
	s := newDesktopTestServer(t, workspaceDir)

	body, _ := json.Marshal(WorkspaceCommitRequest{
		Message: "chore(fork): pre-fork checkpoint before switching to qwen_code",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/workspace/commit-and-push", bytes.NewReader(body))
	s.handleWorkspaceCommitAndPush(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var resp WorkspaceCommitResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success, "expected overall success; got: %+v", resp.Repos)
	require.Len(t, resp.Repos, 1)
	r := resp.Repos[0]
	assert.Equal(t, "testproj", r.Name)
	assert.Equal(t, "committed", r.Action, "must report committed, not clean / failed")
	assert.Equal(t, 2, r.UncommittedFiles)
	assert.Empty(t, r.Error)

	// Local commit landed with the expected message.
	out, err := exec.Command("git", "-C", repoDir, "log", "--pretty=%s", "-1").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "chore(fork): pre-fork checkpoint before switching to qwen_code",
		strings.TrimSpace(string(out)))

	// Working tree clean post-commit.
	statusOut, err := exec.Command("git", "-C", repoDir, "status", "--porcelain").CombinedOutput()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(statusOut)), "working tree must be clean after commit")

	// And — the regression-catcher — the commit reached the remote.
	// `git -C <bare-repo> log` works on bare repos via HEAD.
	remoteOut, err := exec.Command("git", "-C", originDir, "log", "--pretty=%s", "-1", "main").CombinedOutput()
	require.NoError(t, err, "couldn't read origin's main: %s", string(remoteOut))
	assert.Equal(t, "chore(fork): pre-fork checkpoint before switching to qwen_code",
		strings.TrimSpace(string(remoteOut)),
		"the commit message visible on origin proves git push completed")
}

// TestHandleWorkspaceCommitAndPush_Clean asserts the no-op path:
// nothing to commit, nothing to push, action=clean, no error.
func TestHandleWorkspaceCommitAndPush_Clean(t *testing.T) {
	workspaceDir, _, _ := setupTestRepoWithRemote(t, "testproj", false)
	s := newDesktopTestServer(t, workspaceDir)

	body, _ := json.Marshal(WorkspaceCommitRequest{Message: "chore: noop"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/workspace/commit-and-push", bytes.NewReader(body))
	s.handleWorkspaceCommitAndPush(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp WorkspaceCommitResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	require.Len(t, resp.Repos, 1)
	assert.Equal(t, "clean", resp.Repos[0].Action)
}

// TestValidateBranchName_Sanitiser pins the security boundary: the
// validator must reject anything that could arg-smuggle into git
// (leading '-' = flag) and accept the names spec-tasks actually
// produce. This is the sanitiser CodeQL's go/command-injection rule
// relies on — if it ever stops being strict, the alerts come back.
func TestValidateBranchName_Sanitiser(t *testing.T) {
	cases := []struct {
		name    string
		wantErr bool
	}{
		// Spec-task / common branch name shapes — must be accepted.
		{"main", false},
		{"feature/000011-please-tell-me-a-joke", false},
		{"feat/some.thing-v1.2", false},
		{"helix-specs", false},
		{"a/b/c-d_e.f", false},

		// Arg-smuggling vectors — must be rejected.
		{"--orphan", true},
		{"-D", true},
		{"-x evil", true},
		{"--upload-pack=...", true},

		// Other unsafe shapes.
		{"", true},
		{"branch with spaces", true},
		{"branch;rm -rf /", true},
		{"branch\nwith\nnewlines", true},
		{"$(cmd)", true},
		{"`cmd`", true},
		{"branch?", true},
		{"branch*", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBranchName(tc.name)
			if tc.wantErr {
				assert.Error(t, err, "expected %q to be rejected", tc.name)
			} else {
				assert.NoError(t, err, "expected %q to be accepted", tc.name)
			}
		})
	}
}

// TestHandleWorkspaceCommitAndPush_RejectsBadBranch confirms the
// handler returns HTTP 400 (not a confusing 500 from a deep git
// error) when expected_branch fails validation. The fork-handler
// caller would treat this as the abort signal.
func TestHandleWorkspaceCommitAndPush_RejectsBadBranch(t *testing.T) {
	workspaceDir, _, _ := setupTestRepoWithRemote(t, "testproj", true)
	s := newDesktopTestServer(t, workspaceDir)

	body, _ := json.Marshal(WorkspaceCommitRequest{
		Message:        "chore(fork): pre-fork checkpoint",
		ExpectedBranch: "--orphan",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/workspace/commit-and-push", bytes.NewReader(body))
	s.handleWorkspaceCommitAndPush(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), "invalid expected_branch")
}

// TestHandleWorkspaceCommitAndPush_BranchRecovery covers the
// production failure where the parent's container is sitting on
// `main` (the post-clone default) with dirty files that morally
// belong to the spec task's feature branch. Without recovery, the
// push tries to update main and gets rejected by the remote's
// pre-receive hook. With ExpectedBranch set, the handler switches
// to the right branch first (git preserves dirty files across the
// checkout), THEN commits and pushes — landing on the right ref.
func TestHandleWorkspaceCommitAndPush_BranchRecovery(t *testing.T) {
	workspaceDir, repoDir, originDir := setupTestRepoWithRemote(t, "testproj", false)
	s := newDesktopTestServer(t, workspaceDir)

	// Set up a feature branch on origin (so checkout can find it via
	// origin/feature-x) and then put the local repo back on main with
	// dirty files — mirroring the post-clone state of a spec-task
	// container before the agent runs `git checkout`.
	runIn(t, repoDir, "git", "checkout", "-b", "feature-x")
	runIn(t, repoDir, "git", "push", "-u", "origin", "feature-x")
	runIn(t, repoDir, "git", "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("dirty on main\n"), 0644))

	// Mark the repo as primary so the recovery path engages (the
	// guard requires IsPrimary to avoid churning aux repos).
	t.Setenv("HELIX_PRIMARY_REPO_NAME", "testproj")

	body, _ := json.Marshal(WorkspaceCommitRequest{
		Message:        "chore(fork): pre-fork checkpoint before switching to qwen_code",
		ExpectedBranch: "feature-x",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/workspace/commit-and-push", bytes.NewReader(body))
	s.handleWorkspaceCommitAndPush(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var resp WorkspaceCommitResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success, "expected recovery to succeed: %+v", resp.Repos)
	require.Len(t, resp.Repos, 1)
	assert.Equal(t, "committed", resp.Repos[0].Action)

	// Local repo should now be on feature-x.
	branchOut, err := exec.Command("git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "feature-x", strings.TrimSpace(string(branchOut)),
		"recovery must switch HEAD to the expected branch")

	// And the commit must be on origin/feature-x (NOT origin/main —
	// that would have been rejected by a real pre-receive hook).
	remoteOut, err := exec.Command("git", "-C", originDir, "log", "--pretty=%s", "-1", "feature-x").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "chore(fork): pre-fork checkpoint before switching to qwen_code",
		strings.TrimSpace(string(remoteOut)))
}

// TestHandleWorkspaceCommitAndPush_BranchRecoveryFreshTask covers
// the brand-new-task variant of branch recovery: the spec task's
// expected branch doesn't exist yet anywhere (no local ref, no
// remote ref) because the agent hasn't pushed anything yet. The
// recovery must create the branch from the current HEAD so the
// dirty files have somewhere to land, and the subsequent push will
// create the remote ref. This is the typical state immediately
// after creating a fresh spec task — agent boots, user dirties
// files, user forks before the agent has its own chance to
// `git checkout -b`.
func TestHandleWorkspaceCommitAndPush_BranchRecoveryFreshTask(t *testing.T) {
	// `false` here means setupTestRepoWithRemote doesn't write the
	// marker files for us — we control them after seeding the brand-
	// new state, before dirtying.
	workspaceDir, repoDir, originDir := setupTestRepoWithRemote(t, "testproj", false)
	s := newDesktopTestServer(t, workspaceDir)

	// Brand-new task: no feature branch on origin, none locally,
	// container sitting on main with a dirty file.
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "marker.txt"), []byte("dirty on main\n"), 0644))
	t.Setenv("HELIX_PRIMARY_REPO_NAME", "testproj")

	body, _ := json.Marshal(WorkspaceCommitRequest{
		Message:        "chore(fork): pre-fork checkpoint before switching to qwen_code",
		ExpectedBranch: "feature/000099-brand-new",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/workspace/commit-and-push", bytes.NewReader(body))
	s.handleWorkspaceCommitAndPush(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	var resp WorkspaceCommitResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Success, "fresh-task recovery must succeed: %+v", resp.Repos)
	require.Len(t, resp.Repos, 1)
	assert.Equal(t, "committed", resp.Repos[0].Action)

	branchOut, err := exec.Command("git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, "feature/000099-brand-new", strings.TrimSpace(string(branchOut)))

	// The push should have CREATED the branch on origin.
	remoteOut, err := exec.Command("git", "-C", originDir, "log", "--pretty=%s", "-1", "feature/000099-brand-new").CombinedOutput()
	require.NoError(t, err, "remote branch must exist after push: %s", string(remoteOut))
	assert.Equal(t, "chore(fork): pre-fork checkpoint before switching to qwen_code",
		strings.TrimSpace(string(remoteOut)))
}

// TestHandleWorkspaceCommitAndPush_HookRejection is the focussed
// regression for the production failure: install a commit-msg hook
// that rejects the previous (non-conventional) message format and
// verify the handler surfaces a failed action with the hook's stderr
// in the Error field so the fork handler can abort and the user
// can see what went wrong.
func TestHandleWorkspaceCommitAndPush_HookRejection(t *testing.T) {
	workspaceDir, repoDir, _ := setupTestRepoWithRemote(t, "testproj", true)
	s := newDesktopTestServer(t, workspaceDir)

	// Install a commit-msg hook that requires "chore(" or "feat(" prefix.
	hooksDir := filepath.Join(repoDir, ".git", "hooks")
	require.NoError(t, os.MkdirAll(hooksDir, 0755))
	hook := `#!/bin/bash
msg=$(head -n1 "$1")
if [[ ! "$msg" =~ ^(chore|feat|fix|refactor|docs|test|style|perf|ci|build|revert)(\([a-z]+\))?:\ .+ ]]; then
  echo "ERROR: not a conventional commit: $msg" >&2
  exit 1
fi
`
	require.NoError(t, os.WriteFile(filepath.Join(hooksDir, "commit-msg"), []byte(hook), 0755))

	// Old-format message that the hook will reject.
	body, _ := json.Marshal(WorkspaceCommitRequest{
		Message: "Pre-fork checkpoint (switching to qwen_code)",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/workspace/commit-and-push", bytes.NewReader(body))
	s.handleWorkspaceCommitAndPush(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "endpoint returns 200 with per-repo failures inside")
	var resp WorkspaceCommitResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.False(t, resp.Success, "expected overall failure: %+v", resp.Repos)
	require.Len(t, resp.Repos, 1)
	assert.Equal(t, "failed", resp.Repos[0].Action)
	assert.Contains(t, resp.Repos[0].Error, "git commit",
		"the error must mention git commit so the fork handler's wrapped message stays diagnostic")
}
