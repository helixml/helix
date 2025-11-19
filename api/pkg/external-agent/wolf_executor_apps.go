package external_agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
)

// WebSocketConnectionChecker provides a way to check if a Zed instance has connected via WebSocket
type WebSocketConnectionChecker interface {
	IsExternalAgentConnected(sessionID string) bool
}

// AppWolfExecutor implements the Executor interface using Wolf Apps API (stable branch)
// This is the simpler, more reliable approach without lobbies
type AppWolfExecutor struct {
	wolfClient        WolfClientInterface
	store             store.Store
	sessions          map[string]*ZedSession
	mutex             sync.RWMutex
	zedImage          string
	helixAPIURL       string
	helixAPIToken     string
	workspaceBasePath string
	wsChecker         WebSocketConnectionChecker
}

// NewAppWolfExecutor creates a new app-based Wolf executor
func NewAppWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, store store.Store, wsChecker WebSocketConnectionChecker) *AppWolfExecutor {
	// CRITICAL: Validate HELIX_HOST_HOME is set - required for dev mode bind-mounts
	// In production mode (HELIX_DEV_MODE != true), files are baked into the image
	devMode := os.Getenv("HELIX_DEV_MODE") == "true"
	helixHostHome := os.Getenv("HELIX_HOST_HOME")

	if devMode && helixHostHome == "" {
		log.Fatal().Msg("HELIX_DEV_MODE is enabled but HELIX_HOST_HOME is not set. This variable must point to the Helix installation directory (e.g., /opt/HelixML or $HOME/HelixML) for dev bind-mounts. Please set it in your .env file.")
	}

	if devMode {
		log.Info().
			Str("helix_host_home", helixHostHome).
			Msg("Wolf executor (apps mode) initialized with HELIX_HOST_HOME (dev mode)")
	} else {
		log.Info().Msg("Wolf executor (apps mode) initialized (production mode - using files baked into image)")
	}

	wolfClient := wolf.NewClient(wolfSocketPath)

	executor := &AppWolfExecutor{
		wolfClient:        wolfClient,
		wsChecker:         wsChecker,
		store:             store,
		sessions:          make(map[string]*ZedSession),
		zedImage:          zedImage,
		helixAPIURL:       helixAPIURL,
		helixAPIToken:     helixAPIToken,
		workspaceBasePath: "/opt/helix/filestore/workspaces",
	}

	// Apps-based model: apps are lost on Wolf restart
	// We don't auto-restart Wolf - let Docker/systemd handle container crashes
	// Apps will need to be manually recreated if Wolf restarts

	return executor
}

