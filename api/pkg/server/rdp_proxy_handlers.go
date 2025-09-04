package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
)

// GuacamoleClient handles interactions with the Guacamole REST API
type GuacamoleClient struct {
	baseURL    string
	username   string
	password   string
	authToken  string
	dataSource string
	httpClient *http.Client
	mu         sync.RWMutex
}

// GuacamoleAuthRequest represents the authentication request to Guacamole
type GuacamoleAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// GuacamoleAuthResponse represents the authentication response from Guacamole
type GuacamoleAuthResponse struct {
	AuthToken            string   `json:"authToken"`
	Username             string   `json:"username"`
	DataSource           string   `json:"dataSource"`
	AvailableDataSources []string `json:"availableDataSources"`
}

// GuacamoleConnection represents a connection in Guacamole
type GuacamoleConnection struct {
	Name             string            `json:"name"`
	ParentIdentifier string            `json:"parentIdentifier"`
	Protocol         string            `json:"protocol"`
	Parameters       map[string]string `json:"parameters"`
	Attributes       map[string]string `json:"attributes"`
}

// GuacamoleConnectionResponse represents the response when creating a connection
type GuacamoleConnectionResponse struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
}

// NewGuacamoleClient creates a new Guacamole REST API client
func NewGuacamoleClient(baseURL, username, password string) *GuacamoleClient {
	return &GuacamoleClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		username:   username,
		password:   password,
		dataSource: "postgresql", // Default data source name
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// authenticate gets an auth token from Guacamole
func (gc *GuacamoleClient) authenticate() error {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	authReq := GuacamoleAuthRequest{
		Username: gc.username,
		Password: gc.password,
	}

	reqBody, err := json.Marshal(authReq)
	if err != nil {
		return fmt.Errorf("failed to marshal auth request: %w", err)
	}

	resp, err := gc.httpClient.Post(
		gc.baseURL+"/api/tokens",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to authenticate with Guacamole: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Guacamole authentication failed: %s (status: %d)", string(body), resp.StatusCode)
	}

	var authResp GuacamoleAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("failed to decode auth response: %w", err)
	}

	gc.authToken = authResp.AuthToken
	if authResp.DataSource != "" {
		gc.dataSource = authResp.DataSource
	}

	log.Info().
		Str("username", gc.username).
		Str("data_source", gc.dataSource).
		Msg("Successfully authenticated with Guacamole")

	return nil
}

