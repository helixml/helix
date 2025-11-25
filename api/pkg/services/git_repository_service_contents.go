package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

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

	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	w, err := gitRepo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	head, err := gitRepo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	if head.Name() != branchRef {
		err = w.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
		})
		if err != nil {
			err = w.Checkout(&git.CheckoutOptions{
				Branch: branchRef,
				Create: true,
			})
			if err != nil {
				return "", fmt.Errorf("failed to checkout or create branch %s: %w", branch, err)
			}
			log.Info().
				Str("repo_id", repoID).
				Str("branch", branch).
				Msg("Created and checked out new branch")
		} else {
			log.Info().
				Str("repo_id", repoID).
				Str("branch", branch).
				Msg("Checked out existing branch")
		}
	}

	filename := filepath.Join(repo.LocalPath, path)

	fileDir := filepath.Dir(filename)
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	err = os.WriteFile(filename, content, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	_, err = w.Add(path)
	if err != nil {
		return "", fmt.Errorf("failed to add file to staging: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree status: %w", err)
	}

	log.Debug().
		Str("repo_id", repoID).
		Str("path", path).
		Str("status", status.String()).
		Msg("Worktree status after adding file")

	commitHash, err := w.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}

	commitObj, err := gitRepo.CommitObject(commitHash)
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
