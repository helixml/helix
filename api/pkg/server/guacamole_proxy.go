package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// GuacamoleProxy manages WebSocket connections between frontend and Guacamole server
type GuacamoleProxy struct {
	guacamoleServerURL string
	guacamoleUsername  string
	guacamolePassword  string
	authToken          string
	connections        map[string]*GuacamoleConnection
	mu                 sync.RWMutex
	httpClient         *http.Client
}

// GuacamoleConnection represents an active proxy connection
type GuacamoleConnection struct {
	connectionID   string
	sessionID      string
	runnerID       string
	frontendWS     *websocket.Conn
	guacamoleWS    *websocket.Conn
	rdpProxyPort   int
	rdpPassword    string
	connectionType string // "session" or "runner"
	createdAt      time.Time
	lastActivity   time.Time
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewGuacamoleProxy creates a new Guacamole proxy
func NewGuacamoleProxy(guacamoleServerURL, username, password string) *GuacamoleProxy {
	return &GuacamoleProxy{
		guacamoleServerURL: guacamoleServerURL,
		guacamoleUsername:  username,
		guacamolePassword:  password,
		connections:        make(map[string]*GuacamoleConnection),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RegisterRoutes registers the Guacamole proxy routes
func (apiServer *HelixAPIServer) registerGuacamoleProxyRoutes(authRouter *mux.Router) {
	// Session-specific Guacamole proxy
	authRouter.HandleFunc("/sessions/{sessionID}/guac/proxy", apiServer.handleSessionGuacamoleProxy).Methods("GET")

	// Runner-specific Guacamole proxy
	authRouter.HandleFunc("/external-agents/runners/{runnerID}/guac/proxy", apiServer.handleRunnerGuacamoleProxy).Methods("GET")
	
	// Admin endpoint to cleanup all Guacamole connections
	authRouter.HandleFunc("/admin/guacamole/cleanup", apiServer.handleGuacamoleCleanup).Methods("POST")
}

// handleSessionGuacamoleProxy handles session-specific Guacamole WebSocket connections
func (apiServer *HelixAPIServer) handleSessionGuacamoleProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionID"]
	if sessionID == "" {
		http.Error(w, "sessionID is required", http.StatusBadRequest)
		return
	}

	// Verify user owns the session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	if session.Owner != user.ID {
		http.Error(w, "you are not allowed to access this session", http.StatusForbidden)
		return
	}

	// Check if this is an external agent session
	if session.Metadata.AgentType != "zed_external" {
		http.Error(w, "session does not support RDP access", http.StatusBadRequest)
		return
	}

	// Find which runner is handling this session
	runnerID, rdpProxyPort, rdpPassword, err := apiServer.getSessionRDPInfo(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session RDP info")
		http.Error(w, "failed to get RDP connection info", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Int("rdp_port", rdpProxyPort).
		Msg("Starting Guacamole proxy for session")

	err = apiServer.startGuacamoleProxy(w, r, sessionID, runnerID, rdpProxyPort, rdpPassword, "session")
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to start Guacamole proxy")
		http.Error(w, "failed to start Guacamole proxy", http.StatusInternalServerError)
		return
	}
}

// handleRunnerGuacamoleProxy handles runner-specific Guacamole WebSocket connections
func (apiServer *HelixAPIServer) handleRunnerGuacamoleProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	runnerID := vars["runnerID"]
	if runnerID == "" {
		http.Error(w, "runnerID is required", http.StatusBadRequest)
		return
	}

	// Check if user has admin permissions for runner access
	if !apiServer.isAdmin(r) {
		log.Warn().
			Str("user_id", user.ID).
			Str("runner_id", runnerID).
			Msg("Non-admin user attempted to access runner VNC")
		http.Error(w, "admin access required for runner connections", http.StatusForbidden)
		return
	}

	// Get runner VNC password (stored in RDP password field for now)
	vncPassword, err := apiServer.Store.GetAgentRunnerRDPPassword(ctx, runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to get runner VNC password")
		http.Error(w, "runner VNC access not available", http.StatusServiceUnavailable)
		return
	}

	// Create or get RDP proxy for this runner
	if apiServer.rdpProxyManager == nil {
		http.Error(w, "RDP proxy manager not available", http.StatusServiceUnavailable)
		return
	}

	proxy, err := apiServer.rdpProxyManager.CreateRunnerRDPProxy(ctx, runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to create RDP proxy for runner")
		http.Error(w, "failed to create RDP proxy", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("runner_id", runnerID).
		Int("rdp_port", proxy.LocalPort).
		Msg("Starting Guacamole proxy for runner")

	err = apiServer.startGuacamoleProxy(w, r, runnerID, runnerID, proxy.LocalPort, vncPassword, "runner")
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to start Guacamole proxy")
		http.Error(w, "failed to start Guacamole proxy", http.StatusInternalServerError)
		return
	}
}

// startGuacamoleProxy starts a WebSocket proxy connection to the Guacamole server
func (apiServer *HelixAPIServer) startGuacamoleProxy(w http.ResponseWriter, r *http.Request, sessionID, runnerID string, rdpProxyPort int, rdpPassword, connectionType string) error {
	// Upgrade frontend connection to WebSocket
	frontendWS, err := userWebsocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return fmt.Errorf("failed to upgrade frontend WebSocket: %w", err)
	}
	defer frontendWS.Close()

	// Create connection ID
	connectionID := fmt.Sprintf("%s-%s-%d", connectionType, sessionID, time.Now().UnixNano())

	log.Info().
		Str("connection_id", connectionID).
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Str("connection_type", connectionType).
		Msg("Frontend WebSocket upgraded for Guacamole proxy")

	// Create Guacamole connection in server
	guacConnectionID, err := apiServer.createGuacamoleConnection(connectionID, rdpProxyPort, rdpPassword)
	if err != nil {
		return fmt.Errorf("failed to create Guacamole connection: %w", err)
	}

	// Connect to Guacamole server WebSocket
	guacamoleWS, err := apiServer.connectToGuacamoleWebSocket(guacConnectionID)
	if err != nil {
		return fmt.Errorf("failed to connect to Guacamole WebSocket: %w", err)
	}
	defer guacamoleWS.Close()

	// Create connection context
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Store connection
	conn := &GuacamoleConnection{
		connectionID:   connectionID,
		sessionID:      sessionID,
		runnerID:       runnerID,
		frontendWS:     frontendWS,
		guacamoleWS:    guacamoleWS,
		rdpProxyPort:   rdpProxyPort,
		rdpPassword:    rdpPassword,
		connectionType: connectionType,
		createdAt:      time.Now(),
		lastActivity:   time.Now(),
		ctx:            ctx,
		cancel:         cancel,
	}

	if apiServer.guacamoleProxy != nil {
		apiServer.guacamoleProxy.addConnection(connectionID, conn)
		defer apiServer.guacamoleProxy.removeConnection(connectionID)
	}

	log.Info().
		Str("connection_id", connectionID).
		Str("guac_connection_id", guacConnectionID).
		Msg("Connected to Guacamole server WebSocket")

	// Start bidirectional proxy
	return apiServer.proxyGuacamoleTraffic(ctx, conn)
}

// createGuacamoleConnection creates a new VNC connection in the Guacamole server
func (apiServer *HelixAPIServer) createGuacamoleConnection(connectionID string, rdpPort int, rdpPassword string) (string, error) {
	// Create VNC connection configuration for Guacamole
	connectionConfig := map[string]interface{}{
		"parentIdentifier": "ROOT",
		"name":             fmt.Sprintf("helix-%s", connectionID),
		"protocol":         "vnc",
		"parameters": map[string]string{
			"hostname":     "zed-runner", // Direct connection to zed-runner container
			"port":         "5901", // VNC port on zed-runner
			"password":     "helix123", // Static VNC password
			"color-depth":  "24",
			"cursor":       "remote",
			"read-only":    "false",
			"swap-red-blue": "false",
		},
		"attributes": map[string]string{
			"max-connections":          "",
			"max-connections-per-user": "",
			"weight":                   "",
			"failover-only":            "",
			"guacd-port":               "",
			"guacd-encryption":         "",
			"guacd-hostname":           "",
		},
	}

	// Authenticate with Guacamole first
	authToken, err := apiServer.authenticateWithGuacamole()
	if err != nil {
		return "", fmt.Errorf("failed to authenticate with Guacamole: %w", err)
	}

	// Call Guacamole REST API to create connection
	guacamoleURL := fmt.Sprintf("%s/guacamole/api/session/data/postgresql/connections?token=%s",
		apiServer.guacamoleProxy.guacamoleServerURL, authToken)

	configJSON, err := json.Marshal(connectionConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal connection config: %w", err)
	}

	log.Debug().
		Str("connection_id", connectionID).
		Int("rdp_port", rdpPort).
		Str("guacamole_url", guacamoleURL).
		Msg("Creating Guacamole connection via REST API")

	// Make HTTP POST request to create connection
	resp, err := http.Post(guacamoleURL, "application/json", strings.NewReader(string(configJSON)))
	if err != nil {
		return "", fmt.Errorf("failed to make request to Guacamole API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("Guacamole API returned status %d", resp.StatusCode)
	}

	// Parse response to get connection identifier
	var guacConnection map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&guacConnection); err != nil {
		return "", fmt.Errorf("failed to decode Guacamole response: %w", err)
	}

	guacConnectionID, ok := guacConnection["identifier"].(string)
	if !ok {
		guacConnectionID = fmt.Sprintf("guac-%s", connectionID)
		log.Warn().Msg("Could not parse Guacamole connection ID, using fallback")
	}

	log.Info().
		Str("connection_id", connectionID).
		Str("guac_connection_id", guacConnectionID).
		RawJSON("config", configJSON).
		Msg("Created Guacamole connection via REST API")

	return guacConnectionID, nil
}

// connectToGuacamoleWebSocket establishes WebSocket connection to Guacamole server
func (apiServer *HelixAPIServer) connectToGuacamoleWebSocket(guacConnectionID string) (*websocket.Conn, error) {
	// Build Guacamole WebSocket URL
	u, err := url.Parse(apiServer.guacamoleProxy.guacamoleServerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Guacamole server URL: %w", err)
	}

	// Convert to WebSocket URL
	wsScheme := "ws"
	if u.Scheme == "https" {
		wsScheme = "wss"
	}

	wsURL := fmt.Sprintf("%s://%s/guacamole/websocket-tunnel?token=%s", wsScheme, u.Host, guacConnectionID)

	log.Debug().
		Str("guac_connection_id", guacConnectionID).
		Str("ws_url", wsURL).
		Msg("Connecting to Guacamole WebSocket")

	// Connect to Guacamole WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to dial Guacamole WebSocket: %w", err)
	}

	return conn, nil
}

