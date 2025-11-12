package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// CreateSpecTask creates a new spec-driven task
func (s *PostgresStore) CreateSpecTask(ctx context.Context, task *types.SpecTask) error {
	result := s.gdb.WithContext(ctx).Create(task)
	if result.Error != nil {
		return fmt.Errorf("failed to create spec task: %w", result.Error)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("project_id", task.ProjectID).
		Str("status", task.Status).
		Msg("Created spec task")

	return nil
}

// GetSpecTask retrieves a spec-driven task by ID
func (s *PostgresStore) GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error) {
	task := &types.SpecTask{}

	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("spec task not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get spec task: %w", err)
	}

	return task, nil
}

// UpdateSpecTask updates an existing spec-driven task
func (s *PostgresStore) UpdateSpecTask(ctx context.Context, task *types.SpecTask) error {
	task.UpdatedAt = time.Now()

	result := s.gdb.WithContext(ctx).Save(task)

	if result.Error != nil {
		return fmt.Errorf("failed to update spec task: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("spec task not found: %s", task.ID)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("status", task.Status).
		Str("planning_session_id", task.PlanningSessionID).
		Msg("Updated spec task")

	return nil
}

// ListSpecTasks retrieves spec-driven tasks with optional filtering
func (s *PostgresStore) ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	var tasks []*types.SpecTask

	db := s.gdb.WithContext(ctx)

	// Apply filters using GORM query builder
	if filters != nil {
		if filters.ProjectID != "" {
			db = db.Where("project_id = ?", filters.ProjectID)
		}
		if filters.Status != "" {
			db = db.Where("status = ?", filters.Status)
		}
		if filters.UserID != "" {
			db = db.Where("created_by = ?", filters.UserID)
		}
		if filters.Type != "" {
			db = db.Where("type = ?", filters.Type)
		}
		if filters.Priority != "" {
			db = db.Where("priority = ?", filters.Priority)
		}
		// Archive filtering logic
		if filters.ArchivedOnly {
			db = db.Where("archived = ?", true)
		} else if !filters.IncludeArchived {
			db = db.Where("archived = ? OR archived IS NULL", false)
		}

		if filters.Limit > 0 {
			db = db.Limit(filters.Limit)
		}
		if filters.Offset > 0 {
			db = db.Offset(filters.Offset)
		}
	}

	err := db.Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list spec tasks: %w", err)
	}

	log.Info().
		Int("task_count", len(tasks)).
		Interface("filters", filters).
		Msg("Listed spec tasks")

	return tasks, nil
}
