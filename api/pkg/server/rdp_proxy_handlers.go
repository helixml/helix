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

	"github.com/helixml/helix/api/pkg/connman"
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

// GuacamoleConnectionConfig represents a connection configuration in Guacamole
type GuacamoleConnectionConfig struct {
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

	connection := GuacamoleConnectionConfig{
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

// RDPProxyManager manages RDP proxy connections via reverse dial
type RDPProxyManager struct {
	guacamoleClient *GuacamoleClient
	connman         *connman.ConnectionManager
	connections     map[string]*RDPProxyConnection
	mu              sync.RWMutex
}

// RDPProxyConnection represents an active RDP proxy connection
type RDPProxyConnection struct {
	SessionID      string
	RunnerID       string // For external agent runner connections
	ConnectionID   string
	LocalPort      int              // Local TCP port for this proxy
	ConnectionType string           // "session" or "runner"
	CreatedAt      time.Time
	LastActivity   time.Time
	listener       net.Listener       // TCP listener for cleanup
	cancel         context.CancelFunc // Cancel function to stop proxy
}

// NewRDPProxyManager creates a new RDP proxy manager
func NewRDPProxyManager(guacamoleURL, guacamoleUser, guacamolePass string, connman *connman.ConnectionManager) *RDPProxyManager {
	return &RDPProxyManager{
		guacamoleClient: NewGuacamoleClient(guacamoleURL, guacamoleUser, guacamolePass),
		connman:         connman,
		connections:     make(map[string]*RDPProxyConnection),
	}
}

// startRDPProxy handles RDP proxy WebSocket connections using reverse dial
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

	// Get session info to find the runnerID
	sessionInfo, err := s.getZedSessionInfo(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session info")
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Get runnerID from external agent runner manager
	runnerID := ""
	if s.externalAgentRunnerManager != nil {
		connections := s.externalAgentRunnerManager.listConnections()
		if len(connections) > 0 {
			runnerID = connections[0].SessionID // Use first available runner for now
		}
	}
	if runnerID == "" {
		log.Error().Str("session_id", sessionID).Msg("No runners available for session")
		http.Error(w, "no runners available for session", http.StatusServiceUnavailable)
		return
	}

	// Start local RDP proxy for this session (if not already running)
	proxyPort, err := s.rdpProxyManager.startRDPProxy(sessionID, runnerID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to start RDP proxy")
		http.Error(w, "failed to start RDP proxy", http.StatusInternalServerError)
		return
	}

	// Create or get Guacamole connection pointing to our API container proxy
	connectionID, err := s.rdpProxyManager.createOrGetConnection(sessionID, "api", proxyPort, sessionInfo)
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

// startRDPProxy starts a local TCP RDP proxy for a session using reverse dial
func (rpm *RDPProxyManager) startRDPProxy(sessionID, runnerID string) (int, error) {
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
		SessionID:      sessionID,
		RunnerID:       runnerID,
		LocalPort:      proxyPort,
		ConnectionType: "session",
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		cancel:         cancel,
	}

	// Start the TCP server in a goroutine
	go rpm.runSimpleTCPProxy(ctx, proxy)

	// Store connection info
	rpm.connections[sessionID] = proxy

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Int("local_port", proxyPort).
		Msg("Started RDP TCP proxy using reverse dial")

	return proxyPort, nil
}

// createOrGetConnection creates or retrieves a Guacamole connection
func (rpm *RDPProxyManager) createOrGetConnection(sessionID, hostname string, port int, sessionInfo *ZedAgentSession) (string, error) {
	connectionID, err := rpm.guacamoleClient.createRDPConnection(
		sessionID,
		hostname,
		port,
		sessionInfo.RDPUsername,
		sessionInfo.RDPPassword, // Use actual configured password
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

// runSimpleTCPProxy runs a TCP proxy that forwards RDP traffic via reverse dial
func (rpm *RDPProxyManager) runSimpleTCPProxy(ctx context.Context, proxy *RDPProxyConnection) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", proxy.LocalPort))
	if err != nil {
		log.Error().Err(err).
			Str("session_id", proxy.SessionID).
			Str("runner_id", proxy.RunnerID).
			Int("port", proxy.LocalPort).
			Msg("Failed to start TCP proxy listener")
		return
	}
	defer listener.Close()

	// Store listener for cleanup
	proxy.listener = listener

	log.Info().
		Str("session_id", proxy.SessionID).
		Str("runner_id", proxy.RunnerID).
		Int("port", proxy.LocalPort).
		Msg("RDP TCP proxy listening (reverse dial)")

	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("session_id", proxy.SessionID).
				Str("runner_id", proxy.RunnerID).
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
					Str("runner_id", proxy.RunnerID).
					Msg("Failed to accept TCP connection")
				continue
			}
		}

		// Handle each connection in a goroutine
		go rpm.handleSimpleTCPConnection(ctx, proxy, conn)
	}
}

