package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ZedToHelixSessionService handles creation of Helix sessions when Zed threads are created
// This enables the reverse flow: Zed thread creation → Helix session creation
type ZedToHelixSessionService struct {
	store                 store.Store
	sessionContextService *SessionContextService
	testMode              bool
}

// ZedThreadCreationContext represents context for creating a session from Zed thread
type ZedThreadCreationContext struct {
	ZedInstanceID             string                 `json:"zed_instance_id"`
	ZedThreadID               string                 `json:"zed_thread_id"`
	SpecTaskID                string                 `json:"spec_task_id"`
	ParentZedThreadID         string                 `json:"parent_zed_thread_id,omitempty"`
	ThreadName                string                 `json:"thread_name"`
	ThreadDescription         string                 `json:"thread_description,omitempty"`
	UserID                    string                 `json:"user_id"`
	ProjectPath               string                 `json:"project_path,omitempty"`
	SpawnReason               string                 `json:"spawn_reason,omitempty"`
	AgentConfiguration        map[string]interface{} `json:"agent_configuration,omitempty"`
	EnvironmentVariables      []string               `json:"environment_variables,omitempty"`
	InitialPrompt             string                 `json:"initial_prompt,omitempty"`
	ExpectedWorkType          string                 `json:"expected_work_type,omitempty"`
	EstimatedDurationHours    float64                `json:"estimated_duration_hours,omitempty"`
	RelatedImplementationTask int                    `json:"related_implementation_task,omitempty"`
}

// ZedSessionCreationResult represents the result of creating a Helix session from Zed thread
type ZedSessionCreationResult struct {
	WorkSession       *types.SpecTaskWorkSession `json:"work_session"`
	HelixSession      *types.Session             `json:"helix_session"`
	ZedThread         *types.SpecTaskZedThread   `json:"zed_thread"`
	SpecTask          *types.SpecTask            `json:"spec_task"`
	CreationMethod    string                     `json:"creation_method"` // "spawned", "planned", "ad_hoc"
	ParentWorkSession *types.SpecTaskWorkSession `json:"parent_work_session,omitempty"`
	Success           bool                       `json:"success"`
	Message           string                     `json:"message"`
	Warnings          []string                   `json:"warnings,omitempty"`
}

// NewZedToHelixSessionService creates a new Zed-to-Helix session service
func NewZedToHelixSessionService(
	store store.Store,
	sessionContextService *SessionContextService,
) *ZedToHelixSessionService {
	return &ZedToHelixSessionService{
		store:                 store,
		sessionContextService: sessionContextService,
		testMode:              false,
	}
}