// createRDPConnection creates a new RDP connection in Guacamole
func (gc *GuacamoleClient) createRDPConnection(sessionID, hostname string, port int, username, password string) (string, error) {
	if gc.authToken == "" {
		if err := gc.authenticate(); err != nil {
			return "", err
		}
	}

	connection := GuacamoleConnection{
		Name:             fmt.Sprintf("Helix-Session-%s", sessionID),
		ParentIdentifier: "ROOT",
		Protocol:         "rdp",
		Parameters: map[string]string{
			"hostname":                   hostname,
			"port":                       fmt.Sprintf("%d", port),
			"username":                   username,
			"password":                   password,
			"security":                   "any",
			"ignore-cert":                "true",
			"resize-method":              "reconnect",
			"enable-wallpaper":           "false",
			"enable-theming":             "false",
			"enable-font-smoothing":      "false",
			"enable-full-window-drag":    "false",
			"enable-desktop-composition": "false",
			"enable-menu-animations":     "false",
		},
		Attributes: map[string]string{
			"guac-full-screen": "false",
		},
	}

	reqBody, err := json.Marshal(connection)
	if err != nil {
		return "", fmt.Errorf("failed to marshal connection request: %w", err)
	}

	url := fmt.Sprintf("%s/api/session/data/%s/connections", gc.baseURL, gc.dataSource)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Guacamole-Token", gc.authToken)

	resp, err := gc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create Guacamole connection: %s (status: %d)", string(body), resp.StatusCode)
	}

	var connResp GuacamoleConnectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&connResp); err != nil {
		return "", fmt.Errorf("failed to decode connection response: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("connection_id", connResp.Identifier).
		Str("hostname", hostname).
		Int("port", port).
		Msg("Created Guacamole RDP connection")

	return connResp.Identifier, nil
}

// deleteConnection removes a connection from Guacamole
func (gc *GuacamoleClient) deleteConnection(connectionID string) error {
	if gc.authToken == "" {
		return fmt.Errorf("not authenticated")
	}

	url := fmt.Sprintf("%s/api/session/data/%s/connections/%s", gc.baseURL, gc.dataSource, connectionID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	req.Header.Set("Guacamole-Token", gc.authToken)

	resp, err := gc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Warn().
			Str("connection_id", connectionID).
			Int("status", resp.StatusCode).
			Str("response", string(body)).
			Msg("Failed to delete Guacamole connection")
	}

	return nil
}

// RDPProxyManager manages RDP proxy connections via Guacamole
type RDPProxyManager struct {
	guacamoleClient *GuacamoleClient
	pubsub          pubsub.PubSub
	connections     map[string]*RDPProxyConnection
	mu              sync.RWMutex
}

// RDPProxyConnection represents an active RDP proxy connection
type RDPProxyConnection struct {
	SessionID      string
	ConnectionID   string
	GuacamoleToken string
	LocalPort      int              // Local TCP port for this proxy
	ZedSession     *ZedAgentSession // Zed session info
	CreatedAt      time.Time
	LastActivity   time.Time
	listener       net.Listener       // TCP listener for cleanup
	cancel         context.CancelFunc // Cancel function to stop proxy
}

// NewRDPProxyManager creates a new RDP proxy manager
func NewRDPProxyManager(guacamoleURL, guacamoleUser, guacamolePass string, pubsub pubsub.PubSub) *RDPProxyManager {
	return &RDPProxyManager{
		guacamoleClient: NewGuacamoleClient(guacamoleURL, guacamoleUser, guacamolePass),
		pubsub:          pubsub,
		connections:     make(map[string]*RDPProxyConnection),
	}
}

// startRDPProxy handles RDP proxy WebSocket connections (now via local RDP proxy ports)
func (s *HelixAPIServer) startRDPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["session_id"]
	if sessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}

	// Verify user has access to this session
	session, err := s.Controller.Options.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if session.Owner != user.ID {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	// Check if session has external agent running
	if session.Type != types.SessionTypeText || session.Mode != types.SessionModeInference {
		http.Error(w, "session does not support RDP access", http.StatusBadRequest)
		return
	}

	// Get Zed agent session info
	zedSession, err := s.getZedSessionInfo(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get Zed session info")
		http.Error(w, "Zed session not found", http.StatusNotFound)
		return
	}

	// Start local RDP proxy for this session (if not already running)
	proxyPort, err := s.rdpProxyManager.startRDPProxy(sessionID, zedSession)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to start RDP proxy")
		http.Error(w, "failed to start RDP proxy", http.StatusInternalServerError)
		return
	}

	// Create or get Guacamole connection pointing to our API container proxy
	connectionID, err := s.rdpProxyManager.createOrGetConnection(sessionID, "api", proxyPort)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to create Guacamole connection")
		http.Error(w, "failed to create RDP connection", http.StatusInternalServerError)
		return
	}

	// Upgrade to WebSocket and proxy to Guacamole
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins in development
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade WebSocket connection")
		return
	}
	defer conn.Close()

	// Proxy WebSocket traffic to Guacamole
	s.proxyWebSocketToGuacamole(ctx, conn, connectionID, sessionID)
}

