package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func newTaskSpecSubject(projectID string) string {
	return fmt.Sprintf("spec-task.%s", projectID)
}

// CreateSpecTask creates a new spec-driven task
func (s *PostgresStore) CreateSpecTask(ctx context.Context, task *types.SpecTask) error {
	if task.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	if task.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Omit("DependsOn").Create(task)
		if result.Error != nil {
			return result.Error
		}

		if err := syncSpecTaskDependsOn(ctx, tx, task); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create spec task: %w", err)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("project_id", task.ProjectID).
		Str("status", task.Status.String()).
		Msg("Created spec task")

	_ = s.notifyTaskUpdates(ctx, task)

	return nil
}

func (s *PostgresStore) GetSpecTasksCount(ctx context.Context, query *GetSpecTasksCountQuery) (int64, error) {
	if query.UserID == "" && query.OrganizationID == "" {
		return 0, fmt.Errorf("user ID or organization ID is required")
	}

	q := s.gdb.WithContext(ctx).Model(&types.SpecTask{})

	if query.OrganizationID != "" {
		q = q.Where("organization_id = ?", query.OrganizationID)
	} else {
		q = q.Where("user_id = ?", query.UserID)
	}

	if !query.IncludeArchived {
		q = q.Where("archived = ?", false)
	}

	if !query.IncludeDone {
		q = q.Where("status != ?", types.TaskStatusDone)
	}

	var count int64

	err := q.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("error getting spec tasks count: %w", err)
	}
	return count, nil
}

// GetSpecTask retrieves a spec-driven task by ID
func (s *PostgresStore) GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error) {
	if id == "" {
		return nil, fmt.Errorf("task ID is required")
	}

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
	if task.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	task.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Omit("DependsOn").Save(task)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("spec task not found: %s", task.ID)
		}

		if err := syncSpecTaskDependsOn(ctx, tx, task); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update spec task: %w", err)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("status", task.Status.String()).
		Str("planning_session_id", task.PlanningSessionID).
		Msg("Updated spec task")

	s.notifyTaskUpdates(ctx, task)

	return nil
}

func syncSpecTaskDependsOn(ctx context.Context, tx *gorm.DB, task *types.SpecTask) error {
	if task.DependsOn == nil {
		return nil
	}

	dependencyIDs := extractSpecTaskDependencyIDs(task.DependsOn)
	for _, dependencyID := range dependencyIDs {
		if dependencyID == task.ID {
			return fmt.Errorf("task cannot depend on itself")
		}
	}

	if len(dependencyIDs) == 0 {
		if err := tx.WithContext(ctx).Model(task).Association("DependsOn").Clear(); err != nil {
			return fmt.Errorf("failed to clear spec task dependencies: %w", err)
		}
		return nil
	}

	var dependencies []types.SpecTask
	err := tx.WithContext(ctx).Where("id IN ?", dependencyIDs).Find(&dependencies).Error
	if err != nil {
		return fmt.Errorf("failed to load spec task dependencies: %w", err)
	}
	if len(dependencies) != len(dependencyIDs) {
		return fmt.Errorf("depends on task not found")
	}

	for _, dependency := range dependencies {
		if dependency.ProjectID != task.ProjectID {
			return fmt.Errorf("depends on task must be in same project")
		}
	}

	if err := validateSpecTaskDependencyGraph(ctx, tx, task.ProjectID, task.ID, dependencyIDs); err != nil {
		return err
	}

	args := make([]interface{}, 0, len(dependencies))
	for i := range dependencies {
		args = append(args, &dependencies[i])
	}

	if err := tx.WithContext(ctx).Model(task).Association("DependsOn").Replace(args...); err != nil {
		return fmt.Errorf("failed to sync spec task dependencies: %w", err)
	}

	return nil
}

type specTaskDependencyEdge struct {
	SpecTaskID  string
	DependsOnID string
}

func validateSpecTaskDependencyGraph(ctx context.Context, tx *gorm.DB, projectID, taskID string, dependencyIDs []string) error {
	var edges []specTaskDependencyEdge
	if err := tx.WithContext(ctx).Raw(`
		SELECT std.spec_task_id, std.depends_on_id
		FROM spec_task_dependencies std
		JOIN spec_tasks owner ON owner.id = std.spec_task_id
		JOIN spec_tasks dependency ON dependency.id = std.depends_on_id
		WHERE owner.project_id = ? AND dependency.project_id = ?
	`, projectID, projectID).Scan(&edges).Error; err != nil {
		return fmt.Errorf("failed to load spec task dependency graph: %w", err)
	}

	graph := make(map[string][]string, len(edges)+1)
	for _, edge := range edges {
		graph[edge.SpecTaskID] = append(graph[edge.SpecTaskID], edge.DependsOnID)
	}
	graph[taskID] = dependencyIDs

	for _, dependencyID := range dependencyIDs {
		if hasSpecTaskDependencyPath(graph, dependencyID, taskID) {
			return fmt.Errorf("circular dependency detected")
		}
	}

	return nil
}

