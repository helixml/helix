package services

import (
	"context"
	"fmt"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// BranchDivergenceError is returned when local and upstream branches have diverged
type BranchDivergenceError struct {
	BranchName   string
	LocalAhead   int
	LocalBehind  int
	LocalCommit  string
	RemoteCommit string
}

func (e *BranchDivergenceError) Error() string {
	return fmt.Sprintf("branch '%s' has diverged: local is %d commits ahead and %d commits behind upstream",
		e.BranchName, e.LocalAhead, e.LocalBehind)
}

// SyncBaseBranch syncs ONLY the specified branch from the external remote.
// This is used before starting a new SpecTask to ensure the base branch is up-to-date.
//
// The method:
// 1. Fetches the branch from remote to a temporary ref (refs/remotes/origin/<branch>)
// 2. Compares with local branch to detect divergence
// 3. If fast-forward possible: updates local branch
// 4. If diverged: returns BranchDivergenceError with details
//
// If branchName is empty, uses the repository's DefaultBranch.
func (s *GitRepositoryService) SyncBaseBranch(ctx context.Context, repoID, branchName string) error {
	gitRepo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}

	// For non-external repos, nothing to sync
	if !gitRepo.IsExternal {
		log.Debug().Str("repo_id", repoID).Msg("Repository is not external, skipping base branch sync")
		return nil
	}

	if gitRepo.ExternalURL == "" {
		return fmt.Errorf("external URL is not configured")
	}

	// Use repository's default branch if not specified
	if branchName == "" {
		branchName = gitRepo.DefaultBranch
	}
	if branchName == "" {
		return fmt.Errorf("no branch specified and repository has no default branch set")
	}

	repo, err := git.PlainOpen(gitRepo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open bare repository: %w", err)
	}

	auth := s.GetAuthConfig(gitRepo)

	// First, fetch the branch to a remote-tracking ref for comparison
	// Use refs/remotes/origin/<branch> so we can compare before updating local
	fetchRefSpec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branchName, branchName))
	fetchOpts := &git.FetchOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{fetchRefSpec},
		Force:      true, // Always update remote-tracking ref
	}
	if auth != nil {
		fetchOpts.Auth = auth
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Msg("Fetching base branch from upstream for sync check")

	err = repo.Fetch(fetchOpts)
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch from remote: %w", err)
	}

	// Get local branch ref
	localRef, err := repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err != nil {
		// Local branch doesn't exist - this is OK, we'll create it
		log.Info().
			Str("repo_id", gitRepo.ID).
			Str("branch", branchName).
			Msg("Local branch doesn't exist, will create from upstream")

		// Get remote ref and create local branch pointing to same commit
		remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branchName), true)
		if err != nil {
			return fmt.Errorf("failed to get remote branch reference: %w", err)
		}

		// Create local branch
		newLocalRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), remoteRef.Hash())
		if err := repo.Storer.SetReference(newLocalRef); err != nil {
			return fmt.Errorf("failed to create local branch: %w", err)
		}

		log.Info().
			Str("repo_id", gitRepo.ID).
			Str("branch", branchName).
			Str("commit", remoteRef.Hash().String()[:8]).
			Msg("Created local branch from upstream")
		return nil
	}
	localHash := localRef.Hash()

	// Get remote ref
	remoteRef, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branchName), true)
	if err != nil {
		return fmt.Errorf("failed to get remote branch reference: %w", err)
	}
	remoteHash := remoteRef.Hash()

	// If they're the same, nothing to do
	if localHash == remoteHash {
		log.Info().
			Str("repo_id", gitRepo.ID).
			Str("branch", branchName).
			Msg("Base branch is already up-to-date with upstream")
		return nil
	}

	// Check for divergence using the existing helper
	ahead, behind, err := s.countCommitsDiff(repo, localHash, remoteHash)
	if err != nil {
		return fmt.Errorf("failed to check divergence: %w", err)
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Int("ahead", ahead).
		Int("behind", behind).
		Str("local", localHash.String()[:8]).
		Str("remote", remoteHash.String()[:8]).
		Msg("Comparing local and upstream branches")

	// If local has commits not in remote, we have divergence
	if ahead > 0 {
		return &BranchDivergenceError{
			BranchName:   branchName,
			LocalAhead:   ahead,
			LocalBehind:  behind,
			LocalCommit:  localHash.String(),
			RemoteCommit: remoteHash.String(),
		}
	}

	// Fast-forward: local is behind, no local-only commits
	// Update local branch to point to remote commit
	newLocalRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), remoteHash)
	if err := repo.Storer.SetReference(newLocalRef); err != nil {
		return fmt.Errorf("failed to fast-forward local branch: %w", err)
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Int("commits_synced", behind).
		Str("old_commit", localHash.String()[:8]).
		Str("new_commit", remoteHash.String()[:8]).
		Msg("Fast-forwarded base branch to upstream")

	return nil
}

