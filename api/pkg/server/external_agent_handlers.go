package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

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

	// Store user mapping for this external agent session
	if apiServer.externalAgentUserMapping == nil {
		apiServer.externalAgentUserMapping = make(map[string]string)
	}
	apiServer.externalAgentUserMapping[agent.SessionID] = user.ID

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", user.ID).
		Msg("Stored user mapping for external agent session")

	// Create Helix session FIRST so responses can be routed to it
	helixSession := types.Session{
		ID:        "", // Will be generated
		Name:      fmt.Sprintf("Zed Agent %s", agent.SessionID[:8]),
		Owner:     user.ID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		ModelName: "claude-sonnet-4-5-latest", // Force Sonnet 4.5 for external agents
		Created:   time.Now(),
		Updated:   time.Now(),
		Metadata: types.SessionMetadata{
			SystemPrompt: "You are a helpful AI assistant integrated with Zed editor.",
			AgentType:    "zed_external",
		},
	}

	createdSession, err := apiServer.Controller.Options.Store.CreateSession(req.Context(), helixSession)
	if err != nil {
		log.Error().Err(err).Str("agent_session_id", agent.SessionID).Msg("failed to create Helix session")
		http.Error(res, fmt.Sprintf("failed to create Helix session: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("agent_session_id", agent.SessionID).
		Str("helix_session_id", createdSession.ID).
		Str("user_id", user.ID).
		Msg("✅ Created Helix session for external agent")

	// Store mapping: agent_session_id -> helix_session_id
	if apiServer.externalAgentSessionMapping == nil {
		apiServer.externalAgentSessionMapping = make(map[string]string)
	}
	apiServer.externalAgentSessionMapping[agent.SessionID] = createdSession.ID

	// Generate request_id for initial message and store mapping
	requestID := system.GenerateRequestID()
	if apiServer.requestToSessionMapping == nil {
		apiServer.requestToSessionMapping = make(map[string]string)
	}
	apiServer.requestToSessionMapping[requestID] = createdSession.ID

	log.Info().
		Str("request_id", requestID).
		Str("helix_session_id", createdSession.ID).
		Msg("✅ Stored request_id -> session mapping for initial message")

	// Create initial interaction for the user's message
	interaction := &types.Interaction{
		ID:              "", // Will be generated
		GenerationID:    0,
		Created:         time.Now(),
		Updated:         time.Now(),
		Scheduled:       time.Now(),
		Completed:       time.Time{},
		SessionID:       createdSession.ID,
		UserID:          user.ID,
		Mode:            types.SessionModeInference,
		PromptMessage:   agent.Input,
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	createdInteraction, err := apiServer.Controller.Options.Store.CreateInteraction(req.Context(), interaction)
	if err != nil {
		log.Error().Err(err).Str("helix_session_id", createdSession.ID).Msg("failed to create initial interaction")
		http.Error(res, fmt.Sprintf("failed to create interaction: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Store session -> waiting interaction mapping
	if apiServer.sessionToWaitingInteraction == nil {
		apiServer.sessionToWaitingInteraction = make(map[string]string)
	}
	apiServer.sessionToWaitingInteraction[createdSession.ID] = createdInteraction.ID

	log.Info().
		Str("interaction_id", createdInteraction.ID).
		Str("helix_session_id", createdSession.ID).
		Msg("✅ Created initial interaction for external agent")

	// Generate auth token for WebSocket connection
	token, err := apiServer.generateExternalAgentToken(agent.SessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to generate external agent token")
		http.Error(res, fmt.Sprintf("failed to create external agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Set the Helix session ID on the agent so Wolf knows which session this serves
	agent.HelixSessionID = createdSession.ID

	// Start the external agent
	response, err := apiServer.externalAgentExecutor.StartZedAgent(req.Context(), &agent)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to start external agent")
		http.Error(res, fmt.Sprintf("failed to start external agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("status", response.Status).
		Str("lobby_id", response.WolfLobbyID).
		Msg("External agent started successfully")

	// Store the lobby PIN in the Helix session metadata (Phase 3: Multi-tenancy)
	if response.WolfLobbyPIN != "" {
		createdSession.Metadata.WolfLobbyPIN = response.WolfLobbyPIN
		_, err = apiServer.Controller.Options.Store.UpdateSession(req.Context(), *createdSession)
		if err != nil {
			log.Error().Err(err).Str("session_id", createdSession.ID).Msg("Failed to store lobby PIN in session")
			// Continue anyway - PIN just won't be in database
		} else {
			log.Info().
				Str("helix_session_id", createdSession.ID).
				Str("lobby_pin", response.WolfLobbyPIN).
				Msg("✅ Stored lobby PIN in Helix session metadata")
		}
	}

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
		Status:    "running",
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
			Status:    session.Status,
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
		Msg("Found external agent session (RDP replaced with Wolf)")

	// Return Wolf-based connection details with WebSocket info
	connectionInfo := map[string]interface{}{
		"session_id":          session.SessionID,
		"screenshot_url":      fmt.Sprintf("/api/v1/external-agents/%s/screenshot", session.SessionID),
		"stream_url":          "moonlight://localhost:47989",
		"status":              session.Status,
		"websocket_url":       fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, session.SessionID),
		"websocket_connected": apiServer.IsExternalAgentConnected(session.SessionID),
	}

	log.Info().
		Str("session_id", session.SessionID).
		Str("status", session.Status).
		Bool("websocket_connected", connectionInfo["websocket_connected"].(bool)).
		Msg("Returning Wolf connection info")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(connectionInfo)
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
		SessionID:     session.SessionID,
		ScreenshotURL: fmt.Sprintf("/api/v1/external-agents/%s/screenshot", session.SessionID),
		StreamURL:     "moonlight://localhost:47989",
		Status:        session.Status,
		WolfAppID:     session.WolfAppID,
		ContainerName: session.ContainerName,
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

// IsExternalAgentConnected checks if external agent is connected via WebSocket
// This implements the external_agent.WebSocketConnectionChecker interface
func (apiServer *HelixAPIServer) IsExternalAgentConnected(sessionID string) bool {
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

// getExternalAgentScreenshot handles GET /api/v1/external-agents/{sessionID}/screenshot
func (apiServer *HelixAPIServer) getExternalAgentScreenshot(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get container name using Docker API - external agent containers have HELIX_SESSION_ID env var
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	containerName, err := apiServer.externalAgentExecutor.FindContainerBySessionID(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to find external agent container")
		http.Error(res, "External agent container not found", http.StatusNotFound)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("session_id", sessionID).
		Str("container_name", containerName).
		Msg("Requesting screenshot from external agent container screenshot server")

	// Make HTTP request to screenshot server inside the container
	screenshotURL := fmt.Sprintf("http://%s:9876/screenshot", containerName)

	screenshotReq, err := http.NewRequestWithContext(req.Context(), "GET", screenshotURL, nil)
	if err != nil {
		log.Error().Err(err).Str("container_name", containerName).Msg("Failed to create screenshot request")
		http.Error(res, "Failed to create screenshot request", http.StatusInternalServerError)
		return
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	screenshotResp, err := httpClient.Do(screenshotReq)
	if err != nil {
		log.Error().Err(err).Str("container_name", containerName).Msg("Failed to get screenshot from container")
		http.Error(res, "Failed to retrieve screenshot", http.StatusInternalServerError)
		return
	}
	defer screenshotResp.Body.Close()

	// Check screenshot server response status
	if screenshotResp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", screenshotResp.StatusCode).
			Str("container_name", containerName).
			Msg("Screenshot server returned error")
		http.Error(res, "Failed to retrieve screenshot from container", screenshotResp.StatusCode)
		return
	}

	// Return PNG image directly
	res.Header().Set("Content-Type", "image/png")
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.WriteHeader(http.StatusOK)

	// Stream the PNG data from screenshot server to response
	_, err = io.Copy(res, screenshotResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to stream screenshot data")
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("container_name", containerName).
		Msg("Successfully retrieved screenshot from external agent container")
}

// @Summary Get keepalive session status
// @Description Get keepalive session health status for an external agent
// @Tags ExternalAgents
// @Accept json
// @Produce json
// @Param sessionID path string true "Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/external-agents/{sessionID}/keepalive [get]
func (apiServer *HelixAPIServer) getExternalAgentKeepaliveStatus(res http.ResponseWriter, req *http.Request) {
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
		http.Error(res, "external agent executor not available", http.StatusServiceUnavailable)
		return
	}

	// Get session from executor
	session, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Session not found for keepalive status")
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	// Calculate connection uptime
	var uptimeSeconds int64
	if session.KeepaliveStartTime != nil {
		uptimeSeconds = int64(time.Since(*session.KeepaliveStartTime).Seconds())
	}

	// Build response
	response := map[string]interface{}{
		"session_id":              session.SessionID,
		"lobby_id":                session.WolfLobbyID,
		"keepalive_status":        session.KeepaliveStatus,
		"keepalive_start_time":    session.KeepaliveStartTime,
		"keepalive_last_check":    session.KeepaliveLastCheck,
		"connection_uptime_seconds": uptimeSeconds,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}
