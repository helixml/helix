package external_agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
)

// WolfExecutor implements the Executor interface using Wolf API
type WolfExecutor struct {
	wolfClient    *wolf.Client
	healthMonitor *wolf.HealthMonitor
	store         store.Store
	sessions      map[string]*ZedSession
	mutex         sync.RWMutex

	// Zed configuration
	zedImage      string
	helixAPIURL   string
	helixAPIToken string

	// Workspace configuration for dev stack
	workspaceBasePath string
}

// generateWolfAppID creates a consistent, numeric Wolf-compatible app ID
// Uses user ID and environment name to ensure the same environment always gets the same ID
// Wolf expects numeric-only IDs for Moonlight protocol compatibility
func (w *WolfExecutor) generateWolfAppID(userID, environmentName string) string {
	stableKey := fmt.Sprintf("%s-%s", userID, environmentName)
	// Create a numeric hash by summing byte values
	var numericHash uint64
	for _, b := range []byte(stableKey) {
		numericHash = numericHash*31 + uint64(b)
	}
	// Convert to string and limit to reasonable length for Wolf
	return fmt.Sprintf("%d", numericHash%1000000000) // Max 9 digits
}

// generateLobbyPIN generates a random 4-digit PIN for lobby access control
func generateLobbyPIN() ([]int16, string) {
	pin := make([]int16, 4)
	b := make([]byte, 4)
	rand.Read(b)
	for i := range pin {
		pin[i] = int16(b[i] % 10) // 0-9
	}
	pinString := fmt.Sprintf("%d%d%d%d", pin[0], pin[1], pin[2], pin[3])
	return pin, pinString
}

// SwayWolfAppConfig contains configuration for creating a Sway-based Wolf app
type SwayWolfAppConfig struct {
	WolfAppID         string
	Title             string
	ContainerHostname string
	UserID            string   // User ID for SSH key mounting
	SessionID         string   // Session ID for settings sync daemon
	WorkspaceDir      string
	ExtraEnv          []string
	DisplayWidth      int
	DisplayHeight     int
	DisplayFPS        int
}

// createSwayWolfApp creates a Wolf app with Sway compositor (shared between PDEs and external agents)
func (w *WolfExecutor) createSwayWolfApp(config SwayWolfAppConfig) *wolf.App {
	// Build base environment variables (common to all Sway apps)
	env := []string{
		"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
		"RUN_SWAY=1",
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", os.Getenv("ANTHROPIC_API_KEY")),
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		"ZED_HELIX_URL=api:8080",
		fmt.Sprintf("ZED_HELIX_TOKEN=%s", w.helixAPIToken),
		"ZED_HELIX_TLS=false",
		"RUST_LOG=info", // Enable Rust logging for Zed
		// Settings sync daemon configuration
		fmt.Sprintf("HELIX_SESSION_ID=%s", config.SessionID),
		// CRITICAL: Must use Docker network hostname, not localhost, from inside container
		"HELIX_API_URL=http://api:8080",
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		"SETTINGS_SYNC_PORT=9877",
	}

	// Add any extra environment variables
	env = append(env, config.ExtraEnv...)

	// Build standard mounts (common to all Sway apps)
	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", config.WorkspaceDir),
		fmt.Sprintf("%s/zed-build:/zed-build:ro", os.Getenv("HELIX_HOST_HOME")),
		fmt.Sprintf("%s/wolf/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro", os.Getenv("HELIX_HOST_HOME")),
		fmt.Sprintf("%s/wolf/sway-config/start-zed-helix.sh:/usr/local/bin/start-zed-helix.sh:ro", os.Getenv("HELIX_HOST_HOME")),
		"/var/run/docker.sock:/var/run/docker.sock",
	}

	// Add SSH keys mount if user has SSH keys
	// The SSH key directory is created by the API when keys are created
	// Mount as read-only for security
	sshKeyDir := fmt.Sprintf("/opt/helix/filestore/ssh-keys/%s", config.UserID)
	if _, err := os.Stat(sshKeyDir); err == nil {
		mounts = append(mounts, fmt.Sprintf("%s:/home/retro/.ssh:ro", sshKeyDir))
		log.Info().
			Str("user_id", config.UserID).
			Str("ssh_key_dir", sshKeyDir).
			Msg("Mounting SSH keys for git access")
	}

	// Standard Docker configuration (same for all Sway apps)
	baseCreateJSON := fmt.Sprintf(`{
  "Hostname": "%s",
  "HostConfig": {
    "IpcMode": "host",
    "NetworkMode": "helix_default",
    "Privileged": false,
    "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
    "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"],
    "Ulimits": [
      {
        "Name": "nofile",
        "Soft": 65536,
        "Hard": 65536
      }
    ]
  }
}`, config.ContainerHostname)

	// Create Wolf app
	return wolf.NewMinimalDockerApp(
		config.WolfAppID,
		config.Title,
		config.ContainerHostname,
		w.zedImage, // Now uses helix-sway:latest for both PDEs and external agents
		env,
		mounts,
		baseCreateJSON,
		config.DisplayWidth,
		config.DisplayHeight,
		config.DisplayFPS,
	)
}

// NewWolfExecutor creates a Wolf executor based on WOLF_MODE environment variable
// WOLF_MODE=apps (default) - simpler, more reliable apps-based approach
// WOLF_MODE=lobbies - feature-rich lobbies with keepalive and PINs
func NewWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, store store.Store, wsChecker WebSocketConnectionChecker) Executor {
	wolfMode := os.Getenv("WOLF_MODE")
	if wolfMode == "" {
		wolfMode = "apps" // Default to simpler, more stable apps model
	}

	log.Info().Str("wolf_mode", wolfMode).Msg("Initializing Wolf executor")

	switch wolfMode {
	case "lobbies":
		return NewLobbyWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken, store)
	case "apps":
		return NewAppWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken, store, wsChecker)
	default:
		log.Fatal().Str("wolf_mode", wolfMode).Msg("Invalid WOLF_MODE - must be 'apps' or 'lobbies'")
		return nil
	}
}

// NewLobbyWolfExecutor creates a lobby-based Wolf executor (current implementation)
func NewLobbyWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, store store.Store) *WolfExecutor {
	wolfClient := wolf.NewClient(wolfSocketPath)

	executor := &WolfExecutor{
		wolfClient:        wolfClient,
		store:             store,
		sessions:          make(map[string]*ZedSession),
		zedImage:          zedImage,
		helixAPIURL:       helixAPIURL,
		helixAPIToken:     helixAPIToken,
		workspaceBasePath: "/opt/helix/filestore/workspaces", // Default workspace base path
	}

	// Create health monitor for Wolf restarts (no keepalive reconciliation in lobbies mode)
	executor.healthMonitor = wolf.NewHealthMonitor(wolfClient, func(ctx context.Context) {
		// After Wolf restarts, reconcile lobbies
		log.Info().Msg("Wolf restarted, reconciling lobbies")
		executor.reconcileLobbies(ctx)
	})

	// Lobbies mode doesn't need keepalive reconciliation
	// Lobbies persist naturally without the keepalive hack

	// Start idle external agent cleanup loop (30min timeout)
	go executor.idleExternalAgentCleanupLoop(context.Background())

	// Start Wolf health monitoring
	executor.healthMonitor.Start(context.Background())

	return executor
}

// keepaliveReconciliationLoop runs periodically to check and restart failed keepalive sessions
// This handles moonlight-web restarts and other connection failures
func (w *WolfExecutor) keepaliveReconciliationLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Info().Msg("Starting keepalive reconciliation loop")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Keepalive reconciliation loop stopped")
			return

		case <-ticker.C:
			w.reconcileKeepaliveSessions(ctx)
		}
	}
}

// reconcileKeepaliveSessions checks all active sessions and restarts failed keepalive connections
func (w *WolfExecutor) reconcileKeepaliveSessions(ctx context.Context) {
	w.mutex.RLock()
	sessionsToReconcile := make(map[string]*ZedSession)
	for sessionID, session := range w.sessions {
		// Check if keepalive needs reconciliation
		if session.KeepaliveStatus == "failed" || session.KeepaliveStatus == "" {
			sessionsToReconcile[sessionID] = session
		} else if session.KeepaliveStatus == "active" && session.KeepaliveLastCheck != nil {
			// Check if last check was too long ago (stale connection)
			if time.Since(*session.KeepaliveLastCheck) > 2*time.Minute {
				log.Warn().
					Str("session_id", sessionID).
					Time("last_check", *session.KeepaliveLastCheck).
					Msg("Keepalive session appears stale, will restart")
				sessionsToReconcile[sessionID] = session
			}
		}
	}
	w.mutex.RUnlock()

	if len(sessionsToReconcile) == 0 {
		return
	}

	log.Info().
		Int("session_count", len(sessionsToReconcile)).
		Msg("Reconciling keepalive sessions")

	for sessionID, session := range sessionsToReconcile {
		log.Info().
			Str("session_id", sessionID).
			Str("lobby_id", session.WolfLobbyID).
			Str("keepalive_status", session.KeepaliveStatus).
			Msg("Restarting failed/stale keepalive session")

		// Get lobby PIN from session (we need it for reconnection)
		// Note: For external agents, we don't store the PIN in the session currently
		// This is a limitation - we should store it for reconciliation
		// For now, attempt without PIN (will fail if PIN is required)

		// Start new keepalive goroutine
		go w.startKeepaliveSession(ctx, sessionID, session.WolfLobbyID, "")
	}
}