// startRDPProxy starts a local TCP RDP proxy for a Zed session
func (rpm *RDPProxyManager) startRDPProxy(sessionID string, zedSession *ZedAgentSession) (int, error) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	// Check if proxy already exists
	if existing, exists := rpm.connections[sessionID]; exists {
		existing.LastActivity = time.Now()
		return existing.LocalPort, nil
	}

	// Allocate a local port for this session's RDP proxy
	proxyPort := rpm.allocatePort()

	// Start TCP proxy server
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &RDPProxyConnection{
		SessionID:    sessionID,
		LocalPort:    proxyPort,
		ZedSession:   zedSession,
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		cancel:       cancel,
	}

	// Start the TCP server in a goroutine
	go rpm.runTCPProxy(ctx, proxy)

	// Store connection info
	rpm.connections[sessionID] = proxy

	log.Info().
		Str("session_id", sessionID).
		Int("local_port", proxyPort).
		Msg("Started RDP TCP proxy")

	return proxyPort, nil
}

// createOrGetConnection creates or retrieves a Guacamole connection
func (rpm *RDPProxyManager) createOrGetConnection(sessionID, hostname string, port int) (string, error) {
	// Create new Guacamole connection pointing to our local proxy
	connectionID, err := rpm.guacamoleClient.createRDPConnection(
		sessionID,
		hostname,
		port,
		"zed",    // Standard username for Zed sessions
		"zed123", // This would come from the actual session
	)
	if err != nil {
		return "", err
	}

	log.Info().
		Str("session_id", sessionID).
		Str("connection_id", connectionID).
		Str("proxy_target", fmt.Sprintf("%s:%d", hostname, port)).
		Msg("Created Guacamole connection to local RDP proxy")

	return connectionID, nil
}

// proxyWebSocketToGuacamole proxies WebSocket traffic between frontend and Guacamole
func (s *HelixAPIServer) proxyWebSocketToGuacamole(ctx context.Context, frontendConn *websocket.Conn, connectionID, sessionID string) {
	// Build Guacamole WebSocket URL
	guacamoleWS := fmt.Sprintf("ws://guacamole-client:8080/guacamole/websocket-tunnel?token=%s&GUAC_DATA_SOURCE=%s&GUAC_ID=%s&GUAC_TYPE=c&GUAC_WIDTH=1024&GUAC_HEIGHT=768&GUAC_DPI=96",
		url.QueryEscape(s.rdpProxyManager.guacamoleClient.authToken),
		url.QueryEscape(s.rdpProxyManager.guacamoleClient.dataSource),
		url.QueryEscape(connectionID),
	)

	// Connect to Guacamole WebSocket
	guacamoleConn, _, err := websocket.DefaultDialer.Dial(guacamoleWS, nil)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to connect to Guacamole WebSocket")
		frontendConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to RDP server"))
		return
	}
	defer guacamoleConn.Close()

	log.Info().
		Str("session_id", sessionID).
		Str("connection_id", connectionID).
		Msg("Established RDP proxy connection via Guacamole")

	// Set up bidirectional proxy
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Proxy frontend -> Guacamole
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				messageType, data, err := frontendConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Error().Err(err).Str("session_id", sessionID).Msg("Error reading from frontend WebSocket")
					}
					return
				}

				if err := guacamoleConn.WriteMessage(messageType, data); err != nil {
					log.Error().Err(err).Str("session_id", sessionID).Msg("Error writing to Guacamole WebSocket")
					return
				}
			}
		}
	}()

	// Proxy Guacamole -> frontend
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				messageType, data, err := guacamoleConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Error().Err(err).Str("session_id", sessionID).Msg("Error reading from Guacamole WebSocket")
					}
					return
				}

				if err := frontendConn.WriteMessage(messageType, data); err != nil {
					log.Error().Err(err).Str("session_id", sessionID).Msg("Error writing to frontend WebSocket")
					return
				}
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	log.Info().
		Str("session_id", sessionID).
		Str("connection_id", connectionID).
		Msg("RDP proxy connection closed")
}

// ZedAgentSession represents a Zed agent session for RDP access
type ZedAgentSession struct {
	SessionID   string `json:"session_id"`
	RDPPort     int    `json:"rdp_port"`
	RDPUsername string `json:"rdp_username"`
	RDPPassword string `json:"rdp_password"`
	Status      string `json:"status"`
}

