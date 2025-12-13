package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ZedIntegrationService manages Zed instances and threads for multi-session SpecTasks
type ZedIntegrationService struct {
	store          store.Store
	controller     *controller.Controller
	pubsub         pubsub.PubSub
	protocolClient *pubsub.ZedProtocolClient
	testMode       bool
}

// NewZedIntegrationService creates a new Zed integration service
func NewZedIntegrationService(
	store store.Store,
	controller *controller.Controller,
	ps pubsub.PubSub,
) *ZedIntegrationService {
	service := &ZedIntegrationService{
		store:      store,
		controller: controller,
		pubsub:     ps,
		testMode:   false,
	}

	// Initialize protocol client
	service.protocolClient = pubsub.NewZedProtocolClient(ps)

	return service
}

// SetTestMode enables or disables test mode
func (s *ZedIntegrationService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// CreateZedInstanceForSpecTask creates a new Zed instance for a SpecTask
func (s *ZedIntegrationService) CreateZedInstanceForSpecTask(
	ctx context.Context,
	specTask *types.SpecTask,
	config map[string]interface{},
) (string, error) {
	// Generate unique Zed instance ID
	instanceID := fmt.Sprintf("zed_instance_%s_%d", specTask.ID, time.Now().Unix())

	// Determine project path
	projectPath := specTask.ProjectPath
	if projectPath == "" {
		projectPath = fmt.Sprintf("/workspace/%s", specTask.ID)
	}

	// Create initial Zed agent configuration for the instance
	zedAgent := &types.ZedAgent{
		SessionID:   specTask.ID, // Use SpecTask ID as session ID for instance
		UserID:      specTask.CreatedBy,
		Input:       fmt.Sprintf("Initialize workspace for: %s", specTask.Name),
		ProjectPath: projectPath,
		WorkDir:     projectPath,
		InstanceID:  instanceID,
		Env: []string{
			"SPEC_TASK_ID=" + specTask.ID,
			"SPEC_TASK_NAME=" + specTask.Name,
			"INSTANCE_MODE=multi_session",
		},
	}

	// Add environment variables from config
	if config != nil {
		for key, value := range config {
			if strValue, ok := value.(string); ok {
				zedAgent.Env = append(zedAgent.Env, fmt.Sprintf("%s=%s", key, strValue))
			}
		}
	}

	// Launch Zed instance (unless in test mode)
	if !s.testMode {
		err := s.launchZedInstance(ctx, zedAgent)
		if err != nil {
			return "", fmt.Errorf("failed to launch Zed instance: %w", err)
		}
	}

	// Update SpecTask with instance ID
	err := s.store.UpdateSpecTaskZedInstance(ctx, specTask.ID, instanceID)
	if err != nil {
		return "", fmt.Errorf("failed to update SpecTask with Zed instance: %w", err)
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("zed_instance_id", instanceID).
		Str("project_path", projectPath).
		Msg("Created Zed instance for SpecTask")

	return instanceID, nil
}

// CreateZedThreadForWorkSession creates a new Zed thread for a work session
func (s *ZedIntegrationService) CreateZedThreadForWorkSession(
	ctx context.Context,
	workSession *types.SpecTaskWorkSession,
	zedInstanceID string,
) (*types.SpecTaskZedThread, error) {
	// Generate unique thread ID within the instance
	threadID := fmt.Sprintf("thread_%s", workSession.ID)

	// Create Zed thread mapping
	zedThread := &types.SpecTaskZedThread{
		WorkSessionID: workSession.ID,
		SpecTaskID:    workSession.SpecTaskID,
		ZedThreadID:   threadID,
		Status:        types.SpecTaskZedStatusPending,
	}

	// Set thread configuration
	threadConfig := map[string]interface{}{
		"work_session_name":         workSession.Name,
		"description":               workSession.Description,
		"implementation_task_title": workSession.ImplementationTaskTitle,
		"implementation_task_index": workSession.ImplementationTaskIndex,
		"phase":                     workSession.Phase,
	}

	configBytes, err := json.Marshal(threadConfig)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to marshal thread config")
	} else {
		zedThread.ThreadConfig = configBytes
	}

	// Create Zed thread record
	err = s.store.CreateSpecTaskZedThread(ctx, zedThread)
	if err != nil {
		return nil, fmt.Errorf("failed to create Zed thread record: %w", err)
	}

	// Send thread creation command to Zed instance (unless in test mode)
	if !s.testMode {
		err = s.createThreadInZedInstance(ctx, zedInstanceID, threadID, threadConfig)
		if err != nil {
			log.Error().Err(err).
				Str("zed_instance_id", zedInstanceID).
				Str("thread_id", threadID).
				Msg("Failed to create thread in Zed instance")
			// Don't fail completely - the thread record is created
		}
	}

	log.Info().
		Str("zed_thread_id", zedThread.ID).
		Str("work_session_id", workSession.ID).
		Str("spec_task_id", workSession.SpecTaskID).
		Str("thread_id_external", threadID).
		Str("zed_instance_id", zedInstanceID).
		Msg("Created Zed thread for work session")

	return zedThread, nil
}

// UpdateZedThreadStatus updates the status of a Zed thread
func (s *ZedIntegrationService) UpdateZedThreadStatus(
	ctx context.Context,
	threadID string,
	status types.SpecTaskZedStatus,
	activityData map[string]interface{},
) error {
	zedThread, err := s.store.GetSpecTaskZedThread(ctx, threadID)
	if err != nil {
		return fmt.Errorf("failed to get Zed thread: %w", err)
	}

	oldStatus := zedThread.Status
	zedThread.Status = status

	// Update activity timestamp
	if status == types.SpecTaskZedStatusActive || activityData != nil {
		now := time.Now()
		zedThread.LastActivityAt = &now
	}

	// Update thread configuration if activity data provided
	if activityData != nil {
		configBytes, err := json.Marshal(activityData)
		if err == nil {
			zedThread.ThreadConfig = configBytes
		}
	}

	err = s.store.UpdateSpecTaskZedThread(ctx, zedThread)
	if err != nil {
		return fmt.Errorf("failed to update Zed thread status: %w", err)
	}

	log.Info().
		Str("zed_thread_id", threadID).
		Str("work_session_id", zedThread.WorkSessionID).
		Str("old_status", string(oldStatus)).
		Str("new_status", string(status)).
		Msg("Updated Zed thread status")

	return nil
}

// CleanupZedInstance cleans up a Zed instance when SpecTask completes
func (s *ZedIntegrationService) CleanupZedInstance(
	ctx context.Context,
	specTaskID string,
) error {
	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return fmt.Errorf("failed to get SpecTask: %w", err)
	}

	if specTask.ZedInstanceID == "" {
		// No Zed instance to clean up
		return nil
	}

	// Get all Zed threads for this SpecTask
	zedThreads, err := s.store.ListSpecTaskZedThreads(ctx, specTaskID)
	if err != nil {
		log.Warn().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to list Zed threads for cleanup")
	}

	// Mark all threads as completed
	for _, thread := range zedThreads {
		if thread.Status != types.SpecTaskZedStatusCompleted {
			thread.Status = types.SpecTaskZedStatusCompleted
			err = s.store.UpdateSpecTaskZedThread(ctx, thread)
			if err != nil {
				log.Warn().Err(err).Str("zed_thread_id", thread.ID).Msg("Failed to mark thread as completed")
			}
		}
	}

	// Send cleanup command to Zed instance (unless in test mode)
	if !s.testMode {
		err = s.shutdownZedInstance(ctx, specTask.ZedInstanceID)
		if err != nil {
			log.Error().Err(err).
				Str("zed_instance_id", specTask.ZedInstanceID).
				Msg("Failed to shutdown Zed instance")
		}
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("zed_instance_id", specTask.ZedInstanceID).
		Int("thread_count", len(zedThreads)).
		Msg("Cleaned up Zed instance for completed SpecTask")

	return nil
}

// GetZedInstanceStatus returns the status of a Zed instance for a SpecTask
func (s *ZedIntegrationService) GetZedInstanceStatus(
	ctx context.Context,
	specTaskID string,
) (*types.ZedInstanceStatus, error) {
	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	if specTask.ZedInstanceID == "" {
		return &types.ZedInstanceStatus{
			SpecTaskID: specTaskID,
			Status:     "not_created",
		}, nil
	}

	// Get all Zed threads for status
	zedThreads, err := s.store.ListSpecTaskZedThreads(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list Zed threads: %w", err)
	}

	// Calculate overall status
	status := s.calculateInstanceStatus(zedThreads)

	var lastActivity *time.Time
	activeThreads := 0
	for _, thread := range zedThreads {
		if thread.Status == types.SpecTaskZedStatusActive {
			activeThreads++
		}
		if thread.LastActivityAt != nil && (lastActivity == nil || thread.LastActivityAt.After(*lastActivity)) {
			lastActivity = thread.LastActivityAt
		}
	}

	return &types.ZedInstanceStatus{
		SpecTaskID:    specTaskID,
		ZedInstanceID: specTask.ZedInstanceID,
		Status:        status,
		ThreadCount:   len(zedThreads),
		ActiveThreads: activeThreads,
		LastActivity:  lastActivity,
		ProjectPath:   specTask.ProjectPath,
	}, nil
}

// Private helper methods

func (s *ZedIntegrationService) launchZedInstance(ctx context.Context, zedAgent *types.ZedAgent) error {
	// Convert ZedAgent to instance creation request
	instanceReq := pubsub.ConvertZedAgentToInstanceRequest(zedAgent)

	// Send instance creation request via protocol
	err := s.protocolClient.SendInstanceCreateRequest(ctx, instanceReq)
	if err != nil {
		return fmt.Errorf("failed to send instance create request: %w", err)
	}

	log.Info().
		Str("instance_id", zedAgent.InstanceID).
		Str("spec_task_id", zedAgent.SessionID).
		Str("project_path", zedAgent.ProjectPath).
		Msg("Sent Zed instance creation request via protocol")

	return nil
}

func (s *ZedIntegrationService) createThreadInZedInstance(
	ctx context.Context,
	instanceID string,
	threadID string,
	config map[string]interface{},
) error {
	// Create thread configuration
	threadConfig := pubsub.ZedThreadConfig{
		ThreadID: threadID,
		Name:     fmt.Sprintf("Thread %s", threadID),
	}

	// Extract work session info from config
	if workSessionID, ok := config["work_session_id"].(string); ok {
		threadConfig.WorkSessionID = workSessionID
	}
	if name, ok := config["work_session_name"].(string); ok {
		threadConfig.Name = name
	}
	if desc, ok := config["description"].(string); ok {
		threadConfig.Description = desc
	}
	if title, ok := config["implementation_task_title"].(string); ok {
		threadConfig.ImplementationTaskTitle = title
	}
	if index, ok := config["implementation_task_index"].(int); ok {
		threadConfig.ImplementationTaskIndex = index
	}

	// Create thread request
	threadReq := &pubsub.ZedThreadCreateRequest{
		InstanceID: instanceID,
		Thread:     threadConfig,
	}

	// Send thread creation request via protocol
	err := s.protocolClient.SendThreadCreateRequest(ctx, threadReq)
	if err != nil {
		return fmt.Errorf("failed to send thread create request: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Str("thread_id", threadID).
		Str("work_session_id", threadConfig.WorkSessionID).
		Msg("Sent thread creation request via protocol")

	return nil
}

func (s *ZedIntegrationService) shutdownZedInstance(ctx context.Context, instanceID string) error {
	// Create shutdown message
	data := map[string]interface{}{
		"instance_id": instanceID,
		"command":     "shutdown",
	}

	metadata := pubsub.ZedMessageMetadata{
		InstanceID: instanceID,
		Priority:   1,
		TTL:        60,
	}

	msg := pubsub.NewZedProtocolMessage(pubsub.ZedMessageTypeInstanceStop, data, metadata)

	// Serialize and send
	msgData, err := pubsub.SerializeZedMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize shutdown message: %w", err)
	}

	headers := pubsub.CreateMessageHeaders(msg)
	stream := pubsub.GetStreamForMessageType(msg.Type)
	queue := pubsub.GetQueueForMessageType(msg.Type)

	_, err = s.pubsub.StreamRequest(ctx, stream, queue, msgData, headers, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send shutdown command: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Msg("Sent shutdown command via protocol")

	return nil
}

func (s *ZedIntegrationService) calculateInstanceStatus(threads []*types.SpecTaskZedThread) string {
	if len(threads) == 0 {
		return "empty"
	}

	activeCount := 0
	completedCount := 0
	failedCount := 0
	disconnectedCount := 0

	for _, thread := range threads {
		switch thread.Status {
		case types.SpecTaskZedStatusActive:
			activeCount++
		case types.SpecTaskZedStatusCompleted:
			completedCount++
		case types.SpecTaskZedStatusFailed:
			failedCount++
		case types.SpecTaskZedStatusDisconnected:
			disconnectedCount++
		}
	}

	// Determine overall status
	if failedCount > 0 {
		return "failed"
	}
	if disconnectedCount > 0 && activeCount == 0 {
		return "disconnected"
	}
	if completedCount == len(threads) {
		return "completed"
	}
	if activeCount > 0 {
		return "active"
	}
	return "pending"
}

// HandleZedInstanceEvent processes events from Zed instances
func (s *ZedIntegrationService) HandleZedInstanceEvent(
	ctx context.Context,
	event *types.ZedInstanceEvent,
) error {
	log.Info().
		Str("instance_id", event.InstanceID).
		Str("event_type", event.EventType).
		Str("thread_id", event.ThreadID).
		Msg("Processing Zed instance event")

	switch event.EventType {
	case "instance_ready":
		return s.handleInstanceReady(ctx, event)
	case "thread_created":
		return s.handleThreadCreated(ctx, event)
	case "thread_status_changed":
		return s.handleThreadStatusChanged(ctx, event)
	case "instance_disconnected":
		return s.handleInstanceDisconnected(ctx, event)
	case "instance_error":
		return s.handleInstanceError(ctx, event)
	default:
		log.Warn().
			Str("event_type", event.EventType).
			Msg("Unknown Zed instance event type")
		return nil
	}
}

func (s *ZedIntegrationService) handleInstanceReady(ctx context.Context, event *types.ZedInstanceEvent) error {
	// Instance is ready, we can start creating threads
	log.Info().
		Str("instance_id", event.InstanceID).
		Msg("Zed instance ready for thread creation")

	// TODO: Trigger creation of initial threads if needed
	return nil
}

func (s *ZedIntegrationService) handleThreadCreated(ctx context.Context, event *types.ZedInstanceEvent) error {
	// Thread was created in Zed, update our records
	if event.ThreadID == "" {
		return fmt.Errorf("thread_id required for thread_created event")
	}

	// Find the Zed thread record
	zedThreads, err := s.store.ListSpecTaskZedThreads(ctx, event.SpecTaskID)
	if err != nil {
		return fmt.Errorf("failed to list Zed threads: %w", err)
	}

	for _, thread := range zedThreads {
		if thread.ZedThreadID == event.ThreadID {
			err = s.UpdateZedThreadStatus(ctx, thread.ID, types.SpecTaskZedStatusActive, event.Data)
			if err != nil {
				log.Error().Err(err).Str("zed_thread_id", thread.ID).Msg("Failed to update thread status")
			}
			break
		}
	}

	return nil
}

func (s *ZedIntegrationService) handleThreadStatusChanged(ctx context.Context, event *types.ZedInstanceEvent) error {
	// Thread status changed in Zed, update our records
	if event.ThreadID == "" {
		return fmt.Errorf("thread_id required for thread_status_changed event")
	}

	// Parse new status from event data
	newStatus := types.SpecTaskZedStatusActive
	if statusStr, ok := event.Data["status"].(string); ok {
		newStatus = types.SpecTaskZedStatus(statusStr)
	}

	// Find and update the Zed thread record
	zedThreads, err := s.store.ListSpecTaskZedThreads(ctx, event.SpecTaskID)
	if err != nil {
		return fmt.Errorf("failed to list Zed threads: %w", err)
	}

	for _, thread := range zedThreads {
		if thread.ZedThreadID == event.ThreadID {
			err = s.UpdateZedThreadStatus(ctx, thread.ID, newStatus, event.Data)
			if err != nil {
				log.Error().Err(err).Str("zed_thread_id", thread.ID).Msg("Failed to update thread status")
			}
			break
		}
	}

	return nil
}

func (s *ZedIntegrationService) handleInstanceDisconnected(ctx context.Context, event *types.ZedInstanceEvent) error {
	// Mark all threads in instance as disconnected
	zedThreads, err := s.store.ListSpecTaskZedThreads(ctx, event.SpecTaskID)
	if err != nil {
		return fmt.Errorf("failed to list Zed threads: %w", err)
	}

	for _, thread := range zedThreads {
		if thread.Status == types.SpecTaskZedStatusActive {
			err = s.UpdateZedThreadStatus(ctx, thread.ID, types.SpecTaskZedStatusDisconnected, event.Data)
			if err != nil {
				log.Error().Err(err).Str("zed_thread_id", thread.ID).Msg("Failed to mark thread as disconnected")
			}
		}
	}

	log.Warn().
		Str("instance_id", event.InstanceID).
		Str("spec_task_id", event.SpecTaskID).
		Msg("Zed instance disconnected, marked threads as disconnected")

	return nil
}

func (s *ZedIntegrationService) handleInstanceError(ctx context.Context, event *types.ZedInstanceEvent) error {
	// Mark all threads in instance as failed
	zedThreads, err := s.store.ListSpecTaskZedThreads(ctx, event.SpecTaskID)
	if err != nil {
		return fmt.Errorf("failed to list Zed threads: %w", err)
	}

	for _, thread := range zedThreads {
		if thread.Status != types.SpecTaskZedStatusCompleted {
			err = s.UpdateZedThreadStatus(ctx, thread.ID, types.SpecTaskZedStatusFailed, event.Data)
			if err != nil {
				log.Error().Err(err).Str("zed_thread_id", thread.ID).Msg("Failed to mark thread as failed")
			}
		}
	}

	log.Error().
		Str("instance_id", event.InstanceID).
		Str("spec_task_id", event.SpecTaskID).
		Interface("error_data", event.Data).
		Msg("Zed instance error, marked threads as failed")

	return nil
}

// Types for Zed integration

// Integration with existing agent session manager
// LaunchZedAgent launches a Zed agent (used by planning service)
func (s *ZedIntegrationService) LaunchZedAgent(ctx context.Context, zedAgent *types.ZedAgent) error {
	return s.launchZedInstance(ctx, zedAgent)
}

// StopDesktop stops a desktop session by session ID
func (s *ZedIntegrationService) StopDesktop(ctx context.Context, sessionID string) error {
	// Find the instance ID for this session
	// For now, just log the stop request
	log.Info().Str("session_id", sessionID).Msg("Stop Zed agent requested")
	return nil
}

func (s *ZedIntegrationService) IntegrateWithSessionManager(sessionManager *controller.Controller) {
	// This method would be called during service initialization to ensure
	// the ZedIntegrationService is available to the session manager
	// The session manager can then call this service when launching Zed agents
	log.Info().Msg("ZedIntegrationService integrated with session manager")
}
