package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SessionContextService manages context propagation and coordination between work sessions
type SessionContextService struct {
	store    store.Store
	testMode bool

	// In-memory context cache for active sessions
	contextCache map[string]*SpecTaskSessionContext
	cacheMutex   sync.RWMutex
}

// SpecTaskSessionContext represents shared context for all sessions in a SpecTask
type SpecTaskSessionContext struct {
	SpecTaskID        string                  `json:"spec_task_id"`
	SharedState       map[string]interface{}  `json:"shared_state"`
	SessionRegistry   map[string]*SessionInfo `json:"session_registry"`
	CoordinationLog   []CoordinationEvent     `json:"coordination_log"`
	LastUpdated       time.Time               `json:"last_updated"`
	ActiveSessions    int                     `json:"active_sessions"`
	CompletedSessions int                     `json:"completed_sessions"`
	Mutex             sync.RWMutex            `json:"-"`
}

// SessionInfo represents information about a work session in the context
type SessionInfo struct {
	WorkSessionID           string                 `json:"work_session_id"`
	HelixSessionID          string                 `json:"helix_session_id"`
	ZedThreadID             string                 `json:"zed_thread_id,omitempty"`
	Name                    string                 `json:"name"`
	Status                  string                 `json:"status"`
	Phase                   string                 `json:"phase"`
	ImplementationTaskIndex int                    `json:"implementation_task_index"`
	StartedAt               *time.Time             `json:"started_at,omitempty"`
	LastActivity            time.Time              `json:"last_activity"`
	Progress                float64                `json:"progress"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
}

// CoordinationEvent represents coordination events between sessions
type CoordinationEvent struct {
	ID             string                 `json:"id"`
	FromSessionID  string                 `json:"from_session_id"`
	ToSessionID    string                 `json:"to_session_id,omitempty"` // Empty for broadcast
	EventType      CoordinationEventType  `json:"event_type"`
	Message        string                 `json:"message"`
	Data           map[string]interface{} `json:"data,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`
	Acknowledged   bool                   `json:"acknowledged"`
	AcknowledgedAt *time.Time             `json:"acknowledged_at,omitempty"`
	Response       string                 `json:"response,omitempty"`
}

// CoordinationEventType defines types of coordination events
type CoordinationEventType string

const (
	CoordinationEventTypeHandoff      CoordinationEventType = "handoff"
	CoordinationEventTypeBlocking     CoordinationEventType = "blocking"
	CoordinationEventTypeNotification CoordinationEventType = "notification"
	CoordinationEventTypeRequest      CoordinationEventType = "request"
	CoordinationEventTypeResponse     CoordinationEventType = "response"
	CoordinationEventTypeBroadcast    CoordinationEventType = "broadcast"
	CoordinationEventTypeCompletion   CoordinationEventType = "completion"
	CoordinationEventTypeSpawn        CoordinationEventType = "spawn"
)

// NewSessionContextService creates a new session context service
func NewSessionContextService(store store.Store) *SessionContextService {
	return &SessionContextService{
		store:        store,
		testMode:     false,
		contextCache: make(map[string]*SpecTaskSessionContext),
	}
}

