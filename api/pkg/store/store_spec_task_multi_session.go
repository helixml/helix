package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SpecTaskWorkSession CRUD operations

func (s *PostgresStore) CreateSpecTaskWorkSession(ctx context.Context, workSession *types.SpecTaskWorkSession) error {
	if workSession.ID == "" {
		workSession.ID = types.GenerateSpecTaskWorkSessionID()
	}

	// Set timestamps
	now := time.Now()
	workSession.CreatedAt = now
	workSession.UpdatedAt = now

	// Validate required fields
	if workSession.SpecTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}
	if workSession.HelixSessionID == "" {
		return fmt.Errorf("helix_session_id is required")
	}
	if workSession.Phase == "" {
		workSession.Phase = types.SpecTaskPhaseImplementation
	}
	if workSession.Status == "" {
		workSession.Status = types.SpecTaskWorkSessionStatusPending
	}

	err := s.gdb.WithContext(ctx).Create(workSession).Error
	if err != nil {
		return fmt.Errorf("failed to create spec task work session: %w", err)
	}

	log.Info().
		Str("work_session_id", workSession.ID).
		Str("spec_task_id", workSession.SpecTaskID).
		Str("helix_session_id", workSession.HelixSessionID).
		Str("phase", string(workSession.Phase)).
		Msg("Created spec task work session")

	return nil
}

func (s *PostgresStore) GetSpecTaskWorkSession(ctx context.Context, id string) (*types.SpecTaskWorkSession, error) {
	var workSession types.SpecTaskWorkSession

	err := s.gdb.WithContext(ctx).
		Preload("SpecTask").
		Preload("HelixSession").
		Preload("ParentWorkSession").
		Preload("SpawnedBySession").
		Preload("ZedThread").
		Preload("ChildWorkSessions").
		First(&workSession, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("spec task work session not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get spec task work session: %w", err)
	}

	return &workSession, nil
}

func (s *PostgresStore) UpdateSpecTaskWorkSession(ctx context.Context, workSession *types.SpecTaskWorkSession) error {
	workSession.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(workSession).Error
	if err != nil {
		return fmt.Errorf("failed to update spec task work session: %w", err)
	}

	log.Info().
		Str("work_session_id", workSession.ID).
		Str("status", string(workSession.Status)).
		Msg("Updated spec task work session")

	return nil
}

func (s *PostgresStore) DeleteSpecTaskWorkSession(ctx context.Context, id string) error {
	// First, check if there are any dependent ZedThreads
	var zedThreadCount int64
	err := s.gdb.WithContext(ctx).
		Model(&types.SpecTaskZedThread{}).
		Where("work_session_id = ?", id).
		Count(&zedThreadCount).Error
	if err != nil {
		return fmt.Errorf("failed to check zed thread dependencies: %w", err)
	}

	if zedThreadCount > 0 {
		return fmt.Errorf("cannot delete work session with active zed threads")
	}

	// Check for child work sessions
	var childCount int64
	err = s.gdb.WithContext(ctx).
		Model(&types.SpecTaskWorkSession{}).
		Where("parent_work_session_id = ? OR spawned_by_session_id = ?", id, id).
		Count(&childCount).Error
	if err != nil {
		return fmt.Errorf("failed to check child work session dependencies: %w", err)
	}

	if childCount > 0 {
		return fmt.Errorf("cannot delete work session with child sessions")
	}

	err = s.gdb.WithContext(ctx).Delete(&types.SpecTaskWorkSession{}, "id = ?", id).Error
	if err != nil {
		return fmt.Errorf("failed to delete spec task work session: %w", err)
	}

	log.Info().Str("work_session_id", id).Msg("Deleted spec task work session")
	return nil
}

func (s *PostgresStore) ListSpecTaskWorkSessions(ctx context.Context, specTaskID string) ([]*types.SpecTaskWorkSession, error) {
	var workSessions []*types.SpecTaskWorkSession

	err := s.gdb.WithContext(ctx).
		Preload("HelixSession").
		Preload("ZedThread").
		Where("spec_task_id = ?", specTaskID).
		Order("created_at ASC").
		Find(&workSessions).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list spec task work sessions: %w", err)
	}

	return workSessions, nil
}

