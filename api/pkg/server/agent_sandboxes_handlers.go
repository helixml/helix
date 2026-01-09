package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/wolf"
	"github.com/rs/zerolog/log"
)

// AgentSandboxesDebugResponse combines data from multiple Wolf endpoints
// for comprehensive debugging of the agent streaming infrastructure
type AgentSandboxesDebugResponse struct {
	Memory             *WolfSystemMemory       `json:"memory"`
	Apps               []WolfAppInfo           `json:"apps,omitempty"`    // Apps mode
	Lobbies            []WolfLobbyInfo         `json:"lobbies,omitempty"` // Lobbies mode
	Sessions           []WolfSessionInfo       `json:"sessions"`
	MoonlightClients   []MoonlightClientInfo   `json:"moonlight_clients"`             // moonlight-web client connections
	WolfMode           string                  `json:"wolf_mode"`                     // Current Wolf mode ("apps" or "lobbies")
	GPUStats           *GPUStats               `json:"gpu_stats,omitempty"`           // GPU encoder stats from Wolf (via nvidia-smi or rocm-smi)
	GStreamerPipelines *GStreamerPipelineStats `json:"gstreamer_pipelines,omitempty"` // Actual pipeline count from Wolf
}

// GPUStats represents real-time GPU metrics from Wolf (via nvidia-smi or rocm-smi)
type GPUStats struct {
	GPUName               string  `json:"gpu_name"`
	EncoderSessionCount   int     `json:"encoder_session_count"`
	EncoderAverageFps     float64 `json:"encoder_average_fps"`
	EncoderAverageLatency int     `json:"encoder_average_latency_us"`
	EncoderUtilization    int     `json:"encoder_utilization_percent"`
	GPUUtilization        int     `json:"gpu_utilization_percent"`
	MemoryUtilization     int     `json:"memory_utilization_percent"`
	MemoryUsedMB          int     `json:"memory_used_mb"`
	MemoryTotalMB         int     `json:"memory_total_mb"`
	TemperatureC          int     `json:"temperature_celsius"`
	QueryDurationMS       int     `json:"query_duration_ms"` // How long nvidia-smi took in Wolf
	Available             bool    `json:"available"`         // false if nvidia-smi failed
	Error                 string  `json:"error,omitempty"`
}

// GStreamerPipelineStats represents actual GStreamer pipeline counts from Wolf state
type GStreamerPipelineStats struct {
	ProducerPipelines int `json:"producer_pipelines"` // Video + audio producers (2 per lobby)
	ConsumerPipelines int `json:"consumer_pipelines"` // Video + audio consumers (2 per session)
	TotalPipelines    int `json:"total_pipelines"`    // Sum of producers + consumers
}

// MoonlightClientInfo represents a moonlight-web client connection
type MoonlightClientInfo struct {
	SessionID      string  `json:"session_id"`
	ClientUniqueID *string `json:"client_unique_id,omitempty"` // Unique Moonlight client ID (null for browser clients)
	Mode           string  `json:"mode"`                       // "create", "keepalive", "join"
	HasWebsocket   bool    `json:"has_websocket"`              // Is a WebRTC client currently connected?
}

// WolfAppInfo represents a Wolf app (apps mode)
type WolfAppInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// WolfSystemMemory represents Wolf's system memory usage (supports both apps and lobbies modes)
type WolfSystemMemory struct {
	Success              bool                    `json:"success"`
	ProcessRSSBytes      int64                   `json:"process_rss_bytes"`
	GStreamerBufferBytes int64                   `json:"gstreamer_buffer_bytes"`
	TotalMemoryBytes     int64                   `json:"total_memory_bytes"`
	Apps                 []WolfAppMemory         `json:"apps,omitempty"`    // Apps mode
	Lobbies              []WolfLobbyMemory       `json:"lobbies,omitempty"` // Lobbies mode
	Clients              []WolfClientConnection  `json:"clients"`
	GPUStats             *GPUStats               `json:"gpu_stats,omitempty"`           // From Wolf's GPU monitoring query (nvidia-smi or rocm-smi)
	GStreamerPipelines   *GStreamerPipelineStats `json:"gstreamer_pipelines,omitempty"` // From Wolf's state
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
	ClientCount int    `json:"client_count"`
	MemoryBytes int64  `json:"memory_bytes"`
}

