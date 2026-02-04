// Package desktop provides desktop integration for Helix sandboxes.
package desktop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileDiff represents a single file's diff information
type FileDiff struct {
	// Path relative to the repository root
	Path string `json:"path"`
	// Status: "added", "modified", "deleted", "renamed", "copied"
	Status string `json:"status"`
	// OldPath is set for renamed files
	OldPath string `json:"old_path,omitempty"`
	// Additions is the number of lines added
	Additions int `json:"additions"`
	// Deletions is the number of lines deleted
	Deletions int `json:"deletions"`
	// Diff is the unified diff content (optional, only if include_content=true)
	Diff string `json:"diff,omitempty"`
	// IsBinary indicates if the file is binary
	IsBinary bool `json:"is_binary,omitempty"`
}

// DiffResponse is the response from the /diff endpoint
type DiffResponse struct {
	// Files is the list of changed files
	Files []FileDiff `json:"files"`
	// TotalAdditions across all files
	TotalAdditions int `json:"total_additions"`
	// TotalDeletions across all files
	TotalDeletions int `json:"total_deletions"`
	// Branch is the current branch name
	Branch string `json:"branch,omitempty"`
	// BaseBranch is the branch being compared against (usually main)
	BaseBranch string `json:"base_branch,omitempty"`
	// HasUncommittedChanges indicates if there are uncommitted changes
	HasUncommittedChanges bool `json:"has_uncommitted_changes"`
	// WorkDir is the working directory used
	WorkDir string `json:"work_dir,omitempty"`
	// Error message if something went wrong (partial results may still be returned)
	Error string `json:"error,omitempty"`
}

