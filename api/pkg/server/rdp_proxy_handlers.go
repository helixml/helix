package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// RDPProxyConnection represents an active RDP proxy connection
// Proxies between frontend WebSocket and runner via NATS
type RDPProxyConnection struct {
	SessionID    string
	UserID       string
	WebSocket    *websocket.Conn // Frontend connection
	LastActivity time.Time
	mu           sync.RWMutex
}

// RDPProxyManager manages active RDP proxy connections
type RDPProxyManager struct {
	connections map[string]*RDPProxyConnection
	mu          sync.RWMutex
}

// Global proxy manager instance
var rdpProxyManager = &RDPProxyManager{
	connections: make(map[string]*RDPProxyConnection),
}

// WebSocket upgrader for RDP proxy
var rdpUpgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from same origin only for security
		origin := r.Header.Get("Origin")
		host := r.Header.Get("Host")
		return origin == "http://"+host || origin == "https://"+host || origin == ""
	},
}

// proxyExternalAgentRDP handles WebSocket connections for RDP proxy via NATS to runner
// GET /api/v1/external-agents/{sessionID}/rdp/proxy
func (s *HelixAPIServer) proxyExternalAgentRDP(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(w, "session ID required", http.StatusBadRequest)
		return
	}

	// Security check: verify user owns this session
	sessionData, err := s.Store.GetSession(r.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for ownership check")
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if sessionData.Owner != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("session_owner", sessionData.Owner).
			Str("session_id", sessionID).
			Msg("RDP access denied: user does not own session")
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	// Upgrade HTTP connection to WebSocket
	frontendConn, err := rdpUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to upgrade WebSocket connection")
		return
	}
	defer frontendConn.Close()

	// Register the connection for management
	proxyConn := &RDPProxyConnection{
		SessionID:    sessionID,
		UserID:       user.ID,
		WebSocket:    frontendConn,
		LastActivity: time.Now(),
	}
	rdpProxyManager.addConnection(sessionID, proxyConn)
	defer rdpProxyManager.removeConnection(sessionID)

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", user.ID).
		Msg("Starting RDP proxy via NATS to runner")

	// Start RDP proxy via NATS
	err = s.startRDPProxyViaNATS(r.Context(), sessionID, frontendConn)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", sessionID).
			Msg("RDP proxy via NATS failed")
	}

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", user.ID).
		Msg("RDP proxy connection closed")
}

// startRDPProxyViaNATS handles RDP data routing via NATS to the specific runner
func (s *HelixAPIServer) startRDPProxyViaNATS(ctx context.Context, sessionID string, frontendConn *websocket.Conn) error {
	// Subscribe to RDP responses from the runner for this session
	rdpResponseTopic := fmt.Sprintf("rdp.responses.%s", sessionID)

	rdpSub, err := s.pubsub.Subscribe(ctx, rdpResponseTopic, func(payload []byte) error {
		// Forward RDP data from runner to frontend as Guacamole protocol
		return s.forwardRDPDataToFrontend(frontendConn, payload)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to RDP responses: %w", err)
	}
	defer rdpSub.Unsubscribe()

	log.Info().
		Str("session_id", sessionID).
		Str("topic", rdpResponseTopic).
		Msg("Subscribed to RDP responses from runner")

	// Handle frontend messages and route to runner via NATS
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		frontendConn.SetReadDeadline(time.Now().Add(30 * time.Second))
		messageType, data, err := frontendConn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Str("session_id", sessionID).Msg("Frontend WebSocket error")
			}
			return err
		}

		// Route frontend data to runner via NATS
		if err := s.routeRDPDataToRunner(ctx, sessionID, messageType, data); err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to route RDP data to runner")
			return err
		}
	}
}