// WolfClientConnection represents individual client connections for leak detection
type WolfClientConnection struct {
	SessionID   string  `json:"session_id"` // Wolf returns this as string (Moonlight protocol requirement)
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
	SessionID        string  `json:"session_id"`                 // Exposed as session_id for frontend (Wolf's client_id)
	ClientUniqueID   *string `json:"client_unique_id,omitempty"` // Helix client ID (helix-agent-{session_id}-{instance_id})
	ClientIP         string  `json:"client_ip"`
	AppID            string  `json:"app_id"`             // Wolf UI app ID in lobbies mode
	LobbyID          *string `json:"lobby_id,omitempty"` // Which lobby this session is connected to (lobbies mode)
	IdleSeconds      int     `json:"idle_seconds"`       // Seconds since last ENET packet (for timeout monitoring)
	VideoWidth       int     `json:"-"`                  // Internal field from Wolf
	VideoHeight      int     `json:"-"`                  // Internal field from Wolf
	VideoRefreshRate int     `json:"-"`                  // Internal field from Wolf
	DisplayMode      struct {
		Width         int  `json:"width"`
		Height        int  `json:"height"`
		RefreshRate   int  `json:"refresh_rate"`
		HEVCSupported bool `json:"hevc_supported"`
		AV1Supported  bool `json:"av1_supported"`
	} `json:"display_mode"`
}