// StartZedAgent implements the Executor interface for external agent sessions (apps model)
func (w *AppWolfExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("project_path", agent.ProjectPath).
		Msg("Starting external Zed agent via Wolf (apps mode)")

	// Generate numeric Wolf app ID
	wolfAppID := generateWolfAppID(agent.UserID, agent.SessionID)

	// Determine workspace directory
	workspaceDir := agent.WorkDir
	if workspaceDir == "" {
		workspaceDir = filepath.Join(w.workspaceBasePath, "external-agents", agent.SessionID)
	}

	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Create Sway config
	err := createSwayConfig(agent.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Sway config: %w", err)
	}

	// Container hostname
	sessionIDPart := strings.TrimPrefix(agent.SessionID, "ses_")
	containerHostname := fmt.Sprintf("zed-external-%s", sessionIDPart)

	// Determine Helix session ID
	helixSessionID := agent.SessionID
	if agent.HelixSessionID != "" {
		helixSessionID = agent.HelixSessionID
	}

	// Build environment variables
	extraEnv := []string{
		fmt.Sprintf("HELIX_AGENT_INSTANCE_ID=zed-session-%s", agent.SessionID),
		fmt.Sprintf("HELIX_SCOPE_TYPE=session"),
		fmt.Sprintf("HELIX_SCOPE_ID=%s", agent.SessionID),
		fmt.Sprintf("HELIX_SESSION_ID=%s", helixSessionID),
		fmt.Sprintf("HELIX_USER_ID=%s", agent.UserID),
		"SWAY_STOP_ON_APP_EXIT=no",
	}
	extraEnv = append(extraEnv, agent.Env...)

	// Display settings with defaults
	displayWidth := agent.DisplayWidth
	if displayWidth == 0 {
		displayWidth = 3840  // 4K width
	}
	displayHeight := agent.DisplayHeight
	if displayHeight == 0 {
		displayHeight = 2160  // 4K height
	}
	displayRefreshRate := agent.DisplayRefreshRate
	if displayRefreshRate == 0 {
		displayRefreshRate = 60
	}

	// Create Wolf app
	app := createSwayWolfAppForAppsMode(SwayWolfAppConfig{
		WolfAppID:         wolfAppID,
		Title:             fmt.Sprintf("Agent %s", getShortID(agent.SessionID)),
		ContainerHostname: containerHostname,
		UserID:            agent.UserID,
		SessionID:         agent.SessionID,
		WorkspaceDir:      workspaceDir,
		ExtraEnv:          extraEnv,
		DisplayWidth:      displayWidth,
		DisplayHeight:     displayHeight,
		DisplayFPS:        displayRefreshRate,
	}, w.zedImage, w.helixAPIToken)

	// Add app to Wolf
	err = w.wolfClient.AddApp(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("failed to add external agent app to Wolf: %w", err)
	}

	log.Info().
		Str("wolf_app_id", wolfAppID).
		Str("session_id", agent.SessionID).
		Msg("Wolf app created successfully for external agent (apps mode)")

	// Wait for Wolf app to be available in Moonlight API before proceeding
	// For external agents, the title uses getShortID(sessionID)
	err = w.waitForWolfAppInMoonlightAPI(ctx, wolfAppID, fmt.Sprintf("Agent %s", getShortID(agent.SessionID)), 15*time.Second)
	if err != nil {
		// If app doesn't become available, remove it and fail
		w.wolfClient.RemoveApp(ctx, wolfAppID)
		return nil, fmt.Errorf("Wolf app not available in Moonlight API after external agent creation: %w", err)
	}

	// Auto-pair Wolf with moonlight-web before creating session
	// This ensures moonlight-web can connect to Wolf without manual pairing
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080"
	}
	credentials := os.Getenv("MOONLIGHT_CREDENTIALS")
	if credentials == "" {
		credentials = "helix"
	}

	// Attempt pairing using MOONLIGHT_INTERNAL_PAIRING_PIN
	// Wolf will auto-accept when it receives the PIN
	// Type-assert the interface to concrete type for pairing function
	if wolfClient, ok := w.wolfClient.(*wolf.Client); ok {
		if err := ensureWolfPaired(ctx, wolfClient, moonlightWebURL, credentials); err != nil {
			log.Warn().
				Err(err).
				Msg("Auto-pairing failed - Wolf may not be paired with moonlight-web")
			// Don't fail here - pairing might already be done manually
		}
	}

	log.Info().
		Str("wolf_app_id", wolfAppID).
		Str("session_id", agent.SessionID).
		Msg("Wolf app created, attempting keepalive connection with retries")

	// Establish keepalive WebSocket connection with retries for AppNotFound
	// Wolf's Moonlight HTTPS API can lag behind internal API, causing transient AppNotFound
	maxRetries := 5
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = w.connectKeepaliveWebSocketForApp(ctx, wolfAppID, agent.SessionID, displayWidth, displayHeight, displayRefreshRate)
		if err == nil {
			break // Success!
		}

		lastErr = err

		// Only retry on AppNotFound errors (timing issue)
		if !strings.Contains(err.Error(), "AppNotFound") {
			// Different error - fail immediately
			w.wolfClient.RemoveApp(ctx, wolfAppID)
			return nil, fmt.Errorf("failed to create moonlight-web keepalive session: %w", err)
		}

		if attempt < maxRetries {
			waitTime := time.Duration(attempt) * 2 * time.Second // 2s, 4s, 6s, 8s
			log.Warn().
				Err(err).
				Str("wolf_app_id", wolfAppID).
				Int("attempt", attempt).
				Int("max_retries", maxRetries).
				Dur("retry_in", waitTime).
				Msg("Keepalive connection failed with AppNotFound, retrying...")
			time.Sleep(waitTime)
		}
	}

	if lastErr != nil {
		// All retries exhausted
		w.wolfClient.RemoveApp(ctx, wolfAppID)
		return nil, fmt.Errorf("failed to create moonlight-web keepalive session after %d attempts: %w", maxRetries, lastErr)
	}

	log.Info().
		Str("wolf_app_id", wolfAppID).
		Str("session_id", agent.SessionID).
		Msg("Moonlight-web keepalive session established successfully for external agent (apps mode)")

	// Track session (simple - no lobbies, no keepalive)
	session := &ZedSession{
		SessionID:      agent.SessionID,
		HelixSessionID: helixSessionID,
		UserID:         agent.UserID,
		Status:         "starting",
		StartTime:      time.Now(),
		LastAccess:     time.Now(),
		ProjectPath:    agent.ProjectPath,
		WolfAppID:      wolfAppID,
		ContainerName:  containerHostname,
	}
	w.sessions[agent.SessionID] = session

	// Return response
	response := &types.ZedAgentResponse{
		SessionID:     agent.SessionID,
		ScreenshotURL: fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
		StreamURL:     fmt.Sprintf("moonlight://localhost:47989"),
		Status:        "starting",
		ContainerName: containerHostname,
		WolfAppID:     wolfAppID,
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("wolf_app_id", wolfAppID).
		Msg("External Zed agent started successfully (apps mode)")

	return response, nil
}

