package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SpecTaskMultiSessionManager manages multi-session workflows for SpecTasks at the infrastructure level
// This service coordinates between Helix sessions and Zed instances/threads, but does not manage
// sessions through agent tools - that happens at the infrastructure level
type SpecTaskMultiSessionManager struct {
	store                    store.Store
	controller               *controller.Controller
	specDrivenTaskService    *SpecDrivenTaskService
	zedIntegrationService    *ZedIntegrationService
	sessionContextService    *SessionContextService
	defaultImplementationApp string // Default Zed app for implementation
	testMode                 bool   // If true, skip async operations
}

// NewSpecTaskMultiSessionManager creates a new multi-session manager
func NewSpecTaskMultiSessionManager(
	store store.Store,
	controller *controller.Controller,
	specDrivenTaskService *SpecDrivenTaskService,
	zedIntegrationService *ZedIntegrationService,
	defaultImplementationApp string,
) *SpecTaskMultiSessionManager {
	return &SpecTaskMultiSessionManager{
		store:                    store,
		controller:               controller,
		specDrivenTaskService:    specDrivenTaskService,
		zedIntegrationService:    zedIntegrationService,
		sessionContextService:    NewSessionContextService(store),
		defaultImplementationApp: defaultImplementationApp,
		testMode:                 false,
	}
}

// SetTestMode enables or disables test mode
func (m *SpecTaskMultiSessionManager) SetTestMode(enabled bool) {
	m.testMode = enabled
	if m.zedIntegrationService != nil {
		m.zedIntegrationService.SetTestMode(enabled)
	}
	if m.sessionContextService != nil {
		m.sessionContextService.SetTestMode(enabled)
	}
}

// CreateImplementationSessions creates work sessions from an approved SpecTask
func (m *SpecTaskMultiSessionManager) CreateImplementationSessions(
	ctx context.Context,
	specTaskID string,
	config *types.SpecTaskImplementationSessionsCreateRequest,
) (*types.SpecTaskMultiSessionOverviewResponse, error) {
	// Get and validate spec task
	specTask, err := m.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spec task: %w", err)
	}

	if specTask.Status != types.TaskStatusSpecApproved {
		return nil, fmt.Errorf("spec task must be approved before creating implementation sessions (current status: %s)", specTask.Status)
	}

	if specTask.ImplementationPlan == "" {
		return nil, fmt.Errorf("spec task has no implementation plan")
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("project_path", config.ProjectPath).
		Bool("auto_create", config.AutoCreateSessions).
		Msg("Creating implementation sessions for spec task")

	// Create Zed instance if needed
	zedInstanceID := ""
	if specTask.ImplementationAgent != "" && m.zedIntegrationService != nil {
		// Check if this is a Zed-based implementation agent
		app, err := m.store.GetApp(ctx, specTask.ImplementationAgent)
		if err != nil {
			log.Warn().Err(err).Str("app_id", specTask.ImplementationAgent).Msg("Failed to get implementation app")
		} else if m.isZedBasedApp(app) {
			zedInstanceID, err = m.zedIntegrationService.CreateZedInstanceForSpecTask(ctx, specTask, config.WorkspaceConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create Zed instance: %w", err)
			}
		}
	}

	// Create implementation sessions via store
	workSessions, err := m.store.CreateImplementationSessions(ctx, specTaskID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create implementation sessions: %w", err)
	}

	// Create Zed threads for work sessions if we have a Zed instance
	if zedInstanceID != "" && m.zedIntegrationService != nil {
		for _, workSession := range workSessions {
			_, err = m.zedIntegrationService.CreateZedThreadForWorkSession(ctx, workSession, zedInstanceID)
			if err != nil {
				log.Error().Err(err).
					Str("work_session_id", workSession.ID).
					Str("zed_instance_id", zedInstanceID).
					Msg("Failed to create Zed thread for work session")
				// Continue with other sessions rather than failing completely
			}
		}
	}

	// Update spec task status
	specTask.Status = types.TaskStatusImplementation
	if zedInstanceID != "" {
		specTask.ZedInstanceID = zedInstanceID
	}
	specTask.UpdatedAt = time.Now()

	err = m.store.UpdateSpecTask(ctx, specTask)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to update spec task status")
	}

	// Register all work sessions in context service
	for _, workSession := range workSessions {
		m.sessionContextService.OnWorkSessionCreated(ctx, workSession)
	}

	// Start implementation sessions (unless in test mode)
	if !m.testMode {
		go m.startImplementationSessions(context.Background(), specTask, workSessions)
	}

	// Return overview
	return m.store.GetSpecTaskMultiSessionOverview(ctx, specTaskID)
}

