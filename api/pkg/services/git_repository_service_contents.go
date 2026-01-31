package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// WorkingCopy represents a temporary non-bare clone of a bare repository
// that can be used for worktree operations (add, commit, checkout, etc.)
type WorkingCopy struct {
	TempDir string // Temporary directory containing the working copy
}

// Cleanup removes the temporary working copy directory
func (wc *WorkingCopy) Cleanup() {
	if wc.TempDir != "" {
		_ = os.RemoveAll(wc.TempDir)
	}
}

// PushToBare pushes changes from the working copy back to the bare repository (origin)
func (wc *WorkingCopy) PushToBare(ctx context.Context, branch string) error {
	refSpec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)
	err := giteagit.Push(ctx, wc.TempDir, giteagit.PushOptions{
		Remote: "origin",
		Branch: refSpec,
	})
	if err != nil {
		// Ignore "already up to date" - this is not an error
		if giteagit.IsErrPushOutOfDate(err) || giteagit.IsErrPushRejected(err) {
			return fmt.Errorf("failed to push to bare repo: %w", err)
		}
		// Check if error message indicates already up to date
		if strings.Contains(err.Error(), "already up-to-date") || strings.Contains(err.Error(), "Everything up-to-date") {
			return nil
		}
		return fmt.Errorf("failed to push to bare repo: %w", err)
	}
	return nil
}

// PushToHelixBare pushes changes to the helix-bare remote (used for external repos)
func (wc *WorkingCopy) PushToHelixBare(ctx context.Context, branch string) error {
	refSpec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)
	err := giteagit.Push(ctx, wc.TempDir, giteagit.PushOptions{
		Remote: "helix-bare",
		Branch: refSpec,
	})
	if err != nil {
		return fmt.Errorf("failed to push to helix-bare repo: %w", err)
	}
	return nil
}

// PushToOrigin pushes changes to origin (external remote) with optional force
// authURL should be the authenticated URL with credentials embedded
func (wc *WorkingCopy) PushToOrigin(ctx context.Context, branch string, force bool, authURL string) error {
	refSpec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)
	if force {
		refSpec = "+" + refSpec
	}

	// If authURL is provided, temporarily update origin URL
	if authURL != "" {
		if err := SetRemoteURL(ctx, wc.TempDir, "origin", authURL); err != nil {
			return fmt.Errorf("failed to set origin URL: %w", err)
		}
	}

	err := giteagit.Push(ctx, wc.TempDir, giteagit.PushOptions{
		Remote: "origin",
		Branch: refSpec,
		Force:  force,
	})
	if err != nil {
		return fmt.Errorf("failed to push to origin: %w", err)
	}
	return nil
}