// reconcileLobbies checks lobbies after Wolf restarts (lobbies persist naturally, no keepalive needed)
func (w *WolfExecutor) reconcileLobbies(ctx context.Context) {
	// In lobbies mode, lobbies persist naturally without keepalive
	// This is just a placeholder for future lobby reconciliation logic if needed
	log.Info().Msg("Lobbies reconciliation completed (lobbies persist without keepalive)")
}

// StartZedAgent implements the Executor interface for external agent sessions
func (w *WolfExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("project_path", agent.ProjectPath).
		Msg("Starting external Zed agent via Wolf")

	// Generate numeric Wolf app ID for Moonlight protocol compatibility
	// Use session ID as environment name for consistency
	wolfAppID := w.generateWolfAppID(agent.UserID, agent.SessionID)

	// Determine workspace directory - use session-specific path
	workspaceDir := agent.WorkDir
	if workspaceDir == "" {
		workspaceDir = filepath.Join(w.workspaceBasePath, "external-agents", agent.SessionID)
	}

	// Create workspace directory if it doesn't exist
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	log.Info().
		Str("wolf_app_id", wolfAppID).
		Str("workspace_dir", workspaceDir).
		Str("session_id", agent.SessionID).
		Msg("Creating Wolf app for external Zed agent")

	// Create Sway compositor configuration (same as PDEs)
	err := w.createSwayConfig(agent.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Sway config: %w", err)
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("workspace_dir", workspaceDir).
		Msg("Created Sway compositor configuration for external agent")

	// Define container hostname for external agent
	// Use session ID (without ses_ prefix) so we can construct hostname from session ID
	sessionIDPart := strings.TrimPrefix(agent.SessionID, "ses_")
	containerHostname := fmt.Sprintf("zed-external-%s", sessionIDPart)

	// Build agent instance ID for this session-scoped agent
	agentInstanceID := fmt.Sprintf("zed-session-%s", agent.SessionID)

	// Build extra environment variables specific to external agents
	// Determine which session ID to use for HELIX_SESSION_ID env var
	// If HelixSessionID is set, use it (agent created FOR an existing Helix session)
	// Otherwise use agent.SessionID (legacy behavior)
	helixSessionID := agent.SessionID
	if agent.HelixSessionID != "" {
		helixSessionID = agent.HelixSessionID
	}

	extraEnv := []string{
		// Agent identification (used for WebSocket connection)
		fmt.Sprintf("HELIX_AGENT_INSTANCE_ID=%s", agentInstanceID),
		fmt.Sprintf("HELIX_SCOPE_TYPE=session"),
		fmt.Sprintf("HELIX_SCOPE_ID=%s", agent.SessionID),

		// CRITICAL: Use actual Helix session ID for WebSocket communication
		fmt.Sprintf("HELIX_SESSION_ID=%s", helixSessionID),
		fmt.Sprintf("HELIX_USER_ID=%s", agent.UserID),

		"SWAY_STOP_ON_APP_EXIT=no", // Keep desktop alive when Zed restarts
	}
	// Add custom env vars from agent request
	extraEnv = append(extraEnv, agent.Env...)

	// Extract video settings from agent config (Phase 3.5) with defaults
	displayWidth := agent.DisplayWidth
	if displayWidth == 0 {
		displayWidth = 2560 // MacBook Pro 13" default
	}
	displayHeight := agent.DisplayHeight
	if displayHeight == 0 {
		displayHeight = 1600
	}
	displayRefreshRate := agent.DisplayRefreshRate
	if displayRefreshRate == 0 {
		displayRefreshRate = 60
	}

	// Auto-pair Wolf with moonlight-web before creating lobby
	// This ensures moonlight-web can connect to Wolf without manual pairing
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080"
	}
	credentials := os.Getenv("MOONLIGHT_CREDENTIALS")

	// Attempt pairing using MOONLIGHT_INTERNAL_PAIRING_PIN
	// Wolf will auto-accept when it receives the PIN
	if err := ensureWolfPaired(ctx, w.wolfClient, moonlightWebURL, credentials); err != nil {
		log.Warn().
			Err(err).
			Msg("Auto-pairing failed - Wolf may not be paired with moonlight-web")
	}

	// Generate PIN for lobby access control (Phase 3: Multi-tenancy)
	lobbyPIN, lobbyPINString := generateLobbyPIN()

	// NEW: Create lobby instead of app for immediate auto-start
	lobbyReq := &wolf.CreateLobbyRequest{
		ProfileID:              "helix-sessions",
		Name:                   fmt.Sprintf("Agent %s", agent.SessionID[len(agent.SessionID)-4:]),
		MultiUser:              true,
		StopWhenEveryoneLeaves: false, // CRITICAL: Agent must keep running when no Moonlight clients connected!
		PIN:                    lobbyPIN, // NEW: Require PIN to join lobby
		VideoSettings: &wolf.LobbyVideoSettings{
			Width:                   displayWidth,
			Height:                  displayHeight,
			RefreshRate:             displayRefreshRate,
			WaylandRenderNode:       "/dev/dri/renderD128",
			RunnerRenderNode:        "/dev/dri/renderD128",
			VideoProducerBufferCaps: "video/x-raw", // Simpler caps without memory type
		},
		AudioSettings: &wolf.LobbyAudioSettings{
			ChannelCount: 2,
		},
		RunnerStateFolder: filepath.Join("/wolf-state", "agent-"+agent.SessionID),
		Runner: w.createSwayWolfApp(SwayWolfAppConfig{
			WolfAppID:         wolfAppID, // Still used for app config, but not for Wolf API
			Title:             fmt.Sprintf("External Agent %s", agent.SessionID),
			ContainerHostname: containerHostname,
			UserID:            agent.UserID,
			SessionID:         agent.SessionID,
			WorkspaceDir:      workspaceDir,
			ExtraEnv:          extraEnv,
			DisplayWidth:      displayWidth,
			DisplayHeight:     displayHeight,
			DisplayFPS:        displayRefreshRate,
		}).Runner, // Use the runner config from the app
	}

	// Create lobby (container starts immediately!)
	lobbyResp, err := w.wolfClient.CreateLobby(ctx, lobbyReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create lobby for external agent: %w", err)
	}

	log.Info().
		Str("lobby_id", lobbyResp.LobbyID).
		Str("session_id", agent.SessionID).
		Str("lobby_pin", lobbyPINString).
		Msg("Wolf lobby created successfully - container starting immediately")

	// Track session with lobby ID
	session := &ZedSession{
		SessionID:       agent.SessionID,
		HelixSessionID:  helixSessionID, // Store Helix session ID for screenshot lookup
		UserID:          agent.UserID,
		Status:          "running", // Container is running immediately with lobbies
		StartTime:       time.Now(),
		LastAccess:      time.Now(),
		ProjectPath:     agent.ProjectPath,
		WolfLobbyID:     lobbyResp.LobbyID, // NEW: Track lobby ID
		ContainerName:   containerHostname,
		KeepaliveStatus: "starting", // Initialize keepalive as starting
	}
	w.sessions[agent.SessionID] = session

	// Lobbies mode doesn't need keepalive - lobbies persist naturally
	// (keepalive was a hack for apps mode to prevent stale buffer crashes)

	// Return response with screenshot URL, Moonlight info, and PIN
	response := &types.ZedAgentResponse{
		SessionID:     agent.SessionID,
		ScreenshotURL: fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
		StreamURL:     fmt.Sprintf("moonlight://localhost:47989"),
		Status:        "running", // Lobby starts immediately
		ContainerName: containerHostname,
		WolfLobbyID:   lobbyResp.LobbyID, // NEW: Return lobby ID
		WolfLobbyPIN:  lobbyPINString, // NEW: Return PIN for storage in Helix session
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("screenshot_url", response.ScreenshotURL).
		Str("container_name", containerHostname).
		Msg("External Zed agent started successfully")

	return response, nil
}

// buildZedCommand constructs the Zed execution command with proper environment variables
func (w *WolfExecutor) buildZedCommand(agent *types.ZedAgent) string {
	// Build environment variables for Zed
	envVars := []string{
		fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		fmt.Sprintf("ZED_SESSION_ID=%s", agent.SessionID),
		fmt.Sprintf("ZED_USER_ID=%s", agent.UserID),
		fmt.Sprintf("ZED_PROJECT_PATH=%s", agent.ProjectPath),
		fmt.Sprintf("ZED_WORK_DIR=%s", agent.WorkDir),
		"DISPLAY=:0",
		"WAYLAND_DISPLAY=wayland-1",
	}

	// Add any additional environment variables from the agent
	for _, env := range agent.Env {
		envVars = append(envVars, env)
	}

	// Construct the full command
	// This assumes we'll have a Zed container image or binary available
	cmd := fmt.Sprintf("env %s /usr/local/bin/zed --foreground",
		joinEnvVars(envVars))

	return cmd
}

// joinEnvVars joins environment variables with spaces
func joinEnvVars(envVars []string) string {
	result := ""
	for i, env := range envVars {
		if i > 0 {
			result += " "
		}
		result += env
	}
	return result
}

// StopZedAgent implements the Executor interface
func (w *WolfExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().Str("session_id", sessionID).Msg("Stopping Zed agent via Wolf")

	session, exists := w.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Stop the lobby (tears down container)
	if session.WolfLobbyID != "" {
		stopReq := &wolf.StopLobbyRequest{
			LobbyID: session.WolfLobbyID,
			// PIN not needed - Helix created the lobby, can stop it without PIN
		}
		err := w.wolfClient.StopLobby(ctx, stopReq)
		if err != nil {
			log.Error().Err(err).Str("lobby_id", session.WolfLobbyID).Msg("Failed to stop Wolf lobby")
			// Continue with cleanup even if stop fails
		}
		log.Info().
			Str("lobby_id", session.WolfLobbyID).
			Str("session_id", sessionID).
			Msg("Wolf lobby stopped successfully")
	} else {
		// Fallback for old app-based sessions (backward compatibility during migration)
		appID := fmt.Sprintf("zed-agent-%s", sessionID)
		err := w.wolfClient.StopSession(ctx, sessionID)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to stop Wolf session")
		}
		err = w.wolfClient.RemoveApp(ctx, appID)
		if err != nil {
			log.Error().Err(err).Str("app_id", appID).Msg("Failed to remove Wolf app")
		}
	}

	// Update session status
	session.Status = "stopped"

	// Remove from our tracking
	delete(w.sessions, sessionID)

	log.Info().Str("session_id", sessionID).Msg("Zed agent stopped successfully")

	return nil
}