// proxyGuacamoleTraffic handles bidirectional proxying between frontend and Guacamole
func (apiServer *HelixAPIServer) proxyGuacamoleTraffic(ctx context.Context, conn *GuacamoleConnection) error {
	done := make(chan struct{}, 2)
	errChan := make(chan error, 2)

	// Frontend -> Guacamole
	go func() {
		defer func() { done <- struct{}{} }()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Read from frontend WebSocket
			_, message, err := conn.frontendWS.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					errChan <- fmt.Errorf("frontend WebSocket read error: %w", err)
				}
				return
			}

			// Forward to Guacamole WebSocket
			err = conn.guacamoleWS.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				errChan <- fmt.Errorf("Guacamole WebSocket write error: %w", err)
				return
			}

			conn.lastActivity = time.Now()
		}
	}()

	// Guacamole -> Frontend
	go func() {
		defer func() { done <- struct{}{} }()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Read from Guacamole WebSocket
			_, message, err := conn.guacamoleWS.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					errChan <- fmt.Errorf("Guacamole WebSocket read error: %w", err)
				}
				return
			}

			// Forward to frontend WebSocket
			err = conn.frontendWS.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				errChan <- fmt.Errorf("frontend WebSocket write error: %w", err)
				return
			}

			conn.lastActivity = time.Now()
		}
	}()

	// Wait for completion or error
	select {
	case <-done:
		log.Info().
			Str("connection_id", conn.connectionID).
			Str("session_id", conn.sessionID).
			Msg("Guacamole proxy connection closed normally")
	case err := <-errChan:
		log.Debug().
			Err(err).
			Str("connection_id", conn.connectionID).
			Str("session_id", conn.sessionID).
			Msg("Guacamole proxy connection error")
	case <-ctx.Done():
		log.Debug().
			Str("connection_id", conn.connectionID).
			Str("session_id", conn.sessionID).
			Msg("Guacamole proxy context cancelled")
	}

	return nil
}

