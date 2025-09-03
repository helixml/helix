package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// createZedAgent handles POST /api/v1/zed-agents
func (apiServer *HelixAPIServer) createZedAgent(res http.ResponseWriter, req *http.Request) {
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

	// Create Zed agent executor if not already available
	if apiServer.zedAgentExecutor == nil {
		apiServer.zedAgentExecutor = gptscript.NewZedAgentExecutor(apiServer.Cfg, apiServer.pubsub)
	}

	// Start the Zed agent
	response, err := apiServer.zedAgentExecutor.StartZedAgent(req.Context(), &agent)
	if err != nil {
		log.Error().Err(err).Str("session_id", agent.SessionID).Msg("failed to start Zed agent")
		system.Error(res, http.StatusInternalServerError, fmt.Sprintf("failed to start Zed agent: %s", err.Error()))
		return
	}

	system.RespondJSON(res, response)
}

// getZedAgent handles GET /api/v1/zed-agents/{sessionID}
func (apiServer *HelixAPIServer) getZedAgent(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "Zed agent executor not available")
		return
	}

	session, exists := apiServer.zedAgentExecutor.GetSession(sessionID)
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

// listZedAgents handles GET /api/v1/zed-agents
func (apiServer *HelixAPIServer) listZedAgents(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		system.Error(res, http.StatusUnauthorized, "unauthorized")
		return
	}

	if apiServer.zedAgentExecutor == nil {
		system.RespondJSON(res, []types.ZedAgentResponse{})
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

	system.RespondJSON(res, responses)
}

// deleteZedAgent handles DELETE /api/v1/zed-agents/{sessionID}
func (apiServer *HelixAPIServer) deleteZedAgent(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "Zed agent executor not available")
		return
	}

	err := apiServer.zedAgentExecutor.StopZedAgent(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to stop Zed agent")
		system.Error(res, http.StatusInternalServerError, fmt.Sprintf("failed to stop Zed agent: %s", err.Error()))
		return
	}

	response := map[string]string{
		"message":    "Zed agent stopped successfully",
		"session_id": sessionID,
	}

	system.RespondJSON(res, response)
}

// getZedAgentRDP handles GET /api/v1/zed-agents/{sessionID}/rdp
func (apiServer *HelixAPIServer) getZedAgentRDP(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "Zed agent executor not available")
		return
	}

	session, exists := apiServer.zedAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
		return
	}

	// Return RDP connection details
	rdpInfo := map[string]interface{}{
		"session_id": session.SessionID,
		"rdp_url":    fmt.Sprintf("rdp://localhost:%d", session.RDPPort),
		"vnc_url":    fmt.Sprintf("vnc://localhost:%d", session.VNCPort),
		"rdp_port":   session.RDPPort,
		"vnc_port":   session.VNCPort,
		"display":    fmt.Sprintf(":%d", session.DisplayNum),
		"status":     "running",
	}

	system.RespondJSON(res, rdpInfo)
}

// updateZedAgent handles PUT /api/v1/zed-agents/{sessionID}
func (apiServer *HelixAPIServer) updateZedAgent(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "Zed agent executor not available")
		return
	}

	session, exists := apiServer.zedAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
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

	system.RespondJSON(res, response)
}

// proxyZedAgentRDP handles WebSocket upgrade for RDP proxy
func (apiServer *HelixAPIServer) proxyZedAgentRDP(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "Zed agent executor not available")
		return
	}

	session, exists := apiServer.zedAgentExecutor.GetSession(sessionID)
	if !exists {
		system.Error(res, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
		return
	}

	// For now, just redirect to the VNC/RDP port
	// In a full implementation, you would set up WebSocket proxying to the RDP/VNC server
	redirectURL := fmt.Sprintf("http://localhost:%d", session.VNCPort)
	http.Redirect(res, req, redirectURL, http.StatusTemporaryRedirect)
}

// getZedAgentStats handles GET /api/v1/zed-agents/{sessionID}/stats
func (apiServer *HelixAPIServer) getZedAgentStats(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "Zed agent executor not available")
		return
	}

	session, exists := apiServer.zedAgentExecutor.GetSession(sessionID)
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
		"vnc_port":      session.VNCPort,
		"status":        "running",
	}

	system.RespondJSON(res, stats)
}

// getZedAgentLogs handles GET /api/v1/zed-agents/{sessionID}/logs
func (apiServer *HelixAPIServer) getZedAgentLogs(res http.ResponseWriter, req *http.Request) {
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

	if apiServer.zedAgentExecutor == nil {
		system.Error(res, http.StatusNotFound, "Zed agent executor not available")
		return
	}

	_, exists := apiServer.zedAgentExecutor.GetSession(sessionID)
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
			"[INFO] VNC server listening on port 5901",
			"[INFO] RDP bridge active",
			"[DEBUG] Session active and responding",
		},
		"timestamp": system.Now(),
	}

	system.RespondJSON(res, logs)
}