// GetSession implements the Executor interface
func (w *WolfExecutor) GetSession(sessionID string) (*ZedSession, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	session, exists := w.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Update last access time
	session.LastAccess = time.Now()

	return session, nil
}

// CleanupExpiredSessions implements the Executor interface
func (w *WolfExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
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
			Msg("Cleaning up expired Zed session")

		err := w.StopZedAgent(ctx, sessionID)
		if err != nil {
			log.Error().
				Err(err).
				Str("session_id", sessionID).
				Msg("Failed to stop expired session")
		}
	}
}

// ListSessions implements the Executor interface
func (w *WolfExecutor) ListSessions() []*ZedSession {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	sessions := make([]*ZedSession, 0, len(w.sessions))
	for _, session := range w.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// Multi-session SpecTask methods (for future use)

// StartZedInstance implements the Executor interface
func (w *WolfExecutor) StartZedInstance(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	// For now, delegate to single-session method
	return w.StartZedAgent(ctx, agent)
}

// CreateZedThread implements the Executor interface
func (w *WolfExecutor) CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error {
	// TODO: Implement multi-threading support when needed
	return fmt.Errorf("multi-threading not yet implemented in Wolf executor")
}

// StopZedInstance implements the Executor interface
func (w *WolfExecutor) StopZedInstance(ctx context.Context, instanceID string) error {
	// For now, delegate to single-session method
	return w.StopZedAgent(ctx, instanceID)
}

// GetInstanceStatus implements the Executor interface
func (w *WolfExecutor) GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error) {
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

// ListInstanceThreads implements the Executor interface
func (w *WolfExecutor) ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error) {
	session, err := w.GetSession(instanceID)
	if err != nil {
		return nil, err
	}

	// Return single thread for now
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

// Personal Development Environment Management

// CreatePersonalDevEnvironment creates a new personal development environment with default display settings
func (w *WolfExecutor) CreatePersonalDevEnvironment(ctx context.Context, userID, appID, environmentName string) (*ZedInstanceInfo, error) {
	return w.CreatePersonalDevEnvironmentWithDisplay(ctx, userID, appID, environmentName, 2360, 1640, 120)
}

// CreatePersonalDevEnvironmentWithDisplay creates a new personal development environment with custom display settings
func (w *WolfExecutor) CreatePersonalDevEnvironmentWithDisplay(ctx context.Context, userID, appID, environmentName string, displayWidth, displayHeight, displayFPS int) (*ZedInstanceInfo, error) {
	// Validate display parameters
	if err := validateDisplayParams(displayWidth, displayHeight, displayFPS); err != nil {
		return nil, fmt.Errorf("invalid display configuration: %w", err)
	}

	// Create Wolf app for this personal dev environment
	wolfAppID := w.generateWolfAppID(userID, environmentName)

	// Generate unique timestamp-based ID for this instance
	timestamp := time.Now().Unix()
	instanceID := fmt.Sprintf("personal-dev-%s-%d", userID, timestamp)

	log.Info().
		Str("instance_id", instanceID).
		Str("user_id", userID).
		Str("app_id", appID).
		Str("environment_name", environmentName).
		Int("display_width", displayWidth).
		Int("display_height", displayHeight).
		Int("display_fps", displayFPS).
		Msg("Creating personal development environment via Wolf")

	// Create persistent workspace directory
	workspaceDir, err := w.createWorkspaceDirectory(instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Str("workspace_dir", workspaceDir).
		Msg("Created persistent workspace directory")

	// Create Sway configuration for this personal dev environment
	err = w.createSwayConfig(instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Sway config: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Msg("Created Sway compositor configuration")

	// Use Wolf app ID as both container name and hostname for predictable DNS
	containerHostname := fmt.Sprintf("personal-dev-%s", wolfAppID)
	containerName := containerHostname

	// Build extra environment variables specific to PDEs
	extraEnv := []string{
		"HELIX_STARTUP_SCRIPT=/home/retro/work/startup.sh",
	}

	// Auto-pair Wolf with moonlight-web before creating lobby
	// This ensures moonlight-web can connect to Wolf without manual pairing
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080"
	}
	credentials := os.Getenv("MOONLIGHT_CREDENTIALS")

	// Attempt pairing using MOONLIGHT_INTERNAL_PAIRING_PIN
	// Wolf will auto-accept when it receives the PIN
	if err := ensureWolfPaired(ctx, w.wolfClient, moonlightWebURL, credentials); err != nil {
		log.Warn().
			Err(err).
			Msg("Auto-pairing failed - Wolf may not be paired with moonlight-web")
	}

	// Generate PIN for lobby access control (Phase 3: Multi-tenancy)
	lobbyPIN, lobbyPINString := generateLobbyPIN()

	// NEW: Create lobby instead of app for immediate auto-start
	lobbyReq := &wolf.CreateLobbyRequest{
		ProfileID:              "helix-sessions",
		Name:                   fmt.Sprintf("PDE: %s", environmentName),
		MultiUser:              true,
		StopWhenEveryoneLeaves: false, // CRITICAL: Keep running when clients disconnect
		PIN:                    lobbyPIN, // NEW: Require PIN to join lobby
		VideoSettings: &wolf.LobbyVideoSettings{
			Width:                   displayWidth,
			Height:                  displayHeight,
			RefreshRate:             displayFPS,
			WaylandRenderNode:       "/dev/dri/renderD128",
			RunnerRenderNode:        "/dev/dri/renderD128",
			VideoProducerBufferCaps: "video/x-raw", // Simpler caps without memory type
		},
		AudioSettings: &wolf.LobbyAudioSettings{
			ChannelCount: 2,
		},
		RunnerStateFolder: filepath.Join("/wolf-state", instanceID),
		Runner: w.createSwayWolfApp(SwayWolfAppConfig{
			WolfAppID:         wolfAppID,
			Title:             fmt.Sprintf("Personal Dev Environment %s", environmentName),
			ContainerHostname: containerHostname,
			UserID:            userID,
			SessionID:         instanceID, // Use instance ID as session ID for settings sync
			WorkspaceDir:      workspaceDir,
			ExtraEnv:          extraEnv,
			DisplayWidth:      displayWidth,
			DisplayHeight:     displayHeight,
			DisplayFPS:        displayFPS,
		}).Runner,
	}

	// Create lobby (container starts immediately!)
	lobbyResp, err := w.wolfClient.CreateLobby(ctx, lobbyReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create lobby for PDE: %w", err)
	}

	log.Info().
		Str("lobby_id", lobbyResp.LobbyID).
		Str("instance_id", instanceID).
		Str("lobby_pin", lobbyPINString).
		Msg("Wolf lobby created successfully - PDE container starting immediately")

	// Save to database
	pde := &types.PersonalDevEnvironment{
		ID:              instanceID,
		UserID:          userID,
		AppID:           appID, // Original Helix App ID for configuration
		WolfAppID:       wolfAppID, // Keep for backward compatibility
		WolfLobbyID:     lobbyResp.LobbyID, // NEW: Track lobby ID
		WolfLobbyPIN:    lobbyPINString, // NEW: Store PIN for access control
		EnvironmentName: environmentName,
		Status:          "running", // Container is running immediately with lobbies
		LastActivity:    time.Now(),
		DisplayWidth:    displayWidth,
		DisplayHeight:   displayHeight,
		DisplayFPS:      displayFPS,
		ContainerName:   containerName,
		VNCPort:         5901,
		StreamURL:       fmt.Sprintf("moonlight://localhost:47989"), // Moonlight streaming
		WolfSessionID:   "", // No longer used with lobbies
	}

	pde, err = w.store.CreatePersonalDevEnvironment(ctx, pde)
	if err != nil {
		return nil, fmt.Errorf("failed to save personal dev environment to database: %w", err)
	}

	log.Info().
		Str("instance_id", instanceID).
		Str("wolf_lobby_id", lobbyResp.LobbyID).
		Str("wolf_app_id", wolfAppID).
		Msg("Personal development environment created successfully via Wolf lobbies")

	// Convert to ZedInstanceInfo for backward compatibility
	instance := &ZedInstanceInfo{
		InstanceID:      pde.ID,
		SpecTaskID:      "",
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
		ConfiguredTools: []string{},
		DataSources:     []string{},
		StreamURL:       pde.StreamURL,
		WolfSessionID:   pde.WolfSessionID,
		DisplayWidth:    pde.DisplayWidth,
		DisplayHeight:   pde.DisplayHeight,
		DisplayFPS:      pde.DisplayFPS,
		ContainerName:   pde.ContainerName,
		VNCPort:         pde.VNCPort,
	}

	return instance, nil
}

// buildPersonalDevZedCommand constructs the Zed execution command for personal dev environments
func (w *WolfExecutor) buildPersonalDevZedCommand(userID, appID, instanceID string) string {
	// Build environment variables for personal dev Zed
	envVars := []string{
		fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		fmt.Sprintf("ZED_INSTANCE_ID=%s", instanceID),
		fmt.Sprintf("ZED_USER_ID=%s", userID),
		fmt.Sprintf("ZED_APP_ID=%s", appID),
		fmt.Sprintf("ZED_INSTANCE_TYPE=personal_dev"),
		fmt.Sprintf("ZED_WORK_DIR=/workspace"),
		"DISPLAY=:0",
		"WAYLAND_DISPLAY=wayland-1",
	}

	// Construct the full command
	cmd := fmt.Sprintf("env %s /usr/local/bin/zed --foreground",
		joinEnvVars(envVars))

	return cmd
}

// GetPersonalDevEnvironments returns all personal dev environments for a user
func (w *WolfExecutor) GetPersonalDevEnvironments(ctx context.Context, userID string) ([]*ZedInstanceInfo, error) {
	// Read from database
	pdes, err := w.store.ListPersonalDevEnvironments(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list personal dev environments from database: %w", err)
	}

	// Convert to ZedInstanceInfo for backward compatibility
	var personalEnvs []*ZedInstanceInfo
	for _, pde := range pdes {
		instance := &ZedInstanceInfo{
			InstanceID:      pde.ID,
			SpecTaskID:      "",
			UserID:          pde.UserID,
			AppID:           pde.WolfAppID,
			InstanceType:    "personal_dev",
			Status:          pde.Status,
			CreatedAt:       pde.Created,
			LastActivity:    pde.LastActivity,
			ProjectPath:     fmt.Sprintf("/workspace/%s", pde.EnvironmentName),
			ThreadCount:     1,
			IsPersonalEnv:   true,
			EnvironmentName: pde.EnvironmentName,
			ConfiguredTools: []string{},
			DataSources:     []string{},
			StreamURL:       pde.StreamURL,
			WolfSessionID:   pde.WolfSessionID,
			DisplayWidth:    pde.DisplayWidth,
			DisplayHeight:   pde.DisplayHeight,
			DisplayFPS:      pde.DisplayFPS,
			ContainerName:   pde.ContainerName,
			VNCPort:         pde.VNCPort,
		}
		personalEnvs = append(personalEnvs, instance)
	}

	log.Info().
		Str("user_id", userID).
		Int("environment_count", len(personalEnvs)).
		Msg("Retrieved personal dev environments from database")

	return personalEnvs, nil
}

// StopPersonalDevEnvironment stops a personal development environment
func (w *WolfExecutor) StopPersonalDevEnvironment(ctx context.Context, userID, instanceID string) error {
	// Read from database
	pde, err := w.store.GetPersonalDevEnvironment(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("personal dev environment %s not found", instanceID)
	}

	// Check access
	if pde.UserID != userID {
		return fmt.Errorf("access denied: environment belongs to different user")
	}

	log.Info().Str("instance_id", instanceID).Msg("Stopping personal dev environment via Wolf")

	// Stop the lobby (tears down container)
	if pde.WolfLobbyID != "" {
		stopReq := &wolf.StopLobbyRequest{
			LobbyID: pde.WolfLobbyID,
			// PIN not needed - Helix created the lobby, can stop it without PIN
		}
		err := w.wolfClient.StopLobby(ctx, stopReq)
		if err != nil {
			log.Error().Err(err).Str("lobby_id", pde.WolfLobbyID).Msg("Failed to stop Wolf lobby")
			// Continue with cleanup even if stop fails
		}
		log.Info().
			Str("lobby_id", pde.WolfLobbyID).
			Str("instance_id", instanceID).
			Msg("Wolf lobby stopped successfully")
	} else {
		// Fallback for old app-based PDEs (backward compatibility during migration)
		wolfAppID := w.generateWolfAppID(pde.UserID, pde.EnvironmentName)

		if pde.WolfSessionID != "" {
			err := w.wolfClient.StopSession(ctx, pde.WolfSessionID)
			if err != nil {
				log.Error().Err(err).Str("wolf_session_id", pde.WolfSessionID).Msg("Failed to stop Wolf session")
			}
		}

		err := w.wolfClient.RemoveApp(ctx, wolfAppID)
		if err != nil {
			log.Error().Err(err).Str("wolf_app_id", wolfAppID).Msg("Failed to remove Wolf app")
		}
	}

	// Clean up Sway configuration file
	swayConfigPath := fmt.Sprintf("/tmp/sway-config-%s", instanceID)
	if err := os.Remove(swayConfigPath); err != nil {
		log.Warn().Err(err).Str("config_path", swayConfigPath).Msg("Failed to remove Sway config file")
	} else {
		log.Info().Str("config_path", swayConfigPath).Msg("Removed Sway config file")
	}

	// Delete from database
	err = w.store.DeletePersonalDevEnvironment(ctx, instanceID)
	if err != nil {
		log.Error().Err(err).Str("instance_id", instanceID).Msg("Failed to delete personal dev environment from database")
		return fmt.Errorf("failed to delete environment from database: %w", err)
	}

	log.Info().Str("instance_id", instanceID).Msg("Personal dev environment stopped and cleaned up successfully")

	return nil
}

// GetPersonalDevEnvironment returns a specific personal dev environment for a user
func (w *WolfExecutor) GetPersonalDevEnvironment(ctx context.Context, userID, environmentID string) (*ZedInstanceInfo, error) {
	// Read from database
	pde, err := w.store.GetPersonalDevEnvironment(ctx, environmentID)
	if err != nil {
		return nil, fmt.Errorf("environment not found: %s", environmentID)
	}

	// Check access
	if pde.UserID != userID {
		return nil, fmt.Errorf("access denied: environment belongs to different user")
	}

	// Convert to ZedInstanceInfo for backward compatibility
	instance := &ZedInstanceInfo{
		InstanceID:      pde.ID,
		SpecTaskID:      "",
		UserID:          pde.UserID,
		AppID:           pde.WolfAppID,
		InstanceType:    "personal_dev",
		Status:          pde.Status,
		CreatedAt:       pde.Created,
		LastActivity:    pde.LastActivity,
		ProjectPath:     fmt.Sprintf("/workspace/%s", pde.EnvironmentName),
		ThreadCount:     1,
		IsPersonalEnv:   true,
		EnvironmentName: pde.EnvironmentName,
		ConfiguredTools: []string{},
		DataSources:     []string{},
		StreamURL:       pde.StreamURL,
		WolfSessionID:   pde.WolfSessionID,
		DisplayWidth:    pde.DisplayWidth,
		DisplayHeight:   pde.DisplayHeight,
		DisplayFPS:      pde.DisplayFPS,
		ContainerName:   pde.ContainerName,
		VNCPort:         pde.VNCPort,
	}

	return instance, nil
}

// ReconcilePersonalDevEnvironments cleans up orphaned configuration files
func (w *WolfExecutor) ReconcilePersonalDevEnvironments(ctx context.Context) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	log.Info().Msg("Starting personal dev environment reconciliation (Wolf apps + config cleanup)")

	// Step 1: Reconcile Wolf apps against Helix instances
	err := w.reconcileWolfApps(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reconcile Wolf apps")
		// Continue with config cleanup even if Wolf reconciliation fails
	}

	// Clean up orphaned Sway config files
	orphanedConfigs := 0
	configPattern := "/tmp/sway-config-personal-dev-*"
	configFiles, err := filepath.Glob(configPattern)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to find Sway config files for cleanup")
	} else {
		for _, configFile := range configFiles {
			// Extract instance ID from config filename
			basename := filepath.Base(configFile)
			instanceID := strings.TrimPrefix(basename, "sway-config-")

			// Check if this instance exists in database
			_, dbErr := w.store.GetPersonalDevEnvironment(ctx, instanceID)
			if dbErr != nil {
				// Instance not found in database, config is orphaned
				log.Info().
					Str("config_file", configFile).
					Str("instance_id", instanceID).
					Msg("Found orphaned Sway config file, removing")

				err = os.Remove(configFile)
				if err != nil {
					log.Error().Err(err).Str("config_file", configFile).Msg("Failed to remove orphaned Sway config")
				} else {
					orphanedConfigs++
				}
			}
		}
	}

	log.Info().
		Int("orphaned_configs", orphanedConfigs).
		Msg("Personal dev environment reconciliation completed")

	return nil
}

