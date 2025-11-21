package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// BrowseTree lists files and directories at a given path in a specific branch
func (s *GitRepositoryService) BrowseTree(ctx context.Context, repoID string, path string, branch string) ([]types.TreeEntry, error) {
	// Get repository to find local path
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return nil, fmt.Errorf("repository has no local path")
	}

	// Open the bare repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get reference for specified branch, default to HEAD
	var ref *plumbing.Reference
	if branch != "" {
		// Try to resolve the branch
		branchRef := plumbing.NewBranchReferenceName(branch)
		ref, err = gitRepo.Reference(branchRef, true)
		if err != nil {
			return nil, fmt.Errorf("failed to find branch %s: %w", branch, err)
		}
	} else {
		// Default to HEAD
		ref, err = gitRepo.Head()
		if err != nil {
			return nil, fmt.Errorf("failed to get HEAD: %w", err)
		}
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

// CreateOrUpdateFileContents creates or updates a file in a repository and commits it
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

	if repo.LocalPath == "" {
		return "", fmt.Errorf("repository has no local path")
	}

	if branch == "" {
		branch = repo.DefaultBranch
		if branch == "" {
			branch = "main"
		}
	}

	if commitMessage == "" {
		commitMessage = fmt.Sprintf("Update %s", path)
	}

	tempClone, err := os.MkdirTemp("", "helix-git-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone)

	var gitRepo *git.Repository
	if repo.IsExternal && repo.ExternalURL != "" {
		cloneOptions := &git.CloneOptions{
			URL: repo.ExternalURL,
		}
		if repo.Password != "" {
			username := repo.Username
			if username == "" {
				username = "PAT"
			}
			cloneOptions.Auth = &http.BasicAuth{
				Username: username,
				Password: repo.Password,
			}
		}
		gitRepo, err = git.PlainClone(tempClone, cloneOptions)
		if err != nil {
			return "", fmt.Errorf("failed to clone external repository: %w", err)
		}
	} else {
		cloneURL := repo.LocalPath
		if !strings.HasPrefix(cloneURL, "file://") && !strings.HasPrefix(cloneURL, "http://") && !strings.HasPrefix(cloneURL, "https://") {
			cloneURL = "file://" + cloneURL
		}
		gitRepo, err = git.PlainClone(tempClone, &git.CloneOptions{
			URL: cloneURL,
		})
		if err != nil {
			return "", fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Try to find and checkout the branch
	branchRef := plumbing.NewBranchReferenceName(branch)
	_, err = gitRepo.Reference(branchRef, true)
	if err != nil {
		// Branch does not exist locally. Try to find it on remote.
		var remoteRef *plumbing.Reference
		remoteBranchName := plumbing.NewRemoteReferenceName("origin", branch)

		// Check if we already have it in remotes (from clone)
		remoteRef, err = gitRepo.Reference(remoteBranchName, true)

		// If not found, try to fetch it explicitly
		if err != nil {
			remote, remoteErr := gitRepo.Remote("origin")
			if remoteErr == nil {
				fetchErr := remote.Fetch(&git.FetchOptions{
					RefSpecs: []config.RefSpec{
						config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/remotes/origin/%s", branch, branch)),
					},
				})
				// Accept success or AlreadyUpToDate
				if fetchErr == nil || fetchErr == git.NoErrAlreadyUpToDate {
					remoteRef, _ = gitRepo.Reference(remoteBranchName, true)
				}
			}
		}

		if remoteRef != nil {
			// Found on remote, create local tracking branch
			err = gitRepo.Storer.SetReference(plumbing.NewHashReference(branchRef, remoteRef.Hash()))
			if err != nil {
				return "", fmt.Errorf("failed to create local branch ref from remote: %w", err)
			}
		} else {
			// Not found on remote either, create new branch from HEAD
			headRef, headErr := gitRepo.Head()
			if headErr != nil {
				return "", fmt.Errorf("failed to get HEAD and branch %s does not exist: %w", branch, err)
			}
			err = gitRepo.Storer.SetReference(plumbing.NewHashReference(branchRef, headRef.Hash()))
			if err != nil {
				return "", fmt.Errorf("failed to create new branch %s: %w", branch, err)
			}
		}

		// Now checkout the branch
		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
			Create: false,
		})
		if err != nil {
			return "", fmt.Errorf("failed to checkout branch %s: %w", branch, err)
		}
	} else {
		// Branch exists locally, just checkout
		err = worktree.Checkout(&git.CheckoutOptions{
			Branch: branchRef,
			Create: false,
		})
		if err != nil {
			return "", fmt.Errorf("failed to checkout branch %s: %w", branch, err)
		}
	}

	fullPath := filepath.Join(tempClone, path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if _, err := worktree.Add(path); err != nil {
		return "", fmt.Errorf("failed to stage file: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get git status: %w", err)
	}

	if status.IsClean() {
		existingContent, readErr := os.ReadFile(fullPath)
		if readErr == nil && string(existingContent) == string(content) {
			return string(content), nil
		}
	}

	if authorName == "" {
		authorName = s.gitUserName
	}
	if authorEmail == "" {
		authorEmail = s.gitUserEmail
	}

	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}

	pushOptions := &git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	}
	if repo.IsExternal && repo.Password != "" {
		username := repo.Username
		if username == "" {
			username = "PAT"
		}
		pushOptions.Auth = &http.BasicAuth{
			Username: username,
			Password: repo.Password,
		}
	}
	err = gitRepo.Push(pushOptions)
	if err != nil {
		return "", fmt.Errorf("failed to push to repository: %w", err)
	}

	log.Info().
		Str("repo_id", repoID).
		Str("path", path).
		Str("branch", branch).
		Msg("Successfully created/updated file in repository")

	return string(content), nil
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
