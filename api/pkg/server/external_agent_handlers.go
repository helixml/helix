package server

import (
	"encoding/json"
	"fmt"
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
		http.Error(res, "external agent executor not available", http.StatusServiceUnavailable)
		return
	}

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

	// Add WebSocket connection info to response
	response.WebSocketURL = fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, agent.SessionID)
	response.AuthToken = token

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
		http.Error(res, "external agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
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

	session, err := apiServer.externalAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	// Return RDP connection details with WebSocket info
	rdpInfo := map[string]interface{}{
		"session_id":          session.SessionID,
		"rdp_url":             fmt.Sprintf("rdp://localhost:%d", 8080),
		"rdp_port":            8080,
		"rdp_password":        session.RDPPassword, // Secure random password
		"display":             fmt.Sprintf(":%d", 1),
		"status":              "running",
		"username":            "zed", // Default RDP username
		"host":                "localhost",
		"proxy_url":           fmt.Sprintf("wss://%s/api/v1/external-agents/%s/rdp/proxy", req.Host, session.SessionID),
		"websocket_url":       fmt.Sprintf("wss://%s/api/v1/external-agents/sync?session_id=%s", req.Host, session.SessionID),
		"websocket_connected": apiServer.isExternalAgentConnected(session.SessionID),
	}

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

	connections := apiServer.externalAgentWSManager.listConnections()
	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(connections)
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