func hasSpecTaskDependencyPath(graph map[string][]string, fromID, toID string) bool {
	if fromID == toID {
		return true
	}

	visited := map[string]struct{}{fromID: {}}
	stack := append([]string(nil), graph[fromID]...)

	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if current == toID {
			return true
		}
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		stack = append(stack, graph[current]...)
	}

	return false
}

func extractSpecTaskDependencyIDs(dependsOn []types.SpecTask) []string {
	if len(dependsOn) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(dependsOn))
	dependencyIDs := make([]string, 0, len(dependsOn))
	for _, dependency := range dependsOn {
		if dependency.ID == "" {
			continue
		}
		if _, exists := seen[dependency.ID]; exists {
			continue
		}
		seen[dependency.ID] = struct{}{}
		dependencyIDs = append(dependencyIDs, dependency.ID)
	}

	return dependencyIDs
}

func (s *PostgresStore) DeleteSpecTask(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("task ID is required")
	}

	// Clean up junction table entries where this task is either the owner or a dependency
	if err := s.gdb.WithContext(ctx).Exec(
		"DELETE FROM spec_task_dependencies WHERE spec_task_id = ? OR depends_on_id = ?", id, id,
	).Error; err != nil {
		return fmt.Errorf("failed to delete spec task dependencies: %w", err)
	}

	result := s.gdb.WithContext(ctx).Delete(&types.SpecTask{}, "id = ?", id)
	if result.Error != nil {
		return fmt.Errorf("failed to delete spec task: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("spec task not found: %s", id)
	}

	log.Info().
		Str("task_id", id).
		Msg("Deleted spec task")

	return nil
}

// ListSpecTasks retrieves spec-driven tasks with optional filtering
func (s *PostgresStore) ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	var tasks []*types.SpecTask

	db := s.gdb.WithContext(ctx)

	// Apply filters using GORM query builder

	if filters.WithDependsOn {
		db = db.Preload("DependsOn")
	}
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
	// DesignDocPath filter - used for matching pushed design doc directories to tasks
	if filters.DesignDocPath != "" {
		db = db.Where("design_doc_path = ?", filters.DesignDocPath)
	}
	// BranchName filter - used for uniqueness check across projects
	if filters.BranchName != "" {
		db = db.Where("branch_name = ?", filters.BranchName)
	}

	if filters.Limit > 0 {
		db = db.Limit(filters.Limit)
	}
	if filters.Offset > 0 {
		db = db.Offset(filters.Offset)
	}

	err := db.Order("created_at DESC").Find(&tasks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list spec tasks: %w", err)
	}

	return tasks, nil
}

// SubscribeForTasks subscribes to task updates with optional filtering by status and project.
// Returns a subscription that receives task updates matching the filter criteria.
// Supports multiple concurrent subscribers in a broadcast style.
func (s *PostgresStore) SubscribeForTasks(ctx context.Context, filter *SpecTaskSubscriptionFilter, handler func(task *types.SpecTask) error) (pubsub.Subscription, error) {
	if filter.ProjectID == "" {
		filter.ProjectID = "*"
	}

	subject := newTaskSpecSubject(filter.ProjectID)

	sub, err := s.pubsub.Subscribe(ctx, subject, func(payload []byte) error {
		var task types.SpecTask

		err := json.Unmarshal(payload, &task)
		if err != nil {
			return fmt.Errorf("failed to unmarshal task: %w", err)
		}
		if !filter.Matches(&task) {
			return nil
		}

		return handler(&task)
	})

	return sub, err
}

func (s *PostgresStore) notifyTaskUpdates(ctx context.Context, task *types.SpecTask) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	message, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}

	return s.pubsub.Publish(ctx, newTaskSpecSubject(task.ProjectID), message)
}

type SpecTaskSubscriptionFilter struct {
	Statuses  []types.SpecTaskStatus
	ProjectID string
}

func (f *SpecTaskSubscriptionFilter) Matches(task *types.SpecTask) bool {
	if f == nil {
		return true
	}

	if len(f.Statuses) > 0 {
		for _, status := range f.Statuses {
			if task.Status == status {
				return true
			}
		}
		return false
	}

	return true
}

type SpecTaskSubscription interface {
	Unsubscribe()
}
