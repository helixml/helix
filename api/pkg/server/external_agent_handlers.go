package server

import (
	"bufio"
	"bytes"
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

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
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
//
//	This prevents frontend manipulation of which Wolf client gets joined to the lobby
func (apiServer *HelixAPIServer) autoJoinWolfLobby(ctx context.Context, helixSessionID string, lobbyID string, lobbyPIN string) error {
	// Look up session to get Wolf instance ID
	session, err := apiServer.Store.GetSession(ctx, helixSessionID)
	if err != nil {
		return fmt.Errorf("failed to get session for Wolf instance lookup: %w", err)
	}

	wolfInstanceID := session.WolfInstanceID
	if wolfInstanceID == "" {
		return fmt.Errorf("session %s has no Wolf instance ID - session may be corrupted", helixSessionID)
	}

	// Get Wolf client for this session's Wolf instance
	type WolfClientForSessionProvider interface {
		GetWolfClientForSession(wolfInstanceID string) external_agent.WolfClientInterface
	}
	provider, ok := apiServer.externalAgentExecutor.(WolfClientForSessionProvider)
	if !ok {
		return fmt.Errorf("Wolf executor does not provide Wolf client")
	}
	wolfClient := provider.GetWolfClientForSession(wolfInstanceID)

	// Get Wolf apps using the existing ListApps method
	apps, err := wolfClient.ListApps(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Wolf apps: %w", err)
	}

	// Find placeholder app - prefer "Select Agent" (Wolf-UI with real Wayland compositor)
	// The "Blank" test pattern causes NVENC buffer registration failures on second session
	// because shared lobby buffers have stale registrations from previous encoder sessions.
	// See design/2025-12-04-websocket-mode-session-leak.md for details.
	var placeholderAppID string
	for _, app := range apps {
		if app.Title == "Select Agent" {
			placeholderAppID = app.ID
			break
		}
		if app.Title == "Blank" && placeholderAppID == "" {
			placeholderAppID = app.ID
		}
	}
	if placeholderAppID == "" {
		return fmt.Errorf("placeholder app (Blank or Select Agent) not found in apps list")
	}

	log.Info().
		Str("lobby_id", lobbyID).
		Str("placeholder_app_id", placeholderAppID).
		Msg("[AUTO-JOIN] Found placeholder app, querying sessions to find client")

	// Query Wolf sessions to find the Wolf UI client via interface
	sessions, err := wolfClient.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to query Wolf sessions: %w", err)
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

	for _, session := range sessions {
		if session.AppID == placeholderAppID {
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
			Int("total_sessions", len(sessions)).
			Msg("[AUTO-JOIN] No Wolf UI session found - client may not have connected yet")
		return fmt.Errorf("Wolf UI session not found - client may not have connected yet")
	}

	// DEFENSIVE CHECK: Verify Wolf-UI session still exists (not stopped/orphaned)
	// Re-query Wolf sessions to ensure session wasn't stopped since we last checked
	freshSessions, err := wolfClient.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to re-query Wolf sessions: %w", err)
	}

	// Verify the Wolf session we're trying to use still exists
	sessionExists := false
	for _, session := range freshSessions {
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
		Msg("[AUTO-JOIN] JoinLobby API called, waiting for pipeline to stabilize...")

	// RACE CONDITION FIX: Wait for GStreamer interpipe switch to complete
	// The JoinLobby triggers an interpipe producer switch, but the consumer pipeline
	// needs time for the switch to propagate and new frames to start flowing.
	// On low-latency connections (localhost), the frontend can receive the success response
	// and expect frames before the pipeline switch is complete.
	//
	// Poll Wolf sessions to verify the session has switched to the lobby's producer
	// by checking that the session's app_id has changed from the test pattern app
	// to the lobby's app.
	maxWait := 2 * time.Second
	pollInterval := 100 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		// Query Wolf sessions to check if the switch has completed
		sessions, err := wolfClient.ListSessions(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("[AUTO-JOIN] Failed to poll sessions, continuing anyway")
			break
		}

		// Find our session and check if it's now connected to the lobby
		for _, session := range sessions {
			if session.ClientID == moonlightSessionID {
				// Check if the session is no longer on the test pattern app (Wolf UI / Blank)
				if session.AppID != placeholderAppID {
					log.Info().
						Str("moonlight_session_id", moonlightSessionID).
						Str("new_app_id", session.AppID).
						Str("old_app_id", placeholderAppID).
						Msg("[AUTO-JOIN] ✅ Session switched to lobby producer, pipeline ready")
					return nil
				}
			}
		}

		time.Sleep(pollInterval)
	}

	// If we get here, the switch might not have completed, but continue anyway
	log.Warn().
		Str("moonlight_session_id", moonlightSessionID).
		Str("lobby_id", lobbyID).
		Dur("waited", maxWait).
		Msg("[AUTO-JOIN] ⚠️ Timed out waiting for producer switch, continuing anyway")

	return nil
}