// SetTestMode enables or disables test mode
func (s *ZedToHelixSessionService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// CreateHelixSessionFromZedThread creates a new Helix session when a Zed thread is created
func (s *ZedToHelixSessionService) CreateHelixSessionFromZedThread(
	ctx context.Context,
	creationContext *ZedThreadCreationContext,
) (*ZedSessionCreationResult, error) {
	log.Info().
		Str("zed_instance_id", creationContext.ZedInstanceID).
		Str("zed_thread_id", creationContext.ZedThreadID).
		Str("spec_task_id", creationContext.SpecTaskID).
		Str("thread_name", creationContext.ThreadName).
		Msg("Creating Helix session from Zed thread")

	// Validate creation context
	if err := s.validateCreationContext(creationContext); err != nil {
		return nil, fmt.Errorf("invalid creation context: %w", err)
	}

	// Get SpecTask to ensure it exists and user has permission
	specTask, err := s.store.GetSpecTask(ctx, creationContext.SpecTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Validate user permission
	if specTask.CreatedBy != creationContext.UserID {
		return nil, fmt.Errorf("user %s does not have permission to create sessions for SpecTask %s",
			creationContext.UserID, creationContext.SpecTaskID)
	}

	// Determine creation method and parent session
	creationMethod, parentWorkSession, err := s.determineCreationMethod(ctx, creationContext, specTask)
	if err != nil {
		return nil, fmt.Errorf("failed to determine creation method: %w", err)
	}

	// Create work session and Helix session
	workSession, helixSession, err := s.createSessionPair(ctx, creationContext, specTask, parentWorkSession, creationMethod)
	if err != nil {
		return nil, fmt.Errorf("failed to create session pair: %w", err)
	}

	// Create or update Zed thread mapping
	zedThread, err := s.createOrUpdateZedThreadMapping(ctx, workSession, creationContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create Zed thread mapping: %w", err)
	}

	// Register session in context service
	s.sessionContextService.OnWorkSessionCreated(ctx, workSession)

	// If this was spawned from another session, notify coordination
	if parentWorkSession != nil {
		s.sessionContextService.OnWorkSessionSpawned(
			ctx,
			parentWorkSession.ID,
			workSession.ID,
			creationContext.SpawnReason,
		)
	}

	result := &ZedSessionCreationResult{
		WorkSession:       workSession,
		HelixSession:      helixSession,
		ZedThread:         zedThread,
		SpecTask:          specTask,
		CreationMethod:    creationMethod,
		ParentWorkSession: parentWorkSession,
		Success:           true,
		Message:           fmt.Sprintf("Successfully created Helix session for Zed thread '%s'", creationContext.ThreadName),
	}

	log.Info().
		Str("work_session_id", workSession.ID).
		Str("helix_session_id", helixSession.ID).
		Str("zed_thread_id", zedThread.ID).
		Str("creation_method", creationMethod).
		Msg("Successfully created Helix session from Zed thread")

	return result, nil
}

// HandleZedThreadSpawnEvent handles events when Zed threads spawn other threads
func (s *ZedToHelixSessionService) HandleZedThreadSpawnEvent(
	ctx context.Context,
	parentZedThreadID string,
	spawnedZedThreadID string,
	spawnContext map[string]interface{},
) (*ZedSessionCreationResult, error) {
	// Find parent work session from Zed thread
	parentZedThread, err := s.findZedThreadByZedID(ctx, parentZedThreadID)
	if err != nil {
		return nil, fmt.Errorf("failed to find parent Zed thread: %w", err)
	}

	parentWorkSession, err := s.store.GetSpecTaskWorkSession(ctx, parentZedThread.WorkSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent work session: %w", err)
	}

	// Extract spawn information from context
	threadName, _ := spawnContext["thread_name"].(string)
	if threadName == "" {
		threadName = fmt.Sprintf("Spawned from %s", parentWorkSession.Name)
	}

	description, _ := spawnContext["description"].(string)
	if description == "" {
		description = fmt.Sprintf("Work session spawned from '%s' for additional implementation needs", parentWorkSession.Name)
	}

	spawnReason, _ := spawnContext["spawn_reason"].(string)
	if spawnReason == "" {
		spawnReason = "Additional work identified during implementation"
	}

	userID, _ := spawnContext["user_id"].(string)
	if userID == "" {
		// Load SpecTask to get CreatedBy
		specTask, err := s.store.GetSpecTask(ctx, parentWorkSession.SpecTaskID)
		if err == nil && specTask != nil {
			userID = specTask.CreatedBy
		}
	}

	// Create context for spawned thread
	creationContext := &ZedThreadCreationContext{
		ZedInstanceID:        parentZedThread.SpecTaskID, // Use SpecTask ID as instance ID
		ZedThreadID:          spawnedZedThreadID,
		SpecTaskID:           parentWorkSession.SpecTaskID,
		ParentZedThreadID:    parentZedThreadID,
		ThreadName:           threadName,
		ThreadDescription:    description,
		UserID:               userID,
		SpawnReason:          spawnReason,
		AgentConfiguration:   extractAgentConfig(spawnContext),
		EnvironmentVariables: extractEnvironmentVars(spawnContext),
		InitialPrompt:        extractInitialPrompt(spawnContext),
		ExpectedWorkType:     extractWorkType(spawnContext),
	}

	// Create session from spawned thread
	return s.CreateHelixSessionFromZedThread(ctx, creationContext)
}

// ValidateZedThreadPermissions validates that a user can create threads in a SpecTask
func (s *ZedToHelixSessionService) ValidateZedThreadPermissions(
	ctx context.Context,
	userID string,
	specTaskID string,
	zedInstanceID string,
) error {
	// Get SpecTask
	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Check ownership
	if specTask.CreatedBy != userID {
		return fmt.Errorf("user %s does not own SpecTask %s", userID, specTaskID)
	}

	// Check that SpecTask is in implementation phase
	if specTask.Status != types.TaskStatusImplementation {
		return fmt.Errorf("SpecTask must be in implementation phase to create threads (current: %s)", specTask.Status)
	}

	// Validate Zed instance matches
	if specTask.ZedInstanceID != "" && specTask.ZedInstanceID != zedInstanceID {
		return fmt.Errorf("Zed instance ID mismatch: expected %s, got %s", specTask.ZedInstanceID, zedInstanceID)
	}

	return nil
}

// Private helper methods

func (s *ZedToHelixSessionService) validateCreationContext(ctx *ZedThreadCreationContext) error {
	if ctx.ZedInstanceID == "" {
		return fmt.Errorf("zed_instance_id is required")
	}
	if ctx.ZedThreadID == "" {
		return fmt.Errorf("zed_thread_id is required")
	}
	if ctx.SpecTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}
	if ctx.ThreadName == "" {
		return fmt.Errorf("thread_name is required")
	}
	if ctx.UserID == "" {
		return fmt.Errorf("user_id is required")
	}

	// Validate thread name (basic sanitization)
	if len(ctx.ThreadName) > 255 {
		return fmt.Errorf("thread_name too long (max 255 characters)")
	}

	// Validate Zed thread ID format
	if !strings.HasPrefix(ctx.ZedThreadID, "thread_") {
		return fmt.Errorf("invalid zed_thread_id format (should start with 'thread_')")
	}

	return nil
}

func (s *ZedToHelixSessionService) determineCreationMethod(
	ctx context.Context,
	creationContext *ZedThreadCreationContext,
	specTask *types.SpecTask,
) (string, *types.SpecTaskWorkSession, error) {
	// Check if this corresponds to a planned implementation task
	if creationContext.RelatedImplementationTask >= 0 {
		// This is a planned session from the implementation plan
		return "planned", nil, nil
	}

	// Check if this is spawned from an existing thread
	if creationContext.ParentZedThreadID != "" {
		parentZedThread, err := s.findZedThreadByZedID(ctx, creationContext.ParentZedThreadID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to find parent Zed thread: %w", err)
		}

		parentWorkSession, err := s.store.GetSpecTaskWorkSession(ctx, parentZedThread.WorkSessionID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get parent work session: %w", err)
		}

		return "spawned", parentWorkSession, nil
	}

	// This is an ad-hoc session creation
	return "ad_hoc", nil, nil
}

func (s *ZedToHelixSessionService) createSessionPair(
	ctx context.Context,
	creationContext *ZedThreadCreationContext,
	specTask *types.SpecTask,
	parentWorkSession *types.SpecTaskWorkSession,
	creationMethod string,
) (*types.SpecTaskWorkSession, *types.Session, error) {
	// Get the code agent runtime from the spec task's app
	codeAgentRuntime := s.getCodeAgentRuntimeForSpecTask(ctx, specTask)

	// Create Helix session first
	helixSession := &types.Session{
		ID:      system.GenerateSessionID(),
		Name:    s.generateSessionName(creationContext, creationMethod),
		Owner:   creationContext.UserID,
		Type:    types.SessionTypeText,
		Mode:    types.SessionModeInference,
		Created: time.Now(),
		Updated: time.Now(),
		Metadata: types.SessionMetadata{
			AgentType:        "zed_external", // Single agent type for entire workflow
			SpecTaskID:       creationContext.SpecTaskID,
			SessionRole:      "implementation",
			ZedThreadID:      creationContext.ZedThreadID,
			ZedInstanceID:    creationContext.ZedInstanceID,
			SystemPrompt:     s.generateSystemPrompt(creationContext, specTask),
			CodeAgentRuntime: codeAgentRuntime, // Store for thread resume
		},
	}

	// Set project context if available
	if specTask.ProjectID != "" {
		helixSession.ParentApp = specTask.ProjectID
	}

	// Create the Helix session
	_, err := s.store.CreateSession(ctx, *helixSession)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Helix session: %w", err)
	}

	// Create work session
	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:     creationContext.SpecTaskID,
		HelixSessionID: helixSession.ID,
		Name:           creationContext.ThreadName,
		Description:    creationContext.ThreadDescription,
		Phase:          types.SpecTaskPhaseImplementation,
		Status:         types.SpecTaskWorkSessionStatusPending,
	}

	// Set parent relationship if spawned
	if parentWorkSession != nil {
		workSession.ParentWorkSessionID = parentWorkSession.ID
		workSession.SpawnedBySessionID = parentWorkSession.ID
	}

	// Set implementation task context if related to planned task
	if creationContext.RelatedImplementationTask >= 0 {
		workSession.ImplementationTaskIndex = creationContext.RelatedImplementationTask

		// Try to get implementation task details
		implTasks, err := s.store.ListSpecTaskImplementationTasks(ctx, creationContext.SpecTaskID)
		if err == nil && creationContext.RelatedImplementationTask < len(implTasks) {
			task := implTasks[creationContext.RelatedImplementationTask]
			workSession.ImplementationTaskTitle = task.Title
			workSession.ImplementationTaskDescription = task.Description
		}
	}

	// Set agent and environment configuration
	if creationContext.AgentConfiguration != nil {
		configBytes, err := json.Marshal(creationContext.AgentConfiguration)
		if err == nil {
			workSession.AgentConfig = configBytes
		}
	}

	// Create the work session
	err = s.store.CreateSpecTaskWorkSession(ctx, workSession)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create work session: %w", err)
	}

	// Update session metadata with work session ID
	helixSession.Metadata.WorkSessionID = workSession.ID
	if workSession.ImplementationTaskIndex >= 0 {
		helixSession.Metadata.ImplementationTaskIndex = workSession.ImplementationTaskIndex
	}

	_, err = s.store.UpdateSession(ctx, *helixSession)
	if err != nil {
		log.Warn().Err(err).Str("session_id", helixSession.ID).Msg("Failed to update session metadata")
	}

	return workSession, helixSession, nil
}