// handleDiff handles GET /diff requests
// Query params:
//   - base: base branch to compare against (default: "main")
//   - include_content: include full diff content for each file (default: false)
//   - path: filter to specific file path (optional)
//   - workspace: name of the workspace/repo to diff (optional, defaults to first found)
//   - helix_specs: if "true", diff the helix-specs branch instead of the current branch
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	baseBranch := r.URL.Query().Get("base")
	if baseBranch == "" {
		baseBranch = "main"
	}
	includeContent := r.URL.Query().Get("include_content") == "true"
	pathFilter := r.URL.Query().Get("path")
	workspaceName := r.URL.Query().Get("workspace")
	helixSpecs := r.URL.Query().Get("helix_specs") == "true"

	// Find the workspace directory
	var workDir string
	if workspaceName != "" {
		// Look for a specific workspace by name
		workDir = findWorkspaceByNameFunc(workspaceName)
		if workDir == "" {
			s.logger.Warn("specified workspace not found", "workspace", workspaceName)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DiffResponse{
				Files: []FileDiff{},
				Error: fmt.Sprintf("workspace '%s' not found", workspaceName),
			})
			return
		}
	} else {
		// Use default workspace finding logic
		workDir = findWorkspaceDir()
	}

	if workDir == "" {
		s.logger.Warn("no workspace directory found for diff")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(DiffResponse{
			Files: []FileDiff{},
			Error: "no workspace directory found",
		})
		return
	}

	s.logger.Info("generating diff",
		"work_dir", workDir,
		"base_branch", baseBranch,
		"include_content", includeContent,
		"path_filter", pathFilter,
		"workspace", workspaceName,
		"helix_specs", helixSpecs,
	)

	// For helix-specs mode, diff the helix-specs branch against the base branch
	// instead of diffing the current branch
	if helixSpecs {
		response := s.generateHelixSpecsDiff(workDir, baseBranch, includeContent, pathFilter)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	response := DiffResponse{
		Files:      []FileDiff{},
		BaseBranch: baseBranch,
		WorkDir:    workDir,
	}

	// Get current branch name
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = workDir
	branchOut, err := branchCmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get current branch: %v", err), http.StatusInternalServerError)
		return
	}
	response.Branch = strings.TrimSpace(string(branchOut))

	// Resolve the actual base branch ref (main, origin/main, master, origin/master)
	resolvedBase := resolveBaseBranch(workDir, baseBranch)

	// Check if on base branch - still show uncommitted changes
	onBaseBranch := response.Branch == baseBranch || response.Branch == "origin/"+baseBranch

	if !onBaseBranch && resolvedBase == "" {
		http.Error(w, fmt.Sprintf("base branch '%s' not found (tried %s, origin/%s)", baseBranch, baseBranch, baseBranch), http.StatusBadRequest)
		return
	}

	// Check for uncommitted changes (staged + unstaged + untracked)
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workDir
	statusOut, err := statusCmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get git status: %v", err), http.StatusInternalServerError)
		return
	}
	response.HasUncommittedChanges = len(strings.TrimSpace(string(statusOut))) > 0

	// Find the merge-base between current HEAD and the base branch
	var mergeBase string
	if !onBaseBranch {
		mergeBaseCmd := exec.Command("git", "merge-base", resolvedBase, "HEAD")
		mergeBaseCmd.Dir = workDir
		mergeBaseOut, err := mergeBaseCmd.Output()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to find merge-base between %s and HEAD: %v", resolvedBase, err), http.StatusInternalServerError)
			return
		}
		mergeBase = strings.TrimSpace(string(mergeBaseOut))
	}

	// Get diff against base branch (committed changes)
	// Skip if on base branch (no committed changes to show)
	var numstatOut []byte
	if !onBaseBranch {
		diffArgs := []string{"diff", "--numstat", mergeBase + "..HEAD"}
		if pathFilter != "" {
			diffArgs = append(diffArgs, "--", pathFilter)
		}
		numstatCmd := exec.Command("git", diffArgs...)
		numstatCmd.Dir = workDir
		numstatOut, _ = numstatCmd.Output()
	}

	// Parse numstat output: "additions\tdeletions\tfilename"
	if len(numstatOut) > 0 {
		lines := strings.Split(strings.TrimSpace(string(numstatOut)), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) >= 3 {
				fileDiff := FileDiff{
					Path:   parts[2],
					Status: "modified", // Will be refined below
				}

				// Parse additions/deletions (- means binary)
				if parts[0] == "-" {
					fileDiff.IsBinary = true
				} else {
					fmt.Sscanf(parts[0], "%d", &fileDiff.Additions)
					fmt.Sscanf(parts[1], "%d", &fileDiff.Deletions)
				}

				response.TotalAdditions += fileDiff.Additions
				response.TotalDeletions += fileDiff.Deletions
				response.Files = append(response.Files, fileDiff)
			}
		}
	}

	// Also include uncommitted changes (working directory diff)
	if response.HasUncommittedChanges {
		// Get unstaged changes
		unstagedArgs := []string{"diff", "--numstat"}
		if pathFilter != "" {
			unstagedArgs = append(unstagedArgs, "--", pathFilter)
		}
		unstagedCmd := exec.Command("git", unstagedArgs...)
		unstagedCmd.Dir = workDir
		if unstagedOut, err := unstagedCmd.Output(); err == nil && len(unstagedOut) > 0 {
			lines := strings.Split(strings.TrimSpace(string(unstagedOut)), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				parts := strings.Split(line, "\t")
				if len(parts) >= 3 {
					// Check if file already in list
					found := false
					for i := range response.Files {
						if response.Files[i].Path == parts[2] {
							found = true
							// Update with uncommitted changes
							if parts[0] != "-" {
								var add, del int
								fmt.Sscanf(parts[0], "%d", &add)
								fmt.Sscanf(parts[1], "%d", &del)
								response.Files[i].Additions += add
								response.Files[i].Deletions += del
								response.TotalAdditions += add
								response.TotalDeletions += del
							}
							break
						}
					}
					if !found {
						fileDiff := FileDiff{
							Path:   parts[2],
							Status: "modified",
						}
						if parts[0] == "-" {
							fileDiff.IsBinary = true
						} else {
							fmt.Sscanf(parts[0], "%d", &fileDiff.Additions)
							fmt.Sscanf(parts[1], "%d", &fileDiff.Deletions)
						}
						response.TotalAdditions += fileDiff.Additions
						response.TotalDeletions += fileDiff.Deletions
						response.Files = append(response.Files, fileDiff)
					}
				}
			}
		}

		// Get untracked files
		untrackedCmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
		untrackedCmd.Dir = workDir
		if untrackedOut, err := untrackedCmd.Output(); err == nil && len(untrackedOut) > 0 {
			lines := strings.Split(strings.TrimSpace(string(untrackedOut)), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				if pathFilter != "" && !strings.HasPrefix(line, pathFilter) {
					continue
				}
				// Check if already in list
				found := false
				for _, f := range response.Files {
					if f.Path == line {
						found = true
						break
					}
				}
				if !found {
					response.Files = append(response.Files, FileDiff{
						Path:   line,
						Status: "added",
					})
				}
			}
		}
	}

	// Get file statuses (added, deleted, modified, renamed)
	// Skip if on base branch (no committed changes to show)
	if !onBaseBranch {
		statusArgs := []string{"diff", "--name-status", mergeBase + "..HEAD"}
		if pathFilter != "" {
			statusArgs = append(statusArgs, "--", pathFilter)
		}
		nameStatusCmd := exec.Command("git", statusArgs...)
		nameStatusCmd.Dir = workDir
		if nameStatusOut, err := nameStatusCmd.Output(); err == nil && len(nameStatusOut) > 0 {
			lines := strings.Split(strings.TrimSpace(string(nameStatusOut)), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					status := parts[0]
					path := parts[1]
					var oldPath string
					if len(parts) >= 3 && (status[0] == 'R' || status[0] == 'C') {
						oldPath = parts[1]
						path = parts[2]
					}

					// Find and update the file in our list
					for i := range response.Files {
						if response.Files[i].Path == path {
							switch status[0] {
							case 'A':
								response.Files[i].Status = "added"
							case 'D':
								response.Files[i].Status = "deleted"
							case 'M':
								response.Files[i].Status = "modified"
							case 'R':
								response.Files[i].Status = "renamed"
								response.Files[i].OldPath = oldPath
							case 'C':
								response.Files[i].Status = "copied"
								response.Files[i].OldPath = oldPath
							}
							break
						}
					}
				}
			}
		}
	}

	// If include_content is true, get the actual diff content
	if includeContent {
		for i := range response.Files {
			if response.Files[i].IsBinary {
				continue
			}

			// Get diff for this specific file
			var diffOut []byte
			var err error

			// First try committed diff against merge-base (skip if on base branch)
			if !onBaseBranch {
				diffCmd := exec.Command("git", "diff", mergeBase+"..HEAD", "--", response.Files[i].Path)
				diffCmd.Dir = workDir
				diffOut, err = diffCmd.Output()
			}

			// If no committed diff (or on base branch), try working directory diff
			if err != nil || len(diffOut) == 0 {
				diffCmd := exec.Command("git", "diff", "--", response.Files[i].Path)
				diffCmd.Dir = workDir
				diffOut, err = diffCmd.Output()
			}

			// For untracked files, show entire file content
			if err != nil || len(diffOut) == 0 {
				if response.Files[i].Status == "added" {
					filePath := filepath.Join(workDir, response.Files[i].Path)
					if content, readErr := os.ReadFile(filePath); readErr == nil {
						// Format as a unified diff showing all lines as additions
						lines := strings.Split(string(content), "\n")
						var diffBuilder strings.Builder
						diffBuilder.WriteString(fmt.Sprintf("--- /dev/null\n+++ b/%s\n", response.Files[i].Path))
						diffBuilder.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
						for _, line := range lines {
							diffBuilder.WriteString("+" + line + "\n")
						}
						response.Files[i].Diff = diffBuilder.String()
						response.Files[i].Additions = len(lines)
					}
				}
			} else {
				response.Files[i].Diff = string(diffOut)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// findWorkspaceDir finds the git repository workspace directory
// It checks common locations where the workspace might be mounted
func findWorkspaceDir() string {
	candidates := []string{
		"/home/retro/work",         // Default Helix workspace symlink
		os.Getenv("WORKSPACE_DIR"), // Set by container executor
		"/home/retro/workspace",    // Alternative name
		"/workspace",               // Container workspace mount
	}

	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if isGitRepo(dir) {
			return dir
		}
		if found := findGitRepoInDir(dir); found != "" {
			return found
		}
	}

	return ""
}

// findGitRepoInDir looks for a git repository in immediate subdirectories
func findGitRepoInDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subdir := filepath.Join(dir, entry.Name())
		if isGitRepo(subdir) {
			return subdir
		}
	}
	return ""
}

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