// StopZedAgent implements the Executor interface (apps model)
func (w *AppWolfExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().Str("session_id", sessionID).Msg("Stopping Zed agent via Wolf (apps mode)")

	session, exists := w.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Remove app from Wolf (tears down container)
	// In apps mode with moonlight-web persistence, the container lifecycle is managed by Wolf
	// The moonlight-web session will be cleaned up automatically when the app is removed
	if session.WolfAppID != "" {
		err := w.wolfClient.RemoveApp(ctx, session.WolfAppID)
		if err != nil {
			log.Error().Err(err).Str("wolf_app_id", session.WolfAppID).Msg("Failed to remove Wolf app")
		}
	}

	// Update session status
	session.Status = "stopped"
	delete(w.sessions, sessionID)

	log.Info().Str("session_id", sessionID).Msg("Zed agent stopped successfully (apps mode)")

	return nil
}

// GetSession implements the Executor interface
func (w *AppWolfExecutor) GetSession(sessionID string) (*ZedSession, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	session, exists := w.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	session.LastAccess = time.Now()
	return session, nil
}

// CleanupExpiredSessions implements the Executor interface
func (w *AppWolfExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	cutoff := time.Now().Add(-timeout)
	var expiredSessions []string

	for sessionID, session := range w.sessions {
		if session.LastAccess.Before(cutoff) {
			expiredSessions = append(expiredSessions, sessionID)
		}
	}

	for _, sessionID := range expiredSessions {
		log.Info().
			Str("session_id", sessionID).
			Dur("timeout", timeout).
			Msg("Cleaning up expired Zed session (apps mode)")

		err := w.StopZedAgent(ctx, sessionID)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to stop expired session")
		}
	}
}

// ListSessions implements the Executor interface
func (w *AppWolfExecutor) ListSessions() []*ZedSession {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	sessions := make([]*ZedSession, 0, len(w.sessions))
	for _, session := range w.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// Multi-session methods (delegate to single-session for now)
func (w *AppWolfExecutor) StartZedInstance(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	return w.StartZedAgent(ctx, agent)
}

func (w *AppWolfExecutor) CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error {
	return fmt.Errorf("multi-threading not yet implemented in Wolf executor (apps mode)")
}

func (w *AppWolfExecutor) StopZedInstance(ctx context.Context, instanceID string) error {
	return w.StopZedAgent(ctx, instanceID)
}

func (w *AppWolfExecutor) GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error) {
	session, err := w.GetSession(instanceID)
	if err != nil {
		return nil, err
	}

	return &ZedInstanceStatus{
		InstanceID:    instanceID,
		Status:        session.Status,
		ThreadCount:   1,
		ActiveThreads: 1,
		LastActivity:  &session.LastAccess,
		ProjectPath:   session.ProjectPath,
	}, nil
}