// SetTestMode enables or disables test mode
func (s *SessionContextService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// RegisterSession registers a work session in the SpecTask context
func (s *SessionContextService) RegisterSession(
	ctx context.Context,
	workSession *types.SpecTaskWorkSession,
) error {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	// Get or create context for SpecTask
	taskContext := s.getOrCreateContext(workSession.SpecTaskID)
	taskContext.Mutex.Lock()
	defer taskContext.Mutex.Unlock()

	// Create session info
	sessionInfo := &SessionInfo{
		WorkSessionID:           workSession.ID,
		HelixSessionID:          workSession.HelixSessionID,
		Name:                    workSession.Name,
		Status:                  string(workSession.Status),
		Phase:                   string(workSession.Phase),
		ImplementationTaskIndex: workSession.ImplementationTaskIndex,
		LastActivity:            time.Now(),
		Progress:                0.0,
		Metadata:                make(map[string]interface{}),
	}

	if workSession.StartedAt != nil {
		sessionInfo.StartedAt = workSession.StartedAt
	}

	// Get Zed thread ID if available
	if zedThread, err := s.store.GetSpecTaskZedThreadByWorkSession(ctx, workSession.ID); err == nil {
		sessionInfo.ZedThreadID = zedThread.ZedThreadID
	}

	// Register session in context
	taskContext.SessionRegistry[workSession.ID] = sessionInfo

	// Update counters
	s.updateSessionCounters(taskContext)

	// Log coordination event
	event := CoordinationEvent{
		ID:            generateEventID(),
		FromSessionID: workSession.ID,
		EventType:     CoordinationEventTypeNotification,
		Message:       fmt.Sprintf("Session '%s' registered and active", workSession.Name),
		Data: map[string]interface{}{
			"action": "session_registered",
			"phase":  workSession.Phase,
		},
		Timestamp: time.Now(),
	}
	taskContext.CoordinationLog = append(taskContext.CoordinationLog, event)

	log.Info().
		Str("spec_task_id", workSession.SpecTaskID).
		Str("work_session_id", workSession.ID).
		Str("name", workSession.Name).
		Msg("Registered work session in SpecTask context")

	return nil
}

// UpdateSessionStatus updates the status of a session in the context
func (s *SessionContextService) UpdateSessionStatus(
	ctx context.Context,
	workSessionID string,
	status types.SpecTaskWorkSessionStatus,
	progress float64,
) error {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	// Find the context containing this session
	var taskContext *SpecTaskSessionContext
	var specTaskID string

	for id, context := range s.contextCache {
		if _, exists := context.SessionRegistry[workSessionID]; exists {
			taskContext = context
			specTaskID = id
			break
		}
	}

	if taskContext == nil {
		return fmt.Errorf("session context not found for work session: %s", workSessionID)
	}

	taskContext.Mutex.Lock()
	defer taskContext.Mutex.Unlock()

	// Update session info
	sessionInfo := taskContext.SessionRegistry[workSessionID]
	oldStatus := sessionInfo.Status
	sessionInfo.Status = string(status)
	sessionInfo.Progress = progress
	sessionInfo.LastActivity = time.Now()

	// Update counters
	s.updateSessionCounters(taskContext)

	// Log coordination event for status change
	event := CoordinationEvent{
		ID:            generateEventID(),
		FromSessionID: workSessionID,
		EventType:     CoordinationEventTypeNotification,
		Message:       fmt.Sprintf("Session status changed: %s → %s", oldStatus, status),
		Data: map[string]interface{}{
			"action":     "status_change",
			"old_status": oldStatus,
			"new_status": string(status),
			"progress":   progress,
		},
		Timestamp: time.Now(),
	}
	taskContext.CoordinationLog = append(taskContext.CoordinationLog, event)

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("work_session_id", workSessionID).
		Str("old_status", oldStatus).
		Str("new_status", string(status)).
		Float64("progress", progress).
		Msg("Updated session status in context")

	return nil
}

// SendCoordinationMessage sends a coordination message between sessions
func (s *SessionContextService) SendCoordinationMessage(
	ctx context.Context,
	fromSessionID string,
	toSessionID string, // Empty for broadcast
	eventType CoordinationEventType,
	message string,
	data map[string]interface{},
) error {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	// Find the context containing the from session
	var taskContext *SpecTaskSessionContext
	var specTaskID string

	for id, context := range s.contextCache {
		if _, exists := context.SessionRegistry[fromSessionID]; exists {
			taskContext = context
			specTaskID = id
			break
		}
	}

	if taskContext == nil {
		return fmt.Errorf("session context not found for session: %s", fromSessionID)
	}

	taskContext.Mutex.Lock()
	defer taskContext.Mutex.Unlock()

	// Create coordination event
	event := CoordinationEvent{
		ID:            generateEventID(),
		FromSessionID: fromSessionID,
		ToSessionID:   toSessionID,
		EventType:     eventType,
		Message:       message,
		Data:          data,
		Timestamp:     time.Now(),
		Acknowledged:  false,
	}

	// Add to coordination log
	taskContext.CoordinationLog = append(taskContext.CoordinationLog, event)

	// Update last activity for the from session
	if sessionInfo, exists := taskContext.SessionRegistry[fromSessionID]; exists {
		sessionInfo.LastActivity = time.Now()
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("from_session_id", fromSessionID).
		Str("to_session_id", toSessionID).
		Str("event_type", string(eventType)).
		Str("message", message).
		Msg("Sent coordination message")

	return nil
}

// GetSpecTaskContext returns the full context for a SpecTask
func (s *SessionContextService) GetSpecTaskContext(
	ctx context.Context,
	specTaskID string,
) (*SpecTaskSessionContext, error) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	taskContext, exists := s.contextCache[specTaskID]
	if !exists {
		return s.loadContextFromStore(ctx, specTaskID)
	}

	// Return a copy to avoid external mutations
	taskContext.Mutex.RLock()
	defer taskContext.Mutex.RUnlock()

	return s.copyContext(taskContext), nil
}

// GetSessionCoordination returns coordination events for a specific session
func (s *SessionContextService) GetSessionCoordination(
	ctx context.Context,
	workSessionID string,
) ([]CoordinationEvent, error) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	// Find the context containing this session
	var taskContext *SpecTaskSessionContext

	for _, context := range s.contextCache {
		if _, exists := context.SessionRegistry[workSessionID]; exists {
			taskContext = context
			break
		}
	}

	if taskContext == nil {
		return nil, fmt.Errorf("session context not found for work session: %s", workSessionID)
	}

	taskContext.Mutex.RLock()
	defer taskContext.Mutex.RUnlock()

	// Filter coordination events for this session
	var sessionEvents []CoordinationEvent
	for _, event := range taskContext.CoordinationLog {
		if event.FromSessionID == workSessionID || event.ToSessionID == workSessionID || event.ToSessionID == "" {
			sessionEvents = append(sessionEvents, event)
		}
	}

	return sessionEvents, nil
}

