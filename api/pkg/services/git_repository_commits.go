package services

import (
	"context"
	"fmt"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
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

	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	var ref *plumbing.Reference
	if req.Branch == "" {
		ref, err = gitRepo.Head()
		if err != nil {
			return nil, fmt.Errorf("failed to get HEAD: %w", err)
		}
	} else {
		ref, err = gitRepo.Reference(plumbing.NewBranchReferenceName(req.Branch), true)
		if err != nil {
			return nil, fmt.Errorf("failed to get branch reference: %w", err)
		}
	}

	var sinceTime, untilTime *time.Time
	if req.Since != "" {
		t, err := time.Parse(time.RFC3339, req.Since)
		if err != nil {
			return nil, fmt.Errorf("invalid since date format: %w", err)
		}
		sinceTime = &t
	}
	if req.Until != "" {
		t, err := time.Parse(time.RFC3339, req.Until)
		if err != nil {
			return nil, fmt.Errorf("invalid until date format: %w", err)
		}
		untilTime = &t
	}

	perPage := req.PerPage
	if perPage <= 0 {
		perPage = 30
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	skip := (page - 1) * perPage

	commitIter, err := gitRepo.Log(&git.LogOptions{
		From: ref.Hash(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get git log: %w", err)
	}

	var commits []*types.Commit
	count := 0
	skipped := 0

	errStopIteration := fmt.Errorf("stop iteration")

	err = commitIter.ForEach(func(commit *object.Commit) error {
		if sinceTime != nil && commit.Author.When.Before(*sinceTime) {
			return nil
		}
		if untilTime != nil && commit.Author.When.After(*untilTime) {
			return nil
		}

		if skipped < skip {
			skipped++
			return nil
		}

		if count >= perPage {
			return errStopIteration
		}

		commits = append(commits, &types.Commit{
			SHA:       commit.Hash.String(),
			Message:   commit.Message,
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Timestamp: commit.Author.When,
		})

		count++
		return nil
	})

	if err != nil && err != errStopIteration {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return &types.ListCommitsResponse{
		Commits: commits,
	}, nil
}