// runTCPProxy runs a TCP proxy that forwards RDP traffic via NATS
func (rpm *RDPProxyManager) runTCPProxy(ctx context.Context, proxy *RDPProxyConnection) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", proxy.LocalPort))
	if err != nil {
		log.Error().Err(err).
			Str("session_id", proxy.SessionID).
			Int("port", proxy.LocalPort).
			Msg("Failed to start TCP proxy listener")
		return
	}
	defer listener.Close()

	// Store listener for cleanup
	proxy.listener = listener

	log.Info().
		Str("session_id", proxy.SessionID).
		Int("port", proxy.LocalPort).
		Msg("RDP TCP proxy listening")

	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("session_id", proxy.SessionID).
				Int("port", proxy.LocalPort).
				Msg("RDP TCP proxy shutting down")
			return
		default:
		}

		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				// Expected error due to context cancellation
				return
			default:
				log.Error().Err(err).
					Str("session_id", proxy.SessionID).
					Msg("Failed to accept TCP connection")
				continue
			}
		}

		// Handle each connection in a goroutine
		go rpm.handleTCPConnection(ctx, proxy, conn)
	}
}

// handleTCPConnection handles a single TCP connection by proxying over NATS
func (rpm *RDPProxyManager) handleTCPConnection(ctx context.Context, proxy *RDPProxyConnection, conn net.Conn) {
	defer conn.Close()

	log.Info().
		Str("session_id", proxy.SessionID).
		Str("remote_addr", conn.RemoteAddr().String()).
		Msg("New RDP TCP connection")

	// Set up NATS topics for this connection
	rdpCommandTopic := fmt.Sprintf("rdp.commands.%s", proxy.SessionID)
	rdpResponseTopic := fmt.Sprintf("rdp.responses.%s", proxy.SessionID)

	// Subscribe to RDP responses from the Zed container
	connCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to NATS responses and forward to TCP connection
	rdpSub, err := rpm.pubsub.Subscribe(connCtx, rdpResponseTopic, func(payload []byte) error {
		var rdpData types.ZedAgentRDPData
		if err := json.Unmarshal(payload, &rdpData); err != nil {
			log.Error().Err(err).Str("session_id", proxy.SessionID).Msg("Failed to unmarshal RDP response")
			return err
		}

		// Forward RDP data from Zed container to TCP connection
		_, writeErr := conn.Write(rdpData.Data)
		if writeErr != nil {
			log.Error().Err(writeErr).Str("session_id", proxy.SessionID).Msg("Failed to write RDP data to TCP connection")
			return writeErr
		}

		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("session_id", proxy.SessionID).Msg("Failed to subscribe to RDP responses")
		return
	}
	defer rdpSub.Unsubscribe()

	// Handle the TCP connection lifecycle
	buffer := make([]byte, 4096)
	for {
		select {
		case <-connCtx.Done():
			log.Debug().
				Str("session_id", proxy.SessionID).
				Msg("TCP connection context cancelled")
			return
		default:
		}

		// Set read timeout to allow context cancellation
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := conn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout, check context and continue
				continue
			}
			log.Debug().Err(err).
				Str("session_id", proxy.SessionID).
				Msg("TCP connection closed")
			break
		}

		// Forward RDP data to Zed container via NATS
		rdpMessage := types.ZedAgentRDPData{
			SessionID: proxy.SessionID,
			Type:      "rdp_data",
			Data:      buffer[:n],
			Timestamp: time.Now().Unix(),
		}

		// Publish RDP data to NATS for forwarding to Zed container
		messageData, err := json.Marshal(rdpMessage)
		if err != nil {
			log.Error().Err(err).Str("session_id", proxy.SessionID).Msg("Failed to marshal RDP message")
			continue
		}

		err = rpm.pubsub.Publish(connCtx, rdpCommandTopic, messageData)
		if err != nil {
			log.Error().Err(err).Str("session_id", proxy.SessionID).Msg("Failed to publish RDP data to NATS")
			continue
		}

		log.Debug().
			Str("session_id", proxy.SessionID).
			Int("bytes", n).
			Msg("Forwarded RDP data to NATS")
	}
}