// handleSimpleTCPConnection handles a single TCP connection by proxying over reverse dial
func (rpm *RDPProxyManager) handleSimpleTCPConnection(ctx context.Context, proxy *RDPProxyConnection, guacdConn net.Conn) {
	defer guacdConn.Close()

	log.Info().
		Str("session_id", proxy.SessionID).
		Str("runner_id", proxy.RunnerID).
		Str("remote_addr", guacdConn.RemoteAddr().String()).
		Msg("New RDP TCP connection (reverse dial)")

	// Get reverse dial connection to runner
	deviceConn, err := rpm.connman.Dial(ctx, proxy.RunnerID)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", proxy.SessionID).
			Str("runner_id", proxy.RunnerID).
			Msg("Failed to dial runner via reverse dial")
		return
	}
	defer deviceConn.Close()

	log.Info().
		Str("session_id", proxy.SessionID).
		Str("runner_id", proxy.RunnerID).
		Msg("Established reverse dial connection to runner")

	// Update last activity
	proxy.LastActivity = time.Now()

	// Simple bidirectional TCP proxy (no protocol conversion)
	done := make(chan struct{}, 2)

	// Forward guacd -> runner
	go func() {
		defer func() { done <- struct{}{} }()
		bytes, err := io.Copy(deviceConn, guacdConn)
		if err != nil {
			log.Debug().Err(err).
				Str("session_id", proxy.SessionID).
				Str("runner_id", proxy.RunnerID).
				Msg("Forward guacd->runner ended")
		} else {
			log.Debug().
				Str("session_id", proxy.SessionID).
				Str("runner_id", proxy.RunnerID).
				Int64("bytes", bytes).
				Msg("Forward guacd->runner completed")
		}
	}()

	// Forward runner -> guacd
	go func() {
		defer func() { done <- struct{}{} }()
		bytes, err := io.Copy(guacdConn, deviceConn)
		if err != nil {
			log.Debug().Err(err).
				Str("session_id", proxy.SessionID).
				Str("runner_id", proxy.RunnerID).
				Msg("Forward runner->guacd ended")
		} else {
			log.Debug().
				Str("session_id", proxy.SessionID).
				Str("runner_id", proxy.RunnerID).
				Int64("bytes", bytes).
				Msg("Forward runner->guacd completed")
		}
	}()

	// Wait for either direction to complete
	<-done

	log.Info().
		Str("session_id", proxy.SessionID).
		Str("runner_id", proxy.RunnerID).
		Msg("RDP TCP connection closed (reverse dial)")
}

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

// getZedSessionInfo retrieves Zed session information
func (s *HelixAPIServer) getZedSessionInfo(ctx context.Context, sessionID string) (*ZedAgentSession, error) {
	// Try to get actual session info from external agent executor
	if s.externalAgentExecutor != nil {
		session, err := s.externalAgentExecutor.GetSession(sessionID)
		if err == nil {
			return &ZedAgentSession{
				SessionID:   sessionID,
				RDPPort:     5900,
				RDPUsername: "zed",
				RDPPassword: session.RDPPassword, // Use actual configured password
				Status:      session.Status,
			}, nil
		}
		log.Debug().Err(err).Str("session_id", sessionID).Msg("Session not found in external agent executor")
	}

	// Fail securely - no fallback passwords
	log.Error().Str("session_id", sessionID).Msg("Session not found in external agent executor - RDP access unavailable")
	return nil, fmt.Errorf("RDP access not available for session %s", sessionID)
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

// Legacy compatibility methods for other parts of the codebase

// CreateRunnerRDPProxy creates an RDP proxy for a runner (legacy method)
func (rpm *RDPProxyManager) CreateRunnerRDPProxy(ctx context.Context, runnerID string) (*RDPProxyConnection, error) {
	// Create a basic proxy connection using reverse dial approach
	proxyPort := rpm.allocatePort()
	
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &RDPProxyConnection{
		RunnerID:       runnerID,
		LocalPort:      proxyPort,
		ConnectionType: "runner",
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		cancel:         cancel,
	}

	// Start the TCP server in a goroutine
	go rpm.runSimpleTCPProxy(ctx, proxy)

	// Store connection info
	rpm.mu.Lock()
	rpm.connections[runnerID] = proxy
	rpm.mu.Unlock()

	log.Info().
		Str("runner_id", runnerID).
		Int("local_port", proxyPort).
		Msg("Created RDP proxy for runner using reverse dial")

	return proxy, nil
}

// CreateSessionRDPProxy creates an RDP proxy for a session (legacy method)
func (rpm *RDPProxyManager) CreateSessionRDPProxy(ctx context.Context, sessionID, runnerID string) (*RDPProxyConnection, error) {
	// Use the provided runnerID
	
	proxyPort := rpm.allocatePort()
	
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &RDPProxyConnection{
		SessionID:      sessionID,
		RunnerID:       runnerID,
		LocalPort:      proxyPort,
		ConnectionType: "session",
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
		cancel:         cancel,
	}

	// Start the TCP server in a goroutine
	go rpm.runSimpleTCPProxy(ctx, proxy)

	// Store connection info
	rpm.mu.Lock()
	rpm.connections[sessionID] = proxy
	rpm.mu.Unlock()

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Int("local_port", proxyPort).
		Msg("Created RDP proxy for session using reverse dial")

	return proxy, nil
}


// GetSessionProxy gets session proxy info (legacy method)
func (rpm *RDPProxyManager) GetSessionProxy(sessionID string) (*RDPProxyConnection, error) {
	rpm.mu.RLock()
	defer rpm.mu.RUnlock()
	
	if conn, exists := rpm.connections[sessionID]; exists {
		return conn, nil
	}
	return nil, fmt.Errorf("no proxy found for session %s", sessionID)
}