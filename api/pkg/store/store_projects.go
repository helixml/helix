package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateProject creates a new project
func (s *PostgresStore) CreateProject(ctx context.Context, project *types.Project) (*types.Project, error) {
	err := s.gdb.WithContext(ctx).Create(project).Error
	if err != nil {
		return nil, fmt.Errorf("error creating project: %w", err)
	}
	return project, nil
}

// GetProject gets a project by ID
func (s *PostgresStore) GetProject(ctx context.Context, projectID string) (*types.Project, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project ID is required")
	}

	var project types.Project
	err := s.gdb.WithContext(ctx).Where("id = ?", projectID).First(&project).Error
	if err != nil {
		return nil, fmt.Errorf("error getting project: %w", err)
	}
	return &project, nil
}

// UpdateProject updates an existing project
func (s *PostgresStore) UpdateProject(ctx context.Context, project *types.Project) error {
	if project.ID == "" {
		return fmt.Errorf("project ID is required")
	}

	err := s.gdb.WithContext(ctx).Save(project).Error
	if err != nil {
		return fmt.Errorf("error updating project: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetProjectsCount(ctx context.Context, query *GetProjectsCountQuery) (int64, error) {
	if query.UserID == "" && query.OrganizationID == "" {
		return 0, fmt.Errorf("user ID or organization ID is required")
	}

	var count int64
	q := s.gdb.WithContext(ctx).Model(&types.Project{})
	if query.UserID != "" {
		q = q.Where("user_id = ?", query.UserID)
	}
	if query.OrganizationID != "" {
		q = q.Where("organization_id = ?", query.OrganizationID)
	}
	err := q.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("error getting projects count: %w", err)
	}
	return count, nil
}

// ListProjects lists all projects for a given user
func (s *PostgresStore) ListProjects(ctx context.Context, req *ListProjectsQuery) ([]*types.Project, error) {
	if req.UserID == "" && req.OrganizationID == "" {
		return []*types.Project{}, nil
	}

	var projects []*types.Project

	q := s.gdb.WithContext(ctx).Model(&types.Project{})

	if req.UserID != "" {
		q = q.Where("user_id = ?", req.UserID)
	}

	if req.OrganizationID != "" {
		q = q.Where("organization_id = ?", req.OrganizationID)
	} else {
		// If we are listing just for the user, we don't want to filter by organization
		q = q.Where("organization_id IS NULL OR organization_id = ''")
	}

	err := q.Order("created_at DESC").Find(&projects).Error
	if err != nil {
		return nil, fmt.Errorf("error listing projects: %w", err)
	}

	if req.IncludeStats {
		err = s.populateProjectStats(ctx, projects)
		if err != nil {
			return nil, fmt.Errorf("error populating project stats: %w", err)
		}
	}

	return projects, nil
}

type projectTaskStats struct {
	ProjectID              string  `gorm:"column:project_id"`
	TotalTasks             int     `gorm:"column:total_tasks"`
	BacklogTasks           int     `gorm:"column:backlog_tasks"`
	PlanningTasks          int     `gorm:"column:planning_tasks"`
	InProgressTasks        int     `gorm:"column:in_progress_tasks"`
	PendingReviewTasks     int     `gorm:"column:pending_review_tasks"`
	CompletedTasks         int     `gorm:"column:completed_tasks"`
	TotalCompletionHours   float64 `gorm:"column:total_completion_hours"`
	CompletedTasksWithTime int     `gorm:"column:completed_tasks_with_time"`
}

func (s *PostgresStore) populateProjectStats(ctx context.Context, projects []*types.Project) error {
	if len(projects) == 0 {
		return nil
	}

	projectIDs := make([]string, len(projects))
	for i, p := range projects {
		projectIDs[i] = p.ID
	}

	var stats []projectTaskStats
	err := s.gdb.WithContext(ctx).Raw(`
		SELECT
			project_id,
			COUNT(*) as total_tasks,
			COUNT(*) FILTER (WHERE status = 'done') as completed_tasks,
			COUNT(*) FILTER (WHERE status IN ('spec_generation', 'implementation', 'spec_revision', 'queued_implementation', 'queued_spec_generation')) as in_progress_tasks,
			COUNT(*) FILTER (WHERE status = 'backlog') as backlog_tasks,
			COUNT(*) FILTER (WHERE status = 'planning') as planning_tasks,
			COUNT(*) FILTER (WHERE status IN ('spec_review', 'implementation_review', 'pull_request')) as pending_review_tasks,
			COALESCE(SUM(EXTRACT(EPOCH FROM (completed_at - created_at)) / 3600) FILTER (WHERE status = 'done' AND completed_at IS NOT NULL), 0) as total_completion_hours,
			COUNT(*) FILTER (WHERE status = 'done' AND completed_at IS NOT NULL) as completed_tasks_with_time
		FROM spec_tasks
		WHERE project_id IN (?) AND archived = false
		GROUP BY project_id
	`, projectIDs).Scan(&stats).Error
	if err != nil {
		return fmt.Errorf("error fetching project stats: %w", err)
	}

	statsMap := make(map[string]projectTaskStats)
	for _, s := range stats {
		statsMap[s.ProjectID] = s
	}

	for _, p := range projects {
		if s, ok := statsMap[p.ID]; ok {
			p.Stats = types.ProjectStats{
				TotalTasks:         s.TotalTasks,
				CompletedTasks:     s.CompletedTasks,
				InProgressTasks:    s.InProgressTasks,
				BacklogTasks:       s.BacklogTasks,
				PlanningTasks:      s.PlanningTasks,
				PendingReviewTasks: s.PendingReviewTasks,
			}
			if s.CompletedTasksWithTime > 0 {
				p.Stats.AverageTaskTime = s.TotalCompletionHours / float64(s.CompletedTasksWithTime)
			}
		}
	}

	return nil
}

// DeleteProject deletes a project by ID
func (s *PostgresStore) DeleteProject(ctx context.Context, projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.Project{}, "id = ?", projectID).Error
	if err != nil {
		return fmt.Errorf("error deleting project: %w", err)
	}
	return nil
}

// GetProjectRepositories gets all repositories attached to a project
// func (s *PostgresStore) GetProjectRepositories(ctx context.Context, projectID string) ([]*types.GitRepository, error) {
// 	if projectID == "" {
// 		return nil, fmt.Errorf("project id not specified")
// 	}

// 	var repos []*types.GitRepository
// 	err := s.gdb.WithContext(ctx).Where("project_id = ?", projectID).Find(&repos).Error
// 	if err != nil {
// 		return nil, fmt.Errorf("error getting project repositories: %w", err)
// 	}
// 	return repos, nil
// }
