// Package desktop provides desktop integration for Helix sandboxes.
package desktop

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// isGitRepo checks if a directory is a git repository
// Supports both regular repos (.git is a directory) and worktrees (.git is a file)
func isGitRepo(dir string) bool {
	gitPath := filepath.Join(dir, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return false
	}
	// .git can be a directory (normal repo) or a file (git worktree)
	return info.IsDir() || info.Mode().IsRegular()
}

// WorkspaceInfo represents information about a git workspace/repository
type WorkspaceInfo = types.WorkspaceInfo

// WorkspacesResponse is the response from the /workspaces endpoint
type WorkspacesResponse = types.WorkspacesResponse

const agentWorkspaceRoot = "/home/retro/work"

// handleWorkspaces handles GET /workspaces requests
// Returns a list of all git repositories in the workspace directory
func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workspaces := findAllWorkspaces()

	response := WorkspacesResponse{
		Workspaces: workspaces,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// findAllWorkspaces finds all git repositories in the workspace directory.
//
// Resolution order: WORKSPACE_DIR env var (explicit override — used by
// the container executor AND by unit tests for isolation) THEN the
// default /home/retro/work. Without this priority, tests that set
// WORKSPACE_DIR to a temp dir would still pick up the dev machine's
// real /home/retro/work and do destructive git ops against actual
// repos.
func findAllWorkspaces() []WorkspaceInfo {
	var workspaces []WorkspaceInfo

	workDirs := []string{
		os.Getenv("WORKSPACE_DIR"),
		"/home/retro/work",
	}

	var baseDir string
	for _, dir := range workDirs {
		if dir != "" {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				baseDir = dir
				break
			}
		}
	}

	if baseDir == "" {
		return workspaces
	}

	// Get primary repo name from environment
	primaryRepoName := os.Getenv("HELIX_PRIMARY_REPO_NAME")

	// Check if baseDir itself is a git repo
	if isGitRepo(baseDir) {
		ws := getWorkspaceInfo(baseDir, agentWorkspaceRoot, filepath.Base(baseDir), primaryRepoName)
		workspaces = append(workspaces, ws)
		return workspaces
	}

	// Otherwise, look for git repos in subdirectories
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return workspaces
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := filepath.Join(baseDir, entry.Name())
		if isGitRepo(subdir) {
			ws := getWorkspaceInfo(subdir, filepath.Join(agentWorkspaceRoot, entry.Name()), entry.Name(), primaryRepoName)
			workspaces = append(workspaces, ws)
		}
	}

	return workspaces
}

// getWorkspaceInfo builds workspace info for a git repository
func getWorkspaceInfo(repoPath, agentPath, name, primaryRepoName string) WorkspaceInfo {
	ws := WorkspaceInfo{
		Name:      name,
		Path:      repoPath,
		AgentPath: agentPath,
		IsPrimary: name == primaryRepoName,
	}

	// Get current branch
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = repoPath
	if branchOut, err := branchCmd.Output(); err == nil {
		ws.CurrentBranch = strings.TrimSpace(string(branchOut))
	}

	// Check if helix-specs branch exists
	specsCmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/helix-specs")
	specsCmd.Dir = repoPath
	if err := specsCmd.Run(); err == nil {
		ws.HasHelixSpecs = true
	} else {
		// Also check remote
		specsCmd = exec.Command("git", "show-ref", "--verify", "--quiet", "refs/remotes/origin/helix-specs")
		specsCmd.Dir = repoPath
		if err := specsCmd.Run(); err == nil {
			ws.HasHelixSpecs = true
		}
	}

	return ws
}

// findWorkspaceByNameFunc is the function used to find workspaces by name.
// It can be overridden in tests.
var findWorkspaceByNameFunc = findWorkspaceByName

// findWorkspaceByName finds a workspace by its directory name
func findWorkspaceByName(name string) string {
	// Get the base work directory
	workDirs := []string{
		"/home/retro/work",
		os.Getenv("WORKSPACE_DIR"),
	}

	for _, baseDir := range workDirs {
		if baseDir == "" {
			continue
		}
		info, err := os.Stat(baseDir)
		if err != nil || !info.IsDir() {
			continue
		}

		// Check if baseDir itself matches
		if filepath.Base(baseDir) == name && isGitRepo(baseDir) {
			return baseDir
		}

		// Check subdirectories
		subdir := filepath.Join(baseDir, name)
		if isGitRepo(subdir) {
			return subdir
		}
	}

	return ""
}