func (s *ZedToHelixSessionService) createOrUpdateZedThreadMapping(
	ctx context.Context,
	workSession *types.SpecTaskWorkSession,
	creationContext *ZedThreadCreationContext,
) (*types.SpecTaskZedThread, error) {
	// Check if Zed thread mapping already exists
	existingThread, err := s.findZedThreadByZedID(ctx, creationContext.ZedThreadID)
	if err == nil && existingThread != nil {
		// Update existing mapping
		existingThread.WorkSessionID = workSession.ID
		existingThread.Status = types.SpecTaskZedStatusActive
		now := time.Now()
		existingThread.LastActivityAt = &now

		err = s.store.UpdateSpecTaskZedThread(ctx, existingThread)
		if err != nil {
			return nil, fmt.Errorf("failed to update existing Zed thread: %w", err)
		}

		log.Info().
			Str("zed_thread_id", existingThread.ID).
			Str("work_session_id", workSession.ID).
			Msg("Updated existing Zed thread mapping")

		return existingThread, nil
	}

	// Create new Zed thread mapping
	zedThread := &types.SpecTaskZedThread{
		WorkSessionID: workSession.ID,
		SpecTaskID:    workSession.SpecTaskID,
		ZedThreadID:   creationContext.ZedThreadID,
		Status:        types.SpecTaskZedStatusActive,
	}

	// Set thread configuration
	threadConfig := map[string]interface{}{
		"thread_name":              creationContext.ThreadName,
		"thread_description":       creationContext.ThreadDescription,
		"spawn_reason":             creationContext.SpawnReason,
		"expected_work_type":       creationContext.ExpectedWorkType,
		"estimated_duration_hours": creationContext.EstimatedDurationHours,
		"initial_prompt":           creationContext.InitialPrompt,
		"created_from_zed":         true,
		"creation_timestamp":       time.Now(),
	}

	configBytes, err := json.Marshal(threadConfig)
	if err == nil {
		zedThread.ThreadConfig = configBytes
	}

	// Set activity timestamp
	now := time.Now()
	zedThread.LastActivityAt = &now

	err = s.store.CreateSpecTaskZedThread(ctx, zedThread)
	if err != nil {
		return nil, fmt.Errorf("failed to create Zed thread mapping: %w", err)
	}

	log.Info().
		Str("zed_thread_id", zedThread.ID).
		Str("work_session_id", workSession.ID).
		Str("zed_thread_id_external", creationContext.ZedThreadID).
		Msg("Created new Zed thread mapping")

	return zedThread, nil
}

