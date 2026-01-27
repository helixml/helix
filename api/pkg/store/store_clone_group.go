package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// CreateCloneGroup creates a new clone group record
func (s *PostgresStore) CreateCloneGroup(ctx context.Context, group *types.CloneGroup) (*types.CloneGroup, error) {
	if group.ID == "" {
		group.ID = system.GenerateCloneGroupID()
	}
	group.CreatedAt = time.Now()

	if err := s.gdb.WithContext(ctx).Create(group).Error; err != nil {
		return nil, fmt.Errorf("failed to create clone group: %w", err)
	}
	return group, nil
}

// GetCloneGroup retrieves a clone group by ID
func (s *PostgresStore) GetCloneGroup(ctx context.Context, id string) (*types.CloneGroup, error) {
	var group types.CloneGroup
	if err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&group).Error; err != nil {
		return nil, fmt.Errorf("clone group not found: %w", err)
	}
	return &group, nil
}

// ListCloneGroupsForTask returns all clone groups where the task was the source
func (s *PostgresStore) ListCloneGroupsForTask(ctx context.Context, taskID string) ([]*types.CloneGroup, error) {
	var groups []*types.CloneGroup
	if err := s.gdb.WithContext(ctx).
		Where("source_task_id = ?", taskID).
		Order("created_at DESC").
		Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("failed to list clone groups: %w", err)
	}
	return groups, nil
}

// GetCloneGroupProgress calculates progress for all tasks in a clone group
func (s *PostgresStore) GetCloneGroupProgress(ctx context.Context, groupID string) (*types.CloneGroupProgress, error) {
	// Get the clone group
	group, err := s.GetCloneGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}

	// Get all tasks in this clone group
	var tasks []types.SpecTask
	if err := s.gdb.WithContext(ctx).
		Where("clone_group_id = ?", groupID).
		Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("failed to get tasks for clone group: %w", err)
	}

	// Get project names for all tasks
	projectIDs := make([]string, 0, len(tasks)+1)
	projectIDs = append(projectIDs, group.SourceProjectID)
	for _, t := range tasks {
		projectIDs = append(projectIDs, t.ProjectID)
	}

	var projects []types.Project
	if err := s.gdb.WithContext(ctx).
		Where("id IN ?", projectIDs).
		Find(&projects).Error; err != nil {
		log.Warn().Err(err).Msg("Failed to fetch project names for clone group")
	}

	projectNames := make(map[string]string)
	for _, p := range projects {
		projectNames[p.ID] = p.Name
	}

	// Build progress response
	progress := &types.CloneGroupProgress{
		CloneGroupID: groupID,
		SourceTask: &types.CloneGroupSourceTask{
			TaskID:      group.SourceTaskID,
			ProjectID:   group.SourceProjectID,
			ProjectName: projectNames[group.SourceProjectID],
			Name:        group.SourceTaskName,
		},
		Tasks:           make([]types.CloneGroupTaskProgress, 0, len(tasks)),
		FullTasks:       make([]types.SpecTaskWithProject, 0, len(tasks)),
		TotalTasks:      len(tasks),
		StatusBreakdown: make(map[string]int),
	}

	for _, t := range tasks {
		// Minimal progress info (for stacked bar)
		progress.Tasks = append(progress.Tasks, types.CloneGroupTaskProgress{
			TaskID:      t.ID,
			ProjectID:   t.ProjectID,
			ProjectName: projectNames[t.ProjectID],
			Name:        t.Name,
			Status:      t.Status.String(),
		})

		// Full task with project name (for TaskCard rendering)
		progress.FullTasks = append(progress.FullTasks, types.SpecTaskWithProject{
			SpecTask:    t,
			ProjectName: projectNames[t.ProjectID],
		})

		progress.StatusBreakdown[t.Status.String()]++

		if t.Status == types.TaskStatusDone {
			progress.CompletedTasks++
		}
	}

	if progress.TotalTasks > 0 {
		progress.ProgressPct = (progress.CompletedTasks * 100) / progress.TotalTasks
	}

	return progress, nil
}

// ListReposWithoutProjects returns repositories that don't have an associated project
func (s *PostgresStore) ListReposWithoutProjects(ctx context.Context, organizationID string) ([]*types.GitRepository, error) {
	var repos []*types.GitRepository

	query := s.gdb.WithContext(ctx).
		Model(&types.GitRepository{}).
		Where("project_id = '' OR project_id IS NULL")

	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}

	if err := query.Find(&repos).Error; err != nil {
		return nil, fmt.Errorf("failed to list repos without projects: %w", err)
	}

	return repos, nil
}