// IsBranchDivergenceError checks if an error is a BranchDivergenceError
func IsBranchDivergenceError(err error) bool {
	_, ok := err.(*BranchDivergenceError)
	return ok
}

// GetBranchDivergenceError extracts BranchDivergenceError from an error
func GetBranchDivergenceError(err error) *BranchDivergenceError {
	if divergeErr, ok := err.(*BranchDivergenceError); ok {
		return divergeErr
	}
	return nil
}

// FormatDivergenceErrorForUser returns a user-friendly error message for branch divergence
func FormatDivergenceErrorForUser(err *BranchDivergenceError, repoName string) string {
	return fmt.Sprintf(`Cannot sync base branch '%s': local and upstream have diverged.

Local branch has %d commits not in upstream.
Upstream branch has %d commits not in local.

This can happen when:
- Someone pushed directly to the Helix copy of this branch
- The upstream branch was force-pushed
- A previous Helix task's changes were merged differently

To resolve:
1. Go to the external repository and reconcile the branches manually
2. Or use "Force Sync" in Helix to overwrite local with upstream (warning: loses local changes)

Repository: %s
Branch: %s
Local commit: %s
Upstream commit: %s`,
		err.BranchName,
		err.LocalAhead,
		err.LocalBehind,
		repoName,
		err.BranchName,
		err.LocalCommit[:8],
		err.RemoteCommit[:8],
	)
}

// SyncBaseBranchResult contains the result of a base branch sync operation
type SyncBaseBranchResult struct {
	Synced       bool   // True if sync was performed (vs already up-to-date)
	CommitsBefore string // Commit hash before sync
	CommitsAfter  string // Commit hash after sync
	CommitsSynced int    // Number of commits synced
}

// SyncBaseBranchWithResult syncs the base branch and returns detailed result
func (s *GitRepositoryService) SyncBaseBranchWithResult(ctx context.Context, repoID, branchName string) (*SyncBaseBranchResult, error) {
	// For now, just wrap the basic sync - can be enhanced later to return more details
	err := s.SyncBaseBranch(ctx, repoID, branchName)
	if err != nil {
		return nil, err
	}
	return &SyncBaseBranchResult{Synced: true}, nil
}

// SyncBaseBranchForTask syncs the base branch for a SpecTask, handling errors appropriately
func (s *GitRepositoryService) SyncBaseBranchForTask(ctx context.Context, task *types.SpecTask, repos []*types.GitRepository) error {
	// Determine base branch - use task's BaseBranch or fall back to repo default
	baseBranch := task.BaseBranch

	for _, repo := range repos {
		if !repo.IsExternal {
			continue
		}

		// Use repo's default branch if task doesn't specify
		branchToSync := baseBranch
		if branchToSync == "" {
			branchToSync = repo.DefaultBranch
		}

		log.Info().
			Str("task_id", task.ID).
			Str("repo_id", repo.ID).
			Str("branch", branchToSync).
			Msg("Syncing base branch before starting task")

		err := s.SyncBaseBranch(ctx, repo.ID, branchToSync)
		if err != nil {
			if divergeErr := GetBranchDivergenceError(err); divergeErr != nil {
				// Return formatted error for divergence
				return fmt.Errorf(FormatDivergenceErrorForUser(divergeErr, repo.Name))
			}
			return fmt.Errorf("failed to sync base branch '%s' for repository '%s': %w", branchToSync, repo.Name, err)
		}
	}

	return nil
}

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
