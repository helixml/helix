package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateProjectRepository creates a new project-repository relationship.
// This is idempotent - if the relationship already exists, it does nothing.
func (s *PostgresStore) CreateProjectRepository(ctx context.Context, projectID, repositoryID, organizationID string) error {
	if projectID == "" {
		return fmt.Errorf("project_id not specified")
	}
	if repositoryID == "" {
		return fmt.Errorf("repository_id not specified")
	}

	now := time.Now()
	pr := &types.ProjectRepository{
		ProjectID:      projectID,
		RepositoryID:   repositoryID,
		OrganizationID: organizationID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	err := s.gdb.WithContext(ctx).Create(pr).Error
	if err != nil {
		// Handle duplicate key - this is idempotent, so we just return nil
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return nil
		}
		return fmt.Errorf("failed to create project repository: %w", err)
	}
	return nil
}

// DeleteProjectRepository removes a specific project-repository relationship
func (s *PostgresStore) DeleteProjectRepository(ctx context.Context, projectID, repositoryID string) error {
	if projectID == "" {
		return fmt.Errorf("project_id not specified")
	}
	if repositoryID == "" {
		return fmt.Errorf("repository_id not specified")
	}

	err := s.gdb.WithContext(ctx).
		Where("project_id = ? AND repository_id = ?", projectID, repositoryID).
		Delete(&types.ProjectRepository{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete project repository: %w", err)
	}
	return nil
}

// DeleteProjectRepositoriesByProject removes all repository attachments for a project.
// Used for cascade delete when a project is deleted.
func (s *PostgresStore) DeleteProjectRepositoriesByProject(ctx context.Context, projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project_id not specified")
	}

	err := s.gdb.WithContext(ctx).
		Where("project_id = ?", projectID).
		Delete(&types.ProjectRepository{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete project repositories by project: %w", err)
	}
	return nil
}

// DeleteProjectRepositoriesByRepository removes all project attachments for a repository.
// Used for cascade delete when a repository is deleted.
func (s *PostgresStore) DeleteProjectRepositoriesByRepository(ctx context.Context, repositoryID string) error {
	if repositoryID == "" {
		return fmt.Errorf("repository_id not specified")
	}

	err := s.gdb.WithContext(ctx).
		Where("repository_id = ?", repositoryID).
		Delete(&types.ProjectRepository{}).Error
	if err != nil {
		return fmt.Errorf("failed to delete project repositories by repository: %w", err)
	}
	return nil
}

// ListProjectRepositories lists project-repository relationships based on query parameters
func (s *PostgresStore) ListProjectRepositories(ctx context.Context, q *types.ListProjectRepositoriesQuery) ([]*types.ProjectRepository, error) {
	query := s.gdb.WithContext(ctx)

	if q != nil {
		if q.ProjectID != "" {
			query = query.Where("project_id = ?", q.ProjectID)
		}
		if q.RepositoryID != "" {
			query = query.Where("repository_id = ?", q.RepositoryID)
		}
		if q.OrganizationID != "" {
			query = query.Where("organization_id = ?", q.OrganizationID)
		}
	}

	var prs []*types.ProjectRepository
	err := query.Find(&prs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list project repositories: %w", err)
	}

	return prs, nil
}

// GetProjectsForRepository returns all project IDs that have this repository attached.
// This is used by git_http_server.go to find all projects when processing pushes.
func (s *PostgresStore) GetProjectsForRepository(ctx context.Context, repositoryID string) ([]string, error) {
	if repositoryID == "" {
		return nil, fmt.Errorf("repository_id not specified")
	}

	var prs []*types.ProjectRepository
	err := s.gdb.WithContext(ctx).
		Where("repository_id = ?", repositoryID).
		Find(&prs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get projects for repository: %w", err)
	}

	projectIDs := make([]string, len(prs))
	for i, pr := range prs {
		projectIDs[i] = pr.ProjectID
	}
	return projectIDs, nil
}

// GetRepositoriesForProject returns all repository IDs attached to this project.
func (s *PostgresStore) GetRepositoriesForProject(ctx context.Context, projectID string) ([]string, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id not specified")
	}

	var prs []*types.ProjectRepository
	err := s.gdb.WithContext(ctx).
		Where("project_id = ?", projectID).
		Find(&prs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get repositories for project: %w", err)
	}

	repoIDs := make([]string, len(prs))
	for i, pr := range prs {
		repoIDs[i] = pr.RepositoryID
	}
	return repoIDs, nil
}

// migrateProjectRepositories migrates existing project_id values from git_repositories
// to the project_repositories junction table. This is a one-time migration that runs
// on startup and is idempotent.
func (s *PostgresStore) migrateProjectRepositories(ctx context.Context) error {
	// Find all repositories with a project_id set that don't have a junction table entry
	var repos []*types.GitRepository
	err := s.gdb.WithContext(ctx).
		Where("project_id IS NOT NULL AND project_id != '' AND project_id != '00000000-0000-0000-0000-000000000000'").
		Find(&repos).Error
	if err != nil {
		return fmt.Errorf("failed to find repositories with project_id: %w", err)
	}

	if len(repos) == 0 {
		return nil
	}

	migrated := 0
	for _, repo := range repos {
		// Check if junction entry already exists
		var count int64
		s.gdb.WithContext(ctx).Model(&types.ProjectRepository{}).
			Where("project_id = ? AND repository_id = ?", repo.ProjectID, repo.ID).
			Count(&count)

		if count > 0 {
			continue // Already migrated
		}

		// Create junction entry
		now := time.Now()
		pr := &types.ProjectRepository{
			ProjectID:      repo.ProjectID,
			RepositoryID:   repo.ID,
			OrganizationID: repo.OrganizationID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		err := s.gdb.WithContext(ctx).Create(pr).Error
		if err != nil {
			// Log but continue - we'll retry on next startup
			continue
		}
		migrated++
	}

	if migrated > 0 {
		// Log migration success (using fmt since we don't import log here)
		// The caller will log the result
	}

	return nil
}