// resolveBaseBranch finds the actual git ref for the base branch
// It tries the branch name directly, then origin/<branch>, then common defaults
func resolveBaseBranch(workDir, baseBranch string) string {
	candidates := []string{baseBranch, "origin/" + baseBranch}
	addCandidate := func(ref string) {
		for _, existing := range candidates {
			if existing == ref {
				return
			}
		}
		candidates = append(candidates, ref)
	}

	switch baseBranch {
	case "main":
		addCandidate("master")
		addCandidate("origin/master")
	case "master":
		addCandidate("main")
		addCandidate("origin/main")
	default:
		addCandidate("main")
		addCandidate("origin/main")
		addCandidate("master")
		addCandidate("origin/master")
	}

	for _, ref := range candidates {
		cmd := exec.Command("git", "rev-parse", "--verify", ref)
		cmd.Dir = workDir
		if err := cmd.Run(); err == nil {
			return ref
		}
	}

	return ""
}

// WorkspaceInfo represents information about a git workspace/repository
type WorkspaceInfo struct {
	// Name is the directory name (e.g., "my-repo")
	Name string `json:"name"`
	// Path is the full path to the repository
	Path string `json:"path"`
	// CurrentBranch is the currently checked out branch
	CurrentBranch string `json:"current_branch"`
	// IsPrimary indicates if this is the primary repository
	IsPrimary bool `json:"is_primary"`
	// HasHelixSpecs indicates if the repo has a helix-specs branch
	HasHelixSpecs bool `json:"has_helix_specs"`
}