// SpawnWorkSession creates a new work session spawned from an existing one
func (m *SpecTaskMultiSessionManager) SpawnWorkSession(
	ctx context.Context,
	parentSessionID string,
	config *types.SpecTaskWorkSessionSpawnRequest,
) (*types.SpecTaskWorkSessionDetailResponse, error) {
	log.Info().
		Str("parent_session_id", parentSessionID).
		Str("name", config.Name).
		Msg("Spawning new work session")

	// Create spawned work session via store
	workSession, err := m.store.SpawnWorkSession(ctx, parentSessionID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn work session: %w", err)
	}

	// Get the spec task to check for Zed instance
	specTask, err := m.store.GetSpecTask(ctx, workSession.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spec task: %w", err)
	}

	// Create Zed thread if spec task has Zed instance
	if specTask.ZedInstanceID != "" && m.zedIntegrationService != nil {
		_, err = m.zedIntegrationService.CreateZedThreadForWorkSession(ctx, workSession, specTask.ZedInstanceID)
		if err != nil {
			log.Error().Err(err).
				Str("work_session_id", workSession.ID).
				Str("zed_instance_id", specTask.ZedInstanceID).
				Msg("Failed to create Zed thread for spawned work session")
		}
	}

	// Register spawned work session in context service
	m.sessionContextService.OnWorkSessionCreated(ctx, workSession)
	m.sessionContextService.OnWorkSessionSpawned(ctx, parentSessionID, workSession.ID, config.Description)

	// Start the new work session (unless in test mode)
	if !m.testMode {
		go m.startWorkSession(context.Background(), workSession, specTask)
	}

	// Return detailed response
	return m.getWorkSessionDetail(ctx, workSession.ID)
}

// GetMultiSessionOverview returns overview of all sessions for a SpecTask
func (m *SpecTaskMultiSessionManager) GetMultiSessionOverview(
	ctx context.Context,
	specTaskID string,
) (*types.SpecTaskMultiSessionOverviewResponse, error) {
	return m.store.GetSpecTaskMultiSessionOverview(ctx, specTaskID)
}

// GetSpecTaskProgress returns detailed progress information
func (m *SpecTaskMultiSessionManager) GetSpecTaskProgress(
	ctx context.Context,
	specTaskID string,
) (*types.SpecTaskProgressResponse, error) {
	return m.store.GetSpecTaskProgress(ctx, specTaskID)
}

// GetWorkSessionDetail returns detailed information about a work session
func (m *SpecTaskMultiSessionManager) GetWorkSessionDetail(
	ctx context.Context,
	workSessionID string,
) (*types.SpecTaskWorkSessionDetailResponse, error) {
	return m.getWorkSessionDetail(ctx, workSessionID)
}

