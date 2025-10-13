package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/external-agent"
)

// AgentSandboxesDebugResponse combines data from multiple Wolf endpoints
// for comprehensive debugging of the agent streaming infrastructure
type AgentSandboxesDebugResponse struct {
	Memory   *WolfSystemMemory `json:"memory"`
	Lobbies  []WolfLobbyInfo   `json:"lobbies"`
	Sessions []WolfSessionInfo `json:"sessions"`
}

// WolfSystemMemory represents Wolf's system memory usage
type WolfSystemMemory struct {
	Success             bool                   `json:"success"`
	ProcessRSSBytes     string                 `json:"process_rss_bytes"`
	GStreamerBufferBytes string                `json:"gstreamer_buffer_bytes"`
	TotalMemoryBytes    string                 `json:"total_memory_bytes"`
	Lobbies             []WolfLobbyMemory      `json:"lobbies"`
	Clients             []WolfClientConnection `json:"clients"`
}

// WolfLobbyMemory represents per-lobby memory usage
type WolfLobbyMemory struct {
	LobbyID     string `json:"lobby_id"`
	LobbyName   string `json:"lobby_name"`
	Resolution  string `json:"resolution"`
	ClientCount string `json:"client_count"`
	MemoryBytes string `json:"memory_bytes"`
}

// WolfClientConnection represents individual client connections for leak detection
type WolfClientConnection struct {
	SessionID   string  `json:"session_id"`
	ClientIP    string  `json:"client_ip"`
	Resolution  string  `json:"resolution"`
	LobbyID     *string `json:"lobby_id"` // null if orphaned
	MemoryBytes string  `json:"memory_bytes"`
}

// WolfLobbyInfo represents a Wolf lobby
type WolfLobbyInfo struct {
	ID                     string  `json:"id"`
	Name                   string  `json:"name"`
	StartedByProfileID     string  `json:"started_by_profile_id"`
	MultiUser              bool    `json:"multi_user"`
	StopWhenEveryoneLeaves bool    `json:"stop_when_everyone_leaves"`
	PIN                    []int16 `json:"pin,omitempty"`
}

// WolfSessionInfo represents a Wolf streaming session
// Note: Wolf returns flat structure, we transform it for frontend
type WolfSessionInfo struct {
	SessionID       string `json:"session_id"` // Exposed as session_id for frontend
	ClientIP        string `json:"client_ip"`
	AppID           string `json:"app_id"`
	VideoWidth      int    `json:"-"` // Internal field from Wolf
	VideoHeight     int    `json:"-"` // Internal field from Wolf
	VideoRefreshRate int   `json:"-"` // Internal field from Wolf
	DisplayMode     struct {
		Width         int  `json:"width"`
		Height        int  `json:"height"`
		RefreshRate   int  `json:"refresh_rate"`
		HEVCSupported bool `json:"hevc_supported"`
		AV1Supported  bool `json:"av1_supported"`
	} `json:"display_mode"`
}

// wolfSessionRaw matches Wolf's actual API response format
type wolfSessionRaw struct {
	ClientID        string `json:"client_id"`
	ClientIP        string `json:"client_ip"`
	AppID           string `json:"app_id"`
	VideoWidth      int    `json:"video_width"`
	VideoHeight     int    `json:"video_height"`
	VideoRefreshRate int   `json:"video_refresh_rate"`
}

// @Summary Get Wolf debugging data
// @Description Retrieves combined debug data from Wolf (memory, lobbies, sessions) for the Agent Sandboxes dashboard
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} AgentSandboxesDebugResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/admin/agent-sandboxes/debug [get]
func (apiServer *HelixAPIServer) getAgentSandboxesDebug(rw http.ResponseWriter, req *http.Request) {
	// Get Wolf client from external agent executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(rw, "Wolf executor not available", http.StatusInternalServerError)
		return
	}
	wolfClient := wolfExecutor.GetWolfClient()

	ctx := req.Context()
	response := &AgentSandboxesDebugResponse{}

	// Fetch system memory data
	memoryData, err := fetchWolfMemoryData(ctx, wolfClient)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to fetch Wolf memory data: %v", err), http.StatusInternalServerError)
		return
	}
	response.Memory = memoryData

	// Fetch lobbies
	lobbiesData, err := fetchWolfLobbies(ctx, wolfClient)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to fetch Wolf lobbies: %v", err), http.StatusInternalServerError)
		return
	}
	response.Lobbies = lobbiesData

	// Fetch sessions
	sessionsData, err := fetchWolfSessions(ctx, wolfClient)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to fetch Wolf sessions: %v", err), http.StatusInternalServerError)
		return
	}
	response.Sessions = sessionsData

	// Return combined data
	writeResponse(rw, response, http.StatusOK)
}

