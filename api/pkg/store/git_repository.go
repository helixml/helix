package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// CreateGitRepository creates a new git repository record
func (s *PostgresStore) CreateGitRepository(ctx context.Context, repo *types.GitRepository) error {
	return s.gdb.WithContext(ctx).Create(repo).Error
}

// GetGitRepository retrieves a git repository by ID
func (s *PostgresStore) GetGitRepository(ctx context.Context, id string) (*types.GitRepository, error) {
	if id == "" {
		return nil, fmt.Errorf("id cannot be empty")
	}

	var repo types.GitRepository
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&repo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &repo, nil
}

// ListGitRepositories lists all git repositories, optionally filtered by owner
func (s *PostgresStore) ListGitRepositories(ctx context.Context, request *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error) {
	var repos []*types.GitRepository

	query := s.gdb.WithContext(ctx).Model(&types.GitRepository{})
	if request.OwnerID != "" {
		query = query.Where("git_repositories.owner_id = ?", request.OwnerID)
	}
	if request.OrganizationID != "" {
		query = query.Where("git_repositories.organization_id = ?", request.OrganizationID)
	}

	// Use junction table for project filtering (supports many-to-many)
	if request.ProjectID != "" {
		query = query.Joins("INNER JOIN project_repositories ON project_repositories.repository_id = git_repositories.id").
			Where("project_repositories.project_id = ?", request.ProjectID)
	}

	err := query.Order("git_repositories.created_at DESC").Find(&repos).Error
	if err != nil {
		return nil, err
	}

	return repos, nil
}

// UpdateGitRepository updates a git repository record
func (s *PostgresStore) UpdateGitRepository(ctx context.Context, repo *types.GitRepository) error {
	if repo.ID == "" {
		return fmt.Errorf("id not specified")
	}

	repo.UpdatedAt = time.Now()
	return s.gdb.WithContext(ctx).Model(&types.GitRepository{}).Where("id = ?", repo.ID).Save(repo).Error
}

// DeleteGitRepository deletes a git repository record
func (s *PostgresStore) DeleteGitRepository(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	return s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&types.GitRepository{}).Error
}