// @Summary Get Wolf debugging data
// @Description Retrieves combined debug data from Wolf (memory, lobbies, sessions) for the Agent Sandboxes dashboard
// @Tags Admin
// @Accept json
// @Produce json
// @Param wolf_instance_id query string true "Wolf instance ID to query"
// @Success 200 {object} AgentSandboxesDebugResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/admin/agent-sandboxes/debug [get]
func (apiServer *HelixAPIServer) getAgentSandboxesDebug(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Get Wolf instance ID from query parameter
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(rw, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get Wolf client provider for querying specific instances
	type WolfClientForSessionProvider interface {
		GetWolfClientForSession(wolfInstanceID string) external_agent.WolfClientInterface
	}

	provider, ok := apiServer.externalAgentExecutor.(WolfClientForSessionProvider)
	if !ok {
		http.Error(rw, "Wolf executor not available", http.StatusInternalServerError)
		return
	}

	wolfClient := provider.GetWolfClientForSession(wolfInstanceID)

	response := &AgentSandboxesDebugResponse{}

	// Check WOLF_MODE to determine whether to fetch apps or lobbies
	wolfMode := os.Getenv("WOLF_MODE")
	if wolfMode == "" {
		wolfMode = "lobbies" // Default (consistent with wolf_executor.go)
	}

	// Fetch system memory data
	memoryData, err := fetchWolfMemoryData(ctx, wolfClient)
	if err != nil {
		log.Warn().Err(err).Str("wolf_instance_id", wolfInstanceID).Msg("Failed to fetch Wolf memory data")
	} else {
		response.Memory = memoryData
	}

	var rawLobbies []wolf.Lobby

	if wolfMode == "lobbies" {
		// Fetch lobbies (lobbies mode)
		lobbiesData, err := fetchWolfLobbies(ctx, wolfClient)
		if err != nil {
			log.Warn().Err(err).Str("wolf_instance_id", wolfInstanceID).Msg("Failed to fetch Wolf lobbies")
		} else {
			response.Lobbies = lobbiesData
		}

		// Fetch raw lobbies for session matching
		rawLobbies, err = wolfClient.ListLobbies(ctx)
		if err != nil {
			log.Warn().Err(err).Str("wolf_instance_id", wolfInstanceID).Msg("Failed to fetch raw Wolf lobbies")
		}
	} else {
		// Fetch apps (apps mode)
		appsData, err := fetchWolfApps(ctx, wolfClient)
		if err != nil {
			log.Warn().Err(err).Str("wolf_instance_id", wolfInstanceID).Msg("Failed to fetch Wolf apps")
		} else {
			response.Apps = appsData
		}
	}

	// Fetch sessions
	sessionsData, err := fetchWolfSessions(ctx, wolfClient)
	if err != nil {
		log.Warn().Err(err).Str("wolf_instance_id", wolfInstanceID).Msg("Failed to fetch Wolf sessions")
	}

	// In lobbies mode, match Wolf sessions with lobbies by extracting Helix session ID
	// from client_unique_id and matching against lobby env vars
	if wolfMode == "lobbies" && len(rawLobbies) > 0 {
		for i := range sessionsData {
			session := &sessionsData[i]
			if session.ClientUniqueID == nil {
				continue
			}

			// Extract Helix session ID from client_unique_id: helix-agent-{session_id}-{instance_id}
			uniqueID := *session.ClientUniqueID
			if !strings.HasPrefix(uniqueID, "helix-agent-") {
				continue
			}

			// Remove "helix-agent-" prefix and instance ID suffix
			parts := strings.Split(strings.TrimPrefix(uniqueID, "helix-agent-"), "-")
			if len(parts) == 0 {
				continue
			}

			// Helix session ID is everything except the last UUID part
			// Session IDs are ~30 chars, UUIDs are 36 chars with hyphens
			helixSessionID := strings.Join(parts[:len(parts)-1], "-")
			if len(helixSessionID) < 20 {
				continue // Too short to be a session ID
			}

			// Find lobby with matching HELIX_SESSION_ID in env vars
			for _, lobby := range rawLobbies {
				// Parse lobby.Runner to extract env vars
				if runnerMap, ok := lobby.Runner.(map[string]interface{}); ok {
					if envList, ok := runnerMap["env"].([]interface{}); ok {
						for _, envVar := range envList {
							if envStr, ok := envVar.(string); ok {
								expectedEnv := fmt.Sprintf("HELIX_SESSION_ID=%s", helixSessionID)
								if envStr == expectedEnv {
									session.LobbyID = &lobby.ID
									break
								}
							}
						}
					}
				}
				if session.LobbyID != nil {
					break
				}
			}
		}
	}

	response.Sessions = sessionsData

	// Fetch moonlight-web client connections via RevDial
	moonlightClients, err := apiServer.fetchMoonlightWebSessions(ctx, wolfInstanceID)
	if err != nil {
		// Non-fatal - just log and continue without moonlight-web data
		log.Warn().Err(err).Str("wolf_instance_id", wolfInstanceID).Msg("Failed to fetch moonlight-web sessions via RevDial")
		response.MoonlightClients = []MoonlightClientInfo{}
	} else {
		response.MoonlightClients = moonlightClients
	}

	// Set Wolf mode in response so frontend knows which mode is active
	response.WolfMode = wolfMode

	// GPU stats and pipeline stats are already included in response.Memory from Wolf
	if response.Memory != nil {
		response.GPUStats = response.Memory.GPUStats
		response.GStreamerPipelines = response.Memory.GStreamerPipelines
	}

	// Return combined data
	writeResponse(rw, response, http.StatusOK)
}

// fetchWolfMemoryData retrieves memory usage data from Wolf via WolfClientInterface
func fetchWolfMemoryData(ctx context.Context, wolfClient external_agent.WolfClientInterface) (*WolfSystemMemory, error) {
	memResp, err := wolfClient.GetSystemMemory(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf memory endpoint: %w", err)
	}

	// Convert wolf.SystemMemoryResponse to WolfSystemMemory
	result := &WolfSystemMemory{
		Success:              true,
		ProcessRSSBytes:      memResp.ProcessRSSBytes,
		GStreamerBufferBytes: memResp.GStreamerBufferBytes,
		TotalMemoryBytes:     memResp.TotalMemoryBytes,
	}

	// Convert apps
	if memResp.Apps != nil {
		result.Apps = make([]WolfAppMemory, len(memResp.Apps))
		for i, app := range memResp.Apps {
			result.Apps[i] = WolfAppMemory{
				AppID:       app.AppID,
				AppName:     app.AppName,
				Resolution:  app.Resolution,
				ClientCount: app.ClientCount,
			}
		}
	}

	// Convert lobbies
	if memResp.Lobbies != nil {
		result.Lobbies = make([]WolfLobbyMemory, len(memResp.Lobbies))
		for i, lobby := range memResp.Lobbies {
			result.Lobbies[i] = WolfLobbyMemory{
				LobbyID:     lobby.LobbyID,
				LobbyName:   lobby.LobbyName,
				ClientCount: lobby.ClientCount,
				MemoryBytes: lobby.MemoryBytes,
			}
		}
	}

	// Convert clients (wolf.ClientMemoryInfo only has ClientID and MemoryBytes)
	if memResp.Clients != nil {
		result.Clients = make([]WolfClientConnection, len(memResp.Clients))
		for i, client := range memResp.Clients {
			result.Clients[i] = WolfClientConnection{
				SessionID:   client.ClientID,
				MemoryBytes: client.MemoryBytes,
				// Note: ClientIP, Resolution, LobbyID, AppID are not available in memory endpoint
				// They come from the sessions endpoint instead
			}
		}
	}

	// Convert GPU stats if available
	if memResp.GPUStats != nil {
		result.GPUStats = &GPUStats{
			Available:             memResp.GPUStats.Available,
			GPUName:               memResp.GPUStats.GPUName,
			EncoderSessionCount:   memResp.GPUStats.EncoderSessionCount,
			EncoderAverageFps:     memResp.GPUStats.EncoderAverageFPS,
			EncoderAverageLatency: memResp.GPUStats.EncoderAverageLatencyUs,
			EncoderUtilization:    memResp.GPUStats.EncoderUtilizationPercent,
			GPUUtilization:        memResp.GPUStats.GPUUtilizationPercent,
			MemoryUtilization:     memResp.GPUStats.MemoryUtilizationPercent,
			MemoryUsedMB:          memResp.GPUStats.MemoryUsedMB,
			MemoryTotalMB:         memResp.GPUStats.MemoryTotalMB,
			TemperatureC:          memResp.GPUStats.TemperatureCelsius,
			QueryDurationMS:       memResp.GPUStats.QueryDurationMs,
			Error:                 memResp.GPUStats.Error,
		}
	}

	// Convert GStreamer pipelines if available
	if memResp.GStreamerPipelines != nil {
		result.GStreamerPipelines = &GStreamerPipelineStats{
			ProducerPipelines: memResp.GStreamerPipelines.ProducerPipelines,
			ConsumerPipelines: memResp.GStreamerPipelines.ConsumerPipelines,
			TotalPipelines:    memResp.GStreamerPipelines.TotalPipelines,
		}
	}

	return result, nil
}

// fetchWolfLobbies retrieves all lobbies from Wolf via WolfClientInterface
func fetchWolfLobbies(ctx context.Context, wolfClient external_agent.WolfClientInterface) ([]WolfLobbyInfo, error) {
	lobbies, err := wolfClient.ListLobbies(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf lobbies endpoint: %w", err)
	}

	// Convert wolf.Lobby to WolfLobbyInfo
	result := make([]WolfLobbyInfo, len(lobbies))
	for i, lobby := range lobbies {
		result[i] = WolfLobbyInfo{
			ID:                     lobby.ID,
			Name:                   lobby.Name,
			StartedByProfileID:     lobby.StartedByProfileID,
			MultiUser:              lobby.MultiUser,
			StopWhenEveryoneLeaves: lobby.StopWhenEveryoneLeaves,
			PIN:                    lobby.PIN,
		}
	}

	return result, nil
}

// fetchWolfApps retrieves all apps from Wolf (apps mode) via WolfClientInterface
func fetchWolfApps(ctx context.Context, wolfClient external_agent.WolfClientInterface) ([]WolfAppInfo, error) {
	apps, err := wolfClient.ListApps(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf apps endpoint: %w", err)
	}

	// Convert wolf.App to WolfAppInfo
	result := make([]WolfAppInfo, len(apps))
	for i, app := range apps {
		result[i] = WolfAppInfo{
			ID:    app.ID,
			Title: app.Title,
		}
	}

	return result, nil
}

// getWolfUIAppID godoc
// @Summary Get Wolf UI app ID
// @Description Get the Wolf UI app ID for lobbies mode streaming
// @Tags Wolf
// @Produce json
// @Param session_id query string false "Session ID to look up Wolf instance"
// @Success 200 {object} map[string]string
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/wolf/ui-app-id [get]
func (apiServer *HelixAPIServer) getWolfUIAppID(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Get session ID from query parameter to determine which Wolf instance to query
	sessionID := req.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(rw, "session_id query parameter is required to determine Wolf instance", http.StatusBadRequest)
		return
	}

	// Look up session to get Wolf instance ID
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to get session: %v", err), http.StatusNotFound)
		return
	}

	wolfInstanceID := session.WolfInstanceID
	if wolfInstanceID == "" {
		http.Error(rw, "Session has no Wolf instance ID - session may be corrupted", http.StatusInternalServerError)
		return
	}

	wolfClient := apiServer.externalAgentExecutor.GetWolfClientForSession(wolfInstanceID)

	// Fetch Wolf apps
	apps, err := fetchWolfApps(ctx, wolfClient)
	if err != nil {
		http.Error(rw, fmt.Sprintf("Failed to fetch Wolf apps: %v", err), http.StatusInternalServerError)
		return
	}

	// Find placeholder app by name - prefer "Select Agent" (Wolf-UI with real Wayland compositor)
	// The "Blank" test pattern causes NVENC buffer registration failures on second session
	// because shared lobby buffers have stale registrations from previous encoder sessions.
	// See design/2025-12-04-websocket-mode-session-leak.md for details.
	var foundAppID string
	for _, app := range apps {
		if app.Title == "Select Agent" {
			foundAppID = app.ID
			break
		}
		if app.Title == "Blank" && foundAppID == "" {
			foundAppID = app.ID
		}
	}

	if foundAppID != "" {
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]string{
			"placeholder_app_id": foundAppID,
		})
		return
	}

	http.Error(rw, "Placeholder app (Blank or Select Agent) not found in apps list", http.StatusNotFound)
}

