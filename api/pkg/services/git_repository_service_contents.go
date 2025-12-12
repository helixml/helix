package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// WorkingCopy represents a temporary non-bare clone of a bare repository
// that can be used for worktree operations (add, commit, checkout, etc.)
type WorkingCopy struct {
	TempDir  string          // Temporary directory containing the working copy
	Repo     *git.Repository // The cloned repository
	Worktree *git.Worktree   // The worktree for making changes
}

// Cleanup removes the temporary working copy directory
func (wc *WorkingCopy) Cleanup() {
	if wc.TempDir != "" {
		_ = os.RemoveAll(wc.TempDir)
	}
}

// PushToBare pushes changes from the working copy back to the bare repository (origin)
func (wc *WorkingCopy) PushToBare(branch string) error {
	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
	err := wc.Repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push to bare repo: %w", err)
	}
	return nil
}

// PushToHelixBare pushes changes to the helix-bare remote (used for external repos)
func (wc *WorkingCopy) PushToHelixBare(branch string) error {
	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
	err := wc.Repo.Push(&git.PushOptions{
		RemoteName: "helix-bare",
		RefSpecs:   []config.RefSpec{refSpec},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push to helix-bare repo: %w", err)
	}
	return nil
}

// PushToOrigin pushes changes to origin (external remote) with optional force and auth
func (wc *WorkingCopy) PushToOrigin(branch string, force bool, auth transport.AuthMethod) error {
	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
	if force {
		refSpec = config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch))
	}
	opts := &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
	}
	if auth != nil {
		opts.Auth = auth
	}
	err := wc.Repo.Push(opts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push to origin: %w", err)
	}
	return nil
}

// Pull fetches and merges changes from origin
func (wc *WorkingCopy) Pull(auth transport.AuthMethod) error {
	opts := &git.PullOptions{
		RemoteName: "origin",
	}
	if auth != nil {
		opts.Auth = auth
	}
	err := wc.Worktree.Pull(opts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to pull from origin: %w", err)
	}
	return nil
}

// getWorkingCopy creates a temporary non-bare clone of a bare repository
// This is needed because bare repositories don't have a worktree, so operations
// like Add, Commit, Checkout require a non-bare clone.
// The caller MUST call Cleanup() when done to remove the temporary directory.
func (s *GitRepositoryService) getWorkingCopy(bareRepoPath string, branch string, auth transport.AuthMethod) (*WorkingCopy, error) {
	tempDir, err := os.MkdirTemp("", "helix-git-workdir-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cloneOpts := &git.CloneOptions{
		URL: bareRepoPath,
	}
	if auth != nil {
		cloneOpts.Auth = auth
	}

	repo, err := git.PlainClone(tempDir, cloneOpts)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to clone bare repo: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	if branch != "" {
		branchRef := plumbing.NewBranchReferenceName(branch)
		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
		})
		if err != nil {
			err = worktree.Checkout(&git.CheckoutOptions{
				Branch: branchRef,
				Create: true,
			})
			if err != nil {
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to checkout branch %s: %w", branch, err)
			}
		}
	}

	return &WorkingCopy{
		TempDir:  tempDir,
		Repo:     repo,
		Worktree: worktree,
	}, nil
}

// getExternalWorkingCopy creates a temporary non-bare clone from an external repository
// This is used for syncing with external repositories (GitHub, ADO, etc.)
// The working copy has the external repo as "origin" and can push/pull from it.
// The caller MUST call Cleanup() when done to remove the temporary directory.
func (s *GitRepositoryService) getExternalWorkingCopy(
	externalURL string,
	bareRepoPath string,
	branch string,
	auth transport.AuthMethod,
) (*WorkingCopy, error) {
	tempDir, err := os.MkdirTemp("", "helix-git-external-workdir-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cloneOpts := &git.CloneOptions{
		URL: externalURL,
	}
	if auth != nil {
		cloneOpts.Auth = auth
	}

	repo, err := git.PlainClone(tempDir, cloneOpts)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to clone external repo: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	if branch != "" {
		branchRef := plumbing.NewBranchReferenceName(branch)
		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
		})
		if err != nil {
			err = worktree.Checkout(&git.CheckoutOptions{
				Branch: branchRef,
				Create: true,
			})
			if err != nil {
				os.RemoveAll(tempDir)
				return nil, fmt.Errorf("failed to checkout branch %s: %w", branch, err)
			}
		}
	}

	if bareRepoPath != "" {
		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: "helix-bare",
			URLs: []string{bareRepoPath},
		})
		if err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to add helix-bare remote: %w", err)
		}
	}

	return &WorkingCopy{
		TempDir:  tempDir,
		Repo:     repo,
		Worktree: worktree,
	}, nil
}

