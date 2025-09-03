package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// createExternalAgent handles POST /api/v1/external-agents
func (apiServer *HelixAPIServer) createExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	var agent types.ZedAgent
	err := json.NewDecoder(req.Body).Decode(&agent)
	if err != nil {
		system.Error(res, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %s", err.Error()))
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
		system.Error(res, http.StatusBadRequest, "input is required")
		return
	}

	// Create external agent executor if not already available
	if apiServer.externalAgentExecutor == nil {
		apiServer.externalAgentExecutor = external_agent.NewExecutor(apiServer.Cfg, apiServer.pubsub)
	}

	// Generate auth token for WebSocket connection
	token, err := apiServer.generateExternalAgentToken(agent.SessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to generate external agent token")
		system.Error(res, http.StatusInternalServerError, fmt.Sprintf("failed to generate auth token: %s", err.Error()))
		return
	}

	// Start the external agent
	response, err := apiServer.externalAgentExecutor.StartZedAgent(req.Context(), &agent)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to start external agent")
		system.Error(res, http.StatusInternalServerError, fmt.Sprintf("failed to start external agent: %s", err.Error()))
		return
	}

	// Add WebSocket connection info to response
	response.WebSocketURL = fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, agent.SessionID)
	response.AuthToken = token

	system.RespondJSON(res, response)
}

// getExternalAgent handles GET /api/v1/external-agents/{sessionID}
func (apiServer *HelixAPIServer) getExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		system.Error(res, http.StatusBadRequest, "session ID is required")
		return
	}

	if apiServer.externalAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "external agent executor not available")
		return
	}

	session, exists := apiServer.externalAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
		return
	}

	response := types.ZedAgentResponse{
		SessionID: session.SessionID,
		RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		Status:    "running",
		PID:       session.PID,
	}

	system.RespondJSON(res, response)
}

// listExternalAgents handles GET /api/v1/external-agents
func (apiServer *HelixAPIServer) listExternalAgents(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	if apiServer.externalAgentExecutor == nil {
		system.RespondJSON(res, []types.ZedAgentResponse{})
		return
	}

	sessions := apiServer.externalAgentExecutor.ListSessions()
	responses := make([]types.ZedAgentResponse, len(sessions))

	for i, session := range sessions {
		responses[i] = types.ZedAgentResponse{
			SessionID: session.SessionID,
			RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
			Status:    "running",
			PID:       session.PID,
		}
	}

	system.RespondJSON(res, responses)
}

// deleteExternalAgent handles DELETE /api/v1/external-agents/{sessionID}
func (apiServer *HelixAPIServer) deleteExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		system.Error(res, http.StatusBadRequest, "session ID is required")
		return
	}

	if apiServer.externalAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "external agent executor not available")
		return
	}

	err := apiServer.externalAgentExecutor.StopZedAgent(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to stop external agent")
		system.Error(res, http.StatusInternalServerError, fmt.Sprintf("failed to stop external agent: %s", err.Error()))
		return
	}

	response := map[string]string{
		"message":    "External agent stopped successfully",
		"session_id": sessionID,
	}

	system.RespondJSON(res, response)
}

// getExternalAgentRDP handles GET /api/v1/external-agents/{sessionID}/rdp
func (apiServer *HelixAPIServer) getExternalAgentRDP(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		system.Error(res, http.StatusBadRequest, "session ID is required")
		return
	}

	if apiServer.externalAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "external agent executor not available")
		return
	}

	session, exists := apiServer.externalAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
		return
	}

	// Return RDP connection details with WebSocket info
	rdpInfo := map[string]interface{}{
		"session_id":          session.SessionID,
		"rdp_url":             fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		"rdp_port":            session.RDPPort,
		"display":             fmt.Sprintf(":%d", session.DisplayNum),
		"status":              "running",
		"username":            "zed", // Default RDP username
		"host":                "localhost",
		"websocket_url":       fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, session.SessionID),
		"websocket_connected": apiServer.isExternalAgentConnected(session.SessionID),
	}

	system.RespondJSON(res, rdpInfo)
}

// updateExternalAgent handles PUT /api/v1/external-agents/{sessionID}
func (apiServer *HelixAPIServer) updateExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		system.Error(res, http.StatusBadRequest, "session ID is required")
		return
	}

	var updateData map[string]interface{}
	err := json.NewDecoder(req.Body).Decode(&updateData)
	if err != nil {
		system.Error(res, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %s", err.Error()))
		return
	}

	if apiServer.externalAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "external agent executor not available")
		return
	}

	session, exists := apiServer.externalAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
		return
	}

	// For now, just update the last access time
	// In a full implementation, you might want to support updating other session properties
	session, _ = apiServer.externalAgentExecutor.GetSession(sessionID)

	response := types.ZedAgentResponse{
		SessionID: session.SessionID,
		RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		Status:    "running",
		PID:       session.PID,
	}

	system.RespondJSON(res, response)
}

// getExternalAgentStats handles GET /api/v1/external-agents/{sessionID}/stats
func (apiServer *HelixAPIServer) getExternalAgentStats(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		system.Error(res, http.StatusBadRequest, "session ID is required")
		return
	}

	if apiServer.externalAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "external agent executor not available")
		return
	}

	session, exists := apiServer.externalAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
		return
	}

	stats := map[string]interface{}{
		"session_id":    session.SessionID,
		"pid":           session.PID,
		"start_time":    session.StartTime,
		"last_access":   session.LastAccess,
		"uptime":        session.LastAccess.Sub(session.StartTime).Seconds(),
		"workspace_dir": session.WorkspaceDir,
		"display_num":   session.DisplayNum,
		"rdp_port":      session.RDPPort,
		"status":        "running",
	}

	system.RespondJSON(res, stats)
}

// getExternalAgentLogs handles GET /api/v1/external-agents/{sessionID}/logs
func (apiServer *HelixAPIServer) getExternalAgentLogs(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		system.Error(res, http.StatusBadRequest, "session ID is required")
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
		system.Error(res, http.StatusNotFound, "external agent executor not available")
		return
	}

	_, exists := apiServer.externalAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
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
		"timestamp": system.Now(),
	}

	system.RespondJSON(res, logs)
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
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	connections := apiServer.externalAgentWSManager.listConnections()
	system.RespondJSON(res, connections)
}

// sendCommandToExternalAgentHandler allows manual command sending for testing
func (apiServer *HelixAPIServer) sendCommandToExternalAgentHandler(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		system.Error(res, http.StatusBadRequest, "session ID is required")
		return
	}

	var command types.ExternalAgentCommand
	err := json.NewDecoder(req.Body).Decode(&command)
	if err != nil {
		system.Error(res, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %s", err.Error()))
		return
	}

	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to send command to external agent")
		system.Error(res, http.StatusInternalServerError, fmt.Sprintf("failed to send command: %s", err.Error()))
		return
	}

	response := map[string]string{
		"message":    "Command sent successfully",
		"session_id": sessionID,
	}

	system.RespondJSON(res, response)
}
