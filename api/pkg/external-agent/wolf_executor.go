package external_agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
)

// extractHostPortAndTLS parses a URL and returns host:port and whether TLS is needed
// Used to configure Zed's WebSocket connection to the API
func extractHostPortAndTLS(rawURL string) (hostPort string, useTLS bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Fallback: assume it's already host:port
		return rawURL, false
	}

	useTLS = parsed.Scheme == "https" || parsed.Scheme == "wss"
	hostPort = parsed.Host

	// Add default port if missing
	if !strings.Contains(hostPort, ":") {
		if useTLS {
			hostPort = hostPort + ":443"
		} else {
			hostPort = hostPort + ":80"
		}
	}

	return hostPort, useTLS
}

// lobbyCacheEntry represents a cached lobby lookup result
type lobbyCacheEntry struct {
	lobbyID   string
	timestamp time.Time
}

var _ Executor = &WolfExecutor{}

// WolfExecutor implements the Executor interface using Wolf API
type WolfExecutor struct {
	store    store.Store
	sessions map[string]*ZedSession
	mutex    sync.RWMutex

	// Zed configuration
	zedImage      string
	helixAPIURL   string
	helixAPIToken string

	// Workspace configuration for dev stack
	// CRITICAL: We need TWO paths because API container and Wolf/host see different paths:
	// - workspaceBasePathForCloning: Path inside API container where we git clone repos
	// - workspaceBasePathForMounting: Absolute host path that Wolf uses to mount into containers
	workspaceBasePathForCloning  string // e.g., /filestore/workspaces (inside API container)
	workspaceBasePathForMounting string // e.g., /var/lib/docker/volumes/helix_helix-filestore/_data/workspaces (on host)

	// Cache for lobby lookups to prevent Wolf API spam
	lobbyCache      map[string]*lobbyCacheEntry
	lobbyCacheMutex sync.RWMutex
	lobbyCacheTTL   time.Duration

	// Per-session locks to prevent concurrent lobby creation for same session
	creationLocks      map[string]*sync.Mutex
	creationLocksMutex sync.Mutex

	// Track if GPU monitoring (nvidia-smi/rocm-smi) has ever worked (avoid false alarms on systems without GPU monitoring)
	hasSeenValidGPUStats bool
	gpuStatsMutex        sync.RWMutex

	// Wolf scheduler for multi-Wolf distributed deployment
	wolfScheduler *store.WolfScheduler

	// Connection manager for RevDial connections to sandboxes and remote Wolf instances
	connman connmanInterface

	// Max concurrent lobbies (GPU streaming sessions) - configurable via EXTERNAL_AGENTS_MAX_CONCURRENT_LOBBIES
	maxConcurrentLobbies int

	// Hydra multi-Docker isolation
	// When enabled, each scope (SpecTask/session/exploratory) gets its own dockerd
	hydraEnabled bool
}

// connmanInterface defines the interface for RevDial connection management
type connmanInterface interface {
	Dial(ctx context.Context, deviceID string) (net.Conn, error)
}

// getWolfClient returns a Wolf client for the specified instance
// Uses RevDial to connect to Wolf API (Wolf runs in separate container/machine)
// IMPORTANT: wolfInstanceID should ALWAYS be provided - there is no valid default
// In multi-Wolf mode, each session is scheduled to a specific Wolf instance stored in the database
func (w *WolfExecutor) getWolfClient(wolfInstanceID string) WolfClientInterface {
	if wolfInstanceID == "" {
		// This is a programming error - callers must always provide the Wolf instance ID
		// by looking it up from the session or other context
		panic("getWolfClient called without wolfInstanceID - this is a bug, all callers must look up the Wolf instance from session/database")
	}
	return wolf.NewRevDialClient(w.connman, wolfInstanceID)
}

// translateToHostPath converts API container path to absolute host path for Wolf mounting
// API container sees: /filestore/workspaces/...
// Host sees: /var/lib/docker/volumes/helix_helix-filestore/_data/workspaces/...
func (w *WolfExecutor) translateToHostPath(containerPath string) string {
	// Replace /filestore with the absolute host volume path
	if strings.HasPrefix(containerPath, "/filestore/") {
		relativePath := strings.TrimPrefix(containerPath, "/filestore/")
		// workspaceBasePathForMounting already includes "workspaces" from the volume root
		// e.g., /var/lib/docker/volumes/.../workspaces
		// So we need to go up to the volume root first
		volumeRoot := filepath.Dir(w.workspaceBasePathForMounting)
		return filepath.Join(volumeRoot, relativePath)
	}
	return containerPath
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
	UserID            string // User ID for SSH key mounting
	SessionID         string // Session ID for settings sync daemon
	WorkspaceDir      string
	ExtraEnv          []string
	ExtraMounts []string // Additional directory mounts
	ZedImage    string   // Override for helix-sway image (uses instance's SwayVersion)
	// NOTE: Startup script lives in primary code repo at .helix/startup.sh
	DisplayWidth  int
	DisplayHeight int
	DisplayFPS    int
	// Docker socket configuration (Hydra multi-Docker isolation)
	DockerSocket string // Path to Docker socket to mount (defaults to /var/run/docker.sock if empty)
}

// computeZedImageFromVersion converts a SwayVersion (commit hash) from Wolf instance
// into a full Docker image tag. Returns empty string if no version is set (falls back to default).
func (w *WolfExecutor) computeZedImageFromVersion(swayVersion string) string {
	if swayVersion == "" {
		return "" // Fall back to default w.zedImage in createSwayWolfApp
	}
	// SwayVersion is a commit hash (e.g., "abc123def")
	// Convert to full image tag: helix-sway:abc123def
	return fmt.Sprintf("helix-sway:%s", swayVersion)
}

// createSwayWolfApp creates a Wolf app with Sway compositor (shared between PDEs and external agents)
func (w *WolfExecutor) createSwayWolfApp(config SwayWolfAppConfig) *wolf.App {
	// GOW_REQUIRED_DEVICES tells the GOW container launcher which device files to pass through.
	// We include all possible GPU devices - the glob won't match non-existent devices.
	// - /dev/uinput: User-space input device (for virtual keyboard/mouse from streaming client)
	// - /dev/input/*: Input devices (event*, mice, mouse*)
	// - /dev/dri/*: DRM render nodes (Intel/AMD/software)
	// - /dev/nvidia*: NVIDIA GPU devices
	// - /dev/kfd: AMD ROCm Kernel Fusion Driver
	gpuDevices := "/dev/uinput /dev/input/* /dev/dri/* /dev/nvidia* /dev/kfd"

	// Extract host:port and TLS setting from API URL for Zed WebSocket connection
	zedHelixURL, zedHelixTLS := extractHostPortAndTLS(w.helixAPIURL)

	// Build base environment variables (common to all Sway apps)
	env := []string{
		fmt.Sprintf("GOW_REQUIRED_DEVICES=%s", gpuDevices),
		"RUN_SWAY=1",
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", os.Getenv("ANTHROPIC_API_KEY")),
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		"ZED_ALLOW_EMULATED_GPU=1", // Allow software rendering with llvmpipe
		fmt.Sprintf("ZED_HELIX_URL=%s", zedHelixURL),
		fmt.Sprintf("ZED_HELIX_TOKEN=%s", w.helixAPIToken),
		fmt.Sprintf("ZED_HELIX_TLS=%t", zedHelixTLS),
		"RUST_LOG=info", // Enable Rust logging for Zed
		// Settings sync daemon configuration
		fmt.Sprintf("HELIX_SESSION_ID=%s", config.SessionID),
		// API URL for settings sync daemon and other services
		fmt.Sprintf("HELIX_API_URL=%s", w.helixAPIURL),
		fmt.Sprintf("HELIX_API_TOKEN=%s", w.helixAPIToken),
		"SETTINGS_SYNC_PORT=9877",
	}

	// Startup script lives in primary code repo at .helix/startup.sh
	// start-zed-helix.sh will execute it from disk if present

	// Add any extra environment variables
	env = append(env, config.ExtraEnv...)

	// Determine Docker socket to mount (Hydra isolation or default)
	dockerSocket := config.DockerSocket
	if dockerSocket == "" {
		dockerSocket = "/var/run/docker.sock" // Default to Wolf's Docker socket
	}

	// Build standard mounts (common to all Sway apps)
	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", config.WorkspaceDir),
		fmt.Sprintf("%s:/var/run/docker.sock", dockerSocket),
	}

	// Development mode: mount files for hot-reloading
	// CRITICAL: In DinD mode, use paths INSIDE Wolf container (/helix-dev/...), not host paths!
	// These files are mounted into Wolf via docker-compose, then re-mounted into sandboxes
	// Production mode: use files baked into helix-sway image
	if os.Getenv("HELIX_DEV_MODE") == "true" {
		log.Info().Msg("HELIX_DEV_MODE enabled - mounting dev files for hot-reloading (DinD-aware paths)")

		// Use paths inside Wolf's filesystem (bind-mounted from host into Wolf)
		mounts = append(mounts,
			"/helix-dev/zed-build:/zed-build:ro",
			"/helix-dev/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro",
			"/helix-dev/sway-config/start-zed-helix.sh:/usr/local/bin/start-zed-helix.sh:ro",
			"/helix-dev/sway-config/helix-specs-create.sh:/usr/local/bin/helix-specs-create.sh:ro",
		)
	} else {
		log.Debug().Msg("Production mode - using files baked into helix-sway image")
	}

	// Add SSH keys mount if user has SSH keys
	// The SSH key directory is created by the API when keys are created
	// Mount as read-only for security
	// CRITICAL: Use /filestore/ prefix for translateToHostPath compatibility
	// - Check existence with API container path (/filestore/...)
	// - Mount with HOST path (translated for Wolf)
	sshKeyDirAPI := fmt.Sprintf("/filestore/ssh-keys/%s", config.UserID)
	if _, err := os.Stat(sshKeyDirAPI); err == nil {
		// Translate to host path for Wolf mount
		sshKeyDirHost := w.translateToHostPath(sshKeyDirAPI)
		mounts = append(mounts, fmt.Sprintf("%s:/home/retro/.ssh:ro", sshKeyDirHost))
		log.Info().
			Str("user_id", config.UserID).
			Str("ssh_key_dir_api", sshKeyDirAPI).
			Str("ssh_key_dir_host", sshKeyDirHost).
			Msg("Mounting SSH keys for git access")
	}

	// Add extra mounts (e.g., internal project repo)
	mounts = append(mounts, config.ExtraMounts...)

	// Standard Docker configuration (same for all Sway apps)
	// CRITICAL: In DinD mode, sandboxes are on Wolf's isolated network
	// Add extra_hosts to reach API container on host's network (dev mode)
	// Production: RevDial handles all API communication
	baseCreateJSON := fmt.Sprintf(`{
  "Hostname": "%s",
  "HostConfig": {
    "IpcMode": "host",
    "NetworkMode": "helix_default",
    "Privileged": false,
    "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
    "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
    "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"],
    "ExtraHosts": ["api:172.19.0.20"],
    "Ulimits": [
      {
        "Name": "nofile",
        "Soft": 65536,
        "Hard": 65536
      }
    ]
  }
}`, config.ContainerHostname)

	// Determine which image to use (prefer config override from Wolf instance's SwayVersion)
	zedImage := w.zedImage
	if config.ZedImage != "" {
		zedImage = config.ZedImage
		log.Info().
			Str("zed_image", zedImage).
			Str("session_id", config.SessionID).
			Msg("Using versioned helix-sway image from Wolf instance")
	}

	// Create Wolf app
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