// UpdateWorkSessionStatus updates the status of a work session and handles state transitions
func (m *SpecTaskMultiSessionManager) UpdateWorkSessionStatus(
	ctx context.Context,
	workSessionID string,
	status types.SpecTaskWorkSessionStatus,
) error {
	workSession, err := m.store.GetSpecTaskWorkSession(ctx, workSessionID)
	if err != nil {
		return fmt.Errorf("failed to get work session: %w", err)
	}

	oldStatus := workSession.Status
	workSession.Status = status

	// Set completion time if completing
	if status == types.SpecTaskWorkSessionStatusCompleted && oldStatus != types.SpecTaskWorkSessionStatusCompleted {
		now := time.Now()
		workSession.CompletedAt = &now
	}

	// Set start time if starting
	if status == types.SpecTaskWorkSessionStatusActive && oldStatus == types.SpecTaskWorkSessionStatusPending {
		now := time.Now()
		workSession.StartedAt = &now
	}

	err = m.store.UpdateSpecTaskWorkSession(ctx, workSession)
	if err != nil {
		return fmt.Errorf("failed to update work session status: %w", err)
	}

	// Update context service with status change
	m.sessionContextService.OnWorkSessionStatusChanged(ctx, workSessionID, status, 0.0)

	// Notify completion if applicable
	if status == types.SpecTaskWorkSessionStatusCompleted {
		m.sessionContextService.OnWorkSessionCompleted(ctx, workSessionID, "Work session completed", "")
	}

	log.Info().
		Str("work_session_id", workSessionID).
		Str("old_status", string(oldStatus)).
		Str("new_status", string(status)).
		Msg("Updated work session status")

	// Check if we need to update overall SpecTask status
	if status == types.SpecTaskWorkSessionStatusCompleted {
		go m.checkSpecTaskCompletion(context.Background(), workSession.SpecTaskID)
	}

	return nil
}

// UpdateZedThreadStatus updates the status of a Zed thread
func (m *SpecTaskMultiSessionManager) UpdateZedThreadStatus(
	ctx context.Context,
	workSessionID string,
	status types.SpecTaskZedStatus,
) error {
	zedThread, err := m.store.GetSpecTaskZedThreadByWorkSession(ctx, workSessionID)
	if err != nil {
		return fmt.Errorf("failed to get zed thread: %w", err)
	}

	oldStatus := zedThread.Status
	zedThread.Status = status

	// Update activity timestamp
	now := time.Now()
	zedThread.LastActivityAt = &now

	err = m.store.UpdateSpecTaskZedThread(ctx, zedThread)
	if err != nil {
		return fmt.Errorf("failed to update zed thread status: %w", err)
	}

	log.Info().
		Str("zed_thread_id", zedThread.ID).
		Str("work_session_id", workSessionID).
		Str("old_status", string(oldStatus)).
		Str("new_status", string(status)).
		Msg("Updated zed thread status")

	// Update corresponding work session status if needed
	if status == types.SpecTaskZedStatusActive {
		err = m.UpdateWorkSessionStatus(ctx, workSessionID, types.SpecTaskWorkSessionStatusActive)
		if err != nil {
			log.Error().Err(err).Str("work_session_id", workSessionID).Msg("Failed to update work session status")
		}
	} else if status == types.SpecTaskZedStatusCompleted {
		err = m.UpdateWorkSessionStatus(ctx, workSessionID, types.SpecTaskWorkSessionStatusCompleted)
		if err != nil {
			log.Error().Err(err).Str("work_session_id", workSessionID).Msg("Failed to update work session status")
		}
	}

	return nil
}

// Private helper methods

// createZedInstance is now delegated to ZedIntegrationService
// This method is kept for backward compatibility but delegates to the service
func (m *SpecTaskMultiSessionManager) createZedInstance(
	ctx context.Context,
	specTask *types.SpecTask,
	config *types.SpecTaskImplementationSessionsCreateRequest,
) (string, error) {
	return m.zedIntegrationService.CreateZedInstanceForSpecTask(ctx, specTask, config.WorkspaceConfig)
}

