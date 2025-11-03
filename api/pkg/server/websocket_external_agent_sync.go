package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// External agent WebSocket connections
type ExternalAgentWSManager struct {
	connections map[string]*ExternalAgentWSConnection
	mu          sync.RWMutex
	upgrader    websocket.Upgrader
}

type ExternalAgentWSConnection struct {
	SessionID   string
	Conn        *websocket.Conn
	ConnectedAt time.Time
	LastPing    time.Time
	SendChan    chan types.ExternalAgentCommand
	mu          sync.Mutex
}

func NewExternalAgentWSManager() *ExternalAgentWSManager {
	return &ExternalAgentWSManager{
		connections: make(map[string]*ExternalAgentWSConnection),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // TODO: Add proper origin validation
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// External agent runner connection manager (tracks /ws/external-agent-runner connections)
type ExternalAgentRunnerManager struct {
	runnerConnections map[string][]*ExternalAgentRunnerConnection // map[runnerID][]connections
	mu                sync.RWMutex
}

type ExternalAgentRunnerConnection struct {
	ConnectionID string
	RunnerID     string
	ConnectedAt  time.Time
	LastPing     time.Time
	Concurrency  int
	Status       string
}

func NewExternalAgentRunnerManager() *ExternalAgentRunnerManager {
	return &ExternalAgentRunnerManager{
		runnerConnections: make(map[string][]*ExternalAgentRunnerConnection),
	}
}

// addConnection adds a runner connection with unique connection ID
func (manager *ExternalAgentRunnerManager) addConnection(runnerID string, concurrency int) string {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	now := time.Now()
	// Generate unique connection ID: runnerID + timestamp + microseconds
	connectionID := fmt.Sprintf("%s-%d-%d", runnerID, now.Unix(), now.Nanosecond()/1000)

	newConnection := &ExternalAgentRunnerConnection{
		ConnectionID: connectionID,
		RunnerID:     runnerID,
		ConnectedAt:  now,
		LastPing:     now,
		Concurrency:  concurrency,
		Status:       "connected",
	}

	// Add to runner's connection array
	manager.runnerConnections[runnerID] = append(manager.runnerConnections[runnerID], newConnection)

	log.Info().
		Str("runner_id", runnerID).
		Str("connection_id", connectionID).
		Int("concurrency", concurrency).
		Int("total_connections", len(manager.runnerConnections[runnerID])).
		Msg("🔗 External agent runner connection added to manager")

	return connectionID
}

// removeConnection removes a specific connection by runner ID and connection ID
func (manager *ExternalAgentRunnerManager) removeConnection(runnerID, connectionID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	// Find and remove the connection from the specific runner's array
	connections, exists := manager.runnerConnections[runnerID]
	if !exists {
		log.Warn().
			Str("runner_id", runnerID).
			Str("connection_id", connectionID).
			Msg("⚠️ Attempted to remove connection from non-existent runner")
		return
	}

	for i, conn := range connections {
		if conn.ConnectionID == connectionID {
			// Remove this connection from the slice
			manager.runnerConnections[runnerID] = append(connections[:i], connections[i+1:]...)

			// If no connections left for this runner, remove the runner entry
			if len(manager.runnerConnections[runnerID]) == 0 {
				delete(manager.runnerConnections, runnerID)
			}

			log.Info().
				Str("runner_id", runnerID).
				Str("connection_id", connectionID).
				Int("remaining_connections", len(manager.runnerConnections[runnerID])).
				Msg("🔌 External agent runner connection removed from manager")
			return
		}
	}

	log.Warn().
		Str("runner_id", runnerID).
		Str("connection_id", connectionID).
		Msg("⚠️ Attempted to remove non-existent connection")
}

// updatePingByRunner updates the last ping time for the most recent connection of a runner
func (manager *ExternalAgentRunnerManager) updatePingByRunner(runnerID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	connections, exists := manager.runnerConnections[runnerID]
	if !exists || len(connections) == 0 {
		log.Warn().
			Str("runner_id", runnerID).
			Msg("⚠️ Attempted to update ping for non-existent runner connection")
		return
	}

	// Find the most recent connection for this runner
	var mostRecentConn *ExternalAgentRunnerConnection
	var mostRecentTime time.Time

	for _, conn := range connections {
		if conn.ConnectedAt.After(mostRecentTime) {
			mostRecentConn = conn
			mostRecentTime = conn.ConnectedAt
		}
	}

	if mostRecentConn != nil {
		oldPing := mostRecentConn.LastPing
		mostRecentConn.LastPing = time.Now()

		log.Info().
			Str("runner_id", runnerID).
			Str("connection_id", mostRecentConn.ConnectionID).
			Time("old_ping", oldPing).
			Time("new_ping", mostRecentConn.LastPing).
			Dur("ping_interval", mostRecentConn.LastPing.Sub(oldPing)).
			Int("total_connections", len(connections)).
			Msg("🏓 External agent runner ping timestamp updated")
	}
}

// listConnections returns all active runner connections, one per runner (most recent per runner)
func (manager *ExternalAgentRunnerManager) listConnections() []types.ExternalAgentConnection {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	connections := make([]types.ExternalAgentConnection, 0, len(manager.runnerConnections))

	// For each runner, find the most recent connection and include it in the list
	for runnerID, runnerConns := range manager.runnerConnections {
		if len(runnerConns) == 0 {
			continue
		}

		// Find the most recent connection for this runner
		var mostRecentConn *ExternalAgentRunnerConnection
		var mostRecentTime time.Time

		for _, conn := range runnerConns {
			if conn.ConnectedAt.After(mostRecentTime) {
				mostRecentConn = conn
				mostRecentTime = conn.ConnectedAt
			}
		}

		if mostRecentConn != nil {
			connections = append(connections, types.ExternalAgentConnection{
				SessionID:   runnerID, // Use RunnerID as SessionID for consistency
				ConnectedAt: mostRecentConn.ConnectedAt,
				LastPing:    mostRecentConn.LastPing,
				Status:      mostRecentConn.Status,
			})
		}
	}

	return connections
}

// handleExternalAgentSync handles WebSocket connections from external agents (Zed instances)
func (apiServer *HelixAPIServer) handleExternalAgentSync(res http.ResponseWriter, req *http.Request) {
	log.Info().
		Str("method", req.Method).
		Str("url", req.URL.String()).
		Str("remote_addr", req.RemoteAddr).
		Msg("🔌 [HELIX] External agent WebSocket connection attempt")
	// Extract session ID from query parameters (checks both session_id and agent_id for compatibility)
	agentID := req.URL.Query().Get("session_id")
	if agentID == "" {
		agentID = req.URL.Query().Get("agent_id")
	}
	if agentID == "" {
		// Generate a unique agent ID for this connection
		agentID = fmt.Sprintf("external-agent-%d", time.Now().UnixNano())
		log.Info().Str("agent_id", agentID).Msg("Generated agent ID for external agent connection")
	}

	// Validate auth token
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(res, "Authorization header required", http.StatusUnauthorized)
		return
	}

	// Extract token from "Bearer <token>" format
	token := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}

	if !apiServer.validateExternalAgentToken(agentID, token) {
		http.Error(res, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := apiServer.externalAgentWSManager.upgrader.Upgrade(res, req, nil)
	if err != nil {
		log.Error().Err(err).Str("agent_id", agentID).Msg("Failed to upgrade WebSocket")
		return
	}

	log.Info().Str("agent_id", agentID).Msg("External agent WebSocket connected")

	// Create connection wrapper
	wsConn := &ExternalAgentWSConnection{
		SessionID:   agentID, // Using agent ID as connection identifier
		Conn:        conn,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
		SendChan:    make(chan types.ExternalAgentCommand, 100),
	}

	// Register connection with agent ID
	apiServer.externalAgentWSManager.registerConnection(agentID, wsConn)
	defer apiServer.externalAgentWSManager.unregisterConnection(agentID)

	// Check if this agent has a Helix session mapping
	// agentID could be either agent_session_id (req_*) or helix_session_id (ses_*)
	helixSessionID := ""
	if strings.HasPrefix(agentID, "ses_") {
		// Direct Helix session ID
		helixSessionID = agentID
		log.Info().
			Str("agent_session_id", agentID).
			Str("helix_session_id", helixSessionID).
			Msg("🚀 [HELIX] External agent connected with Helix session ID, checking for initial message")
	} else if mappedHelixID, exists := apiServer.externalAgentSessionMapping[agentID]; exists {
		// Agent session ID mapping - register connection with BOTH IDs for routing
		helixSessionID = mappedHelixID
		apiServer.externalAgentWSManager.registerConnection(helixSessionID, wsConn)
		defer apiServer.externalAgentWSManager.unregisterConnection(helixSessionID)
		log.Info().
			Str("agent_session_id", agentID).
			Str("helix_session_id", helixSessionID).
			Msg("🚀 [HELIX] External agent connected with agent session ID, registered with BOTH IDs for routing")
	}

	// Start goroutines for handling connection
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	// Start message sender
	go apiServer.handleExternalAgentSender(ctx, wsConn)

	if helixSessionID != "" {

		// Get the Helix session to find the initial interaction
		helixSession, err := apiServer.Controller.Options.Store.GetSession(ctx, helixSessionID)
		if err == nil && helixSession != nil {
			// Find the waiting interaction
			interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
				SessionID:    helixSessionID,
				GenerationID: helixSession.GenerationID,
				PerPage:      1000,
			})
			if err == nil && len(interactions) > 0 {
				// Find the most recent waiting interaction
				for i := len(interactions) - 1; i >= 0; i-- {
					if interactions[i].State == types.InteractionStateWaiting {
						// Found the initial message - send it to Zed
						// Find the request_id for this session
						var requestID string
						for rid, sid := range apiServer.requestToSessionMapping {
							if sid == helixSessionID {
								requestID = rid
								break
							}
						}

						if requestID != "" {
							// Combine system prompt and user message into a single message
							// This ensures Zed receives the planning instructions
							fullMessage := interactions[i].PromptMessage
							if interactions[i].SystemPrompt != "" {
								fullMessage = interactions[i].SystemPrompt + "\n\n**User Request:**\n" + interactions[i].PromptMessage
							}

							command := types.ExternalAgentCommand{
								Type: "chat_message",
								Data: map[string]interface{}{
									"message":       fullMessage,
									"request_id":    requestID,
									"acp_thread_id": nil, // null = create new thread
								},
							}

							// Send immediately via channel
							select {
							case wsConn.SendChan <- command:
								log.Info().
									Str("agent_session_id", agentID).
									Str("request_id", requestID).
									Str("helix_session_id", helixSessionID).
									Msg("✅ [HELIX] Sent initial chat_message to Zed")
							default:
								log.Warn().
									Str("agent_session_id", agentID).
									Msg("⚠️ [HELIX] SendChan full, could not send initial message")
							}
						} else {
							log.Warn().
								Str("helix_session_id", helixSessionID).
								Msg("⚠️ [HELIX] No request_id found for initial message")
						}
						break
					}
				}
			}
		}
	}

	// Handle incoming messages (blocking)
	apiServer.handleExternalAgentReceiver(ctx, wsConn)
}