func (w *AppWolfExecutor) ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error) {
	session, err := w.GetSession(instanceID)
	if err != nil {
		return nil, err
	}

	return []*ZedThreadInfo{
		{
			ThreadID:      instanceID,
			WorkSessionID: session.SessionID,
			Status:        session.Status,
			CreatedAt:     session.StartTime,
			LastActivity:  &session.LastAccess,
			Config:        map[string]interface{}{},
		},
	}, nil
}
func (w *AppWolfExecutor) FindContainerBySessionID(ctx context.Context, helixSessionID string) (string, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	for _, session := range w.sessions {
		if session.HelixSessionID == helixSessionID {
			return session.ContainerName, nil
		}
	}

	return "", fmt.Errorf("no external agent session found with Helix session ID: %s", helixSessionID)
}

// connectKeepaliveWebSocketForApp switches between single and multi modes based on MOONLIGHT_WEB_MODE env var
func (w *AppWolfExecutor) connectKeepaliveWebSocketForApp(ctx context.Context, wolfAppID, sessionID string, displayWidth, displayHeight, displayFPS int) error {
	moonlightMode := os.Getenv("MOONLIGHT_WEB_MODE")
	if moonlightMode == "" {
		moonlightMode = "single" // Default to single mode (session-persistence)
	}

	if moonlightMode == "multi" {
		// Multi-WebRTC mode: Use streamers REST API
		return w.connectKeepaliveWebSocketForAppMulti(ctx, wolfAppID, sessionID, displayWidth, displayHeight, displayFPS)
	} else {
		// Single mode: Use WebSocket with keepalive
		return w.connectKeepaliveWebSocketForAppSingle(ctx, wolfAppID, sessionID, displayWidth, displayHeight, displayFPS)
	}
}