// Pull fetches and merges changes from origin
// authURL should be the authenticated URL with credentials embedded
func (wc *WorkingCopy) Pull(ctx context.Context, authURL string) error {
	// If authURL is provided, temporarily update origin URL
	if authURL != "" {
		if err := SetRemoteURL(ctx, wc.TempDir, "origin", authURL); err != nil {
			return fmt.Errorf("failed to set origin URL: %w", err)
		}
	}

	// Use native git fetch + merge via Fetch helper
	// Note: git pull = git fetch + git merge
	err := Fetch(ctx, wc.TempDir, FetchOptions{
		Remote:  "origin",
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("failed to fetch from origin: %w", err)
	}

	// For simplicity, we just fetch. If merge is needed, it can be done separately.
	// In practice, this is used for syncing which typically just needs fetch.
	return nil
}

// getWorkingCopy creates a temporary non-bare clone of a bare repository
// This is needed because bare repositories don't have a worktree, so operations
// like Add, Commit, Checkout require a non-bare clone.
// The caller MUST call Cleanup() when done to remove the temporary directory.
func (s *GitRepositoryService) getWorkingCopy(ctx context.Context, bareRepoPath string, branch string) (*WorkingCopy, error) {
	tempDir, err := os.MkdirTemp("", "helix-git-workdir-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Clone from bare repo using gitea's wrapper
	err = giteagit.Clone(ctx, bareRepoPath, tempDir, giteagit.CloneRepoOptions{
		Branch: branch,
	})
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to clone bare repo: %w", err)
	}

	// Checkout branch if specified
	if branch != "" {
		// Try to checkout existing branch first
		err = GitCheckout(ctx, tempDir, branch, false)
		if err != nil {
			// Branch doesn't exist, create it
			err = GitCheckout(ctx, tempDir, branch, true)
			if err != nil {
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to checkout branch %s: %w", branch, err)
			}
		}
	}

	return &WorkingCopy{
		TempDir: tempDir,
	}, nil
}

// getExternalWorkingCopy creates a temporary non-bare clone from an external repository
// This is used for syncing with external repositories (GitHub, ADO, etc.)
// The working copy has the external repo as "origin" and can push/pull from it.
// The caller MUST call Cleanup() when done to remove the temporary directory.
func (s *GitRepositoryService) getExternalWorkingCopy(
	ctx context.Context,
	externalURL string,
	bareRepoPath string,
	branch string,
) (*WorkingCopy, error) {
	tempDir, err := os.MkdirTemp("", "helix-git-external-workdir-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Clone from external URL using gitea's wrapper
	err = giteagit.Clone(ctx, externalURL, tempDir, giteagit.CloneRepoOptions{
		Branch: branch,
	})
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to clone external repo: %w", err)
	}

	// Checkout branch if specified
	if branch != "" {
		err = GitCheckout(ctx, tempDir, branch, false)
		if err != nil {
			err = GitCheckout(ctx, tempDir, branch, true)
			if err != nil {
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to checkout branch %s: %w", branch, err)
			}
		}
	}

	// Add helix-bare remote if provided
	if bareRepoPath != "" {
		if err := AddRemote(ctx, tempDir, "helix-bare", bareRepoPath); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to add helix-bare remote: %w", err)
		}
	}

	return &WorkingCopy{
		TempDir: tempDir,
	}, nil
}

// BrowseTree lists files and directories at a given path in a specific branch
// Uses native git via gitea's wrappers for reliable operations.
func (s *GitRepositoryService) BrowseTree(ctx context.Context, repoID string, path string, branch string) ([]types.TreeEntry, error) {
	if branch == "" {
		return nil, fmt.Errorf("branch is required")
	}

	// Get repository to find local path
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return nil, fmt.Errorf("repository %s has no local path", repoID)
	}

	// Open the repository using gitea's wrapper
	gitRepo, err := giteagit.OpenRepository(ctx, repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}
	defer gitRepo.Close()

	// Get commit for the branch
	commit, err := gitRepo.GetBranchCommit(branch)
	if err != nil {
		return nil, fmt.Errorf("failed to find branch %s: %w", branch, err)
	}

	// Get the tree - commit embeds Tree
	tree := &commit.Tree

	// Navigate to the requested path if specified
	if path != "." && path != "" {
		tree, err = tree.SubTree(path)
		if err != nil {
			return nil, fmt.Errorf("path not found in repository: %w", err)
		}
	}

	// List entries
	entries, err := tree.ListEntries()
	if err != nil {
		return nil, fmt.Errorf("failed to list tree entries: %w", err)
	}

	// Build tree entries
	result := make([]types.TreeEntry, 0, len(entries))
	for _, entry := range entries {
		entryPath := path
		if entryPath == "." || entryPath == "" {
			entryPath = entry.Name()
		} else {
			entryPath = filepath.Join(path, entry.Name())
		}

		// Determine if entry is a directory
		isDir := entry.IsDir()

		// Get size (only available for files/blobs)
		var size int64
		if !isDir {
			size = entry.Size()
		}

		result = append(result, types.TreeEntry{
			Name:  entry.Name(),
			Path:  entryPath,
			IsDir: isDir,
			Size:  size,
		})
	}

	return result, nil
}

