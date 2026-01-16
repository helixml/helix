package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/hydra"
)

// AgentSandboxesDebugResponse combines data from multiple sandbox endpoints
// for comprehensive debugging of the agent streaming infrastructure
type AgentSandboxesDebugResponse struct {
	Message       string                       `json:"message"`
	Sandboxes     []SandboxInstanceInfo        `json:"sandboxes,omitempty"`
	GPUs          []hydra.GPUInfo              `json:"gpus,omitempty"`
	DevContainers []DevContainerWithClients    `json:"dev_containers,omitempty"`
}

// SandboxInstanceInfo represents a running sandbox instance
type SandboxInstanceInfo struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`
	ContainerID string `json:"container_id,omitempty"`
}

// DevContainerWithClients extends DevContainerResponse with connected clients
type DevContainerWithClients struct {
	hydra.DevContainerResponse
	SandboxID string       `json:"sandbox_id"`
	Clients   []ClientInfo `json:"clients,omitempty"`
}

// ClientInfo represents a connected WebSocket client
type ClientInfo struct {
	ID        uint32 `json:"id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Color     string `json:"color"`
	LastX     int32  `json:"last_x"`
	LastY     int32  `json:"last_y"`
	LastSeen  string `json:"last_seen"`
}

// SessionSandboxStateResponse represents the sandbox state for a specific external agent session
type SessionSandboxStateResponse struct {
	SessionID   string `json:"session_id"`
	State       string `json:"state"` // "absent", "running", "starting"
	ContainerID string `json:"container_id,omitempty"`
}

// @Summary Get sandbox debugging data
// @Description Retrieves debug data for agent sandboxes (Hydra-based) including GPU stats, dev containers, and connected clients
// @Tags Admin
// @Accept json
// @Produce json
// @Param sandbox_id query string false "Sandbox instance ID to query"
// @Success 200 {object} AgentSandboxesDebugResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/admin/agent-sandboxes/debug [get]
func (apiServer *HelixAPIServer) getAgentSandboxesDebug(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// List all registered sandbox instances from the store
	sandboxes, err := apiServer.Store.ListSandboxes(ctx)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to list sandboxes: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to response format
	sandboxInfos := make([]SandboxInstanceInfo, len(sandboxes))
	for i, sb := range sandboxes {
		sandboxInfos[i] = SandboxInstanceInfo{
			ID:     sb.ID,
			Status: sb.Status,
		}
	}

	// Aggregate GPU info and dev containers from all sandboxes
	var allGPUs []hydra.GPUInfo
	var allDevContainers []DevContainerWithClients

	for _, sb := range sandboxes {
		// Skip non-running sandboxes
		if sb.Status != "running" {
			continue
		}

		// Create Hydra client via RevDial
		hydraRunnerID := fmt.Sprintf("hydra-%s", sb.ID)
		hydraClient := hydra.NewRevDialClient(apiServer.connman, hydraRunnerID)

		// Query system stats for GPU info
		ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
		stats, err := hydraClient.GetSystemStats(ctxTimeout)
		cancel()
		if err == nil && stats != nil {
			// Add GPUs (avoid duplicates by checking first)
			for _, gpu := range stats.GPUs {
				// For now, just append (could add dedup logic if needed)
				allGPUs = append(allGPUs, gpu)
			}
		}

		// Query dev containers
		ctxTimeout, cancel = context.WithTimeout(ctx, 5*time.Second)
		containers, err := hydraClient.ListDevContainers(ctxTimeout)
		cancel()
		if err == nil && containers != nil {
			for _, dc := range containers.Containers {
				dcWithClients := DevContainerWithClients{
					DevContainerResponse: dc,
					SandboxID:            sb.ID,
				}

				// Query desktop server for connected clients (only for running containers)
				if dc.Status == hydra.DevContainerStatusRunning {
					clients := apiServer.queryDesktopClients(ctx, hydraClient, dc.SessionID)
					dcWithClients.Clients = clients
				}

				allDevContainers = append(allDevContainers, dcWithClients)
			}
		}
	}

	response := &AgentSandboxesDebugResponse{
		Message:       "Hydra-based sandbox infrastructure",
		Sandboxes:     sandboxInfos,
		GPUs:          allGPUs,
		DevContainers: allDevContainers,
	}

	writeResponse(rw, response, http.StatusOK)
}

// queryDesktopClients queries the desktop server's /clients endpoint via Hydra proxy
func (apiServer *HelixAPIServer) queryDesktopClients(ctx context.Context, hydraClient *hydra.RevDialClient, sessionID string) []ClientInfo {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := hydraClient.GetDevContainerClients(ctxTimeout, sessionID)
	if err != nil {
		// Not an error - container may not have users connected
		return nil
	}

	// Convert hydra.ConnectedClient to ClientInfo
	clients := make([]ClientInfo, len(resp.Clients))
	for i, c := range resp.Clients {
		clients[i] = ClientInfo{
			ID:        c.ID,
			UserID:    c.UserID,
			UserName:  c.UserName,
			AvatarURL: c.AvatarURL,
			Color:     c.Color,
			LastX:     c.LastX,
			LastY:     c.LastY,
			LastSeen:  c.LastSeen,
		}
	}
	return clients
}

// @Summary Get sandbox real-time events (SSE)
// @Description Streams Server-Sent Events for real-time sandbox monitoring
// @Tags Admin
// @Accept json
// @Produce text/event-stream
// @Success 200 {string} string "event: message"
// @Failure 401 {object} system.HTTPError
// @Failure 501 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/admin/agent-sandboxes/events [get]
func (apiServer *HelixAPIServer) getAgentSandboxesEvents(rw http.ResponseWriter, req *http.Request) {
	// SSE events not yet implemented for Hydra-based sandboxes
	http.Error(rw, "Real-time events not yet implemented", http.StatusNotImplemented)
}

// @Summary Get sandbox state for a session
// @Description Returns the current sandbox state for an external agent session (absent/running/starting)
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} SessionSandboxStateResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sessions/{id}/sandbox-state [get]
func (apiServer *HelixAPIServer) getSessionSandboxState(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get session ID from URL path using mux
	vars := mux.Vars(req)
	sessionID := vars["id"]

	// Check session access
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("session not found: %v", err), http.StatusNotFound)
		return
	}

	// Verify user has access to this session
	if session.Owner != user.ID && !user.Admin {
		http.Error(rw, "forbidden: you don't have access to this session", http.StatusForbidden)
		return
	}

	// Check if this is an external agent session
	if session.Metadata.AgentType != "zed_external" {
		http.Error(rw, "not an external agent session", http.StatusBadRequest)
		return
	}

	// Determine state based on session metadata
	state := "absent"
	containerID := ""

	// Check if session has an active sandbox (SandboxID is used as the sandbox identifier)
	if session.SandboxID != "" {
		// Session has a sandbox assigned - it's running
		state = "running"
	}

	response := SessionSandboxStateResponse{
		SessionID:   sessionID,
		State:       state,
		ContainerID: containerID,
	}

	writeResponse(rw, response, http.StatusOK)
}