func (s *PostgresStore) ListWorkSessionsBySpecTask(ctx context.Context, specTaskID string, phase *types.SpecTaskPhase) ([]*types.SpecTaskWorkSession, error) {
	query := s.gdb.WithContext(ctx).
		Preload("HelixSession").
		Preload("ZedThread").
		Where("spec_task_id = ?", specTaskID)

	if phase != nil {
		query = query.Where("phase = ?", *phase)
	}

	var workSessions []*types.SpecTaskWorkSession
	err := query.Order("created_at ASC").Find(&workSessions).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list work sessions by spec task: %w", err)
	}

	return workSessions, nil
}

func (s *PostgresStore) GetSpecTaskWorkSessionByHelixSession(ctx context.Context, helixSessionID string) (*types.SpecTaskWorkSession, error) {
	var workSession types.SpecTaskWorkSession

	err := s.gdb.WithContext(ctx).
		Preload("SpecTask").
		Preload("HelixSession").
		Preload("ParentWorkSession").
		Preload("SpawnedBySession").
		Preload("ZedThread").
		Preload("ChildWorkSessions").
		First(&workSession, "helix_session_id = ?", helixSessionID).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("spec task work session not found for helix session: %s", helixSessionID)
		}
		return nil, fmt.Errorf("failed to get spec task work session by helix session: %w", err)
	}

	return &workSession, nil
}

// SpecTaskZedThread CRUD operations

func (s *PostgresStore) CreateSpecTaskZedThread(ctx context.Context, zedThread *types.SpecTaskZedThread) error {
	if zedThread.ID == "" {
		zedThread.ID = types.GenerateSpecTaskZedThreadID()
	}

	// Set timestamps
	now := time.Now()
	zedThread.CreatedAt = now
	zedThread.UpdatedAt = now

	// Validate required fields
	if zedThread.WorkSessionID == "" {
		return fmt.Errorf("work_session_id is required")
	}
	if zedThread.SpecTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}
	if zedThread.ZedThreadID == "" {
		return fmt.Errorf("zed_thread_id is required")
	}
	if zedThread.Status == "" {
		zedThread.Status = types.SpecTaskZedStatusPending
	}

	err := s.gdb.WithContext(ctx).Create(zedThread).Error
	if err != nil {
		return fmt.Errorf("failed to create spec task zed thread: %w", err)
	}

	log.Info().
		Str("zed_thread_id", zedThread.ID).
		Str("work_session_id", zedThread.WorkSessionID).
		Str("spec_task_id", zedThread.SpecTaskID).
		Str("zed_thread_id_external", zedThread.ZedThreadID).
		Msg("Created spec task zed thread")

	return nil
}

func (s *PostgresStore) GetSpecTaskZedThread(ctx context.Context, id string) (*types.SpecTaskZedThread, error) {
	var zedThread types.SpecTaskZedThread

	err := s.gdb.WithContext(ctx).
		Preload("WorkSession").
		Preload("SpecTask").
		First(&zedThread, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("spec task zed thread not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get spec task zed thread: %w", err)
	}

	return &zedThread, nil
}

func (s *PostgresStore) GetSpecTaskZedThreadByWorkSession(ctx context.Context, workSessionID string) (*types.SpecTaskZedThread, error) {
	var zedThread types.SpecTaskZedThread

	err := s.gdb.WithContext(ctx).
		Preload("WorkSession").
		Preload("SpecTask").
		First(&zedThread, "work_session_id = ?", workSessionID).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("spec task zed thread not found for work session: %s", workSessionID)
		}
		return nil, fmt.Errorf("failed to get spec task zed thread by work session: %w", err)
	}

	return &zedThread, nil
}

func (s *PostgresStore) UpdateSpecTaskZedThread(ctx context.Context, zedThread *types.SpecTaskZedThread) error {
	zedThread.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(zedThread).Error
	if err != nil {
		return fmt.Errorf("failed to update spec task zed thread: %w", err)
	}

	log.Info().
		Str("zed_thread_id", zedThread.ID).
		Str("status", string(zedThread.Status)).
		Msg("Updated spec task zed thread")

	return nil
}

