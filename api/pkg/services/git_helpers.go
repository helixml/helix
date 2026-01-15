package services

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/rs/zerolog/log"
)

// GitRepo wraps a go-git repository for common operations.
// This replaces exec.Command("git", ...) calls with pure Go equivalents.
type GitRepo struct {
	repo *git.Repository
	path string
}

// OpenGitRepo opens a git repository at the given path.
// Works with both bare and non-bare repositories.
func OpenGitRepo(repoPath string) (*GitRepo, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository at %s: %w", repoPath, err)
	}
	return &GitRepo{repo: repo, path: repoPath}, nil
}

// GetBranchCommitHash returns the commit hash for a branch.
// Equivalent to: git rev-parse <branch>
func (g *GitRepo) GetBranchCommitHash(branchName string) (string, error) {
	// Try refs/heads/<branch> first (local branch)
	ref, err := g.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		// Try as a direct reference name (e.g., for helix-specs)
		ref, err = g.repo.Reference(plumbing.ReferenceName("refs/heads/"+branchName), true)
		if err != nil {
			return "", fmt.Errorf("branch %s not found: %w", branchName, err)
		}
	}
	return ref.Hash().String(), nil
}

// ListFilesInBranch returns all file paths in a branch.
// Equivalent to: git ls-tree --name-only -r <branch>
func (g *GitRepo) ListFilesInBranch(branchName string) ([]string, error) {
	ref, err := g.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}

	commit, err := g.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit for branch %s: %w", branchName, err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree for branch %s: %w", branchName, err)
	}

	var files []string
	err = tree.Files().ForEach(func(f *object.File) error {
		files = append(files, f.Name)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate files: %w", err)
	}

	return files, nil
}

// ReadFileFromBranch reads the content of a file from a specific branch.
// Equivalent to: git show <branch>:<filepath>
func (g *GitRepo) ReadFileFromBranch(branchName, filePath string) ([]byte, error) {
	ref, err := g.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}

	commit, err := g.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit for branch %s: %w", branchName, err)
	}

	file, err := commit.File(filePath)
	if err != nil {
		return nil, fmt.Errorf("file %s not found in branch %s: %w", filePath, branchName, err)
	}

	reader, err := file.Reader()
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	return content, nil
}

// IsBranchMergedInto checks if sourceBranch is merged into targetBranch.
// Equivalent to: git branch --merged <target> --list <source>
func (g *GitRepo) IsBranchMergedInto(sourceBranch, targetBranch string) (bool, error) {
	sourceRef, err := g.repo.Reference(plumbing.NewBranchReferenceName(sourceBranch), true)
	if err != nil {
		return false, fmt.Errorf("source branch %s not found: %w", sourceBranch, err)
	}

	targetRef, err := g.repo.Reference(plumbing.NewBranchReferenceName(targetBranch), true)
	if err != nil {
		return false, fmt.Errorf("target branch %s not found: %w", targetBranch, err)
	}

	sourceCommit, err := g.repo.CommitObject(sourceRef.Hash())
	if err != nil {
		return false, fmt.Errorf("failed to get source commit: %w", err)
	}

	targetCommit, err := g.repo.CommitObject(targetRef.Hash())
	if err != nil {
		return false, fmt.Errorf("failed to get target commit: %w", err)
	}

	// Check if source commit is an ancestor of target commit
	isAncestor, err := sourceCommit.IsAncestor(targetCommit)
	if err != nil {
		return false, fmt.Errorf("failed to check ancestry: %w", err)
	}

	return isAncestor, nil
}

// GetChangedFilesInCommit returns files changed in a specific commit.
// Equivalent to: git diff-tree --no-commit-id --name-only -r <commit>
func (g *GitRepo) GetChangedFilesInCommit(commitHash string) ([]string, error) {
	hash := plumbing.NewHash(commitHash)
	commit, err := g.repo.CommitObject(hash)
	if err != nil {
		return nil, fmt.Errorf("commit %s not found: %w", commitHash, err)
	}

	// Get the parent commit (if exists)
	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent commit: %w", err)
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("failed to get parent tree: %w", err)
		}
	}

	currentTree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get current tree: %w", err)
	}

	var files []string

	if parentTree == nil {
		// Initial commit - all files are "changed"
		err = currentTree.Files().ForEach(func(f *object.File) error {
			files = append(files, f.Name)
			return nil
		})
	} else {
		// Diff between parent and current
		changes, err := parentTree.Diff(currentTree)
		if err != nil {
			return nil, fmt.Errorf("failed to diff trees: %w", err)
		}
		for _, change := range changes {
			// Get the file path (could be From or To depending on operation)
			if change.To.Name != "" {
				files = append(files, change.To.Name)
			} else if change.From.Name != "" {
				files = append(files, change.From.Name)
			}
		}
	}

	return files, err
}

// GetChangedFilesInBranch returns files changed in the latest commit of a branch.
// Equivalent to: git diff-tree -m --no-commit-id --name-only -r <branch>
func (g *GitRepo) GetChangedFilesInBranch(branchName string) ([]string, error) {
	ref, err := g.repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}
	return g.GetChangedFilesInCommit(ref.Hash().String())
}

// ListBranches returns all branch names in the repository.
func (g *GitRepo) ListBranches() ([]string, error) {
	refs, err := g.repo.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	var branches []string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			branches = append(branches, ref.Name().Short())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate references: %w", err)
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