// connectKeepaliveWebSocketForAppMulti creates persistent streamer via REST API (multi-WebRTC architecture)
// This creates a Moonlight stream that persists independently of WebRTC peer connections
func (w *AppWolfExecutor) connectKeepaliveWebSocketForAppMulti(ctx context.Context, wolfAppID, sessionID string, displayWidth, displayHeight, displayFPS int) error {
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080" // Default internal URL
	}

	// Parse wolfAppID to uint32
	appIDUint, err := strconv.ParseUint(wolfAppID, 10, 32)
	if err != nil {
		return fmt.Errorf("failed to parse wolf app ID %s: %w", wolfAppID, err)
	}

	// Create streamer via REST API
	streamerID := fmt.Sprintf("agent-%s", sessionID)
	createReq := map[string]interface{}{
		"streamer_id":             streamerID,
		"client_unique_id":        fmt.Sprintf("helix-agent-%s", sessionID), // Unique client ID per agent
		"host_id":                 0,                                        // Local Wolf instance
		"app_id":                  uint32(appIDUint),
		"bitrate":                 20000,
		"packet_size":             1024,
		"fps":                     displayFPS,
		"width":                   displayWidth,
		"height":                  displayHeight,
		"video_sample_queue_size": 10,
		"play_audio_local":        false,
		"audio_sample_queue_size": 10,
		"video_supported_formats": 1,        // H264 only
		"video_colorspace":        "Rec709",
		"video_color_range_full":  false,
	}

	createJSON, err := json.Marshal(createReq)
	if err != nil {
		return fmt.Errorf("failed to marshal create streamer request: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("streamer_id", streamerID).
		Str("wolf_app_id", wolfAppID).
		Str("url", moonlightWebURL+"/api/streamers").
		Msg("🚀 [Helix] Creating persistent streamer via REST API")

	log.Info().
		Str("request_body", string(createJSON)).
		Msg("🚀 [Helix] POST /api/streamers request body")

	// POST /api/streamers
	httpClient := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", moonlightWebURL+"/api/streamers", strings.NewReader(string(createJSON)))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("MOONLIGHT_CREDENTIALS"))

	log.Info().
		Str("authorization", "Bearer "+os.Getenv("MOONLIGHT_CREDENTIALS")).
		Msg("🚀 [Helix] Sending POST request to moonlight-web...")

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Error().
			Err(err).
			Str("url", moonlightWebURL+"/api/streamers").
			Msg("🚀 [Helix] POST /api/streamers request FAILED")
		return fmt.Errorf("failed to POST /api/streamers: %w", err)
	}
	defer resp.Body.Close()

	log.Info().
		Int("status_code", resp.StatusCode).
		Msg("🚀 [Helix] Got response from POST /api/streamers")

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("🚀 [Helix] POST /api/streamers returned non-200 status")
		return fmt.Errorf("POST /api/streamers failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	body, _ := io.ReadAll(resp.Body)
	log.Info().
		Str("response_body", string(body)).
		Msg("🚀 [Helix] POST /api/streamers response body")

	var streamerInfo map[string]interface{}
	if err := json.Unmarshal(body, &streamerInfo); err != nil {
		log.Error().
			Err(err).
			Str("body", string(body)).
			Msg("🚀 [Helix] Failed to parse streamer response")
		return fmt.Errorf("failed to parse streamer response: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("streamer_id", streamerID).
		Interface("streamer_info", streamerInfo).
		Msg("✅ [Helix] Persistent streamer created successfully!")

	return nil
}

// connectKeepaliveWebSocketForAppSingle creates session via WebSocket (single/session-persistence architecture)
// This establishes a persistent Moonlight session that supports browser join/leave cycles
func (w *AppWolfExecutor) connectKeepaliveWebSocketForAppSingle(ctx context.Context, wolfAppID, sessionID string, displayWidth, displayHeight, displayFPS int) error {
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080" // Default internal URL
	}

	// Build WebSocket URL (moonlight-web expects /api/host/stream endpoint)
	wsURL := strings.Replace(moonlightWebURL, "http://", "ws://", 1) + "/api/host/stream"

	log.Info().
		Str("session_id", sessionID).
		Str("wolf_app_id", wolfAppID).
		Str("ws_url", wsURL).
		Msg("Connecting keepalive WebSocket to moonlight-web for apps mode (single/session-persistence)")

	// Connect WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}
	defer conn.Close()

	// Parse wolfAppID string to uint32 for moonlight-web API
	appIDUint, err := strconv.ParseUint(wolfAppID, 10, 32)
	if err != nil {
		return fmt.Errorf("failed to parse wolf app ID %s: %w", wolfAppID, err)
	}

	// Send AuthenticateAndInit message with session persistence
	// mode=keepalive: creates session without WebRTC peer (headless)
	// KICKOFF APPROACH: Use separate session_id for kickoff vs browser
	// - Kickoff: "agent-{sessionID}-kickoff" → Will terminate after 10s
	// - Browser: "agent-{sessionID}" → Fresh session/streamer, but same client_unique_id → auto-RESUME!
	authMsg := map[string]interface{}{
		"AuthenticateAndInit": map[string]interface{}{
			"credentials":             os.Getenv("MOONLIGHT_CREDENTIALS"),                      // Use MOONLIGHT_CREDENTIALS for auth
			"session_id":              fmt.Sprintf("agent-%s-kickoff", sessionID),              // KICKOFF session ID (separate from browser)
			"mode":                    "keepalive",                                             // Keepalive mode (no WebRTC)
			"client_unique_id":        fmt.Sprintf("helix-agent-%s", sessionID),                // SAME client ID as browser → enables RESUME
			"host_id":                 0,                                                       // Local Wolf instance
			"app_id":                  uint32(appIDUint),                                       // Connect to the Wolf app (u32)
			"bitrate":                 20000,                                                   // Match agent display settings
			"packet_size":             1024,
			"fps":                     displayFPS,     // Use agent's configured FPS
			"width":                   displayWidth,   // Use agent's configured width
			"height":                  displayHeight,  // Use agent's configured height
			"video_sample_queue_size": 10,
			"play_audio_local":        false,
			"audio_sample_queue_size": 10,
			"video_supported_formats": 1,        // H264 only
			"video_colorspace":        "Rec709", // String format for new API
			"video_color_range_full":  false,
		},
	}

	authJSON, err := json.Marshal(authMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal auth message: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, authJSON); err != nil {
		return fmt.Errorf("failed to send auth message: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("wolf_app_id", wolfAppID).
		Msg("Sent keepalive auth message to moonlight-web with session persistence (single mode)")

	// KICKOFF APPROACH: Wait for Zed to connect via WebSocket (smarter than fixed 10 seconds)
	// This triggers Wolf to launch the app, making it "resumable" for this client
	// When browser connects later, it will RESUME this app instead of creating new session
	log.Info().
		Str("session_id", sessionID).
		Msg("Waiting for Zed instance to connect via WebSocket (kickoff approach)...")

	// Wait for Zed WebSocket connection with 60-second timeout
	if err := w.waitForZedConnection(ctx, sessionID, 60*time.Second); err != nil {
		log.Warn().
			Err(err).
			Str("session_id", sessionID).
			Msg("Zed WebSocket connection not detected, but continuing with kickoff (may work anyway)")
		// Don't fail - container might still be starting, just log the warning
	}

	// Disconnect cleanly - app is now resumable for this client
	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "kickoff complete")
	conn.WriteMessage(websocket.CloseMessage, closeMsg)

	log.Info().
		Str("session_id", sessionID).
		Str("wolf_app_id", wolfAppID).
		Msg("Kickoff complete - app is now resumable for this client (Moonlight certificate reused)")

	return nil
}

// GetWolfClient returns the Wolf client for direct access to Wolf API
// Note: This type-asserts the interface back to the concrete type.
// Only use this when you need direct access to wolf.Client specific methods.
func (w *AppWolfExecutor) GetWolfClient() *wolf.Client {
	if client, ok := w.wolfClient.(*wolf.Client); ok {
		return client
	}
	// This should never happen in production, only in tests with mocks
	log.Warn().Msg("GetWolfClient called but wolfClient is not *wolf.Client (likely a test mock)")
	return nil
}

// waitForZedConnection waits for the Zed instance to connect via WebSocket
// This replaces the fixed 10-second sleep with a smarter wait based on actual connection
func (w *AppWolfExecutor) waitForZedConnection(ctx context.Context, sessionID string, timeout time.Duration) error {
	if w.wsChecker == nil {
		log.Warn().
			Str("session_id", sessionID).
			Msg("WebSocket checker not available, cannot wait for connection")
		return fmt.Errorf("WebSocket checker not configured")
	}

	log.Info().
		Str("session_id", sessionID).
		Dur("timeout", timeout).
		Msg("Waiting for Zed WebSocket connection")

	startTime := time.Now()
	checkInterval := 500 * time.Millisecond

	for time.Since(startTime) < timeout {
		// Check if Zed has connected via WebSocket
		if w.wsChecker.IsExternalAgentConnected(sessionID) {
			elapsed := time.Since(startTime)
			log.Info().
				Str("session_id", sessionID).
				Dur("elapsed", elapsed).
				Msg("✅ Zed WebSocket connection detected - kickoff ready!")
			return nil
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(checkInterval):
			// Continue checking
		}

		// Log progress every 5 seconds
		elapsed := time.Since(startTime)
		if int(elapsed.Seconds())%5 == 0 && elapsed > 0 {
			timeLeft := timeout - elapsed
			log.Debug().
				Str("session_id", sessionID).
				Dur("elapsed", elapsed).
				Dur("time_left", timeLeft).
				Msg("Still waiting for Zed WebSocket connection...")
		}
	}

	elapsed := time.Since(startTime)
	return fmt.Errorf("timeout waiting for Zed WebSocket connection after %v", elapsed)
}

// waitForWolfAppInMoonlightAPI waits for Wolf app to be available via HTTPS Moonlight API
// This is what moonlight-web actually queries, ensuring no AppNotFound errors
func (w *AppWolfExecutor) waitForWolfAppInMoonlightAPI(ctx context.Context, wolfAppID, expectedTitle string, timeout time.Duration) error {
	log.Info().
		Str("wolf_app_id", wolfAppID).
		Str("expected_title", expectedTitle).
		Dur("timeout", timeout).
		Msg("Waiting for Wolf app in Moonlight HTTPS API")

	startTime := time.Now()
	checkInterval := 500 * time.Millisecond

	for time.Since(startTime) < timeout {
		// Check internal API first (fast path - ensures app exists somewhere)
		apps, err := w.wolfClient.ListApps(ctx)
		if err != nil {
			log.Warn().
				Err(err).
				Str("wolf_app_id", wolfAppID).
				Msg("Failed to list Wolf apps from internal API, will retry")
			time.Sleep(checkInterval)
			continue
		}

		// Check if our app is in internal API
		foundInInternal := false
		for _, app := range apps {
			if app.ID == wolfAppID || app.Title == expectedTitle {
				foundInInternal = true
				break
			}
		}

		if !foundInInternal {
			log.Debug().
				Str("wolf_app_id", wolfAppID).
				Msg("Wolf app not yet in internal API, waiting...")
			time.Sleep(checkInterval)
			continue
		}

		// App in internal API - now ACTUALLY verify it's in Moonlight HTTPS API
		// Query Wolf's HTTPS server on port 47984 (what moonlight-web actually uses)
		// Use HTTP client that accepts self-signed certs (Wolf uses self-signed)
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient := &http.Client{
			Timeout:   2 * time.Second,
			Transport: tr,
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "https://wolf:47984/applist?uniqueid=helix&uuid=test", nil)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create HTTP request")
			time.Sleep(checkInterval)
			continue
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Debug().
				Err(err).
				Str("wolf_app_id", wolfAppID).
				Dur("elapsed", time.Since(startTime)).
				Msg("Failed to query Wolf Moonlight HTTPS API, will retry")
			time.Sleep(checkInterval)
			continue
		}

		// Read response body to check if app is in the list
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Warn().
				Err(err).
				Msg("Failed to read applist response")
			time.Sleep(checkInterval)
			continue
		}

		// Check if our app ID appears in the XML response
		if strings.Contains(string(body), fmt.Sprintf("<ID>%s</ID>", wolfAppID)) {
			log.Info().
				Str("wolf_app_id", wolfAppID).
				Dur("elapsed", time.Since(startTime)).
				Msg("Wolf app NOW available in Moonlight HTTPS API")
			return nil
		}

		// Debug: Log what apps Wolf's Moonlight API actually returned
		appCount := strings.Count(string(body), "<App>")
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200]
		}
		log.Debug().
			Str("wolf_app_id", wolfAppID).
			Dur("elapsed", time.Since(startTime)).
			Int("internal_apps", len(apps)).
			Int("https_apps", appCount).
			Str("https_response_preview", preview).
			Msg("Wolf app in internal API but NOT yet in Moonlight HTTPS API, waiting...")

		time.Sleep(checkInterval)
	}

	return fmt.Errorf("Wolf app %s (title: %s) not available in Moonlight API after %v", wolfAppID, expectedTitle, timeout)
}