// sendResponseToZed sends a response back to Zed via WebSocket
func (apiServer *HelixAPIServer) sendResponseToZed(sessionID, contextID, content string, isComplete bool) error {
	// Get the WebSocket connection for this session
	wsConn, exists := apiServer.externalAgentWSManager.getConnection(sessionID)
	if !exists || wsConn == nil {
		return fmt.Errorf("no WebSocket connection found for session %s", sessionID)
	}

	// Determine event type based on completion status
	eventType := "chat_response_chunk"
	if isComplete {
		eventType = "chat_response_done"
	}

	// Create command to send to Zed
	command := types.ExternalAgentCommand{
		Type: eventType,
		Data: map[string]interface{}{
			"context_id": contextID,
			"content":    content,
			"timestamp":  time.Now(),
		},
	}

	// Send the command
	select {
	case wsConn.SendChan <- command:
		log.Debug().
			Str("session_id", sessionID).
			Str("context_id", contextID).
			Str("event_type", eventType).
			Msg("Sent response to Zed")
		return nil
	default:
		return fmt.Errorf("failed to send response to Zed: channel full")
	}
}

// handleExternalAgentReceiver handles incoming messages from external agent
func (apiServer *HelixAPIServer) handleExternalAgentReceiver(ctx context.Context, wsConn *ExternalAgentWSConnection) {
	defer wsConn.Conn.Close()

	// Set read deadline and pong handler
	wsConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	wsConn.Conn.SetPongHandler(func(appData string) error {
		wsConn.LastPing = time.Now()
		wsConn.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		select {
		case <-ctx.Done():
			return
		default:
			var syncMsg types.SyncMessage
			if err := wsConn.Conn.ReadJSON(&syncMsg); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Error().Err(err).Str("session_id", wsConn.SessionID).Msg("WebSocket read error")
				}
				return
			}

			// Process sync message
			if err := apiServer.processExternalAgentSyncMessage(wsConn.SessionID, &syncMsg); err != nil {
				log.Error().Err(err).Str("session_id", wsConn.SessionID).Str("event_type", syncMsg.EventType).Msg("Failed to process sync message")
			}
		}
	}
}

