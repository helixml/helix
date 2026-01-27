package services

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ExternalRepoWriteOptions configures the behavior of WithExternalRepoWrite
type ExternalRepoWriteOptions struct {
	// Branch is the branch being written to (required for rollback)
	Branch string

	// FailOnSyncError if true, returns error if pre-sync fails.
	// If false, logs warning and continues (use for non-critical paths like project creation)
	FailOnSyncError bool

	// FailOnPushError if true, returns error if post-push fails (after rollback).
	// If false, logs warning and continues (use for non-critical paths)
	FailOnPushError bool
}

// WithExternalRepoWrite executes a write operation on an external repo with proper sync semantics.
//
// For external repos, this function:
//  1. Pre-syncs from upstream (fetches latest state)
//  2. Captures the current branch ref (for rollback)
//  3. Executes the write function
//  4. Pushes to upstream
//  5. If push fails, rolls back local changes and returns error
//
// For non-external repos, this simply executes the write function.
//
// Usage:
//
//	err := gitRepoService.WithExternalRepoWrite(ctx, repo, opts, func() error {
//	    // Your write logic here
//	    return CommitFileToBareBranch(ctx, repo.LocalPath, branch, ...)
//	})
func (s *GitRepositoryService) WithExternalRepoWrite(
	ctx context.Context,
	repo *types.GitRepository,
	opts ExternalRepoWriteOptions,
	writeFunc func() error,
) error {
	// For non-external repos, just execute the write
	if !repo.IsExternal || repo.ExternalURL == "" {
		return writeFunc()
	}

	if opts.Branch == "" {
		return fmt.Errorf("branch is required for external repo write operations")
	}

	// 1. Pre-sync from upstream
	log.Debug().
		Str("repo_id", repo.ID).
		Str("branch", opts.Branch).
		Msg("Pre-syncing from upstream before write")

	if err := s.SyncAllBranches(ctx, repo.ID, false); err != nil {
		if opts.FailOnSyncError {
			return fmt.Errorf("failed to sync from upstream before writing (local may be out of sync): %w", err)
		}
		log.Warn().
			Err(err).
			Str("repo_id", repo.ID).
			Msg("Pre-sync from upstream failed (continuing with write)")
	}

	// 2. Capture current branch ref for rollback
	oldRef, _ := GetBranchCommitID(ctx, repo.LocalPath, opts.Branch)

	// 3. Execute the write operation
	if err := writeFunc(); err != nil {
		return err
	}

	// 4. Push to upstream
	log.Debug().
		Str("repo_id", repo.ID).
		Str("branch", opts.Branch).
		Msg("Pushing changes to upstream after write")

	if err := s.PushBranchToRemote(ctx, repo.ID, opts.Branch, false); err != nil {
		// 5. Rollback on push failure
		if oldRef != "" {
			log.Warn().
				Str("repo_id", repo.ID).
				Str("branch", opts.Branch).
				Str("rollback_to", ShortHash(oldRef)).
				Msg("Push to upstream failed, rolling back local changes")

			if rollbackErr := UpdateBranchRef(ctx, repo.LocalPath, opts.Branch, oldRef); rollbackErr != nil {
				log.Error().
					Err(rollbackErr).
					Str("repo_id", repo.ID).
					Str("branch", opts.Branch).
					Msg("Failed to rollback branch ref after push failure")
			}
		}

		if opts.FailOnPushError {
			return fmt.Errorf("failed to push to upstream (local changes rolled back): %w", err)
		}
		log.Warn().
			Err(err).
			Str("repo_id", repo.ID).
			Str("branch", opts.Branch).
			Msg("Push to upstream failed (changes rolled back)")
	}

	log.Debug().
		Str("repo_id", repo.ID).
		Str("branch", opts.Branch).
		Msg("Successfully wrote to external repo and pushed to upstream")

	return nil
}

// WithExternalRepoRead executes a read operation on an external repo with proper sync semantics.
//
// For external repos, this function:
//  1. Syncs from upstream (fetches latest state)
//  2. Executes the read function
//
// For non-external repos, this simply executes the read function.
//
// Usage:
//
//	var content string
//	err := gitRepoService.WithExternalRepoRead(ctx, repo, func() error {
//	    var err error
//	    content, err = GetFileContents(ctx, repo.LocalPath, path, branch)
//	    return err
//	})
func (s *GitRepositoryService) WithExternalRepoRead(
	ctx context.Context,
	repo *types.GitRepository,
	readFunc func() error,
) error {
	// For non-external repos, just execute the read
	if !repo.IsExternal || repo.ExternalURL == "" {
		return readFunc()
	}

	// Sync from upstream before reading
	log.Debug().
		Str("repo_id", repo.ID).
		Msg("Syncing from upstream before read")

	if err := s.SyncAllBranches(ctx, repo.ID, false); err != nil {
		// Don't fail reads on sync error - local data may still be useful
		log.Warn().
			Err(err).
			Str("repo_id", repo.ID).
			Msg("Sync from upstream failed before read (continuing with local data)")
	}

	return readFunc()
}

// MustSyncBeforeRead is like WithExternalRepoRead but fails if sync fails.
// Use this when stale data is not acceptable.
func (s *GitRepositoryService) MustSyncBeforeRead(
	ctx context.Context,
	repo *types.GitRepository,
	readFunc func() error,
) error {
	// For non-external repos, just execute the read
	if !repo.IsExternal || repo.ExternalURL == "" {
		return readFunc()
	}

	// Sync from upstream before reading - fail if sync fails
	log.Debug().
		Str("repo_id", repo.ID).
		Msg("Syncing from upstream before read (strict mode)")

	if err := s.SyncAllBranches(ctx, repo.ID, false); err != nil {
		return fmt.Errorf("failed to sync from upstream before reading: %w", err)
	}

	return readFunc()
}