// createWorkspaceDirectory creates a persistent workspace directory for an instance
func (w *WolfExecutor) createWorkspaceDirectory(instanceID string) (string, error) {
	workspaceDir := filepath.Join(w.workspaceBasePath, instanceID)

	// Create the workspace directory
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Create a startup script template
	startupScriptPath := filepath.Join(workspaceDir, "startup.sh")
	if _, err := os.Stat(startupScriptPath); os.IsNotExist(err) {
		startupScript := `#!/bin/bash
# Personal Development Environment Startup Script
# This script runs when your dev environment starts up
# You can add commands here to install packages, configure tools, etc.

echo "Starting up personal dev environment: ` + instanceID + `"

# Ensure workspace directory has correct permissions
sudo chown -R retro:retro ~/work

# Install JupyterLab and OnlyOffice
echo "Installing JupyterLab and OnlyOffice..."
sudo apt update
sudo apt install -y python3-pip
pip3 install jupyterlab

# Install OnlyOffice
sudo apt install -y snapd
sudo snap install onlyoffice-desktopeditors

# Example: Configure git
# git config --global user.name "Your Name"
# git config --global user.email "your.email@example.com"

# Example: Install development tools
# curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
# sudo apt install -y nodejs

echo "Startup script completed!"
echo "JupyterLab can be started with: jupyter lab --ip=0.0.0.0 --allow-root"
echo "OnlyOffice is available in applications menu"
`
		if err := os.WriteFile(startupScriptPath, []byte(startupScript), 0755); err != nil {
			return "", fmt.Errorf("failed to create startup script: %w", err)
		}
	}

	// Copy the welcome README for users to see when they open Zed
	welcomeReadmePath := filepath.Join(workspaceDir, "README.md")
	log.Info().
		Str("workspace_dir", workspaceDir).
		Str("readme_path", welcomeReadmePath).
		Msg("Checking if welcome README needs to be created")

	if _, err := os.Stat(welcomeReadmePath); os.IsNotExist(err) {
		log.Info().Msg("README does not exist, creating from template")
		// Read the template README
		templatePath := "/opt/helix/WORKDIR_README.md"
		welcomeContent, err := os.ReadFile(templatePath)
		if err != nil {
			log.Error().Err(err).Str("template_path", templatePath).Msg("Failed to read README template")
			return "", fmt.Errorf("failed to read README template at %s: %w", templatePath, err)
		}
		if err := os.WriteFile(welcomeReadmePath, welcomeContent, 0644); err != nil {
			log.Error().Err(err).Str("path", welcomeReadmePath).Msg("Failed to write welcome README")
			return "", fmt.Errorf("failed to create welcome README: %w", err)
		}
		log.Info().Str("path", welcomeReadmePath).Msg("Successfully created welcome README")
	} else if err != nil {
		log.Warn().Err(err).Str("path", welcomeReadmePath).Msg("Error checking if README exists")
	} else {
		log.Info().Str("path", welcomeReadmePath).Msg("README already exists, skipping creation")
	}

	return workspaceDir, nil
}

