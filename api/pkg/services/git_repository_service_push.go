package services

import (
	"context"
	"fmt"
	"os"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/rs/zerolog/log"
)

// PushBranchToRemote pushes a local branch from the bare repository to an external remote.
// This is used when you have local commits in the Helix bare repo that need to be pushed
// to an external repository (GitHub, Azure DevOps, etc.).
//
// The method:
// 1. Creates a temporary working copy from the local bare repo
// 2. Adds the external URL as a remote called "external"
// 3. Pushes the specified branch to the external remote
// 4. Cleans up the temporary working copy
func (s *GitRepositoryService) PushBranchToRemote(ctx context.Context, repoID, branchName string, force bool) error {
	gitRepo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}

	if !gitRepo.IsExternal {
		return fmt.Errorf("repository is not external, cannot push to remote")
	}

	if gitRepo.ExternalURL == "" {
		return fmt.Errorf("external URL is not configured")
	}

	if branchName == "" {
		branchName = gitRepo.DefaultBranch
		if branchName == "" {
			branchName = "main"
		}
	}

	auth := s.getAuthConfig(gitRepo)

	wc, err := s.getWorkingCopyForExternalPush(gitRepo.LocalPath, gitRepo.ExternalURL, branchName, auth)
	if err != nil {
		return fmt.Errorf("failed to create working copy: %w", err)
	}
	defer wc.Cleanup()

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Str("external_url", gitRepo.ExternalURL).
		Bool("force", force).
		Msg("Pushing local branch to external repository")

	err = wc.PushToExternal(branchName, force, auth)
	if err != nil {
		return fmt.Errorf("failed to push to external repository: %w", err)
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Msg("Successfully pushed local branch to external repository")

	return nil
}

// PushToExternal pushes changes to the "external" remote with optional force and auth
func (wc *WorkingCopy) PushToExternal(branch string, force bool, auth transport.AuthMethod) error {
	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
	if force {
		refSpec = config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch))
	}
	opts := &git.PushOptions{
		RemoteName: "external",
		RefSpecs:   []config.RefSpec{refSpec},
	}
	if auth != nil {
		opts.Auth = auth
	}
	err := wc.Repo.Push(opts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push to external: %w", err)
	}
	return nil
}

// getWorkingCopyForExternalPush creates a temporary working copy from the local bare repo
// with the external URL added as a remote called "external".
// This is used for pushing local changes to an external repository.
// The caller MUST call Cleanup() when done to remove the temporary directory.
func (s *GitRepositoryService) getWorkingCopyForExternalPush(
	bareRepoPath string,
	externalURL string,
	branch string,
	auth transport.AuthMethod,
) (*WorkingCopy, error) {
	tempDir, err := os.MkdirTemp("", "helix-git-push-workdir-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	cloneOpts := &git.CloneOptions{
		URL: bareRepoPath,
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

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "external",
		URLs: []string{externalURL},
	})
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to add external remote: %w", err)
	}

	return &WorkingCopy{
		TempDir:  tempDir,
		Repo:     repo,
		Worktree: worktree,
	}, nil
}
