package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/wolf"
)

// AgentSandboxesDebugResponse combines data from multiple Wolf endpoints
// for comprehensive debugging of the agent streaming infrastructure
type AgentSandboxesDebugResponse struct {
	Memory           *WolfSystemMemory      `json:"memory"`
	Apps             []WolfAppInfo          `json:"apps,omitempty"`    // Apps mode
	Lobbies          []WolfLobbyInfo        `json:"lobbies,omitempty"` // Lobbies mode
	Sessions         []WolfSessionInfo      `json:"sessions"`
	MoonlightClients []MoonlightClientInfo  `json:"moonlight_clients"` // NEW: moonlight-web client connections
	WolfMode         string                 `json:"wolf_mode"`         // Current Wolf mode ("apps" or "lobbies")
}

// MoonlightClientInfo represents a moonlight-web client connection
type MoonlightClientInfo struct {
	SessionID     string `json:"session_id"`
	Mode          string `json:"mode"`           // "create", "keepalive", "join"
	HasWebsocket  bool   `json:"has_websocket"`  // Is a WebRTC client currently connected?
}

// WolfAppInfo represents a Wolf app (apps mode)
type WolfAppInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// WolfSystemMemory represents Wolf's system memory usage (supports both apps and lobbies modes)
type WolfSystemMemory struct {
	Success              bool                   `json:"success"`
	ProcessRSSBytes      int64                  `json:"process_rss_bytes"`
	GStreamerBufferBytes int64                  `json:"gstreamer_buffer_bytes"`
	TotalMemoryBytes     int64                  `json:"total_memory_bytes"`
	Apps                 []WolfAppMemory        `json:"apps,omitempty"`    // Apps mode
	Lobbies              []WolfLobbyMemory      `json:"lobbies,omitempty"` // Lobbies mode
	Clients              []WolfClientConnection `json:"clients"`
}

// WolfAppMemory represents per-app memory usage (apps mode)
type WolfAppMemory struct {
	AppID       string `json:"app_id"`
	AppName     string `json:"app_name"`
	Resolution  string `json:"resolution"`
	ClientCount int    `json:"client_count"`
	MemoryBytes int64  `json:"memory_bytes"`
}

// WolfLobbyMemory represents per-lobby memory usage (lobbies mode)
type WolfLobbyMemory struct {
	LobbyID     string `json:"lobby_id"`
	LobbyName   string `json:"lobby_name"`
	Resolution  string `json:"resolution"`
	ClientCount string `json:"client_count"`
	MemoryBytes int64  `json:"memory_bytes"`
}

// WolfClientConnection represents individual client connections for leak detection
type WolfClientConnection struct {
	SessionID   int64   `json:"session_id"` // Wolf returns this as a number
	ClientIP    string  `json:"client_ip"`
	Resolution  string  `json:"resolution"`
	LobbyID     *string `json:"lobby_id,omitempty"` // lobbies mode: connected lobby
	AppID       *string `json:"app_id,omitempty"`   // apps mode: connected app
	MemoryBytes int64   `json:"memory_bytes"`
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
	// Get Wolf client - works with both executor types
	type WolfClientProvider interface {
		GetWolfClient() *wolf.Client
	}

	provider, ok := apiServer.externalAgentExecutor.(WolfClientProvider)
	if !ok {
		http.Error(rw, "Wolf executor not available", http.StatusInternalServerError)
		return
	}
	wolfClient := provider.GetWolfClient()

	ctx := req.Context()
	response := &AgentSandboxesDebugResponse{}

	// Fetch system memory data
	memoryData, err := fetchWolfMemoryData(ctx, wolfClient)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to fetch Wolf memory data: %v", err), http.StatusInternalServerError)
		return
	}
	response.Memory = memoryData

	// Check WOLF_MODE to determine whether to fetch apps or lobbies
	wolfMode := os.Getenv("WOLF_MODE")
	if wolfMode == "" {
		wolfMode = "apps" // Default
	}

	if wolfMode == "lobbies" {
		// Fetch lobbies (lobbies mode)
		lobbiesData, err := fetchWolfLobbies(ctx, wolfClient)
		if err != nil {
			http.Error(rw, fmt.Sprintf("Failed to fetch Wolf lobbies: %v", err), http.StatusInternalServerError)
			return
		}
		response.Lobbies = lobbiesData
	} else {
		// Fetch apps (apps mode)
		appsData, err := fetchWolfApps(ctx, wolfClient)
		if err != nil {
			http.Error(rw, fmt.Sprintf("Failed to fetch Wolf apps: %v", err), http.StatusInternalServerError)
			return
		}
		response.Apps = appsData
	}

	// Fetch sessions (both modes)
	sessionsData, err := fetchWolfSessions(ctx, wolfClient)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to fetch Wolf sessions: %v", err), http.StatusInternalServerError)
		return
	}
	response.Sessions = sessionsData

	// Fetch moonlight-web client connections
	moonlightClients, err := fetchMoonlightWebSessions(ctx)
	if err != nil {
		// Non-fatal - just log and continue without moonlight-web data
		fmt.Printf("Warning: Failed to fetch moonlight-web sessions: %v\n", err)
		response.MoonlightClients = []MoonlightClientInfo{}
	} else {
		response.MoonlightClients = moonlightClients
	}

	// Set Wolf mode in response so frontend knows which mode is active
	response.WolfMode = wolfMode

	// Return combined data
	writeResponse(rw, response, http.StatusOK)
}