// CreateOrUpdateFileContents creates or updates a file in a repository and commits it.
// For bare repositories (which Helix uses to accept remote pushes), this creates a
// temporary working copy, makes the changes, commits, and pushes back to the bare repo.
// Returns the commit hash.
//
// Uses native git via gitea's wrappers for reliable operations.
func (s *GitRepositoryService) CreateOrUpdateFileContents(
	ctx context.Context,
	repoID string,
	path string,
	branch string,
	content []byte,
	commitMessage string,
	authorName string,
	authorEmail string,
) (string, error) {
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %w", err)
	}

	log.Info().
		Str("repo_id", repoID).
		Str("path", path).
		Str("branch", branch).
		Str("author_name", authorName).
		Msg("Creating or updating file contents")

	if repo.LocalPath == "" {
		return "", fmt.Errorf("repository has no local path")
	}

	if branch == "" {
		return "", fmt.Errorf("branch is required")
	}

	if commitMessage == "" {
		commitMessage = fmt.Sprintf("Update %s", path)
	}

	// External repos MUST have author credentials to avoid commits with non-corporate emails
	// Enterprise ADO deployments reject pushes containing commits from non-corporate email addresses
	if repo.IsExternal {
		if authorName == "" || authorEmail == "" {
			return "", fmt.Errorf("author name and email are required for external repositories")
		}
	} else {
		// Only allow fallback for internal repos
		if authorName == "" {
			authorName = s.gitUserName
		}
		if authorEmail == "" {
			authorEmail = s.gitUserEmail
		}
	}

	wc, err := s.getWorkingCopy(ctx, repo.LocalPath, branch)
	if err != nil {
		return "", fmt.Errorf("failed to create working copy: %w", err)
	}
	defer wc.Cleanup()

	filename := filepath.Join(wc.TempDir, path)

	fileDir := filepath.Dir(filename)
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	err = os.WriteFile(filename, content, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Add file using gitea's wrapper
	if err := giteagit.AddChanges(ctx, wc.TempDir, false, path); err != nil {
		return "", fmt.Errorf("failed to add file to staging: %w", err)
	}

	// Commit using gitea's wrapper
	if err := giteagit.CommitChanges(ctx, wc.TempDir, giteagit.CommitChangesOptions{
		Committer: &giteagit.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
		Author: &giteagit.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
		Message: commitMessage,
	}); err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}

	// Push to bare repo
	if err := wc.PushToBare(ctx, branch); err != nil {
		return "", fmt.Errorf("failed to push to bare repository: %w", err)
	}

	// Get commit hash from the working copy
	commitHash, err := GetBranchCommitID(ctx, wc.TempDir, branch)
	if err != nil {
		return "", fmt.Errorf("failed to get commit hash: %w", err)
	}

	log.Info().
		Str("repo_id", repoID).
		Str("path", path).
		Str("branch", branch).
		Str("commit_hash", commitHash).
		Msg("Successfully created/updated file in repository")

	return commitHash, nil
}

// GetFileContents reads the contents of a file from a specific branch
// Uses native git via gitea's wrappers for reliable operations.
func (s *GitRepositoryService) GetFileContents(ctx context.Context, repoID string, path string, branch string) (string, error) {
	// Get repository to find local path
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return "", fmt.Errorf("repository has no local path")
	}

	// Open the repository using gitea's wrapper
	gitRepo, err := giteagit.OpenRepository(ctx, repo.LocalPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}
	defer gitRepo.Close()

	// Get branch reference
	var commit *giteagit.Commit
	if branch == "" {
		// Use HEAD
		headBranch, err := GetHEADBranch(ctx, repo.LocalPath)
		if err != nil {
			return "", fmt.Errorf("failed to get HEAD: %w", err)
		}
		commit, err = gitRepo.GetBranchCommit(headBranch)
		if err != nil {
			return "", fmt.Errorf("failed to get HEAD commit: %w", err)
		}
	} else {
		commit, err = gitRepo.GetBranchCommit(branch)
		if err != nil {
			return "", fmt.Errorf("failed to get branch commit: %w", err)
		}
	}

	// Get file contents directly from commit
	content, err := commit.GetFileContent(path, 0) // 0 = no limit
	if err != nil {
		return "", fmt.Errorf("file not found in repository: %w", err)
	}

	return content, nil
}
