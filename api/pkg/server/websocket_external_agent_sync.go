package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

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
	// Extract agent ID from query parameters (optional - will generate one if not provided)
	agentID := req.URL.Query().Get("agent_id")
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

	// Register connection
	apiServer.externalAgentWSManager.registerConnection(agentID, wsConn)
	defer apiServer.externalAgentWSManager.unregisterConnection(agentID)

	// Start goroutines for handling connection
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	// Start message sender
	go apiServer.handleExternalAgentSender(ctx, wsConn)

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
	log.Debug().
		Str("session_id", sessionID).
		Str("event_type", syncMsg.EventType).
		Msg("Processing external agent sync message")

	// Process sync message directly
	switch syncMsg.EventType {
	case "context_created":
		return apiServer.handleContextCreated(sessionID, syncMsg)
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
	case "chat_response_error":
		return apiServer.handleChatResponseError(sessionID, syncMsg)
	default:
		log.Warn().Str("event_type", syncMsg.EventType).Msg("Unknown sync message type")
		return nil
	}
}

// handleContextCreated processes context creation from external agent
func (apiServer *HelixAPIServer) handleContextCreated(sessionID string, syncMsg *types.SyncMessage) error {
	contextID, ok := syncMsg.Data["context_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid context_id")
	}

	title, _ := syncMsg.Data["title"].(string)
	if title == "" {
		title = "New Conversation"
	}

	log.Info().
		Str("session_id", sessionID).
		Str("context_id", contextID).
		Str("title", title).
		Msg("External agent created new context")

	// Get the external agent session to get user information
	agentSession, err := apiServer.Controller.GetExternalAgentStatus(context.Background(), sessionID)
	if err != nil {
		return fmt.Errorf("failed to get external agent session: %w", err)
	}

	// Create a new Helix session for this Zed context
	helixSession := types.Session{
		ID:        "", // Will be generated
		Name:      title,
		Owner:     agentSession.UserID,
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
		Str("session_id", sessionID).
		Str("context_id", contextID).
		Str("helix_session_id", createdSession.ID).
		Str("title", title).
		Msg("Created Helix session for Zed context")

	// Store the context mapping for future message routing
	// We'll use a simple in-memory mapping for now - in production this should be persistent
	if apiServer.contextMappings == nil {
		apiServer.contextMappings = make(map[string]string)
	}
	apiServer.contextMappings[contextID] = createdSession.ID

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
	command := types.ExternalAgentCommand{
		Type: "chat_message",
		Data: map[string]interface{}{
			"session_id": sessionID,
			"message":    interaction.PromptMessage,
			"role":       "user",
			"request_id": interaction.ID, // Use interaction ID as request ID for response tracking
		},
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
	contextID, ok := syncMsg.Data["context_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid context_id")
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

	// Create interaction for this message
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
		Msg("Created Helix interaction for Zed message")

	// If this is a user message, the system will automatically process it
	// TODO: Hook into session completion to send responses back to Zed
	if role == "user" || role == "human" {
		log.Info().
			Str("helix_session_id", helixSessionID).
			Str("interaction_id", createdInteraction.ID).
			Msg("User message from Zed will be processed by Helix - response streaming to be implemented")
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

	apiServer.externalAgentWSManager.mu.RLock()
	connections := apiServer.externalAgentWSManager.connections
	apiServer.externalAgentWSManager.mu.RUnlock()

	if len(connections) == 0 {
		return fmt.Errorf("no external agent connections available")
	}

	// Broadcast to all connected Zed agents
	sentCount := 0
	for agentID, conn := range connections {
		select {
		case conn.SendChan <- command:
			sentCount++
			log.Debug().
				Str("agent_id", agentID).
				Str("session_id", sessionID).
				Str("command_type", command.Type).
				Msg("Sent command to external Zed agent")
		default:
			log.Warn().
				Str("agent_id", agentID).
				Str("session_id", sessionID).
				Msg("External agent send channel full, skipping")
		}
	}

	if sentCount == 0 {
		return fmt.Errorf("failed to send command to any external agent connections")
	}

	log.Info().
		Int("sent_count", sentCount).
		Int("total_connections", len(connections)).
		Str("session_id", sessionID).
		Str("command_type", command.Type).
		Msg("Broadcasted command to external Zed agents")

	return nil
}

// registerConnection registers a new external agent connection
func (manager *ExternalAgentWSManager) registerConnection(sessionID string, conn *ExternalAgentWSConnection) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.connections[sessionID] = conn
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

	// Handle response via legacy channel handling
	responseChan, doneChan, _, exists := apiServer.getResponseChannel(sessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for request")
		return nil
	}

	// Send content as single chunk
	select {
	case responseChan <- content:
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
	requestID, ok := syncMsg.Data["request_id"].(string)
	if !ok {
		log.Warn().Str("session_id", sessionID).Msg("Chat response done missing request_id")
		return nil
	}

	// Handle response completion via legacy channel handling
	_, doneChan, _, exists := apiServer.getResponseChannel(sessionID, requestID)
	if !exists {
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("No response channel found for done signal")
		return nil
	}

	// Send completion signal
	select {
	case doneChan <- true:
	default:
		log.Warn().Str("session_id", sessionID).Str("request_id", requestID).Msg("Done channel full")
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
