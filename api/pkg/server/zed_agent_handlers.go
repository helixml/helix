package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// createZedAgent handles POST /api/v1/zed-agents
func (apiServer *HelixAPIServer) createZedAgent(res http.ResponseWriter, req *http.Request) {
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

	// Create Zed agent executor if not already available
	if apiServer.zedAgentExecutor == nil {
		zedExecutor := external_agent.NewZedExecutor("/tmp/zed-workspaces")
		apiServer.zedAgentExecutor = external_agent.NewDirectExecutor(zedExecutor)
	}

	// Start the Zed agent
	response, err := apiServer.zedAgentExecutor.StartZedAgent(req.Context(), &agent)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to start Zed agent")
		http.Error(res, fmt.Sprintf("failed to start Zed agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// getZedAgent handles GET /api/v1/zed-agents/{sessionID}
func (apiServer *HelixAPIServer) getZedAgent(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		http.Error(res, "Zed agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.zedAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	response := types.ZedAgentResponse{
		SessionID: session.SessionID,
		RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		Status:    "running",
		PID:       session.PID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// listZedAgents handles GET /api/v1/zed-agents
func (apiServer *HelixAPIServer) listZedAgents(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	if apiServer.zedAgentExecutor == nil {
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode([]types.ZedAgentResponse{})
		return
	}

	sessions := apiServer.zedAgentExecutor.ListSessions()
	responses := make([]types.ZedAgentResponse, len(sessions))

	for i, session := range sessions {
		responses[i] = types.ZedAgentResponse{
			SessionID: session.SessionID,
			RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
			Status:    "running",
			PID:       session.PID,
		}
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(responses)
}

// deleteZedAgent handles DELETE /api/v1/zed-agents/{sessionID}
func (apiServer *HelixAPIServer) deleteZedAgent(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		http.Error(res, "Zed agent executor not available", http.StatusNotFound)
		return
	}

	err := apiServer.zedAgentExecutor.StopZedAgent(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to stop Zed agent")
		http.Error(res, fmt.Sprintf("failed to stop Zed agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"message":    "Zed agent stopped successfully",
		"session_id": sessionID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// getZedAgentRDP handles GET /api/v1/zed-agents/{sessionID}/rdp
func (apiServer *HelixAPIServer) getZedAgentRDP(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		http.Error(res, "Zed agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.zedAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	// Return RDP connection details
	rdpInfo := map[string]interface{}{
		"session_id": session.SessionID,
		"rdp_url":    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		"rdp_port":   session.RDPPort,
		"display":    fmt.Sprintf(":%d", session.DisplayNum),
		"status":     "running",
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(rdpInfo)
}

// updateZedAgent handles PUT /api/v1/zed-agents/{sessionID}
func (apiServer *HelixAPIServer) updateZedAgent(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		http.Error(res, "Zed agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.zedAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	// For now, just update the last access time
	// In a full implementation, you might want to support updating other session properties
	session, _ = apiServer.zedAgentExecutor.GetSession(sessionID)

	response := types.ZedAgentResponse{
		SessionID: session.SessionID,
		RDPURL:    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		Status:    "running",
		PID:       session.PID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// proxyZedAgentRDP handles WebSocket upgrade for RDP proxy
func (apiServer *HelixAPIServer) proxyZedAgentRDP(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		http.Error(res, "Zed agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.zedAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
		return
	}

	// For now, just redirect to the VNC/RDP port
	// In a full implementation, you would set up WebSocket proxying to the RDP/VNC server
	redirectURL := fmt.Sprintf("http://localhost:%d", session.RDPPort)
	http.Redirect(res, req, redirectURL, http.StatusTemporaryRedirect)
}

// getZedAgentStats handles GET /api/v1/zed-agents/{sessionID}/stats
func (apiServer *HelixAPIServer) getZedAgentStats(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		http.Error(res, "Zed agent executor not available", http.StatusNotFound)
		return
	}

	session, err := apiServer.zedAgentExecutor.GetSession(sessionID)
	if err != nil {
		http.Error(res, fmt.Sprintf("session %s not found", sessionID), http.StatusNotFound)
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

		"status": "running",
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(stats)
}

// getZedAgentLogs handles GET /api/v1/zed-agents/{sessionID}/logs
func (apiServer *HelixAPIServer) getZedAgentLogs(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		http.Error(res, "Zed agent executor not available", http.StatusNotFound)
		return
	}

	_, err := apiServer.zedAgentExecutor.GetSession(sessionID)
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
			"[INFO] VNC server listening on port 5901",
			"[INFO] RDP bridge active",
			"[DEBUG] Session active and responding",
		},
		"timestamp": time.Now(),
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(logs)
}