// WorkspacesResponse is the response from the /workspaces endpoint
type WorkspacesResponse struct {
	// Workspaces is the list of git repositories found
	Workspaces []WorkspaceInfo `json:"workspaces"`
	// Error message if something went wrong
	Error string `json:"error,omitempty"`
}

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

// findAllWorkspaces finds all git repositories in the workspace directory
func findAllWorkspaces() []WorkspaceInfo {
	var workspaces []WorkspaceInfo

	// Get the base work directory
	workDirs := []string{
		"/home/retro/work",
		os.Getenv("WORKSPACE_DIR"),
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
		ws := getWorkspaceInfo(baseDir, filepath.Base(baseDir), primaryRepoName)
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
			ws := getWorkspaceInfo(subdir, entry.Name(), primaryRepoName)
			workspaces = append(workspaces, ws)
		}
	}

	return workspaces
}

// getWorkspaceInfo builds workspace info for a git repository
func getWorkspaceInfo(repoPath, name, primaryRepoName string) WorkspaceInfo {
	ws := WorkspaceInfo{
		Name:      name,
		Path:      repoPath,
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

// generateHelixSpecsDiff shows uncommitted changes in the helix-specs worktree
// The helix-specs branch is an orphan branch (no common history with main),
// so we show uncommitted changes within the worktree rather than comparing branches.
func (s *Server) generateHelixSpecsDiff(workDir, baseBranch string, includeContent bool, pathFilter string) *DiffResponse {
	response := &DiffResponse{
		Files:      []FileDiff{},
		BaseBranch: "", // No base branch - orphan branch
		Branch:     "helix-specs",
		WorkDir:    workDir,
	}

	// Find the helix-specs worktree location
	// The worktree is typically at ../helix-specs relative to the main repo
	specsWorktree := findHelixSpecsWorktree(workDir)

	if specsWorktree == "" {
		// No worktree found - show committed files in helix-specs branch instead
		response.Error = "helix-specs worktree not found (no uncommitted changes to show)"
		return response
	}

	response.WorkDir = specsWorktree

	// Check for uncommitted changes in the worktree (staged + unstaged + untracked)
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = specsWorktree
	statusOut, err := statusCmd.Output()
	if err != nil {
		response.Error = fmt.Sprintf("failed to get git status: %v", err)
		return response
	}

	response.HasUncommittedChanges = len(strings.TrimSpace(string(statusOut))) > 0

	// Get unstaged changes (modified files not yet staged)
	unstagedArgs := []string{"diff", "--numstat"}
	if pathFilter != "" {
		unstagedArgs = append(unstagedArgs, "--", pathFilter)
	}
	unstagedCmd := exec.Command("git", unstagedArgs...)
	unstagedCmd.Dir = specsWorktree
	if unstagedOut, err := unstagedCmd.Output(); err == nil && len(unstagedOut) > 0 {
		s.parseNumstatToFiles(unstagedOut, response, "modified")
	}

	// Get staged changes (ready to commit)
	stagedArgs := []string{"diff", "--cached", "--numstat"}
	if pathFilter != "" {
		stagedArgs = append(stagedArgs, "--", pathFilter)
	}
	stagedCmd := exec.Command("git", stagedArgs...)
	stagedCmd.Dir = specsWorktree
	if stagedOut, err := stagedCmd.Output(); err == nil && len(stagedOut) > 0 {
		s.parseNumstatToFiles(stagedOut, response, "modified")
	}

	// Get untracked files
	untrackedCmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	untrackedCmd.Dir = specsWorktree
	if untrackedOut, err := untrackedCmd.Output(); err == nil && len(untrackedOut) > 0 {
		lines := strings.Split(strings.TrimSpace(string(untrackedOut)), "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			if pathFilter != "" && !strings.HasPrefix(line, pathFilter) {
				continue
			}
			// Check if already in list
			found := false
			for _, f := range response.Files {
				if f.Path == line {
					found = true
					break
				}
			}
			if !found {
				// Count lines in the new file
				filePath := filepath.Join(specsWorktree, line)
				additions := 0
				if content, err := os.ReadFile(filePath); err == nil {
					additions = strings.Count(string(content), "\n")
					if len(content) > 0 && content[len(content)-1] != '\n' {
						additions++
					}
				}
				response.Files = append(response.Files, FileDiff{
					Path:      line,
					Status:    "added",
					Additions: additions,
				})
				response.TotalAdditions += additions
			}
		}
	}

	// Include diff content if requested
	if includeContent {
		for i := range response.Files {
			if response.Files[i].IsBinary {
				continue
			}

			var diffOut []byte

			// Try staged diff first
			diffCmd := exec.Command("git", "diff", "--cached", "--", response.Files[i].Path)
			diffCmd.Dir = specsWorktree
			diffOut, _ = diffCmd.Output()

			// If no staged diff, try unstaged diff
			if len(diffOut) == 0 {
				diffCmd = exec.Command("git", "diff", "--", response.Files[i].Path)
				diffCmd.Dir = specsWorktree
				diffOut, _ = diffCmd.Output()
			}

			// For new/untracked files, show file content as additions
			if len(diffOut) == 0 && response.Files[i].Status == "added" {
				filePath := filepath.Join(specsWorktree, response.Files[i].Path)
				if content, err := os.ReadFile(filePath); err == nil {
					lines := strings.Split(string(content), "\n")
					var diffBuilder strings.Builder
					diffBuilder.WriteString(fmt.Sprintf("--- /dev/null\n+++ b/%s\n", response.Files[i].Path))
					diffBuilder.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))
					for _, line := range lines {
						diffBuilder.WriteString("+" + line + "\n")
					}
					diffOut = []byte(diffBuilder.String())
				}
			}

			if len(diffOut) > 0 {
				response.Files[i].Diff = string(diffOut)
			}
		}
	}

	return response
}