// allocatePort allocates a free local port for RDP proxy
func (rpm *RDPProxyManager) allocatePort() int {
	// Simple port allocation - start from 15900 and increment
	// In production, this should check for availability
	basePort := 15900
	for i := 0; i < 100; i++ {
		port := basePort + i
		// Check if port is already in use
		inUse := false
		for _, conn := range rpm.connections {
			if conn.LocalPort == port {
				inUse = true
				break
			}
		}
		if !inUse {
			return port
		}
	}
	// Fallback to a high port
	return basePort + len(rpm.connections)
}

// getZedSessionInfo retrieves Zed session information (placeholder implementation)
func (s *HelixAPIServer) getZedSessionInfo(ctx context.Context, sessionID string) (*ZedAgentSession, error) {
	// This would need to be implemented to retrieve actual Zed session info
	// For now, return a placeholder
	return &ZedAgentSession{
		SessionID:   sessionID,
		RDPPort:     5900,
		RDPUsername: "zed",
		RDPPassword: "zed123", // This would be the actual secure password
		Status:      "running",
	}, nil
}

// stopRDPProxy stops the RDP proxy for a session
func (rpm *RDPProxyManager) stopRDPProxy(sessionID string) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	if conn, exists := rpm.connections[sessionID]; exists {
		// Cancel the proxy context to stop all goroutines
		if conn.cancel != nil {
			conn.cancel()
		}

		// Close the TCP listener
		if conn.listener != nil {
			conn.listener.Close()
		}

		log.Info().
			Str("session_id", sessionID).
			Int("local_port", conn.LocalPort).
			Msg("Stopped RDP TCP proxy")
	}
}

// cleanupConnection removes a connection when the session ends
func (rpm *RDPProxyManager) cleanupConnection(sessionID string) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	if conn, exists := rpm.connections[sessionID]; exists {
		// Stop the RDP proxy first
		if conn.cancel != nil {
			conn.cancel()
		}
		if conn.listener != nil {
			conn.listener.Close()
		}

		// Delete from Guacamole
		if err := rpm.guacamoleClient.deleteConnection(conn.ConnectionID); err != nil {
			log.Error().Err(err).
				Str("session_id", sessionID).
				Str("connection_id", conn.ConnectionID).
				Msg("Failed to delete Guacamole connection")
		}

		// Remove from local cache
		delete(rpm.connections, sessionID)

		log.Info().
			Str("session_id", sessionID).
			Str("connection_id", conn.ConnectionID).
			Msg("Cleaned up RDP proxy connection")
	}
}

// cleanupExpiredConnections removes connections that haven't been used recently
func (rpm *RDPProxyManager) cleanupExpiredConnections(maxAge time.Duration) {
	rpm.mu.Lock()
	defer rpm.mu.Unlock()

	now := time.Now()
	for sessionID, conn := range rpm.connections {
		if now.Sub(conn.LastActivity) > maxAge {
			// Stop the RDP proxy first
			if conn.cancel != nil {
				conn.cancel()
			}
			if conn.listener != nil {
				conn.listener.Close()
			}

			// Delete from Guacamole
			if err := rpm.guacamoleClient.deleteConnection(conn.ConnectionID); err != nil {
				log.Error().Err(err).
					Str("session_id", sessionID).
					Str("connection_id", conn.ConnectionID).
					Msg("Failed to delete expired Guacamole connection")
			}

			// Remove from local cache
			delete(rpm.connections, sessionID)

			log.Info().
				Str("session_id", sessionID).
				Str("connection_id", conn.ConnectionID).
				Dur("age", now.Sub(conn.LastActivity)).
				Msg("Cleaned up expired RDP proxy connection")
		}
	}
}