// handleExternalAgentSender handles outgoing messages to external agent
func (apiServer *HelixAPIServer) handleExternalAgentSender(ctx context.Context, wsConn *ExternalAgentWSConnection) {
	ticker := time.NewTicker(54 * time.Second) // Ping every 54 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case command := <-wsConn.SendChan:
			wsConn.mu.Lock()
			if err := wsConn.Conn.WriteJSON(command); err != nil {
				log.Error().Err(err).Str("session_id", wsConn.SessionID).Msg("Failed to send command to external agent")
				wsConn.mu.Unlock()
				return
			}
			wsConn.mu.Unlock()
		case <-ticker.C:
			wsConn.mu.Lock()
			if err := wsConn.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Error().Err(err).Str("session_id", wsConn.SessionID).Msg("Failed to send ping")
				wsConn.mu.Unlock()
				return
			}
			wsConn.mu.Unlock()
		}
	}
}

// processExternalAgentSyncMessage processes incoming sync messages from external agents
func (apiServer *HelixAPIServer) processExternalAgentSyncMessage(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("agent_session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("🔄 [HELIX] PROCESSING MESSAGE FROM EXTERNAL AGENT")

	log.Debug().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Msg("Processing external agent sync message")

	// Update activity tracking to prevent idle timeout for active sessions
	// Get activity record to update last_interaction timestamp
	activity, err := apiServer.Store.GetExternalAgentActivity(context.Background(), sessionID)
	if err == nil && activity != nil {
		// Activity record exists - update it to extend idle timeout
		activity.LastInteraction = time.Now()
		err = apiServer.Store.UpsertExternalAgentActivity(context.Background(), activity)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to update activity for WebSocket message")
			// Non-fatal - continue processing message
		}
	}
	// If no activity record exists, that's OK - might be an old session or non-external-agent session

	// Process sync message directly
	switch syncMsg.EventType {
	case "thread_created":
		return apiServer.handleThreadCreated(sessionID, syncMsg)
	case "user_created_thread":
		return apiServer.handleUserCreatedThread(sessionID, syncMsg)
	case "thread_title_changed":
		return apiServer.handleThreadTitleChanged(sessionID, syncMsg)
	case "context_created": // Legacy support - redirect to thread_created
		return apiServer.handleThreadCreated(sessionID, syncMsg)
	case "message_added":
		return apiServer.handleMessageAdded(sessionID, syncMsg)
	case "message_updated":
		return apiServer.handleMessageUpdated(sessionID, syncMsg)
	case "context_title_changed":
		return apiServer.handleContextTitleChanged(sessionID, syncMsg)
	case "chat_response":
		return apiServer.handleChatResponse(sessionID, syncMsg)
	case "chat_response_chunk":
		return apiServer.handleChatResponseChunk(sessionID, syncMsg)
	case "chat_response_done":
		return apiServer.handleChatResponseDone(sessionID, syncMsg)
	case "message_completed":
		return apiServer.handleMessageCompleted(sessionID, syncMsg)
	case "chat_response_error":
		return apiServer.handleChatResponseError(sessionID, syncMsg)
	default:
		log.Warn().Str("event_type", syncMsg.EventType).Msg("Unknown sync message type")
		return nil
	}
}