func (s *PostgresStore) DeleteSpecTaskZedThread(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.SpecTaskZedThread{}, "id = ?", id).Error
	if err != nil {
		return fmt.Errorf("failed to delete spec task zed thread: %w", err)
	}

	log.Info().Str("zed_thread_id", id).Msg("Deleted spec task zed thread")
	return nil
}

func (s *PostgresStore) ListSpecTaskZedThreads(ctx context.Context, specTaskID string) ([]*types.SpecTaskZedThread, error) {
	var zedThreads []*types.SpecTaskZedThread

	err := s.gdb.WithContext(ctx).
		Preload("WorkSession").
		Where("spec_task_id = ?", specTaskID).
		Order("created_at ASC").
		Find(&zedThreads).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list spec task zed threads: %w", err)
	}

	return zedThreads, nil
}

// SpecTaskImplementationTask CRUD operations

func (s *PostgresStore) CreateSpecTaskImplementationTask(ctx context.Context, implTask *types.SpecTaskImplementationTask) error {
	if implTask.ID == "" {
		implTask.ID = types.GenerateSpecTaskImplementationTaskID()
	}

	implTask.CreatedAt = time.Now()

	// Validate required fields
	if implTask.SpecTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}
	if implTask.Title == "" {
		return fmt.Errorf("title is required")
	}
	if implTask.Status == "" {
		implTask.Status = types.SpecTaskImplementationStatusPending
	}

	err := s.gdb.WithContext(ctx).Create(implTask).Error
	if err != nil {
		return fmt.Errorf("failed to create spec task implementation task: %w", err)
	}

	log.Info().
		Str("impl_task_id", implTask.ID).
		Str("spec_task_id", implTask.SpecTaskID).
		Str("title", implTask.Title).
		Int("index", implTask.Index).
		Msg("Created spec task implementation task")

	return nil
}

func (s *PostgresStore) GetSpecTaskImplementationTask(ctx context.Context, id string) (*types.SpecTaskImplementationTask, error) {
	var implTask types.SpecTaskImplementationTask

	err := s.gdb.WithContext(ctx).
		Preload("SpecTask").
		Preload("AssignedWorkSession").
		First(&implTask, "id = ?", id).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("spec task implementation task not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get spec task implementation task: %w", err)
	}

	return &implTask, nil
}

func (s *PostgresStore) UpdateSpecTaskImplementationTask(ctx context.Context, implTask *types.SpecTaskImplementationTask) error {
	err := s.gdb.WithContext(ctx).Save(implTask).Error
	if err != nil {
		return fmt.Errorf("failed to update spec task implementation task: %w", err)
	}

	log.Info().
		Str("impl_task_id", implTask.ID).
		Str("status", string(implTask.Status)).
		Msg("Updated spec task implementation task")

	return nil
}

func (s *PostgresStore) DeleteSpecTaskImplementationTask(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.SpecTaskImplementationTask{}, "id = ?", id).Error
	if err != nil {
		return fmt.Errorf("failed to delete spec task implementation task: %w", err)
	}

	log.Info().Str("impl_task_id", id).Msg("Deleted spec task implementation task")
	return nil
}

func (s *PostgresStore) ListSpecTaskImplementationTasks(ctx context.Context, specTaskID string) ([]*types.SpecTaskImplementationTask, error) {
	var implTasks []*types.SpecTaskImplementationTask

	err := s.gdb.WithContext(ctx).
		Preload("AssignedWorkSession").
		Where("spec_task_id = ?", specTaskID).
		Order("index ASC").
		Find(&implTasks).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list spec task implementation tasks: %w", err)
	}

	return implTasks, nil
}

