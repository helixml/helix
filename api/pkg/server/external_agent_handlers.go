package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// createExternalAgent handles POST /api/v1/external-agents
func (apiServer *HelixAPIServer) createExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	var agent types.ZedAgent
	err := json.NewDecoder(req.Body).Decode(&agent)
	if err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Generate session ID if not provided
	if agent.SessionID == "" {
		agent.SessionID = system.GenerateRequestID()
	}

	// Set default values if not provided
	if agent.WorkDir == "" {
		agent.WorkDir = fmt.Sprintf("/tmp/zed-workspaces/%s", agent.SessionID)
	}

	// Validate required fields
	if agent.Input == "" {
		http.Error(res, "input is required", http.StatusBadRequest)
		return
	}

	// External agent executor should be initialized in server constructor
	if apiServer.externalAgentExecutor == nil {
		log.Error().Str("session_id", agent.SessionID).Msg("External agent executor not available")
		http.Error(res, "external agent executor not available", http.StatusServiceUnavailable)
		return
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("project_path", agent.ProjectPath).
		Msg("Creating external agent via API endpoint")

	// Generate auth token for WebSocket connection
	token, err := apiServer.generateExternalAgentToken(agent.SessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to generate external agent token")
		http.Error(res, fmt.Sprintf("failed to create external agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Start the external agent
	response, err := apiServer.externalAgentExecutor.StartZedAgent(req.Context(), &agent)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to start external agent")
		http.Error(res, fmt.Sprintf("failed to start external agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("rdp_url", response.RDPURL).
		Str("status", response.Status).
		Msg("External agent started successfully")

	// Add WebSocket connection info to response
	response.WebSocketURL = fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, agent.SessionID)
	response.AuthToken = token

	log.Info().
		Str("session_id", agent.SessionID).
		Str("websocket_url", response.WebSocketURL).
		Bool("has_auth_token", len(response.AuthToken) > 0).
		Msg("External agent API response prepared")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// getExternalAgent handles GET /api/v1/external-agents/{sessionID}
func (apiServer *HelixAPIServer) getExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	if apiServer.externalAgentExecutor == nil {
		log.Error().Str("session_id", sessionID).Msg("External agent executor not available")
		http.Error(res, "external agent executor not available", http.StatusNotFound)
		return
	}

	log.Debug().
		Str("session_id", sessionID).
		Msg("Getting external agent session info")

	session, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("External agent session not found")
		http.Error(res, fmt.Sprintf("session %s not found: %s", sessionID, err.Error()), http.StatusNotFound)
		return
	}

	response := types.ZedAgentResponse{
		SessionID: session.SessionID,
		RDPURL:    session.RDPURL,
		Status:    "running",
		PID:       0, // PID not available in ZedSession
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// listExternalAgents handles GET /api/v1/external-agents
func (apiServer *HelixAPIServer) listExternalAgents(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	if apiServer.externalAgentExecutor == nil {
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode([]types.ZedAgentResponse{})
		return
	}

	sessions := apiServer.externalAgentExecutor.ListSessions()
	responses := make([]types.ZedAgentResponse, len(sessions))

	for i, session := range sessions {
		responses[i] = types.ZedAgentResponse{
			SessionID: session.SessionID,
			RDPURL:    session.RDPURL,
			Status:    session.Status,
			PID:       0, // PID not available in ZedSession
		}
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(responses)
}

// deleteExternalAgent handles DELETE /api/v1/external-agents/{sessionID}
func (apiServer *HelixAPIServer) deleteExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "external agent executor not available", http.StatusNotFound)
		return
	}

	err := apiServer.externalAgentExecutor.StopZedAgent(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to stop external agent")
		http.Error(res, fmt.Sprintf("failed to stop external agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"message":    "External agent stopped successfully",
		"session_id": sessionID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// getExternalAgentRDP handles GET /api/v1/external-agents/{sessionID}/rdp
func (apiServer *HelixAPIServer) getExternalAgentRDP(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "external agent executor not available", http.StatusNotFound)
		return
	}

	log.Debug().
		Str("session_id", sessionID).
		Msg("Looking up external agent session for RDP info")

	session, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Msg("Failed to find external agent session")

		// Try to list all sessions to see what's available
		allSessions := apiServer.externalAgentExecutor.ListSessions()
		log.Debug().
			Int("total_sessions", len(allSessions)).
			Msg("Current external agent sessions")

		for _, s := range allSessions {
			log.Debug().
				Str("available_session_id", s.SessionID).
				Str("status", s.Status).
				Msg("Available session")
		}

		http.Error(res, fmt.Sprintf("session %s not found: %s", sessionID, err.Error()), http.StatusNotFound)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("status", session.Status).
		Str("rdp_url", session.RDPURL).
		Msg("Found external agent session for RDP")

	// Return RDP connection details with WebSocket info
	rdpInfo := map[string]interface{}{
		"session_id":          session.SessionID,
		"rdp_url":             fmt.Sprintf("rdp://localhost:%d", 8080),
		"rdp_port":            8080,
		"rdp_password":        session.RDPPassword, // Secure random password
		"display":             fmt.Sprintf(":%d", 1),
		"status":              session.Status,
		"username":            "zed", // Default RDP username
		"host":                "localhost",
		"proxy_url":           fmt.Sprintf("wss://%s/api/v1/external-agents/%s/rdp/proxy", req.Host, session.SessionID),
		"websocket_url":       fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, session.SessionID),
		"websocket_connected": apiServer.isExternalAgentConnected(session.SessionID),
	}

	log.Info().
		Str("session_id", session.SessionID).
		Str("status", session.Status).
		Bool("websocket_connected", rdpInfo["websocket_connected"].(bool)).
		Msg("Returning RDP connection info")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(rdpInfo)
}

// updateExternalAgent handles PUT /api/v1/external-agents/{sessionID}
func (apiServer *HelixAPIServer) updateExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	var updateData map[string]interface{}
	err := json.NewDecoder(req.Body).Decode(&updateData)
	if err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "external agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	// For now, just update the last access time
	// In a full implementation, you might want to support updating other session properties
	session, _ = apiServer.externalAgentExecutor.GetSession(sessionID)

	response := types.ZedAgentResponse{
		SessionID: session.SessionID,
		RDPURL:    session.RDPURL,
		Status:    session.Status,
		PID:       0, // PID not available in ZedSession
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// getExternalAgentStats handles GET /api/v1/external-agents/{sessionID}/stats
func (apiServer *HelixAPIServer) getExternalAgentStats(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "external agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	stats := map[string]interface{}{
		"session_id":    session.SessionID,
		"pid":           0,
		"start_time":    session.StartTime,
		"last_access":   session.LastAccess,
		"uptime":        session.LastAccess.Sub(session.StartTime).Seconds(),
		"workspace_dir": session.ProjectPath,
		"display_num":   1,
		"rdp_port":      8080,
		"status":        "running",
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(stats)
}

// getExternalAgentLogs handles GET /api/v1/external-agents/{sessionID}/logs
func (apiServer *HelixAPIServer) getExternalAgentLogs(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	// Get optional query parameters
	lines := 100 // default
	if linesStr := req.URL.Query().Get("lines"); linesStr != "" {
		if parsedLines, err := strconv.Atoi(linesStr); err == nil && parsedLines > 0 {
			lines = parsedLines
		}
	}

	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "external agent executor not available", http.StatusNotFound)
		return
	}

	_, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	// For now, return a placeholder response
	// In a full implementation, you would read actual logs from the Zed process
	logs := map[string]interface{}{
		"session_id": sessionID,
		"lines":      lines,
		"logs": []string{
			"[INFO] Zed editor started",
			"[INFO] X server initialized",
			"[INFO] XRDP server listening on port 3389",
			"[DEBUG] Session active and responding",
		},
		"timestamp": time.Now(),
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(logs)
}

// isExternalAgentConnected checks if external agent is connected via WebSocket
func (apiServer *HelixAPIServer) isExternalAgentConnected(sessionID string) bool {
	_, exists := apiServer.externalAgentWSManager.getConnection(sessionID)
	return exists
}

// getExternalAgentConnections lists all active external agent connections
func (apiServer *HelixAPIServer) getExternalAgentConnections(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	allConnections := make([]types.ExternalAgentConnection, 0) // Initialize as empty array, not nil

	// Get Zed instance connections (via /external-agents/sync)
	if apiServer.externalAgentWSManager != nil {
		syncConnections := apiServer.externalAgentWSManager.listConnections()
		allConnections = append(allConnections, syncConnections...)
	}

	// Get external agent runner connections (via /ws/external-agent-runner)
	if apiServer.externalAgentRunnerManager != nil {
		runnerConnections := apiServer.externalAgentRunnerManager.listConnections()
		allConnections = append(allConnections, runnerConnections...)
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(allConnections)
}

// getExternalAgentRunnerRDP handles GET /api/v1/external-agents/runners/{runnerID}/rdp
func (apiServer *HelixAPIServer) getExternalAgentRunnerRDP(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	runnerID := vars["runnerID"]

	if runnerID == "" {
		http.Error(res, "runner ID is required", http.StatusBadRequest)
		return
	}

	// Check if runner is connected
	if apiServer.externalAgentRunnerManager == nil {
		http.Error(res, "external agent runner manager not available", http.StatusNotFound)
		return
	}

	// Verify runner exists in our connection manager
	connections := apiServer.externalAgentRunnerManager.listConnections()
	var runnerExists bool
	for _, conn := range connections {
		if conn.SessionID == runnerID { // SessionID is used as RunnerID in our connections
			runnerExists = true
			break
		}
	}

	if !runnerExists {
		http.Error(res, fmt.Sprintf("runner %s not found or not connected", runnerID), http.StatusNotFound)
		return
	}

	// Create RDP proxy for this runner
	if apiServer.rdpProxyManager == nil {
		http.Error(res, "RDP proxy manager not available", http.StatusServiceUnavailable)
		return
	}

	proxy, err := apiServer.rdpProxyManager.CreateRunnerRDPProxy(req.Context(), runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to create RDP proxy for runner")
		http.Error(res, fmt.Sprintf("failed to create RDP proxy: %v", err), http.StatusInternalServerError)
		return
	}

	// Return RDP connection details for the runner
	// Get the actual RDP password from the store
	rdpPassword, err := apiServer.getRunnerRDPPassword(req.Context(), runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to get runner RDP password")
		http.Error(res, fmt.Sprintf("RDP password not available for runner %s", runnerID), http.StatusServiceUnavailable)
		return
	}

	rdpInfo := map[string]interface{}{
		"runner_id":         runnerID,
		"rdp_url":           fmt.Sprintf("rdp://localhost:%d", proxy.LocalPort), // Use proxy port
		"rdp_port":          proxy.LocalPort,
		"rdp_password":      rdpPassword, // Use actual configured password
		"display":           ":1",        // Default display
		"status":            "connected",
		"username":          "zed", // Default RDP username
		"host":              "localhost",
		"connection_type":   "external_agent_runner",
		"desktop_available": true,
		"proxy_url":         fmt.Sprintf("ws://localhost:8080/api/v1/external-agents/runners/%s/rdp/proxy", runnerID),
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("rdp_url", rdpInfo["rdp_url"].(string)).
		Msg("Returning RDP connection info for external agent runner")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(rdpInfo)
}

// startRunnerRDPProxy handles WebSocket connections for runner RDP proxy
func (apiServer *HelixAPIServer) startRunnerRDPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	runnerID := vars["runnerID"]
	if runnerID == "" {
		http.Error(w, "runner ID is required", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("user_id", user.ID).
		Msg("Starting RDP proxy WebSocket for runner")

	// Verify runner exists and get proxy connection
	if apiServer.rdpProxyManager == nil {
		http.Error(w, "RDP proxy manager not available", http.StatusServiceUnavailable)
		return
	}

	// Get or create proxy connection for this runner
	proxy, err := apiServer.rdpProxyManager.CreateRunnerRDPProxy(ctx, runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to create runner RDP proxy")
		http.Error(w, fmt.Sprintf("failed to create RDP proxy: %v", err), http.StatusInternalServerError)
		return
	}

	// Upgrade to WebSocket connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to upgrade WebSocket for runner RDP")
		return
	}
	defer conn.Close()

	log.Info().
		Str("runner_id", runnerID).
		Msg("WebSocket upgraded for runner RDP proxy")

	// Proxy WebSocket traffic to the TCP RDP proxy
	apiServer.proxyWebSocketToTCPForRunner(ctx, conn, proxy, runnerID)
}

// proxyWebSocketToTCPForRunner proxies WebSocket traffic to TCP RDP proxy for runners
func (apiServer *HelixAPIServer) proxyWebSocketToTCPForRunner(ctx context.Context, wsConn *websocket.Conn, proxy *RDPProxyConnection, runnerID string) {
	// Connect to the local TCP proxy
	tcpConn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxy.LocalPort))
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Int("port", proxy.LocalPort).Msg("Failed to connect to RDP TCP proxy for runner")
		wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to RDP server"))
		return
	}
	defer tcpConn.Close()

	log.Info().
		Str("runner_id", runnerID).
		Int("port", proxy.LocalPort).
		Msg("Connected to TCP RDP proxy for runner")

	// Create context for cancellation
	proxyCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Proxy WebSocket -> TCP
	go func() {
		defer cancel()
		for {
			select {
			case <-proxyCtx.Done():
				return
			default:
				_, data, err := wsConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Error().Err(err).Str("runner_id", runnerID).Msg("Error reading from WebSocket for runner")
					}
					return
				}

				if _, err := tcpConn.Write(data); err != nil {
					log.Error().Err(err).Str("runner_id", runnerID).Msg("Error writing to TCP connection for runner")
					return
				}
			}
		}
	}()

	// Proxy TCP -> WebSocket
	buffer := make([]byte, 4096)
	for {
		select {
		case <-proxyCtx.Done():
			return
		default:
			tcpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
			n, err := tcpConn.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Debug().Err(err).Str("runner_id", runnerID).Msg("TCP connection closed for runner")
				return
			}

			if err := wsConn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
				log.Error().Err(err).Str("runner_id", runnerID).Msg("Error writing to WebSocket for runner")
				return
			}
		}
	}
}