// handleThreadCreated processes thread creation from external agent (new protocol)
func (apiServer *HelixAPIServer) handleThreadCreated(sessionID string, syncMsg *types.SyncMessage) error {
	// NEW PROTOCOL: use acp_thread_id
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok {
		// FALLBACK: try old context_id for compatibility
		acpThreadID, ok = syncMsg.Data["context_id"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid acp_thread_id/context_id")
		}
	}

	contextID := acpThreadID // Use contextID as alias for compatibility with rest of code

	title, _ := syncMsg.Data["title"].(string)
	if title == "" {
		title = "New Conversation"
	}

	// NEW PROTOCOL: Extract request_id for correlation
	requestID, _ := syncMsg.Data["request_id"].(string)

	// Check if this is a response to a Helix-initiated request
	// If syncMsg has a session_id, this is a response to an existing Helix session
	helixSessionID := syncMsg.SessionID

	log.Info().
		Str("agent_session_id", sessionID).
		Str("helix_session_id", helixSessionID).
		Str("acp_thread_id", acpThreadID).
		Str("request_id", requestID).
		Str("title", title).
		Msg("🔧 [HELIX] Processing thread_created from external agent")

	// PRIORITY 1: Check if request_id maps to an existing Helix session
	// This handles the case where API sent chat_message to Zed with a request_id
	if requestID != "" {
		if mappedSessionID, exists := apiServer.requestToSessionMapping[requestID]; exists {
			log.Info().
				Str("request_id", requestID).
				Str("helix_session_id", mappedSessionID).
				Str("acp_thread_id", acpThreadID).
				Msg("✅ [HELIX] Found existing Helix session via request_id mapping")

			helixSessionID = mappedSessionID // Use the mapped session

			// Clean up the request mapping now that we have the thread mapping
			delete(apiServer.requestToSessionMapping, requestID)
			log.Info().
				Str("request_id", requestID).
				Msg("🧹 [HELIX] Cleaned up request_id mapping")
		}
	}

	// PRIORITY 2: Check if helixSessionID is provided or was found via request_id
	// If helixSessionID is provided, REUSE that session instead of creating a new one
	if helixSessionID != "" {
		// This is a response to a Helix-initiated request - store zed_context_id on the session
		log.Info().
			Str("agent_session_id", sessionID).
			Str("helix_session_id", helixSessionID).
			Str("zed_context_id", contextID).
			Msg("✅ [HELIX] Storing Zed context ID on existing Helix session")

		// Get the existing session
		helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
		if err != nil {
			return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
		}

		// Store the zed_context_id on the session metadata
		helixSession.Metadata.ZedThreadID = contextID
		helixSession.Updated = time.Now()

		// Update the session in the database
		_, err = apiServer.Controller.Options.Store.UpdateSession(context.Background(), *helixSession)
		if err != nil {
			return fmt.Errorf("failed to update session with zed_context_id: %w", err)
		}

		// CRITICAL: Store the mapping so message_added can find the session
		apiServer.contextMappings[contextID] = helixSessionID

		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("zed_context_id", contextID).
			Msg("✅ [HELIX] Successfully stored zed_context_id on session and populated contextMappings")

		return nil
	}

	// If no helixSessionID provided, this is a NEW context created by user inside Zed
	// Only in this case should we create a new Helix session
	log.Info().
		Str("agent_session_id", sessionID).
		Str("context_id", contextID).
		Msg("🆕 [HELIX] Creating NEW Helix session for user-initiated Zed context")

	// Get the real user ID who created this external agent session
	userID, exists := apiServer.externalAgentUserMapping[sessionID]
	if !exists || userID == "" {
		log.Warn().
			Str("agent_session_id", sessionID).
			Msg("⚠️ [HELIX] No user mapping found for external agent, using default")
		userID = "external-agent-user" // Fallback for safety
	}

	log.Info().
		Str("agent_session_id", sessionID).
		Str("user_id", userID).
		Msg("✅ [HELIX] Using real user ID for Helix session")

	// Create a new Helix session for this Zed context
	helixSession := types.Session{
		ID:        "", // Will be generated
		Name:      title,
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		ModelName: "claude-3.5-sonnet", // Default model, could be configurable
		Created:   time.Now(),
		Updated:   time.Now(),
		Metadata: types.SessionMetadata{
			SystemPrompt: "You are a helpful AI assistant integrated with Zed editor.",
			AgentType:    "zed_external",
		},
	}

	// Create the session in the store
	createdSession, err := apiServer.Controller.Options.Store.CreateSession(context.Background(), helixSession)
	if err != nil {
		return fmt.Errorf("failed to create Helix session: %w", err)
	}

	log.Info().
		Str("agent_session_id", sessionID).
		Str("context_id", contextID).
		Str("helix_session_id", createdSession.ID).
		Str("title", title).
		Msg("Created Helix session for user-initiated Zed context")

	// Store the context mapping for future message routing
	if apiServer.contextMappings == nil {
		apiServer.contextMappings = make(map[string]string)
	}
	apiServer.contextMappings[contextID] = createdSession.ID

	// CRITICAL: Create an interaction for this new session
	// The request_id from thread_created contains the message that triggered this thread
	log.Info().
		Str("context_id", contextID).
		Str("helix_session_id", createdSession.ID).
		Str("request_id", requestID).
		Msg("🆕 [HELIX] Creating initial interaction for new Zed thread")

	interaction := &types.Interaction{
		ID:             "", // Will be generated
		GenerationID:   0,
		Created:        time.Now(),
		Updated:        time.Now(),
		Scheduled:      time.Now(),
		Completed:      time.Time{},
		SessionID:      createdSession.ID,
		UserID:         createdSession.Owner,
		Mode:           types.SessionModeInference,
		PromptMessage:  "New conversation started via Zed", // Default message
		State:          types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(context.Background(), interaction)
	if err != nil {
		log.Error().Err(err).
			Str("helix_session_id", createdSession.ID).
			Msg("❌ [HELIX] Failed to create interaction for new thread")
		return fmt.Errorf("failed to create interaction: %w", err)
	}

	// Store the session->interaction mapping
	if apiServer.sessionToWaitingInteraction == nil {
		apiServer.sessionToWaitingInteraction = make(map[string]string)
	}
	apiServer.sessionToWaitingInteraction[createdSession.ID] = createdInteraction.ID

	log.Info().
		Str("helix_session_id", createdSession.ID).
		Str("interaction_id", createdInteraction.ID).
		Msg("✅ [HELIX] Created initial interaction and stored mapping")

	return nil
}

// NotifyExternalAgentOfNewInteraction sends a message to external agent when a new interaction is created
func (apiServer *HelixAPIServer) NotifyExternalAgentOfNewInteraction(sessionID string, interaction *types.Interaction) error {
	// Get the session to check if it has an external agent
	session, err := apiServer.Controller.Options.Store.GetSession(context.Background(), sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Check if this session uses an external Zed agent
	if session.Metadata.AgentType != "zed_external" {
		// Not an external agent session, nothing to do
		return nil
	}

	log.Info().
		Str("session_id", sessionID).
		Str("interaction_id", interaction.ID).
		Str("agent_type", session.Metadata.AgentType).
		Msg("Notifying external agent of new interaction")

	// Find the external agent connection for this session type
	// For now, we'll send to all connected external agents
	// In a more sophisticated implementation, we'd track which agent is handling which session
	apiServer.externalAgentWSManager.mu.RLock()
	defer apiServer.externalAgentWSManager.mu.RUnlock()

	if len(apiServer.externalAgentWSManager.connections) == 0 {
		log.Warn().Str("session_id", sessionID).Msg("No external agents connected to handle session")
		return nil
	}

	// Create the command to send to Zed
	// CRITICAL: Include acp_thread_id if this session already has one (for follow-up messages)
	commandData := map[string]interface{}{
		"session_id": sessionID,
		"message":    interaction.PromptMessage,
		"role":       "user",
		"request_id": interaction.ID, // Use interaction ID as request ID for response tracking
	}

	// If session already has a Zed thread ID, include it so message goes to existing thread
	if session.Metadata.ZedThreadID != "" {
		commandData["acp_thread_id"] = session.Metadata.ZedThreadID
		log.Info().
			Str("session_id", sessionID).
			Str("acp_thread_id", session.Metadata.ZedThreadID).
			Msg("🔗 [HELIX] Sending follow-up message to existing Zed thread")
	}

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: commandData,
	}

	// Send to all connected external agents (in future, route to specific agent)
	var sentCount int
	for agentSessionID, wsConn := range apiServer.externalAgentWSManager.connections {
		select {
		case wsConn.SendChan <- command:
			log.Info().
				Str("session_id", sessionID).
				Str("agent_session_id", agentSessionID).
				Str("interaction_id", interaction.ID).
				Msg("Sent interaction to external agent via WebSocket")
			sentCount++
		default:
			log.Warn().
				Str("session_id", sessionID).
				Str("agent_session_id", agentSessionID).
				Msg("Failed to send to external agent: channel full")
		}
	}

	if sentCount > 0 {
		log.Info().
			Str("session_id", sessionID).
			Int("sent_count", sentCount).
			Msg("Successfully notified external agents of new interaction")
	}

	return nil
}

// handleMessageAdded processes message addition from external agent
func (apiServer *HelixAPIServer) handleMessageAdded(sessionID string, syncMsg *types.SyncMessage) error {
	// NEW PROTOCOL: use acp_thread_id
	contextID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok {
		// FALLBACK: try old context_id for compatibility
		contextID, ok = syncMsg.Data["context_id"].(string)
		if !ok {
			return fmt.Errorf("missing or invalid acp_thread_id/context_id")
		}
	}

	messageID, ok := syncMsg.Data["message_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid message_id")
	}

	content, ok := syncMsg.Data["content"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid content")
	}

	var role string
	role, ok = syncMsg.Data["role"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid role")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("context_id", contextID).
		Str("message_id", messageID).
		Str("role", role).
		Msg("External agent added message")

	// Find the Helix session that corresponds to this Zed context
	helixSessionID, exists := apiServer.contextMappings[contextID]
	if !exists {
		return fmt.Errorf("no Helix session found for context_id: %s", contextID)
	}

	// Get the Helix session
	helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
	if err != nil {
		return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
	}

	if role == "assistant" {
		// For assistant messages, we need to load interactions to update them
		// Load interactions following the same pattern as handlers.go getSession
		interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
			SessionID:    helixSessionID,
			GenerationID: helixSession.GenerationID,
			PerPage:      1000,
		})
		if err != nil {
			return fmt.Errorf("failed to list interactions for session %s: %w", helixSessionID, err)
		}

		log.Info().
			Str("helix_session_id", helixSessionID).
			Int("interaction_count", len(interactions)).
			Msg("🔍 [DEBUG] Retrieved session interactions")

		// CRITICAL: Use session->interaction mapping to find the exact interaction
		// This mapping was stored when we sent the chat_message command
		var targetInteraction *types.Interaction
		if interactionID, exists := apiServer.sessionToWaitingInteraction[helixSessionID]; exists {
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("mapped_interaction_id", interactionID).
				Msg("🔍 [DEBUG] Found interaction mapping")

			// Find the interaction by ID
			for i := range interactions {
				if interactions[i].ID == interactionID {
					targetInteraction = interactions[i]
					log.Info().
						Str("helix_session_id", helixSessionID).
						Str("interaction_id", interactionID).
						Msg("🎯 [HELIX] Found interaction for session using mapping")
					break
				}
			}
		}

		// Fallback: Find the most recent interaction that needs an AI response
		if targetInteraction == nil {
			for i := len(interactions) - 1; i >= 0; i-- {
				// Look for interactions that are either Waiting OR Complete with empty response
				if interactions[i].State == types.InteractionStateWaiting ||
					(interactions[i].State == types.InteractionStateComplete && interactions[i].ResponseMessage == "") {
					targetInteraction = interactions[i]
					log.Info().
						Str("helix_session_id", helixSessionID).
						Str("interaction_id", interactions[i].ID).
						Str("interaction_state", string(interactions[i].State)).
						Msg("⚠️ [HELIX] No session mapping found, using most recent empty interaction")
					break
				}
			}
		}

		if targetInteraction != nil {
			// Update the existing interaction with the AI response content
			// IMPORTANT: Keep state as Waiting - only message_completed marks it as Complete
			// NOTE: Zed sends full content each time (not incremental), so overwriting is correct
			targetInteraction.ResponseMessage = content
			targetInteraction.Updated = time.Now()

			_, err := apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), targetInteraction)
			if err != nil {
				return fmt.Errorf("failed to update interaction %s: %w", targetInteraction.ID, err)
			}

			log.Info().
				Str("session_id", sessionID).
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", targetInteraction.ID).
				Str("role", role).
				Str("content", content).
				Msg("📝 [HELIX] Updated interaction with AI response (keeping Waiting state)")

			// CRITICAL: Also send to response channel for HTTP streaming
			// The request_id was sent to Zed in the chat_message command
			// We need to find it using the request->session mapping
			var foundRequestID string
			if apiServer.requestToSessionMapping != nil {
				for reqID, sessID := range apiServer.requestToSessionMapping {
					if sessID == helixSessionID {
						foundRequestID = reqID
						break
					}
				}
			}

			if foundRequestID != "" {
				responseChan, _, _, exists := apiServer.getResponseChannel(helixSessionID, foundRequestID)
				if exists {
					select {
					case responseChan <- content:
						log.Info().
							Str("session_id", helixSessionID).
							Str("request_id", foundRequestID).
							Int("content_length", len(content)).
							Msg("✅ [HELIX] Sent message_added content to HTTP streaming channel")
					default:
						log.Warn().
							Str("session_id", helixSessionID).
							Str("request_id", foundRequestID).
							Msg("HTTP response channel full or closed")
					}
				} else {
					log.Debug().
						Str("session_id", helixSessionID).
						Str("request_id", foundRequestID).
						Msg("No HTTP response channel found (may not be an HTTP streaming request)")
				}
			}

			// Reload session with all interactions so WebSocket event has latest data
			reloadedSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
			if err != nil {
				log.Error().Err(err).Str("session_id", helixSessionID).Msg("Failed to reload session")
			} else {
				// Load all interactions
				allInteractions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
					SessionID:    helixSessionID,
					GenerationID: reloadedSession.GenerationID,
					PerPage:      1000,
				})
				if err == nil {
					reloadedSession.Interactions = allInteractions

					// DEBUG: Log what we're about to send
					log.Info().
						Str("session_id", helixSessionID).
						Int("interaction_count", len(allInteractions)).
						Int("last_interaction_response_len", len(allInteractions[len(allInteractions)-1].ResponseMessage)).
						Str("last_interaction_state", string(allInteractions[len(allInteractions)-1].State)).
						Msg("🔍 [DEBUG] About to publish session update")

					// Publish session update to frontend so UI updates in real-time
					err = apiServer.publishSessionUpdateToFrontend(reloadedSession, targetInteraction)
					if err != nil {
						log.Error().Err(err).
							Str("session_id", helixSessionID).
							Str("interaction_id", targetInteraction.ID).
							Msg("Failed to publish session update to frontend")
					}
				}
			}
		} else {
			log.Warn().
				Str("session_id", sessionID).
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Msg("No interaction found to update with assistant response")
		}
	} else {
		// For user messages, create new interaction
		interaction := &types.Interaction{
			ID:            "", // Will be generated
			Created:       time.Now(),
			Updated:       time.Now(),
			SessionID:     helixSessionID,
			UserID:        helixSession.Owner,
			Mode:          types.SessionModeInference,
			PromptMessage: content,
			State:         types.InteractionStateWaiting,
		}

		// Create the interaction in the store
		createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(context.Background(), interaction)
		if err != nil {
			return fmt.Errorf("failed to create interaction: %w", err)
		}

		log.Info().
			Str("session_id", sessionID).
			Str("context_id", contextID).
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", createdInteraction.ID).
			Str("role", role).
			Msg("💬 [HELIX] Created interaction for user message from Zed")

		// CRITICAL: Map this interaction so the AI response goes to it!
		apiServer.sessionToWaitingInteraction[helixSessionID] = createdInteraction.ID
		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", createdInteraction.ID).
			Msg("🗺️ [HELIX] Mapped session to new interaction from Zed user message")
	}

	return nil
}