func (s *PostgresStore) ParseAndCreateImplementationTasks(ctx context.Context, specTaskID string, implementationPlan string) ([]*types.SpecTaskImplementationTask, error) {
	// Parse the implementation plan markdown to extract discrete tasks
	tasks := parseImplementationPlan(implementationPlan)

	// Create implementation task records
	var implTasks []*types.SpecTaskImplementationTask

	for i, task := range tasks {
		implTask := &types.SpecTaskImplementationTask{
			SpecTaskID:         specTaskID,
			Title:              task.Title,
			Description:        task.Description,
			AcceptanceCriteria: task.AcceptanceCriteria,
			EstimatedEffort:    task.EstimatedEffort,
			Priority:           task.Priority,
			Index:              i,
			Status:             types.SpecTaskImplementationStatusPending,
		}

		// Convert dependencies to JSON
		if len(task.Dependencies) > 0 {
			depsJSON, err := json.Marshal(task.Dependencies)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to marshal dependencies")
			} else {
				implTask.Dependencies = datatypes.JSON(depsJSON)
			}
		}

		err := s.CreateSpecTaskImplementationTask(ctx, implTask)
		if err != nil {
			return nil, fmt.Errorf("failed to create implementation task %d: %w", i, err)
		}

		implTasks = append(implTasks, implTask)
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Int("task_count", len(implTasks)).
		Msg("Parsed and created implementation tasks")

	return implTasks, nil
}

// Multi-session management operations

func (s *PostgresStore) CreateImplementationSessions(ctx context.Context, specTaskID string, config *types.SpecTaskImplementationSessionsCreateRequest) ([]*types.SpecTaskWorkSession, error) {
	// First, get the spec task
	specTask, err := s.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spec task: %w", err)
	}

	// Get organization ID from the project
	orgID := ""
	if specTask.ProjectID != "" {
		project, err := s.GetProject(ctx, specTask.ProjectID)
		if err != nil {
			log.Warn().Err(err).Str("project_id", specTask.ProjectID).Msg("Failed to get project for org ID")
		} else if project != nil {
			orgID = project.OrganizationID
		}
	}

	// Update spec task with Zed instance configuration
	if config.ProjectPath != "" {
		specTask.ProjectPath = config.ProjectPath
	}
	if config.WorkspaceConfig != nil {
		configJSON, err := json.Marshal(config.WorkspaceConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal workspace config: %w", err)
		}
		specTask.WorkspaceConfig = datatypes.JSON(configJSON)
	}

	err = s.UpdateSpecTask(ctx, specTask)
	if err != nil {
		return nil, fmt.Errorf("failed to update spec task with workspace config: %w", err)
	}

	var workSessions []*types.SpecTaskWorkSession

	if config.AutoCreateSessions {
		// Parse implementation plan and create work sessions
		implTasks, err := s.ParseAndCreateImplementationTasks(ctx, specTaskID, specTask.ImplementationPlan)
		if err != nil {
			return nil, fmt.Errorf("failed to parse implementation tasks: %w", err)
		}

		// Create work sessions for each implementation task
		for _, implTask := range implTasks {
			workSession := &types.SpecTaskWorkSession{
				SpecTaskID:                    specTaskID,
				Name:                          implTask.Title,
				Description:                   implTask.Description,
				Phase:                         types.SpecTaskPhaseImplementation,
				Status:                        types.SpecTaskWorkSessionStatusPending,
				ImplementationTaskTitle:       implTask.Title,
				ImplementationTaskDescription: implTask.Description,
				ImplementationTaskIndex:       implTask.Index,
			}

			// Create corresponding Helix session
			helixSession := &types.Session{
				ID:             system.GenerateSessionID(),
				Name:           fmt.Sprintf("[%s] %s", specTask.Name, implTask.Title),
				Owner:          specTask.CreatedBy,
				OrganizationID: orgID,
				Type:           types.SessionTypeText,
				Mode:           types.SessionModeInference,
				Created:        time.Now(),
				Updated:        time.Now(),
				Metadata: types.SessionMetadata{
					AgentType:               "zed_external", // Same external agent as planning
					SpecTaskID:              specTask.ID,
					SessionRole:             "implementation",
					ImplementationTaskIndex: implTask.Index,
					SystemPrompt:            "", // Will be set when session starts
				},
			}

			_, err = s.CreateSession(ctx, *helixSession)
			if err != nil {
				return nil, fmt.Errorf("failed to create helix session for task %s: %w", implTask.Title, err)
			}

			workSession.HelixSessionID = helixSession.ID

			err = s.CreateSpecTaskWorkSession(ctx, workSession)
			if err != nil {
				return nil, fmt.Errorf("failed to create work session for task %s: %w", implTask.Title, err)
			}

			// Update session metadata with work session ID after creation
			helixSession.Metadata.WorkSessionID = workSession.ID
			_, err = s.UpdateSession(ctx, *helixSession)
			if err != nil {
				log.Warn().Err(err).Str("session_id", helixSession.ID).Msg("Failed to update session metadata with work session ID")
			}

			// Update implementation task assignment
			implTask.AssignedWorkSessionID = workSession.ID
			implTask.Status = types.SpecTaskImplementationStatusAssigned
			err = s.UpdateSpecTaskImplementationTask(ctx, implTask)
			if err != nil {
				log.Warn().Err(err).Str("impl_task_id", implTask.ID).Msg("Failed to update implementation task assignment")
			}

			workSessions = append(workSessions, workSession)
		}
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Int("work_session_count", len(workSessions)).
		Bool("auto_create", config.AutoCreateSessions).
		Msg("Created implementation sessions")

	return workSessions, nil
}