// getRunnerRDPPassword retrieves the RDP password for a runner from the store
func (apiServer *HelixAPIServer) getRunnerRDPPassword(ctx context.Context, runnerID string) (string, error) {
	// Try to get the runner from the store first
	runner, err := apiServer.Store.GetAgentRunner(ctx, runnerID)
	if err != nil {
		// If runner doesn't exist in store, create it with a new password
		if errors.Is(err, store.ErrNotFound) {
			log.Info().
				Str("runner_id", runnerID).
				Msg("Agent runner not found in store, creating new one with generated password")

			newRunner, createErr := apiServer.Store.CreateAgentRunner(ctx, runnerID)
			if createErr != nil {
				return "", fmt.Errorf("failed to create agent runner %s: %w", runnerID, createErr)
			}
			return newRunner.RDPPassword, nil
		}
		return "", fmt.Errorf("failed to get agent runner %s: %w", runnerID, err)
	}

	// Update heartbeat to show runner is active
	if heartbeatErr := apiServer.Store.UpdateAgentRunnerHeartbeat(ctx, runnerID); heartbeatErr != nil {
		log.Warn().Err(heartbeatErr).Str("runner_id", runnerID).Msg("Failed to update runner heartbeat")
	}

	return runner.RDPPassword, nil
}

// startSessionRDPProxy handles WebSocket connections for session RDP proxy
func (apiServer *HelixAPIServer) startSessionRDPProxy(w http.ResponseWriter, r *http.Request) {
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

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", user.ID).
		Msg("Starting session RDP proxy WebSocket")

	// Verify RDP proxy manager is available
	if apiServer.rdpProxyManager == nil {
		http.Error(w, "RDP proxy manager not available", http.StatusServiceUnavailable)
		return
	}

	// Get the proxy connection for this session (should already be created by getSessionRDPConnection)
	proxy, err := apiServer.rdpProxyManager.GetSessionProxy(sessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("RDP proxy not found for session %s", sessionID), http.StatusNotFound)
		return
	}

	// Upgrade connection to WebSocket
	conn, err := userWebsocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to upgrade WebSocket for session RDP")
		return
	}
	defer conn.Close()

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", proxy.RunnerID).
		Msg("WebSocket upgraded for session RDP proxy")

	// Proxy WebSocket traffic to the TCP RDP proxy
	apiServer.proxyWebSocketToTCPForSession(ctx, conn, proxy, sessionID)
}