// handleMessageUpdated processes message updates from external agent
func (apiServer *HelixAPIServer) handleMessageUpdated(sessionID string, syncMsg *types.SyncMessage) error {
	// TODO: Handle message updates (e.g., editing)
	log.Debug().Str("session_id", sessionID).Msg("Message updated")
	return nil
}

// handleContextTitleChanged processes context title changes
func (apiServer *HelixAPIServer) handleContextTitleChanged(sessionID string, syncMsg *types.SyncMessage) error {
	// TODO: Update context title in Helix
	log.Debug().Str("session_id", sessionID).Msg("Context title changed")
	return nil
}

// sendCommandToExternalAgent sends a command to the external agent
func (apiServer *HelixAPIServer) sendCommandToExternalAgent(sessionID string, command types.ExternalAgentCommand) error {
	// Add session_id to the command data for context
	if command.Data == nil {
		command.Data = make(map[string]interface{})
	}
	command.Data["session_id"] = sessionID

	// Get the WebSocket connection for this session
	wsConn, exists := apiServer.externalAgentWSManager.getConnection(sessionID)
	if !exists || wsConn == nil {
		return fmt.Errorf("no WebSocket connection found for session %s", sessionID)
	}

	// Send command to the specific Zed agent
	select {
	case wsConn.SendChan <- command:
		log.Info().
			Str("session_id", sessionID).
			Str("command_type", command.Type).
			Msg("✅ Sent command to specific external Zed agent")
		return nil
	default:
		return fmt.Errorf("external agent send channel full for session %s", sessionID)
	}
}

