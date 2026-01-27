package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"
)

// GitRepo wraps a gitea repository for common operations.
// This replaces exec.Command("git", ...) calls with pure Go equivalents.
type GitRepo struct {
	repo *giteagit.Repository
	path string
	ctx  context.Context
}

// OpenGitRepo opens a git repository at the given path.
// Works with both bare and non-bare repositories.
func OpenGitRepo(repoPath string) (*GitRepo, error) {
	ctx := context.Background()
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository at %s: %w", repoPath, err)
	}
	return &GitRepo{repo: repo, path: repoPath, ctx: ctx}, nil
}

// Close releases resources held by the repository
func (g *GitRepo) Close() {
	if g.repo != nil {
		g.repo.Close()
	}
}

// GetBranchCommitHash returns the commit hash for a branch.
// Equivalent to: git rev-parse <branch>
func (g *GitRepo) GetBranchCommitHash(branchName string) (string, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return "", fmt.Errorf("branch %s not found: %w", branchName, err)
	}
	return commit.ID.String(), nil
}

// ListFilesInBranch returns all file paths in a branch.
// Equivalent to: git ls-tree --name-only -r <branch>
func (g *GitRepo) ListFilesInBranch(branchName string) ([]string, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}

	// Get the tree - commit embeds Tree
	tree := &commit.Tree

	// List all entries recursively by walking the tree
	var files []string
	err = g.walkTree(tree, "", &files)
	if err != nil {
		return nil, fmt.Errorf("failed to walk tree: %w", err)
	}

	return files, nil
}

// walkTree recursively walks a git tree and collects file paths
func (g *GitRepo) walkTree(tree *giteagit.Tree, prefix string, files *[]string) error {
	entries, err := tree.ListEntries()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		path := entry.Name()
		if prefix != "" {
			path = filepath.Join(prefix, entry.Name())
		}

		if entry.IsDir() {
			// Recurse into subdirectory
			subtree, err := tree.SubTree(entry.Name())
			if err != nil {
				continue // Skip if we can't access subtree
			}
			if err := g.walkTree(subtree, path, files); err != nil {
				continue // Skip on error
			}
		} else {
			*files = append(*files, path)
		}
	}
	return nil
}

// ReadFileFromBranch reads the content of a file from a specific branch.
// Equivalent to: git show <branch>:<filepath>
func (g *GitRepo) ReadFileFromBranch(branchName, filePath string) ([]byte, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}

	// Use GetFileContent from commit which returns string
	content, err := commit.GetFileContent(filePath, 0) // 0 = no limit
	if err != nil {
		return nil, fmt.Errorf("file %s not found in branch %s: %w", filePath, branchName, err)
	}

	return []byte(content), nil
}

// IsBranchMergedInto checks if sourceBranch is merged into targetBranch.
// Equivalent to: git branch --merged <target> --list <source>
func (g *GitRepo) IsBranchMergedInto(sourceBranch, targetBranch string) (bool, error) {
	sourceCommit, err := g.repo.GetBranchCommit(sourceBranch)
	if err != nil {
		return false, fmt.Errorf("source branch %s not found: %w", sourceBranch, err)
	}

	targetCommit, err := g.repo.GetBranchCommit(targetBranch)
	if err != nil {
		return false, fmt.Errorf("target branch %s not found: %w", targetBranch, err)
	}

	// Check if source commit is an ancestor of target commit using git merge-base
	_, _, err = gitcmd.NewCommand("merge-base", "--is-ancestor").
		AddDynamicArguments(sourceCommit.ID.String(), targetCommit.ID.String()).
		RunStdString(g.ctx, &gitcmd.RunOpts{Dir: g.path})
	if err != nil {
		// Exit code 1 means not an ancestor, exit code 0 means it is
		return false, nil // Not an ancestor
	}
	return true, nil // Is an ancestor
}

// GetChangedFilesInCommit returns files changed in a specific commit.
// Uses gitea's high-level GetCommitFileStatus API.
func (g *GitRepo) GetChangedFilesInCommit(commitHash string) ([]string, error) {
	// Use gitea's high-level API to get file status for the commit
	status, err := giteagit.GetCommitFileStatus(g.ctx, g.path, commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	// Combine all changed files (added, modified, removed)
	var files []string
	files = append(files, status.Added...)
	files = append(files, status.Modified...)
	files = append(files, status.Removed...)
	return files, nil
}

// GetChangedFilesInBranch returns files changed in the latest commit of a branch.
// Equivalent to: git diff-tree -m --no-commit-id --name-only -r <branch>
func (g *GitRepo) GetChangedFilesInBranch(branchName string) ([]string, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}
	return g.GetChangedFilesInCommit(commit.ID.String())
}