// NewWolfExecutor creates a Wolf executor using lobbies mode
// Lobbies persist naturally across Wolf restarts, no keepalive needed
func NewWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, store store.Store, connmanInst connmanInterface) Executor {
	log.Info().Msg("Initializing Wolf executor (lobbies mode)")
	return NewLobbyWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken, store, connmanInst)
}

// NewLobbyWolfExecutor creates a lobby-based Wolf executor (current implementation)
func NewLobbyWolfExecutor(wolfSocketPath, zedImage, helixAPIURL, helixAPIToken string, storeInst store.Store, connmanInst connmanInterface) *WolfExecutor {
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

	// Wolf client is created per-request based on session's WolfInstanceID
	// This allows routing to different Wolf instances in multi-Wolf deployments
	// See getWolfClient(wolfInstanceID) method below

	// CRITICAL: Workspace paths need to work in two contexts:
	// 1. Inside API container where we git clone: /filestore/workspaces
	// 2. Wolf creates containers on HOST: needs absolute host path /var/lib/docker/volumes/.../workspaces
	//
	// Get the absolute host path for the filestore volume
	filestoreVolumePath := os.Getenv("FILESTORE_VOLUME_PATH")
	if filestoreVolumePath == "" {
		// Default to standard Docker volume path on host
		filestoreVolumePath = "/var/lib/docker/volumes/helix_helix-filestore/_data"
		log.Info().
			Str("default_path", filestoreVolumePath).
			Msg("FILESTORE_VOLUME_PATH not set, using default Docker volume path")
	}

	// Get max concurrent lobbies from environment variable (default: 10)
	maxLobbies := 10
	if maxLobbiesStr := os.Getenv("EXTERNAL_AGENTS_MAX_CONCURRENT_LOBBIES"); maxLobbiesStr != "" {
		if parsed, err := strconv.Atoi(maxLobbiesStr); err == nil && parsed > 0 {
			maxLobbies = parsed
		}
	}
	log.Info().Int("max_concurrent_lobbies", maxLobbies).Msg("Wolf executor lobby limit configured")

	// Check if Hydra multi-Docker isolation is enabled
	// When enabled, each scope (SpecTask/session/exploratory) gets its own dockerd
	hydraEnabled := os.Getenv("HYDRA_ENABLED") == "true"
	if hydraEnabled {
		log.Info().Msg("Hydra multi-Docker isolation enabled - each scope gets isolated dockerd")
	}

	executor := &WolfExecutor{
		store:                        storeInst,
		sessions:                     make(map[string]*ZedSession),
		zedImage:                     zedImage,
		helixAPIURL:                  helixAPIURL,
		helixAPIToken:                helixAPIToken,
		workspaceBasePathForCloning:  "/filestore/workspaces",                          // Path inside API container for git clone operations
		workspaceBasePathForMounting: filepath.Join(filestoreVolumePath, "workspaces"), // Absolute host path for Wolf to mount
		lobbyCache:                   make(map[string]*lobbyCacheEntry),
		lobbyCacheTTL:                5 * time.Second,              // Cache lobby lookups for 5 seconds to prevent Wolf API spam
		creationLocks:                make(map[string]*sync.Mutex), // Per-session locks for lobby creation
		wolfScheduler:                store.NewWolfScheduler(storeInst),
		connman:                      connmanInst, // RevDial connection manager for screenshot/clipboard and remote Wolf instances
		maxConcurrentLobbies:         maxLobbies,
		hydraEnabled:                 hydraEnabled,
	}

	// Lobbies mode doesn't need health monitoring or reconciliation
	// Lobbies persist naturally across Wolf restarts

	// Start idle external agent cleanup loop (5min timeout)
	go executor.idleExternalAgentCleanupLoop(context.Background())

	// Start Wolf resource monitoring loop (logs metrics every minute)
	go executor.wolfResourceMonitoringLoop(context.Background())

	// TEMPORARILY DISABLED: Start orphaned Wolf-UI session cleanup loop (cleans up streaming sessions without active containers)
	// ISSUE: This cleanup kills Wolf-UI sessions that have active browser connections
	// It only checks if lobby exists, not if browsers are actively streaming
	// Disabling until we can add proper check for active WebRTC connections
	// go executor.cleanupOrphanedWolfUISessionsLoop(context.Background())

	return executor
}