// registerConnection registers a new external agent connection
func (manager *ExternalAgentWSManager) registerConnection(sessionID string, conn *ExternalAgentWSConnection) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.connections[sessionID] = conn
	log.Info().
		Str("session_id", sessionID).
		Int("total_connections", len(manager.connections)).
		Msg("🔗 [HELIX] Registered external agent connection")
}

// unregisterConnection unregisters an external agent connection
func (manager *ExternalAgentWSManager) unregisterConnection(sessionID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if conn, exists := manager.connections[sessionID]; exists {
		close(conn.SendChan)
		delete(manager.connections, sessionID)
	}
}

// getConnection gets an external agent connection
func (manager *ExternalAgentWSManager) getConnection(sessionID string) (*ExternalAgentWSConnection, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	conn, exists := manager.connections[sessionID]
	return conn, exists
}

// listConnections returns all active connections
func (manager *ExternalAgentWSManager) listConnections() []types.ExternalAgentConnection {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	connections := make([]types.ExternalAgentConnection, 0, len(manager.connections))
	for sessionID, conn := range manager.connections {
		connections = append(connections, types.ExternalAgentConnection{
			SessionID:   sessionID,
			ConnectedAt: conn.ConnectedAt,
			LastPing:    conn.LastPing,
			Status:      "connected",
		})
	}
	return connections
}

// handleChatResponse processes complete chat response from external agent
func (apiServer *HelixAPIServer) handleChatResponse(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("🔵 [HELIX] RECEIVED CHAT_RESPONSE FROM EXTERNAL AGENT")
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response missing request_id")
		return nil
	}

	content, ok := syncMsg.Data["content"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Chat response missing content")
		return nil
	}

	// Skip placeholder acknowledgment responses - they should not trigger completion
	if content == "🤖 Processing your request with AI... (Real response will follow via async system)" {
		log.Info().
			Str("session_id", sessionID).
			Str("request_id", requestID).
			Msg("Skipping placeholder acknowledgment response - waiting for real AI response")
		return nil
	}

	// CRITICAL FIX: Use the Helix Session ID from the message, not the Agent Session ID
	helixSessionID := syncMsg.SessionID
	log.Info().
		Str("agent_session_id", sessionID).
		Str("helix_session_id", helixSessionID).
		Str("request_id", requestID).
		Msg("🔧 [HELIX] USING HELIX SESSION ID FOR RESPONSE CHANNEL LOOKUP")

	// Handle response via legacy channel handling
	responseChan, doneChan, _, exists := apiServer.getResponseChannel(helixSessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for request")
		return nil
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Str("content", content).
		Msg("🔵 [HELIX] SENDING RESPONSE TO RESPONSE CHANNEL")

	// Send content as single chunk
	select {
	case responseChan <- content:
		log.Info().
			Str("session_id", sessionID).
			Str("request_id", requestID).
			Msg("✅ [HELIX] RESPONSE SENT TO CHANNEL SUCCESSFULLY")
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Response channel full")
	}

	// Send completion signal
	select {
	case doneChan <- true:
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Done channel full")
	}

	return nil
}