// routeRDPDataToRunner sends RDP data to the specific runner via NATS
func (s *HelixAPIServer) routeRDPDataToRunner(ctx context.Context, sessionID string, messageType int, data []byte) error {
	// Create RDP proxy message
	rdpMessage := types.ZedAgentRDPData{
		SessionID: sessionID,
		Type:      "rdp_frontend_data",
		Data:      data,
		Timestamp: time.Now().Unix(),
	}

	payload, err := json.Marshal(rdpMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal RDP message: %w", err)
	}

	// Send to the runner handling this session via NATS
	rdpTopic := fmt.Sprintf("rdp.commands.%s", sessionID)

	err = s.pubsub.Publish(ctx, rdpTopic, payload)
	if err != nil {
		return fmt.Errorf("failed to publish RDP data to runner: %w", err)
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("topic", rdpTopic).
		Int("data_size", len(data)).
		Msg("Routed RDP data to runner via NATS")

	return nil
}

// forwardRDPDataToFrontend converts RDP data from runner to Guacamole protocol for frontend
func (s *HelixAPIServer) forwardRDPDataToFrontend(frontendConn *websocket.Conn, payload []byte) error {
	var rdpData types.ZedAgentRDPData
	if err := json.Unmarshal(payload, &rdpData); err != nil {
		return fmt.Errorf("failed to unmarshal RDP data: %w", err)
	}

	// Convert RDP data to Guacamole protocol instruction
	// In a full implementation, this would parse RDP packets and convert to Guacamole
	guacamoleInstruction := s.convertRDPToGuacamole(rdpData.Data)

	// Send to frontend
	frontendConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	err := frontendConn.WriteMessage(websocket.TextMessage, []byte(guacamoleInstruction))
	if err != nil {
		return fmt.Errorf("failed to send Guacamole instruction to frontend: %w", err)
	}

	log.Debug().
		Str("session_id", rdpData.SessionID).
		Int("rdp_data_size", len(rdpData.Data)).
		Str("guacamole_instruction", guacamoleInstruction).
		Msg("Forwarded RDP data to frontend as Guacamole")

	return nil
}

// convertRDPToGuacamole converts RDP protocol data to Guacamole protocol instructions
func (s *HelixAPIServer) convertRDPToGuacamole(rdpData []byte) string {
	// This is a simplified RDP-to-Guacamole converter
	// In a full implementation, you would parse RDP packets properly

	if len(rdpData) == 0 {
		return "nop;"
	}

	// Simple heuristics for RDP packet types
	if len(rdpData) > 100 {
		// Likely bitmap update - convert to PNG instruction
		return fmt.Sprintf("png,0,0,0,%s;", "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")
	} else if len(rdpData) < 20 {
		// Likely cursor or control data
		return "cursor,0,0;"
	} else {
		// Medium size - likely screen region update
		return "rect,0,0,0,100,100;"
	}
}

// addConnection adds a new proxy connection
func (rpm *RDPProxyManager) addConnection(sessionID string, conn *RDPProxyConnection) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()
	rpm.connections[sessionID] = conn
}

// removeConnection removes a proxy connection
func (rpm *RDPProxyManager) removeConnection(sessionID string) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()
	if conn, exists := rpm.connections[sessionID]; exists {
		conn.WebSocket.Close()
		delete(rpm.connections, sessionID)
	}
}

// getConnectionStats returns statistics about active connections
func (rpm *RDPProxyManager) getConnectionStats() map[string]interface{} {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()

	stats := map[string]interface{}{
		"active_connections": len(rpm.connections),
		"connections":        make([]map[string]interface{}, 0, len(rpm.connections)),
	}

	for sessionID, conn := range rpm.connections {
		conn.mu.RLock()
		connInfo := map[string]interface{}{
			"session_id":    sessionID,
			"user_id":       conn.UserID,
			"last_activity": conn.LastActivity,
			"duration":      time.Since(conn.LastActivity).Seconds(),
		}
		conn.mu.RUnlock()

		stats["connections"] = append(stats["connections"].([]map[string]interface{}), connInfo)
	}

	return stats
}

// cleanupInactiveConnections removes connections that have been inactive
func (rpm *RDPProxyManager) cleanupInactiveConnections(timeout time.Duration) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	now := time.Now()
	toRemove := []string{}

	for sessionID, conn := range rpm.connections {
		conn.mu.RLock()
		if now.Sub(conn.LastActivity) > timeout {
			toRemove = append(toRemove, sessionID)
		}
		conn.mu.RUnlock()
	}

	for _, sessionID := range toRemove {
		if conn, exists := rpm.connections[sessionID]; exists {
			conn.WebSocket.Close()
			delete(rpm.connections, sessionID)

			log.Info().
				Str("session_id", sessionID).
				Str("user_id", conn.UserID).
				Msg("Cleaned up inactive RDP proxy connection")
		}
	}
}

// RDP proxy health check endpoint
func (s *HelixAPIServer) getRDPProxyHealth(w http.ResponseWriter, r *http.Request) {
	stats := rdpProxyManager.getConnectionStats()
	system.RespondJSON(w, map[string]interface{}{
		"status":     "healthy",
		"timestamp":  time.Now(),
		"statistics": stats,
	})
}

// Start background cleanup routine for RDP proxy connections
func (s *HelixAPIServer) startRDPProxyCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			// Clean up connections inactive for more than 30 minutes
			rdpProxyManager.cleanupInactiveConnections(30 * time.Minute)
		}
	}()

	log.Info().Msg("RDP proxy cleanup routine started")
}
