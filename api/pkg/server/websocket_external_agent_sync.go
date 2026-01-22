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
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// External agent WebSocket connections
type ExternalAgentWSManager struct {
	connections map[string]*ExternalAgentWSConnection
	mu          sync.RWMutex
	upgrader    websocket.Upgrader

	// Session readiness tracking - prevents sending messages before agent is ready
	readinessState map[string]*SessionReadinessState
	readinessMu    sync.RWMutex
}

// SessionReadinessState tracks whether an agent session is ready to receive messages
// This prevents race conditions where we send prompts before Zed has loaded the agent
type SessionReadinessState struct {
	IsReady       bool                         // True when agent_ready received
	ReadyAt       time.Time                    // When agent became ready
	PendingQueue  []types.ExternalAgentCommand // Commands queued before ready
	TimeoutTimer  *time.Timer                  // Fallback timeout (60s)
	SessionID     string                       // For logging
	NeedsContinue bool                         // Whether to send continue prompt when ready
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
		connections:    make(map[string]*ExternalAgentWSConnection),
		readinessState: make(map[string]*SessionReadinessState),
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
	} else {
		apiServer.contextMappingsMutex.RLock()
		mappedHelixID, exists := apiServer.externalAgentSessionMapping[agentID]
		apiServer.contextMappingsMutex.RUnlock()
		if exists {
			// Agent session ID mapping - register connection with BOTH IDs for routing
			helixSessionID = mappedHelixID
			apiServer.externalAgentWSManager.registerConnection(helixSessionID, wsConn)
			defer apiServer.externalAgentWSManager.unregisterConnection(helixSessionID)
			log.Info().
				Str("agent_session_id", agentID).
				Str("helix_session_id", helixSessionID).
				Msg("🚀 [HELIX] External agent connected with agent session ID, registered with BOTH IDs for routing")
		}
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
			// CRITICAL: Rebuild contextMappings from persisted ZedThreadID if present
			// This ensures message routing works after API server restarts
			if helixSession.Metadata.ZedThreadID != "" {
				apiServer.contextMappingsMutex.Lock()
				apiServer.contextMappings[helixSession.Metadata.ZedThreadID] = helixSessionID
				apiServer.contextMappingsMutex.Unlock()
				log.Info().
					Str("helix_session_id", helixSessionID).
					Str("zed_thread_id", helixSession.Metadata.ZedThreadID).
					Msg("🔧 [HELIX] Restored contextMappings from session metadata (ensures message routing after restart)")
			}

			// Check if agent was working before disconnect (to determine if continue prompt needed)
			needsContinue := false

			// Initialize readiness tracking - we'll wait for agent_ready before sending continue prompt
			// This prevents race conditions where we send prompts before the agent is ready
			apiServer.externalAgentWSManager.initReadinessState(helixSessionID, needsContinue, nil)
			defer apiServer.externalAgentWSManager.cleanupReadinessState(helixSessionID)

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

							// Determine which agent to use based on the spec task's code agent config
							agentName := apiServer.getAgentNameForSession(ctx, helixSession)

							// CRITICAL FIX: Use existing thread if available, otherwise create new
							// This ensures message routing works after container restart by continuing
							// in the same Zed thread (whose ID is stored on the session)
							var acpThreadID interface{} = nil
							if helixSession.Metadata.ZedThreadID != "" {
								acpThreadID = helixSession.Metadata.ZedThreadID
								log.Info().
									Str("helix_session_id", helixSessionID).
									Str("zed_thread_id", helixSession.Metadata.ZedThreadID).
									Msg("🔗 [HELIX] Resuming in existing Zed thread after reconnect")
							}

							command := types.ExternalAgentCommand{
								Type: "chat_message",
								Data: map[string]interface{}{
									"message":       fullMessage,
									"request_id":    requestID,
									"acp_thread_id": acpThreadID, // Use existing thread if available
									"agent_name":    agentName,   // Which agent to use (zed-agent or qwen)
								},
							}

							// Queue message - will be sent when agent_ready is received
							// This prevents race condition where we send before Zed is stable
							if apiServer.externalAgentWSManager.queueOrSend(helixSessionID, command) {
								log.Info().
									Str("agent_session_id", agentID).
									Str("request_id", requestID).
									Str("helix_session_id", helixSessionID).
									Msg("✅ [HELIX] Queued initial chat_message for Zed (will send when agent_ready)")

							} else {
								log.Warn().
									Str("agent_session_id", agentID).
									Msg("⚠️ [HELIX] Failed to queue initial message")
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
	// Determine event type based on completion status
	eventType := "chat_response_chunk"
	if isComplete {
		eventType = "chat_response_done"
	}

	command := types.ExternalAgentCommand{
		Type: eventType,
		Data: map[string]interface{}{
			"context_id": contextID,
			"content":    content,
			"timestamp":  time.Now(),
		},
	}

	return apiServer.sendCommandToExternalAgent(sessionID, command)
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
	// activity, err := apiServer.Store.GetExternalAgentActivity(context.Background(), sessionID)
	// if err == nil && activity != nil {
	// 	// Activity record exists - update it to extend idle timeout
	// 	activity.LastInteraction = time.Now()
	// 	err = apiServer.Store.UpsertExternalAgentActivity(context.Background(), activity)
	// 	if err != nil {
	// 		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to update activity for WebSocket message")
	// 		// Non-fatal - continue processing message
	// 	}
	// }
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
	case "thread_load_error":
		return apiServer.handleThreadLoadError(sessionID, syncMsg)
	case "chat_response_error":
		return apiServer.handleChatResponseError(sessionID, syncMsg)
	case "agent_ready":
		return apiServer.handleAgentReady(sessionID, syncMsg)
	case "ping":
		return nil
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
		apiServer.contextMappingsMutex.RLock()
		mappedSessionID, exists := apiServer.requestToSessionMapping[requestID]
		apiServer.contextMappingsMutex.RUnlock()
		if exists {
			log.Info().
				Str("request_id", requestID).
				Str("helix_session_id", mappedSessionID).
				Str("acp_thread_id", acpThreadID).
				Msg("✅ [HELIX] Found existing Helix session via request_id mapping")

			helixSessionID = mappedSessionID // Use the mapped session

			// Clean up the request mappings now that we have the thread mapping
			delete(apiServer.requestToSessionMapping, requestID)
			delete(apiServer.requestToCommenterMapping, requestID)
			log.Info().
				Str("request_id", requestID).
				Msg("🧹 [HELIX] Cleaned up request_id mappings")
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
		apiServer.contextMappingsMutex.Lock()
		apiServer.contextMappings[contextID] = helixSessionID
		apiServer.contextMappingsMutex.Unlock()

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
	apiServer.contextMappingsMutex.RLock()
	userID, exists := apiServer.externalAgentUserMapping[sessionID]
	apiServer.contextMappingsMutex.RUnlock()
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
	apiServer.contextMappingsMutex.Lock()
	if apiServer.contextMappings == nil {
		apiServer.contextMappings = make(map[string]string)
	}
	apiServer.contextMappings[contextID] = createdSession.ID
	apiServer.contextMappingsMutex.Unlock()

	// CRITICAL: Create an interaction for this new session
	// The request_id from thread_created contains the message that triggered this thread
	log.Info().
		Str("context_id", contextID).
		Str("helix_session_id", createdSession.ID).
		Str("request_id", requestID).
		Msg("🆕 [HELIX] Creating initial interaction for new Zed thread")

	interaction := &types.Interaction{
		ID:              "", // Will be generated
		GenerationID:    0,
		Created:         time.Now(),
		Updated:         time.Now(),
		Scheduled:       time.Now(),
		Completed:       time.Time{},
		SessionID:       createdSession.ID,
		UserID:          createdSession.Owner,
		Mode:            types.SessionModeInference,
		PromptMessage:   "New conversation started via Zed", // Default message
		State:           types.InteractionStateWaiting,
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
	apiServer.contextMappingsMutex.Lock()
	if apiServer.sessionToWaitingInteraction == nil {
		apiServer.sessionToWaitingInteraction = make(map[string]string)
	}
	apiServer.sessionToWaitingInteraction[createdSession.ID] = createdInteraction.ID
	apiServer.contextMappingsMutex.Unlock()

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

	// Build command data - include acp_thread_id if session already has one (for follow-up messages)
	commandData := map[string]interface{}{
		"message":    interaction.PromptMessage,
		"role":       "user",
		"request_id": interaction.ID, // Use interaction ID as request ID for response tracking
	}

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

	// Use the unified sendCommandToExternalAgent which handles connection lookup and routing
	return apiServer.sendCommandToExternalAgent(sessionID, command)
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
	apiServer.contextMappingsMutex.RLock()
	helixSessionID, exists := apiServer.contextMappings[contextID]
	apiServer.contextMappingsMutex.RUnlock()
	if !exists {
		// FALLBACK: contextMappings may be empty after API restart
		// Try to find session by ZedThreadID in database
		log.Info().
			Str("context_id", contextID).
			Msg("🔍 [HELIX] contextMappings miss, attempting database fallback lookup by ZedThreadID")

		foundSession, err := apiServer.findSessionByZedThreadID(context.Background(), contextID)
		if err != nil || foundSession == nil {
			// No session found for this thread. For user messages, create a session on-the-fly.
			// This handles the race condition where MessageAdded(role=user) arrives before UserCreatedThread.
			if role != "assistant" {
				log.Info().
					Str("context_id", contextID).
					Str("agent_session_id", sessionID).
					Msg("🔧 [HELIX] No session for user message - creating session on-the-fly")

				// Get the existing agent session to copy config from
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				existingSession, err := apiServer.Controller.Options.Store.GetSession(ctx, sessionID)
				if err != nil {
					return fmt.Errorf("failed to load agent session to copy config: %w", err)
				}

				// Create new session with same config as agent session
				newSession := &types.Session{
					ID:             system.GenerateSessionID(),
					Created:        time.Now(),
					Updated:        time.Now(),
					Mode:           types.SessionModeInference,
					Type:           existingSession.Type,
					ModelName:      existingSession.ModelName,
					ParentApp:      existingSession.ParentApp,
					OrganizationID: existingSession.OrganizationID,
					Owner:          existingSession.Owner,
					OwnerType:      existingSession.OwnerType,
					Metadata: types.SessionMetadata{
						ZedThreadID:         contextID,
						AgentType:           existingSession.Metadata.AgentType,
						ExternalAgentConfig: existingSession.Metadata.ExternalAgentConfig,
					},
					Name: "Zed Chat", // Default name, will be updated by thread_title_changed
				}

				_, err = apiServer.Controller.Options.Store.CreateSession(ctx, *newSession)
				if err != nil {
					return fmt.Errorf("failed to create on-the-fly session: %w", err)
				}

				helixSessionID = newSession.ID
				apiServer.contextMappingsMutex.Lock()
				apiServer.contextMappings[contextID] = helixSessionID
				apiServer.contextMappingsMutex.Unlock()

				log.Info().
					Str("context_id", contextID).
					Str("helix_session_id", helixSessionID).
					Msg("✅ [HELIX] Created on-the-fly session for user message")
			} else {
				// Assistant message with no session is an error - shouldn't happen
				return fmt.Errorf("no Helix session found for context_id: %s (in-memory miss, database fallback failed)", contextID)
			}
		} else {
			helixSessionID = foundSession.ID
			// Restore the mapping for future messages
			apiServer.contextMappingsMutex.Lock()
			apiServer.contextMappings[contextID] = helixSessionID
			apiServer.contextMappingsMutex.Unlock()
			log.Info().
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Msg("✅ [HELIX] Found session via database fallback, restored contextMappings")
		}
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
		apiServer.contextMappingsMutex.RLock()
		interactionID, exists := apiServer.sessionToWaitingInteraction[helixSessionID]
		apiServer.contextMappingsMutex.RUnlock()
		if exists {
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
			//
			// MULTI-MESSAGE HANDLING using Zed's message_id (restart-resilient):
			// Zed sends a unique message_id with each message:
			// - Same message_id = streaming update of same message (cumulative content) → OVERWRITE
			// - Different message_id = new distinct message from agent → APPEND
			//
			// We persist LastZedMessageID in the database, so this works across restarts.
			existingContent := targetInteraction.ResponseMessage
			lastMessageID := targetInteraction.LastZedMessageID
			shouldAppend := false

			if lastMessageID == "" {
				// First message for this interaction - overwrite
				shouldAppend = false
				log.Debug().
					Str("interaction_id", targetInteraction.ID).
					Str("message_id", messageID).
					Msg("📝 [HELIX] First message for interaction (setting LastZedMessageID)")
			} else if lastMessageID == messageID {
				// Same message ID - this is a streaming update with cumulative content
				shouldAppend = false
				log.Debug().
					Str("interaction_id", targetInteraction.ID).
					Str("message_id", messageID).
					Msg("📝 [HELIX] Streaming update (same message_id, overwriting)")
			} else {
				// Different message ID - this is a new distinct message from the agent
				shouldAppend = true
				log.Info().
					Str("interaction_id", targetInteraction.ID).
					Str("last_message_id", lastMessageID).
					Str("new_message_id", messageID).
					Msg("📝 [HELIX] New distinct message detected (different message_id)")
			}

			// Always update the LastZedMessageID to the current message
			targetInteraction.LastZedMessageID = messageID

			if shouldAppend && existingContent != "" {
				// New distinct message - append it
				targetInteraction.ResponseMessage = existingContent + "\n\n" + content
				log.Info().
					Str("interaction_id", targetInteraction.ID).
					Int("existing_len", len(existingContent)).
					Int("new_len", len(content)).
					Msg("📝 [HELIX] Appending new message to interaction (multi-message response)")
			} else {
				// Cumulative update or first message - overwrite
				targetInteraction.ResponseMessage = content
			}
			targetInteraction.Updated = time.Now()

			_, err := apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), targetInteraction)
			if err != nil {
				return fmt.Errorf("failed to update interaction %s: %w", targetInteraction.ID, err)
			}

			// Link agent response to design review comment if this interaction came from a comment
			go func() {
				if err := apiServer.linkAgentResponseToComment(context.Background(), targetInteraction); err != nil {
					log.Debug().
						Err(err).
						Str("interaction_id", targetInteraction.ID).
						Msg("No design review comment linked to this interaction (this is normal for non-comment interactions)")
				}
			}()

			log.Info().
				Str("session_id", sessionID).
				Str("context_id", contextID).
				Str("helix_session_id", helixSessionID).
				Str("interaction_id", targetInteraction.ID).
				Str("role", role).
				Str("content_length", fmt.Sprintf("%d", len(content))).
				Bool("appended_new_message", shouldAppend).
				Msg("📝 [HELIX] Updated interaction with AI response (keeping Waiting state)")

			// DATABASE-FIRST: Link response to pending design review comment
			// Query database for comments with pending request_id (survives API and container restarts)
			// IMPORTANT: Get the pending comment ID synchronously to avoid race conditions
			// If we spawn a goroutine and it runs after message_completed, the next comment
			// might already have RequestID set, causing us to update the wrong comment.
			pendingComment, err := apiServer.Store.GetPendingCommentByPlanningSessionID(context.Background(), helixSessionID)
			if err == nil && pendingComment != nil {
				// Found a pending comment - capture its ID and RequestID synchronously
				commentID := pendingComment.ID
				requestID := pendingComment.RequestID

				// Update the comment with streaming response content
				go func(sessionID, commentID, requestID, responseContent string) {
					// Use the request_id to update - this ensures we update the correct comment
					if err := apiServer.updateCommentWithStreamingResponse(context.Background(), requestID, responseContent); err != nil {
						log.Debug().
							Err(err).
							Str("session_id", sessionID).
							Str("request_id", requestID).
							Msg("Failed to update comment with streaming response (this is normal for non-comment interactions)")
						return
					}

					log.Info().
						Str("comment_id", commentID).
						Str("session_id", sessionID).
						Str("request_id", requestID).
						Int("response_length", len(responseContent)).
						Msg("✅ [HELIX] Updated comment with streaming agent response")

					// Also try HTTP streaming for real-time updates (if channel exists)
					if requestID != "" {
						responseChan, _, _, exists := apiServer.getResponseChannel(sessionID, requestID)
						if exists {
							select {
							case responseChan <- responseContent:
								log.Debug().
									Str("session_id", sessionID).
									Str("request_id", requestID).
									Msg("Sent response to HTTP streaming channel")
							default:
								log.Debug().Msg("HTTP response channel full or closed")
							}
						}
					}
				}(helixSessionID, commentID, requestID, content)
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
			GenerationID:  helixSession.GenerationID, // Must match session's generation for query to find it
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
		apiServer.contextMappingsMutex.Lock()
		if apiServer.sessionToWaitingInteraction == nil {
			apiServer.sessionToWaitingInteraction = make(map[string]string)
		}
		apiServer.sessionToWaitingInteraction[helixSessionID] = createdInteraction.ID
		apiServer.contextMappingsMutex.Unlock()
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

// sendChatMessageToExternalAgent sends a chat message to an external agent session
// This is the proper way to send messages that trigger agent responses
func (apiServer *HelixAPIServer) sendChatMessageToExternalAgent(sessionID, message, requestID string) error {
	// Look up the session to get its ZedThreadID - we want to continue in the existing thread
	// instead of creating a new one. This maintains the 1:1 mapping between Zed threads and Helix sessions.
	var acpThreadID interface{} = nil
	session, err := apiServer.Controller.Options.Store.GetSession(context.Background(), sessionID)
	if err == nil && session != nil && session.Metadata.ZedThreadID != "" {
		acpThreadID = session.Metadata.ZedThreadID
		log.Info().
			Str("session_id", sessionID).
			Str("zed_thread_id", session.Metadata.ZedThreadID).
			Msg("🔗 [HELIX] Using existing ZedThreadID for chat message")
	} else {
		log.Info().
			Str("session_id", sessionID).
			Msg("🆕 [HELIX] No ZedThreadID found, will create new thread")
	}

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":       message,
			"request_id":    requestID,
			"acp_thread_id": acpThreadID, // Use existing thread if available, nil = create new
		},
	}

	return apiServer.sendCommandToExternalAgent(sessionID, command)
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

// Session Readiness Management
// These methods track whether the agent in a session is ready to receive messages.
// This prevents race conditions where we send prompts before Zed has loaded the agent.

// initReadinessState initializes readiness tracking for a session
// Returns a callback that should be called when agent_ready is received (or on timeout)
func (manager *ExternalAgentWSManager) initReadinessState(sessionID string, needsContinue bool, onReady func()) {
	manager.readinessMu.Lock()
	defer manager.readinessMu.Unlock()

	// Clean up any existing state for this session
	if existing, exists := manager.readinessState[sessionID]; exists && existing.TimeoutTimer != nil {
		existing.TimeoutTimer.Stop()
	}

	state := &SessionReadinessState{
		IsReady:       false,
		SessionID:     sessionID,
		PendingQueue:  make([]types.ExternalAgentCommand, 0),
		NeedsContinue: needsContinue,
	}

	// Set up fallback timeout (60 seconds)
	// If we don't receive agent_ready within 60s, assume ready and send anyway
	state.TimeoutTimer = time.AfterFunc(60*time.Second, func() {
		log.Warn().
			Str("session_id", sessionID).
			Msg("⏰ [READINESS] Timeout waiting for agent_ready, proceeding with queued messages")
		manager.markSessionReady(sessionID, onReady)
	})

	manager.readinessState[sessionID] = state

	log.Info().
		Str("session_id", sessionID).
		Bool("needs_continue", needsContinue).
		Msg("🔧 [READINESS] Initialized readiness tracking for session (waiting for agent_ready)")
}

// markSessionReady marks a session as ready and flushes pending messages
func (manager *ExternalAgentWSManager) markSessionReady(sessionID string, onReady func()) {
	manager.readinessMu.Lock()

	state, exists := manager.readinessState[sessionID]
	if !exists {
		manager.readinessMu.Unlock()
		log.Debug().Str("session_id", sessionID).Msg("No readiness state found for session")
		return
	}

	if state.IsReady {
		manager.readinessMu.Unlock()
		log.Debug().Str("session_id", sessionID).Msg("Session already marked as ready")
		return
	}

	// Stop the timeout timer
	if state.TimeoutTimer != nil {
		state.TimeoutTimer.Stop()
	}

	state.IsReady = true
	state.ReadyAt = time.Now()
	pendingQueue := state.PendingQueue
	state.PendingQueue = nil // Clear the queue

	manager.readinessMu.Unlock()

	log.Info().
		Str("session_id", sessionID).
		Int("pending_count", len(pendingQueue)).
		Msg("✅ [READINESS] Session marked as ready, flushing pending messages")

	// Flush pending messages (after releasing lock to avoid deadlock)
	if len(pendingQueue) > 0 {
		conn, exists := manager.getConnection(sessionID)
		if exists {
			for _, cmd := range pendingQueue {
				select {
				case conn.SendChan <- cmd:
					log.Debug().
						Str("session_id", sessionID).
						Str("type", cmd.Type).
						Msg("📤 [READINESS] Sent queued message")
				default:
					log.Warn().
						Str("session_id", sessionID).
						Str("type", cmd.Type).
						Msg("⚠️ [READINESS] SendChan full, dropped queued message")
				}
			}
		}
	}

	// Call the onReady callback (e.g., to send continue prompt)
	if onReady != nil {
		onReady()
	}
}

// isSessionReady checks if a session is ready to receive messages
func (manager *ExternalAgentWSManager) isSessionReady(sessionID string) bool {
	manager.readinessMu.RLock()
	defer manager.readinessMu.RUnlock()

	state, exists := manager.readinessState[sessionID]
	if !exists {
		// No readiness tracking = assume ready (for backward compatibility)
		return true
	}
	return state.IsReady
}

// queueOrSend queues a command if session isn't ready, or sends immediately if ready
func (manager *ExternalAgentWSManager) queueOrSend(sessionID string, cmd types.ExternalAgentCommand) bool {
	manager.readinessMu.Lock()
	state, exists := manager.readinessState[sessionID]
	if !exists || state.IsReady {
		manager.readinessMu.Unlock()
		// Session is ready or not tracked - send immediately
		conn, connExists := manager.getConnection(sessionID)
		if !connExists {
			log.Warn().Str("session_id", sessionID).Msg("No connection found for session")
			return false
		}
		select {
		case conn.SendChan <- cmd:
			return true
		default:
			log.Warn().Str("session_id", sessionID).Msg("SendChan full, could not send command")
			return false
		}
	}

	// Session not ready - queue the message
	state.PendingQueue = append(state.PendingQueue, cmd)
	manager.readinessMu.Unlock()

	log.Debug().
		Str("session_id", sessionID).
		Str("type", cmd.Type).
		Int("queue_size", len(state.PendingQueue)).
		Msg("📥 [READINESS] Queued message (waiting for agent_ready)")
	return true
}

// cleanupReadinessState removes readiness tracking for a session
func (manager *ExternalAgentWSManager) cleanupReadinessState(sessionID string) {
	manager.readinessMu.Lock()
	defer manager.readinessMu.Unlock()

	if state, exists := manager.readinessState[sessionID]; exists {
		if state.TimeoutTimer != nil {
			state.TimeoutTimer.Stop()
		}
		delete(manager.readinessState, sessionID)
		log.Debug().Str("session_id", sessionID).Msg("Cleaned up readiness state")
	}
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

	// Try to link agent response to design review comment (if this request came from a comment)
	go func() {
		if err := apiServer.linkAgentResponseToCommentByRequestID(context.Background(), requestID, content); err != nil {
			log.Debug().
				Err(err).
				Str("request_id", requestID).
				Msg("No design review comment linked to this request (this is normal for non-comment requests)")
		}
	}()

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
	apiServer.contextMappingsMutex.RLock()
	helixSessionID, ok := apiServer.contextMappings[acpThreadID]
	apiServer.contextMappingsMutex.RUnlock()
	if !ok {
		// FALLBACK: contextMappings may be empty after API restart
		// Try to find session by ZedThreadID in database
		log.Info().
			Str("acp_thread_id", acpThreadID).
			Msg("🔍 [HELIX] contextMappings miss in message_completed, attempting database fallback")

		foundSession, err := apiServer.findSessionByZedThreadID(context.Background(), acpThreadID)
		if err != nil || foundSession == nil {
			log.Warn().
				Str("acp_thread_id", acpThreadID).
				Msg("⚠️ [HELIX] No Helix session mapping found for this thread (database fallback failed) - skipping message_completed")
			return nil
		}
		helixSessionID = foundSession.ID
		// Restore the mapping for future messages
		apiServer.contextMappingsMutex.Lock()
		apiServer.contextMappings[acpThreadID] = helixSessionID
		apiServer.contextMappingsMutex.Unlock()
		log.Info().
			Str("acp_thread_id", acpThreadID).
			Str("helix_session_id", helixSessionID).
			Msg("✅ [HELIX] Found session via database fallback in message_completed, restored contextMappings")
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

	// CRITICAL: Publish final session update to frontend so it gets the complete state
	// Without this, the frontend never receives the final update with state=complete
	helixSession.Interactions = append(helixSession.Interactions[:0], targetInteraction)
	err = apiServer.publishSessionUpdateToFrontend(helixSession, targetInteraction)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", helixSessionID).
			Str("interaction_id", targetInteraction.ID).
			Msg("Failed to publish final session update to frontend")
	}

	// FINALIZE COMMENT RESPONSE
	// PRIMARY APPROACH: Use request_id from message data (echoed back by agent)
	// This is the definitive link to the comment and doesn't rely on session ID matching
	// FALLBACK: Session-based lookup (for backwards compatibility with agents that don't echo request_id)

	messageRequestID, hasRequestID := syncMsg.Data["request_id"].(string)
	if hasRequestID && messageRequestID != "" {
		// PRIMARY: Use request_id from message data directly
		log.Info().
			Str("request_id", messageRequestID).
			Str("helix_session_id", helixSessionID).
			Msg("🎯 [HELIX] Using request_id from message_completed data to finalize comment")

		go func(sessionID, requestID string) {
			if err := apiServer.finalizeCommentResponse(context.Background(), requestID); err != nil {
				log.Debug().
					Err(err).
					Str("request_id", requestID).
					Msg("No comment found for request_id (this is normal for non-comment interactions)")
				return
			}

			log.Info().
				Str("session_id", sessionID).
				Str("request_id", requestID).
				Msg("✅ [HELIX] Finalized comment response via request_id from message data")

			// Send completion signal to done channel for HTTP streaming clients
			_, doneChan, _, exists := apiServer.getResponseChannel(sessionID, requestID)
			if exists {
				select {
				case doneChan <- true:
					log.Debug().Str("request_id", requestID).Msg("Sent done signal to channel")
				default:
					log.Debug().Msg("Done channel full")
				}
			}
		}(helixSessionID, messageRequestID)
	} else {
		// FALLBACK: Session-based lookup (for agents that don't echo request_id)
		// This may fail if helixSessionID != planning_session_id, but we try anyway
		log.Debug().
			Str("helix_session_id", helixSessionID).
			Msg("No request_id in message_completed data, falling back to session-based lookup")

		pendingComment, err := apiServer.Store.GetPendingCommentByPlanningSessionID(context.Background(), helixSessionID)
		if err == nil && pendingComment != nil {
			requestID := pendingComment.RequestID
			commentID := pendingComment.ID

			go func(sessionID, commentID, requestID string) {
				if err := apiServer.finalizeCommentResponse(context.Background(), requestID); err != nil {
					log.Error().
						Err(err).
						Str("comment_id", commentID).
						Str("request_id", requestID).
						Msg("Failed to finalize comment response")
					return
				}

				log.Info().
					Str("comment_id", commentID).
					Str("session_id", sessionID).
					Str("request_id", requestID).
					Msg("✅ [HELIX] Finalized comment response via session-based lookup (fallback)")

				_, doneChan, _, exists := apiServer.getResponseChannel(sessionID, requestID)
				if exists {
					select {
					case doneChan <- true:
						log.Debug().Str("request_id", requestID).Msg("Sent done signal to channel")
					default:
						log.Debug().Msg("Done channel full")
					}
				}
			}(helixSessionID, commentID, requestID)
		} else {
			log.Debug().
				Str("session_id", helixSessionID).
				Msg("No pending design review comment to finalize for session (this is normal for non-comment interactions)")
		}
	}

	// Process next non-interrupt prompt from queue (if any)
	go apiServer.processPromptQueue(context.Background(), helixSessionID)

	return nil
}

// processPromptQueue checks for pending non-interrupt prompts and sends the next one
// This is called after a message is completed to process queued non-interrupt messages
func (apiServer *HelixAPIServer) processPromptQueue(ctx context.Context, sessionID string) {
	// Get the next pending non-interrupt prompt for this session
	nextPrompt, err := apiServer.Store.GetNextPendingPrompt(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get next pending prompt")
		return
	}

	if nextPrompt == nil {
		log.Debug().Str("session_id", sessionID).Msg("No pending non-interrupt prompts in queue")
		return
	}

	isRetry := nextPrompt.Status == "failed"
	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Str("content_preview", truncateString(nextPrompt.Content, 50)).
		Bool("is_retry", isRetry).
		Msg("📤 [QUEUE] Processing next non-interrupt prompt from queue")

	// Mark as pending before sending (in case it was 'failed', this prevents race conditions)
	if err := apiServer.Store.MarkPromptAsPending(ctx, nextPrompt.ID); err != nil {
		log.Error().Err(err).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as pending before send")
	}

	// Send the prompt to the session
	err = apiServer.sendQueuedPromptToSession(ctx, sessionID, nextPrompt)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("prompt_id", nextPrompt.ID).
			Msg("Failed to send queued prompt to session")

		// Mark as failed
		if markErr := apiServer.Store.MarkPromptAsFailed(ctx, nextPrompt.ID); markErr != nil {
			log.Error().Err(markErr).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as failed")
		}
		return
	}

	// Mark as sent
	if err := apiServer.Store.MarkPromptAsSent(ctx, nextPrompt.ID); err != nil {
		log.Error().Err(err).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as sent")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Msg("✅ [QUEUE] Successfully sent queued prompt to session")
}

// processAnyPendingPrompt checks for any pending prompt (interrupt or non-interrupt) and sends it
// This is used when the session is idle to process ALL pending prompts, not just non-interrupt ones
func (apiServer *HelixAPIServer) processAnyPendingPrompt(ctx context.Context, sessionID string) {
	// Get the next pending prompt (any type)
	nextPrompt, err := apiServer.Store.GetAnyPendingPrompt(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get any pending prompt")
		return
	}

	if nextPrompt == nil {
		log.Debug().Str("session_id", sessionID).Msg("No pending prompts in queue")
		return
	}

	isRetry := nextPrompt.Status == "failed"
	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Str("content_preview", truncateString(nextPrompt.Content, 50)).
		Bool("interrupt", nextPrompt.Interrupt).
		Bool("is_retry", isRetry).
		Msg("📤 [QUEUE] Processing pending prompt")

	// CRITICAL: Mark as 'sent' IMMEDIATELY to prevent race conditions.
	// Once we start processing, mark it done so no other process picks it up.
	if err := apiServer.Store.MarkPromptAsSent(ctx, nextPrompt.ID); err != nil {
		log.Error().Err(err).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as sent")
		// Continue anyway - better to risk duplicate than lose the message
	}

	// Send the prompt to the session (creates interaction and sends to agent)
	if err := apiServer.sendQueuedPromptToSession(ctx, sessionID, nextPrompt); err != nil {
		// Interaction creation failed - revert to 'failed' so it can be retried
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("prompt_id", nextPrompt.ID).
			Msg("Failed to create interaction for pending prompt - reverting to failed")
		if markErr := apiServer.Store.MarkPromptAsFailed(ctx, nextPrompt.ID); markErr != nil {
			log.Error().Err(markErr).Str("prompt_id", nextPrompt.ID).Msg("Failed to mark prompt as failed after interaction creation error")
		}
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("prompt_id", nextPrompt.ID).
		Bool("interrupt", nextPrompt.Interrupt).
		Msg("✅ [QUEUE] Successfully processed pending prompt")
}

// sendQueuedPromptToSession sends a queued prompt to an external agent session
// CRITICAL: Creates an interaction BEFORE sending so that agent responses have somewhere to go
func (apiServer *HelixAPIServer) sendQueuedPromptToSession(ctx context.Context, sessionID string, prompt *types.PromptHistoryEntry) error {
	// Get the session to retrieve the ZedThreadID and owner
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Check if session has required metadata for queue processing
	// External agent sessions need ZedThreadID to route messages
	if session.Metadata.ZedThreadID == "" {
		return fmt.Errorf("session %s has no ZedThreadID - cannot process queued prompt (is this an external agent session?)", sessionID)
	}

	// CRITICAL: Create an interaction BEFORE sending the message
	// This ensures that when the agent responds, handleMessageAdded has an interaction to update
	interaction := &types.Interaction{
		ID:            "", // Will be generated
		Created:       time.Now(),
		Updated:       time.Now(),
		Scheduled:     time.Now(),
		SessionID:     sessionID,
		UserID:        session.Owner,
		GenerationID:  session.GenerationID, // Must match session's generation for query to find it
		Mode:          types.SessionModeInference,
		PromptMessage: prompt.Content,
		State:         types.InteractionStateWaiting,
	}

	createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(ctx, interaction)
	if err != nil {
		return fmt.Errorf("failed to create interaction for queue prompt: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("interaction_id", createdInteraction.ID).
		Str("content_preview", truncateString(prompt.Content, 30)).
		Msg("✅ [QUEUE] Created interaction for queue prompt")

	// CRITICAL: Store the mapping so handleMessageAdded can find this interaction
	apiServer.contextMappingsMutex.Lock()
	if apiServer.sessionToWaitingInteraction == nil {
		apiServer.sessionToWaitingInteraction = make(map[string]string)
	}
	apiServer.sessionToWaitingInteraction[sessionID] = createdInteraction.ID
	apiServer.contextMappingsMutex.Unlock()

	// Determine agent name
	agentName := apiServer.getAgentNameForSession(ctx, session)

	// Use interaction ID as request ID for better tracing
	requestID := createdInteraction.ID

	// Create the command to send to the external agent
	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"acp_thread_id": session.Metadata.ZedThreadID,
			"message":       prompt.Content,
			"request_id":    requestID,
			"agent_name":    agentName,
			"from_queue":    true, // Indicate this came from the queue
		},
	}

	log.Info().
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Str("interaction_id", createdInteraction.ID).
		Str("acp_thread_id", session.Metadata.ZedThreadID).
		Str("content_preview", truncateString(prompt.Content, 30)).
		Msg("📤 [QUEUE] Sending queued prompt via sendCommandToExternalAgent")

	// Use the unified sendCommandToExternalAgent which handles connection lookup,
	// adds session_id to data, and updates agent work state
	//
	// IMPORTANT: We don't return error if sending to agent fails, because the
	// interaction was already created. The queue's job is to persist the message
	// to the backend - which succeeded. Agent send failures are logged but don't
	// affect the prompt status. The user will see the interaction in the session
	// and can retry if needed.
	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		log.Error().Err(err).
			Str("session_id", sessionID).
			Str("interaction_id", createdInteraction.ID).
			Str("prompt_id", prompt.ID).
			Msg("❌ [QUEUE] Failed to send to agent, but interaction was created - prompt will be marked as sent")
	}

	return nil
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// handleThreadLoadError handles thread load failures from Zed
// This happens when Zed tries to load an existing thread but fails (e.g., session already active via UI)
// We need to treat this like a completion so the UI clears the text box and shows an error
func (apiServer *HelixAPIServer) handleThreadLoadError(sessionID string, syncMsg *types.SyncMessage) error {
	log.Warn().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Interface("data", syncMsg.Data).
		Msg("⚠️ [HELIX] RECEIVED THREAD_LOAD_ERROR FROM EXTERNAL AGENT")

	// Extract error details
	acpThreadID, _ := syncMsg.Data["acp_thread_id"].(string)
	requestID, _ := syncMsg.Data["request_id"].(string)
	errorMsg, _ := syncMsg.Data["error"].(string)

	log.Error().
		Str("acp_thread_id", acpThreadID).
		Str("request_id", requestID).
		Str("error", errorMsg).
		Msg("❌ [HELIX] Thread load failed in Zed - session may be active via UI click")

	// Look up helix_session_id from context mapping (if thread was previously mapped)
	var helixSessionID string
	if acpThreadID != "" {
		apiServer.contextMappingsMutex.RLock()
		helixSessionID = apiServer.contextMappings[acpThreadID]
		apiServer.contextMappingsMutex.RUnlock()
	}

	// If we have a request_id, try to send error to the done channel
	// This allows the HTTP streaming to complete with an error message
	if requestID != "" {
		lookupSessionID := helixSessionID
		if lookupSessionID == "" {
			// Fall back to the WebSocket session ID
			lookupSessionID = sessionID
		}

		_, doneChan, errorChan, exists := apiServer.getResponseChannel(lookupSessionID, requestID)
		if exists {
			// Send error message
			if errorChan != nil {
				select {
				case errorChan <- fmt.Errorf("thread load failed: %s", errorMsg):
					log.Info().
						Str("request_id", requestID).
						Msg("✅ [HELIX] Sent error to error channel")
				default:
					log.Debug().Msg("Error channel full")
				}
			}

			// Send completion signal so UI clears
			if doneChan != nil {
				select {
				case doneChan <- true:
					log.Info().
						Str("request_id", requestID).
						Msg("✅ [HELIX] Sent done signal (after error)")
				default:
					log.Debug().Msg("Done channel full")
				}
			}
		}
	}

	// If we have a helix session, update the interaction to show error
	if helixSessionID != "" {
		helixSession, err := apiServer.Controller.Options.Store.GetSession(context.Background(), helixSessionID)
		if err == nil && helixSession != nil {
			// Find the waiting interaction and mark it with error
			interactions, _, err := apiServer.Controller.Options.Store.ListInteractions(context.Background(), &types.ListInteractionsQuery{
				SessionID:    helixSessionID,
				GenerationID: helixSession.GenerationID,
				PerPage:      1000,
			})
			if err == nil {
				for i := len(interactions) - 1; i >= 0; i-- {
					if interactions[i].State == types.InteractionStateWaiting {
						interactions[i].State = types.InteractionStateError
						interactions[i].Error = fmt.Sprintf("Thread load failed: %s", errorMsg)
						interactions[i].Updated = time.Now()
						interactions[i].Completed = time.Now()
						apiServer.Controller.Options.Store.UpdateInteraction(context.Background(), interactions[i])

						log.Info().
							Str("helix_session_id", helixSessionID).
							Str("interaction_id", interactions[i].ID).
							Msg("✅ [HELIX] Marked interaction as error due to thread load failure")
						break
					}
				}
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

// handleAgentReady processes the agent_ready event from Zed
// This is sent when the agent (e.g., qwen-code) has finished initialization and is ready for prompts
func (apiServer *HelixAPIServer) handleAgentReady(sessionID string, syncMsg *types.SyncMessage) error {
	log.Info().
		Str("session_id", sessionID).
		Interface("data", syncMsg.Data).
		Msg("🚀 [READINESS] Received agent_ready event from Zed")

	// Extract optional metadata from the ready event
	agentName, _ := syncMsg.Data["agent_name"].(string)
	threadID, _ := syncMsg.Data["thread_id"].(string)

	// Mark the session as ready, which will:
	// 1. Flush any queued messages
	// 2. Trigger the onReady callback (which sends continue prompt if needed)
	apiServer.externalAgentWSManager.readinessMu.RLock()
	state, exists := apiServer.externalAgentWSManager.readinessState[sessionID]
	apiServer.externalAgentWSManager.readinessMu.RUnlock()

	if !exists {
		// No readiness tracking for this session - that's fine, just log
		log.Debug().
			Str("session_id", sessionID).
			Msg("No readiness state found for session (may be already ready or legacy connection)")
		return nil
	}

	// Get the connection for sending continue prompt
	wsConn, connExists := apiServer.externalAgentWSManager.getConnection(sessionID)

	// Create the onReady callback that will send the continue prompt
	var onReadyCallback func()
	if state.NeedsContinue && connExists {
		onReadyCallback = func() {
			log.Info().
				Str("session_id", sessionID).
				Str("agent_name", agentName).
				Str("thread_id", threadID).
				Msg("🔄 [READINESS] Agent ready, now sending continue prompt")
			apiServer.sendContinuePromptIfNeeded(context.Background(), sessionID, wsConn)
		}
	}

	// Mark as ready (this flushes queued messages and calls onReady)
	apiServer.externalAgentWSManager.markSessionReady(sessionID, onReadyCallback)

	// Process any pending prompts (including interrupt=true ones)
	// When agent is ready/idle, we should process ALL pending prompts, not just non-interrupt ones
	go apiServer.processAnyPendingPrompt(context.Background(), sessionID)

	return nil
}

// findSessionByZedThreadID finds a session by its ZedThreadID metadata
// This is a database fallback when contextMappings is empty (e.g., after API restart)
func (apiServer *HelixAPIServer) findSessionByZedThreadID(ctx context.Context, zedThreadID string) (*types.Session, error) {
	// Query sessions with matching ZedThreadID in metadata
	// The ZedThreadID is stored in session.Metadata.ZedThreadID
	// For now, we iterate through recent sessions (this could be optimized with a DB index on metadata)
	sessions, _, err := apiServer.Controller.Options.Store.ListSessions(ctx, store.ListSessionsQuery{
		PerPage: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	for _, session := range sessions {
		if session.Metadata.ZedThreadID == zedThreadID {
			log.Info().
				Str("session_id", session.ID).
				Str("zed_thread_id", zedThreadID).
				Msg("🔍 [HELIX] Found session by ZedThreadID in database")
			return session, nil
		}
	}

	return nil, fmt.Errorf("no session found with ZedThreadID: %s", zedThreadID)
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
// If requestID is provided, it will also publish to the commenter's queue (for design review streaming)
func (apiServer *HelixAPIServer) publishSessionUpdateToFrontend(session *types.Session, interaction *types.Interaction, requestID ...string) error {
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

	// Publish to session owner's queue
	err = apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(session.Owner, session.ID), messageBytes)
	if err != nil {
		return fmt.Errorf("failed to publish to pubsub: %w", err)
	}

	log.Info().
		Str("session_id", session.ID).
		Str("interaction_id", interaction.ID).
		Str("owner", session.Owner).
		Msg("📤 [HELIX] Published session update to frontend (owner)")

	// If requestID is provided, check if there's a commenter who should also receive the update
	// This handles the case where the design review commenter is different from the session owner
	if len(requestID) > 0 && requestID[0] != "" {
		if apiServer.requestToCommenterMapping != nil {
			if commenterID, exists := apiServer.requestToCommenterMapping[requestID[0]]; exists && commenterID != session.Owner {
				// Publish to commenter's queue as well
				err = apiServer.pubsub.Publish(context.Background(), pubsub.GetSessionQueue(commenterID, session.ID), messageBytes)
				if err != nil {
					log.Warn().
						Err(err).
						Str("session_id", session.ID).
						Str("commenter_id", commenterID).
						Msg("Failed to publish session update to commenter")
				} else {
					log.Info().
						Str("session_id", session.ID).
						Str("interaction_id", interaction.ID).
						Str("commenter_id", commenterID).
						Msg("📤 [HELIX] Published session update to commenter")
				}
			}
		}
	}

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

	// IDEMPOTENCY: Check if we already have a session for this thread
	// This can happen if MessageAdded(role=user) arrived before UserCreatedThread
	// and created the session on-the-fly
	apiServer.contextMappingsMutex.RLock()
	existingMappedSession, alreadyExists := apiServer.contextMappings[acpThreadID]
	apiServer.contextMappingsMutex.RUnlock()

	if alreadyExists {
		log.Info().
			Str("acp_thread_id", acpThreadID).
			Str("existing_session_id", existingMappedSession).
			Msg("✅ [HELIX] Session already exists for thread (created on-the-fly), skipping creation")
		return nil
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
	apiServer.contextMappingsMutex.Lock()
	apiServer.contextMappings[acpThreadID] = session.ID
	apiServer.contextMappingsMutex.Unlock()

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
	apiServer.contextMappingsMutex.RLock()
	helixSessionID, exists := apiServer.contextMappings[acpThreadID]
	apiServer.contextMappingsMutex.RUnlock()

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

// sendContinuePromptIfNeeded checks agent work state and sends continue prompt if agent was working
// Called when WebSocket reconnects after container restart
func (apiServer *HelixAPIServer) sendContinuePromptIfNeeded(ctx context.Context, sessionID string, wsConn *ExternalAgentWSConnection) {
	log.Info().
		Str("session_id", sessionID).
		// Str("spec_task_id", activity.SpecTaskID).
		Msg("Agent was working before disconnect, sending continue prompt")

	// Get session for thread ID and agent config
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for continue prompt")
		return
	}

	// Determine agent name from session config
	agentName := apiServer.getAgentNameForSession(ctx, session)

	// Build continue prompt
	continueMessage := `The sandbox was restarted. Please continue working on your current task.

If you were in the middle of something, please resume from where you left off.
If you need to verify the current state, check the git status and any running processes.`

	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"message":       continueMessage,
			"request_id":    system.GenerateRequestID(),
			"acp_thread_id": session.Metadata.ZedThreadID,
			"agent_name":    agentName,
			"is_continue":   true, // Flag so agent knows this is a recovery prompt
		},
	}

	// Send via channel
	select {
	case wsConn.SendChan <- command:
		log.Info().
			Str("session_id", sessionID).
			Msg("Sent continue prompt to agent after reconnect")
	default:
		log.Warn().
			Str("session_id", sessionID).
			Msg("Failed to send continue prompt - channel full")
	}
}