// handleChatResponseChunk processes streaming chat response chunk from external agent
func (apiServer *HelixAPIServer) handleChatResponseChunk(sessionID string, syncMsg *types.SyncMessage) error {
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response chunk missing request_id")
		return nil
	}

	chunk, ok := syncMsg.Data["chunk"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Chat response chunk missing chunk")
		return nil
	}

	// Handle response chunk via legacy channel handling
	responseChan, _, _, exists := apiServer.getResponseChannel(sessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for chunk")
		return nil
	}

	// Send chunk
	select {
	case responseChan <- chunk:
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Response channel full for chunk")
	}

	return nil
}

// handleChatResponseDone processes completion signal from external agent
func (apiServer *HelixAPIServer) handleChatResponseDone(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("🔵 [HELIX] RECEIVED CHAT_RESPONSE_DONE FROM EXTERNAL AGENT")
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response done missing request_id")
		return nil
	}

	// CRITICAL FIX: Use the Helix Session ID from the message, not the Agent Session ID
	helixSessionID := syncMsg.SessionID
	log.Info().
		Str("agent_session_id", sessionID).
		Str("helix_session_id", helixSessionID).
		Str("request_id", requestID).
		Msg("🔧 [HELIX] USING HELIX SESSION ID FOR DONE CHANNEL LOOKUP")

	// Handle response completion via legacy channel handling
	_, doneChan, _, exists := apiServer.getResponseChannel(helixSessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for done signal")
		return nil
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Msg("🔵 [HELIX] SENDING DONE SIGNAL TO DONE CHANNEL")

	// Send completion signal
	select {
	case doneChan <- true:
		log.Info().
			Str("session_id", sessionID).
			Str("request_id", requestID).
			Msg("✅ [HELIX] DONE SIGNAL SENT TO CHANNEL SUCCESSFULLY")
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Done channel full")
	}

	return nil
}

// handleMessageCompleted marks the interaction as complete when AI finishes responding
func (apiServer *HelixAPIServer) handleMessageCompleted(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("🎯 [HELIX] RECEIVED MESSAGE_COMPLETED FROM EXTERNAL AGENT")

	// Extract acp_thread_id from the data
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok || acpThreadID == "" {
		return fmt.Errorf("missing acp_thread_id in message_completed data")
	}

	// Look up helix_session_id from context mapping
	helixSessionID, ok := apiServer.contextMappings[acpThreadID]
	if !ok {
		log.Warn().
			Str("acp_thread_id", acpThreadID).
			Msg("⚠️ [HELIX] No Helix session mapping found for this thread - skipping message_completed")
		return nil
	}

	log.Info().
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", helixSessionID).
		Msg("✅ [HELIX] Found Helix session mapping for message_completed")

	// Get the session
	helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
	if err != nil {
		return fmt.Errorf("failed to get Helix session %s: %w", helixSessionID, err)
	}

	// Load interactions for this session (GetSession doesn't load them)
	interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID:    helixSessionID,
		GenerationID: helixSession.GenerationID,
		PerPage:      1000,
	})
	if err != nil {
		return fmt.Errorf("failed to list interactions for session %s: %w", helixSessionID, err)
	}

	log.Info().
		Str("helix_session_id", helixSessionID).
		Int("interaction_count", len(interactions)).
		Msg("🔍 [DEBUG] Loaded interactions for message_completed")

	// Find the most recent waiting interaction
	var targetInteractionID string
	for i := len(interactions) - 1; i >= 0; i-- {
		if interactions[i].State == types.InteractionStateWaiting {
			targetInteractionID = interactions[i].ID
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", interactions[i].ID).
				Str("state", string(interactions[i].State)).
				Msg("✅ [HELIX] Found waiting interaction to mark as complete")
			break
		}
	}

	if targetInteractionID == "" {
		log.Warn().
			Str("helix_session_id", helixSessionID).
			Msg("⚠️ [HELIX] No waiting interaction found to mark as complete")
		return nil
	}

	// CRITICAL: Reload the interaction from database to get latest response_message
	// The message_added handler may have just updated it, so we need the freshest data
	targetInteraction, err := apiServer.Controller.Options.Store.GetInteraction(context.Background(), targetInteractionID)
	if err != nil {
		return fmt.Errorf("failed to reload interaction %s: %w", targetInteractionID, err)
	}

	log.Info().
		Str("helix_session_id", helixSessionID).
		Str("interaction_id", targetInteraction.ID).
		Int("response_length", len(targetInteraction.ResponseMessage)).
		Str("response_preview", targetInteraction.ResponseMessage).
		Str("state_before", string(targetInteraction.State)).
		Msg("🔄 [HELIX] Reloaded interaction with latest response content")

	// Mark the interaction as complete
	targetInteraction.State = types.InteractionStateComplete
	targetInteraction.Completed = time.Now()
	targetInteraction.Updated = time.Now()

	_, err = apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), targetInteraction)
	if err != nil {
		return fmt.Errorf("failed to update interaction %s: %w", targetInteraction.ID, err)
	}

	log.Info().
		Str("helix_session_id", helixSessionID).
		Str("interaction_id", targetInteraction.ID).
		Int("final_response_length", len(targetInteraction.ResponseMessage)).
		Str("final_state", string(targetInteraction.State)).
		Msg("✅ [HELIX] Marked interaction as complete")

	// Also send completion signal to done channel for legacy handling
	requestID, ok := syncMsg.Data["request_id"].(string)
	if ok {
		_, doneChan, _, exists := apiServer.getResponseChannel(helixSessionID, requestID)
		if exists {
			select {
			case doneChan <- true:
				log.Info().Str("request_id", requestID).Msg("✅ [HELIX] Sent done signal to channel")
			default:
				log.Warn().Str("request_id", requestID).Msg("Done channel full")
			}
		}
	}

	return nil
}