func (s *ZedToHelixSessionService) generateSessionName(
	creationContext *ZedThreadCreationContext,
	creationMethod string,
) string {
	switch creationMethod {
	case "spawned":
		return fmt.Sprintf("[Spawned] %s", creationContext.ThreadName)
	case "planned":
		return fmt.Sprintf("[Planned] %s", creationContext.ThreadName)
	case "ad_hoc":
		return fmt.Sprintf("[Ad-hoc] %s", creationContext.ThreadName)
	default:
		return creationContext.ThreadName
	}
}

func (s *ZedToHelixSessionService) generateSystemPrompt(
	creationContext *ZedThreadCreationContext,
	specTask *types.SpecTask,
) string {
	basePrompt := fmt.Sprintf(`You are a senior software engineer working on a specific task within a larger SpecTask.

**SpecTask Context:**
- Project: %s
- Overall Description: %s
- Current Phase: Implementation

**Your Specific Thread:**
- Thread Name: %s
- Description: %s`,
		specTask.Name,
		specTask.Description,
		creationContext.ThreadName,
		creationContext.ThreadDescription,
	)

	// Add spawn context if applicable
	if creationContext.SpawnReason != "" {
		basePrompt += fmt.Sprintf(`
- Spawn Reason: %s`, creationContext.SpawnReason)
	}

	// Add implementation task context if available
	if creationContext.RelatedImplementationTask >= 0 {
		basePrompt += fmt.Sprintf(`
- Implementation Task Index: %d`, creationContext.RelatedImplementationTask)
	}

	// Add approved specifications
	if specTask.RequirementsSpec != "" || specTask.TechnicalDesign != "" || specTask.ImplementationPlan != "" {
		basePrompt += `

**Approved Specifications:**`

		if specTask.RequirementsSpec != "" {
			basePrompt += fmt.Sprintf(`

## Requirements
%s`, specTask.RequirementsSpec)
		}

		if specTask.TechnicalDesign != "" {
			basePrompt += fmt.Sprintf(`

## Technical Design
%s`, specTask.TechnicalDesign)
		}

		if specTask.ImplementationPlan != "" {
			basePrompt += fmt.Sprintf(`

## Implementation Plan Context
%s`, specTask.ImplementationPlan)
		}
	}

	// Add role instructions
	basePrompt += `

**Your Role:**
You are working within a multi-session SpecTask where other agents may be working on related tasks in parallel. Your focus should be:

1. Implement your specific task following the approved specifications
2. Coordinate with other sessions when needed using available skills
3. Write clean, tested, production-ready code
4. Use the LoopInHuman skill when you need clarification or help
5. Use the JobCompleted skill when your specific work is done

Remember: You are part of a larger coordinated effort. Other agents are working on related tasks in parallel within the same Zed instance.`

	return basePrompt
}