// createSwayConfig creates a Sway compositor configuration for the personal dev environment
func (w *WolfExecutor) createSwayConfig(instanceID string) error {
	swayConfigPath := fmt.Sprintf("/tmp/sway-config-%s", instanceID)

	swayConfig := `# Sway configuration for Helix Personal Dev Environment
# Generated for instance: ` + instanceID + `

# Set mod key to Super (Windows key)
set $mod Mod4

# Font for window titles
font pango:Monospace 8

# Use Mouse+$mod to drag floating windows
floating_modifier $mod

# Start a terminal
bindsym $mod+Return exec kitty

# Kill focused window
bindsym $mod+Shift+q kill

# Start launcher (fuzzel for Wayland)
bindsym $mod+d exec fuzzel

# Change focus
bindsym $mod+j focus left
bindsym $mod+k focus down
bindsym $mod+l focus up
bindsym $mod+semicolon focus right

# Move focused window
bindsym $mod+Shift+j move left
bindsym $mod+Shift+k move down
bindsym $mod+Shift+l move up
bindsym $mod+Shift+semicolon move right

# Split orientation
bindsym $mod+h split h
bindsym $mod+v split v

# Fullscreen mode
bindsym $mod+f fullscreen toggle

# Change container layout
bindsym $mod+s layout stacking
bindsym $mod+w layout tabbed
bindsym $mod+e layout toggle split

# Toggle floating
bindsym $mod+Shift+space floating toggle

# Workspaces
set $ws1 "1"
set $ws2 "2"
set $ws3 "3"
set $ws4 "4"

# Switch to workspace
bindsym $mod+1 workspace $ws1
bindsym $mod+2 workspace $ws2
bindsym $mod+3 workspace $ws3
bindsym $mod+4 workspace $ws4

# Move focused container to workspace
bindsym $mod+Shift+1 move container to workspace $ws1
bindsym $mod+Shift+2 move container to workspace $ws2
bindsym $mod+Shift+3 move container to workspace $ws3
bindsym $mod+Shift+4 move container to workspace $ws4

# Reload configuration
bindsym $mod+Shift+c reload

# Restart Sway
bindsym $mod+Shift+r restart

# Exit Sway
bindsym $mod+Shift+e exec swaynag -t warning -m 'Exit Sway?' -b 'Yes' 'swaymsg exit'

# Auto-start applications for development
exec kitty --working-directory=/home/user/work
exec --no-startup-id swaybg -c "#2e3440"

# Window rules
for_window [app_id="kitty"] focus
for_window [app_id="zed"] focus

# Output configuration for Wolf streaming
output * {
    mode 1920x1080@60Hz
    pos 0 0
}

# Input configuration
input * {
    xkb_layout "us"
    xkb_variant ""
    xkb_options ""
}
`

	if err := os.WriteFile(swayConfigPath, []byte(swayConfig), 0644); err != nil {
		return fmt.Errorf("failed to create Sway config: %w", err)
	}

	return nil
}

