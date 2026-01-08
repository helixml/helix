package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// addUserAPITokenToAgent adds the user's API token to agent environment for git operations
// This ensures RBAC is enforced - agent can only access repos the user can access
// IMPORTANT: Only uses personal API keys (not app-scoped keys) to ensure full access
func (apiServer *HelixAPIServer) addUserAPITokenToAgent(ctx context.Context, agent *types.ZedAgent, userID string) error {
	userAPIKey, err := apiServer.specDrivenTaskService.GetOrCreateSandboxAPIKey(ctx, &services.SandboxAPIKeyRequest{
		UserID:     userID,
		ProjectID:  agent.ProjectPath,
		SpecTaskID: agent.SpecTaskID,
	})
	if err != nil {
		return fmt.Errorf("failed to get user API key for external agent: %w", err)
	}

	// Add USER_API_TOKEN to agent environment using personal API key
	agent.Env = append(agent.Env, fmt.Sprintf("USER_API_TOKEN=%s", userAPIKey))

	log.Debug().
		Str("user_id", userID).
		Bool("key_is_personal", true).
		Msg("Added user API token to agent for git operations")

	return nil
}

// RegisterRequestToSessionMapping registers a request_id to session_id mapping for external agent sessions
// This is used to route initial messages to Zed when it connects via WebSocket
func (apiServer *HelixAPIServer) RegisterRequestToSessionMapping(requestID, sessionID string) {
	apiServer.contextMappingsMutex.Lock()
	if apiServer.requestToSessionMapping == nil {
		apiServer.requestToSessionMapping = make(map[string]string)
	}
	apiServer.requestToSessionMapping[requestID] = sessionID
	apiServer.contextMappingsMutex.Unlock()
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

	// CRITICAL: Set UserID from authenticated user for git commit email lookup
	// wolf_executor uses agent.UserID to look up user email for git config
	agent.UserID = user.ID

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
	apiServer.contextMappingsMutex.Lock()
	if apiServer.externalAgentUserMapping == nil {
		apiServer.externalAgentUserMapping = make(map[string]string)
	}
	apiServer.externalAgentUserMapping[agent.SessionID] = user.ID
	apiServer.contextMappingsMutex.Unlock()

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
	apiServer.contextMappingsMutex.Lock()
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
	apiServer.contextMappingsMutex.Unlock()

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
	apiServer.contextMappingsMutex.Lock()
	if apiServer.sessionToWaitingInteraction == nil {
		apiServer.sessionToWaitingInteraction = make(map[string]string)
	}
	apiServer.sessionToWaitingInteraction[createdSession.ID] = createdInteraction.ID
	apiServer.contextMappingsMutex.Unlock()

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

	// Add user's API token for git operations
	if err := apiServer.addUserAPITokenToAgent(req.Context(), &agent, user.ID); err != nil {
		log.Warn().Err(err).Str("user_id", user.ID).Msg("Failed to add user API token (continuing without git)")
		// Don't fail - external agents can work without git
	}

	// Start the external agent
	response, err := apiServer.externalAgentExecutor.StartDesktop(req.Context(), &agent)
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

	// Store the lobby ID, PIN, and Wolf instance ID in the Helix session (Phase 3: Multi-tenancy)
	if response.WolfLobbyID != "" {
		createdSession.Metadata.WolfLobbyID = response.WolfLobbyID
	}
	if response.WolfLobbyPIN != "" {
		createdSession.Metadata.WolfLobbyPIN = response.WolfLobbyPIN
	}
	// CRITICAL: Store WolfInstanceID on session record - required for Moonlight streaming proxy
	// Without this, the moonlight proxy falls back to "moonlight-dev" which doesn't exist
	if response.WolfInstanceID != "" {
		createdSession.WolfInstanceID = response.WolfInstanceID
	}

	// Update session with Wolf lobby info and instance ID
	if response.WolfLobbyID != "" || response.WolfLobbyPIN != "" || response.WolfInstanceID != "" {
		_, err = apiServer.Controller.Options.Store.UpdateSession(req.Context(), *createdSession)
		if err != nil {
			log.Error().Err(err).Str("session_id", createdSession.ID).Msg("Failed to store Wolf lobby info in session")
			// Continue anyway - lobby info just won't be in database
		} else {
			log.Info().
				Str("helix_session_id", createdSession.ID).
				Str("lobby_id", response.WolfLobbyID).
				Str("lobby_pin", response.WolfLobbyPIN).
				Str("wolf_instance_id", response.WolfInstanceID).
				Msg("✅ Stored Wolf lobby ID, PIN, and instance ID in Helix session")
		}
	}

	// Note: Immediate lobby attachment happens via Wolf's pending_session_configs
	// Pre-configured by ConfigurePendingSession API call after lobby creation
	// Session auto-attaches to lobby interpipe when Moonlight client connects

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

	err := apiServer.externalAgentExecutor.StopDesktop(req.Context(), sessionID)
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

	// Try RevDial connection to sandbox first (registered as "sandbox-{session_id}")
	// RevDial is the primary communication mechanism - Wolf API lookup is only for debugging
	runnerID := fmt.Sprintf("sandbox-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP request over RevDial tunnel
	httpReq, err := http.NewRequest("GET", "http://localhost:9876/screenshot", nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create screenshot request")
		http.Error(res, "Failed to create screenshot request", http.StatusInternalServerError)
		return
	}

	// Write request to RevDial connection
	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write request to RevDial connection")
		http.Error(res, "Failed to send screenshot request", http.StatusInternalServerError)
		return
	}

	// Read response from RevDial connection
	screenshotResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read screenshot response from RevDial")
		http.Error(res, "Failed to read screenshot response", http.StatusInternalServerError)
		return
	}
	defer screenshotResp.Body.Close()

	// Check screenshot server response status
	if screenshotResp.StatusCode != http.StatusOK {
		// Read response body for debugging
		errorBody, _ := io.ReadAll(screenshotResp.Body)
		log.Error().
			Int("status", screenshotResp.StatusCode).
			Str("session_id", sessionID).
			Str("error_body", string(errorBody)).
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

}