// StartZedAgent implements the Executor interface for external agent sessions
func (w *WolfExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	// Get or create a per-session lock to prevent concurrent lobby creation for the same session
	w.creationLocksMutex.Lock()
	sessionLock, exists := w.creationLocks[agent.SessionID]
	if !exists {
		sessionLock = &sync.Mutex{}
		w.creationLocks[agent.SessionID] = sessionLock
	}
	w.creationLocksMutex.Unlock()

	// Lock this specific session to prevent duplicate lobby creation
	sessionLock.Lock()
	defer sessionLock.Unlock()

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("project_path", agent.ProjectPath).
		Msg("Starting external Zed agent via Wolf (with per-session creation lock)")

	// Select best Wolf instance for this sandbox
	// Use SelectWolfInstanceWithOptions to support privileged mode filtering
	wolfInstance, err := w.wolfScheduler.SelectWolfInstanceWithOptions(ctx, store.WolfSelectionOptions{
		GPUType:               "", // TODO: Extract GPU type from agent config if needed
		RequirePrivilegedMode: agent.UseHostDocker,
	})
	if err != nil {
		return nil, fmt.Errorf("no Wolf instances available - ensure Wolf container is running and connected via RevDial: %w", err)
	}

	log.Info().
		Str("wolf_id", wolfInstance.ID).
		Str("wolf_name", wolfInstance.Name).
		Int("current_load", wolfInstance.ConnectedSandboxes).
		Int("max_capacity", wolfInstance.MaxSandboxes).
		Bool("privileged_mode", wolfInstance.PrivilegedModeEnabled).
		Bool("use_host_docker", agent.UseHostDocker).
		Msg("Selected Wolf instance for sandbox")

	// Generate numeric Wolf app ID for Moonlight protocol compatibility
	// Use session ID as environment name for consistency
	wolfAppID := w.generateWolfAppID(agent.UserID, agent.SessionID)

	// Determine workspace directory - use task-scoped for SpecTasks, session-scoped otherwise
	// CRITICAL: Use workspaceBasePathForCloning here since we'll be git cloning into this directory
	workspaceDir := agent.WorkDir
	if workspaceDir == "" {
		if agent.SpecTaskID != "" {
			// SpecTask agents share workspace across planning and implementation
			workspaceDir = filepath.Join(w.workspaceBasePathForCloning, "spec-tasks", agent.SpecTaskID)
			log.Info().
				Str("spec_task_id", agent.SpecTaskID).
				Str("workspace_dir", workspaceDir).
				Msg("Using task-scoped workspace for SpecTask agent")
		} else {
			// Regular external agents use session-scoped workspace
			workspaceDir = filepath.Join(w.workspaceBasePathForCloning, "external-agents", agent.SessionID)
		}
	}

	// Create workspace directory if it doesn't exist
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Clone git repositories if specified (for SpecTasks with repository context)
	// Repository cloning is handled by startup script (start-zed-helix.sh)
	var primaryRepoName string
	if len(agent.RepositoryIDs) > 0 {
		// Repository cloning now handled by startup script (start-zed-helix.sh)
		// Startup script uses HELIX_REPOSITORY_IDS and USER_API_TOKEN env vars
		// This is cleaner: git credentials already configured in container, uses HTTP connectivity
		log.Info().
			Strs("repository_ids", agent.RepositoryIDs).
			Msg("Repositories will be cloned by startup script before Zed starts")

		// Get primary repository name for environment variable (startup script needs this)
		if agent.PrimaryRepositoryID != "" {
			repo, err := w.store.GetGitRepository(ctx, agent.PrimaryRepositoryID)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to get primary repository name")
			} else {
				primaryRepoName = repo.Name
				log.Info().
					Str("primary_repo_id", agent.PrimaryRepositoryID).
					Str("primary_repo_name", primaryRepoName).
					Msg("Primary repository will be used for design docs worktree")
			}
		}
	}

	log.Info().
		Str("wolf_app_id", wolfAppID).
		Str("workspace_dir", workspaceDir).
		Str("session_id", agent.SessionID).
		Msg("Creating Wolf app for external Zed agent")

	// Create Sway compositor configuration (same as PDEs)
	err = w.createSwayConfig(agent.SessionID)
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

	// Add primary repository name for design docs worktree setup
	if primaryRepoName != "" {
		extraEnv = append(extraEnv, fmt.Sprintf("HELIX_PRIMARY_REPO_NAME=%s", primaryRepoName))
	}

	// Pass repository information for startup script to clone
	// Format: "id:name:type,id:name:type,..." (simple parsing, no jq needed)
	if len(agent.RepositoryIDs) > 0 {
		var repoSpecs []string
		for _, repoID := range agent.RepositoryIDs {
			repo, err := w.store.GetGitRepository(ctx, repoID)
			if err != nil {
				log.Warn().Err(err).Str("repo_id", repoID).Msg("Failed to get repository metadata")
				continue
			}
			// Format: id:name:type
			repoSpec := fmt.Sprintf("%s:%s:%s", repo.ID, repo.Name, repo.RepoType)
			repoSpecs = append(repoSpecs, repoSpec)
		}
		if len(repoSpecs) > 0 {
			extraEnv = append(extraEnv, fmt.Sprintf("HELIX_REPOSITORIES=%s", strings.Join(repoSpecs, ",")))
		}
	}

	// Pass API base URL for git cloning
	extraEnv = append(extraEnv, fmt.Sprintf("HELIX_API_BASE_URL=%s", w.helixAPIURL))

	// Add custom env vars from agent request (includes USER_API_TOKEN for git + RevDial)
	extraEnv = append(extraEnv, agent.Env...)

	// Extract video settings from agent config (Phase 3.5) with defaults
	// Use 1080p as default - AMD GPUs only support up to 1080p hardware encoding
	displayWidth := agent.DisplayWidth
	if displayWidth == 0 {
		displayWidth = 1920 // 1080p default (AMD GPU hardware encoder limit)
	}
	displayHeight := agent.DisplayHeight
	if displayHeight == 0 {
		displayHeight = 1080 // 1080p default (AMD GPU hardware encoder limit)
	}
	displayRefreshRate := agent.DisplayRefreshRate
	if displayRefreshRate == 0 {
		displayRefreshRate = 60
	}

	// Extra mounts for additional directories
	extraMounts := []string{}

	// CRITICAL: Check if lobby already exists for this session to prevent duplicates
	// This prevents GPU resource exhaustion when resume endpoint is called multiple times
	existingLobbyID, err := w.FindExistingLobbyForSession(ctx, agent.SessionID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", agent.SessionID).Msg("Failed to check for existing lobby")
		// Continue with creation anyway - not a fatal error
	} else if existingLobbyID != "" {
		// Lobby already exists for this session - reuse it instead of creating duplicate
		log.Info().
			Str("lobby_id", existingLobbyID).
			Str("session_id", agent.SessionID).
			Msg("🔄 Reusing existing lobby for session (prevents GPU resource exhaustion)")

		// CRITICAL: Still need to track the session for WebSocket sync to work
		// Even though lobby exists, we need to register it in our sessions map
		session := &ZedSession{
			SessionID:      agent.SessionID,
			HelixSessionID: helixSessionID,
			UserID:         agent.UserID,
			Status:         "running",
			StartTime:      time.Now(),
			LastAccess:     time.Now(),
			ProjectPath:    agent.ProjectPath,
			WolfLobbyID:    existingLobbyID,
			ContainerName:  containerHostname,
		}
		w.sessions[agent.SessionID] = session

		// Track activity for idle cleanup
		agentType := "agent"
		if agent.ProjectID != "" {
			agentType = "exploratory"
		}
		if agent.SpecTaskID != "" {
			agentType = "spectask"
		}

		err = w.store.UpsertExternalAgentActivity(ctx, &types.ExternalAgentActivity{
			ExternalAgentID: agent.SessionID,
			SpecTaskID:      agent.SpecTaskID,
			LastInteraction: time.Now(),
			AgentType:       agentType,
			WolfAppID:       "", // Don't have app ID for reused lobby
			WorkspaceDir:    workspaceDir,
			UserID:          agent.UserID,
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("session_id", agent.SessionID).
				Msg("Failed to track activity for reused lobby")
		}

		// Fetch PIN and Wolf instance ID from session metadata for auto-join support
		// With the session_handlers fix, this will preserve existing PINs even if we return empty
		var lobbyPIN string
		var existingWolfInstanceID string
		if helixSessionID != "" {
			helixSession, err := w.store.GetSession(ctx, helixSessionID)
			if err != nil {
				log.Warn().Err(err).Str("helix_session_id", helixSessionID).Msg("Failed to get session for PIN retrieval")
			} else {
				lobbyPIN = helixSession.Metadata.WolfLobbyPIN
				existingWolfInstanceID = helixSession.WolfInstanceID
				log.Debug().
					Str("helix_session_id", helixSessionID).
					Bool("has_pin", lobbyPIN != "").
					Str("wolf_instance_id", existingWolfInstanceID).
					Msg("Retrieved lobby PIN and Wolf instance from session metadata for reuse")
			}
		}

		// Build response using existing lobby
		response := &types.ZedAgentResponse{
			SessionID:      agent.SessionID,
			ScreenshotURL:  fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
			StreamURL:      fmt.Sprintf("moonlight://localhost:47989"),
			Status:         "running",
			WolfLobbyID:    existingLobbyID,
			WolfLobbyPIN:   lobbyPIN, // Include PIN for auto-join support
			WolfInstanceID: existingWolfInstanceID,
			ContainerName:  containerHostname,
		}

		log.Info().
			Str("session_id", agent.SessionID).
			Str("lobby_id", existingLobbyID).
			Bool("has_pin", lobbyPIN != "").
			Msg("Reused existing lobby and registered for WebSocket sync")

		return response, nil
	}

	// No existing lobby - create a new one
	log.Info().Str("session_id", agent.SessionID).Msg("No existing lobby found, creating new lobby")

	// CRITICAL: Enforce hard limit of 5 concurrent lobbies to prevent GPU resource exhaustion
	// Discovery: NVML fails at ~5-6 lobbies, GPU crashes at 6-7 lobbies
	// See: design/2025-11-05-wolf-gpu-resource-limits-and-monitoring.md
	lobbies, err := w.getWolfClient(wolfInstance.ID).ListLobbies(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check lobby count before creation")
		return nil, fmt.Errorf("failed to check GPU resource availability: %w", err)
	}

	if len(lobbies) >= w.maxConcurrentLobbies {
		log.Error().
			Int("active_lobbies", len(lobbies)).
			Int("max_lobbies", w.maxConcurrentLobbies).
			Str("session_id", agent.SessionID).
			Msg("GPU resource limit reached - cannot create new lobby")
		return nil, fmt.Errorf("GPU resource limit reached (%d/%d lobbies active). Please close an unused session and try again", len(lobbies), w.maxConcurrentLobbies)
	}

	log.Info().
		Int("active_lobbies", len(lobbies)).
		Int("max_lobbies", w.maxConcurrentLobbies).
		Msg("GPU capacity check passed, proceeding with lobby creation")

	// Generate PIN for lobby access control (Phase 3: Multi-tenancy)
	lobbyPIN, lobbyPINString := generateLobbyPIN()

	// Log Wolf resources before creating lobby (for correlation with failures)
	w.logWolfResourceMetrics(ctx)

	// CRITICAL: Translate workspace path from API container path to absolute host path
	// Wolf runs on host and needs host paths for mounts, not container paths
	workspaceDirForMount := w.translateToHostPath(workspaceDir)

	log.Info().
		Str("workspace_dir_container", workspaceDir).
		Str("workspace_dir_host", workspaceDirForMount).
		Msg("Translated workspace path for Wolf mounting")

	// Translate extra mounts to host paths as well
	translatedExtraMounts := []string{}
	for _, mount := range extraMounts {
		// Mount format is "source:dest:options"
		parts := strings.Split(mount, ":")
		if len(parts) >= 2 {
			hostSource := w.translateToHostPath(parts[0])
			parts[0] = hostSource
			translatedExtraMounts = append(translatedExtraMounts, strings.Join(parts, ":"))
		} else {
			translatedExtraMounts = append(translatedExtraMounts, mount)
		}
	}

	// Wolf's compute_pipeline_defaults() handles GPU auto-detection for video settings.
	// We send empty strings and let Wolf figure out the optimal pipeline based on:
	// - NVIDIA: video/x-raw(memory:CUDAMemory) for zero-copy CUDA encoding
	// - AMD/Intel: video/x-raw(memory:DMABuf) for zero-copy VA-API encoding
	// - Software: video/x-raw for CPU-based encoding
	// This avoids duplicating GPU detection logic and enables AMD/Intel zero-copy DMABuf.

	// Hydra multi-Docker isolation: Create isolated dockerd for this session
	// When Hydra is enabled, each session gets its own dockerd for complete isolation
	// We always key on session ID (not SpecTask ID) because:
	// 1. Session ID is unique per agent instance
	// 2. Works for all agent types: SpecTask, exploratory, and direct session agents
	// 3. Each session should have isolated Docker state
	var dockerSocket string
	if w.hydraEnabled {
		// Determine scope type for categorization (but always use session ID as the key)
		var scopeType string
		if agent.SpecTaskID != "" {
			scopeType = "spectask"
		} else if agent.ProjectID != "" {
			scopeType = "exploratory"
		} else {
			scopeType = "session"
		}
		// Always use session ID as the scope ID for isolation
		scopeID := agent.SessionID

		log.Info().
			Str("scope_type", scopeType).
			Str("scope_id", scopeID).
			Str("session_id", agent.SessionID).
			Str("spec_task_id", agent.SpecTaskID).
			Bool("use_host_docker", agent.UseHostDocker).
			Msg("Creating isolated Docker instance via Hydra")

		// Call Hydra API to create Docker instance via RevDial
		// Hydra runs in sandbox container, accessible via RevDial tunnel
		hydraRunnerID := fmt.Sprintf("hydra-%s", wolfInstance.ID)
		log.Info().
			Str("hydra_runner_id", hydraRunnerID).
			Str("wolf_instance_id", wolfInstance.ID).
			Msg("Connecting to Hydra via RevDial")

		hydraClient := hydra.NewRevDialClient(w.connman, hydraRunnerID)
		dockerInstance, err := hydraClient.CreateDockerInstance(ctx, &hydra.CreateDockerInstanceRequest{
			ScopeType:     hydra.ScopeType(scopeType),
			ScopeID:       scopeID,
			UserID:        agent.UserID,
			UseHostDocker: agent.UseHostDocker, // Privileged mode: use host Docker socket
		})
		if err != nil {
			// Hydra is enabled but failed - this is a hard error, not a fallback
			// The user explicitly wants isolation, so don't silently use shared socket
			return nil, fmt.Errorf("failed to create isolated Docker instance via Hydra (runner_id=%s): %w. "+
				"Check that Hydra is running in sandbox and RevDial is connected", hydraRunnerID, err)
		}
		dockerSocket = dockerInstance.DockerSocket
		log.Info().
			Str("scope_type", scopeType).
			Str("scope_id", scopeID).
			Str("docker_socket", dockerSocket).
			Str("hydra_runner_id", hydraRunnerID).
			Msg("Created isolated Docker instance via Hydra")
	}

	lobbyReq := &wolf.CreateLobbyRequest{
		ProfileID:              "helix-sessions",
		Name:                   fmt.Sprintf("Agent %s", agent.SessionID[len(agent.SessionID)-4:]),
		MultiUser:              true,
		StopWhenEveryoneLeaves: false,    // CRITICAL: Agent must keep running when no Moonlight clients connected!
		PIN:                    lobbyPIN, // NEW: Require PIN to join lobby
		VideoSettings: &wolf.LobbyVideoSettings{
			Width:       displayWidth,
			Height:      displayHeight,
			RefreshRate: displayRefreshRate,
			// Empty strings → Wolf's compute_pipeline_defaults() auto-detects optimal GPU pipeline
			WaylandRenderNode:       "",
			RunnerRenderNode:        "",
			VideoProducerBufferCaps: "",
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
			WorkspaceDir:      workspaceDirForMount, // CRITICAL: Use host path for Wolf mount
			ExtraEnv:          extraEnv,
			ExtraMounts:       translatedExtraMounts, // Translated to host paths
			DisplayWidth:      displayWidth,
			DisplayHeight:     displayHeight,
			DisplayFPS:        displayRefreshRate,
			ZedImage:          w.computeZedImageFromVersion(wolfInstance.SwayVersion), // Use version from sandbox heartbeat
			DockerSocket:      dockerSocket,                                           // Hydra-managed socket (empty = default)
		}).Runner, // Use the runner config from the app
	}

	// Create lobby (container starts immediately!)
	lobbyResp, err := w.getWolfClient(wolfInstance.ID).CreateLobby(ctx, lobbyReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create lobby for external agent: %w", err)
	}

	log.Info().
		Str("lobby_id", lobbyResp.LobbyID).
		Str("session_id", agent.SessionID).
		Str("lobby_pin", lobbyPINString).
		Msg("Wolf lobby created successfully - container starting immediately")

	// Log resources AFTER lobby creation to see impact
	go func() {
		// Wait a few seconds for container to fully start
		time.Sleep(3 * time.Second)
		w.logWolfResourceMetrics(context.Background())
	}()

	// Immediately cache the new lobby to prevent duplicate creation on rapid resume attempts
	w.lobbyCacheMutex.Lock()
	w.lobbyCache[agent.SessionID] = &lobbyCacheEntry{
		lobbyID:   lobbyResp.LobbyID,
		timestamp: time.Now(),
	}
	w.lobbyCacheMutex.Unlock()

	// Update Helix session metadata with lobby ID, PIN, and Wolf instance ID
	if helixSessionID != "" {
		helixSession, err := w.store.GetSession(ctx, helixSessionID)
		if err != nil {
			log.Warn().Err(err).Str("helix_session_id", helixSessionID).Msg("Failed to get Helix session for lobby metadata update")
		} else {
			// Update session metadata with Wolf lobby information
			helixSession.Metadata.WolfLobbyID = lobbyResp.LobbyID
			helixSession.Metadata.WolfLobbyPIN = lobbyPINString
			helixSession.Metadata.SwayVersion = wolfInstance.SwayVersion // Track which helix-sway version is running
			helixSession.Metadata.GPUVendor = wolfInstance.GPUVendor     // Track GPU vendor for debugging
			helixSession.Metadata.RenderNode = wolfInstance.RenderNode   // Track render node for debugging
			helixSession.WolfInstanceID = wolfInstance.ID

			_, err = w.store.UpdateSession(ctx, *helixSession)
			if err != nil {
				log.Error().Err(err).Str("helix_session_id", helixSessionID).Msg("Failed to update Helix session with lobby metadata")
			} else {
				log.Info().
					Str("helix_session_id", helixSessionID).
					Str("lobby_id", lobbyResp.LobbyID).
					Str("lobby_pin", lobbyPINString).
					Str("wolf_instance_id", wolfInstance.ID).
					Str("sway_version", wolfInstance.SwayVersion).
					Str("gpu_vendor", wolfInstance.GPUVendor).
					Str("render_node", wolfInstance.RenderNode).
					Msg("Updated Helix session metadata with Wolf lobby ID, PIN, instance, sway version, and GPU info")
			}
		}
	}

	// Increment Wolf's sandbox count
	if wolfInstance != nil && wolfInstance.ID != "" {
		err = w.store.IncrementWolfSandboxCount(ctx, wolfInstance.ID)
		if err != nil {
			log.Error().Err(err).Str("wolf_id", wolfInstance.ID).Msg("Failed to increment Wolf sandbox count")
		} else {
			log.Info().
				Str("wolf_id", wolfInstance.ID).
				Int("new_count", wolfInstance.ConnectedSandboxes+1).
				Msg("Incremented Wolf sandbox count")
		}
	}

	// Track session with lobby ID
	session := &ZedSession{
		SessionID:      agent.SessionID,
		HelixSessionID: helixSessionID, // Store Helix session ID for screenshot lookup
		UserID:         agent.UserID,
		Status:         "running", // Container is running immediately with lobbies
		StartTime:      time.Now(),
		LastAccess:     time.Now(),
		ProjectPath:    agent.ProjectPath,
		WolfLobbyID:    lobbyResp.LobbyID, // NEW: Track lobby ID
		ContainerName:  containerHostname,
	}
	w.sessions[agent.SessionID] = session

	// Lobbies mode doesn't need keepalive - lobbies persist naturally
	// (keepalive was a hack for apps mode to prevent stale buffer crashes)

	// Return response with screenshot URL, Moonlight info, and PIN
	response := &types.ZedAgentResponse{
		SessionID:      agent.SessionID,
		ScreenshotURL:  fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
		StreamURL:      fmt.Sprintf("moonlight://localhost:47989"),
		Status:         "running", // Lobby starts immediately
		ContainerName:  containerHostname,
		WolfLobbyID:    lobbyResp.LobbyID,        // NEW: Return lobby ID
		WolfLobbyPIN:   lobbyPINString,           // NEW: Return PIN for storage in Helix session
		WolfInstanceID: wolfInstance.ID,          // NEW: Return Wolf instance ID for multi-Wolf deployment
		SwayVersion:    wolfInstance.SwayVersion, // helix-sway image version for debugging
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
		WolfLobbyID:     response.WolfLobbyID,  // Store lobby ID for cleanup even after session deleted
		WolfLobbyPIN:    response.WolfLobbyPIN, // Store lobby PIN for cleanup
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
	log.Info().Str("session_id", sessionID).Msg("Stopping Zed agent via Wolf")

	// CRITICAL: Only hold mutex when accessing in-memory map
	// Do NOT hold mutex during Wolf API calls (prevents deadlock)
	w.mutex.Lock()
	session, exists := w.sessions[sessionID]
	var wolfLobbyID string
	if exists {
		wolfLobbyID = session.WolfLobbyID
	}
	w.mutex.Unlock()

	// Always fetch from database to get lobby ID, PIN, and Wolf instance ID
	// Use GetSessionIncludingDeleted to find soft-deleted sessions (e.g., from project deletion)
	dbSession, err := w.store.GetSessionIncludingDeleted(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session from database for stop (including soft-deleted)")
		return fmt.Errorf("session %s not found in database", sessionID)
	}

	// Use database lobby ID if we don't have it from memory
	if wolfLobbyID == "" {
		wolfLobbyID = dbSession.Metadata.WolfLobbyID
	}

	// Get PIN and Wolf instance ID from database
	wolfLobbyPIN := dbSession.Metadata.WolfLobbyPIN
	wolfInstanceID := dbSession.WolfInstanceID

	// Save final screenshot before stopping (for paused state preview)
	screenshotPath := ""
	screenshotBytes, err := w.getContainerScreenshot(ctx, sessionID)
	if err == nil && len(screenshotBytes) > 0 {
		// Save to filestore (use cloning path since we're writing from API container)
		screenshotPath = filepath.Join(w.workspaceBasePathForCloning, "paused-screenshots", fmt.Sprintf("%s.png", sessionID))
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
	// Add timeout to prevent hanging forever on stuck Wolf API calls
	if wolfInstanceID == "" {
		log.Warn().
			Str("session_id", sessionID).
			Msg("Session has no WolfInstanceID - skipping Wolf session cleanup (legacy session or missing data)")
	} else {
		listCtx, listCancel := context.WithTimeout(ctx, 5*time.Second)
		defer listCancel()

		sessions, err := w.getWolfClient(wolfInstanceID).ListSessions(listCtx)
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

					// Add timeout to prevent hanging on stuck Wolf sessions
					stopCtx, stopCancel := context.WithTimeout(ctx, 5*time.Second)
					err := w.getWolfClient(wolfInstanceID).StopSession(stopCtx, session.ClientID)
					stopCancel()

					if err != nil {
						log.Warn().
							Err(err).
							Str("client_id", session.ClientID).
							Msg("Failed to stop Wolf-UI session (will be orphaned - timeout or error)")
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
	}

	// Stop the lobby (tears down Zed container)
	if wolfLobbyID == "" {
		log.Warn().
			Str("session_id", sessionID).
			Msg("No Wolf lobby ID found in database - session may not have external agent running")
		return fmt.Errorf("no Wolf lobby ID found for session %s", sessionID)
	}

	if wolfInstanceID == "" {
		log.Warn().
			Str("session_id", sessionID).
			Str("lobby_id", wolfLobbyID).
			Msg("Session has no WolfInstanceID - cannot stop lobby (legacy session or missing data)")
		return fmt.Errorf("no Wolf instance ID found for session %s - cannot stop lobby", sessionID)
	}

	// wolfLobbyID != "" && wolfInstanceID != "" from here
	{
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

		// Add timeout to prevent hanging on stuck Wolf API
		lobbyStopCtx, lobbyStopCancel := context.WithTimeout(ctx, 10*time.Second)
		err = w.getWolfClient(wolfInstanceID).StopLobby(lobbyStopCtx, stopReq)
		lobbyStopCancel()

		if err != nil {
			log.Error().
				Err(err).
				Str("lobby_id", wolfLobbyID).
				Interface("lobby_pin", lobbyPIN).
				Msg("Failed to stop Wolf lobby (timeout or error)")
			// Continue with cleanup even if stop fails
		} else {
			log.Info().
				Str("lobby_id", wolfLobbyID).
				Str("session_id", sessionID).
				Msg("Wolf lobby stopped successfully")
		}
	}

	// Update in-memory session status and remove from tracking
	// CRITICAL: Acquire mutex only for map operations
	w.mutex.Lock()
	if session, exists := w.sessions[sessionID]; exists {
		session.Status = "stopped"
	}
	delete(w.sessions, sessionID)
	w.mutex.Unlock()

	// Invalidate lobby cache so restart creates fresh lobby instead of reusing stopped one
	w.lobbyCacheMutex.Lock()
	delete(w.lobbyCache, sessionID)
	w.lobbyCacheMutex.Unlock()

	// Decrement Wolf's sandbox count
	if wolfInstanceID != "" {
		err = w.store.DecrementWolfSandboxCount(ctx, wolfInstanceID)
		if err != nil {
			log.Error().Err(err).Str("wolf_id", wolfInstanceID).Msg("Failed to decrement Wolf sandbox count")
		} else {
			log.Info().
				Str("wolf_id", wolfInstanceID).
				Msg("Decremented Wolf sandbox count")
		}
	}

	// Stop Hydra Docker instance if enabled (preserves data for resume)
	if w.hydraEnabled && wolfInstanceID != "" {
		// Determine scope type for categorization (but always use session ID as the key)
		var scopeType hydra.ScopeType
		if dbSession.Metadata.SpecTaskID != "" {
			scopeType = hydra.ScopeTypeSpecTask
		} else if dbSession.Metadata.ProjectID != "" {
			scopeType = hydra.ScopeTypeExploratory
		} else {
			scopeType = hydra.ScopeTypeSession
		}
		// Always use session ID as the scope ID (matches StartZedAgent)
		scopeID := sessionID

		log.Info().
			Str("scope_type", string(scopeType)).
			Str("scope_id", scopeID).
			Msg("Stopping Hydra Docker instance (data preserved for resume)")

		hydraClient := hydra.NewRevDialClient(w.connman, fmt.Sprintf("hydra-%s", wolfInstanceID))
		_, err := hydraClient.DeleteDockerInstance(ctx, scopeType, scopeID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("scope_type", string(scopeType)).
				Str("scope_id", scopeID).
				Msg("Failed to stop Hydra Docker instance (non-fatal)")
		} else {
			log.Info().
				Str("scope_type", string(scopeType)).
				Str("scope_id", scopeID).
				Msg("Hydra Docker instance stopped successfully")
		}
	}

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
	workspaceDir := filepath.Join(w.workspaceBasePathForCloning, instance.InstanceID)

	// GOW_REQUIRED_DEVICES tells the GOW container launcher which device files to pass through.
	// We include all possible GPU devices - the glob won't match non-existent devices.
	// - /dev/uinput: User-space input device (for virtual keyboard/mouse from streaming client)
	// - /dev/input/*: Input devices (event*, mice, mouse*)
	// - /dev/dri/*: DRM render nodes (Intel/AMD/software)
	// - /dev/nvidia*: NVIDIA GPU devices
	// - /dev/kfd: AMD ROCm Kernel Fusion Driver
	gpuDevices := "/dev/uinput /dev/input/* /dev/dri/* /dev/nvidia* /dev/kfd"

	// Extract host:port and TLS setting from API URL for Zed WebSocket connection
	zedHelixURL, zedHelixTLS := extractHostPortAndTLS(w.helixAPIURL)

	env := []string{
		fmt.Sprintf("GOW_REQUIRED_DEVICES=%s", gpuDevices),
		"RUN_SWAY=1", // Enable Sway compositor mode in GOW launcher
		// Pass through API keys for Zed AI functionality
		fmt.Sprintf("ANTHROPIC_API_KEY=%s", os.Getenv("ANTHROPIC_API_KEY")),
		fmt.Sprintf("OPENAI_API_KEY=%s", os.Getenv("OPENAI_API_KEY")),
		fmt.Sprintf("TOGETHER_API_KEY=%s", os.Getenv("TOGETHER_API_KEY")),
		fmt.Sprintf("HF_TOKEN=%s", os.Getenv("HF_TOKEN")),
		// Zed external websocket sync configuration
		"ZED_EXTERNAL_SYNC_ENABLED=true", // Enables websocket sync (websocket_enabled defaults to this)
		"ZED_ALLOW_EMULATED_GPU=1",       // Allow software rendering with llvmpipe
		fmt.Sprintf("ZED_HELIX_URL=%s", zedHelixURL),
		fmt.Sprintf("ZED_HELIX_TOKEN=%s", w.helixAPIToken),
		fmt.Sprintf("ZED_HELIX_TLS=%t", zedHelixTLS),
		// Enable user startup script execution
		"HELIX_STARTUP_SCRIPT=/home/retro/work/startup.sh",
	}
	mounts := []string{
		fmt.Sprintf("%s:/home/retro/work", workspaceDir), // Mount persistent workspace
		"/var/run/docker.sock:/var/run/docker.sock:rw",   // Mount Wolf's docker socket for devcontainer support
	}

	// Development mode: mount files for hot-reloading
	// CRITICAL: In DinD mode, use paths INSIDE Wolf container (/helix-dev/...), not host paths!
	// These files are mounted into Wolf via docker-compose, then re-mounted into sandboxes
	// Production mode: use files baked into helix-sway image
	if os.Getenv("HELIX_DEV_MODE") == "true" {
		log.Info().Msg("HELIX_DEV_MODE enabled - mounting dev files for hot-reloading (DinD-aware paths)")

		// Use paths inside Wolf's filesystem (bind-mounted from host into Wolf)
		mounts = append(mounts,
			"/helix-dev/zed-build:/zed-build:ro",
			"/helix-dev/sway-config/startup-app.sh:/opt/gow/startup-app.sh:ro",
			"/helix-dev/sway-config/start-zed-helix.sh:/usr/local/bin/start-zed-helix.sh:ro",
			"/helix-dev/sway-config/helix-specs-create.sh:/usr/local/bin/helix-specs-create.sh:ro",
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
	err := w.getWolfClient("").RemoveApp(ctx, wolfAppID)
	if err != nil {
		log.Debug().Err(err).Str("wolf_app_id", wolfAppID).Msg("No existing Wolf app to remove (expected)")
	}

	// Add the app to Wolf
	err = w.getWolfClient("").AddApp(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to recreate Wolf app: %w", err)
	}

	log.Info().
		Str("instance_id", instance.InstanceID).
		Str("wolf_app_id", wolfAppID).
		Msg("Successfully recreated Wolf app for personal dev environment")

	return nil
}

// checkWolfAppExists checks if a Wolf app with the given ID already exists
func (w *WolfExecutor) checkWolfAppExists(ctx context.Context, appID string) (bool, error) {
	apps, err := w.getWolfClient("").ListApps(ctx)
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

// GetWolfClient returns the Wolf client for access to Wolf API via RevDial
// DEPRECATED: This method should not be used - use GetWolfClientForSession instead
// Callers must always provide a Wolf instance ID by looking it up from the session
// This method exists only for backward compatibility and will fail at runtime
func (w *WolfExecutor) GetWolfClient() WolfClientInterface {
	// This is a programming error - all callers should use GetWolfClientForSession
	// with the Wolf instance ID looked up from the session/database
	panic("GetWolfClient() called without instance ID - use GetWolfClientForSession(wolfInstanceID) instead")
}

// GetWolfClientForSession returns a Wolf client for a specific Wolf instance ID
// This is used by handlers that need to query a session's specific Wolf instance
func (w *WolfExecutor) GetWolfClientForSession(wolfInstanceID string) WolfClientInterface {
	return w.getWolfClient(wolfInstanceID)
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
	// Try in-memory cache first (fast path)
	w.mutex.RLock()
	for agentSessionID, session := range w.sessions {
		if session.HelixSessionID == helixSessionID {
			log.Debug().
				Str("helix_session_id", helixSessionID).
				Str("agent_session_id", agentSessionID).
				Str("container_hostname", session.ContainerName).
				Msg("Found external agent container by Helix session ID (in-memory cache)")
			w.mutex.RUnlock()
			return session.ContainerName, nil
		}
	}
	w.mutex.RUnlock()

	// In-memory cache miss - query Wolf lobbies to find container
	// This handles API restarts where in-memory map is cleared but containers are still running
	log.Trace().
		Str("helix_session_id", helixSessionID).
		Msg("Session not in memory, querying Wolf lobbies for container")

	// Look up session to get Wolf instance ID
	session, err := w.store.GetSession(ctx, helixSessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session for Wolf instance lookup: %w", err)
	}

	wolfInstanceID := session.WolfInstanceID
	if wolfInstanceID == "" {
		return "", fmt.Errorf("session %s has no Wolf instance ID - session may be corrupted or from before multi-Wolf support", helixSessionID)
	}

	lobbies, err := w.getWolfClient(wolfInstanceID).ListLobbies(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list Wolf lobbies: %w", err)
	}

	// Search for lobby with matching HELIX_SESSION_ID in env vars
	for _, lobby := range lobbies {
		if runnerMap, ok := lobby.Runner.(map[string]interface{}); ok {
			if envList, ok := runnerMap["env"].([]interface{}); ok {
				for _, envVar := range envList {
					if envStr, ok := envVar.(string); ok {
						// Check for HELIX_SESSION_ID=<session_id>
						expectedEnv := fmt.Sprintf("HELIX_SESSION_ID=%s", helixSessionID)
						if envStr == expectedEnv {
							// Found lobby - extract container hostname
							// Container name format: zed-external-{session_id_without_ses_}_{lobby_id}
							sessionIDPart := strings.TrimPrefix(helixSessionID, "ses_")
							containerHostname := fmt.Sprintf("zed-external-%s", sessionIDPart)

							log.Trace().
								Str("helix_session_id", helixSessionID).
								Str("lobby_id", lobby.ID).
								Str("container_hostname", containerHostname).
								Msg("Found external agent container by querying Wolf lobbies")

							return containerHostname, nil
						}
					}
				}
			}
		}
	}

	log.Error().
		Str("helix_session_id", helixSessionID).
		Int("lobbies_checked", len(lobbies)).
		Msg("No external agent container found for this Helix session ID")

	return "", fmt.Errorf("no external agent container found for Helix session ID: %s", helixSessionID)
}

// idleExternalAgentCleanupLoop runs periodically to cleanup idle SpecTask external agents
// Terminates external agents after 1min of inactivity (for testing, will be 30min in production)
// Timeout is database-based (checks LastInteraction timestamp), so it survives API restarts
func (w *WolfExecutor) idleExternalAgentCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds for faster cleanup in dev mode
	defer ticker.Stop()

	log.Info().Msg("Starting idle external agent cleanup loop (30min timeout, checks every 30s)")

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
	cutoff := time.Now().Add(-30 * time.Minute) // 30 minute idle threshold

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
		// NOTE: We previously checked for active Wolf streaming sessions here,
		// but Wolf sessions persist even without browser connections.
		// If the agent has been idle for 5 minutes (no Helix interactions),
		// it should be terminated regardless of Wolf session state.
		// The Wolf session will be cleaned up when the lobby is stopped.

		log.Info().
			Str("external_agent_id", activity.ExternalAgentID).
			Str("agent_type", activity.AgentType).
			Str("spectask_id_or_project_id", activity.SpecTaskID).
			Str("wolf_app_id", activity.WolfAppID).
			Time("last_interaction", activity.LastInteraction).
			Dur("idle_duration", time.Since(activity.LastInteraction)).
			Msg("Terminating idle external agent (no active streaming sessions)")

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

		// Try to stop via StopZedAgent first (uses session database record including soft-deleted)
		var stopError error
		if sessionIDToStop != "" {
			stopError = w.StopZedAgent(ctx, sessionIDToStop)
		}

		// If session not found (even in soft-deleted), use lobby ID/PIN from activity record
		// This handles cleanup when sessions are hard-deleted or missing
		if stopError != nil && strings.Contains(stopError.Error(), "not found in database") && activity.WolfLobbyID != "" {
			log.Info().
				Str("external_agent_id", activity.ExternalAgentID).
				Str("lobby_id", activity.WolfLobbyID).
				Msg("Session deleted from database, stopping lobby using activity record credentials")

			// Convert PIN string to []int16
			var lobbyPIN []int16
			if activity.WolfLobbyPIN != "" && len(activity.WolfLobbyPIN) == 4 {
				lobbyPIN = make([]int16, 4)
				for i, ch := range activity.WolfLobbyPIN {
					lobbyPIN[i] = int16(ch - '0')
				}
			}

			// Stop lobby directly using Wolf API
			// TODO: Activity record needs wolf_instance_id field for multi-Wolf support
			// For now, this will only work if all sessions are on local Wolf
			stopReq := &wolf.StopLobbyRequest{
				LobbyID: activity.WolfLobbyID,
				PIN:     lobbyPIN,
			}
			log.Warn().
				Str("lobby_id", activity.WolfLobbyID).
				Msg("Attempting to stop lobby without Wolf instance ID - only works for local Wolf (activity record needs wolf_instance_id field)")
			err := w.getWolfClient("local").StopLobby(ctx, stopReq)
			if err != nil {
				log.Error().
					Err(err).
					Str("lobby_id", activity.WolfLobbyID).
					Str("external_agent_id", activity.ExternalAgentID).
					Msg("Failed to stop Wolf lobby using activity record")
				// Continue with cleanup anyway - record the failure but clean up activity
			} else {
				log.Info().
					Str("lobby_id", activity.WolfLobbyID).
					Str("external_agent_id", activity.ExternalAgentID).
					Msg("✅ Wolf lobby stopped successfully using activity record")
			}
		} else if stopError != nil {
			log.Error().
				Err(stopError).
				Str("session_id", sessionIDToStop).
				Str("external_agent_id", activity.ExternalAgentID).
				Msg("Failed to stop idle Zed agent")
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

// wolfResourceMonitoringLoop logs Wolf resource metrics every minute for observability
func (w *WolfExecutor) wolfResourceMonitoringLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute) // Log every minute
	defer ticker.Stop()

	log.Info().Msg("Starting Wolf resource monitoring loop (logs metrics every 60s)")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Wolf resource monitoring loop stopped")
			return

		case <-ticker.C:
			w.logWolfResourceMetrics(ctx)
		}
	}
}

// logWolfResourceMetrics logs detailed Wolf resource usage for trend analysis
func (w *WolfExecutor) logWolfResourceMetrics(ctx context.Context) {
	// Get all Wolf instances
	instances, err := w.store.ListWolfInstances(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list Wolf instances for monitoring")
		return
	}

	// Monitor each Wolf instance
	for _, instance := range instances {
		memory, err := w.getWolfClient(instance.ID).GetSystemMemory(ctx)
		if err != nil {
			log.Error().
				Err(err).
				Str("wolf_instance_id", instance.ID).
				Msg("Failed to get Wolf memory stats for monitoring")
			continue
		}

		// Log metrics for this Wolf instance
		w.logWolfInstanceMetrics(instance.ID, memory)
	}
}

// logWolfInstanceMetrics logs metrics for a specific Wolf instance
func (w *WolfExecutor) logWolfInstanceMetrics(instanceID string, memory *wolf.SystemMemoryResponse) {

	// Log overall Wolf metrics
	log.Info().
		Str("wolf_instance_id", instanceID).
		Int64("wolf_process_rss_mb", memory.ProcessRSSBytes/(1024*1024)).
		Int64("wolf_gstreamer_buffer_mb", memory.GStreamerBufferBytes/(1024*1024)).
		Int64("wolf_total_memory_mb", memory.TotalMemoryBytes/(1024*1024)).
		Int("active_apps", len(memory.Apps)).
		Int("active_lobbies", len(memory.Lobbies)).
		Int("active_clients", len(memory.Clients)).
		Msg("📊 Wolf Resource Monitoring")

	// Log GPU metrics if available
	if memory.GPUStats != nil {
		log.Info().
			Str("gpu_name", memory.GPUStats.GPUName).
			Int("encoder_sessions", memory.GPUStats.EncoderSessionCount).
			Float64("encoder_avg_fps", memory.GPUStats.EncoderAverageFPS).
			Int("encoder_latency_us", memory.GPUStats.EncoderAverageLatencyUs).
			Int("encoder_utilization_pct", memory.GPUStats.EncoderUtilizationPercent).
			Int("gpu_utilization_pct", memory.GPUStats.GPUUtilizationPercent).
			Int("memory_utilization_pct", memory.GPUStats.MemoryUtilizationPercent).
			Int("memory_used_mb", memory.GPUStats.MemoryUsedMB).
			Int("memory_total_mb", memory.GPUStats.MemoryTotalMB).
			Int("temperature_c", memory.GPUStats.TemperatureCelsius).
			Msg("🎮 GPU Metrics")

		// Track if we've ever seen valid GPU stats (to distinguish NVIDIA vs non-NVIDIA systems)
		if memory.GPUStats.GPUName != "" && memory.GPUStats.MemoryTotalMB > 0 {
			w.gpuStatsMutex.Lock()
			w.hasSeenValidGPUStats = true
			w.gpuStatsMutex.Unlock()
		}

		// CRITICAL: Detect when GPU monitoring is broken (nvidia-smi/rocm-smi failure)
		// Only log scary error if we've previously seen valid GPU stats (system with GPU monitoring)
		// This avoids false alarms on systems without GPU monitoring tools
		if memory.GPUStats.GPUName == "" || memory.GPUStats.MemoryTotalMB == 0 {
			w.gpuStatsMutex.RLock()
			hasSeenValid := w.hasSeenValidGPUStats
			w.gpuStatsMutex.RUnlock()

			if hasSeenValid {
				// GPU monitoring was working before but now broken - CRITICAL ALERT!
				log.Error().
					Int("active_lobbies", len(memory.Lobbies)).
					Msg("⚠️ GPU MONITORING FAILED - GPU monitoring tool stopped working! Approaching resource limits!")
			} else {
				// Never seen valid GPU stats - probably system without GPU monitoring tools, no alert needed
				log.Debug().Msg("GPU stats not available (likely system without GPU monitoring tools)")
			}
		}
	} else {
		log.Debug().Msg("GPU stats not available from Wolf")
	}

	// Log GStreamer pipeline stats if available
	if memory.GStreamerPipelines != nil {
		log.Info().
			Int("producer_pipelines", memory.GStreamerPipelines.ProducerPipelines).
			Int("consumer_pipelines", memory.GStreamerPipelines.ConsumerPipelines).
			Int("total_pipelines", memory.GStreamerPipelines.TotalPipelines).
			Msg("🎬 GStreamer Pipelines")
	}

	// Log per-lobby breakdown if we have lobbies
	if len(memory.Lobbies) > 0 {
		for _, lobby := range memory.Lobbies {
			log.Debug().
				Str("lobby_id", lobby.LobbyID).
				Str("lobby_name", lobby.LobbyName).
				Int("client_count", lobby.ClientCount).
				Int64("memory_bytes", lobby.MemoryBytes).
				Msg("🏛️ Lobby details")
		}
	}
}

// cleanupOrphanedWolfUISessionsLoop runs periodically to cleanup Wolf-UI streaming sessions
// that don't have a corresponding active Zed container (orphaned after crashes, etc.)
func (w *WolfExecutor) cleanupOrphanedWolfUISessionsLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute) // Check every 2 minutes
	defer ticker.Stop()

	log.Info().Msg("Starting orphaned Wolf-UI session cleanup loop (checks every 2 minutes)")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Orphaned Wolf-UI cleanup loop context canceled")
			return
		case <-ticker.C:
			w.cleanupOrphanedWolfUISessions(ctx)
		}
	}
}