// fetchWolfMemoryData retrieves memory usage data from Wolf
func fetchWolfMemoryData(ctx context.Context, wolfClient *wolf.Client) (*WolfSystemMemory, error) {
	resp, err := wolfClient.Get(ctx, "/api/v1/system/memory")
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
func fetchWolfLobbies(ctx context.Context, wolfClient *wolf.Client) ([]WolfLobbyInfo, error) {
	resp, err := wolfClient.Get(ctx, "/api/v1/lobbies")
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

// fetchWolfApps retrieves all apps from Wolf (apps mode)
func fetchWolfApps(ctx context.Context, wolfClient *wolf.Client) ([]WolfAppInfo, error) {
	resp, err := wolfClient.Get(ctx, "/api/v1/apps")
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf apps endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf apps endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var appsResponse struct {
		Success bool          `json:"success"`
		Apps    []WolfAppInfo `json:"apps"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&appsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode Wolf apps response: %w", err)
	}

	if !appsResponse.Success {
		return nil, fmt.Errorf("Wolf apps endpoint returned success=false")
	}

	return appsResponse.Apps, nil
}

// fetchWolfSessions retrieves all streaming sessions from Wolf
func fetchWolfSessions(ctx context.Context, wolfClient *wolf.Client) ([]WolfSessionInfo, error) {
	resp, err := wolfClient.Get(ctx, "/api/v1/sessions")
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

// fetchMoonlightWebSessions retrieves all client connections from moonlight-web
func fetchMoonlightWebSessions(ctx context.Context) ([]MoonlightClientInfo, error) {
	// Get moonlight-web URL from environment or use default
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080" // Default internal URL
	}

	// Build request URL
	url := fmt.Sprintf("%s/api/sessions", moonlightWebURL)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authentication header (moonlight-web uses MOONLIGHT_CREDENTIALS)
	credentials := os.Getenv("MOONLIGHT_CREDENTIALS")
	if credentials != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", credentials))
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request moonlight-web sessions endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("moonlight-web sessions endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var sessionsResponse struct {
		Sessions []struct {
			SessionID    string `json:"session_id"`
			Mode         string `json:"mode"`
			HasWebsocket bool   `json:"has_websocket"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessionsResponse); err != nil {
		return nil, fmt.Errorf("failed to decode moonlight-web sessions response: %w", err)
	}

	// Transform to our struct
	clients := make([]MoonlightClientInfo, len(sessionsResponse.Sessions))
	for i, session := range sessionsResponse.Sessions {
		clients[i] = MoonlightClientInfo{
			SessionID:    session.SessionID,
			Mode:         session.Mode,
			HasWebsocket: session.HasWebsocket,
		}
	}

	return clients, nil
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