// fetchWolfMemoryData retrieves memory usage data from Wolf
func fetchWolfMemoryData(ctx context.Context, wolfClient interface{}) (*WolfSystemMemory, error) {
	// Type assert to get the concrete Wolf client type
	type WolfClientGetter interface {
		Get(ctx context.Context, path string) (*http.Response, error)
	}

	client, ok := wolfClient.(WolfClientGetter)
	if !ok {
		return nil, fmt.Errorf("wolf client does not implement Get method")
	}

	resp, err := client.Get(ctx, "/api/v1/system/memory")
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf memory endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf memory endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var memoryData WolfSystemMemory
	if err := json.NewDecoder(resp.Body).Decode(&memoryData); err != nil {
		return nil, fmt.Errorf("failed to decode Wolf memory response: %w", err)
	}

	return &memoryData, nil
}

// fetchWolfLobbies retrieves all lobbies from Wolf
func fetchWolfLobbies(ctx context.Context, wolfClient interface{}) ([]WolfLobbyInfo, error) {
	type WolfClientGetter interface {
		Get(ctx context.Context, path string) (*http.Response, error)
	}

	client, ok := wolfClient.(WolfClientGetter)
	if !ok {
		return nil, fmt.Errorf("wolf client does not implement Get method")
	}

	resp, err := client.Get(ctx, "/api/v1/lobbies")
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf lobbies endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf lobbies endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var lobbiesResponse struct {
		Success bool            `json:"success"`
		Lobbies []WolfLobbyInfo `json:"lobbies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lobbiesResponse); err != nil {
		return nil, fmt.Errorf("failed to decode Wolf lobbies response: %w", err)
	}

	if !lobbiesResponse.Success {
		return nil, fmt.Errorf("Wolf lobbies endpoint returned success=false")
	}

	return lobbiesResponse.Lobbies, nil
}

// fetchWolfSessions retrieves all streaming sessions from Wolf
func fetchWolfSessions(ctx context.Context, wolfClient interface{}) ([]WolfSessionInfo, error) {
	type WolfClientGetter interface {
		Get(ctx context.Context, path string) (*http.Response, error)
	}

	client, ok := wolfClient.(WolfClientGetter)
	if !ok {
		return nil, fmt.Errorf("wolf client does not implement Get method")
	}

	resp, err := client.Get(ctx, "/api/v1/sessions")
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf sessions endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf sessions endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var sessionsResponse struct {
		Success  bool             `json:"success"`
		Sessions []wolfSessionRaw `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessionsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode Wolf sessions response: %w", err)
	}

	if !sessionsResponse.Success {
		return nil, fmt.Errorf("Wolf sessions endpoint returned success=false")
	}

	// Transform Wolf's flat structure to our nested structure
	sessions := make([]WolfSessionInfo, len(sessionsResponse.Sessions))
	for i, raw := range sessionsResponse.Sessions {
		sessions[i] = WolfSessionInfo{
			SessionID: raw.ClientID,
			ClientIP:  raw.ClientIP,
			AppID:     raw.AppID,
			DisplayMode: struct {
				Width         int  `json:"width"`
				Height        int  `json:"height"`
				RefreshRate   int  `json:"refresh_rate"`
				HEVCSupported bool `json:"hevc_supported"`
				AV1Supported  bool `json:"av1_supported"`
			}{
				Width:       raw.VideoWidth,
				Height:      raw.VideoHeight,
				RefreshRate: raw.VideoRefreshRate,
				// TODO: Get codec support from Wolf (not currently in sessions endpoint)
				HEVCSupported: false,
				AV1Supported:  false,
			},
		}
	}

	return sessions, nil
}

// @Summary Get Wolf real-time events (SSE)
// @Description Proxies Server-Sent Events from Wolf for real-time monitoring
// @Tags Admin
// @Accept json
// @Produce text/event-stream
// @Success 200 {string} string "event: message"
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/admin/agent-sandboxes/events [get]
func (apiServer *HelixAPIServer) getAgentSandboxesEvents(rw http.ResponseWriter, req *http.Request) {
	// Get Wolf client from external agent executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(rw, "Wolf executor not available", http.StatusInternalServerError)
		return
	}
	wolfClient := wolfExecutor.GetWolfClient()

	ctx := req.Context()

	// Connect to Wolf's SSE endpoint
	resp, err := wolfClient.Get(ctx, "/api/v1/events")
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to connect to Wolf events: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		http.Error(rw, fmt.Sprintf("Wolf events endpoint returned status %d: %s", resp.StatusCode, string(body)), http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.WriteHeader(http.StatusOK)

	// Flush headers
	if flusher, ok := rw.(http.Flusher); ok {
		flusher.Flush()
	}

	// Stream events from Wolf to client
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			return
		default:
			n, err := resp.Body.Read(buf)
			if err != nil {
				if err != io.EOF {
					// Log error but don't send to client (SSE stream already started)
					fmt.Printf("Error reading Wolf events: %v\n", err)
				}
				return
			}

			// Write events to client
			_, writeErr := rw.Write(buf[:n])
			if writeErr != nil {
				// Client disconnected
				return
			}

			// Flush to ensure events are sent immediately
			if flusher, ok := rw.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}
