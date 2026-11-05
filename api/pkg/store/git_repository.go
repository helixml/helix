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

func (s *PostgresStore) GetRepositoriesCount(ctx context.Context, query *GetRepositoriesCountQuery) (int64, error) {
	if query.UserID == "" && query.OrganizationID == "" {
		return 0, fmt.Errorf("user ID or organization ID is required")
	}

	q := s.gdb.WithContext(ctx).Model(&types.GitRepository{})

	if query.OrganizationID != "" {
		q = q.Where("organization_id = ?", query.OrganizationID)
	} else {
		q = q.Where("owner_id = ?", query.UserID)
	}

	var count int64
	err := q.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("error getting repositories count: %w", err)
	}
	return count, nil
}

// GetGitRepositoryByExternalURL looks up an external repository by its URL within an organization.
func (s *PostgresStore) GetGitRepositoryByExternalURL(ctx context.Context, orgID, externalURL string) (*types.GitRepository, error) {
	var repo types.GitRepository
	err := s.gdb.WithContext(ctx).
		Where("organization_id = ? AND external_url = ? AND is_external = true", orgID, externalURL).
		First(&repo).Error
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
	// Sync state is written concurrently by background git operations; a
	// whole-row save from a stale copy must not overwrite it.
	return s.gdb.WithContext(ctx).Model(&types.GitRepository{}).Where("id = ?", repo.ID).
		Omit("last_sync_at", "last_sync_error", "sync_in_progress_since").Save(repo).Error
}

// MarkGitRepositorySyncStarted records that an upstream fetch or push began.
func (s *PostgresStore) MarkGitRepositorySyncStarted(ctx context.Context, id string, startedAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	return s.gdb.WithContext(ctx).Model(&types.GitRepository{}).Where("id = ?", id).
		UpdateColumn("sync_in_progress_since", startedAt).Error
}

// MarkGitRepositorySyncFinished clears the in-progress marker and records the
// outcome: success sets last_sync_at and clears last_sync_error.
func (s *PostgresStore) MarkGitRepositorySyncFinished(ctx context.Context, id string, finishedAt time.Time, syncErr string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	updates := map[string]interface{}{
		"sync_in_progress_since": nil,
		"last_sync_error":        syncErr,
	}
	if syncErr == "" {
		updates["last_sync_at"] = finishedAt
	}
	return s.gdb.WithContext(ctx).Model(&types.GitRepository{}).Where("id = ?", id).
		UpdateColumns(updates).Error
}

// ClearStaleGitRepositorySync clears an in-progress marker left by a process
// that died mid-sync. It only matches the marker value the caller observed, so
// a sync started since then keeps its marker.
func (s *PostgresStore) ClearStaleGitRepositorySync(ctx context.Context, id string, staleSince time.Time, syncErr string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	return s.gdb.WithContext(ctx).Model(&types.GitRepository{}).
		Where("id = ? AND sync_in_progress_since = ?", id, staleSince).
		UpdateColumns(map[string]interface{}{
			"sync_in_progress_since": nil,
			"last_sync_error":        syncErr,
		}).Error
}

// CountGitRepositoriesByKoditRepoID counts how many git repositories (excluding
// excludeRepoID) reference the given kodit_repo_id in their JSONB metadata.
func (s *PostgresStore) CountGitRepositoriesByKoditRepoID(ctx context.Context, koditRepoID int64, excludeRepoID string) (int64, error) {
	var count int64
	err := s.gdb.WithContext(ctx).
		Model(&types.GitRepository{}).
		Where("id != ?", excludeRepoID).
		Where("(metadata->>'kodit_repo_id')::bigint = ?", koditRepoID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count git repositories by kodit repo ID: %w", err)
	}
	return count, nil
}

// DeleteGitRepository deletes a git repository record
func (s *PostgresStore) DeleteGitRepository(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	return s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&types.GitRepository{}).Error
}
