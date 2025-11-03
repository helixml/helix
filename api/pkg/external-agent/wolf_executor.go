package external_agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
	wolfClient WolfClientInterface
	store      store.Store
	sessions   map[string]*ZedSession
	mutex      sync.RWMutex

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
	ExtraMounts       []string // Additional directory mounts (e.g., internal project repo)
	StartupScript     string   // Optional project startup script to run before Zed starts
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

	// Add project startup script if provided
	if config.StartupScript != "" {
		env = append(env, fmt.Sprintf("HELIX_PROJECT_STARTUP_SCRIPT=%s", config.StartupScript))
	}

	// Add any extra environment variables
	env = append(env, config.ExtraEnv...)

	// Build standard mounts (common to all Sway apps)
	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", config.WorkspaceDir),
		"/var/run/docker.sock:/var/run/docker.sock",
	}

	// Development mode: mount host files for hot-reloading
	// Production mode: use files baked into helix-sway image
	if os.Getenv("HELIX_DEV_MODE") == "true" {
		helixHostHome := os.Getenv("HELIX_HOST_HOME")
		log.Info().
			Str("helix_host_home", helixHostHome).
			Msg("HELIX_DEV_MODE enabled - mounting dev files from host for hot-reloading")

		mounts = append(mounts,
			fmt.Sprintf("%s/zed-build:/zed-build:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/start-zed-helix.sh:/usr/local/bin/start-zed-helix.sh:ro", helixHostHome),
		)
	} else {
		log.Debug().Msg("Production mode - using files baked into helix-sway image")
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

	// Add extra mounts (e.g., internal project repo)
	mounts = append(mounts, config.ExtraMounts...)

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
// WOLF_MODE=lobbies (default) - lobbies persist naturally, no keepalive needed
// WOLF_MODE=apps - requires keepalive sessions to prevent stale buffer crashes
func NewWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, store store.Store, wsChecker WebSocketConnectionChecker) Executor {
	wolfMode := os.Getenv("WOLF_MODE")
	if wolfMode == "" {
		wolfMode = "lobbies" // Default to lobbies - simpler, no keepalive needed
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
			Msg("Wolf executor initialized with HELIX_HOST_HOME (dev mode)")
	} else {
		log.Info().Msg("Wolf executor initialized (production mode - using files baked into image)")
	}

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

	// Lobbies mode doesn't need health monitoring or reconciliation
	// Lobbies persist naturally across Wolf restarts

	// Start idle external agent cleanup loop (30min timeout)
	go executor.idleExternalAgentCleanupLoop(context.Background())

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

	// Determine workspace directory - use task-scoped for SpecTasks, session-scoped otherwise
	workspaceDir := agent.WorkDir
	if workspaceDir == "" {
		if agent.SpecTaskID != "" {
			// SpecTask agents share workspace across planning and implementation
			workspaceDir = filepath.Join(w.workspaceBasePath, "spec-tasks", agent.SpecTaskID)
			log.Info().
				Str("spec_task_id", agent.SpecTaskID).
				Str("workspace_dir", workspaceDir).
				Msg("Using task-scoped workspace for SpecTask agent")
		} else {
			// Regular external agents use session-scoped workspace
			workspaceDir = filepath.Join(w.workspaceBasePath, "external-agents", agent.SessionID)
		}
	}

	// Create workspace directory if it doesn't exist
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Load project startup script - either from SpecTask's project or direct ProjectID (exploratory sessions)
	projectStartupScript := ""
	projectIDToLoad := agent.ProjectID // Direct project ID for exploratory sessions

	if agent.SpecTaskID != "" {
		// Load the SpecTask to get project ID
		specTask, err := w.store.GetSpecTask(ctx, agent.SpecTaskID)
		if err != nil {
			log.Warn().Err(err).Str("spec_task_id", agent.SpecTaskID).Msg("Failed to get SpecTask for startup script, continuing without it")
		} else if specTask.ProjectID != "" {
			projectIDToLoad = specTask.ProjectID
		}
	}

	// Load project and internal repo if we have a project ID
	var projectInternalRepoPath string
	if projectIDToLoad != "" {
		project, err := w.store.GetProject(ctx, projectIDToLoad)
		if err != nil {
			log.Warn().Err(err).Str("project_id", projectIDToLoad).Msg("Failed to get Project for startup script, continuing without it")
		} else {
			if project.StartupScript != "" {
				projectStartupScript = project.StartupScript
				log.Info().
					Str("project_id", project.ID).
					Str("spec_task_id", agent.SpecTaskID).
					Str("direct_project_id", agent.ProjectID).
					Msg("Loaded project startup script for agent")
			}

			// Store internal repo path for mounting
			if project.InternalRepoPath != "" {
				projectInternalRepoPath = project.InternalRepoPath
				log.Info().
					Str("project_id", project.ID).
					Str("internal_repo_path", project.InternalRepoPath).
					Msg("Will mount internal project repository in agent workspace")
			}
		}
	}

	// Clone git repositories if specified (for SpecTasks with repository context)
	if len(agent.RepositoryIDs) > 0 {
		err := w.setupGitRepositories(ctx, workspaceDir, agent.RepositoryIDs, agent.PrimaryRepositoryID)
		if err != nil {
			log.Error().Err(err).Msg("Failed to setup git repositories")
			return nil, fmt.Errorf("failed to setup git repositories: %w", err)
		}
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
		displayWidth = 3840 // 4K default for consistent resolution
	}
	displayHeight := agent.DisplayHeight
	if displayHeight == 0 {
		displayHeight = 2160 // 4K default for consistent resolution
	}
	displayRefreshRate := agent.DisplayRefreshRate
	if displayRefreshRate == 0 {
		displayRefreshRate = 60
	}

	// Build extra mounts for internal project repo
	extraMounts := []string{}
	if projectInternalRepoPath != "" {
		// Mount internal repo at /home/retro/work/.helix-project (read-only)
		internalRepoMount := fmt.Sprintf("%s:/home/retro/work/.helix-project:ro", projectInternalRepoPath)
		extraMounts = append(extraMounts, internalRepoMount)
		log.Info().
			Str("internal_repo_path", projectInternalRepoPath).
			Msg("Mounting internal project repository in agent workspace")
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
			VideoProducerBufferCaps: "video/x-raw(memory:CUDAMemory)", // Match Wolf UI's CUDA memory type
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
			ExtraMounts:       extraMounts,        // Mount internal project repo
			StartupScript:     projectStartupScript, // Project startup script from database
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

	// Update Helix session metadata with lobby ID and PIN so frontend can display them
	if helixSessionID != "" {
		helixSession, err := w.store.GetSession(ctx, helixSessionID)
		if err != nil {
			log.Warn().Err(err).Str("helix_session_id", helixSessionID).Msg("Failed to get Helix session for lobby metadata update")
		} else {
			// Update session metadata with Wolf lobby information
			helixSession.Metadata.WolfLobbyID = lobbyResp.LobbyID
			helixSession.Metadata.WolfLobbyPIN = lobbyPINString

			_, err = w.store.UpdateSession(ctx, *helixSession)
			if err != nil {
				log.Error().Err(err).Str("helix_session_id", helixSessionID).Msg("Failed to update Helix session with lobby metadata")
			} else {
				log.Info().
					Str("helix_session_id", helixSessionID).
					Str("lobby_id", lobbyResp.LobbyID).
					Str("lobby_pin", lobbyPINString).
					Msg("Updated Helix session metadata with Wolf lobby ID and PIN")
			}
		}
	}

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

	// Track activity for idle cleanup (all external agent types)
	// Determine agent type based on session metadata
	agentType := "agent" // Default for regular agent sessions
	if agent.ProjectID != "" {
		agentType = "exploratory" // Exploratory sessions have project ID
	}
	if agent.SpecTaskID != "" {
		agentType = "spectask" // SpecTask agents have spec task ID
	}

	err = w.store.UpsertExternalAgentActivity(ctx, &types.ExternalAgentActivity{
		ExternalAgentID: agent.SessionID,
		SpecTaskID:      agent.SpecTaskID, // May be empty for non-SpecTask agents
		LastInteraction: time.Now(),
		AgentType:       agentType,
		WolfAppID:       response.WolfAppID,
		WorkspaceDir:    workspaceDir,
		UserID:          agent.UserID,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", agent.SessionID).
			Str("agent_type", agentType).
			Msg("Failed to track external agent activity - cleanup won't work for this session")
		// Non-fatal - session is already created
	} else {
		log.Info().
			Str("session_id", agent.SessionID).
			Str("agent_type", agentType).
			Msg("External agent activity tracked for idle cleanup")
	}

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

	// Try in-memory map first (for recently created sessions)
	session, exists := w.sessions[sessionID]
	var wolfLobbyID string
	var wolfLobbyPIN string

	if exists {
		// Found in memory - use cached lobby ID
		wolfLobbyID = session.WolfLobbyID
	}

	// Always fetch from database to get lobby ID and PIN (handles sessions created before restart)
	dbSession, err := w.store.GetSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session from database for stop")
		return fmt.Errorf("session %s not found in database", sessionID)
	}

	// Use database lobby ID if we don't have it from memory
	if wolfLobbyID == "" {
		wolfLobbyID = dbSession.Metadata.WolfLobbyID
	}

	// Get PIN from database
	wolfLobbyPIN = dbSession.Metadata.WolfLobbyPIN

	// Save final screenshot before stopping (for paused state preview)
	screenshotPath := ""
	containerName := fmt.Sprintf("zed-external-%s_%s", sessionID, wolfLobbyID)
	screenshotBytes, err := w.getContainerScreenshot(ctx, containerName)
	if err == nil && len(screenshotBytes) > 0 {
		// Save to filestore
		screenshotPath = filepath.Join(w.workspaceBasePath, "paused-screenshots", fmt.Sprintf("%s.png", sessionID))
		screenshotDir := filepath.Dir(screenshotPath)
		if err := os.MkdirAll(screenshotDir, 0755); err == nil {
			if err := os.WriteFile(screenshotPath, screenshotBytes, 0644); err == nil {
				// Update session metadata with paused screenshot path
				dbSession.Metadata.PausedScreenshotPath = screenshotPath
				_, err = w.store.UpdateSession(ctx, *dbSession)
				if err != nil {
					log.Error().Err(err).Msg("Failed to update session with paused screenshot path")
				}

				log.Info().
					Str("session_id", sessionID).
					Str("screenshot_path", screenshotPath).
					Msg("Saved final screenshot for paused state preview")
			}
		}
	} else {
		log.Debug().Str("session_id", sessionID).Msg("Could not capture final screenshot (agent may already be stopped)")
	}

	// CRITICAL: Stop Wolf-UI streaming sessions BEFORE stopping lobby
	// Wolf-UI sessions persist after lobby stops, consuming 245MB GPU memory each
	// Query all sessions and stop ones matching this Helix session ID
	sessions, err := w.wolfClient.ListSessions(ctx)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to list Wolf sessions for cleanup - will skip session cleanup")
	} else {
		sessionPrefix := fmt.Sprintf("helix-agent-%s-", sessionID)
		stoppedCount := 0

		for _, session := range sessions {
			// Match sessions by client_unique_id prefix (handles multiple browser tabs)
			if session.ClientUniqueID != "" && strings.HasPrefix(session.ClientUniqueID, sessionPrefix) {
				log.Info().
					Str("client_id", session.ClientID).
					Str("client_unique_id", session.ClientUniqueID).
					Str("session_id", sessionID).
					Msg("Stopping Wolf-UI streaming session before lobby teardown")

				err := w.wolfClient.StopSession(ctx, session.ClientID)
				if err != nil {
					log.Warn().
						Err(err).
						Str("client_id", session.ClientID).
						Msg("Failed to stop Wolf-UI session (will be orphaned)")
				} else {
					stoppedCount++
					log.Info().
						Str("client_id", session.ClientID).
						Msg("✅ Stopped Wolf-UI streaming session")
				}
			}
		}

		if stoppedCount > 0 {
			log.Info().
				Str("session_id", sessionID).
				Int("stopped_count", stoppedCount).
				Msg("Cleaned up Wolf-UI streaming sessions")
		}
	}

	// Stop the lobby (tears down Zed container)
	if wolfLobbyID != "" {
		// CRITICAL: Must provide PIN to stop lobby
		var lobbyPIN []int16

		if wolfLobbyPIN != "" && len(wolfLobbyPIN) == 4 {
			// PIN is stored as "1234" string, convert to [1,2,3,4] slice
			lobbyPIN = make([]int16, 4)
			for i, ch := range wolfLobbyPIN {
				lobbyPIN[i] = int16(ch - '0')
			}
			log.Debug().
				Str("lobby_id", wolfLobbyID).
				Str("lobby_pin", wolfLobbyPIN).
				Msg("Retrieved lobby PIN from database for stop request")
		}

		stopReq := &wolf.StopLobbyRequest{
			LobbyID: wolfLobbyID,
			PIN:     lobbyPIN, // CRITICAL: Wolf requires PIN to stop lobbies
		}
		err = w.wolfClient.StopLobby(ctx, stopReq)
		if err != nil {
			log.Error().
				Err(err).
				Str("lobby_id", wolfLobbyID).
				Interface("lobby_pin", lobbyPIN).
				Msg("Failed to stop Wolf lobby")
			// Continue with cleanup even if stop fails
		} else {
			log.Info().
				Str("lobby_id", wolfLobbyID).
				Str("session_id", sessionID).
				Msg("Wolf lobby stopped successfully")
		}
	} else {
		log.Warn().
			Str("session_id", sessionID).
			Msg("No Wolf lobby ID found in database - session may not have external agent running")
		return fmt.Errorf("no Wolf lobby ID found for session %s", sessionID)
	}

	// Update in-memory session status if it exists
	if exists {
		session.Status = "stopped"
	}

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
		fmt.Sprintf("%s:/home/retro/work", workspaceDir), // Mount persistent workspace
	}

	// Development mode: mount host files for hot-reloading
	// Production mode: use files baked into helix-sway image
	if os.Getenv("HELIX_DEV_MODE") == "true" {
		helixHostHome := os.Getenv("HELIX_HOST_HOME")
		mounts = append(mounts,
			fmt.Sprintf("%s/zed-build:/zed-build:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro", helixHostHome),
			fmt.Sprintf("%s/wolf/sway-config/start-zed-helix.sh:/usr/local/bin/start-zed-helix.sh:ro", helixHostHome),
		)
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
// Note: This type-asserts the interface back to the concrete type.
// Only use this when you need direct access to wolf.Client specific methods.
func (w *WolfExecutor) GetWolfClient() *wolf.Client {
	if client, ok := w.wolfClient.(*wolf.Client); ok {
		return client
	}
	// This should never happen in production, only in tests with mocks
	log.Warn().Msg("GetWolfClient called but wolfClient is not *wolf.Client (likely a test mock)")
	return nil
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
// Terminates external agents after 1min of inactivity (for testing, will be 30min in production)
// Timeout is database-based (checks LastInteraction timestamp), so it survives API restarts
func (w *WolfExecutor) idleExternalAgentCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds for faster cleanup in dev mode
	defer ticker.Stop()

	log.Info().Msg("Starting idle external agent cleanup loop (5min timeout, checks every 30s)")

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

// cleanupIdleExternalAgents terminates external agents that have been idle for >5min (development)
func (w *WolfExecutor) cleanupIdleExternalAgents(ctx context.Context) {
	cutoff := time.Now().Add(-5 * time.Minute) // 5 minute idle threshold for development

	// Get idle external agents (SpecTask, exploratory, and regular agent sessions)
	idleAgents, err := w.store.GetIdleExternalAgents(ctx, cutoff, []string{"spectask", "exploratory", "agent"})
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
		Msg("Found idle external agents to terminate (SpecTask + exploratory)")

	for _, activity := range idleAgents {
		log.Info().
			Str("external_agent_id", activity.ExternalAgentID).
			Str("agent_type", activity.AgentType).
			Str("spectask_id_or_project_id", activity.SpecTaskID).
			Str("wolf_app_id", activity.WolfAppID).
			Time("last_interaction", activity.LastInteraction).
			Dur("idle_duration", time.Since(activity.LastInteraction)).
			Msg("Terminating idle external agent")

		// Map external_agent_id to session ID based on agent type:
		// - exploratory/agent: external_agent_id IS the Helix session ID
		// - spectask: Need to look up session from SpecTaskExternalAgent record
		sessionIDToStop := ""
		if activity.AgentType == "exploratory" || activity.AgentType == "agent" {
			sessionIDToStop = activity.ExternalAgentID // Session ID is the agent ID
		} else if activity.AgentType == "spectask" {
			// SpecTask: Look up which Helix session is associated with this agent
			agent, err := w.store.GetSpecTaskExternalAgentByID(ctx, activity.ExternalAgentID)
			if err == nil && len(agent.HelixSessionIDs) > 0 {
				sessionIDToStop = agent.HelixSessionIDs[0] // Use first session
			}
		}

		if sessionIDToStop != "" {
			// Properly stop the agent (stops lobby, terminates containers)
			err := w.StopZedAgent(ctx, sessionIDToStop)
			if err != nil {
				log.Error().
					Err(err).
					Str("session_id", sessionIDToStop).
					Str("external_agent_id", activity.ExternalAgentID).
					Msg("Failed to stop idle Zed agent")
				// Continue with cleanup even if stop fails
			}
		} else {
			log.Warn().
				Str("external_agent_id", activity.ExternalAgentID).
				Msg("No session ID found for idle agent, skipping container cleanup")
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

// setupGitRepositories clones git repositories and sets up design docs worktree for SpecTask agents
func (w *WolfExecutor) setupGitRepositories(ctx context.Context, workspaceDir string, repositoryIDs []string, primaryRepositoryID string) error {
	log.Info().
		Str("workspace_dir", workspaceDir).
		Strs("repository_ids", repositoryIDs).
		Str("primary_repository_id", primaryRepositoryID).
		Msg("Setting up git repositories for external agent")

	// Clone each repository
	for _, repoID := range repositoryIDs {
		if repoID == "" {
			continue // Skip empty repository IDs
		}

		// Get repository details from database
		repo, err := w.store.GetGitRepository(ctx, repoID)
		if err != nil {
			log.Error().Err(err).Str("repository_id", repoID).Msg("Failed to get repository details")
			return fmt.Errorf("failed to get repository %s: %w", repoID, err)
		}

		// Determine clone directory - use repository name
		cloneDir := filepath.Join(workspaceDir, repo.Name)

		// Check if repository already exists
		if _, err := os.Stat(filepath.Join(cloneDir, ".git")); err == nil {
			log.Info().
				Str("repository_id", repoID).
				Str("clone_dir", cloneDir).
				Msg("Repository already cloned, skipping")

			// For primary repository, ensure design docs worktree exists
			if repoID == primaryRepositoryID {
				err := w.setupDesignDocsWorktree(cloneDir, repo.Name)
				if err != nil {
					log.Error().Err(err).Msg("Failed to setup design docs worktree for existing repo")
					// Continue - not fatal
				}
			}
			continue
		}

		log.Info().
			Str("repository_id", repoID).
			Str("repository_url", repo.CloneURL).
			Str("clone_dir", cloneDir).
			Msg("Cloning git repository")

		// Clone the repository
		// Use git clone with --bare for the main repo, then set up worktrees
		cloneCmd := fmt.Sprintf("git clone %s %s", repo.CloneURL, cloneDir)
		output, err := execCommand(ctx, workspaceDir, "bash", "-c", cloneCmd)
		if err != nil {
			log.Error().
				Err(err).
				Str("output", output).
				Str("repository_id", repoID).
				Msg("Failed to clone repository")
			return fmt.Errorf("failed to clone repository %s: %w", repo.Name, err)
		}

		log.Info().
			Str("repository_id", repoID).
			Str("clone_dir", cloneDir).
			Msg("Successfully cloned git repository")

		// For primary repository, set up design docs worktree
		if repoID == primaryRepositoryID {
			err := w.setupDesignDocsWorktree(cloneDir, repo.Name)
			if err != nil {
				log.Error().Err(err).Msg("Failed to setup design docs worktree")
				return fmt.Errorf("failed to setup design docs worktree: %w", err)
			}
		}
	}

	return nil
}

// setupDesignDocsWorktree creates a git worktree for design documents on helix-design-docs branch
func (w *WolfExecutor) setupDesignDocsWorktree(repoPath, repoName string) error {
	worktreePath := filepath.Join(repoPath, ".git-worktrees", "helix-design-docs")

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		log.Info().
			Str("worktree_path", worktreePath).
			Msg("Design docs worktree already exists, skipping")
		return nil
	}

	log.Info().
		Str("repo_path", repoPath).
		Str("worktree_path", worktreePath).
		Msg("Setting up design docs git worktree")

	ctx := context.Background()

	// Check if helix-design-docs branch exists remotely
	checkBranchCmd := "git ls-remote --heads origin helix-design-docs"
	output, err := execCommand(ctx, repoPath, "bash", "-c", checkBranchCmd)
	branchExists := err == nil && output != ""

	if !branchExists {
		// Create orphan branch for design docs (forward-only, no shared history)
		log.Info().
			Str("repo_name", repoName).
			Msg("Creating new helix-design-docs orphan branch")

		createBranchCmd := `
			git checkout --orphan helix-design-docs && \
			git rm -rf . && \
			echo "# Helix Design Documents" > README.md && \
			echo "" >> README.md && \
			echo "This branch contains design documents generated by Helix agents." >> README.md && \
			echo "Documents are organized by task in tasks/ directory." >> README.md && \
			mkdir -p tasks && \
			git add README.md && \
			git commit -m "Initialize helix-design-docs branch" && \
			git push -u origin helix-design-docs && \
			git checkout main
		`
		_, err := execCommand(ctx, repoPath, "bash", "-c", createBranchCmd)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create helix-design-docs branch")
			return fmt.Errorf("failed to create helix-design-docs branch: %w", err)
		}

		log.Info().Msg("Successfully created helix-design-docs branch")
	}

	// Create worktree directory
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	// Add worktree
	addWorktreeCmd := fmt.Sprintf("git worktree add %s helix-design-docs", worktreePath)
	_, err = execCommand(ctx, repoPath, "bash", "-c", addWorktreeCmd)
	if err != nil {
		log.Error().Err(err).Msg("Failed to add git worktree")
		return fmt.Errorf("failed to add git worktree: %w", err)
	}

	log.Info().
		Str("worktree_path", worktreePath).
		Msg("Successfully set up design docs git worktree")

	return nil
}

// execCommand executes a command in the specified directory and returns output
func execCommand(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// getContainerScreenshot fetches a screenshot from the container's screenshot server
func (w *WolfExecutor) getContainerScreenshot(ctx context.Context, containerName string) ([]byte, error) {
	screenshotURL := fmt.Sprintf("http://%s:9876/screenshot", containerName)

	screenshotReq, err := http.NewRequestWithContext(ctx, "GET", screenshotURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create screenshot request: %w", err)
	}

	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	screenshotResp, err := httpClient.Do(screenshotReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get screenshot from container: %w", err)
	}
	defer screenshotResp.Body.Close()

	if screenshotResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("screenshot server returned status %d", screenshotResp.StatusCode)
	}

	// Read screenshot bytes
	screenshotBytes, err := io.ReadAll(screenshotResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read screenshot data: %w", err)
	}

	return screenshotBytes, nil
}