func (s *ZedToHelixSessionService) findZedThreadByZedID(ctx context.Context, zedThreadID string) (*types.SpecTaskZedThread, error) {
	// This is inefficient but works for now - in production we'd add a database index
	// Get all SpecTasks and search through their Zed threads
	// For now, we'll assume we have the SpecTask ID in context

	// This method needs optimization in production - should be a direct database query
	return nil, fmt.Errorf("findZedThreadByZedID not implemented - need database optimization")
}

// Helper functions for extracting data from spawn context

func extractAgentConfig(spawnContext map[string]interface{}) map[string]interface{} {
	if config, ok := spawnContext["agent_config"].(map[string]interface{}); ok {
		return config
	}
	return make(map[string]interface{})
}

func extractEnvironmentVars(spawnContext map[string]interface{}) []string {
	if envVars, ok := spawnContext["environment"].([]interface{}); ok {
		vars := make([]string, 0, len(envVars))
		for _, v := range envVars {
			if str, ok := v.(string); ok {
				vars = append(vars, str)
			}
		}
		return vars
	}
	return []string{}
}

func extractInitialPrompt(spawnContext map[string]interface{}) string {
	if prompt, ok := spawnContext["initial_prompt"].(string); ok {
		return prompt
	}
	return ""
}

func extractWorkType(spawnContext map[string]interface{}) string {
	if workType, ok := spawnContext["work_type"].(string); ok {
		return workType
	}
	return "implementation"
}

// Integration methods for event handling