// FindExistingLobbyForSession checks if a lobby already exists for this Helix session
// Returns lobby ID if found, empty string if not found
// This prevents creating duplicate lobbies when resume endpoint is called multiple times
// PUBLIC: Used by both StartZedAgent and getSessionWolfAppState
func (w *WolfExecutor) FindExistingLobbyForSession(ctx context.Context, sessionID string) (string, error) {
	// Check cache first (prevents Wolf API spam from dashboard polling)
	w.lobbyCacheMutex.RLock()
	if entry, exists := w.lobbyCache[sessionID]; exists {
		age := time.Since(entry.timestamp)
		if age < w.lobbyCacheTTL {
			w.lobbyCacheMutex.RUnlock()
			// Cache hit - return cached lobby ID (no logging, too noisy)
			return entry.lobbyID, nil
		}
	}
	w.lobbyCacheMutex.RUnlock()

	// Cache miss or expired - query Wolf (no logging, too noisy)

	// Look up session to get Wolf instance ID
	session, err := w.store.GetSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("failed to get session for Wolf instance lookup: %w", err)
	}

	wolfInstanceID := session.WolfInstanceID
	if wolfInstanceID == "" {
		return "", fmt.Errorf("session %s has no Wolf instance ID - session may be corrupted or from before multi-Wolf support", sessionID)
	}

	// Get all active lobbies from Wolf
	lobbies, err := w.getWolfClient(wolfInstanceID).ListLobbies(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list Wolf lobbies: %w", err)
	}

	var foundLobbyID string

	// Search for lobby with matching HELIX_SESSION_ID in env vars
	for _, lobby := range lobbies {
		if runnerMap, ok := lobby.Runner.(map[string]interface{}); ok {
			if envList, ok := runnerMap["env"].([]interface{}); ok {
				for _, envVar := range envList {
					if envStr, ok := envVar.(string); ok {
						// Check for HELIX_SESSION_ID=<session_id>
						expectedEnv := fmt.Sprintf("HELIX_SESSION_ID=%s", sessionID)
						if envStr == expectedEnv {
							log.Debug().
								Str("lobby_id", lobby.ID).
								Str("session_id", sessionID).
								Msg("Found existing lobby for session")
							foundLobbyID = lobby.ID
							break
						}
					}
				}
			}
		}
		if foundLobbyID != "" {
			break
		}
	}

	// Cache the result (even if empty - prevents repeated queries for non-existent lobbies)
	w.lobbyCacheMutex.Lock()
	w.lobbyCache[sessionID] = &lobbyCacheEntry{
		lobbyID:   foundLobbyID,
		timestamp: time.Now(),
	}
	w.lobbyCacheMutex.Unlock()

	return foundLobbyID, nil
}