// getSessionRDPInfo retrieves RDP connection information for a session
func (apiServer *HelixAPIServer) getSessionRDPInfo(ctx context.Context, sessionID string) (runnerID string, rdpPort int, rdpPassword string, err error) {
	// Find which runner is handling this session
	if apiServer.externalAgentRunnerManager != nil {
		connections := apiServer.externalAgentRunnerManager.listConnections()
		if len(connections) > 0 {
			runnerID = connections[0].SessionID // Use first available runner for now
		}
	}

	if runnerID == "" {
		return "", 0, "", fmt.Errorf("no runners available to handle session")
	}

	// Get runner's VNC password (stored in RDP password field for now)
	password, err := apiServer.Store.GetAgentRunnerRDPPassword(ctx, runnerID)
	if err != nil {
		return "", 0, "", fmt.Errorf("failed to get runner VNC password: %w", err)
	}

	// Create or get RDP proxy for this session
	if apiServer.rdpProxyManager == nil {
		return "", 0, "", fmt.Errorf("RDP proxy manager not available")
	}

	proxy, err := apiServer.rdpProxyManager.CreateSessionRDPProxy(ctx, sessionID, runnerID)
	if err != nil {
		return "", 0, "", fmt.Errorf("failed to create session RDP proxy: %w", err)
	}

	return runnerID, proxy.LocalPort, password, nil
}

