package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
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

	query := `
		SELECT
			id, project_id, name, description, type, priority, status,
			original_prompt, requirements_spec, technical_design, implementation_plan,
			spec_agent, implementation_agent, spec_session_id, implementation_session_id,
			branch_name, spec_approved_by, spec_approved_at, spec_revision_count,
			estimated_hours, started_at, completed_at,
			created_by, created_at, updated_at, labels, metadata
		FROM spec_tasks
		WHERE id = $1`

	err := s.gdb.WithContext(ctx).Raw(query, id).Scan(&task).Error
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("spec task not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get spec task: %w", err)
	}

	return task, nil
}

// UpdateSpecTask updates an existing spec-driven task
func (s *PostgresStore) UpdateSpecTask(ctx context.Context, task *types.SpecTask) error {
	task.UpdatedAt = time.Now()

	query := `
		UPDATE spec_tasks SET
			project_id = $2, name = $3, description = $4, type = $5, priority = $6, status = $7,
			original_prompt = $8, requirements_spec = $9, technical_design = $10, implementation_plan = $11,
			spec_agent = $12, implementation_agent = $13, spec_session_id = $14, implementation_session_id = $15,
			branch_name = $16, spec_approved_by = $17, spec_approved_at = $18, spec_revision_count = $19,
			estimated_hours = $20, started_at = $21, completed_at = $22,
			created_by = $23, updated_at = $24, labels = $25, metadata = $26
		WHERE id = $1`

	result := s.gdb.WithContext(ctx).Exec(query,
		task.ID, task.ProjectID, task.Name, task.Description, task.Type, task.Priority, task.Status,
		task.OriginalPrompt, task.RequirementsSpec, task.TechnicalDesign, task.ImplementationPlan,
		task.SpecAgent, task.ImplementationAgent, task.SpecSessionID, task.ImplementationSessionID,
		task.BranchName, task.SpecApprovedBy, task.SpecApprovedAt, task.SpecRevisionCount,
		task.EstimatedHours, task.StartedAt, task.CompletedAt,
		task.CreatedBy, task.UpdatedAt, task.LabelsDB, task.Metadata,
	)

	if result.Error != nil {
		return fmt.Errorf("failed to update spec task: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("spec task not found: %s", task.ID)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("status", task.Status).
		Msg("Updated spec task")

	return nil
}

// ListSpecTasks retrieves spec-driven tasks with optional filtering
func (s *PostgresStore) ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	var tasks []*types.SpecTask

	// Build the base query
	query := `
		SELECT
			id, project_id, name, description, type, priority, status,
			original_prompt, requirements_spec, technical_design, implementation_plan,
			spec_agent, implementation_agent, spec_session_id, implementation_session_id,
			branch_name, spec_approved_by, spec_approved_at, spec_revision_count,
			estimated_hours, started_at, completed_at,
			created_by, created_at, updated_at, labels, metadata
		FROM spec_tasks`

	// Build WHERE conditions
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filters != nil {
		if filters.ProjectID != "" {
			conditions = append(conditions, fmt.Sprintf("project_id = $%d", argIndex))
			args = append(args, filters.ProjectID)
			argIndex++
		}
		if filters.Status != "" {
			conditions = append(conditions, fmt.Sprintf("status = $%d", argIndex))
			args = append(args, filters.Status)
			argIndex++
		}
		if filters.UserID != "" {
			conditions = append(conditions, fmt.Sprintf("created_by = $%d", argIndex))
			args = append(args, filters.UserID)
			argIndex++
		}
		if filters.Type != "" {
			conditions = append(conditions, fmt.Sprintf("type = $%d", argIndex))
			args = append(args, filters.Type)
			argIndex++
		}
		if filters.Priority != "" {
			conditions = append(conditions, fmt.Sprintf("priority = $%d", argIndex))
			args = append(args, filters.Priority)
			argIndex++
		}
	}

	// Add WHERE clause if we have conditions
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			query += " AND " + conditions[i]
		}
	}

	// Add ORDER BY
	query += " ORDER BY created_at DESC"

	// Add LIMIT and OFFSET if specified
	if filters != nil {
		if filters.Limit > 0 {
			query += fmt.Sprintf(" LIMIT $%d", argIndex)
			args = append(args, filters.Limit)
			argIndex++
		}
		if filters.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argIndex)
			args = append(args, filters.Offset)
		}
	}

	err := s.gdb.WithContext(ctx).Raw(query, args...).Scan(&tasks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list spec tasks: %w", err)
	}

	log.Info().
		Int("task_count", len(tasks)).
		Interface("filters", filters).
		Msg("Listed spec tasks")

	return tasks, nil
}
