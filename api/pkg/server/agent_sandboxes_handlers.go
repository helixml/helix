package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// AgentSandboxesDebugResponse combines data from multiple sandbox endpoints
// for comprehensive debugging of the agent streaming infrastructure
type AgentSandboxesDebugResponse struct {
	Message       string                    `json:"message"`
	Sandboxes     []SandboxInstanceInfo     `json:"sandboxes,omitempty"`
	GPUs          []hydra.GPUInfo           `json:"gpus,omitempty"`
	DevContainers []DevContainerWithClients `json:"dev_containers,omitempty"`
}

// SandboxInstanceInfo represents a running sandbox instance
type SandboxInstanceInfo struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Status      string `json:"status"`
	ContainerID string `json:"container_id,omitempty"`
}

// DevContainerWithClients extends DevContainerResponse with connected clients and video stats
type DevContainerWithClients struct {
	hydra.DevContainerResponse
	SandboxID        string               `json:"sandbox_id"`
	Clients          []ClientInfo         `json:"clients,omitempty"`
	VideoStats       *VideoStreamingStats `json:"video_stats,omitempty"`
	SessionName      string               `json:"session_name,omitempty"`
	SessionAge       string               `json:"session_age,omitempty"`
	OwnerName        string               `json:"owner_name,omitempty"`
	OrganizationName string               `json:"organization_name,omitempty"`
	ProjectName      string               `json:"project_name,omitempty"`
	ProjectID        string               `json:"project_id,omitempty"`
	OrganizationID   string               `json:"organization_id,omitempty"`
	TaskNumber       int                  `json:"task_number,omitempty"`
	TaskName         string               `json:"task_name,omitempty"`
	TaskPrompt       string               `json:"task_prompt,omitempty"` // First ~80 chars of original prompt
	TaskID           string               `json:"task_id,omitempty"`
}

// VideoStreamingStats contains video streaming buffer statistics
type VideoStreamingStats struct {
	ClientCount    int                 `json:"client_count"`
	FramesReceived uint64              `json:"frames_received"`
	GOPBufferSize  int                 `json:"gop_buffer_size"`
	ClientBuffers  []ClientBufferStats `json:"client_buffers,omitempty"`
}

// ClientBufferStats contains per-client buffer statistics
type ClientBufferStats struct {
	ClientID   uint64 `json:"client_id"`
	BufferUsed int    `json:"buffer_used"`
	BufferSize int    `json:"buffer_size"`
	BufferPct  int    `json:"buffer_pct"`
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
		// Skip non-online sandboxes (status is "online", "offline", or "degraded")
		if sb.Status != "online" {
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

				// Query desktop server for connected clients and video stats (only for running containers)
				if dc.Status == hydra.DevContainerStatusRunning {
					clients := apiServer.queryDesktopClients(ctx, hydraClient, dc.SessionID)
					dcWithClients.Clients = clients

					videoStats := apiServer.queryVideoStats(ctx, hydraClient, dc.SessionID)
					dcWithClients.VideoStats = videoStats
				}

				allDevContainers = append(allDevContainers, dcWithClients)
			}
		}
	}

	// Enrich containers with session details (name, age, owner) and spec task info
	for i := range allDevContainers {
		dc := &allDevContainers[i]
		if dc.SessionID == "" {
			continue
		}
		session, err := apiServer.Store.GetSession(ctx, dc.SessionID)
		if err != nil {
			continue
		}
		dc.SessionName = session.Name
		dc.SessionAge = formatDuration(time.Since(session.Created))

		if session.Owner != "" {
			user, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: session.Owner})
			if err == nil && user != nil {
				if user.FullName != "" {
					dc.OwnerName = user.FullName
				} else {
					dc.OwnerName = user.Username
				}
			}
		}

		// Look up org and project names
		if session.OrganizationID != "" {
			dc.OrganizationID = session.OrganizationID
			org, err := apiServer.Store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: session.OrganizationID})
			if err == nil && org != nil {
				if org.DisplayName != "" {
					dc.OrganizationName = org.DisplayName
				} else {
					dc.OrganizationName = org.Name
				}
			}
		}
		if session.ProjectID != "" {
			dc.ProjectID = session.ProjectID
			project, err := apiServer.Store.GetProject(ctx, session.ProjectID)
			if err == nil && project != nil {
				dc.ProjectName = project.Name
			}
		}

		// Look up spec task: try work session first, then planning session fallback
		specTask := apiServer.findSpecTaskForSession(ctx, dc.SessionID)
		if specTask != nil {
			dc.TaskID = specTask.ID
			dc.TaskNumber = specTask.TaskNumber
			dc.TaskName = specTask.Name
			if specTask.OriginalPrompt != "" {
				prompt := specTask.OriginalPrompt
				if len(prompt) > 80 {
					prompt = prompt[:80] + "..."
				}
				dc.TaskPrompt = prompt
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

// queryVideoStats queries the desktop server's /video/stats endpoint via Hydra proxy
func (apiServer *HelixAPIServer) queryVideoStats(ctx context.Context, hydraClient *hydra.RevDialClient, sessionID string) *VideoStreamingStats {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := hydraClient.GetDevContainerVideoStats(ctxTimeout, sessionID)
	if err != nil {
		// Not an error - container may not have video stats
		return nil
	}

	// Aggregate stats from all sources (usually just one)
	stats := &VideoStreamingStats{}
	for _, src := range resp.Sources {
		stats.ClientCount += src.ClientCount
		stats.FramesReceived += src.FramesReceived
		stats.GOPBufferSize = src.GOPBufferSize

		// Convert client buffers
		for _, cb := range src.Clients {
			stats.ClientBuffers = append(stats.ClientBuffers, ClientBufferStats{
				ClientID:   cb.ClientID,
				BufferUsed: cb.BufferUsed,
				BufferSize: cb.BufferSize,
				BufferPct:  cb.BufferPct,
			})
		}
	}

	return stats
}

// findSpecTaskForSession resolves a helix session ID to its parent SpecTask.
// Tries the work session path first (implementation sessions), then falls back
// to checking PlanningSessionID (planning/spec-generation sessions).
func (apiServer *HelixAPIServer) findSpecTaskForSession(ctx context.Context, sessionID string) *types.SpecTask {
	// Path 1: session → work session → spec task (covers implementation sessions)
	workSession, err := apiServer.Store.GetSpecTaskWorkSessionByHelixSession(ctx, sessionID)
	if err == nil && workSession != nil {
		specTask, err := apiServer.Store.GetSpecTask(ctx, workSession.SpecTaskID)
		if err == nil {
			return specTask
		}
	}

	// Path 2: session is the planning session on the spec task itself
	tasks, err := apiServer.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		PlanningSessionID: sessionID,
		Limit:             1,
	})
	if err == nil && len(tasks) > 0 {
		return tasks[0]
	}

	return nil
}

// formatDuration formats a time.Duration into a human-readable age string like "2h 15m" or "3d 4h".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	if hours < 24 {
		mins := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := hours / 24
	remainHours := hours % 24
	return fmt.Sprintf("%dd %dh", days, remainHours)
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

	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("session not found: %v", err), http.StatusNotFound)
		return
	}

	// Check session access
	err = apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
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
