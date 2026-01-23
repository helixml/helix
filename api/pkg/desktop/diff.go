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

	// Find the workspace directory
	workDir := findWorkspaceDir()
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
	)

	response := DiffResponse{
		Files:      []FileDiff{},
		BaseBranch: baseBranch,
		WorkDir:    workDir,
	}

	// Get current branch name
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = workDir
	if branchOut, err := branchCmd.Output(); err == nil {
		response.Branch = strings.TrimSpace(string(branchOut))
	}

	// Check for uncommitted changes (staged + unstaged + untracked)
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = workDir
	if statusOut, err := statusCmd.Output(); err == nil {
		response.HasUncommittedChanges = len(strings.TrimSpace(string(statusOut))) > 0
	}

	// Get diff against base branch (committed changes)
	// Use git diff with --numstat for file-level stats
	diffArgs := []string{"diff", "--numstat", baseBranch + "...HEAD"}
	if pathFilter != "" {
		diffArgs = append(diffArgs, "--", pathFilter)
	}

	numstatCmd := exec.Command("git", diffArgs...)
	numstatCmd.Dir = workDir
	numstatOut, numstatErr := numstatCmd.Output()

	if numstatErr != nil {
		// Base branch might not exist, try without the ...HEAD
		diffArgs = []string{"diff", "--numstat", baseBranch}
		if pathFilter != "" {
			diffArgs = append(diffArgs, "--", pathFilter)
		}
		numstatCmd = exec.Command("git", diffArgs...)
		numstatCmd.Dir = workDir
		numstatOut, numstatErr = numstatCmd.Output()
	}

	// Parse numstat output: "additions\tdeletions\tfilename"
	if numstatErr == nil && len(numstatOut) > 0 {
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
	statusArgs := []string{"diff", "--name-status", baseBranch + "...HEAD"}
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

	// If include_content is true, get the actual diff content
	if includeContent {
		for i := range response.Files {
			if response.Files[i].IsBinary {
				continue
			}

			// Get diff for this specific file
			var diffOut []byte
			var err error

			// First try committed diff against base
			diffCmd := exec.Command("git", "diff", baseBranch+"...HEAD", "--", response.Files[i].Path)
			diffCmd.Dir = workDir
			diffOut, err = diffCmd.Output()

			// If no committed diff, try working directory diff
			if err != nil || len(diffOut) == 0 {
				diffCmd = exec.Command("git", "diff", "--", response.Files[i].Path)
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