// proxyWebSocketToTCPForSession proxies WebSocket traffic to TCP RDP proxy for sessions
func (apiServer *HelixAPIServer) proxyWebSocketToTCPForSession(ctx context.Context, wsConn *websocket.Conn, proxy *RDPProxyConnection, sessionID string) {
	// Connect to the local TCP proxy
	tcpConn, err := net.Dial("tcp", fmt.Sprintf("localhost:%d", proxy.LocalPort))
	if err != nil {
		log.Error().Err(err).
			Str("session_id", sessionID).
			Str("runner_id", proxy.RunnerID).
			Int("port", proxy.LocalPort).
			Msg("Failed to connect to session RDP TCP proxy")
		wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to connect to RDP server"))
		return
	}
	defer tcpConn.Close()

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", proxy.RunnerID).
		Int("port", proxy.LocalPort).
		Msg("Connected to session RDP TCP proxy")

	// Bidirectional proxy between WebSocket and TCP
	done := make(chan struct{})

	// WebSocket -> TCP
	go func() {
		defer func() {
			select {
			case done <- struct{}{}:
			default:
			}
		}()

		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					log.Debug().Err(err).
						Str("session_id", sessionID).
						Msg("WebSocket read error for session RDP")
				}
				return
			}

			_, err = tcpConn.Write(message)
			if err != nil {
				log.Error().Err(err).
					Str("session_id", sessionID).
					Msg("Failed to write to TCP connection for session RDP")
				return
			}
		}
	}()

	// TCP -> WebSocket
	go func() {
		defer func() {
			select {
			case done <- struct{}{}:
			default:
			}
		}()

		buffer := make([]byte, 4096)
		for {
			n, err := tcpConn.Read(buffer)
			if err != nil {
				if err != io.EOF {
					log.Debug().Err(err).
						Str("session_id", sessionID).
						Msg("TCP read error for session RDP")
				}
				return
			}

			err = wsConn.WriteMessage(websocket.BinaryMessage, buffer[:n])
			if err != nil {
				log.Error().Err(err).
					Str("session_id", sessionID).
					Msg("Failed to write to WebSocket for session RDP")
				return
			}
		}
	}()

	// Wait for either direction to close
	<-done
	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", proxy.RunnerID).
		Msg("Session RDP WebSocket proxy connection closed")
}

// sendCommandToExternalAgentHandler allows manual command sending for testing
func (apiServer *HelixAPIServer) sendCommandToExternalAgentHandler(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	var command types.ExternalAgentCommand
	err := json.NewDecoder(req.Body).Decode(&command)
	if err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to send command to external agent")
		http.Error(res, fmt.Sprintf("failed to send command: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"message":    "Command sent successfully",
		"session_id": sessionID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}