// UpdateSharedState updates shared state that's accessible to all sessions in the SpecTask
func (s *SessionContextService) UpdateSharedState(
	ctx context.Context,
	specTaskID string,
	key string,
	value interface{},
	updatedBySessionID string,
) error {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	taskContext := s.getOrCreateContext(specTaskID)
	taskContext.Mutex.Lock()
	defer taskContext.Mutex.Unlock()

	// Update shared state
	taskContext.SharedState[key] = value
	taskContext.LastUpdated = time.Now()

	// Log coordination event
	event := CoordinationEvent{
		ID:            generateEventID(),
		FromSessionID: updatedBySessionID,
		EventType:     CoordinationEventTypeBroadcast,
		Message:       fmt.Sprintf("Updated shared state: %s", key),
		Data: map[string]interface{}{
			"action": "shared_state_update",
			"key":    key,
			"value":  value,
		},
		Timestamp: time.Now(),
	}
	taskContext.CoordinationLog = append(taskContext.CoordinationLog, event)

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("key", key).
		Str("updated_by", updatedBySessionID).
		Msg("Updated shared state")

	return nil
}

// GetSharedState retrieves shared state for a SpecTask
func (s *SessionContextService) GetSharedState(
	ctx context.Context,
	specTaskID string,
	key string,
) (interface{}, error) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	taskContext, exists := s.contextCache[specTaskID]
	if !exists {
		return nil, fmt.Errorf("context not found for SpecTask: %s", specTaskID)
	}

	taskContext.Mutex.RLock()
	defer taskContext.Mutex.RUnlock()

	value, exists := taskContext.SharedState[key]
	if !exists {
		return nil, fmt.Errorf("shared state key not found: %s", key)
	}

	return value, nil
}