// BrowseTree lists files and directories at a given path in a specific branch
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

	// Open the bare repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get reference for specified branch, default to HEAD
	var ref *plumbing.Reference

	// Try to resolve the branch
	branchRef := plumbing.NewBranchReferenceName(branch)

	ref, err = gitRepo.Reference(branchRef, true)
	if err != nil {
		return nil, fmt.Errorf("failed to find branch %s: %w", branch, err)
	}

	// Get the commit
	commit, err := gitRepo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the tree
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree: %w", err)
	}

	// Navigate to the requested path
	if path != "." && path != "" {
		tree, err = tree.Tree(path)
		if err != nil {
			return nil, fmt.Errorf("path not found in repository: %w", err)
		}
	}

	// Build tree entries
	result := make([]types.TreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		entryPath := path
		if entryPath == "." || entryPath == "" {
			entryPath = entry.Name
		} else {
			entryPath = filepath.Join(path, entry.Name)
		}

		// Determine if entry is a directory
		isDir := entry.Mode == filemode.Dir

		// Get size (only available for files/blobs)
		var size int64
		if !isDir {
			// Get blob to read size
			blob, err := gitRepo.BlobObject(entry.Hash)
			if err == nil {
				size = blob.Size
			}
		}

		result = append(result, types.TreeEntry{
			Name:  entry.Name,
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

	if authorName == "" {
		authorName = s.gitUserName
	}
	if authorEmail == "" {
		authorEmail = s.gitUserEmail
	}

	wc, err := s.getWorkingCopy(repo.LocalPath, branch, nil)
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

	_, err = wc.Worktree.Add(path)
	if err != nil {
		return "", fmt.Errorf("failed to add file to staging: %w", err)
	}

	status, err := wc.Worktree.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree status: %w", err)
	}

	log.Debug().
		Str("repo_id", repoID).
		Str("path", path).
		Str("status", status.String()).
		Msg("Worktree status after adding file")

	commitHash, err := wc.Worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}

	err = wc.PushToBare(branch)
	if err != nil {
		return "", fmt.Errorf("failed to push to bare repository: %w", err)
	}

	commitObj, err := wc.Repo.CommitObject(commitHash)
	if err != nil {
		return "", fmt.Errorf("failed to get commit object: %w", err)
	}

	log.Info().
		Str("repo_id", repoID).
		Str("path", path).
		Str("branch", branch).
		Str("commit_hash", commitHash.String()).
		Str("commit_author", commitObj.Author.String()).
		Msg("Successfully created/updated file in repository")

	return commitHash.String(), nil
}

// GetFileContents reads the contents of a file from a specific branch
func (s *GitRepositoryService) GetFileContents(ctx context.Context, repoID string, path string, branch string) (string, error) {
	// Get repository to find local path
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return "", fmt.Errorf("repository has no local path")
	}

	// Open the bare repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get branch reference - use HEAD if no branch specified
	var ref *plumbing.Reference
	if branch == "" {
		ref, err = gitRepo.Head()
		if err != nil {
			return "", fmt.Errorf("failed to get HEAD: %w", err)
		}
	} else {
		ref, err = gitRepo.Reference(plumbing.NewBranchReferenceName(branch), true)
		if err != nil {
			return "", fmt.Errorf("failed to get branch reference: %w", err)
		}
	}

	// Get the commit
	commit, err := gitRepo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the tree
	tree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("failed to get tree: %w", err)
	}

	// Get the file
	file, err := tree.File(path)
	if err != nil {
		return "", fmt.Errorf("file not found in repository: %w", err)
	}

	// Read file contents
	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("failed to read file contents: %w", err)
	}

	return content, nil
}
