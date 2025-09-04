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

// handleExternalAgentSync handles WebSocket connections from external agents (Zed instances)
func (apiServer *HelixAPIServer) handleExternalAgentSync(res http.ResponseWriter, req *http.Request) {
	// Extract session ID from query parameters
	sessionID := req.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(res, "session_id is required", http.StatusBadRequest)
		return
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

	if !apiServer.validateExternalAgentToken(sessionID, token) {
		http.Error(res, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := apiServer.externalAgentWSManager.upgrader.Upgrade(res, req, nil)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to upgrade WebSocket")
		return
	}

	log.Info().Str("session_id", sessionID).Msg("External agent WebSocket connected")

	// Create connection wrapper
	wsConn := &ExternalAgentWSConnection{
		SessionID:   sessionID,
		Conn:        conn,
		ConnectedAt: time.Now(),
		LastPing:    time.Now(),
		SendChan:    make(chan types.ExternalAgentCommand, 100),
	}

	// Register connection
	apiServer.externalAgentWSManager.registerConnection(sessionID, wsConn)
	defer apiServer.externalAgentWSManager.unregisterConnection(sessionID)

	// Start goroutines for handling connection
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	// Start message sender
	go apiServer.handleExternalAgentSender(ctx, wsConn)

	// Handle incoming messages (blocking)
	apiServer.handleExternalAgentReceiver(ctx, wsConn)
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

	switch syncMsg.EventType {
	case "context_created":
		return apiServer.handleContextCreated(sessionID, syncMsg)
	case "message_added":
		return apiServer.handleMessageAdded(sessionID, syncMsg)
	case "message_updated":
		return apiServer.handleMessageUpdated(sessionID, syncMsg)
	case "context_title_changed":
		return apiServer.handleContextTitleChanged(sessionID, syncMsg)
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

	// TODO: Create corresponding interaction in Helix session
	// This would involve:
	// 1. Getting the session from the store
	// 2. Creating a new interaction with the context mapping
	// 3. Storing the context_id -> interaction_id mapping

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

	_, ok = syncMsg.Data["content"].(string)
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

	// TODO: Add message to corresponding Helix session interaction
	// This would involve:
	// 1. Finding the interaction that maps to this context_id
	// 2. Adding the message to the interaction
	// 3. Triggering AI response if this is a user message
	// 4. Storing message_id mapping for future updates

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
	apiServer.externalAgentWSManager.mu.RLock()
	conn, exists := apiServer.externalAgentWSManager.connections[sessionID]
	apiServer.externalAgentWSManager.mu.RUnlock()

	if !exists {
		return fmt.Errorf("no external agent connection for session %s", sessionID)
	}

	select {
	case conn.SendChan <- command:
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