// @Summary Bandwidth probe for adaptive bitrate
// @Description Returns random uncompressible data for measuring available bandwidth.
// @Description This endpoint starts sending bytes immediately, unlike screenshot which
// @Description has capture latency. Used by adaptive bitrate algorithm to probe throughput.
// @Tags ExternalAgents
// @Produce application/octet-stream
// @Param sessionID path string true "Session ID"
// @Param size query int false "Size of data to return in bytes (default 524288 = 512KB)"
// @Success 200 {file} binary
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/bandwidth-probe [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getBandwidthProbe(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Verify session ownership (lightweight check - just verify session exists and user owns it)
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	if session.Owner != user.ID {
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse size parameter (default 512KB)
	size := 524288 // 512KB default
	if sizeStr := req.URL.Query().Get("size"); sizeStr != "" {
		if parsedSize, err := strconv.Atoi(sizeStr); err == nil && parsedSize > 0 && parsedSize <= 10*1024*1024 {
			size = parsedSize // Max 10MB to prevent abuse
		}
	}

	// Generate random data - crypto/rand produces incompressible data
	// This ensures we're measuring actual bandwidth, not compression efficiency
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		log.Error().Err(err).Msg("Failed to generate random data for bandwidth probe")
		http.Error(res, "Failed to generate probe data", http.StatusInternalServerError)
		return
	}

	// Set headers to prevent caching and compression
	res.Header().Set("Content-Type", "application/octet-stream")
	res.Header().Set("Content-Length", strconv.Itoa(size))
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.Header().Set("Content-Encoding", "identity") // Explicitly disable compression
	res.WriteHeader(http.StatusOK)

	// Write data directly - starts sending immediately
	res.Write(data)
}

// @Summary Initial bandwidth probe (no session required)
// @Description Returns random uncompressible data for measuring available bandwidth before session creation.
// @Description Used by adaptive bitrate to determine optimal initial bitrate before connecting.
// @Description Only requires authentication, not session ownership.
// @Tags ExternalAgents
// @Produce application/octet-stream
// @Param size query int false "Size of data to return in bytes (default 524288 = 512KB)"
// @Success 200 {file} binary
// @Failure 401 {object} system.HTTPError
// @Router /api/v1/bandwidth-probe [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getInitialBandwidthProbe(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse size parameter (default 512KB, max 2MB for initial probe to limit abuse)
	size := 524288 // 512KB default
	if sizeStr := req.URL.Query().Get("size"); sizeStr != "" {
		if parsedSize, err := strconv.Atoi(sizeStr); err == nil && parsedSize > 0 && parsedSize <= 2*1024*1024 {
			size = parsedSize // Max 2MB for initial probe (smaller than session probe)
		}
	}

	// Generate random data - crypto/rand produces incompressible data
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		log.Error().Err(err).Msg("Failed to generate random data for initial bandwidth probe")
		http.Error(res, "Failed to generate probe data", http.StatusInternalServerError)
		return
	}

	// Set headers to prevent caching and compression
	res.Header().Set("Content-Type", "application/octet-stream")
	res.Header().Set("Content-Length", strconv.Itoa(size))
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.Header().Set("Content-Encoding", "identity")
	res.WriteHeader(http.StatusOK)

	res.Write(data)
}