// createZedThreadForWorkSession is now delegated to ZedIntegrationService
// This method is kept for backward compatibility but delegates to the service
func (m *SpecTaskMultiSessionManager) createZedThreadForWorkSession(
	ctx context.Context,
	workSession *types.SpecTaskWorkSession,
	zedInstanceID string,
) error {
	_, err := m.zedIntegrationService.CreateZedThreadForWorkSession(ctx, workSession, zedInstanceID)
	return err
}

func (m *SpecTaskMultiSessionManager) startImplementationSessions(
	ctx context.Context,
	specTask *types.SpecTask,
	workSessions []*types.SpecTaskWorkSession,
) {
	log.Info().
		Str("spec_task_id", specTask.ID).
		Int("session_count", len(workSessions)).
		Msg("Starting implementation sessions")

	for _, workSession := range workSessions {
		err := m.startWorkSession(ctx, workSession, specTask)
		if err != nil {
			log.Error().Err(err).
				Str("work_session_id", workSession.ID).
				Msg("Failed to start work session")
		}
	}
}

func (m *SpecTaskMultiSessionManager) startWorkSession(
	ctx context.Context,
	workSession *types.SpecTaskWorkSession,
	specTask *types.SpecTask,
) error {
	// Build system prompt for implementation session
	systemPrompt := m.buildImplementationSessionPrompt(specTask, workSession)

	// Update session with implementation context
	session, err := m.store.GetSession(ctx, workSession.HelixSessionID)
	if err != nil {
		return fmt.Errorf("failed to get helix session: %w", err)
	}

	// Update session metadata
	session.Metadata.SystemPrompt = systemPrompt
	session.Metadata.AgentType = specTask.ImplementationAgent

	_, err = m.store.UpdateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to update helix session: %w", err)
	}

	// Update work session status to active
	err = m.UpdateWorkSessionStatus(ctx, workSession.ID, types.SpecTaskWorkSessionStatusActive)
	if err != nil {
		return fmt.Errorf("failed to update work session status: %w", err)
	}

	// Update session metadata with work session context
	session.Metadata.WorkSessionID = workSession.ID
	session.Metadata.ImplementationTaskIndex = workSession.ImplementationTaskIndex

	_, err = m.store.UpdateSession(ctx, *session)
	if err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to update session metadata with work session context")
	}

	// Register session as started in context service
	m.sessionContextService.OnWorkSessionStatusChanged(ctx, workSession.ID, types.SpecTaskWorkSessionStatusActive, 0.0)

	log.Info().
		Str("work_session_id", workSession.ID).
		Str("helix_session_id", workSession.HelixSessionID).
		Str("spec_task_id", workSession.SpecTaskID).
		Msg("Started implementation work session")

	return nil
}

func (m *SpecTaskMultiSessionManager) buildImplementationSessionPrompt(
	specTask *types.SpecTask,
	workSession *types.SpecTaskWorkSession,
) string {
	prompt := fmt.Sprintf(`You are a senior software engineer implementing a specific task as part of a larger project.

**Project Context:**
- Project: %s
- Overall Description: %s

**Your Specific Task:**
- Task: %s
- Description: %s

**Approved Specifications:**

## Requirements
%s

## Technical Design
%s

## Implementation Plan Context
%s

**Your Role:**
You are responsible for implementing the specific task listed above. You have access to the full project context and approved specifications. Focus on:

1. Following the technical design and specifications exactly
2. Implementing clean, tested, production-ready code
3. Coordinating with other implementation sessions when needed
4. Using the LoopInHuman skill when you need clarification or help
5. Using the JobCompleted skill when your specific task is done

Remember: You are part of a larger implementation effort. Other agents may be working on related tasks in parallel.`,
		specTask.Name,
		specTask.Description,
		workSession.ImplementationTaskTitle,
		workSession.ImplementationTaskDescription,
		specTask.RequirementsSpec,
		specTask.TechnicalDesign,
		specTask.ImplementationPlan)

	return prompt
}

