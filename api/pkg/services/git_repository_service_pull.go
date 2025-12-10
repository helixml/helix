package services

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/rs/zerolog/log"
)

// PullFromRemote fetches changes from the external remote origin into the local bare repository.
// This is used to update the local bare repo when changes have been made externally
// (e.g., on GitHub, Azure DevOps, etc.).
//
// The method:
// 1. Opens the local bare repository
// 2. Fetches from the "origin" remote (the external URL)
// 3. Updates local refs to match the remote
func (s *GitRepositoryService) PullFromRemote(ctx context.Context, repoID, branchName string, force bool) error {
	gitRepo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}

	if !gitRepo.IsExternal {
		return fmt.Errorf("repository is not external, cannot pull from remote")
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

	repo, err := git.PlainOpen(gitRepo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open bare repository: %w", err)
	}

	auth := s.getAuthConfig(gitRepo)

	var refSpecs []config.RefSpec
	if branchName != "" {
		refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branchName, branchName))
		if force {
			refSpec = config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branchName, branchName))
		}
		refSpecs = []config.RefSpec{refSpec}
	} else {
		refSpec := config.RefSpec("refs/heads/*:refs/heads/*")
		if force {
			refSpec = config.RefSpec("+refs/heads/*:refs/heads/*")
		}
		refSpecs = []config.RefSpec{refSpec}
	}

	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   refSpecs,
		Force:      force,
	}
	if auth != nil {
		fetchOpts.Auth = auth
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Str("external_url", gitRepo.ExternalURL).
		Bool("force", force).
		Msg("Fetching changes from external repository")

	err = repo.Fetch(fetchOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch from remote: %w", err)
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Msg("Successfully fetched changes from external repository")

	return nil
}