// reconcileWolfApps ensures Wolf has lobbies for all running personal dev environments
// and removes orphaned Wolf lobbies that no longer have corresponding Helix instances
func (w *WolfExecutor) reconcileWolfApps(ctx context.Context) error {
	log.Info().Msg("Reconciling Wolf lobbies against Helix personal dev environments")

	// Step 1: Build a set of expected lobby IDs from database
	// List ALL personal dev environments (across all users) for reconciliation
	allPDEs, err := w.store.ListPersonalDevEnvironments(ctx, "") // Empty userID = all users
	if err != nil {
		log.Error().Err(err).Msg("Failed to list personal dev environments from database for reconciliation")
		return fmt.Errorf("failed to list personal dev environments: %w", err)
	}

	expectedLobbyIDs := make(map[string]bool)
	pdeByLobbyID := make(map[string]*types.PersonalDevEnvironment)
	for _, pde := range allPDEs {
		if pde.Status != "running" && pde.Status != "starting" {
			continue // Skip stopped environments
		}
		if pde.WolfLobbyID != "" {
			expectedLobbyIDs[pde.WolfLobbyID] = true
			pdeByLobbyID[pde.WolfLobbyID] = pde
		}
	}

	// Step 2: Get all Wolf lobbies and delete orphaned ones
	wolfLobbies, err := w.wolfClient.ListLobbies(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list Wolf lobbies for reconciliation")
		// Continue with creating missing lobbies even if we couldn't clean up orphaned ones
	} else {
		deletedCount := 0
		for _, lobby := range wolfLobbies {
			// Reconcile Personal Dev Environments (database-backed, long-lived)
			if strings.HasPrefix(lobby.Name, "PDE:") {
				// Check if this lobby ID is expected
				if !expectedLobbyIDs[lobby.ID] {
					log.Info().
						Str("lobby_id", lobby.ID).
						Str("lobby_name", lobby.Name).
						Msg("Found orphaned PDE Wolf lobby, stopping")

					stopReq := &wolf.StopLobbyRequest{
						LobbyID: lobby.ID,
						// No PIN needed - we created these lobbies
					}
					err := w.wolfClient.StopLobby(ctx, stopReq)
					if err != nil {
						log.Error().
							Err(err).
							Str("lobby_id", lobby.ID).
							Msg("Failed to stop orphaned PDE Wolf lobby")
					} else {
						deletedCount++
					}
				}
			}

			// Reconcile External Agent sessions (ephemeral, in-memory tracked)
			// These don't have database persistence, so check against in-memory sessions map
			if strings.HasPrefix(lobby.Name, "Agent") {
				// Check if this lobby ID is tracked in memory
				lobbyTracked := false
				for _, session := range w.sessions {
					if session.WolfLobbyID == lobby.ID {
						lobbyTracked = true
						break
					}
				}

				if !lobbyTracked {
					log.Info().
						Str("lobby_id", lobby.ID).
						Str("lobby_name", lobby.Name).
						Msg("Found orphaned external agent Wolf lobby, stopping")

					// Extract session ID from lobby runner environment to look up PIN
					var sessionID string
					if runnerMap, ok := lobby.Runner.(map[string]interface{}); ok {
						if envList, ok := runnerMap["env"].([]interface{}); ok {
							for _, envItem := range envList {
								if envStr, ok := envItem.(string); ok {
									if strings.HasPrefix(envStr, "HELIX_SESSION_ID=") {
										sessionID = strings.TrimPrefix(envStr, "HELIX_SESSION_ID=")
										break
									}
								}
							}
						}
					}

					// Look up session from database to get PIN
					var lobbyPIN []int16
					if sessionID != "" {
						helixSession, err := w.store.GetSession(ctx, sessionID)
						if err == nil && helixSession.Metadata.WolfLobbyPIN != "" {
							// Convert PIN string to []int16
							for _, r := range helixSession.Metadata.WolfLobbyPIN {
								lobbyPIN = append(lobbyPIN, int16(r-'0'))
							}
							log.Info().
								Str("lobby_id", lobby.ID).
								Str("session_id", sessionID).
								Msg("Retrieved lobby PIN from session metadata for cleanup")
						} else {
							log.Warn().
								Str("lobby_id", lobby.ID).
								Str("session_id", sessionID).
								Msg("Could not retrieve PIN for orphaned lobby - cleanup may fail")
						}
					}

					stopReq := &wolf.StopLobbyRequest{
						LobbyID: lobby.ID,
						PIN:     lobbyPIN,
					}
					err := w.wolfClient.StopLobby(ctx, stopReq)
					if err != nil {
						log.Error().
							Err(err).
							Str("lobby_id", lobby.ID).
							Msg("Failed to stop orphaned external agent Wolf lobby")
					} else {
						deletedCount++
					}
				}
			}
		}
		if deletedCount > 0 {
			log.Info().Int("deleted_count", deletedCount).Msg("Deleted orphaned Wolf lobbies")
		}
	}

	// Step 3: Check each PDE individually and try to recreate lobbies as needed
	reconciledCount := 0
	recreatedCount := 0

	for _, pde := range allPDEs {
		if pde.Status != "running" && pde.Status != "starting" {
			continue // Skip stopped environments
		}

		// Check if lobby exists for this PDE
		if pde.WolfLobbyID == "" {
			// Old PDE without lobby ID - skip (migration complete, no upgrade path needed)
			log.Debug().
				Str("instance_id", pde.ID).
				Msg("PDE has no lobby ID - skipping")
			continue
		}

		log.Info().
			Str("instance_id", pde.ID).
			Str("wolf_lobby_id", pde.WolfLobbyID).
			Str("status", pde.Status).
			Msg("Checking if Wolf lobby exists for personal dev environment")

		// Check if lobby exists
		lobbyExists := false
		for _, lobby := range wolfLobbies {
			if lobby.ID == pde.WolfLobbyID {
				lobbyExists = true
				break
			}
		}

		if lobbyExists {
			log.Debug().
				Str("instance_id", pde.ID).
				Str("wolf_lobby_id", pde.WolfLobbyID).
				Msg("Wolf lobby already exists, skipping recreation")
			continue // Lobby exists, no need to recreate
		}

		log.Info().
			Str("instance_id", pde.ID).
			Str("wolf_lobby_id", pde.WolfLobbyID).
			Msg("Wolf lobby missing, recreating")

		// Recreate the lobby for this PDE
		err := w.recreateLobbyForPDE(ctx, pde)
		if err != nil {
			log.Error().
				Err(err).
				Str("instance_id", pde.ID).
				Msg("Failed to recreate Wolf lobby for personal dev environment")
			// Mark instance as stopped in database since lobby creation failed
			pde.Status = "stopped"
			w.store.UpdatePersonalDevEnvironment(ctx, pde)
		} else {
			log.Info().
				Str("instance_id", pde.ID).
				Msg("Successfully recreated Wolf lobby for personal dev environment")
			recreatedCount++
		}
		reconciledCount++
	}

	log.Info().
		Int("reconciled_count", reconciledCount).
		Int("recreated_count", recreatedCount).
		Msg("Completed Wolf lobby reconciliation")

	return nil
}

// recreateLobbyForPDE recreates a Wolf lobby for a crashed/missing PDE
func (w *WolfExecutor) recreateLobbyForPDE(ctx context.Context, pde *types.PersonalDevEnvironment) error {
	log.Info().
		Str("instance_id", pde.ID).
		Str("environment_name", pde.EnvironmentName).
		Msg("Recreating Wolf lobby for PDE")

	// Get workspace directory
	workspaceDir := filepath.Join(w.workspaceBasePath, pde.ID)

	// Recreate Sway config
	err := w.createSwayConfig(pde.ID)
	if err != nil {
		return fmt.Errorf("failed to create Sway config: %w", err)
	}

	// Generate wolf app ID and container name
	wolfAppID := w.generateWolfAppID(pde.UserID, pde.EnvironmentName)
	containerHostname := fmt.Sprintf("personal-dev-%s", wolfAppID)

	// Auto-pair Wolf with moonlight-web before creating lobby
	// This ensures moonlight-web can connect to Wolf without manual pairing
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080"
	}
	credentials := os.Getenv("MOONLIGHT_CREDENTIALS")

	// Attempt pairing using MOONLIGHT_INTERNAL_PAIRING_PIN
	// Wolf will auto-accept when it receives the PIN
	if err := ensureWolfPaired(ctx, w.wolfClient, moonlightWebURL, credentials); err != nil {
		log.Warn().
			Err(err).
			Msg("Auto-pairing failed - Wolf may not be paired with moonlight-web")
	}

	// Generate new PIN since we don't have the old one
	lobbyPIN, lobbyPINString := generateLobbyPIN()

	// Create lobby request
	lobbyReq := &wolf.CreateLobbyRequest{
		ProfileID:              "helix-sessions",
		Name:                   fmt.Sprintf("PDE: %s", pde.EnvironmentName),
		MultiUser:              true,
		StopWhenEveryoneLeaves: false,
		PIN:                    lobbyPIN,
		VideoSettings: &wolf.LobbyVideoSettings{
			Width:                   pde.DisplayWidth,
			Height:                  pde.DisplayHeight,
			RefreshRate:             pde.DisplayFPS,
			WaylandRenderNode:       "/dev/dri/renderD128",
			RunnerRenderNode:        "/dev/dri/renderD128",
			VideoProducerBufferCaps: "video/x-raw",
		},
		AudioSettings: &wolf.LobbyAudioSettings{
			ChannelCount: 2,
		},
		RunnerStateFolder: filepath.Join("/wolf-state", pde.ID),
		Runner: w.createSwayWolfApp(SwayWolfAppConfig{
			WolfAppID:         wolfAppID,
			Title:             fmt.Sprintf("Personal Dev Environment %s", pde.EnvironmentName),
			ContainerHostname: containerHostname,
			UserID:            pde.UserID,
			SessionID:         pde.ID, // Use PDE ID as session ID for settings sync
			WorkspaceDir:      workspaceDir,
			ExtraEnv:          []string{"HELIX_STARTUP_SCRIPT=/home/retro/work/startup.sh"},
			DisplayWidth:      pde.DisplayWidth,
			DisplayHeight:     pde.DisplayHeight,
			DisplayFPS:        pde.DisplayFPS,
		}).Runner,
	}

	// Create the lobby
	lobbyResp, err := w.wolfClient.CreateLobby(ctx, lobbyReq)
	if err != nil {
		return fmt.Errorf("failed to create lobby: %w", err)
	}

	// Update PDE with new lobby ID and PIN
	pde.WolfLobbyID = lobbyResp.LobbyID
	pde.WolfLobbyPIN = lobbyPINString
	pde.Status = "running"

	_, err = w.store.UpdatePersonalDevEnvironment(ctx, pde)
	if err != nil {
		// Lobby was created but we couldn't update database - log warning
		log.Error().Err(err).Str("instance_id", pde.ID).Msg("Created lobby but failed to update PDE record")
		return fmt.Errorf("failed to update PDE with new lobby ID: %w", err)
	}

	log.Info().
		Str("instance_id", pde.ID).
		Str("lobby_id", lobbyResp.LobbyID).
		Str("lobby_pin", lobbyPINString).
		Msg("Successfully recreated lobby for PDE")

	return nil
}