// AcknowledgeCoordinationEvent acknowledges a coordination event
func (s *SessionContextService) AcknowledgeCoordinationEvent(
	ctx context.Context,
	eventID string,
	acknowledgingSessionID string,
	response string,
) error {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	// Find the context containing this event
	for _, context := range s.contextCache {
		context.Mutex.Lock()
		for i := range context.CoordinationLog {
			if context.CoordinationLog[i].ID == eventID {
				// Found the event
				context.CoordinationLog[i].Acknowledged = true
				now := time.Now()
				context.CoordinationLog[i].AcknowledgedAt = &now
				context.CoordinationLog[i].Response = response

				log.Info().
					Str("event_id", eventID).
					Str("acknowledging_session", acknowledgingSessionID).
					Str("response", response).
					Msg("Acknowledged coordination event")

				context.Mutex.Unlock()
				return nil
			}
		}
		context.Mutex.Unlock()
	}

	return fmt.Errorf("coordination event not found: %s", eventID)
}

// NotifySessionCompletion notifies all other sessions when a session completes
func (s *SessionContextService) NotifySessionCompletion(
	ctx context.Context,
	completedSessionID string,
	completionSummary string,
	nextSteps string,
) error {
	return s.SendCoordinationMessage(
		ctx,
		completedSessionID,
		"", // Broadcast to all sessions
		CoordinationEventTypeCompletion,
		fmt.Sprintf("Session completed: %s", completionSummary),
		map[string]interface{}{
			"action":             "session_completed",
			"completion_summary": completionSummary,
			"next_steps":         nextSteps,
			"completed_at":       time.Now(),
		},
	)
}

// NotifySessionSpawn notifies sessions about new session spawning
func (s *SessionContextService) NotifySessionSpawn(
	ctx context.Context,
	parentSessionID string,
	spawnedSessionID string,
	spawnReason string,
) error {
	return s.SendCoordinationMessage(
		ctx,
		parentSessionID,
		"", // Broadcast to all sessions
		CoordinationEventTypeSpawn,
		fmt.Sprintf("Spawned new session: %s", spawnReason),
		map[string]interface{}{
			"action":             "session_spawned",
			"parent_session_id":  parentSessionID,
			"spawned_session_id": spawnedSessionID,
			"spawn_reason":       spawnReason,
			"spawned_at":         time.Now(),
		},
	)
}

// GetActiveSessionsInfo returns information about all active sessions in a SpecTask
func (s *SessionContextService) GetActiveSessionsInfo(
	ctx context.Context,
	specTaskID string,
) (map[string]*SessionInfo, error) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	taskContext, exists := s.contextCache[specTaskID]
	if !exists {
		return nil, fmt.Errorf("context not found for SpecTask: %s", specTaskID)
	}

	taskContext.Mutex.RLock()
	defer taskContext.Mutex.RUnlock()

	// Return copy of active sessions
	activeSessions := make(map[string]*SessionInfo)
	for sessionID, sessionInfo := range taskContext.SessionRegistry {
		if sessionInfo.Status == string(types.SpecTaskWorkSessionStatusActive) {
			// Create a copy
			copy := *sessionInfo
			activeSessions[sessionID] = &copy
		}
	}

	return activeSessions, nil
}

// CleanupContext removes expired contexts and old coordination events
func (s *SessionContextService) CleanupContext(ctx context.Context, maxAge time.Duration) error {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	now := time.Now()
	cleanedContexts := 0
	cleanedEvents := 0

	for specTaskID, taskContext := range s.contextCache {
		taskContext.Mutex.Lock()

		// Check if context is old and has no active sessions
		if now.Sub(taskContext.LastUpdated) > maxAge && taskContext.ActiveSessions == 0 {
			delete(s.contextCache, specTaskID)
			cleanedContexts++
			taskContext.Mutex.Unlock()
			continue
		}

		// Clean up old coordination events
		var activeEvents []CoordinationEvent
		for _, event := range taskContext.CoordinationLog {
			if now.Sub(event.Timestamp) <= maxAge {
				activeEvents = append(activeEvents, event)
			} else {
				cleanedEvents++
			}
		}
		taskContext.CoordinationLog = activeEvents

		taskContext.Mutex.Unlock()
	}

	if cleanedContexts > 0 || cleanedEvents > 0 {
		log.Info().
			Int("cleaned_contexts", cleanedContexts).
			Int("cleaned_events", cleanedEvents).
			Msg("Cleaned up session contexts")
	}

	return nil
}