// @Summary Get session clipboard content
// @Description Fetch current clipboard content from remote desktop
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Success 200 {object} types.ClipboardData
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/clipboard [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getExternalAgentClipboard(res http.ResponseWriter, req *http.Request) {
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

	// Get container name using Wolf executor
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

	// Get RevDial connection to sandbox (registered as "sandbox-{session_id}")
	runnerID := fmt.Sprintf("sandbox-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP request over RevDial tunnel
	httpReq, err := http.NewRequest("GET", "http://localhost:9876/clipboard", nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create clipboard request")
		http.Error(res, "Failed to create clipboard request", http.StatusInternalServerError)
		return
	}

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write clipboard request to RevDial")
		http.Error(res, "Failed to send clipboard request", http.StatusInternalServerError)
		return
	}

	clipboardResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read clipboard response from RevDial")
		http.Error(res, "Failed to read clipboard response", http.StatusInternalServerError)
		return
	}
	defer clipboardResp.Body.Close()

	// Check clipboard server response status
	if clipboardResp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", clipboardResp.StatusCode).
			Str("container_name", containerName).
			Msg("Clipboard server returned error")
		http.Error(res, "Failed to retrieve clipboard from container", clipboardResp.StatusCode)
		return
	}

	// Return clipboard data directly (JSON format with type and data)
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)

	// Stream the clipboard JSON from clipboard server to response
	_, err = io.Copy(res, clipboardResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to stream clipboard data")
		return
	}

	log.Trace().
		Str("session_id", sessionID).
		Msg("Successfully retrieved clipboard from external agent container")
}

// @Summary Set session clipboard content
// @Description Send clipboard content to remote desktop
// @Tags ExternalAgents
// @Accept json
// @Param sessionID path string true "Session ID"
// @Param clipboard body types.ClipboardData true "Clipboard data to set"
// @Success 200
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/clipboard [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) setExternalAgentClipboard(res http.ResponseWriter, req *http.Request) {
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

	// Get container name using Wolf executor
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

	// Read clipboard content from request body (JSON format)
	clipboardContent, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		http.Error(res, "Failed to read clipboard content", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("container_name", containerName).
		Int("clipboard_size", len(clipboardContent)).
		Msg("Setting clipboard in sandbox via RevDial")

	// Get RevDial connection to sandbox (registered as "sandbox-{session_id}")
	runnerID := fmt.Sprintf("sandbox-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP POST request over RevDial tunnel
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/clipboard", bytes.NewReader(clipboardContent))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create clipboard POST request")
		http.Error(res, "Failed to create clipboard request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write clipboard POST to RevDial")
		http.Error(res, "Failed to send clipboard request", http.StatusInternalServerError)
		return
	}

	clipboardResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read clipboard POST response from RevDial")
		http.Error(res, "Failed to set clipboard", http.StatusInternalServerError)
		return
	}
	defer clipboardResp.Body.Close()

	// Check clipboard server response status
	if clipboardResp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", clipboardResp.StatusCode).
			Str("container_name", containerName).
			Msg("Clipboard server returned error")
		http.Error(res, "Failed to set clipboard in container", clipboardResp.StatusCode)
		return
	}

	res.WriteHeader(http.StatusOK)
	log.Info().
		Str("session_id", sessionID).
		Int("clipboard_size", len(clipboardContent)).
		Msg("Successfully set clipboard in external agent container")
}

// @Summary Send input events to sandbox
// @Description Send keyboard and mouse input events to the remote desktop. Supports single events or batches.
// @Tags ExternalAgents
// @Accept json
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param input body object true "Input event(s). Single event: {type, keycode, state} or batch: {events: [...]}"
// @Success 200 {object} object "success response with processed count"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/input [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) sendInputToSandbox(res http.ResponseWriter, req *http.Request) {
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
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for input")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for input")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get container name using Wolf executor
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	_, err = apiServer.externalAgentExecutor.FindContainerBySessionID(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to find external agent container for input")
		http.Error(res, "External agent container not found", http.StatusNotFound)
		return
	}

	// Read input content from request body
	inputContent, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read input request body")
		http.Error(res, "Failed to read input content", http.StatusBadRequest)
		return
	}

	// Get RevDial connection to sandbox (registered as "sandbox-{session_id}")
	runnerID := fmt.Sprintf("sandbox-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for input")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP POST request over RevDial tunnel
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/input", bytes.NewReader(inputContent))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create input request")
		http.Error(res, "Failed to create input request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write input request to RevDial")
		http.Error(res, "Failed to send input request", http.StatusInternalServerError)
		return
	}

	inputResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read input response from RevDial")
		http.Error(res, "Failed to read input response", http.StatusInternalServerError)
		return
	}
	defer inputResp.Body.Close()

	// Read and forward response
	respBody, err := io.ReadAll(inputResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read input response body")
		http.Error(res, "Failed to read input response", http.StatusInternalServerError)
		return
	}

	// Forward status and body
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(inputResp.StatusCode)
	res.Write(respBody)

	log.Trace().
		Str("session_id", sessionID).
		Int("input_size", len(inputContent)).
		Int("status", inputResp.StatusCode).
		Msg("Input event(s) sent to sandbox")
}

