package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// CreateGitPullRequest inserts a pull request with the next per-repository
// number. Callers serialize creates per repository; the unique index on
// (repository_id, number) is the backstop.
func (s *PostgresStore) CreateGitPullRequest(ctx context.Context, pr *types.GitPullRequest) error {
	if pr.RepositoryID == "" {
		return errors.New("repository_id cannot be empty")
	}
	return s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxNumber *int
		if err := tx.Model(&types.GitPullRequest{}).
			Where("repository_id = ?", pr.RepositoryID).
			Select("MAX(number)").Scan(&maxNumber).Error; err != nil {
			return fmt.Errorf("allocate pull request number: %w", err)
		}
		next := 1
		if maxNumber != nil {
			next = *maxNumber + 1
		}
		pr.Number = next
		return tx.Create(pr).Error
	})
}

// GetGitPullRequest retrieves a pull request by its repository and number.
func (s *PostgresStore) GetGitPullRequest(ctx context.Context, repositoryID string, number int) (*types.GitPullRequest, error) {
	if repositoryID == "" {
		return nil, errors.New("repository_id cannot be empty")
	}
	var pr types.GitPullRequest
	err := s.gdb.WithContext(ctx).
		Where("repository_id = ? AND number = ?", repositoryID, number).
		First(&pr).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &pr, nil
}

// ListGitPullRequests lists a repository's pull requests, newest first.
// state is "" (all), "open" or "closed"; closed includes merged pull requests.
func (s *PostgresStore) ListGitPullRequests(ctx context.Context, repositoryID, state string) ([]*types.GitPullRequest, error) {
	if repositoryID == "" {
		return nil, errors.New("repository_id cannot be empty")
	}
	var prs []*types.GitPullRequest
	q := s.gdb.WithContext(ctx).
		Where("repository_id = ?", repositoryID).
		Order("number DESC")
	if state != "" {
		q = q.Where("state = ?", state)
	}
	if err := q.Find(&prs).Error; err != nil {
		return nil, err
	}
	return prs, nil
}

// UpdateGitPullRequest persists the whole row.
func (s *PostgresStore) UpdateGitPullRequest(ctx context.Context, pr *types.GitPullRequest) error {
	if pr.ID == "" {
		return errors.New("id cannot be empty")
	}
	return s.gdb.WithContext(ctx).Save(pr).Error
}

// DeleteGitPullRequestsForRepository removes every pull request row of a
// repository. Called when the repository is deleted.
func (s *PostgresStore) DeleteGitPullRequestsForRepository(ctx context.Context, repositoryID string) error {
	if repositoryID == "" {
		return errors.New("repository_id cannot be empty")
	}
	return s.gdb.WithContext(ctx).
		Where("repository_id = ?", repositoryID).
		Delete(&types.GitPullRequest{}).Error
}