// GetCoordinationSummary returns a summary of coordination activity for a SpecTask
func (s *SessionContextService) GetCoordinationSummary(
	ctx context.Context,
	specTaskID string,
) (*CoordinationSummary, error) {
	s.cacheMutex.RLock()
	defer s.cacheMutex.RUnlock()

	taskContext, exists := s.contextCache[specTaskID]
	if !exists {
		// Return empty summary for new spec tasks without any sessions yet
		return &CoordinationSummary{
			SpecTaskID:        specTaskID,
			TotalSessions:     0,
			ActiveSessions:    0,
			CompletedSessions: 0,
			TotalEvents:       0,
			EventsByType:      make(map[CoordinationEventType]int),
			RecentEvents:      []CoordinationEvent{},
			LastActivity:      time.Now(),
		}, nil
	}

	taskContext.Mutex.RLock()
	defer taskContext.Mutex.RUnlock()

	// Calculate summary statistics
	summary := &CoordinationSummary{
		SpecTaskID:        specTaskID,
		TotalSessions:     len(taskContext.SessionRegistry),
		ActiveSessions:    taskContext.ActiveSessions,
		CompletedSessions: taskContext.CompletedSessions,
		TotalEvents:       len(taskContext.CoordinationLog),
		LastActivity:      taskContext.LastUpdated,
	}

	// Count events by type
	summary.EventsByType = make(map[CoordinationEventType]int)
	for _, event := range taskContext.CoordinationLog {
		summary.EventsByType[event.EventType]++
	}

	// Recent events (last 10)
	recentCount := 10
	if len(taskContext.CoordinationLog) < recentCount {
		recentCount = len(taskContext.CoordinationLog)
	}
	if recentCount > 0 {
		startIndex := len(taskContext.CoordinationLog) - recentCount
		summary.RecentEvents = taskContext.CoordinationLog[startIndex:]
	}

	return summary, nil
}

// Private helper methods

func (s *SessionContextService) getOrCreateContext(specTaskID string) *SpecTaskSessionContext {
	context, exists := s.contextCache[specTaskID]
	if !exists {
		context = &SpecTaskSessionContext{
			SpecTaskID:      specTaskID,
			SharedState:     make(map[string]interface{}),
			SessionRegistry: make(map[string]*SessionInfo),
			CoordinationLog: make([]CoordinationEvent, 0),
			LastUpdated:     time.Now(),
		}
		s.contextCache[specTaskID] = context
	}
	return context
}

func (s *SessionContextService) updateSessionCounters(taskContext *SpecTaskSessionContext) {
	activeCount := 0
	completedCount := 0

	for _, sessionInfo := range taskContext.SessionRegistry {
		switch sessionInfo.Status {
		case string(types.SpecTaskWorkSessionStatusActive):
			activeCount++
		case string(types.SpecTaskWorkSessionStatusCompleted):
			completedCount++
		}
	}

	taskContext.ActiveSessions = activeCount
	taskContext.CompletedSessions = completedCount
	taskContext.LastUpdated = time.Now()
}

func (s *SessionContextService) copyContext(original *SpecTaskSessionContext) *SpecTaskSessionContext {
	contextCopy := &SpecTaskSessionContext{
		SpecTaskID:        original.SpecTaskID,
		SharedState:       make(map[string]interface{}),
		SessionRegistry:   make(map[string]*SessionInfo),
		CoordinationLog:   make([]CoordinationEvent, len(original.CoordinationLog)),
		LastUpdated:       original.LastUpdated,
		ActiveSessions:    original.ActiveSessions,
		CompletedSessions: original.CompletedSessions,
	}

	// Deep copy shared state
	for k, v := range original.SharedState {
		contextCopy.SharedState[k] = v
	}

	// Deep copy session registry
	for k, v := range original.SessionRegistry {
		sessionCopy := *v
		contextCopy.SessionRegistry[k] = &sessionCopy
	}

	// Copy coordination log
	copy(contextCopy.CoordinationLog, original.CoordinationLog)

	return contextCopy
}