// @Summary Upload file to sandbox
// @Description Upload a file to the sandbox incoming folder (~/work/incoming/). Files can be dragged and dropped onto the sandbox viewer to upload them.
// @Tags ExternalAgents
// @Accept multipart/form-data
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param file formData file true "File to upload"
// @Success 200 {object} types.SandboxFileUploadResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/upload [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) uploadFileToSandbox(res http.ResponseWriter, req *http.Request) {
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
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for file upload")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for file upload")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get container name using Wolf executor
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	_, err = apiServer.externalAgentExecutor.FindContainerBySessionID(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to find external agent container for file upload")
		http.Error(res, "External agent container not found", http.StatusNotFound)
		return
	}

	// Read the multipart body to forward it
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read upload request body")
		http.Error(res, "Failed to read file", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Int("body_size", len(bodyBytes)).
		Str("content_type", req.Header.Get("Content-Type")).
		Msg("Uploading file to sandbox via RevDial")

	// Get RevDial connection to sandbox (registered as "sandbox-{session_id}")
	runnerID := fmt.Sprintf("sandbox-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for file upload")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP POST request over RevDial tunnel
	// Important: preserve the Content-Type header with multipart boundary
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/upload", bytes.NewReader(bodyBytes))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create upload request")
		http.Error(res, "Failed to create upload request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", req.Header.Get("Content-Type"))
	httpReq.ContentLength = int64(len(bodyBytes))

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write upload request to RevDial")
		http.Error(res, "Failed to send upload request", http.StatusInternalServerError)
		return
	}

	uploadResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read upload response from RevDial")
		http.Error(res, "Failed to read upload response", http.StatusInternalServerError)
		return
	}
	defer uploadResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read upload response body")
		http.Error(res, "Failed to read upload response", http.StatusInternalServerError)
		return
	}

	// Check upload server response status
	if uploadResp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", uploadResp.StatusCode).
			Str("response", string(respBody)).
			Msg("Screenshot server returned error for upload")
		http.Error(res, string(respBody), uploadResp.StatusCode)
		return
	}

	// Return the response from screenshot server
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(respBody)

	log.Info().
		Str("session_id", sessionID).
		Str("response", string(respBody)).
		Msg("Successfully uploaded file to sandbox")
}

// ConfigurePendingSessionRequest is the request body for configuring a pending session
type ConfigurePendingSessionRequest struct {
	ClientUniqueID string `json:"client_unique_id"`
}