// Connection management methods
func (gp *GuacamoleProxy) addConnection(connectionID string, conn *GuacamoleConnection) {
	gp.mu.Lock()
	defer gp.mu.Unlock()
	gp.connections[connectionID] = conn
}

func (gp *GuacamoleProxy) removeConnection(connectionID string) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	if conn, exists := gp.connections[connectionID]; exists {
		conn.cancel()
		delete(gp.connections, connectionID)

		log.Info().
			Str("connection_id", connectionID).
			Str("session_id", conn.sessionID).
			Msg("Removed Guacamole proxy connection")
	}
}

func (gp *GuacamoleProxy) getConnections() map[string]*GuacamoleConnection {
	gp.mu.RLock()
	defer gp.mu.RUnlock()

	connections := make(map[string]*GuacamoleConnection)
	for k, v := range gp.connections {
		connections[k] = v
	}
	return connections
}

// Cleanup stale connections
func (gp *GuacamoleProxy) cleanupStaleConnections(maxAge time.Duration) {
	gp.mu.Lock()
	defer gp.mu.Unlock()

	now := time.Now()
	for connectionID, conn := range gp.connections {
		if now.Sub(conn.lastActivity) > maxAge {
			log.Info().
				Str("connection_id", connectionID).
				Str("session_id", conn.sessionID).
				Dur("idle_time", now.Sub(conn.lastActivity)).
				Msg("Cleaning up stale Guacamole connection")

			conn.cancel()
			delete(gp.connections, connectionID)
		}
	}
}

// authenticateWithGuacamole authenticates with Guacamole and returns auth token
func (apiServer *HelixAPIServer) authenticateWithGuacamole() (string, error) {
	authURL := fmt.Sprintf("%s/guacamole/api/tokens", apiServer.guacamoleProxy.guacamoleServerURL)

	// Use form data for authentication
	authData := url.Values{}
	authData.Set("username", apiServer.guacamoleProxy.guacamoleUsername)
	authData.Set("password", apiServer.guacamoleProxy.guacamolePassword)

	resp, err := http.PostForm(authURL, authData)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authentication failed with status %d", resp.StatusCode)
	}

	var authResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}

	token, ok := authResponse["authToken"].(string)
	if !ok {
		return "", fmt.Errorf("no auth token in response")
	}

	log.Debug().Str("auth_url", authURL).Msg("Successfully authenticated with Guacamole")
	return token, nil
}

// handleGuacamoleCleanup handles manual cleanup of all Guacamole connections
func (apiServer *HelixAPIServer) handleGuacamoleCleanup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user has admin permissions
	if !apiServer.isAdmin(r) {
		log.Warn().
			Str("user_id", user.ID).
			Msg("Non-admin user attempted to cleanup Guacamole connections")
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}

	// Trigger cleanup
	if apiServer.guacamoleLifecycle != nil {
		err := apiServer.guacamoleLifecycle.CleanupAllGuacamoleConnections(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to cleanup Guacamole connections")
			http.Error(w, "cleanup failed", http.StatusInternalServerError)
			return
		}
	}

	// Also trigger stale connection cleanup in proxy
	if apiServer.guacamoleProxy != nil {
		apiServer.guacamoleProxy.cleanupStaleConnections(0) // Clean all connections
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
		"message": "Guacamole connections cleaned up",
	})
}