// parseNumstatToFiles parses git numstat output and adds files to the response
func (s *Server) parseNumstatToFiles(numstatOut []byte, response *DiffResponse, defaultStatus string) {
	lines := strings.Split(strings.TrimSpace(string(numstatOut)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) >= 3 {
			// Check if already in list
			found := false
			for i := range response.Files {
				if response.Files[i].Path == parts[2] {
					found = true
					// Update with additional changes
					if parts[0] != "-" {
						var add, del int
						fmt.Sscanf(parts[0], "%d", &add)
						fmt.Sscanf(parts[1], "%d", &del)
						response.Files[i].Additions += add
						response.Files[i].Deletions += del
						response.TotalAdditions += add
						response.TotalDeletions += del
					}
					break
				}
			}
			if !found {
				fileDiff := FileDiff{
					Path:   parts[2],
					Status: defaultStatus,
				}
				if parts[0] == "-" {
					fileDiff.IsBinary = true
				} else {
					fmt.Sscanf(parts[0], "%d", &fileDiff.Additions)
					fmt.Sscanf(parts[1], "%d", &fileDiff.Deletions)
				}
				response.TotalAdditions += fileDiff.Additions
				response.TotalDeletions += fileDiff.Deletions
				response.Files = append(response.Files, fileDiff)
			}
		}
	}
}

// findHelixSpecsWorktree finds the helix-specs worktree location
// Returns empty string if no worktree is found
func findHelixSpecsWorktree(mainRepoDir string) string {
	// Check if there's a worktree for helix-specs
	// Typically it would be at a sibling directory or in .git/worktrees
	worktreeCmd := exec.Command("git", "worktree", "list", "--porcelain")
	worktreeCmd.Dir = mainRepoDir
	worktreeOut, err := worktreeCmd.Output()
	if err != nil {
		return ""
	}

	// Parse worktree list output
	// Format:
	// worktree /path/to/worktree
	// HEAD <sha>
	// branch refs/heads/branch-name
	// (blank line)
	lines := strings.Split(string(worktreeOut), "\n")
	var currentWorktree string
	for _, line := range lines {
		if strings.HasPrefix(line, "worktree ") {
			currentWorktree = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			branch := strings.TrimPrefix(line, "branch ")
			if branch == "refs/heads/helix-specs" && currentWorktree != "" {
				return currentWorktree
			}
		}
	}

	// Also check common locations for helix-specs worktree
	// It might be at ../helix-specs or a subdirectory
	parentDir := filepath.Dir(mainRepoDir)
	candidates := []string{
		filepath.Join(parentDir, "helix-specs"),
		filepath.Join(mainRepoDir, ".helix-specs"),
	}

	for _, candidate := range candidates {
		if isGitRepo(candidate) {
			// Verify it's on helix-specs branch
			branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
			branchCmd.Dir = candidate
			if branchOut, err := branchCmd.Output(); err == nil {
				if strings.TrimSpace(string(branchOut)) == "helix-specs" {
					return candidate
				}
			}
		}
	}

	return ""
}