// handleChatResponseError processes error from external agent
func (apiServer *HelixAPIServer) handleChatResponseError(sessionID string, syncMsg *types.SyncMessage) error {
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response error missing request_id")
		return nil
	}

	errorMsg, ok := syncMsg.Data["error"].(string)
	if !ok {
		errorMsg = "Unknown error from external agent"
	}

	// Handle response error via legacy channel handling
	_, _, errorChan, exists := apiServer.getResponseChannel(sessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for error")
		return nil
	}

	// Send error
	select {
	case errorChan <- fmt.Errorf("%s", errorMsg):
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Error channel full")
	}

	return nil
}

// validateExternalAgentToken validates the auth token for external agent
func (apiServer *HelixAPIServer) validateExternalAgentToken(sessionID, token string) bool {
	// TODO: Implement proper token validation
	// For now, just check if token is not empty and session exists
	if token == "" {
		return false
	}

	// TODO: Check if session exists in store
	// TODO: Validate token against stored session tokens
	// TODO: Check token expiration

	return true // Placeholder - always valid for now
}

// generateExternalAgentToken generates an auth token for external agent
func (apiServer *HelixAPIServer) generateExternalAgentToken(sessionID string) (string, error) {
	// TODO: Generate secure token and store it
	// For now, return a simple token
	token := fmt.Sprintf("ext-agent-%s-%d", sessionID, time.Now().Unix())

	// TODO: Store token with expiration in database/cache
	log.Debug().Str("session_id", sessionID).Str("token", token).Msg("Generated external agent token")

	return token, nil
}

// publishSessionUpdateToFrontend publishes a session update to the frontend via pubsub
func (apiServer *HelixAPIServer) publishSessionUpdateToFrontend(session *types.Session, interaction *types.Interaction) error {
	// Create websocket event for frontend
	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventSessionUpdate,
		SessionID:     session.ID,
		InteractionID: interaction.ID,
		Owner:         session.Owner,
		Session:       session,
	}

	// Marshal to JSON
	messageBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal websocket event: %w", err)
	}

	// Publish to user's session queue
	err = apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(session.Owner, session.ID), messageBytes)
	if err != nil {
		return fmt.Errorf("failed to publish to pubsub: %w", err)
	}

	log.Info().
		Str("session_id", session.ID).
		Str("interaction_id", interaction.ID).
		Str("owner", session.Owner).
		Msg("📤 [HELIX] Published session update to frontend")

	return nil
}

// handleUserCreatedThread processes user-created thread event from Zed UI
// Creates a new Helix session and maps it to the Zed thread
func (apiServer *HelixAPIServer) handleUserCreatedThread(agentSessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("agent_session_id", agentSessionID).
		Interface("data", syncMsg.Data).
		Msg("🆕 [HELIX] User created new thread in Zed UI")

	// Extract thread ID and title
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok || acpThreadID == "" {
		return fmt.Errorf("missing or invalid acp_thread_id in user_created_thread event")
	}

	title, _ := syncMsg.Data["title"].(string)
	if title == "" {
		title = "New Chat" // Default title
	}

	// Get the existing Helix session (agentSessionID is the session ID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existingSession, err := apiServer.Controller.Options.Store.GetSession(ctx, agentSessionID)
	if err != nil {
		return fmt.Errorf("failed to load existing session: %w", err)
	}

	// Create new Helix session for this user-created thread
	// CRITICAL: Copy config from existing session so it has same parent_app, agent_type, etc.
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           existingSession.Type,
		ModelName:      existingSession.ModelName,
		ParentApp:      existingSession.ParentApp, // Copy parent_app for screenshot view
		OrganizationID: existingSession.OrganizationID,
		Owner:          existingSession.Owner,
		OwnerType:      existingSession.OwnerType,
		Metadata: types.SessionMetadata{
			ZedThreadID:         acpThreadID,
			AgentType:           existingSession.Metadata.AgentType,
			ExternalAgentConfig: existingSession.Metadata.ExternalAgentConfig,
		},
		Name: title,
	}

	// Store session in database
	_, err = apiServer.Controller.Options.Store.CreateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Map Zed thread to Helix session (same as handleThreadCreated)
	apiServer.contextMappings[acpThreadID] = session.ID

	log.Info().
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", session.ID).
		Str("title", title).
		Msg("✅ [HELIX] Created new session for user-created Zed thread")

	return nil
}

// handleThreadTitleChanged processes thread title change event from Zed
// Updates the corresponding Helix session name
func (apiServer *HelixAPIServer) handleThreadTitleChanged(agentSessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("agent_session_id", agentSessionID).
		Interface("data", syncMsg.Data).
		Msg("📝 [HELIX] Thread title changed in Zed")

	// Extract thread ID and new title
	acpThreadID, ok := syncMsg.Data["acp_thread_id"].(string)
	if !ok || acpThreadID == "" {
		return fmt.Errorf("missing or invalid acp_thread_id in thread_title_changed event")
	}

	newTitle, ok := syncMsg.Data["title"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid title in thread_title_changed event")
	}

	// Find corresponding Helix session (same as handleThreadCreated uses)
	helixSessionID, exists := apiServer.contextMappings[acpThreadID]

	if !exists {
		log.Warn().
			Str("acp_thread_id", acpThreadID).
			Msg("⚠️ [HELIX] Thread title changed but no Helix session found for thread")
		return nil // Not an error - thread might not have a session yet
	}

	// Load session
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := apiServer.Controller.Options.Store.GetSession(ctx, helixSessionID)
	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	// Update session name
	session.Name = newTitle
	session.Updated = time.Now()

	_, err = apiServer.Controller.Options.Store.UpdateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to update session name: %w", err)
	}

	log.Info().
		Str("acp_thread_id", acpThreadID).
		Str("helix_session_id", helixSessionID).
		Str("new_title", newTitle).
		Msg("✅ [HELIX] Updated session name from Zed thread title")

	// Publish a session update so frontend refetches sessions list with new title
	event := &types.WebsocketEvent{
		Type:      types.WebsocketEventSessionUpdate,
		SessionID: helixSessionID,
		Owner:     session.Owner,
		Session:   session,
	}
	messageBytes, err := json.Marshal(event)
	if err == nil {
		apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(session.Owner, session.ID), messageBytes)
	}

	return nil
}