func (m *SpecTaskMultiSessionManager) checkSpecTaskCompletion(ctx context.Context, specTaskID string) {
	// Get all work sessions for the spec task
	workSessions, err := m.store.ListSpecTaskWorkSessions(ctx, specTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to get work sessions for completion check")
		return
	}

	// Check if all implementation sessions are complete
	var implementationSessions []*types.SpecTaskWorkSession
	var completedCount int

	for _, ws := range workSessions {
		if ws.Phase == types.SpecTaskPhaseImplementation {
			implementationSessions = append(implementationSessions, ws)
			if ws.Status == types.SpecTaskWorkSessionStatusCompleted {
				completedCount++
			}
		}
	}

	// If all implementation sessions are complete, mark the spec task as done
	if len(implementationSessions) > 0 && completedCount == len(implementationSessions) {
		specTask, err := m.store.GetSpecTask(ctx, specTaskID)
		if err != nil {
			log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to get spec task for completion")
			return
		}

		specTask.Status = types.TaskStatusDone
		now := time.Now()
		specTask.CompletedAt = &now
		specTask.UpdatedAt = now

		err = m.store.UpdateSpecTask(ctx, specTask)
		if err != nil {
			log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to mark spec task as complete")
			return
		}

		log.Info().
			Str("spec_task_id", specTaskID).
			Int("completed_sessions", completedCount).
			Int("total_sessions", len(implementationSessions)).
			Msg("Marked spec task as complete")
	}
}

func (m *SpecTaskMultiSessionManager) getWorkSessionDetail(
	ctx context.Context,
	workSessionID string,
) (*types.SpecTaskWorkSessionDetailResponse, error) {
	workSession, err := m.store.GetSpecTaskWorkSession(ctx, workSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work session: %w", err)
	}

	specTask, err := m.store.GetSpecTask(ctx, workSession.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spec task: %w", err)
	}

	helixSession, err := m.store.GetSession(ctx, workSession.HelixSessionID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", workSession.HelixSessionID).Msg("Failed to get helix session")
	}

	var zedThread *types.SpecTaskZedThread
	if zt, err := m.store.GetSpecTaskZedThreadByWorkSession(ctx, workSessionID); err == nil {
		zedThread = zt
	}

	// Get implementation task if linked
	var implTask *types.SpecTaskImplementationTask
	if workSession.ImplementationTaskIndex >= 0 {
		implTasks, err := m.store.ListSpecTaskImplementationTasks(ctx, workSession.SpecTaskID)
		if err == nil {
			for _, it := range implTasks {
				if it.Index == workSession.ImplementationTaskIndex {
					implTask = it
					break
				}
			}
		}
	}

	// Get child work sessions
	childSessions, err := m.store.ListSpecTaskWorkSessions(ctx, workSession.SpecTaskID)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get child sessions")
	}

	// Filter to only children of this session
	var children []types.SpecTaskWorkSession
	for _, child := range childSessions {
		if child.ParentWorkSessionID == workSessionID || child.SpawnedBySessionID == workSessionID {
			children = append(children, *child)
		}
	}

	return &types.SpecTaskWorkSessionDetailResponse{
		WorkSession:        *workSession,
		SpecTask:           *specTask,
		HelixSession:       helixSession,
		ZedThread:          zedThread,
		ImplementationTask: implTask,
		ChildWorkSessions:  children,
	}, nil
}

func (m *SpecTaskMultiSessionManager) isZedBasedApp(app *types.App) bool {
	// Check if the app is configured for Zed integration
	// This could be based on app metadata, agent type, or other configuration
	if app == nil || len(app.Config.Helix.Assistants) == 0 {
		return false
	}

	// Check if any assistant is configured for Zed
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.Model == "zed" || assistant.Name == "zed" {
			return true
		}
	}

	// Also check app name/description for Zed references
	return strings.Contains(strings.ToLower(app.Config.Helix.Name), "zed") ||
		strings.Contains(strings.ToLower(app.Config.Helix.Description), "zed")
}
