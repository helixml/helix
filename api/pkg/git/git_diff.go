package git

import (
	"context"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/netguard"
)

type GetPullRequestChangeOptions struct {
	RemoteURL      string
	TargetCommitID string
	SourceCommitID string
	Owner          string
	Repository     string

	// Outbound, when set, restricts the addresses RemoteURL may reach.
	Outbound *netguard.Policy
}

// GetPullRequestChange downloads the repository and returns changes between target and source commits
func GetPullRequestChange(ctx context.Context, token string, opts GetPullRequestChangeOptions) ([]*PullRequestChange, error) {
	logger := log.Ctx(ctx)

	// Commit IDs come from webhook payloads; check them before any git
	// command (the checkout below takes the target as an argument).
	for _, commit := range []string{opts.TargetCommitID, opts.SourceCommitID} {
		if !commitIDPattern.MatchString(commit) {
			return nil, fmt.Errorf("invalid commit ID %q", commit)
		}
	}

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
	if opts.Outbound != nil {
		manager.WithOutboundPolicy(*opts.Outbound)
	}

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