// recreateWolfAppForInstance recreates a Wolf app for an existing personal dev environment instance
func (w *WolfExecutor) recreateWolfAppForInstance(ctx context.Context, instance *ZedInstanceInfo) error {
	// Use consistent ID generation
	wolfAppID := w.generateWolfAppID(instance.UserID, instance.EnvironmentName)

	// Get workspace directory (should already exist)
	workspaceDir := filepath.Join(w.workspaceBasePath, instance.InstanceID)

	// Create Wolf app using the same Sway configuration as the main creation function
	env := []string{
		"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*",
		"RUN_SWAY=1", // Enable Sway compositor mode in GOW launcher
		// Pass through API keys for Zed AI functionality
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", os.Getenv("ANTHROPIC_API_KEY")),
		fmt.Sprintf("OPENAI_API_KEY=%s", os.Getenv("OPENAI_API_KEY")),
		fmt.Sprintf("TOGETHER_API_KEY=%s", os.Getenv("TOGETHER_API_KEY")),
		fmt.Sprintf("HF_TOKEN=%s", os.Getenv("HF_TOKEN")),
		// Zed external websocket sync configuration
		"ZED_EXTERNAL_SYNC_ENABLED=true", // Enables websocket sync (websocket_enabled defaults to this)
		"ZED_HELIX_URL=api:8080",         // Use Docker network service name (containers can't reach localhost)
		fmt.Sprintf("ZED_HELIX_TOKEN=%s", w.helixAPIToken),
		"ZED_HELIX_TLS=false", // Internal Docker network, no TLS needed
		// Enable user startup script execution
		"HELIX_STARTUP_SCRIPT=/home/retro/work/startup.sh",
	}
	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", workspaceDir),                        // Mount persistent workspace
		fmt.Sprintf("%s/zed-build:/zed-build:ro", os.Getenv("HELIX_HOST_HOME")), // Mount Zed directory to survive inode changes
	}

	// Use Wolf app ID as both container name and hostname for predictable DNS
	containerHostname := fmt.Sprintf("personal-dev-%s", wolfAppID)

	baseCreateJSON := fmt.Sprintf(`{
  "Hostname": "%s",
  "HostConfig": {
    "IpcMode": "host",
    "NetworkMode": "helix_default",
    "Privileged": false,
    "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
    "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"],
    "Ulimits": [
      {
        "Name": "nofile",
        "Soft": 65536,
        "Hard": 65536
      }
    ]
  }
}`, containerHostname)

	// Use Wolf app ID as container name - matches the app ID for consistency
	containerName := containerHostname

	// Use minimal app creation that exactly matches the working XFCE configuration
	app := wolf.NewMinimalDockerApp(
		wolfAppID, // ID
		fmt.Sprintf("Personal Dev %s", instance.EnvironmentName), // Title (no colon to avoid Docker volume syntax issues)
		containerName,       // URL-friendly name with hyphens
		"helix-sway:latest", // Custom Sway image with modern Wayland support and Helix branding
		env,
		mounts,
		baseCreateJSON,
		instance.DisplayWidth,  // Use stored display configuration
		instance.DisplayHeight, // Use stored display configuration
		instance.DisplayFPS,    // Use stored display configuration
	)

	// Try to remove any existing app first to avoid conflicts
	err := w.wolfClient.RemoveApp(ctx, wolfAppID)
	if err != nil {
		log.Debug().Err(err).Str("wolf_app_id", wolfAppID).Msg("No existing Wolf app to remove (expected)")
	}

	// Add the app to Wolf
	err = w.wolfClient.AddApp(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to recreate Wolf app: %w", err)
	}

	log.Info().
		Str("instance_id", instance.InstanceID).
		Str("wolf_app_id", wolfAppID).
		Msg("Successfully recreated Wolf app for personal dev environment")

	return nil
}

// NOTE: Background session creation has been moved to Wolf's native auto_persistent_sessions feature.
// Wolf now handles container lifecycle directly through auto_start_containers = true configuration.
// No need for Helix to create fake background sessions - Wolf automatically starts containers
// when apps are added, and real Moonlight clients can connect to running containers seamlessly.

// generateRandomIP generates a unique fake IP address for RTSP routing
func generateRandomIP() string {
	// Generate a random IP in the 192.168.1.x range to avoid conflicts
	// Wolf uses fake IPs to route RTSP connections back to the correct session
	return fmt.Sprintf("192.168.1.%d", 100+time.Now().UnixNano()%155) // 100-254 range
}

// checkWolfAppExists checks if a Wolf app with the given ID already exists
func (w *WolfExecutor) checkWolfAppExists(ctx context.Context, appID string) (bool, error) {
	apps, err := w.wolfClient.ListApps(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list Wolf apps: %w", err)
	}

	for _, app := range apps {
		if app.ID == appID {
			return true, nil
		}
	}

	return false, nil
}

// GetWolfClient returns the Wolf client for direct access to Wolf API
func (w *WolfExecutor) GetWolfClient() *wolf.Client {
	return w.wolfClient
}

// validateDisplayParams validates display configuration parameters
func validateDisplayParams(width, height, fps int) error {
	if width < 800 || width > 7680 {
		return fmt.Errorf("invalid display width: %d (must be 800-7680)", width)
	}
	if height < 600 || height > 4320 {
		return fmt.Errorf("invalid display height: %d (must be 600-4320)", height)
	}
	if fps < 30 || fps > 144 {
		return fmt.Errorf("invalid display fps: %d (must be 30-144)", fps)
	}

	// Validate aspect ratio is reasonable
	aspectRatio := float64(width) / float64(height)
	if aspectRatio < 0.5 || aspectRatio > 4.0 {
		return fmt.Errorf("invalid aspect ratio: %.2f (must be 0.5-4.0)", aspectRatio)
	}

	return nil
}

// FindContainerBySessionID finds an external agent container by its Helix session ID
// Returns the container hostname (DNS name) for connecting to screenshot server
func (w *WolfExecutor) FindContainerBySessionID(ctx context.Context, helixSessionID string) (string, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	// External agent sessions are keyed by agent session ID, but we need to find by Helix session ID
	// Search through all sessions to find the one with matching HelixSessionID
	for agentSessionID, session := range w.sessions {
		if session.HelixSessionID == helixSessionID {
			log.Info().
				Str("helix_session_id", helixSessionID).
				Str("agent_session_id", agentSessionID).
				Str("container_hostname", session.ContainerName).
				Msg("Found external agent container by Helix session ID")
			return session.ContainerName, nil
		}
	}

	log.Error().
		Str("helix_session_id", helixSessionID).
		Int("total_sessions", len(w.sessions)).
		Msg("No external agent session found with this Helix session ID")

	return "", fmt.Errorf("no external agent session found with Helix session ID: %s", helixSessionID)
}

// startKeepaliveSession starts a headless Moonlight session to keep the lobby alive
// This prevents the stale buffer crash that occurs when all clients disconnect and someone rejoins
func (w *WolfExecutor) startKeepaliveSession(ctx context.Context, sessionID, lobbyID, lobbyPIN string) {
	log.Info().
		Str("session_id", sessionID).
		Str("lobby_id", lobbyID).
		Msg("Starting keepalive session for lobby")

	// Update session status to starting
	w.mutex.Lock()
	session, exists := w.sessions[sessionID]
	if !exists {
		w.mutex.Unlock()
		log.Error().Str("session_id", sessionID).Msg("Session not found when starting keepalive")
		return
	}
	now := time.Now()
	session.KeepaliveStatus = "starting"
	session.KeepaliveStartTime = &now
	w.mutex.Unlock()

	// Retry configuration
	maxRetries := 5
	retryDelay := 5 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Info().
				Str("session_id", sessionID).
				Int("attempt", attempt+1).
				Int("max_retries", maxRetries).
				Msg("Retrying keepalive connection")

			w.mutex.Lock()
			if session, exists := w.sessions[sessionID]; exists {
				session.KeepaliveStatus = "reconnecting"
			}
			w.mutex.Unlock()

			time.Sleep(retryDelay)
		}

		// Attempt connection
		err := w.connectKeepaliveWebSocket(ctx, sessionID, lobbyID, lobbyPIN)
		if err == nil {
			// Success - WebSocket is running
			return
		}

		log.Error().
			Err(err).
			Str("session_id", sessionID).
			Int("attempt", attempt+1).
			Msg("Keepalive connection failed")

		// Store the error for the last attempt
		if attempt == maxRetries-1 {
			w.mutex.Lock()
			if session, exists := w.sessions[sessionID]; exists {
				session.KeepaliveError = err.Error()
			}
			w.mutex.Unlock()
		}
	}

	// All retries exhausted
	log.Error().
		Str("session_id", sessionID).
		Str("lobby_id", lobbyID).
		Msg("Keepalive session failed after all retries")

	w.mutex.Lock()
	if session, exists := w.sessions[sessionID]; exists {
		session.KeepaliveStatus = "failed"
		checkTime := time.Now()
		session.KeepaliveLastCheck = &checkTime
		// KeepaliveError already set in the retry loop
	}
	w.mutex.Unlock()
}