// cleanupOrphanedWolfUISessions removes Wolf-UI streaming sessions without active Zed containers
func (w *WolfExecutor) cleanupOrphanedWolfUISessions(ctx context.Context) {
	// Get all Wolf instances
	instances, err := w.store.ListWolfInstances(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list Wolf instances for orphan cleanup")
		return
	}

	// Clean up orphaned sessions on each Wolf instance
	for _, instance := range instances {
		w.cleanupOrphanedWolfUISessionsForInstance(ctx, instance.ID)
	}
}

// cleanupOrphanedWolfUISessionsForInstance cleans up orphans on a specific Wolf instance
func (w *WolfExecutor) cleanupOrphanedWolfUISessionsForInstance(ctx context.Context, wolfInstanceID string) {
	// Get all Wolf-UI streaming sessions for this instance
	sessions, err := w.getWolfClient(wolfInstanceID).ListSessions(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Str("wolf_instance_id", wolfInstanceID).
			Msg("Failed to list Wolf sessions for orphan cleanup")
		return
	}

	if len(sessions) == 0 {
		return // No sessions to check
	}

	// Get all active lobbies from Wolf to check for orphans
	// CRITICAL: Use Wolf lobbies list, not in-memory map (survives API restarts)
	lobbies, err := w.getWolfClient(wolfInstanceID).ListLobbies(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Str("wolf_instance_id", wolfInstanceID).
			Msg("Failed to list Wolf lobbies for orphan cleanup")
		return
	}

	// Build map of session IDs that have active lobbies
	activeSessions := make(map[string]bool)
	for _, lobby := range lobbies {
		// Extract session ID from lobby name: "Agent {session_suffix}"
		// Or check lobby's runner env vars for HELIX_SESSION_ID
		// For now, build container name from lobby ID and check if it's in helix-agent pattern

		// Lobbies are named like "Agent 4jqgmj" (last 6 chars of session ID)
		// This is insufficient for exact matching, so we need a better approach

		// Instead: Check if lobby's container is running by using lobby ID
		// Container name format: zed-external-{session_id_without_ses_}_{lobby_id}
		// We can't reverse-engineer session ID from this easily

		// BETTER: Extract HELIX_SESSION_ID from lobby's runner env vars if available
		if runnerMap, ok := lobby.Runner.(map[string]interface{}); ok {
			if envList, ok := runnerMap["env"].([]interface{}); ok {
				for _, envVar := range envList {
					if envStr, ok := envVar.(string); ok {
						if strings.HasPrefix(envStr, "HELIX_SESSION_ID=") {
							sessionID := strings.TrimPrefix(envStr, "HELIX_SESSION_ID=")
							activeSessions[sessionID] = true
							break
						}
					}
				}
			}
		}
	}

	log.Info().
		Int("total_sessions", len(sessions)).
		Int("active_lobbies", len(lobbies)).
		Int("tracked_sessions", len(activeSessions)).
		Msg("Checking for orphaned Wolf-UI sessions")

	stoppedCount := 0

	for _, session := range sessions {
		// Only check sessions with our helix-agent prefix
		if !strings.HasPrefix(session.ClientUniqueID, "helix-agent-") {
			continue
		}

		// Extract session ID from client_unique_id: helix-agent-{session_id}-{instance_id}
		parts := strings.Split(session.ClientUniqueID, "-")
		if len(parts) < 3 {
			continue // Invalid format
		}

		// Session ID is part after "helix-agent-"
		sessionID := strings.TrimPrefix(session.ClientUniqueID, "helix-agent-")
		// Remove instance ID suffix (everything after last hyphen for long IDs)
		if idx := strings.LastIndex(sessionID, "-"); idx > 20 { // Session IDs are ~30 chars
			sessionID = sessionID[:idx]
		}

		// Check if lobby exists for this session (using Wolf API, not in-memory map)
		if activeSessions[sessionID] {
			// Session has an active lobby, keep it
			continue
		}

		// No active lobby found - this is an orphaned session
		log.Info().
			Str("client_id", session.ClientID).
			Str("client_unique_id", session.ClientUniqueID).
			Str("session_id", sessionID).
			Str("wolf_instance_id", wolfInstanceID).
			Msg("🧹 Found orphaned Wolf-UI session (no lobby), stopping...")

		err := w.getWolfClient(wolfInstanceID).StopSession(ctx, session.ClientID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("client_id", session.ClientID).
				Msg("Failed to stop orphaned Wolf-UI session")
		} else {
			stoppedCount++
			log.Info().
				Str("client_id", session.ClientID).
				Str("session_id", sessionID).
				Msg("✅ Stopped orphaned Wolf-UI session")
		}
	}

	if stoppedCount > 0 {
		log.Info().
			Int("stopped_count", stoppedCount).
			Msg("Completed orphaned Wolf-UI session cleanup")
	}
}

