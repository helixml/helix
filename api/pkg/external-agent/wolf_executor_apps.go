package external_agent

import (
	"bytes"
	"context"
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

// AppWolfExecutor implements the Executor interface using Wolf Apps API (stable branch)
// This is the simpler, more reliable approach without lobbies
type AppWolfExecutor struct {
	wolfClient        *wolf.Client
	healthMonitor     *wolf.HealthMonitor
	store             store.Store
	sessions          map[string]*ZedSession
	mutex             sync.RWMutex
	zedImage          string
	helixAPIURL       string
	helixAPIToken     string
	workspaceBasePath string
}

// NewAppWolfExecutor creates a new app-based Wolf executor
func NewAppWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, store store.Store) *AppWolfExecutor {
	wolfClient := wolf.NewClient(wolfSocketPath)

	executor := &AppWolfExecutor{
		wolfClient:        wolfClient,
		store:             store,
		sessions:          make(map[string]*ZedSession),
		zedImage:          zedImage,
		helixAPIURL:       helixAPIURL,
		helixAPIToken:     helixAPIToken,
		workspaceBasePath: "/opt/helix/filestore/workspaces",
	}

	// Create health monitor for Wolf crashes
	executor.healthMonitor = wolf.NewHealthMonitor(wolfClient, func(ctx context.Context) {
		log.Info().Msg("Wolf restarted, apps will need to be re-added")
		// Apps-based model: apps are lost on Wolf restart, need to be recreated
		// Reconciliation will handle this
	})

	executor.healthMonitor.Start(context.Background())

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
		displayWidth = 1920
	}
	displayHeight := agent.DisplayHeight
	if displayHeight == 0 {
		displayHeight = 1080
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

	// Wait for app to appear in internal API first
	apps, err := w.wolfClient.ListApps(ctx)
	if err == nil {
		found := false
		for _, app := range apps {
			if app.ID == wolfAppID {
				found = true
				break
			}
		}
		if !found {
			time.Sleep(2 * time.Second) // Brief wait if not immediately available
		}
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
	if err := ensureWolfPaired(ctx, w.wolfClient, moonlightWebURL, credentials); err != nil {
		log.Warn().
			Err(err).
			Msg("Auto-pairing failed - Wolf may not be paired with moonlight-web")
		// Don't fail here - pairing might already be done manually
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

// Personal Dev Environment Management (apps mode)
func (w *AppWolfExecutor) CreatePersonalDevEnvironment(ctx context.Context, userID, appID, environmentName string) (*ZedInstanceInfo, error) {
	return w.CreatePersonalDevEnvironmentWithDisplay(ctx, userID, appID, environmentName, 1920, 1080, 60)
}

func (w *AppWolfExecutor) CreatePersonalDevEnvironmentWithDisplay(ctx context.Context, userID, appID, environmentName string, displayWidth, displayHeight, displayFPS int) (*ZedInstanceInfo, error) {
	if err := validateDisplayParams(displayWidth, displayHeight, displayFPS); err != nil {
		return nil, fmt.Errorf("invalid display configuration: %w", err)
	}

	wolfAppID := generateWolfAppID(userID, environmentName)
	timestamp := time.Now().Unix()
	instanceID := fmt.Sprintf("personal-dev-%s-%d", userID, timestamp)

	log.Info().
		Str("instance_id", instanceID).
		Str("user_id", userID).
		Str("environment_name", environmentName).
		Msg("Creating personal development environment via Wolf (apps mode)")

	// Create workspace directory
	workspaceDir, err := createWorkspaceDirectory(instanceID, w.workspaceBasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Create Sway config
	err = createSwayConfig(instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Sway config: %w", err)
	}

	containerHostname := fmt.Sprintf("personal-dev-%s", wolfAppID)
	extraEnv := []string{"HELIX_STARTUP_SCRIPT=/home/retro/work/startup.sh"}

	// Create Wolf app (no lobbies, no PINs - simpler!)
	app := createSwayWolfAppForAppsMode(SwayWolfAppConfig{
		WolfAppID:         wolfAppID,
		Title:             fmt.Sprintf("Personal Dev Environment %s", environmentName),
		ContainerHostname: containerHostname,
		UserID:            userID,
		SessionID:         instanceID,
		WorkspaceDir:      workspaceDir,
		ExtraEnv:          extraEnv,
		DisplayWidth:      displayWidth,
		DisplayHeight:     displayHeight,
		DisplayFPS:        displayFPS,
	}, w.zedImage, w.helixAPIToken)

	// Add app to Wolf
	err = w.wolfClient.AddApp(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("failed to add personal dev app to Wolf: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Str("wolf_app_id", wolfAppID).
		Msg("Wolf app created for PDE (apps mode)")

	// Wait for Wolf app to be available in Moonlight API before proceeding
	// For PDEs, the title uses environmentName not instanceID
	err = w.waitForWolfAppInMoonlightAPI(ctx, wolfAppID, fmt.Sprintf("Personal Dev Environment %s", environmentName), 15*time.Second)
	if err != nil {
		// If app doesn't become available, remove it and fail
		w.wolfClient.RemoveApp(ctx, wolfAppID)
		return nil, fmt.Errorf("Wolf app not available in Moonlight API after PDE creation: %w", err)
	}

	// Save to database
	pde := &types.PersonalDevEnvironment{
		ID:              instanceID,
		UserID:          userID,
		AppID:           appID,
		WolfAppID:       wolfAppID,
		EnvironmentName: environmentName,
		Status:          "starting",
		LastActivity:    time.Now(),
		DisplayWidth:    displayWidth,
		DisplayHeight:   displayHeight,
		DisplayFPS:      displayFPS,
		ContainerName:   containerHostname,
		VNCPort:         5901,
		StreamURL:       fmt.Sprintf("moonlight://localhost:47989"),
	}

	pde, err = w.store.CreatePersonalDevEnvironment(ctx, pde)
	if err != nil {
		return nil, fmt.Errorf("failed to save personal dev environment to database: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Str("wolf_app_id", wolfAppID).
		Msg("Personal development environment created successfully (apps mode)")

	return &ZedInstanceInfo{
		InstanceID:      pde.ID,
		UserID:          pde.UserID,
		AppID:           pde.WolfAppID,
		InstanceType:    "personal_dev",
		Status:          pde.Status,
		CreatedAt:       pde.Created,
		LastActivity:    pde.LastActivity,
		ProjectPath:     fmt.Sprintf("/workspace/%s", environmentName),
		ThreadCount:     1,
		IsPersonalEnv:   true,
		EnvironmentName: pde.EnvironmentName,
		StreamURL:       pde.StreamURL,
		DisplayWidth:    pde.DisplayWidth,
		DisplayHeight:   pde.DisplayHeight,
		DisplayFPS:      pde.DisplayFPS,
		ContainerName:   pde.ContainerName,
		VNCPort:         pde.VNCPort,
	}, nil
}

func (w *AppWolfExecutor) GetPersonalDevEnvironments(ctx context.Context, userID string) ([]*ZedInstanceInfo, error) {
	pdes, err := w.store.ListPersonalDevEnvironments(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list personal dev environments: %w", err)
	}

	var personalEnvs []*ZedInstanceInfo
	for _, pde := range pdes {
		personalEnvs = append(personalEnvs, &ZedInstanceInfo{
			InstanceID:      pde.ID,
			UserID:          pde.UserID,
			AppID:           pde.WolfAppID,
			InstanceType:    "personal_dev",
			Status:          pde.Status,
			CreatedAt:       pde.Created,
			LastActivity:    pde.LastActivity,
			ProjectPath:     fmt.Sprintf("/workspace/%s", pde.EnvironmentName),
			IsPersonalEnv:   true,
			EnvironmentName: pde.EnvironmentName,
			StreamURL:       pde.StreamURL,
			DisplayWidth:    pde.DisplayWidth,
			DisplayHeight:   pde.DisplayHeight,
			DisplayFPS:      pde.DisplayFPS,
			ContainerName:   pde.ContainerName,
		})
	}

	return personalEnvs, nil
}

func (w *AppWolfExecutor) StopPersonalDevEnvironment(ctx context.Context, userID, instanceID string) error {
	pde, err := w.store.GetPersonalDevEnvironment(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("personal dev environment %s not found", instanceID)
	}

	if pde.UserID != userID {
		return fmt.Errorf("access denied: environment belongs to different user")
	}

	log.Info().Str("instance_id", instanceID).Msg("Stopping personal dev environment (apps mode)")

	// Remove Wolf app
	if pde.WolfAppID != "" {
		err := w.wolfClient.RemoveApp(ctx, pde.WolfAppID)
		if err != nil {
			log.Error().Err(err).Str("wolf_app_id", pde.WolfAppID).Msg("Failed to remove Wolf app")
		}
	}

	// Clean up Sway config
	swayConfigPath := fmt.Sprintf("/tmp/sway-config-%s", instanceID)
	os.Remove(swayConfigPath)

	// Delete from database
	err = w.store.DeletePersonalDevEnvironment(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("failed to delete environment from database: %w", err)
	}

	return nil
}

func (w *AppWolfExecutor) GetPersonalDevEnvironment(ctx context.Context, userID, environmentID string) (*ZedInstanceInfo, error) {
	pde, err := w.store.GetPersonalDevEnvironment(ctx, environmentID)
	if err != nil {
		return nil, fmt.Errorf("environment not found: %s", environmentID)
	}

	if pde.UserID != userID {
		return nil, fmt.Errorf("access denied: environment belongs to different user")
	}

	return &ZedInstanceInfo{
		InstanceID:      pde.ID,
		UserID:          pde.UserID,
		AppID:           pde.WolfAppID,
		InstanceType:    "personal_dev",
		Status:          pde.Status,
		CreatedAt:       pde.Created,
		LastActivity:    pde.LastActivity,
		ProjectPath:     fmt.Sprintf("/workspace/%s", pde.EnvironmentName),
		IsPersonalEnv:   true,
		EnvironmentName: pde.EnvironmentName,
		StreamURL:       pde.StreamURL,
		DisplayWidth:    pde.DisplayWidth,
		DisplayHeight:   pde.DisplayHeight,
		DisplayFPS:      pde.DisplayFPS,
		ContainerName:   pde.ContainerName,
	}, nil
}

func (w *AppWolfExecutor) ReconcilePersonalDevEnvironments(ctx context.Context) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().Msg("Starting personal dev environment reconciliation (apps mode)")

	// Get all PDEs from database
	allPDEs, err := w.store.ListPersonalDevEnvironments(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to list personal dev environments: %w", err)
	}

	// Get all Wolf apps
	wolfApps, err := w.wolfClient.ListApps(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Wolf apps: %w", err)
	}

	// Build set of expected app IDs
	expectedAppIDs := make(map[string]bool)
	for _, pde := range allPDEs {
		if pde.Status == "running" && pde.WolfAppID != "" {
			expectedAppIDs[pde.WolfAppID] = true
		}
	}

	// Remove orphaned apps
	deletedCount := 0
	for _, app := range wolfApps {
		if !expectedAppIDs[app.ID] {
			log.Info().Str("app_id", app.ID).Msg("Found orphaned Wolf app, removing")
			err := w.wolfClient.RemoveApp(ctx, app.ID)
			if err != nil {
				log.Error().Err(err).Str("app_id", app.ID).Msg("Failed to remove orphaned app")
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		log.Info().Int("deleted_count", deletedCount).Msg("Deleted orphaned Wolf apps")
	}

	return nil
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

	// KICKOFF APPROACH: Wait 10 seconds for Wolf to start the container, then disconnect
	// This triggers Wolf to launch the app, making it "resumable" for this client
	// When browser connects later, it will RESUME this app instead of creating new session
	log.Info().
		Str("session_id", sessionID).
		Msg("Waiting 10 seconds for Wolf container to start (kickoff approach)...")

	time.Sleep(10 * time.Second)

	// Disconnect cleanly - app is now resumable for this client
	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "kickoff complete")
	conn.WriteMessage(websocket.CloseMessage, closeMsg)

	log.Info().
		Str("session_id", sessionID).
		Str("wolf_app_id", wolfAppID).
		Msg("Kickoff complete - app is now resumable for this client (Moonlight certificate reused)")

	return nil
}

func (w *AppWolfExecutor) GetWolfClient() *wolf.Client {
	return w.wolfClient
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
		// Query Wolf's HTTP server on port 47989 (what moonlight-web uses)
		httpClient := &http.Client{Timeout: 2 * time.Second}
		req, err := http.NewRequestWithContext(ctx, "GET", "http://wolf:47989/applist?uniqueid=helix&uuid=test", nil)
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
				Msg("Failed to query Wolf Moonlight HTTP API, will retry")
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
			Int("http_apps", appCount).
			Str("http_response_preview", preview).
			Msg("Wolf app in internal API but NOT yet in Moonlight HTTP API, waiting...")

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

func createWorkspaceDirectory(instanceID, basePath string) (string, error) {
	workspaceDir := filepath.Join(basePath, instanceID)
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", err
	}

	// Create startup script
	startupScriptPath := filepath.Join(workspaceDir, "startup.sh")
	if _, err := os.Stat(startupScriptPath); os.IsNotExist(err) {
		startupScript := `#!/bin/bash
echo "Starting up workspace"
sudo chown -R retro:retro ~/work
`
		if err := os.WriteFile(startupScriptPath, []byte(startupScript), 0755); err != nil {
			return "", err
		}
	}

	return workspaceDir, nil
}

func createSwayWolfAppForAppsMode(config SwayWolfAppConfig, zedImage, helixAPIToken string) *wolf.App {
	env := []string{
		"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
		"RUN_SWAY=1",
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
	helixHostHome := os.Getenv("HELIX_HOST_HOME")
	if helixHostHome != "" {
		mounts = append(mounts,
			fmt.Sprintf("%s/zed-build:/zed-build:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/start-zed-helix.sh:/usr/local/bin/start-zed-helix.sh:ro", helixHostHome),
		)
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
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"]
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