// configurePendingSession handles POST /api/v1/external-agents/{sessionID}/configure-pending-session
// @Summary Configure pending session for immediate lobby attachment
// @Description Pre-configures Wolf to attach a client to a lobby when it connects with the given client_unique_id.
// The frontend calls this BEFORE connecting to moonlight-web with the same client_unique_id.
// @Tags ExternalAgents
// @Accept json
// @Produce json
// @Param sessionID path string true "External agent session ID"
// @Param request body ConfigurePendingSessionRequest true "Configuration request"
// @Success 200 {object} map[string]string "success response"
// @Failure 400 {string} string "Bad request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 403 {string} string "Forbidden"
// @Failure 404 {string} string "Session not found"
// @Failure 503 {string} string "Wolf executor not available"
// @Router /api/v1/external-agents/{sessionID}/configure-pending-session [post]
// @Security ApiKeyAuth
func (apiServer *HelixAPIServer) configurePendingSession(res http.ResponseWriter, req *http.Request) {
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

	// Parse request body
	var configReq ConfigurePendingSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&configReq); err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if configReq.ClientUniqueID == "" {
		http.Error(res, "client_unique_id is required", http.StatusBadRequest)
		return
	}

	// Configure pending session via executor
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	if err := apiServer.externalAgentExecutor.ConfigurePendingSession(req.Context(), sessionID, configReq.ClientUniqueID); err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to configure pending session")
		http.Error(res, fmt.Sprintf("failed to configure pending session: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"status":           "configured",
		"session_id":       sessionID,
		"client_unique_id": configReq.ClientUniqueID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// proxyInputWebSocket handles WebSocket /api/v1/external-agents/{sessionID}/ws/input
// This provides direct input from browser to screenshot-server, bypassing Moonlight/Wolf.
// @Summary Direct WebSocket input for PipeWire/GNOME sessions
// @Description Provides a WebSocket connection for sending input events directly to the screenshot-server
// in the sandbox. This bypasses Moonlight/Wolf for input, providing better control over scroll behavior.
// Only available for PipeWire/GNOME desktop sessions.
// @Tags ExternalAgents
// @Param sessionID path string true "Session ID"
// @Success 101 "Switching Protocols"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/ws/input [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) proxyInputWebSocket(res http.ResponseWriter, req *http.Request) {
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
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for input WebSocket")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for input WebSocket")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Check if this is a PipeWire/GNOME session (Ubuntu desktop)
	// For Sway sessions, return an error - they should use Moonlight input
	var desktopType string
	if session.Metadata.ExternalAgentConfig != nil {
		desktopType = session.Metadata.ExternalAgentConfig.GetEffectiveDesktopType()
	} else {
		desktopType = "ubuntu" // Default to ubuntu if no config
	}
	if desktopType != "ubuntu" {
		log.Warn().Str("session_id", sessionID).Str("desktop_type", desktopType).Msg("Direct input WebSocket not supported for non-Ubuntu sessions")
		http.Error(res, "Direct input WebSocket only supported for Ubuntu/GNOME sessions", http.StatusNotImplemented)
		return
	}

	// Get RevDial connection to sandbox (registered as "sandbox-{session_id}")
	runnerID := fmt.Sprintf("sandbox-%s", sessionID)

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Msg("Proxying input WebSocket to screenshot-server via RevDial")

	// Hijack the HTTP connection to get the underlying net.Conn
	hijacker, ok := res.(http.Hijacker)
	if !ok {
		log.Error().Msg("ResponseWriter doesn't support Hijacker interface")
		http.Error(res, "Server doesn't support connection hijacking", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Error().Err(err).Msg("Failed to hijack connection")
		http.Error(res, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Get RevDial connection to the screenshot-server
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	serverConn, err := apiServer.connman.Dial(ctx, runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for input WebSocket")
		// Write HTTP error response since we've already hijacked
		clientConn.Write([]byte("HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain\r\n\r\nSandbox not connected"))
		return
	}
	defer serverConn.Close()

	// Construct WebSocket upgrade request to forward to screenshot-server
	upgradeReq := fmt.Sprintf("GET /ws/input HTTP/1.1\r\n"+
		"Host: localhost:9876\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n", req.Header.Get("Sec-WebSocket-Key"))

	// Forward the WebSocket upgrade request
	if _, err := serverConn.Write([]byte(upgradeReq)); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket upgrade to screenshot-server")
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nFailed to connect to screenshot-server"))
		return
	}

	// Read the upgrade response from screenshot-server
	serverReader := bufio.NewReader(serverConn)
	upgradeResp, err := http.ReadResponse(serverReader, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read WebSocket upgrade response from screenshot-server")
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nScreenshot-server connection failed"))
		return
	}
	defer upgradeResp.Body.Close()

	// Check if upgrade was successful
	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		log.Error().Int("status", upgradeResp.StatusCode).Msg("Screenshot-server didn't accept WebSocket upgrade")
		clientConn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n\r\n", upgradeResp.StatusCode, upgradeResp.Status)))
		return
	}

	// Forward the 101 Switching Protocols response to the client
	upgradeRespBytes := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n"+
		"\r\n", upgradeResp.Header.Get("Sec-WebSocket-Accept"))

	if _, err := clientConn.Write([]byte(upgradeRespBytes)); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket upgrade response to client")
		return
	}

	log.Info().Str("session_id", sessionID).Msg("Input WebSocket connection established, starting bidirectional proxy")

	// Bidirectional copy between client and server
	done := make(chan struct{})

	// Client -> Server
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(serverConn, clientConn)
	}()

	// Server -> Client
	go func() {
		defer func() { done <- struct{}{} }()
		io.Copy(clientConn, serverConn)
	}()

	// Wait for either direction to complete
	<-done

	log.Info().Str("session_id", sessionID).Msg("Input WebSocket connection closed")
}
