package services

import (
	"context"
	"fmt"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
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
//
// If branchName is empty, uses the repository's DefaultBranch.
// If DefaultBranch is also empty, returns an error - use SyncAllBranches instead.
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

	// Use repository's default branch if not specified
	if branchName == "" {
		branchName = gitRepo.DefaultBranch
	}

	// If still empty, require explicit branch or use SyncAllBranches
	if branchName == "" {
		return fmt.Errorf("no branch specified and repository has no default branch set - use SyncAllBranches to fetch all branches")
	}

	repo, err := git.PlainOpen(gitRepo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open bare repository: %w", err)
	}

	auth := s.GetAuthConfig(gitRepo)

	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branchName, branchName))
	if force {
		refSpec = config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branchName, branchName))
	}

	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
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

// SyncAllBranches fetches ALL branches from the external remote into the local bare repository.
// This is used when initially importing a repository or when you need to ensure all
// remote branches are available locally.
//
// The method:
// 1. Opens the local bare repository
// 2. Fetches ALL branches from the "origin" remote
// 3. Updates local refs to match the remote (including new and deleted branches if prune=true)
func (s *GitRepositoryService) SyncAllBranches(ctx context.Context, repoID string, force bool) error {
	gitRepo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}

	if !gitRepo.IsExternal {
		return fmt.Errorf("repository is not external, cannot sync from remote")
	}

	if gitRepo.ExternalURL == "" {
		return fmt.Errorf("external URL is not configured")
	}

	repo, err := git.PlainOpen(gitRepo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open bare repository: %w", err)
	}

	auth := s.GetAuthConfig(gitRepo)

	// Fetch ALL branches using wildcard refspec
	refSpec := config.RefSpec("refs/heads/*:refs/heads/*")
	if force {
		refSpec = config.RefSpec("+refs/heads/*:refs/heads/*")
	}

	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
		Force:      force,
		// Prune removes local branches that no longer exist on remote
		Prune: true,
	}
	if auth != nil {
		fetchOpts.Auth = auth
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("external_url", gitRepo.ExternalURL).
		Bool("force", force).
		Msg("Syncing ALL branches from external repository")

	err = repo.Fetch(fetchOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch from remote: %w", err)
	}

	// Update the repository's branch list
	refs, err := repo.References()
	if err == nil {
		branches := []string{}
		refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Name().IsBranch() {
				branches = append(branches, ref.Name().Short())
			}
			return nil
		})
		gitRepo.Branches = branches
		gitRepo.UpdatedAt = time.Now()

		// Persist updated branch list
		if err := s.store.UpdateGitRepository(ctx, gitRepo); err != nil {
			log.Warn().Err(err).Str("repo_id", repoID).Msg("Failed to update repository branch list in database")
		}

		log.Info().
			Str("repo_id", gitRepo.ID).
			Int("branch_count", len(branches)).
			Strs("branches", branches).
			Msg("Successfully synced all branches from external repository")
	}

	return nil
}
