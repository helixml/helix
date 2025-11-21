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
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/rs/zerolog/log"
)

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

	branchRef := plumbing.NewBranchReferenceName(branch)
	_, err = gitRepo.Reference(branchRef, true)
	if err != nil {
		headRef, headErr := gitRepo.Head()
		if headErr != nil {
			return "", fmt.Errorf("failed to get HEAD and branch %s does not exist: %w", branch, err)
		}
		branchRef = plumbing.NewBranchReferenceName(branch)
		err = gitRepo.Storer.SetReference(plumbing.NewHashReference(branchRef, headRef.Hash()))
		if err != nil {
			return "", fmt.Errorf("failed to create branch %s: %w", branch, err)
		}
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Create: false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to checkout branch %s: %w", branch, err)
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