// Helper functions shared between apps and lobbies executors

// getShortID returns last 4 characters of an ID for compact display names
func getShortID(id string) string {
	if len(id) <= 4 {
		return id
	}
	return id[len(id)-4:]
}

func generateWolfAppID(userID, environmentName string) string {
	stableKey := fmt.Sprintf("%s-%s", userID, environmentName)
	var numericHash uint64
	for _, b := range []byte(stableKey) {
		numericHash = numericHash*31 + uint64(b)
	}
	return fmt.Sprintf("%d", numericHash%1000000000)
}

func createSwayConfig(instanceID string) error {
	swayConfigPath := fmt.Sprintf("/tmp/sway-config-%s", instanceID)
	swayConfig := `# Sway configuration for Helix
set $mod Mod4
font pango:Monospace 8
floating_modifier $mod
bindsym $mod+Return exec kitty
bindsym $mod+Shift+q kill
bindsym $mod+d exec fuzzel
bindsym $mod+f fullscreen toggle
exec kitty --working-directory=/home/user/work
exec --no-startup-id swaybg -c "#2e3440"
for_window [app_id="kitty"] focus
for_window [app_id="zed"] focus
output * {
    mode 1920x1080@60Hz
    pos 0 0
}
input * {
    xkb_layout "us"
}
`
	return os.WriteFile(swayConfigPath, []byte(swayConfig), 0644)
}

