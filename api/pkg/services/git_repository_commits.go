package services

import (
	"context"
	"fmt"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *GitRepositoryService) ListCommits(ctx context.Context, req *types.ListCommitsRequest) (*types.ListCommitsResponse, error) {
	repo, err := s.GetRepository(ctx, req.RepoID)
	if err != nil {
		return nil, err
	}

	if repo.LocalPath == "" {
		return nil, fmt.Errorf("repository has no local path")
	}

	// Open repository using gitea's wrapper
	gitRepo, err := giteagit.OpenRepository(ctx, repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}
	defer gitRepo.Close()

	// Determine starting ref and get the commit
	var startCommit *giteagit.Commit
	if req.Branch == "" {
		// Use HEAD
		headBranch, err := GetHEADBranch(ctx, repo.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("failed to get HEAD: %w", err)
		}
		startCommit, err = gitRepo.GetBranchCommit(headBranch)
		if err != nil {
			return nil, fmt.Errorf("failed to get HEAD commit: %w", err)
		}
	} else {
		// Verify branch exists and get commit
		startCommit, err = gitRepo.GetBranchCommit(req.Branch)
		if err != nil {
			return nil, fmt.Errorf("failed to get branch commit: %w", err)
		}
	}

	// Parse time filters
	var sinceStr, untilStr string
	if req.Since != "" {
		t, err := time.Parse(time.RFC3339, req.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid since date format: %w", err)
		}
		sinceStr = t.Format(time.RFC3339)
	}
	if req.Until != "" {
		t, err := time.Parse(time.RFC3339, req.Until)
		if err != nil {
			return nil, fmt.Errorf("invalid until date format: %w", err)
		}
		untilStr = t.Format(time.RFC3339)
	}

	perPage := req.PerPage
	if perPage <= 0 {
		perPage = 30
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}

	// Use gitea's high-level CommitsByRange API
	// Parameters: page, pageSize, not, since, until
	giteaCommits, err := startCommit.CommitsByRange(page, perPage, "", sinceStr, untilStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get commits: %w", err)
	}

	// Get total count for pagination
	// Note: CommitsCount returns total commits from this commit back (ignores time filters)
	// For accurate total with filters, we'd need to count separately
	totalCount, err := startCommit.CommitsCount()
	if err != nil {
		// Non-fatal, just use current page size as estimate
		totalCount = int64(len(giteaCommits))
	}

	// Convert to our Commit type
	var commits []*types.Commit
	for _, c := range giteaCommits {
		commits = append(commits, &types.Commit{
			SHA:       c.ID.String(),
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Timestamp: c.Author.When,
			Message:   c.Summary(),
		})
	}

	return &types.ListCommitsResponse{
		Commits: commits,
		Total:   int(totalCount),
		Page:    page,
		PerPage: perPage,
	}, nil
}