// execCommand executes a command in the specified directory and returns output
func execCommand(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// getContainerScreenshot fetches a screenshot from the container's screenshot server via RevDial
// Used for saving paused screenshots - non-critical, fails gracefully if RevDial unavailable
func (w *WolfExecutor) getContainerScreenshot(ctx context.Context, sessionID string) ([]byte, error) {
	// RevDial connection manager may not be available in tests or local dev
	if w.connman == nil {
		return nil, fmt.Errorf("connection manager not available (tests or local dev mode)")
	}

	// Try to get RevDial connection to sandbox
	runnerID := fmt.Sprintf("sandbox-%s", sessionID)
	revDialConn, err := w.connman.Dial(ctx, runnerID)
	if err != nil {
		// RevDial not available - this is non-critical (just a paused screenshot preview)
		return nil, fmt.Errorf("RevDial not available for paused screenshot (non-critical): %w", err)
	}
	defer revDialConn.Close()

	// Send HTTP request over RevDial tunnel
	httpReq, err := http.NewRequest("GET", "http://localhost:9876/screenshot", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create screenshot request: %w", err)
	}

	// Write request to RevDial connection
	if err := httpReq.Write(revDialConn); err != nil {
		return nil, fmt.Errorf("failed to send screenshot request via RevDial: %w", err)
	}

	// Read response from RevDial connection
	screenshotResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to read screenshot response from RevDial: %w", err)
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