// OnZedThreadCreated should be called when a Zed thread is created
func (s *ZedToHelixSessionService) OnZedThreadCreated(
	ctx context.Context,
	event *types.ZedInstanceEvent,
) error {
	if event.ThreadID == "" {
		return fmt.Errorf("thread_id required for thread creation event")
	}

	// Extract creation context from event data
	creationContext := &ZedThreadCreationContext{
		ZedInstanceID:     event.InstanceID,
		ZedThreadID:       event.ThreadID,
		SpecTaskID:        event.SpecTaskID,
		ThreadName:        extractStringFromData(event.Data, "thread_name", "New Thread"),
		ThreadDescription: extractStringFromData(event.Data, "description", ""),
		UserID:            extractStringFromData(event.Data, "user_id", ""),
		SpawnReason:       extractStringFromData(event.Data, "spawn_reason", ""),
		InitialPrompt:     extractStringFromData(event.Data, "initial_prompt", ""),
	}

	// Extract parent thread if this is a spawn
	if parentThreadID := extractStringFromData(event.Data, "parent_thread_id", ""); parentThreadID != "" {
		creationContext.ParentZedThreadID = parentThreadID
	}

	// Get user ID from SpecTask if not provided in event
	if creationContext.UserID == "" {
		specTask, err := s.store.GetSpecTask(ctx, creationContext.SpecTaskID)
		if err != nil {
			return fmt.Errorf("failed to get SpecTask for user ID: %w", err)
		}
		creationContext.UserID = specTask.CreatedBy
	}

	// Create Helix session from Zed thread
	result, err := s.CreateHelixSessionFromZedThread(ctx, creationContext)
	if err != nil {
		return fmt.Errorf("failed to create Helix session from Zed thread: %w", err)
	}

	log.Info().
		Str("zed_thread_id", event.ThreadID).
		Str("work_session_id", result.WorkSession.ID).
		Str("helix_session_id", result.HelixSession.ID).
		Str("creation_method", result.CreationMethod).
		Msg("Successfully created Helix session from Zed thread event")

	return nil
}

// OnZedThreadSpawned should be called when a Zed thread spawns another thread
func (s *ZedToHelixSessionService) OnZedThreadSpawned(
	ctx context.Context,
	parentThreadID string,
	spawnedThreadID string,
	spawnContext map[string]interface{},
) error {
	result, err := s.HandleZedThreadSpawnEvent(ctx, parentThreadID, spawnedThreadID, spawnContext)
	if err != nil {
		return fmt.Errorf("failed to handle thread spawn: %w", err)
	}

	log.Info().
		Str("parent_thread_id", parentThreadID).
		Str("spawned_thread_id", spawnedThreadID).
		Str("work_session_id", result.WorkSession.ID).
		Msg("Successfully handled Zed thread spawn event")

	return nil
}

// Helper functions

// getCodeAgentRuntimeForSpecTask looks up the spec task's app and extracts the CodeAgentRuntime.
// This is stored in the session metadata so we can send the correct agent_name in open_thread commands.
func (s *ZedToHelixSessionService) getCodeAgentRuntimeForSpecTask(ctx context.Context, specTask *types.SpecTask) types.CodeAgentRuntime {
	if specTask.HelixAppID == "" {
		log.Debug().Str("spec_task_id", specTask.ID).Msg("Spec task has no HelixAppID, defaulting to zed_agent runtime")
		return types.CodeAgentRuntimeZedAgent
	}

	app, err := s.store.GetApp(ctx, specTask.HelixAppID)
	if err != nil {
		log.Warn().Err(err).
			Str("spec_task_id", specTask.ID).
			Str("helix_app_id", specTask.HelixAppID).
			Msg("Failed to get app for code agent runtime, defaulting to zed_agent")
		return types.CodeAgentRuntimeZedAgent
	}

	// Find the zed_external assistant in the app config
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.AgentType == types.AgentTypeZedExternal {
			if assistant.CodeAgentRuntime != "" {
				log.Debug().
					Str("spec_task_id", specTask.ID).
					Str("helix_app_id", specTask.HelixAppID).
					Str("code_agent_runtime", string(assistant.CodeAgentRuntime)).
					Msg("Found code agent runtime from app config")
				return assistant.CodeAgentRuntime
			}
			break
		}
	}

	log.Debug().
		Str("spec_task_id", specTask.ID).
		Str("helix_app_id", specTask.HelixAppID).
		Msg("No code agent runtime configured in app, defaulting to zed_agent")
	return types.CodeAgentRuntimeZedAgent
}

func extractStringFromData(data map[string]interface{}, key string, defaultValue string) string {
	if value, ok := data[key].(string); ok {
		return value
	}
	return defaultValue
}

func generateZedEventID() string {
	return fmt.Sprintf("event_%d", time.Now().UnixNano())
}