func (s *PostgresStore) SpawnWorkSession(ctx context.Context, parentSessionID string, config *types.SpecTaskWorkSessionSpawnRequest) (*types.SpecTaskWorkSession, error) {
	// Get parent work session
	parentSession, err := s.GetSpecTaskWorkSession(ctx, parentSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent work session: %w", err)
	}

	// Validate that parent can spawn sessions
	if !parentSession.CanSpawnSessions() {
		return nil, fmt.Errorf("parent work session cannot spawn new sessions (status: %s, phase: %s)",
			parentSession.Status, parentSession.Phase)
	}

	// Load the SpecTask to get CreatedBy and ImplementationAgent
	specTask, err := s.GetSpecTask(ctx, parentSession.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Get organization ID from the project
	orgID := ""
	if specTask.ProjectID != "" {
		project, err := s.GetProject(ctx, specTask.ProjectID)
		if err != nil {
			log.Warn().Err(err).Str("project_id", specTask.ProjectID).Msg("Failed to get project for org ID")
		} else if project != nil {
			orgID = project.OrganizationID
		}
	}

	// Create new work session
	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:          parentSession.SpecTaskID,
		Name:                config.Name,
		Description:         config.Description,
		Phase:               types.SpecTaskPhaseImplementation,
		Status:              types.SpecTaskWorkSessionStatusPending,
		ParentWorkSessionID: parentSessionID,
		SpawnedBySessionID:  parentSessionID,
	}

	// Set configuration
	if config.AgentConfig != nil {
		configJSON, err := json.Marshal(config.AgentConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal agent config: %w", err)
		}
		workSession.AgentConfig = datatypes.JSON(configJSON)
	}

	if config.EnvironmentConfig != nil {
		configJSON, err := json.Marshal(config.EnvironmentConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal environment config: %w", err)
		}
		workSession.EnvironmentConfig = datatypes.JSON(configJSON)
	}

	// Create corresponding Helix session
	helixSession := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           fmt.Sprintf("[Spawned] %s", config.Name),
		Owner:          specTask.CreatedBy,
		OrganizationID: orgID,
		Type:           types.SessionTypeText,
		Mode:           types.SessionModeInference,
		Created:        time.Now(),
		Updated:        time.Now(),
		Metadata: types.SessionMetadata{
			AgentType:    "zed_external", // Same external agent as planning
			SpecTaskID:   parentSession.SpecTaskID,
			SessionRole:  "implementation",
			SystemPrompt: "", // Will be set when session starts
		},
	}

	_, err = s.CreateSession(ctx, *helixSession)
	if err != nil {
		return nil, fmt.Errorf("failed to create helix session: %w", err)
	}

	workSession.HelixSessionID = helixSession.ID

	err = s.CreateSpecTaskWorkSession(ctx, workSession)
	if err != nil {
		return nil, fmt.Errorf("failed to create spawned work session: %w", err)
	}

	// Update session metadata with work session ID after creation
	helixSession.Metadata.WorkSessionID = workSession.ID
	_, err = s.UpdateSession(ctx, *helixSession)
	if err != nil {
		log.Warn().Err(err).Str("session_id", helixSession.ID).Msg("Failed to update session metadata with work session ID")
	}

	log.Info().
		Str("work_session_id", workSession.ID).
		Str("parent_session_id", parentSessionID).
		Str("spec_task_id", workSession.SpecTaskID).
		Str("name", config.Name).
		Msg("Spawned new work session")

	return workSession, nil
}

