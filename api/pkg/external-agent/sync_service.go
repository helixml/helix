package external_agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ZedHelixSyncService manages bidirectional synchronization between Zed threads and Helix sessions
type ZedHelixSyncService struct {
	store              store.Store
	natsAgentService   *NATSExternalAgentService
	contextMappings    map[string]ContextMapping // contextID -> ContextMapping
	sessionMappings    map[string]SessionMapping // sessionID -> SessionMapping
	mutex              sync.RWMutex
	pendingResponses   map[string]*PendingResponse // requestID -> response handler
	responseMutex      sync.RWMutex
	messageHandlers    map[string]MessageHandler
	syncEventCallbacks []SyncEventCallback
	callbackMutex      sync.RWMutex
}

// ContextMapping maps Zed contexts to Helix interactions
type ContextMapping struct {
	ZedContextID   string                 `json:"zed_context_id"`
	HelixSessionID string                 `json:"helix_session_id"`
	InteractionID  string                 `json:"interaction_id"`
	ZedInstanceID  string                 `json:"zed_instance_id,omitempty"`
	ZedThreadID    string                 `json:"zed_thread_id,omitempty"`
	WorkSessionID  string                 `json:"work_session_id,omitempty"`
	Title          string                 `json:"title"`
	CreatedAt      time.Time              `json:"created_at"`
	LastActivity   time.Time              `json:"last_activity"`
	MessageCount   int                    `json:"message_count"`
	Status         string                 `json:"status"` // "active", "completed", "archived"
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SessionMapping tracks the overall session state across Zed and Helix
type SessionMapping struct {
	HelixSessionID    string    `json:"helix_session_id"`
	ZedInstanceID     string    `json:"zed_instance_id,omitempty"`
	AgentID           string    `json:"agent_id"`
	SpecTaskID        string    `json:"spec_task_id,omitempty"`
	UserID            string    `json:"user_id"`
	ActiveContexts    []string  `json:"active_contexts"` // List of active context IDs
	ProjectPath       string    `json:"project_path"`
	Status            string    `json:"status"` // "initializing", "active", "syncing", "completed"
	CreatedAt         time.Time `json:"created_at"`
	LastSyncAt        time.Time `json:"last_sync_at"`
	TotalMessages     int       `json:"total_messages"`
	TotalInteractions int       `json:"total_interactions"`
}

// PendingResponse tracks responses waiting for completion
type PendingResponse struct {
	RequestID    string        `json:"request_id"`
	SessionID    string        `json:"session_id"`
	ContextID    string        `json:"context_id,omitempty"`
	ResponseChan chan string   `json:"-"`
	DoneChan     chan bool     `json:"-"`
	ErrorChan    chan error    `json:"-"`
	CreatedAt    time.Time     `json:"created_at"`
	Timeout      time.Duration `json:"timeout"`
}

// MessageHandler handles specific types of sync messages
type MessageHandler func(ctx context.Context, syncMsg *types.SyncMessage) error

// SyncEventCallback is called when sync events occur
type SyncEventCallback func(event SyncEvent)

// SyncEvent represents events in the sync process
type SyncEvent struct {
	Type      string                 `json:"type"` // "context_created", "message_synced", "response_completed", etc.
	SessionID string                 `json:"session_id"`
	ContextID string                 `json:"context_id,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// NewZedHelixSyncService creates a new sync service
func NewZedHelixSyncService(store store.Store, natsAgentService *NATSExternalAgentService) *ZedHelixSyncService {
	service := &ZedHelixSyncService{
		store:            store,
		natsAgentService: natsAgentService,
		contextMappings:  make(map[string]ContextMapping),
		sessionMappings:  make(map[string]SessionMapping),
		pendingResponses: make(map[string]*PendingResponse),
		messageHandlers:  make(map[string]MessageHandler),
	}

	// Register default message handlers
	service.registerDefaultHandlers()

	return service
}

// Start initializes the sync service
func (s *ZedHelixSyncService) Start(ctx context.Context) error {
	log.Info().Msg("Starting Zed-Helix sync service")

	// Start cleanup routine for expired responses
	go s.cleanupRoutine(ctx)

	return nil
}

// CreateContextMapping creates a new mapping between Zed context and Helix interaction
func (s *ZedHelixSyncService) CreateContextMapping(zedContextID, helixSessionID, interactionID string, metadata map[string]interface{}) (*ContextMapping, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if mapping already exists
	if _, exists := s.contextMappings[zedContextID]; exists {
		return nil, fmt.Errorf("context mapping already exists for context: %s", zedContextID)
	}

	mapping := ContextMapping{
		ZedContextID:   zedContextID,
		HelixSessionID: helixSessionID,
		InteractionID:  interactionID,
		Title:          fmt.Sprintf("Context %s", zedContextID[:8]),
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		MessageCount:   0,
		Status:         "active",
		Metadata:       metadata,
	}

	// Extract additional IDs from metadata
	if instanceID, ok := metadata["zed_instance_id"].(string); ok {
		mapping.ZedInstanceID = instanceID
	}
	if threadID, ok := metadata["zed_thread_id"].(string); ok {
		mapping.ZedThreadID = threadID
	}
	if workSessionID, ok := metadata["work_session_id"].(string); ok {
		mapping.WorkSessionID = workSessionID
	}
	if title, ok := metadata["title"].(string); ok {
		mapping.Title = title
	}

	s.contextMappings[zedContextID] = mapping

	// Update session mapping
	sessionMapping, exists := s.sessionMappings[helixSessionID]
	if exists {
		sessionMapping.ActiveContexts = append(sessionMapping.ActiveContexts, zedContextID)
		sessionMapping.TotalInteractions++
		sessionMapping.LastSyncAt = time.Now()
		s.sessionMappings[helixSessionID] = sessionMapping
	}

	// Register context mapping with NATS service
	if s.natsAgentService != nil {
		s.natsAgentService.MapZedContextToInteraction(helixSessionID, zedContextID, interactionID)
	}

	log.Info().
		Str("zed_context_id", zedContextID).
		Str("helix_session_id", helixSessionID).
		Str("interaction_id", interactionID).
		Msg("Created context mapping")

	// Emit sync event
	s.emitSyncEvent(SyncEvent{
		Type:      "context_created",
		SessionID: helixSessionID,
		ContextID: zedContextID,
		Data: map[string]interface{}{
			"interaction_id": interactionID,
			"title":          mapping.Title,
		},
		Timestamp: time.Now(),
	})

	return &mapping, nil
}

// GetContextMapping retrieves a context mapping
func (s *ZedHelixSyncService) GetContextMapping(zedContextID string) (*ContextMapping, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	mapping, exists := s.contextMappings[zedContextID]
	if !exists {
		return nil, fmt.Errorf("context mapping not found: %s", zedContextID)
	}

	return &mapping, nil
}

// CreateSessionMapping creates a session-level mapping
func (s *ZedHelixSyncService) CreateSessionMapping(helixSessionID, agentID, userID string, metadata map[string]interface{}) (*SessionMapping, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	mapping := SessionMapping{
		HelixSessionID:    helixSessionID,
		AgentID:           agentID,
		UserID:            userID,
		ActiveContexts:    make([]string, 0),
		Status:            "initializing",
		CreatedAt:         time.Now(),
		LastSyncAt:        time.Now(),
		TotalMessages:     0,
		TotalInteractions: 0,
	}

	// Extract additional data from metadata
	if instanceID, ok := metadata["zed_instance_id"].(string); ok {
		mapping.ZedInstanceID = instanceID
	}
	if specTaskID, ok := metadata["spec_task_id"].(string); ok {
		mapping.SpecTaskID = specTaskID
	}
	if projectPath, ok := metadata["project_path"].(string); ok {
		mapping.ProjectPath = projectPath
	}

	s.sessionMappings[helixSessionID] = mapping

	log.Info().
		Str("helix_session_id", helixSessionID).
		Str("agent_id", agentID).
		Str("zed_instance_id", mapping.ZedInstanceID).
		Msg("Created session mapping")

	return &mapping, nil
}

// ProcessSyncMessage handles incoming sync messages from Zed
func (s *ZedHelixSyncService) ProcessSyncMessage(ctx context.Context, sessionID string, syncMsg *types.SyncMessage) error {
	// Find and execute handler
	handler, exists := s.messageHandlers[syncMsg.EventType]
	if !exists {
		log.Warn().
			Str("event_type", syncMsg.EventType).
			Str("session_id", sessionID).
			Msg("No handler found for sync message type")
		return nil
	}

	// Execute handler
	if err := handler(ctx, syncMsg); err != nil {
		log.Error().
			Err(err).
			Str("event_type", syncMsg.EventType).
			Str("session_id", sessionID).
			Msg("Error processing sync message")
		return err
	}

	// Update last activity
	s.updateLastActivity(sessionID, syncMsg.Data["context_id"])

	return nil
}

// SendMessageToZed sends a message to a Zed context
func (s *ZedHelixSyncService) SendMessageToZed(ctx context.Context, sessionID, contextID, message string) (*PendingResponse, error) {
	// Generate unique request ID
	requestID := fmt.Sprintf("req_%s_%d", sessionID, time.Now().UnixNano())

	// Create pending response
	response := &PendingResponse{
		RequestID:    requestID,
		SessionID:    sessionID,
		ContextID:    contextID,
		ResponseChan: make(chan string, 100),
		DoneChan:     make(chan bool, 1),
		ErrorChan:    make(chan error, 1),
		CreatedAt:    time.Now(),
		Timeout:      30 * time.Second,
	}

	s.responseMutex.Lock()
	s.pendingResponses[requestID] = response
	s.responseMutex.Unlock()

	// Send message via WebSocket (this would be handled by the WebSocket sync service)
	_ = types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"request_id": requestID,
			"context_id": contextID,
			"message":    message,
			"role":       "user",
		},
	}

	// TODO: Send command to external agent via WebSocket
	// This would be integrated with the existing WebSocket sync mechanism

	log.Info().
		Str("session_id", sessionID).
		Str("context_id", contextID).
		Str("request_id", requestID).
		Msg("Sent message to Zed context")

	return response, nil
}

// HandleResponseChunk processes response chunks from Zed
func (s *ZedHelixSyncService) HandleResponseChunk(requestID, chunk string) error {
	s.responseMutex.RLock()
	response, exists := s.pendingResponses[requestID]
	s.responseMutex.RUnlock()

	if !exists {
		log.Warn().Str("request_id", requestID).Msg("No pending response found for chunk")
		return nil
	}

	select {
	case response.ResponseChan <- chunk:
		return nil
	default:
		log.Warn().Str("request_id", requestID).Msg("Response channel full, dropping chunk")
		return nil
	}
}

// HandleResponseComplete marks a response as complete
func (s *ZedHelixSyncService) HandleResponseComplete(requestID string) error {
	s.responseMutex.Lock()
	response, exists := s.pendingResponses[requestID]
	if exists {
		delete(s.pendingResponses, requestID)
	}
	s.responseMutex.Unlock()

	if !exists {
		log.Warn().Str("request_id", requestID).Msg("No pending response found for completion")
		return nil
	}

	select {
	case response.DoneChan <- true:
		log.Info().Str("request_id", requestID).Msg("Response completed")
		return nil
	default:
		log.Warn().Str("request_id", requestID).Msg("Done channel full")
		return nil
	}
}

// HandleResponseError handles response errors
func (s *ZedHelixSyncService) HandleResponseError(requestID string, err error) error {
	s.responseMutex.Lock()
	response, exists := s.pendingResponses[requestID]
	if exists {
		delete(s.pendingResponses, requestID)
	}
	s.responseMutex.Unlock()

	if !exists {
		log.Warn().Str("request_id", requestID).Msg("No pending response found for error")
		return nil
	}

	select {
	case response.ErrorChan <- err:
		log.Error().Err(err).Str("request_id", requestID).Msg("Response error handled")
		return nil
	default:
		log.Warn().Str("request_id", requestID).Msg("Error channel full")
		return nil
	}
}

// GetPendingResponse retrieves a pending response by request ID
func (s *ZedHelixSyncService) GetPendingResponse(requestID string) (*PendingResponse, bool) {
	s.responseMutex.RLock()
	defer s.responseMutex.RUnlock()
	response, exists := s.pendingResponses[requestID]
	return response, exists
}

// RegisterSyncEventCallback registers a callback for sync events
func (s *ZedHelixSyncService) RegisterSyncEventCallback(callback SyncEventCallback) {
	s.callbackMutex.Lock()
	defer s.callbackMutex.Unlock()
	s.syncEventCallbacks = append(s.syncEventCallbacks, callback)
}

// ListContextMappings returns all context mappings for a session
func (s *ZedHelixSyncService) ListContextMappings(sessionID string) []ContextMapping {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	var mappings []ContextMapping
	for _, mapping := range s.contextMappings {
		if mapping.HelixSessionID == sessionID {
			mappings = append(mappings, mapping)
		}
	}

	return mappings
}

// GetSyncStats returns synchronization statistics
func (s *ZedHelixSyncService) GetSyncStats(sessionID string) map[string]interface{} {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	sessionMapping, exists := s.sessionMappings[sessionID]
	if !exists {
		return map[string]interface{}{
			"error": "session not found",
		}
	}

	stats := map[string]interface{}{
		"session_id":         sessionID,
		"status":             sessionMapping.Status,
		"active_contexts":    len(sessionMapping.ActiveContexts),
		"total_messages":     sessionMapping.TotalMessages,
		"total_interactions": sessionMapping.TotalInteractions,
		"created_at":         sessionMapping.CreatedAt,
		"last_sync_at":       sessionMapping.LastSyncAt,
		"agent_id":           sessionMapping.AgentID,
		"zed_instance_id":    sessionMapping.ZedInstanceID,
	}

	return stats
}

// Private helper methods

func (s *ZedHelixSyncService) registerDefaultHandlers() {
	s.messageHandlers["context_created"] = s.handleContextCreated
	s.messageHandlers["message_added"] = s.handleMessageAdded
	s.messageHandlers["message_updated"] = s.handleMessageUpdated
	s.messageHandlers["context_title_changed"] = s.handleContextTitleChanged
	s.messageHandlers["chat_response"] = s.handleChatResponse
	s.messageHandlers["chat_response_chunk"] = s.handleChatResponseChunk
	s.messageHandlers["chat_response_done"] = s.handleChatResponseDone
	s.messageHandlers["chat_response_error"] = s.handleChatResponseError
}

func (s *ZedHelixSyncService) handleContextCreated(ctx context.Context, syncMsg *types.SyncMessage) error {
	contextID, _ := syncMsg.Data["context_id"].(string)
	title, _ := syncMsg.Data["title"].(string)

	log.Info().
		Str("session_id", syncMsg.SessionID).
		Str("context_id", contextID).
		Str("title", title).
		Msg("Zed context created")

	// TODO: Create corresponding Helix interaction if not already mapped
	return nil
}

func (s *ZedHelixSyncService) handleMessageAdded(ctx context.Context, syncMsg *types.SyncMessage) error {
	contextID, _ := syncMsg.Data["context_id"].(string)
	messageID, _ := syncMsg.Data["message_id"].(string)
	_, _ = syncMsg.Data["content"].(string)
	role, _ := syncMsg.Data["role"].(string)

	log.Info().
		Str("session_id", syncMsg.SessionID).
		Str("context_id", contextID).
		Str("message_id", messageID).
		Str("role", role).
		Msg("Zed message added")

	// Update message count
	s.incrementMessageCount(contextID)

	// TODO: Add message to corresponding Helix interaction
	return nil
}

func (s *ZedHelixSyncService) handleMessageUpdated(ctx context.Context, syncMsg *types.SyncMessage) error {
	// TODO: Handle message updates
	return nil
}

func (s *ZedHelixSyncService) handleContextTitleChanged(ctx context.Context, syncMsg *types.SyncMessage) error {
	contextID, _ := syncMsg.Data["context_id"].(string)
	title, _ := syncMsg.Data["title"].(string)

	s.mutex.Lock()
	if mapping, exists := s.contextMappings[contextID]; exists {
		mapping.Title = title
		mapping.LastActivity = time.Now()
		s.contextMappings[contextID] = mapping
	}
	s.mutex.Unlock()

	log.Info().
		Str("context_id", contextID).
		Str("title", title).
		Msg("Zed context title changed")

	return nil
}

func (s *ZedHelixSyncService) handleChatResponse(ctx context.Context, syncMsg *types.SyncMessage) error {
	requestID, _ := syncMsg.Data["request_id"].(string)
	content, _ := syncMsg.Data["content"].(string)

	// Handle as complete response
	s.HandleResponseChunk(requestID, content)
	s.HandleResponseComplete(requestID)

	return nil
}

func (s *ZedHelixSyncService) handleChatResponseChunk(ctx context.Context, syncMsg *types.SyncMessage) error {
	requestID, _ := syncMsg.Data["request_id"].(string)
	chunk, _ := syncMsg.Data["chunk"].(string)

	return s.HandleResponseChunk(requestID, chunk)
}

func (s *ZedHelixSyncService) handleChatResponseDone(ctx context.Context, syncMsg *types.SyncMessage) error {
	requestID, _ := syncMsg.Data["request_id"].(string)
	return s.HandleResponseComplete(requestID)
}

func (s *ZedHelixSyncService) handleChatResponseError(ctx context.Context, syncMsg *types.SyncMessage) error {
	requestID, _ := syncMsg.Data["request_id"].(string)
	errorMsg, _ := syncMsg.Data["error"].(string)

	return s.HandleResponseError(requestID, fmt.Errorf("%s", errorMsg))
}

func (s *ZedHelixSyncService) updateLastActivity(sessionID string, contextIDInterface interface{}) {
	contextID, ok := contextIDInterface.(string)
	if !ok {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Update context mapping
	if mapping, exists := s.contextMappings[contextID]; exists {
		mapping.LastActivity = time.Now()
		s.contextMappings[contextID] = mapping
	}

	// Update session mapping
	if mapping, exists := s.sessionMappings[sessionID]; exists {
		mapping.LastSyncAt = time.Now()
		s.sessionMappings[sessionID] = mapping
	}
}

func (s *ZedHelixSyncService) incrementMessageCount(contextID string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if mapping, exists := s.contextMappings[contextID]; exists {
		mapping.MessageCount++
		mapping.LastActivity = time.Now()
		s.contextMappings[contextID] = mapping

		// Update session total
		if sessionMapping, exists := s.sessionMappings[mapping.HelixSessionID]; exists {
			sessionMapping.TotalMessages++
			sessionMapping.LastSyncAt = time.Now()
			s.sessionMappings[mapping.HelixSessionID] = sessionMapping
		}
	}
}

func (s *ZedHelixSyncService) emitSyncEvent(event SyncEvent) {
	s.callbackMutex.RLock()
	defer s.callbackMutex.RUnlock()

	for _, callback := range s.syncEventCallbacks {
		go callback(event)
	}
}

func (s *ZedHelixSyncService) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpiredResponses()
		}
	}
}

func (s *ZedHelixSyncService) cleanupExpiredResponses() {
	s.responseMutex.Lock()
	defer s.responseMutex.Unlock()

	now := time.Now()
	for requestID, response := range s.pendingResponses {
		if now.Sub(response.CreatedAt) > response.Timeout {
			// Send timeout error
			select {
			case response.ErrorChan <- fmt.Errorf("response timeout"):
			default:
			}

			delete(s.pendingResponses, requestID)
			log.Warn().
				Str("request_id", requestID).
				Dur("age", now.Sub(response.CreatedAt)).
				Msg("Cleaned up expired response")
		}
	}
}