// connectKeepaliveWebSocket establishes WebSocket connection to moonlight-web
func (w *WolfExecutor) connectKeepaliveWebSocket(ctx context.Context, sessionID, lobbyID, lobbyPIN string) error {
	moonlightWebURL := os.Getenv("MOONLIGHT_WEB_URL")
	if moonlightWebURL == "" {
		moonlightWebURL = "http://moonlight-web:8080" // Default internal URL
	}

	// Build WebSocket URL (moonlight-web expects /api/host/stream endpoint)
	wsURL := strings.Replace(moonlightWebURL, "http://", "ws://", 1) + "/api/host/stream"

	log.Info().
		Str("session_id", sessionID).
		Str("ws_url", wsURL).
		Str("lobby_id", lobbyID).
		Msg("Connecting keepalive WebSocket to moonlight-web")

	// Connect WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}
	defer conn.Close()

	// Set up ping/pong handler to keep connection alive and update timestamp
	// This ensures KeepaliveLastCheck is updated even if no data messages are received
	conn.SetPongHandler(func(appData string) error {
		log.Debug().
			Str("session_id", sessionID).
			Msg("Keepalive received pong, updating timestamp")

		w.mutex.Lock()
		if session, exists := w.sessions[sessionID]; exists {
			checkTime := time.Now()
			session.KeepaliveLastCheck = &checkTime
			log.Debug().
				Str("session_id", sessionID).
				Time("last_check", checkTime).
				Msg("Updated KeepaliveLastCheck via pong handler")
		}
		w.mutex.Unlock()
		return nil
	})

	// NEW: With moonlight-web session persistence, we no longer need Wolf UI app
	// Just connect with keepalive mode and the session will persist without WebRTC peer

	// Look up the Wolf app ID for this lobby by extracting from session
	// For external agents created in lobby mode, they have a dedicated Wolf app per lobby
	w.mutex.RLock()
	_, sessionExists := w.sessions[sessionID]
	w.mutex.RUnlock()

	if !sessionExists {
		return fmt.Errorf("session not found when starting keepalive")
	}

	// Get the Wolf apps to find the app for this lobby
	apps, err := w.wolfClient.ListApps(ctx)
	if err != nil {
		return fmt.Errorf("failed to list Wolf apps: %w", err)
	}

	// Find app by matching container name prefix
	var wolfAppID uint32
	sessionIDSuffix := sessionID[len(sessionID)-4:] // Last 4 chars
	for _, app := range apps {
		if strings.Contains(app.Title, sessionIDSuffix) {
			// Parse app ID as uint32
			fmt.Sscanf(app.ID, "%d", &wolfAppID)
			log.Info().
				Str("wolf_app_id", app.ID).
				Str("app_title", app.Title).
				Msg("Found Wolf app for keepalive session")
			break
		}
	}

	if wolfAppID == 0 {
		return fmt.Errorf("Wolf app not found for session %s", sessionID)
	}

	// Send AuthenticateAndInit message with session persistence
	// mode=keepalive: creates session without WebRTC peer (headless)
	authMsg := map[string]interface{}{
		"AuthenticateAndInit": map[string]interface{}{
			"credentials":                "helix",                          // From moonlight-web config
			"session_id":                 fmt.Sprintf("agent-%s", sessionID), // NEW: persistent session ID
			"mode":                       "keepalive",                      // NEW: keepalive mode (no WebRTC)
			"host_id":                    0,                                // Local Wolf instance
			"app_id":                     wolfAppID,                        // Connect to lobby's Wolf app
			"bitrate":                    5000,                             // Minimal bitrate for keepalive
			"packet_size":                1024,
			"fps":                        30,
			"width":                      1280,
			"height":                     720,
			"video_sample_queue_size":    10,
			"play_audio_local":           false,
			"audio_sample_queue_size":    10,
			"video_supported_formats":    1,     // H264 only
			"video_colorspace":           "Rec709", // String format for new API
			"video_color_range_full":     false,
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
		Msg("Sent keepalive auth message to moonlight-web with session persistence")

	// NEW: With session persistence, moonlight-web handles the streamer lifecycle
	// In keepalive mode, it will:
	// 1. Create streamer if session doesn't exist
	// 2. Close WebSocket immediately (we don't need WebRTC for keepalive)
	// 3. Keep streamer running to Wolf without any WebRTC peer
	// 4. Discard frames without sending anywhere
	// This is MUCH simpler than the old lobby join logic!

	// Wait for stream initialization or error messages
	maxWaitTime := 10 * time.Second
	startTime := time.Now()
	connected := false

	for time.Since(startTime) < maxWaitTime {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			// Timeout is expected - moonlight-web closes WebSocket in keepalive mode
			if err.Error() == "i/o timeout" {
				// Check if enough time passed for session creation
				if time.Since(startTime) > 3*time.Second {
					log.Info().
						Str("session_id", sessionID).
						Msg("Keepalive WebSocket closed as expected - session running headless")
					connected = true
					break
				}
				continue
			}
			// WebSocket closed normally
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Info().
					Str("session_id", sessionID).
					Msg("Keepalive WebSocket closed normally - session persisting")
				connected = true
				break
			}
			return fmt.Errorf("WebSocket error: %w", err)
		}

		// Parse server message
		var serverMsg map[string]interface{}
		if err := json.Unmarshal(message, &serverMsg); err != nil {
			log.Debug().
				Str("session_id", sessionID).
				Str("raw_message", string(message)).
				Msg("Received non-JSON message")
			continue
		}

		log.Debug().
			Str("session_id", sessionID).
			Interface("message", serverMsg).
			Msg("Received initialization message")

		// Check for error messages
		if msgType, ok := serverMsg["type"].(string); ok {
			if msgType == "HostNotFound" || msgType == "HostNotPaired" || msgType == "InternalServerError" {
				return fmt.Errorf("moonlight-web error: %s", msgType)
			}
			if msgType == "ConnectionComplete" {
				log.Info().
					Str("session_id", sessionID).
					Msg("Keepalive stream connected successfully")
				connected = true
				// Keep reading until WebSocket closes
			}
		}
	}

	if !connected {
		return fmt.Errorf("keepalive session failed to initialize within timeout")
	}

	log.Info().
		Str("session_id", sessionID).
		Msg("Keepalive session established - running headless in moonlight-web")

	// Update status to active
	// NEW: With session persistence, we don't need to keep the WebSocket open!
	// The session persists in moonlight-web and the streamer runs headless
	w.mutex.Lock()
	if session, exists := w.sessions[sessionID]; exists {
		session.KeepaliveStatus = "active"
		checkTime := time.Now()
		session.KeepaliveLastCheck = &checkTime
	}
	w.mutex.Unlock()

	log.Info().
		Str("session_id", sessionID).
		Msg("Keepalive session fully established - streamer running headless in moonlight-web")

	return nil
}

// idleExternalAgentCleanupLoop runs periodically to cleanup idle SpecTask external agents
// Terminates external agents after 30min of inactivity across ALL sessions
func (w *WolfExecutor) idleExternalAgentCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Check every 5 minutes
	defer ticker.Stop()

	log.Info().Msg("Starting idle external agent cleanup loop (30min timeout)")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Idle external agent cleanup loop stopped")
			return

		case <-ticker.C:
			w.cleanupIdleExternalAgents(ctx)
		}
	}
}

// cleanupIdleExternalAgents terminates external agents that have been idle for >30min
func (w *WolfExecutor) cleanupIdleExternalAgents(ctx context.Context) {
	cutoff := time.Now().Add(-30 * time.Minute)

	// Get idle external agents (not individual sessions - entire agents)
	idleAgents, err := w.store.GetIdleExternalAgents(ctx, cutoff, []string{"spectask"})
	if err != nil {
		log.Error().Err(err).Msg("Failed to get idle external agents")
		return
	}

	if len(idleAgents) == 0 {
		return // No idle agents to clean up
	}

	log.Info().
		Int("count", len(idleAgents)).
		Time("cutoff", cutoff).
		Msg("Found idle SpecTask external agents to terminate")

	for _, activity := range idleAgents {
		log.Info().
			Str("external_agent_id", activity.ExternalAgentID).
			Str("spectask_id", activity.SpecTaskID).
			Str("wolf_app_id", activity.WolfAppID).
			Time("last_interaction", activity.LastInteraction).
			Dur("idle_duration", time.Since(activity.LastInteraction)).
			Msg("Terminating idle SpecTask external agent")

		// Stop Wolf app (terminates Zed container, frees GPU)
		err := w.wolfClient.RemoveApp(ctx, activity.WolfAppID)
		if err != nil {
			log.Error().
				Err(err).
				Str("wolf_app_id", activity.WolfAppID).
				Str("external_agent_id", activity.ExternalAgentID).
				Msg("Failed to remove idle Wolf app")
			// Continue with cleanup even if Wolf removal fails
		}

		// Update external agent status to terminated
		agent, err := w.store.GetSpecTaskExternalAgentByID(ctx, activity.ExternalAgentID)
		if err == nil {
			agent.Status = "terminated"
			agent.LastActivity = time.Now()
			err = w.store.UpdateSpecTaskExternalAgent(ctx, agent)
			if err != nil {
				log.Error().Err(err).Msg("Failed to update external agent status to terminated")
			}

			// Update ALL affected Helix sessions to mark external agent as terminated
			for _, sessionID := range agent.HelixSessionIDs {
				session, err := w.store.GetSession(ctx, sessionID)
				if err == nil {
					// Update session metadata to indicate external agent is terminated
					session.Metadata.ExternalAgentStatus = "terminated_idle"
					_, err = w.store.UpdateSession(ctx, *session)
					if err != nil {
						log.Error().
							Err(err).
							Str("session_id", sessionID).
							Msg("Failed to update session with terminated status")
					}
				}
			}
		}

		// Delete activity record
		err = w.store.DeleteExternalAgentActivity(ctx, activity.ExternalAgentID)
		if err != nil {
			log.Error().
				Err(err).
				Str("external_agent_id", activity.ExternalAgentID).
				Msg("Failed to delete external agent activity record")
		}

		log.Info().
			Str("external_agent_id", activity.ExternalAgentID).
			Str("workspace_dir", activity.WorkspaceDir).
			Msg("External agent terminated successfully, workspace preserved in filestore")
	}

	log.Info().
		Int("terminated_count", len(idleAgents)).
		Msg("Completed idle external agent cleanup")
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
