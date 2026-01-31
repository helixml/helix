package git

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
)

type GetPullRequestChangeOptions struct {
	RemoteURL      string
	TargetCommitID string
	SourceCommitID string
	Owner          string
	Repository     string
}

// GetPullRequestChange downloads the repository and returns changes between target and source commits
func GetPullRequestChange(ctx context.Context, token string, opts GetPullRequestChangeOptions) ([]*PullRequestChange, error) {
	logger := log.Ctx(ctx)

	gitRepoTempPath, err := os.MkdirTemp("", "helix-git-diff")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	defer func() {
		if cleanupErr := os.RemoveAll(gitRepoTempPath); cleanupErr != nil {
			logger.Warn().Err(cleanupErr).Str("temp_path", gitRepoTempPath).Msg("failed to cleanup temp directory")
		}
	}()

	manager := NewGitManager(opts.RemoteURL, token)

	repo, err := manager.CloneRepository(ctx, gitRepoTempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	err = manager.CheckoutCommit(ctx, repo, opts.TargetCommitID)
	if err != nil {
		return nil, fmt.Errorf("failed to checkout target commit: %w", err)
	}

	// Run git diff between the two commits
	changes, err := manager.Diff(ctx, gitRepoTempPath, opts.TargetCommitID, opts.SourceCommitID)
	if err != nil {
		return nil, fmt.Errorf("failed to run git diff: %w", err)
	}

	logger.Debug().
		Int("change_count", len(changes)).
		Msg("git diff completed successfully")

	return changes, nil
}