func (s *SessionContextService) loadContextFromStore(ctx context.Context, specTaskID string) (*SpecTaskSessionContext, error) {
	// Load work sessions from store to rebuild context
	workSessions, err := s.store.ListSpecTaskWorkSessions(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to load work sessions: %w", err)
	}

	// Create new context
	taskContext := s.getOrCreateContext(specTaskID)
	taskContext.Mutex.Lock()
	defer taskContext.Mutex.Unlock()

	// Populate session registry from database
	for _, ws := range workSessions {
		sessionInfo := &SessionInfo{
			WorkSessionID:           ws.ID,
			HelixSessionID:          ws.HelixSessionID,
			Name:                    ws.Name,
			Status:                  string(ws.Status),
			Phase:                   string(ws.Phase),
			ImplementationTaskIndex: ws.ImplementationTaskIndex,
			LastActivity:            ws.UpdatedAt,
			Progress:                0.0,
		}

		if ws.StartedAt != nil {
			sessionInfo.StartedAt = ws.StartedAt
		}

		taskContext.SessionRegistry[ws.ID] = sessionInfo
	}

	// Update counters
	s.updateSessionCounters(taskContext)

	return s.copyContext(taskContext), nil
}

// Supporting types

// CoordinationSummary provides a summary of coordination activity
type CoordinationSummary struct {
	SpecTaskID        string                        `json:"spec_task_id"`
	TotalSessions     int                           `json:"total_sessions"`
	ActiveSessions    int                           `json:"active_sessions"`
	CompletedSessions int                           `json:"completed_sessions"`
	TotalEvents       int                           `json:"total_events"`
	EventsByType      map[CoordinationEventType]int `json:"events_by_type"`
	RecentEvents      []CoordinationEvent           `json:"recent_events"`
	LastActivity      time.Time                     `json:"last_activity"`
}

// Utility functions

func generateEventID() string {
	return fmt.Sprintf("coord_%d_%d", time.Now().UnixNano(), time.Now().Unix())
}

// Integration methods for other services

// OnWorkSessionCreated should be called when a work session is created
func (s *SessionContextService) OnWorkSessionCreated(ctx context.Context, workSession *types.SpecTaskWorkSession) {
	err := s.RegisterSession(ctx, workSession)
	if err != nil {
		log.Error().Err(err).
			Str("work_session_id", workSession.ID).
			Str("spec_task_id", workSession.SpecTaskID).
			Msg("Failed to register work session in context")
	}
}

// OnWorkSessionStatusChanged should be called when work session status changes
func (s *SessionContextService) OnWorkSessionStatusChanged(
	ctx context.Context,
	workSessionID string,
	status types.SpecTaskWorkSessionStatus,
	progress float64,
) {
	err := s.UpdateSessionStatus(ctx, workSessionID, status, progress)
	if err != nil {
		log.Error().Err(err).
			Str("work_session_id", workSessionID).
			Str("status", string(status)).
			Msg("Failed to update session status in context")
	}
}

// OnWorkSessionSpawned should be called when a work session spawns another session
func (s *SessionContextService) OnWorkSessionSpawned(
	ctx context.Context,
	parentSessionID string,
	spawnedSessionID string,
	reason string,
) {
	err := s.NotifySessionSpawn(ctx, parentSessionID, spawnedSessionID, reason)
	if err != nil {
		log.Error().Err(err).
			Str("parent_session_id", parentSessionID).
			Str("spawned_session_id", spawnedSessionID).
			Msg("Failed to notify session spawn")
	}
}

// OnWorkSessionCompleted should be called when a work session completes
func (s *SessionContextService) OnWorkSessionCompleted(
	ctx context.Context,
	completedSessionID string,
	summary string,
	nextSteps string,
) {
	err := s.NotifySessionCompletion(ctx, completedSessionID, summary, nextSteps)
	if err != nil {
		log.Error().Err(err).
			Str("completed_session_id", completedSessionID).
			Msg("Failed to notify session completion")
	}
}