func (s *PostgresStore) GetSpecTaskMultiSessionOverview(ctx context.Context, specTaskID string) (*types.SpecTaskMultiSessionOverviewResponse, error) {
	// Get the main spec task
	specTask, err := s.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spec task: %w", err)
	}

	// Get all work sessions
	workSessions, err := s.ListSpecTaskWorkSessions(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list work sessions: %w", err)
	}

	// Get implementation tasks
	implTasks, err := s.ListSpecTaskImplementationTasks(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list implementation tasks: %w", err)
	}

	// Get Zed threads
	zedThreads, err := s.ListSpecTaskZedThreads(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list zed threads: %w", err)
	}

	// Calculate counts and statistics
	var activeSessions, completedSessions int
	var lastActivity *time.Time

	for _, ws := range workSessions {
		if ws.Status == types.SpecTaskWorkSessionStatusActive {
			activeSessions++
		} else if ws.Status == types.SpecTaskWorkSessionStatusCompleted {
			completedSessions++
		}

		// Find most recent activity
		if lastActivity == nil || ws.UpdatedAt.After(*lastActivity) {
			lastActivity = &ws.UpdatedAt
		}
	}

	// Convert to non-pointer slices for response
	workSessionsSlice := make([]types.SpecTaskWorkSession, len(workSessions))
	for i, ws := range workSessions {
		workSessionsSlice[i] = *ws
	}

	implTasksSlice := make([]types.SpecTaskImplementationTask, len(implTasks))
	for i, it := range implTasks {
		implTasksSlice[i] = *it
	}

	return &types.SpecTaskMultiSessionOverviewResponse{
		SpecTask:            *specTask,
		WorkSessionCount:    len(workSessions),
		ActiveSessions:      activeSessions,
		CompletedSessions:   completedSessions,
		ZedThreadCount:      len(zedThreads),
		ZedInstanceID:       specTask.ZedInstanceID,
		LastActivity:        lastActivity,
		WorkSessions:        workSessionsSlice,
		ImplementationTasks: implTasksSlice,
	}, nil
}

func (s *PostgresStore) GetSpecTaskProgress(ctx context.Context, specTaskID string) (*types.SpecTaskProgressResponse, error) {
	overview, err := s.GetSpecTaskMultiSessionOverview(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get overview: %w", err)
	}

	// Calculate overall progress
	totalTasks := len(overview.ImplementationTasks)
	completedTasks := 0

	implementationProgress := make(map[int]float64)

	for _, task := range overview.ImplementationTasks {
		progress := 0.0
		if task.Status == types.SpecTaskImplementationStatusCompleted {
			progress = 1.0
			completedTasks++
		} else if task.Status == types.SpecTaskImplementationStatusInProgress {
			progress = 0.5
		}
		implementationProgress[task.Index] = progress
	}

	overallProgress := 0.0
	if totalTasks > 0 {
		overallProgress = float64(completedTasks) / float64(totalTasks)
	}

	// Calculate phase progress
	phaseProgress := map[types.SpecTaskPhase]float64{
		types.SpecTaskPhasePlanning: 1.0, // Planning is always complete if we're at this stage
	}

	if totalTasks > 0 {
		phaseProgress[types.SpecTaskPhaseImplementation] = overallProgress
	}

	// Get active work sessions
	var activeWorkSessions []types.SpecTaskWorkSession
	for _, ws := range overview.WorkSessions {
		if ws.Status == types.SpecTaskWorkSessionStatusActive {
			activeWorkSessions = append(activeWorkSessions, ws)
		}
	}

	// TODO: Get recent activity log entries (would need separate activity log table)
	var recentActivity []types.SpecTaskActivityLogEntry

	return &types.SpecTaskProgressResponse{
		SpecTask:               overview.SpecTask,
		OverallProgress:        overallProgress,
		PhaseProgress:          phaseProgress,
		ImplementationProgress: implementationProgress,
		ActiveWorkSessions:     activeWorkSessions,
		RecentActivity:         recentActivity,
	}, nil
}

