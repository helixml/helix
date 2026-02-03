package desktop

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIfNoGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in PATH")
	}
}

func setupTestGitRepo(t *testing.T) string {
	skipIfNoGit(t)
	t.Helper()
	dir := t.TempDir()

	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644))
	runGit("add", "README.md")
	runGit("commit", "-m", "Initial commit")
	runGit("branch", "-M", "main")

	return dir
}

func TestIsGitRepo(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "valid git repo",
			setup: func(t *testing.T) string {
				return setupTestGitRepo(t)
			},
			expected: true,
		},
		{
			name: "empty directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expected: false,
		},
		{
			name: "non-existent directory",
			setup: func(t *testing.T) string {
				return "/nonexistent/path/that/does/not/exist"
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			result := isGitRepo(dir)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveBaseBranch(t *testing.T) {
	repoDir := setupTestGitRepo(t)

	tests := []struct {
		name       string
		baseBranch string
		expected   string
	}{
		{
			name:       "main branch exists",
			baseBranch: "main",
			expected:   "main",
		},
		{
			name:       "nonexistent branch falls back to main",
			baseBranch: "develop",
			expected:   "main",
		},
		{
			name:       "master requested but only main exists",
			baseBranch: "master",
			expected:   "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveBaseBranch(repoDir, tt.baseBranch)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveBaseBranch_NoValidBranch(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644))
	runGit("add", "file.txt")
	runGit("commit", "-m", "commit")
	runGit("branch", "-M", "feature")

	result := resolveBaseBranch(dir, "main")
	assert.Equal(t, "", result)
}

func newTestServer(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return &Server{logger: logger}
}

func TestHandleDiff_MethodNotAllowed(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/diff", nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleDiff_OnBaseBranch(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir), nil)
	w := httptest.NewRecorder()

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "main", resp.Branch)
	assert.Equal(t, "main", resp.BaseBranch)
	assert.Empty(t, resp.Files)
	assert.False(t, resp.HasUncommittedChanges)
}

func TestHandleDiff_WithChanges(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	runGit := func(args ...string) {
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
	}

	runGit("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new_file.txt"), []byte("new content\n"), 0644))
	runGit("add", "new_file.txt")
	runGit("commit", "-m", "Add new file")

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir), nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "feature", resp.Branch)
	assert.Equal(t, "main", resp.BaseBranch)
	assert.Len(t, resp.Files, 1)
	assert.Equal(t, "new_file.txt", resp.Files[0].Path)
	assert.Equal(t, "added", resp.Files[0].Status)
	assert.Equal(t, 1, resp.Files[0].Additions)
}

func TestHandleDiff_WithUncommittedChanges(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "uncommitted.txt"), []byte("uncommitted\n"), 0644))

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir), nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.True(t, resp.HasUncommittedChanges)
}

func TestHandleDiff_IncludeContent(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	runGit := func(args ...string) {
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
	}

	runGit("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new_file.txt"), []byte("line1\nline2\n"), 0644))
	runGit("add", "new_file.txt")
	runGit("commit", "-m", "Add new file")

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir)+"&include_content=true", nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Len(t, resp.Files, 1)
	assert.NotEmpty(t, resp.Files[0].Diff)
	assert.Contains(t, resp.Files[0].Diff, "+line1")
	assert.Contains(t, resp.Files[0].Diff, "+line2")
}

func TestHandleDiff_BaseBranchNotFound(t *testing.T) {
	skipIfNoGit(t)
	dir := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644))
	runGit("add", "file.txt")
	runGit("commit", "-m", "commit")
	runGit("branch", "-M", "feature")

	server := newTestServer(t)

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(dir) {
			return dir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(dir), nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "not found")
}

func TestHandleDiff_WorkspaceNotFound(t *testing.T) {
	server := newTestServer(t)

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace=nonexistent", nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Contains(t, resp.Error, "not found")
}

func TestHandleDiff_ModifiedFile(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	runGit := func(args ...string) {
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
	}

	runGit("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("# Modified\n\nNew content\n"), 0644))
	runGit("add", "README.md")
	runGit("commit", "-m", "Modify README")

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir), nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Len(t, resp.Files, 1)
	assert.Equal(t, "README.md", resp.Files[0].Path)
	assert.Equal(t, "modified", resp.Files[0].Status)
	assert.Greater(t, resp.Files[0].Additions, 0)
	assert.Greater(t, resp.Files[0].Deletions, 0)
}

func TestHandleDiff_DeletedFile(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	runGit := func(args ...string) {
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
	}

	runGit("checkout", "-b", "feature")
	runGit("rm", "README.md")
	runGit("commit", "-m", "Delete README")

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir), nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Len(t, resp.Files, 1)
	assert.Equal(t, "README.md", resp.Files[0].Path)
	assert.Equal(t, "deleted", resp.Files[0].Status)
}

func TestHandleDiff_PathFilter(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	runGit := func(args ...string) {
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
	}

	runGit("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file1.txt"), []byte("content1\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file2.txt"), []byte("content2\n"), 0644))
	runGit("add", ".")
	runGit("commit", "-m", "Add files")

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir)+"&path=file1.txt", nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.Len(t, resp.Files, 1)
	assert.Equal(t, "file1.txt", resp.Files[0].Path)
}

func TestHandleDiff_CustomBaseBranch(t *testing.T) {
	repoDir := setupTestGitRepo(t)
	server := newTestServer(t)

	runGit := func(args ...string) {
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
	}

	runGit("checkout", "-b", "develop")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "develop.txt"), []byte("develop\n"), 0644))
	runGit("add", ".")
	runGit("commit", "-m", "Develop commit")

	runGit("checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "feature.txt"), []byte("feature\n"), 0644))
	runGit("add", ".")
	runGit("commit", "-m", "Feature commit")

	origFindWorkspaceByName := findWorkspaceByNameFunc
	findWorkspaceByNameFunc = func(name string) string {
		if name == filepath.Base(repoDir) {
			return repoDir
		}
		return ""
	}
	defer func() { findWorkspaceByNameFunc = origFindWorkspaceByName }()

	req := httptest.NewRequest(http.MethodGet, "/diff?workspace="+filepath.Base(repoDir)+"&base=develop", nil)
	w := httptest.NewRecorder()

	server.handleDiff(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp DiffResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "develop", resp.BaseBranch)
	require.Len(t, resp.Files, 1)
	assert.Equal(t, "feature.txt", resp.Files[0].Path)
}