func createSwayWolfAppForAppsMode(config SwayWolfAppConfig, zedImage, helixAPIToken string) *wolf.App {
	env := []string{
		"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia* /dev/dma_heap/system",
		"RUN_SWAY=1",
		"WLR_BACKENDS=drm", // Force wlroots to use DRM backend for headless NVIDIA GPU operation
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", os.Getenv("ANTHROPIC_API_KEY")),
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		"ZED_HELIX_URL=api:8080",
		fmt.Sprintf("ZED_HELIX_TOKEN=%s", helixAPIToken),
		"ZED_HELIX_TLS=false",
		"RUST_LOG=info",
		fmt.Sprintf("HELIX_SESSION_ID=%s", config.SessionID),
		"HELIX_API_URL=http://api:8080",
		fmt.Sprintf("HELIX_API_TOKEN=%s", helixAPIToken),
		"SETTINGS_SYNC_PORT=9877",
	}
	env = append(env, config.ExtraEnv...)

	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", config.WorkspaceDir),
		"/var/run/docker.sock:/var/run/docker.sock",
	}

	// Development mode: bind-mount Zed build and startup scripts from host
	// Production mode: these are baked into the ZED_IMAGE
	if os.Getenv("HELIX_DEV_MODE") == "true" {
		helixHostHome := os.Getenv("HELIX_HOST_HOME")
		log.Info().
			Str("helix_host_home", helixHostHome).
			Msg("HELIX_DEV_MODE enabled - mounting dev files from host for hot-reloading")

		mounts = append(mounts,
			fmt.Sprintf("%s/zed-build:/zed-build:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/config:/cfg/sway/custom-cfg:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/start-zed-helix.sh:/usr/local/bin/start-zed-helix.sh:ro", helixHostHome),
		)
	} else {
		log.Debug().Msg("Production mode - using files baked into helix-sway image")
	}

	// Add SSH keys if available
	sshKeyDir := fmt.Sprintf("/opt/helix/filestore/ssh-keys/%s", config.UserID)
	if _, err := os.Stat(sshKeyDir); err == nil {
		mounts = append(mounts, fmt.Sprintf("%s:/home/retro/.ssh:ro", sshKeyDir))
	}

	baseCreateJSON := fmt.Sprintf(`{
  "Hostname": "%s",
  "HostConfig": {
    "IpcMode": "host",
    "NetworkMode": "helix_default",
    "Privileged": false,
    "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
    "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw", "c 249:* rwm"]
  }
}`, config.ContainerHostname)

	return wolf.NewMinimalDockerApp(
		config.WolfAppID,
		config.Title,
		config.ContainerHostname,
		zedImage,
		env,
		mounts,
		baseCreateJSON,
		config.DisplayWidth,
		config.DisplayHeight,
		config.DisplayFPS,
	)
}