// ListBranches returns all branch names in the repository.
func (g *GitRepo) ListBranches() ([]string, error) {
	branches, _, err := g.repo.GetBranchNames(0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	return branches, nil
}

// FindTaskDirInBranch finds the directory for a specific task in the helix-specs branch.
// Searches for either DesignDocPath or task ID in the directory structure.
func (g *GitRepo) FindTaskDirInBranch(branchName, designDocPath, taskID string) (string, error) {
	files, err := g.ListFilesInBranch(branchName)
	if err != nil {
		return "", err
	}

	// First try DesignDocPath (new human-readable format)
	if designDocPath != "" {
		for _, file := range files {
			if strings.Contains(file, designDocPath) {
				parts := strings.Split(file, "/")
				if len(parts) >= 2 {
					return strings.Join(parts[:len(parts)-1], "/"), nil
				}
			}
		}
	}

	// Fall back to taskID for backwards compatibility
	for _, file := range files {
		if strings.Contains(file, taskID) {
			parts := strings.Split(file, "/")
			if len(parts) >= 2 {
				return strings.Join(parts[:len(parts)-1], "/"), nil
			}
		}
	}

	return "", fmt.Errorf("task directory not found for %s", taskID)
}

// ReadDesignDocs reads the standard design documents from a task directory.
// Returns a map of filename -> content for requirements.md, design.md, tasks.md
func (g *GitRepo) ReadDesignDocs(branchName, taskDir string) (map[string]string, error) {
	docs := make(map[string]string)
	docFilenames := []string{"requirements.md", "design.md", "tasks.md"}

	for _, filename := range docFilenames {
		filePath := taskDir + "/" + filename
		content, err := g.ReadFileFromBranch(branchName, filePath)
		if err != nil {
			log.Debug().
				Err(err).
				Str("filename", filename).
				Str("path", filePath).
				Msg("Design doc file not found (may not exist yet)")
			continue
		}
		docs[filename] = string(content)
	}

	return docs, nil
}

// ParseDesignDocTaskIDs extracts task IDs from design doc file paths.
// Supports both old format (task ID in directory name) and new format (task number).
// Returns taskIDs found directly and dirNames that need DB lookup.
func ParseDesignDocTaskIDs(files []string) (taskIDs []string, dirNamesNeedingLookup []string) {
	taskIDSet := make(map[string]bool)
	dirNameSet := make(map[string]bool)

	for _, file := range files {
		if !strings.Contains(file, "design/tasks/") && !strings.Contains(file, "tasks/") {
			continue
		}

		parts := strings.Split(file, "/")
		if len(parts) < 3 {
			continue
		}

		// The directory name is the second-to-last part (before the filename)
		dirName := parts[len(parts)-2]

		// Task ID is after the last underscore
		lastUnderscore := strings.LastIndex(dirName, "_")
		if lastUnderscore == -1 {
			continue
		}

		lastPart := dirName[lastUnderscore+1:]

		// Check for UUID format (old format)
		isValidUUID := len(lastPart) == 36 && strings.Count(lastPart, "-") == 4

		taskID := lastPart
		foundOldFormat := false

		// For spt_ prefixed IDs
		if strings.Contains(dirName, "_spt_") {
			sptIdx := strings.LastIndex(dirName, "_spt_")
			if sptIdx != -1 {
				taskID = dirName[sptIdx+1:]
				foundOldFormat = true
			}
		}

		// For legacy task_ prefix format
		if !foundOldFormat && strings.Contains(dirName, "task_") {
			taskPrefixIdx := strings.LastIndex(dirName, "task_")
			if taskPrefixIdx != -1 {
				taskID = dirName[taskPrefixIdx:]
				foundOldFormat = true
			}
		}

		if foundOldFormat || isValidUUID {
			taskIDSet[taskID] = true
		} else {
			// New format: needs DB lookup
			dirNameSet[dirName] = true
		}
	}

	for id := range taskIDSet {
		taskIDs = append(taskIDs, id)
	}
	for name := range dirNameSet {
		dirNamesNeedingLookup = append(dirNamesNeedingLookup, name)
	}

	return taskIDs, dirNamesNeedingLookup
}
