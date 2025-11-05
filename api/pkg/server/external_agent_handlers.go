package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
)

// RegisterRequestToSessionMapping registers a request_id to session_id mapping for external agent sessions
// This is used to route initial messages to Zed when it connects via WebSocket
func (apiServer *HelixAPIServer) RegisterRequestToSessionMapping(requestID, sessionID string) {
	if apiServer.requestToSessionMapping == nil {
		apiServer.requestToSessionMapping = make(map[string]string)
	}
	apiServer.requestToSessionMapping[requestID] = sessionID
	log.Info().
		Str("request_id", requestID).
		Str("session_id", sessionID).
		Msg("✅ Registered request_id -> session_id mapping")
}

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

	// WorkDir will be set by wolf_executor to use filestore path
	// Don't override it here - let executor handle workspace management

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

	// Store the lobby ID and PIN in the Helix session metadata (Phase 3: Multi-tenancy)
	if response.WolfLobbyID != "" {
		createdSession.Metadata.WolfLobbyID = response.WolfLobbyID
	}
	if response.WolfLobbyPIN != "" {
		createdSession.Metadata.WolfLobbyPIN = response.WolfLobbyPIN
	}

	// Update session with Wolf lobby info
	if response.WolfLobbyID != "" || response.WolfLobbyPIN != "" {
		_, err = apiServer.Controller.Options.Store.UpdateSession(req.Context(), *createdSession)
		if err != nil {
			log.Error().Err(err).Str("session_id", createdSession.ID).Msg("Failed to store Wolf lobby info in session")
			// Continue anyway - lobby info just won't be in database
		} else {
			log.Info().
				Str("helix_session_id", createdSession.ID).
				Str("lobby_id", response.WolfLobbyID).
				Str("lobby_pin", response.WolfLobbyPIN).
				Msg("✅ Stored Wolf lobby ID and PIN in Helix session metadata")
		}
	}

	// Note: Auto-join happens AFTER user connects via moonlight-web
	// See autoJoinExternalAgentLobby endpoint - called by frontend post-connection

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
	connectionInfo := types.ExternalAgentConnectionInfo{
		SessionID:          session.SessionID,
		ScreenshotURL:      fmt.Sprintf("/api/v1/external-agents/%s/screenshot", session.SessionID),
		StreamURL:          "moonlight://localhost:47989",
		Status:             session.Status,
		WebsocketURL:       fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, session.SessionID),
		WebsocketConnected: apiServer.isExternalAgentConnected(session.SessionID),
	}

	log.Info().
		Str("session_id", session.SessionID).
		Str("status", session.Status).
		Bool("websocket_connected", connectionInfo.WebsocketConnected).
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

	var updateData types.ExternalAgentUpdateRequest
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

	stats := types.ExternalAgentStats{
		SessionID:    session.SessionID,
		PID:          0,
		StartTime:    session.StartTime,
		LastAccess:   session.LastAccess,
		Uptime:       session.LastAccess.Sub(session.StartTime).Seconds(),
		WorkspaceDir: session.ProjectPath,
		DisplayNum:   1,
		RDPPort:      8080,
		Status:       "running",
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
	logs := types.ExternalAgentLogs{
		SessionID: sessionID,
		Lines:     lines,
		Logs: []string{
			"[INFO] Zed editor started",
			"[INFO] X server initialized",
			"[INFO] XRDP server listening on port 3389",
			"[DEBUG] Session active and responding",
		},
		Timestamp: time.Now(),
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

	// Check if agent is paused and has saved screenshot
	if session.Metadata.PausedScreenshotPath != "" {
		// Agent is paused - serve saved screenshot from filestore
		screenshotFile, err := os.Open(session.Metadata.PausedScreenshotPath)
		if err == nil {
			defer screenshotFile.Close()
			res.Header().Set("Content-Type", "image/png")
			res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			res.Header().Set("X-Paused-Screenshot", "true") // Indicate this is a paused screenshot
			res.WriteHeader(http.StatusOK)
			io.Copy(res, screenshotFile)
			return
		}
		// If file not found, fall through to try live screenshot
		log.Warn().Err(err).Str("screenshot_path", session.Metadata.PausedScreenshotPath).Msg("Paused screenshot file not found, trying live screenshot")
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
	response := types.ExternalAgentKeepaliveStatus{
		SessionID:              session.SessionID,
		LobbyID:                session.WolfLobbyID,
		KeepaliveStatus:        session.KeepaliveStatus,
		KeepaliveStartTime:     session.KeepaliveStartTime,
		KeepaliveLastCheck:     session.KeepaliveLastCheck,
		ConnectionUptimeSeconds: uptimeSeconds,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// @Summary Auto-join Wolf lobby after connection
// @Description Automatically join a Wolf lobby after moonlight-web has connected. This endpoint should be called by the frontend after the moonlight-web iframe has loaded and the user has connected to Wolf UI.
// @Tags ExternalAgents
// @Accept json
// @Produce json
// @Param sessionID path string true "Helix Session ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/external-agents/{sessionID}/auto-join-lobby [post]
func (apiServer *HelixAPIServer) autoJoinExternalAgentLobby(res http.ResponseWriter, req *http.Request) {
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

	log.Info().
		Str("session_id", sessionID).
		Str("user_id", user.ID).
		Msg("[AUTO-JOIN] Auto-join lobby request received")

	// Get Helix session
	session, err := apiServer.Controller.Options.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("[AUTO-JOIN] Session not found")
		http.Error(res, "session not found", http.StatusNotFound)
		return
	}

	// Authorize user
	if session.Owner != user.ID && !user.Admin {
		log.Warn().
			Str("session_id", sessionID).
			Str("user_id", user.ID).
			Str("owner_id", session.Owner).
			Msg("[AUTO-JOIN] User not authorized to access session")
		http.Error(res, "forbidden", http.StatusForbidden)
		return
	}

	// Get lobby ID and PIN from session metadata
	lobbyID := session.Metadata.WolfLobbyID
	lobbyPIN := session.Metadata.WolfLobbyPIN

	if lobbyID == "" {
		log.Warn().Str("session_id", sessionID).Msg("[AUTO-JOIN] No lobby associated with this session")
		http.Error(res, "no lobby associated with this session", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("lobby_id", lobbyID).
		Bool("has_pin", lobbyPIN != "").
		Msg("[AUTO-JOIN] Found lobby credentials, attempting auto-join")

	// Call the auto-join function (backend derives client_id securely)
	err = apiServer.autoJoinWolfLobby(req.Context(), sessionID, lobbyID, lobbyPIN)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Str("lobby_id", lobbyID).
			Msg("[AUTO-JOIN] Failed to auto-join lobby")
		http.Error(res, fmt.Sprintf("failed to join lobby: %v", err), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("lobby_id", lobbyID).
		Msg("[AUTO-JOIN] ✅ Successfully auto-joined lobby")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(map[string]interface{}{
		"success":  true,
		"lobby_id": lobbyID,
		"message":  "Successfully auto-joined lobby",
	})
}

// autoJoinWolfLobby performs the actual auto-join operation by finding the Wolf UI session and joining the lobby
// This is a helper function called by autoJoinExternalAgentLobby after the user has connected
// SECURITY: Backend derives wolf_client_id from Wolf API by matching client_unique_id pattern
//           This prevents frontend manipulation of which Wolf client gets joined to the lobby
func (apiServer *HelixAPIServer) autoJoinWolfLobby(ctx context.Context, helixSessionID string, lobbyID string, lobbyPIN string) error {
	// Get Wolf client from executor
	type WolfClientProvider interface {
		GetWolfClient() *wolf.Client
	}
	provider, ok := apiServer.externalAgentExecutor.(WolfClientProvider)
	if !ok {
		return fmt.Errorf("Wolf executor does not provide Wolf client")
	}
	wolfClient := provider.GetWolfClient()

	// Get Wolf apps using the existing ListApps method
	apps, err := wolfClient.ListApps(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Wolf apps: %w", err)
	}

	// Find Wolf UI app
	var wolfUIAppID string
	for _, app := range apps {
		if app.Title == "Wolf UI" {
			wolfUIAppID = app.ID
			break
		}
	}
	if wolfUIAppID == "" {
		return fmt.Errorf("Wolf UI app not found in apps list")
	}

	log.Info().
		Str("lobby_id", lobbyID).
		Str("wolf_ui_app_id", wolfUIAppID).
		Msg("[AUTO-JOIN] Found Wolf UI app, querying sessions to find client")

	// Query Wolf sessions to find the Wolf UI client
	sessionsResp, err := wolfClient.Get(ctx, "/api/v1/sessions")
	if err != nil {
		return fmt.Errorf("failed to query Wolf sessions: %w", err)
	}
	defer sessionsResp.Body.Close()

	if sessionsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(sessionsResp.Body)
		return fmt.Errorf("Wolf sessions API returned status %d: %s", sessionsResp.StatusCode, string(body))
	}

	var sessionsData struct {
		Success  bool `json:"success"`
		Sessions []struct {
			AppID          string `json:"app_id"`
			ClientID       string `json:"client_id"`        // Wolf's session_id (hash of client cert)
			ClientIP       string `json:"client_ip"`        // All same IP in Docker network
			ClientUniqueID string `json:"client_unique_id"` // Moonlight uniqueid for matching
		} `json:"sessions"`
	}
	err = json.NewDecoder(sessionsResp.Body).Decode(&sessionsData)
	if err != nil {
		return fmt.Errorf("failed to parse Wolf sessions response: %w", err)
	}

	// SECURITY: Derive client_id by matching client_unique_id pattern
	// Expected pattern: "helix-agent-{helixSessionID}"
	// This prevents frontend manipulation - backend controls which Wolf client is used
	expectedUniqueID := fmt.Sprintf("helix-agent-%s", helixSessionID)

	log.Info().
		Str("expected_unique_id", expectedUniqueID).
		Str("helix_session_id", helixSessionID).
		Msg("[AUTO-JOIN] Backend deriving Wolf client_id from client_unique_id pattern (secure)")

	// Find the Wolf UI session with matching client_unique_id
	// Prefer: Most recent session (last in list) since old sessions should be cleaned up
	var moonlightSessionID string
	var matchedCount int
	var wolfUISessions []string

	for _, session := range sessionsData.Sessions {
		if session.AppID == wolfUIAppID {
			sessionInfo := fmt.Sprintf("client_id=%s unique_id=%s ip=%s",
				session.ClientID, session.ClientUniqueID, session.ClientIP)
			wolfUISessions = append(wolfUISessions, sessionInfo)

			// SECURITY: Match by client_unique_id prefix pattern (handles FRONTEND_INSTANCE_ID suffix)
			// Expected: "helix-agent-{sessionID}" but actual may be "helix-agent-{sessionID}-{instanceID}"
			// Wolf now properly exposes client_unique_id in /api/v1/sessions
			if session.ClientUniqueID != "" && strings.HasPrefix(session.ClientUniqueID, expectedUniqueID) {
				matchedCount++
				// Use last match (most recent in iteration order)
				// Old sessions should have been cleaned up by moonlight-web on disconnect
				moonlightSessionID = session.ClientID
				log.Info().
					Str("matched_client_id", session.ClientID).
					Str("matched_unique_id", session.ClientUniqueID).
					Str("expected_prefix", expectedUniqueID).
					Int("match_number", matchedCount).
					Msg("[AUTO-JOIN] Found matching Wolf UI session by client_unique_id prefix")
			}
		}
	}

	// DEFENSIVE WARNING: Multiple matching sessions found
	if matchedCount > 1 {
		log.Warn().
			Str("expected_unique_id", expectedUniqueID).
			Int("match_count", matchedCount).
			Strs("all_wolf_ui_sessions", wolfUISessions).
			Str("selected_session", moonlightSessionID).
			Msg("[AUTO-JOIN] ⚠️ Multiple matching sessions found - using last one (old sessions should have been cleaned up)")
	} else if matchedCount == 1 {
		log.Info().
			Str("moonlight_session_id", moonlightSessionID).
			Msg("[AUTO-JOIN] ✅ Found unique Wolf UI session match")
	}

	if moonlightSessionID == "" {
		log.Warn().
			Str("expected_unique_id", expectedUniqueID).
			Strs("available_sessions", wolfUISessions).
			Int("total_sessions", len(sessionsData.Sessions)).
			Msg("[AUTO-JOIN] No Wolf UI session found - client may not have connected yet")
		return fmt.Errorf("Wolf UI session not found - client may not have connected yet")
	}

	// DEFENSIVE CHECK: Verify Wolf-UI session still exists (not stopped/orphaned)
	// Re-query Wolf sessions to ensure session wasn't stopped since we last checked
	freshSessionsResp, err := wolfClient.Get(ctx, "/api/v1/sessions")
	if err != nil {
		return fmt.Errorf("failed to re-query Wolf sessions: %w", err)
	}
	defer freshSessionsResp.Body.Close()

	if freshSessionsResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(freshSessionsResp.Body)
		return fmt.Errorf("Wolf sessions API returned status %d: %s", freshSessionsResp.StatusCode, string(body))
	}

	var freshSessionsData struct {
		Success  bool `json:"success"`
		Sessions []struct {
			ClientID string `json:"client_id"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(freshSessionsResp.Body).Decode(&freshSessionsData); err != nil {
		return fmt.Errorf("failed to parse fresh Wolf sessions response: %w", err)
	}

	// Verify the Wolf session we're trying to use still exists
	sessionExists := false
	for _, session := range freshSessionsData.Sessions {
		if session.ClientID == moonlightSessionID {
			sessionExists = true
			break
		}
	}

	if !sessionExists {
		log.Warn().
			Str("moonlight_session_id", moonlightSessionID).
			Str("expected_unique_id", expectedUniqueID).
			Msg("[AUTO-JOIN] Wolf-UI session no longer exists - may have been stopped")
		return fmt.Errorf("Wolf-UI session %s no longer exists", moonlightSessionID)
	}

	log.Info().
		Str("moonlight_session_id", moonlightSessionID).
		Msg("[AUTO-JOIN] ✅ Wolf-UI session exists")

	// DEFENSIVE CHECK 2: Verify lobby exists before attempting join
	lobbies, err := wolfClient.ListLobbies(ctx)
	if err != nil {
		return fmt.Errorf("failed to list lobbies: %w", err)
	}

	lobbyExists := false
	for _, lobby := range lobbies {
		if lobby.ID == lobbyID {
			lobbyExists = true
			break
		}
	}

	if !lobbyExists {
		log.Warn().
			Str("lobby_id", lobbyID).
			Msg("[AUTO-JOIN] Lobby does not exist - may have been stopped")
		return fmt.Errorf("lobby %s does not exist", lobbyID)
	}

	log.Info().
		Str("lobby_id", lobbyID).
		Msg("[AUTO-JOIN] ✅ Lobby exists, proceeding with join")

	// Convert PIN string to array of int16 for Wolf API
	var pinDigits []int16
	if lobbyPIN != "" {
		for _, char := range lobbyPIN {
			digit := int16(char - '0')
			if digit < 0 || digit > 9 {
				return fmt.Errorf("invalid PIN format: %s", lobbyPIN)
			}
			pinDigits = append(pinDigits, digit)
		}
	}

	// Call Wolf API to join the lobby using the existing JoinLobby method
	joinRequest := &wolf.JoinLobbyRequest{
		LobbyID:            lobbyID,
		MoonlightSessionID: moonlightSessionID,
		PIN:                pinDigits,
	}

	log.Info().
		Str("lobby_id", lobbyID).
		Str("moonlight_session_id", moonlightSessionID).
		Int("pin_length", len(pinDigits)).
		Msg("[AUTO-JOIN] Calling Wolf API to join lobby")

	err = wolfClient.JoinLobby(ctx, joinRequest)
	if err != nil {
		return fmt.Errorf("failed to join lobby: %w", err)
	}

	log.Info().
		Str("lobby_id", lobbyID).
		Str("moonlight_session_id", moonlightSessionID).
		Msg("[AUTO-JOIN] ✅ Successfully called JoinLobby API")

	return nil
}