// ensureWolfPaired ensures Wolf is paired with moonlight-web using auto-pairing PIN
// Based on moonlight_web_pairing.go approach but simplified for runtime usage
func ensureWolfPaired(ctx context.Context, wolfClient *wolf.Client, moonlightWebURL, credentials string) error {
	log.Info().Msg("🔗 Checking if Wolf is paired with moonlight-web")

	// Since Wolf has MOONLIGHT_INTERNAL_PAIRING_PIN set, it will auto-accept pairing
	// We just need to trigger moonlight-web to initiate pairing with Wolf
	// Wolf will automatically fulfill the pairing without waiting for us to submit PIN

	// Step 1: Trigger pairing from moonlight-web to Wolf
	url := fmt.Sprintf("%s/api/pair", moonlightWebURL)
	reqBody := map[string]interface{}{
		"host_id": 0, // Wolf is host 0
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credentials) // Use Bearer, not Basic Auth!

	log.Info().
		Str("url", url).
		Msg("Triggering Wolf pairing in moonlight-web (auto-PIN enabled in Wolf)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call moonlight-web /api/pair: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("moonlight-web pairing failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Read PIN from NDJSON stream (first JSON object)
	var pinResponse struct {
		Pin string `json:"Pin"`
	}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&pinResponse); err != nil {
		return fmt.Errorf("could not read PIN from stream: %w", err)
	}

	log.Info().
		Str("pin", pinResponse.Pin).
		Msg("moonlight-web generated PIN - Wolf should auto-accept via MOONLIGHT_INTERNAL_PAIRING_PIN")

	// Read rest of stream to completion (Wolf auto-accepts, should return "Paired")
	finalResult, _ := io.ReadAll(resp.Body)
	log.Info().
		Str("final_response", string(finalResult)).
		Msg("✅ Pairing stream completed")

	return nil
}