// fetchWolfSessions retrieves all streaming sessions from Wolf via WolfClientInterface
func fetchWolfSessions(ctx context.Context, wolfClient external_agent.WolfClientInterface) ([]WolfSessionInfo, error) {
	wolfSessions, err := wolfClient.ListSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to request Wolf sessions endpoint: %w", err)
	}

	// Convert wolf.WolfStreamSession to WolfSessionInfo
	sessions := make([]WolfSessionInfo, len(wolfSessions))
	for i, raw := range wolfSessions {
		// Convert string to *string for ClientUniqueID (may be empty)
		var clientUniqueID *string
		if raw.ClientUniqueID != "" {
			clientUniqueID = &raw.ClientUniqueID
		}

		sessions[i] = WolfSessionInfo{
			SessionID:      raw.ClientID,
			ClientUniqueID: clientUniqueID, // Helix session identifier (helix-agent-{session_id}-{instance_id})
			ClientIP:       raw.ClientIP,
			AppID:          raw.AppID,       // May be empty string for lobbies mode
			LobbyID:        nil,             // Will be populated later by matching against lobbies
			IdleSeconds:    raw.IdleSeconds, // Seconds since last ENET packet (for timeout monitoring)
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

// fetchMoonlightWebSessions retrieves all client connections from moonlight-web via RevDial
func (apiServer *HelixAPIServer) fetchMoonlightWebSessions(ctx context.Context, wolfInstanceID string) ([]MoonlightClientInfo, error) {
	// Determine moonlight runner ID from wolf instance ID
	moonlightRunnerID := "moonlight-" + wolfInstanceID
	if wolfInstanceID == "" {
		// Fallback to environment variable or dev default
		moonlightRunnerID = os.Getenv("MOONLIGHT_RUNNER_ID")
		if moonlightRunnerID == "" {
			moonlightRunnerID = "moonlight-dev"
		}
	}

	// Dial Moonlight Web via RevDial
	conn, err := apiServer.connman.Dial(ctx, moonlightRunnerID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial moonlight-web via RevDial (runner_id=%s): %w", moonlightRunnerID, err)
	}
	defer conn.Close()

	// Check mode to determine which endpoint to query
	moonlightMode := os.Getenv("MOONLIGHT_WEB_MODE")
	if moonlightMode == "" {
		moonlightMode = "single" // Default to single mode (session-persistence)
	}

	// Build request path based on mode
	var path string
	if moonlightMode == "multi" {
		path = "/api/streamers"
	} else {
		path = "/api/sessions"
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", "http://moonlight-web"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.URL.Path = path
	req.RequestURI = path

	// Set authentication header (moonlight-web uses MOONLIGHT_CREDENTIALS)
	credentials := apiServer.getMoonlightCredentials()
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", credentials))

	// Write HTTP request to RevDial connection
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("failed to write request to RevDial connection: %w", err)
	}

	// Read HTTP response from RevDial connection
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from RevDial connection: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("moonlight-web endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response based on mode
	var clients []MoonlightClientInfo

	if moonlightMode == "multi" {
		// Multi-WebRTC format: streamers API
		var streamersResponse struct {
			Streamers []struct {
				StreamerID         string `json:"streamer_id"`
				Status             string `json:"status"`
				MoonlightConnected bool   `json:"moonlight_connected"`
				ConnectedPeers     int    `json:"connected_peers"`
			} `json:"streamers"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&streamersResponse); err != nil {
			return nil, fmt.Errorf("failed to decode moonlight-web streamers response: %w", err)
		}

		// Transform streamers to client info format
		clients = make([]MoonlightClientInfo, len(streamersResponse.Streamers))
		for i, streamer := range streamersResponse.Streamers {
			clients[i] = MoonlightClientInfo{
				SessionID:      streamer.StreamerID,
				ClientUniqueID: nil,        // Streamers don't expose client_unique_id directly
				Mode:           "streamer", // New architecture uses persistent streamers
				HasWebsocket:   streamer.ConnectedPeers > 0,
			}
		}
	} else {
		// Single mode format: sessions API
		var sessionsResponse struct {
			Sessions []struct {
				SessionID      string  `json:"session_id"`
				ClientUniqueID *string `json:"client_unique_id"` // Unique Moonlight client ID
				Mode           string  `json:"mode"`
				HasWebsocket   bool    `json:"has_websocket"`
			} `json:"sessions"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&sessionsResponse); err != nil {
			return nil, fmt.Errorf("failed to decode moonlight-web sessions response: %w", err)
		}

		// Transform to our struct
		clients = make([]MoonlightClientInfo, len(sessionsResponse.Sessions))
		for i, session := range sessionsResponse.Sessions {
			clients[i] = MoonlightClientInfo{
				SessionID:      session.SessionID,
				ClientUniqueID: session.ClientUniqueID,
				Mode:           session.Mode,
				HasWebsocket:   session.HasWebsocket,
			}
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
	ctx := req.Context()

	// Get Wolf instance ID from query parameter
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(rw, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	wolfClient := apiServer.externalAgentExecutor.GetWolfClientForSession(wolfInstanceID)

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

// SessionWolfAppStateResponse represents the Wolf app state for a specific external agent session
type SessionWolfAppStateResponse struct {
	SessionID      string `json:"session_id"`
	WolfAppID      string `json:"wolf_app_id"`
	State          string `json:"state"`            // "absent", "running", "resumable"
	HasWebsocket   bool   `json:"has_websocket"`    // Is a browser client currently connected?
	ClientUniqueID string `json:"client_unique_id"` // Unique Moonlight client ID for this agent
}

// @Summary Get Wolf app state for a session
// @Description Returns the current Wolf app state for an external agent session (absent/running/resumable)
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} SessionWolfAppStateResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sessions/{id}/wolf-app-state [get]
func (apiServer *HelixAPIServer) getSessionWolfAppState(rw http.ResponseWriter, req *http.Request) {
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

	// Get session's Wolf instance ID for proper RevDial routing
	wolfInstanceID := session.WolfInstanceID

	// CRITICAL: If session has no WolfInstanceID yet, the lobby hasn't been created
	// WolfInstanceID is set in wolf_executor.go AFTER the lobby is created
	// This is the simplest and most reliable way to detect "starting" state
	if wolfInstanceID == "" {
		response := SessionWolfAppStateResponse{
			SessionID:      sessionID,
			WolfAppID:      "", // Not known yet
			State:          "starting",
			HasWebsocket:   false,
			ClientUniqueID: fmt.Sprintf("helix-agent-%s", sessionID),
		}
		writeResponse(rw, response, http.StatusOK)
		return
	}

	wolfClient := apiServer.externalAgentExecutor.GetWolfClientForSession(wolfInstanceID)

	// Determine the expected Wolf app ID and client_unique_id for this session
	// These must match what the backend used in wolf_executor_apps.go
	expectedMoonlightSessionID := fmt.Sprintf("agent-%s", sessionID)
	expectedClientUniqueID := fmt.Sprintf("helix-agent-%s", sessionID)

	// Generate Wolf app ID using same logic as wolf_executor.go
	// wolfAppID = hash(userID + sessionID) % 1000000000
	stableKey := fmt.Sprintf("%s-%s", user.ID, sessionID)
	var numericHash uint64
	for _, b := range []byte(stableKey) {
		numericHash = numericHash*31 + uint64(b)
	}
	wolfAppID := fmt.Sprintf("%d", numericHash%1000000000)

	// Query moonlight-web via RevDial to check session state
	var moonlightSession *MoonlightClientInfo
	moonlightClients, err := apiServer.fetchMoonlightWebSessions(ctx, wolfInstanceID)
	if err != nil {
		// Log warning but continue - we can still return wolfAppID based on Wolf data alone
		log.Warn().Err(err).Str("wolf_instance_id", wolfInstanceID).Msg("Failed to fetch moonlight-web sessions via RevDial")
	} else {
		// Find this session's moonlight client
		for _, client := range moonlightClients {
			if client.SessionID == expectedMoonlightSessionID {
				moonlightSession = &client
				break
			}
		}
	}

	// CRITICAL: Always query Wolf as the single source of truth
	// Never use in-memory maps - they can be stale, partial, or wrong
	var wolfLobbyID string
	var isLobbiesMode bool

	// Query Wolf API directly for lobbies (ONLY source of truth)
	foundLobbyID, err := apiServer.externalAgentExecutor.FindExistingLobbyForSession(ctx, sessionID)
	if err != nil {
		// Wolf query failed - session will be reported as "absent"
	} else if foundLobbyID != "" {
		// Found existing lobby in Wolf
		wolfLobbyID = foundLobbyID
		isLobbiesMode = true
	}

	// Determine state based on moonlight-web and Wolf data
	var state string
	hasWebsocket := false

	if moonlightSession != nil {
		hasWebsocket = moonlightSession.HasWebsocket
		if moonlightSession.Mode == "keepalive" && !moonlightSession.HasWebsocket {
			// Keepalive session with no websocket = resumable (kicked off but browser not connected)
			state = "resumable"
		} else if moonlightSession.HasWebsocket {
			// Has websocket = currently running/streaming
			state = "running"
		} else {
			// Session exists but no clear state
			state = "resumable"
		}
	} else {
		// No moonlight session found
		// Check if Wolf app/lobby still exists (container might be running without moonlight session)
		resourceExists := false

		// Only check Wolf resources if we have a valid Wolf client
		// wolfClient can be nil in Wolf-free mode (direct PipeWire streaming)
		if wolfClient != nil {
			if isLobbiesMode {
				// Check lobbies
				lobbies, err := fetchWolfLobbies(ctx, wolfClient)
				if err == nil {
					for _, lobby := range lobbies {
						if lobby.ID == wolfLobbyID {
							resourceExists = true
							break
						}
					}
				}
			} else {
				// Check apps
				apps, err := fetchWolfApps(ctx, wolfClient)
				if err == nil {
					for _, app := range apps {
						if app.ID == wolfAppID {
							resourceExists = true
							break
						}
					}
				}
			}
		}

		if resourceExists {
			state = "resumable" // App/lobby exists but no moonlight session
		} else {
			// No app/lobby found and we have a WolfInstanceID = container was stopped
			// (if no WolfInstanceID, we already returned "starting" earlier)
			// In Wolf-free mode, this means the desktop container isn't running
			state = "absent"
		}
	}

	response := SessionWolfAppStateResponse{
		SessionID:      sessionID,
		WolfAppID:      wolfAppID,
		State:          state,
		HasWebsocket:   hasWebsocket,
		ClientUniqueID: expectedClientUniqueID,
	}

	writeResponse(rw, response, http.StatusOK)
}

// @Summary Stop Wolf lobby
// @Description Stop a Wolf lobby (terminates container and releases GPU resources)
// @Tags Admin
// @Param lobbyId path string true "Lobby ID"
// @Param wolf_instance_id query string true "Wolf instance ID to operate on"
// @Success 200
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/admin/wolf/lobbies/{lobbyId} [delete]
func (apiServer *HelixAPIServer) deleteWolfLobby(rw http.ResponseWriter, req *http.Request) {
	// Admin-only endpoint - verify user is admin
	user := getRequestUser(req)
	if user == nil || user.Admin == false {
		http.Error(rw, "Unauthorized - admin access required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	lobbyID := vars["lobbyId"]

	log.Info().
		Str("user_id", user.ID).
		Str("lobby_id", lobbyID).
		Msg("Admin stopping Wolf lobby")

	// Look up the lobby PIN from ExternalAgentActivity table
	// This survives session deletion and is designed for cleanup
	activity, err := apiServer.Store.GetExternalAgentActivityByLobbyID(req.Context(), lobbyID)
	if err != nil {
		log.Error().
			Err(err).
			Str("lobby_id", lobbyID).
			Msg("Failed to find external agent activity for lobby - PIN required for Wolf stop")
		http.Error(rw, "Cannot stop lobby: No activity record found (PIN required)", http.StatusNotFound)
		return
	}

	if activity.WolfLobbyPIN == "" {
		log.Error().
			Str("lobby_id", lobbyID).
			Str("external_agent_id", activity.ExternalAgentID).
			Msg("Activity found but PIN is missing")
		http.Error(rw, "Cannot stop lobby: PIN not found in activity record", http.StatusInternalServerError)
		return
	}

	// Convert PIN string "1234" to []int16{1, 2, 3, 4}
	var lobbyPIN []int16
	if len(activity.WolfLobbyPIN) == 4 {
		lobbyPIN = make([]int16, 4)
		for i, ch := range activity.WolfLobbyPIN {
			lobbyPIN[i] = int16(ch - '0')
		}
		log.Info().
			Str("lobby_id", lobbyID).
			Str("external_agent_id", activity.ExternalAgentID).
			Str("pin", activity.WolfLobbyPIN).
			Msg("Found lobby PIN from external agent activity")
	} else {
		log.Error().
			Str("lobby_id", lobbyID).
			Str("pin", activity.WolfLobbyPIN).
			Msg("Invalid PIN format - must be 4 digits")
		http.Error(rw, "Cannot stop lobby: Invalid PIN format", http.StatusInternalServerError)
		return
	}

	// Get Wolf instance ID from query parameter (admin must specify which Wolf instance)
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(rw, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get Wolf client from external agent executor
	wolfClient := apiServer.externalAgentExecutor.GetWolfClientForSession(wolfInstanceID)

	// Stop the lobby with PIN
	stopReq := &wolf.StopLobbyRequest{
		LobbyID: lobbyID,
		PIN:     lobbyPIN, // Use PIN from session metadata
	}

	err = wolfClient.StopLobby(req.Context(), stopReq)
	if err != nil {
		log.Error().
			Err(err).
			Str("lobby_id", lobbyID).
			Interface("pin", lobbyPIN).
			Msg("Failed to stop Wolf lobby")
		http.Error(rw, fmt.Sprintf("Failed to stop lobby: %v", err), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("lobby_id", lobbyID).
		Str("user_id", user.ID).
		Msg("Successfully stopped Wolf lobby")

	rw.WriteHeader(http.StatusOK)
}

// @Summary Stop Wolf streaming session
// @Description Stop a Wolf-UI streaming session (releases GPU memory)
// @Tags Admin
// @Param sessionId path string true "Session ID (client_id from Wolf)"
// @Param wolf_instance_id query string true "Wolf instance ID to operate on"
// @Success 200
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/admin/wolf/sessions/{sessionId} [delete]
func (apiServer *HelixAPIServer) deleteWolfSession(rw http.ResponseWriter, req *http.Request) {
	// Admin-only endpoint - verify user is admin
	user := getRequestUser(req)
	if user == nil || user.Admin == false {
		http.Error(rw, "Unauthorized - admin access required", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionId"]

	// Get Wolf instance ID from query parameter (admin must specify which Wolf instance)
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(rw, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("wolf_session_id", sessionID).
		Str("wolf_instance_id", wolfInstanceID).
		Msg("Admin stopping Wolf streaming session")

	// Get Wolf client from external agent executor
	wolfClient := apiServer.externalAgentExecutor.GetWolfClientForSession(wolfInstanceID)

	// Stop the session
	err := wolfClient.StopSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("wolf_session_id", sessionID).Msg("Failed to stop Wolf session")
		http.Error(rw, fmt.Sprintf("Failed to stop session: %v", err), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("wolf_session_id", sessionID).
		Str("user_id", user.ID).
		Msg("Successfully stopped Wolf streaming session")

	rw.WriteHeader(http.StatusOK)
}