func (s *PostgresStore) UpdateSpecTaskZedInstance(ctx context.Context, specTaskID string, zedInstanceID string) error {
	err := s.gdb.WithContext(ctx).
		Model(&types.SpecTask{}).
		Where("id = ?", specTaskID).
		Update("zed_instance_id", zedInstanceID).Error

	if err != nil {
		return fmt.Errorf("failed to update spec task zed instance: %w", err)
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("zed_instance_id", zedInstanceID).
		Msg("Updated spec task zed instance")

	return nil
}

// Helper functions

type ParsedImplementationTask struct {
	Title              string
	Description        string
	AcceptanceCriteria string
	EstimatedEffort    string
	Priority           int
	Dependencies       []int
}

func parseImplementationPlan(implementationPlan string) []ParsedImplementationTask {
	var tasks []ParsedImplementationTask

	// Split by tasks - look for numbered lists or headers
	taskRegex := regexp.MustCompile(`(?m)^(?:##\s*(?:Task\s*)?(\d+)|(\d+)\.)\s*(.+)$`)

	lines := strings.Split(implementationPlan, "\n")

	var currentTask *ParsedImplementationTask
	var currentSection string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is a task header
		if matches := taskRegex.FindStringSubmatch(line); len(matches) > 0 {
			// Save previous task
			if currentTask != nil {
				tasks = append(tasks, *currentTask)
			}

			// Start new task
			title := matches[3]
			if title == "" && len(matches) > 1 {
				title = matches[1]
			}

			currentTask = &ParsedImplementationTask{
				Title:           strings.TrimSpace(title),
				EstimatedEffort: "medium",
				Priority:        0,
			}
			currentSection = "description"
			continue
		}

		// Skip if no current task
		if currentTask == nil {
			continue
		}

		// Check for section headers
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "description") || strings.Contains(lowerLine, "overview") {
			currentSection = "description"
			continue
		} else if strings.Contains(lowerLine, "acceptance") || strings.Contains(lowerLine, "criteria") {
			currentSection = "acceptance"
			continue
		} else if strings.Contains(lowerLine, "effort") || strings.Contains(lowerLine, "size") {
			currentSection = "effort"
			continue
		} else if strings.Contains(lowerLine, "dependencies") || strings.Contains(lowerLine, "depends") {
			currentSection = "dependencies"
			continue
		}

		// Add content to current section
		switch currentSection {
		case "description":
			if currentTask.Description == "" {
				currentTask.Description = line
			} else {
				currentTask.Description += " " + line
			}
		case "acceptance":
			if currentTask.AcceptanceCriteria == "" {
				currentTask.AcceptanceCriteria = line
			} else {
				currentTask.AcceptanceCriteria += " " + line
			}
		case "effort":
			effort := strings.ToLower(line)
			if strings.Contains(effort, "small") || strings.Contains(effort, "minor") {
				currentTask.EstimatedEffort = "small"
			} else if strings.Contains(effort, "large") || strings.Contains(effort, "major") {
				currentTask.EstimatedEffort = "large"
			} else {
				currentTask.EstimatedEffort = "medium"
			}
		case "dependencies":
			// Try to extract task numbers from dependencies
			depRegex := regexp.MustCompile(`\d+`)
			depMatches := depRegex.FindAllString(line, -1)
			for _, depStr := range depMatches {
				if depNum, err := strconv.Atoi(depStr); err == nil {
					currentTask.Dependencies = append(currentTask.Dependencies, depNum-1) // Convert to 0-based
				}
			}
		}
	}

	// Save the last task
	if currentTask != nil {
		tasks = append(tasks, *currentTask)
	}

	// If no tasks were parsed, create a single default task
	if len(tasks) == 0 {
		tasks = append(tasks, ParsedImplementationTask{
			Title:           "Implementation",
			Description:     implementationPlan,
			EstimatedEffort: "medium",
			Priority:        0,
		})
	}

	return tasks
}
