package external_agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// HydraExecutor implements the Executor interface using Hydra for dev container management.
//
// Architecture: Helix API -> Hydra -> Docker -> Dev Container
// Video streaming: WebSocket streaming (ws_stream.go)
type HydraExecutor struct {
	store    store.Store
	sessions map[string]*ZedSession
	mutex    sync.RWMutex

	// Configuration
	helixAPIURL   string
	helixAPIToken string

	// Workspace path configuration
	workspaceBasePathForContainer string // Path as seen from inside dev container
	workspaceBasePathForCloning   string // Path on sandbox filesystem (Hydra creates dirs)

	// RevDial connection manager for communicating with Hydra in sandbox
	connman connmanInterface

	// Per-session creation locks to prevent duplicate container creation
	creationLocks      map[string]*sync.Mutex
	creationLocksMutex sync.Mutex

	// GPU configuration
	gpuVendor string // "nvidia", "amd", "intel", ""
}

// connmanInterface abstracts the connection manager for RevDial connections to sandboxes
type connmanInterface interface {
	Dial(ctx context.Context, deviceID string) (net.Conn, error)
}

// HydraExecutorConfig holds configuration for creating a HydraExecutor
type HydraExecutorConfig struct {
	Store                         store.Store
	HelixAPIURL                   string
	HelixAPIToken                 string
	WorkspaceBasePathForContainer string
	WorkspaceBasePathForCloning   string
	Connman                       connmanInterface
	GPUVendor                     string
}

// NewHydraExecutor creates a new HydraExecutor instance
func NewHydraExecutor(cfg HydraExecutorConfig) *HydraExecutor {
	return &HydraExecutor{
		store:                         cfg.Store,
		sessions:                      make(map[string]*ZedSession),
		helixAPIURL:                   cfg.HelixAPIURL,
		helixAPIToken:                 cfg.HelixAPIToken,
		workspaceBasePathForContainer: cfg.WorkspaceBasePathForContainer,
		workspaceBasePathForCloning:   cfg.WorkspaceBasePathForCloning,
		connman:                       cfg.Connman,
		creationLocks:                 make(map[string]*sync.Mutex),
		gpuVendor:                     cfg.GPUVendor,
	}
}

// StartDesktop starts a dev container using Hydra
func (h *HydraExecutor) StartDesktop(ctx context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
	// Get or create a per-session lock to prevent concurrent container creation
	h.creationLocksMutex.Lock()
	sessionLock, exists := h.creationLocks[agent.SessionID]
	if !exists {
		sessionLock = &sync.Mutex{}
		h.creationLocks[agent.SessionID] = sessionLock
	}
	h.creationLocksMutex.Unlock()

	// Lock this specific session to prevent duplicate container creation
	sessionLock.Lock()
	defer sessionLock.Unlock()

	log.Info().
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("project_path", agent.ProjectPath).
		Msg("Starting dev container via Hydra")

	// Check if session already exists and is running
	h.mutex.RLock()
	existingSession, exists := h.sessions[agent.SessionID]
	h.mutex.RUnlock()

	if exists && existingSession.Status == "running" {
		log.Info().
			Str("session_id", agent.SessionID).
			Msg("Dev container already running, returning existing session")
		return &types.DesktopAgentResponse{
			SessionID:     agent.SessionID,
			ScreenshotURL: fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
			StreamURL:     fmt.Sprintf("/api/v1/sessions/%s/stream", agent.SessionID),
			Status:        "running",
		}, nil
	}

	// Get Hydra client via RevDial
	// Hydra runner ID follows pattern: hydra-{SANDBOX_INSTANCE_ID}
	// Determine container type first (needed for sandbox selection)
	containerType := h.parseContainerType(agent.DesktopType)

	// Determine sandbox ID - use agent's preference or find an available one
	sandboxID := agent.SandboxID
	if sandboxID == "" {
		// Find an available sandbox with the required desktop image
		// If UseHostDocker is set, we need a sandbox with PrivilegedMode enabled
		sandbox, err := h.store.FindAvailableSandbox(ctx, containerType, agent.UseHostDocker)
		if err != nil {
			return nil, fmt.Errorf("failed to find available sandbox: %w", err)
		}
		if sandbox != nil {
			sandboxID = sandbox.ID
			log.Info().
				Str("sandbox_id", sandboxID).
				Str("container_type", containerType).
				Bool("use_host_docker", agent.UseHostDocker).
				Msg("Auto-selected available sandbox")
		} else {
			if agent.UseHostDocker {
				return nil, fmt.Errorf("no privileged sandbox available (UseHostDocker requires HYDRA_PRIVILEGED_MODE_ENABLED=true on sandbox)")
			}
			// Fallback to "local" if no sandbox found (for backwards compatibility)
			sandboxID = "local"
			log.Warn().
				Str("container_type", containerType).
				Msg("No available sandbox found, falling back to 'local'")
		}
	}
	hydraRunnerID := fmt.Sprintf("hydra-%s", sandboxID)
	hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)

	// NOTE: GPU vendor is NOT passed from API - Hydra reads it from its own
	// GPU_VENDOR env var (set by install.sh). This avoids the complexity of
	// the API needing to know the sandbox's GPU type.

	// Determine workspace directory
	workspaceDir := agent.WorkDir
	if workspaceDir == "" {
		if agent.SpecTaskID != "" {
			workspaceDir = filepath.Join(h.workspaceBasePathForCloning, "spec-tasks", agent.SpecTaskID)
		} else {
			workspaceDir = filepath.Join(h.workspaceBasePathForCloning, "sessions", agent.SessionID)
		}
	}

	// Ensure workspace directory exists
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		log.Warn().Err(err).Str("workspace_dir", workspaceDir).Msg("Failed to create workspace directory")
	}

	// Build container name
	containerName := fmt.Sprintf("%s-external-%s", containerType, strings.TrimPrefix(agent.SessionID, "ses_"))

	// Build container image (looks up version from sandbox heartbeat in database)
	image, err := h.getContainerImage(ctx, containerType, sandboxID, agent)
	if err != nil {
		return nil, fmt.Errorf("failed to get container image: %w", err)
	}

	// CRITICAL: Fetch user for git credentials
	// Enterprise ADO deployments reject commits from non-corporate email addresses
	var gitUserName, gitUserEmail string
	if agent.UserID != "" {
		user, err := h.store.GetUser(ctx, &store.GetUserQuery{ID: agent.UserID})
		if err != nil {
			return nil, fmt.Errorf("failed to get user for git config: %w", err)
		}
		if user != nil {
			gitUserName = user.FullName
			gitUserEmail = user.Email
			// Fall back to username if full name is empty
			if gitUserName == "" {
				gitUserName = user.Username
			}
		}
	}
	if gitUserEmail == "" {
		return nil, fmt.Errorf("GIT_USER_EMAIL not available for user %s - enterprise git requires user email", agent.UserID)
	}

	// Build environment variables
	env := h.buildEnvVars(agent, containerType, workspaceDir)

	// Add git user config (required for enterprise git)
	if gitUserName != "" {
		env = append(env, fmt.Sprintf("GIT_USER_NAME=%s", gitUserName))
	}
	if gitUserEmail != "" {
		env = append(env, fmt.Sprintf("GIT_USER_EMAIL=%s", gitUserEmail))
	}

	// Fetch SpecTask info for git hooks and docker compose project naming
	var specDirName string
	var taskNumber int
	if agent.SpecTaskID != "" {
		specTask, err := h.store.GetSpecTask(ctx, agent.SpecTaskID)
		if err != nil {
			log.Warn().Err(err).Str("spec_task_id", agent.SpecTaskID).Msg("Failed to get spec task for design doc path")
		} else if specTask != nil {
			taskNumber = specTask.TaskNumber
			if specTask.DesignDocPath != "" {
				specDirName = specTask.DesignDocPath
			}
			log.Debug().
				Str("spec_task_id", agent.SpecTaskID).
				Str("spec_dir_name", specDirName).
				Int("task_number", taskNumber).
				Msg("Spec task info for git hooks and docker compose project naming")
		}
	}
	if specDirName != "" {
		env = append(env, fmt.Sprintf("HELIX_SPEC_DIR_NAME=%s", specDirName))
	}
	if taskNumber > 0 {
		env = append(env, fmt.Sprintf("HELIX_TASK_NUMBER=%d", taskNumber))
	}

	// Build repository info for startup script to clone
	// Format: "id:name:type,id:name:type,..." (same as wolf_executor)
	if len(agent.RepositoryIDs) > 0 {
		var repoSpecs []string
		for _, repoID := range agent.RepositoryIDs {
			repo, err := h.store.GetGitRepository(ctx, repoID)
			if err != nil {
				log.Warn().Err(err).Str("repo_id", repoID).Msg("Failed to get repository metadata")
				continue
			}
			// Format: id:name:type
			repoSpec := fmt.Sprintf("%s:%s:%s", repo.ID, repo.Name, repo.RepoType)
			repoSpecs = append(repoSpecs, repoSpec)
		}
		if len(repoSpecs) > 0 {
			env = append(env, fmt.Sprintf("HELIX_REPOSITORIES=%s", strings.Join(repoSpecs, ",")))
		}
	}

	// Get actual primary repository name (not just the ID)
	if agent.PrimaryRepositoryID != "" {
		repo, err := h.store.GetGitRepository(ctx, agent.PrimaryRepositoryID)
		if err != nil {
			log.Warn().Err(err).Str("repo_id", agent.PrimaryRepositoryID).Msg("Failed to get primary repository name")
		} else if repo != nil {
			env = append(env, fmt.Sprintf("HELIX_PRIMARY_REPO_NAME=%s", repo.Name))
			log.Info().
				Str("primary_repo_id", agent.PrimaryRepositoryID).
				Str("primary_repo_name", repo.Name).
				Msg("Primary repository for design docs worktree")
		}
	}

	// Build mounts
	mounts := h.buildMounts(agent, workspaceDir, containerType, agent.UseHostDocker)

	// Create dev container request
	// NOTE: GPUVendor is empty - Hydra reads it from its own GPU_VENDOR env var
	req := &hydra.CreateDevContainerRequest{
		SessionID:     agent.SessionID,
		Image:         image,
		ContainerName: containerName,
		Hostname:      containerName,
		Env:           env,
		Mounts:        mounts,
		DisplayWidth:  agent.DisplayWidth,
		DisplayHeight: agent.DisplayHeight,
		DisplayFPS:    agent.DisplayRefreshRate,
		ContainerType: hydra.DevContainerType(containerType),
		UserID:        agent.UserID,
		Network:       "bridge",
	}

	// If Hydra Docker isolation is enabled, create isolated dockerd first
	if agent.UseHydraDocker {
		log.Info().
			Str("session_id", agent.SessionID).
			Msg("Creating isolated Docker instance via Hydra")

		dockerReq := &hydra.CreateDockerInstanceRequest{
			ScopeType:     hydra.ScopeTypeSession,
			ScopeID:       agent.SessionID,
			UserID:        agent.UserID,
			UseHostDocker: agent.UseHostDocker,
		}
		dockerResp, err := hydraClient.CreateDockerInstance(ctx, dockerReq)
		if err != nil {
			return nil, fmt.Errorf("failed to create isolated Docker instance: %w", err)
		}
		req.DockerSocket = dockerResp.DockerSocket

		log.Info().
			Str("session_id", agent.SessionID).
			Str("docker_socket", dockerResp.DockerSocket).
			Msg("Created isolated Docker instance")
	}

	// Create dev container via Hydra
	log.Info().
		Str("session_id", agent.SessionID).
		Str("image", req.Image).
		Str("container_name", req.ContainerName).
		Str("container_type", string(req.ContainerType)).
		Msg("Creating dev container via Hydra")

	resp, err := hydraClient.CreateDevContainer(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create dev container via Hydra: %w", err)
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("container_id", resp.ContainerID).
		Str("container_name", resp.ContainerName).
		Str("ip_address", resp.IPAddress).
		Msg("Dev container created successfully via Hydra")

	// Wait for desktop-bridge to be ready before returning
	// Desktop-bridge takes time to start: waits for D-Bus, Wayland, portal, GStreamer init
	// Without this, frontend connects immediately but screenshot/video fail
	// Uses RevDial for health check since container IP is inside sandbox's DinD network
	if err := h.waitForDesktopBridge(ctx, agent.SessionID); err != nil {
		log.Warn().Err(err).
			Str("session_id", agent.SessionID).
			Msg("Desktop bridge not ready (continuing anyway, frontend may need to retry)")
		// Don't fail - container is running, just not fully ready yet
		// Frontend should handle this gracefully with retry logic
	}

	// Track session
	session := &ZedSession{
		SessionID: agent.SessionID,
		UserID:    agent.UserID,
		Status:         "running",
		StartTime:      time.Now(),
		LastAccess:     time.Now(),
		ProjectPath:    agent.ProjectPath,
		ContainerName:  resp.ContainerName,
		ContainerID:    resp.ContainerID,
		ContainerIP:    resp.IPAddress,
		SandboxID:      sandboxID,
		// DevContainerID is not used in Hydra mode, but we store container info here
	}
	h.mutex.Lock()
	h.sessions[agent.SessionID] = session
	h.mutex.Unlock()

	// Update database session with container info and debug info
	if dbSession, err := h.store.GetSession(ctx, agent.SessionID); err == nil {
		dbSession.Metadata.ContainerName = resp.ContainerName
		dbSession.Metadata.ContainerID = resp.ContainerID
		dbSession.Metadata.ContainerIP = resp.IPAddress
		dbSession.Metadata.ExecutorMode = "hydra"
		// CRITICAL: Set DevContainerID - used by exploratory session to check if container is running
		dbSession.Metadata.DevContainerID = resp.ContainerID

		// Store debug info in Metadata (serialized as "config" in JSON for frontend)
		dbSession.Metadata.SwayVersion = resp.DesktopVersion
		dbSession.Metadata.GPUVendor = resp.GPUVendor
		dbSession.Metadata.RenderNode = resp.RenderNode

		// Store sandbox ID on the session for port proxying
		dbSession.SandboxID = sandboxID

		if _, err := h.store.UpdateSession(ctx, *dbSession); err != nil {
			log.Warn().Err(err).Str("session_id", agent.SessionID).Msg("Failed to update session metadata with container info")
		}
	}

	return &types.DesktopAgentResponse{
		SessionID:      agent.SessionID,
		ScreenshotURL:  fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
		StreamURL:      fmt.Sprintf("/api/v1/sessions/%s/stream", agent.SessionID),
		Status:         "running",
		ContainerName:  resp.ContainerName,
		ContainerIP:    resp.IPAddress,
		SandboxID:      sandboxID,
		DevContainerID: resp.ContainerID, // Container ID for exploratory session tracking
	}, nil
}

// StopDesktop stops a dev container using Hydra
func (h *HydraExecutor) StopDesktop(ctx context.Context, sessionID string) error {
	log.Info().Str("session_id", sessionID).Msg("Stopping dev container via Hydra")

	h.mutex.Lock()
	session, exists := h.sessions[sessionID]
	var sandboxID string
	if exists {
		sandboxID = session.SandboxID
		delete(h.sessions, sessionID)
	}
	h.mutex.Unlock()

	// Get sandbox ID from database if not in memory
	// Use SandboxID as sandbox identifier for now (they're often the same or related)
	if sandboxID == "" {
		if dbSession, err := h.store.GetSessionIncludingDeleted(ctx, sessionID); err == nil {
			// Try SandboxID first, which indicates which sandbox is handling this session
			sandboxID = dbSession.SandboxID
		}
	}

	if sandboxID == "" {
		// Use "local" to match Hydra's default SANDBOX_INSTANCE_ID
		sandboxID = "local"
	}

	// Get Hydra client via RevDial
	hydraRunnerID := fmt.Sprintf("hydra-%s", sandboxID)
	hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)

	// Delete dev container via Hydra
	resp, err := hydraClient.DeleteDevContainer(ctx, sessionID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to delete dev container (may already be stopped)")
		// Don't return error - container might already be gone
	} else {
		log.Info().
			Str("session_id", sessionID).
			Str("container_id", resp.ContainerID).
			Msg("Dev container stopped successfully via Hydra")
	}

	// Revoke session-scoped ephemeral API keys
	// Keys are minted when desktop starts and should be revoked when it stops
	if err := h.revokeSessionAPIKeys(ctx, sessionID); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to revoke session API keys")
		// Don't fail the stop operation - key cleanup is best-effort
	}

	// Clean up creation lock
	h.creationLocksMutex.Lock()
	delete(h.creationLocks, sessionID)
	h.creationLocksMutex.Unlock()

	return nil
}

// revokeSessionAPIKeys revokes all ephemeral API keys associated with a session.
// This is called when a desktop shuts down to clean up session-scoped keys.
func (h *HydraExecutor) revokeSessionAPIKeys(ctx context.Context, sessionID string) error {
	// List all API keys and filter by session ID
	// Note: This could be optimized with a store method that filters directly
	keys, err := h.store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{})
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}

	var revokedCount int
	for _, key := range keys {
		if key.SessionID == sessionID {
			if err := h.store.DeleteAPIKey(ctx, key.Key); err != nil {
				log.Warn().Err(err).
					Str("key_prefix", key.Key[:8]+"...").
					Str("session_id", sessionID).
					Msg("Failed to revoke session API key")
				continue
			}
			revokedCount++
		}
	}

	if revokedCount > 0 {
		log.Info().
			Str("session_id", sessionID).
			Int("revoked_count", revokedCount).
			Msg("🔒 Revoked ephemeral session API keys on desktop stop")
	}

	return nil
}

// GetSession returns the session for the given session ID
func (h *HydraExecutor) GetSession(sessionID string) (*ZedSession, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	session, exists := h.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Update last access time
	session.LastAccess = time.Now()

	return session, nil
}

// CleanupExpiredSessions removes sessions that have been idle for too long
func (h *HydraExecutor) CleanupExpiredSessions(ctx context.Context, timeout time.Duration) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	now := time.Now()
	for sessionID, session := range h.sessions {
		if now.Sub(session.LastAccess) > timeout {
			log.Info().
				Str("session_id", sessionID).
				Dur("idle_time", now.Sub(session.LastAccess)).
				Msg("Cleaning up expired session")

			// Stop the container (in background)
			go func(sid string) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := h.StopDesktop(ctx, sid); err != nil {
					log.Warn().Err(err).Str("session_id", sid).Msg("Failed to stop expired session")
				}
			}(sessionID)
		}
	}
}

// ListSessions returns all active sessions
func (h *HydraExecutor) ListSessions() []*ZedSession {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	sessions := make([]*ZedSession, 0, len(h.sessions))
	for _, session := range h.sessions {
		sessions = append(sessions, session)
	}

	return sessions
}

// StartZedInstance starts a Zed instance (alias for StartDesktop for multi-session support)
func (h *HydraExecutor) StartZedInstance(ctx context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
	return h.StartDesktop(ctx, agent)
}

// CreateZedThread creates a new thread in an existing Zed instance
func (h *HydraExecutor) CreateZedThread(ctx context.Context, instanceID, threadID string, config map[string]interface{}) error {
	// Thread management is handled by the agent inside the container
	// This is a placeholder for future multi-thread support
	log.Info().
		Str("instance_id", instanceID).
		Str("thread_id", threadID).
		Msg("CreateZedThread called (no-op in Hydra executor)")
	return nil
}

// DeleteZedThread deletes a thread from a Zed instance
func (h *HydraExecutor) DeleteZedThread(ctx context.Context, instanceID, threadID string) error {
	log.Info().
		Str("instance_id", instanceID).
		Str("thread_id", threadID).
		Msg("DeleteZedThread called (no-op in Hydra executor)")
	return nil
}

// StopZedInstance stops a Zed instance (alias for StopDesktop)
func (h *HydraExecutor) StopZedInstance(ctx context.Context, instanceID string) error {
	return h.StopDesktop(ctx, instanceID)
}

// GetInstanceStatus returns the status of a Zed instance
func (h *HydraExecutor) GetInstanceStatus(instanceID string) (*ZedInstanceStatus, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	session, exists := h.sessions[instanceID]
	if !exists {
		return nil, fmt.Errorf("instance %s not found", instanceID)
	}

	return &ZedInstanceStatus{
		InstanceID:  instanceID,
		Status:      session.Status,
		ThreadCount: 1,
		ProjectPath: session.ProjectPath,
	}, nil
}

// ListInstanceThreads returns threads for an instance
func (h *HydraExecutor) ListInstanceThreads(instanceID string) ([]*ZedThreadInfo, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	_, exists := h.sessions[instanceID]
	if !exists {
		return nil, fmt.Errorf("instance %s not found", instanceID)
	}

	// Hydra executor doesn't support multi-threading yet
	return []*ZedThreadInfo{}, nil
}

// FindContainerBySessionID finds the container name for a session
func (h *HydraExecutor) FindContainerBySessionID(ctx context.Context, helixSessionID string) (string, error) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	// First check our in-memory sessions
	for _, session := range h.sessions {
		if session.SessionID == helixSessionID {
			if session.ContainerName != "" {
				return session.ContainerName, nil
			}
		}
	}

	// Try to get from database
	dbSession, err := h.store.GetSession(ctx, helixSessionID)
	if err != nil {
		return "", fmt.Errorf("session not found: %w", err)
	}

	if dbSession.Metadata.ContainerName != "" {
		return dbSession.Metadata.ContainerName, nil
	}

	return "", fmt.Errorf("no container found for session %s", helixSessionID)
}

// HasRunningContainer checks if a session has a running container
func (h *HydraExecutor) HasRunningContainer(ctx context.Context, sessionID string) bool {
	h.mutex.RLock()
	session, exists := h.sessions[sessionID]
	h.mutex.RUnlock()

	if !exists {
		return false
	}

	return session.Status == "running" && session.ContainerID != ""
}

// Helper methods

// parseContainerType converts desktop type string to container type
func (h *HydraExecutor) parseContainerType(desktopType string) string {
	switch strings.ToLower(desktopType) {
	case "ubuntu", "gnome":
		return "ubuntu"
	case "headless":
		return "headless"
	default:
		return "ubuntu" // Default to Ubuntu (GNOME)
	}
}

// getContainerImage returns the appropriate container image for the given type.
// Looks up desktop_versions from the sandbox's database record (populated by heartbeat).
// Returns an error if the version cannot be determined - never falls back to :latest.
func (h *HydraExecutor) getContainerImage(ctx context.Context, containerType string, sandboxID string, agent *types.DesktopAgent) (string, error) {
	// Use custom image if provided
	if agent.CustomImage != "" {
		return agent.CustomImage, nil
	}

	// Map container type to image name and version key
	var imageName, versionKey string
	switch containerType {
	case "ubuntu":
		imageName = "helix-ubuntu"
		versionKey = "ubuntu"
	default:
		imageName = "helix-sway"
		versionKey = "sway"
	}

	// Look up desktop_versions from sandbox's database record
	// The sandbox heartbeat daemon updates this with versions from /opt/images/*.version
	sandbox, err := h.store.GetSandbox(ctx, sandboxID)
	if err != nil {
		return "", fmt.Errorf("failed to get sandbox %q from database: %w (is the sandbox heartbeat running?)", sandboxID, err)
	}

	// Parse desktop_versions JSON from sandbox record
	var desktopVersions map[string]string
	if len(sandbox.DesktopVersions) > 0 {
		if err := json.Unmarshal(sandbox.DesktopVersions, &desktopVersions); err != nil {
			return "", fmt.Errorf("failed to parse desktop_versions JSON for sandbox %q: %w", sandboxID, err)
		}
	}

	// Get version from parsed map
	if version, ok := desktopVersions[versionKey]; ok && version != "" {
		log.Info().
			Str("sandbox_id", sandboxID).
			Str("image", imageName).
			Str("version", version).
			Msg("Using desktop version from sandbox heartbeat")
		return imageName + ":" + version, nil
	}

	return "", fmt.Errorf("no %q version found in sandbox %q heartbeat (desktop_versions: %v) - is the sandbox heartbeat running?",
		versionKey, sandboxID, desktopVersions)
}

// buildEnvVars builds environment variables for the container
func (h *HydraExecutor) buildEnvVars(agent *types.DesktopAgent, containerType, workspaceDir string) []string {
	// Build GPU devices string
	gpuDevices := "/dev/dri/card*:/dev/dri/renderD*:/dev/uinput:/dev/input/event*:/dev/input/js*:/dev/input/mice"

	// Determine Helix URL for Zed's WebSocket connection
	zedHelixURL := strings.TrimPrefix(h.helixAPIURL, "https://")
	zedHelixURL = strings.TrimPrefix(zedHelixURL, "http://")
	zedHelixTLS := strings.HasPrefix(h.helixAPIURL, "https://")

	env := []string{
		// Core Helix env vars
		fmt.Sprintf("HELIX_API_URL=%s", h.helixAPIURL),
		fmt.Sprintf("HELIX_SESSION_ID=%s", agent.SessionID),
		fmt.Sprintf("HELIX_WORKSPACE_DIR=%s", h.workspaceBasePathForContainer),
		// WORKSPACE_DIR is the actual sandbox path (e.g., /data/workspaces/spec-tasks/spt_xxx)
		// This is required by the docker wrapper script to translate /home/retro/work paths
		// to paths that the DinD daemon can access. Using workspaceBasePathForContainer (/workspace)
		// doesn't work because the DinD daemon only has /data/workspaces mounted, not /workspace.
		fmt.Sprintf("WORKSPACE_DIR=%s", workspaceDir),
		// XDG_RUNTIME_DIR is required for PipeWire, D-Bus, and Wayland sockets
		"XDG_RUNTIME_DIR=/run/user/1000",
		// Override default UMASK=000 which causes permission issues
		"UMASK=022",
		// RevDial connection - startup-app.sh expects these specific names
		fmt.Sprintf("HELIX_API_BASE_URL=%s", h.helixAPIURL),

		// GPU/input device passthrough
		fmt.Sprintf("GOW_REQUIRED_DEVICES=%s", gpuDevices),

		// LLM proxy configuration for Zed's built-in agents
		// SECURITY: ANTHROPIC_API_KEY, OPENAI_API_KEY are set via agent.Env with session-scoped token
		// (see addUserAPITokenToAgent). Only set the base URLs here - NOT the runner token.
		fmt.Sprintf("ANTHROPIC_BASE_URL=%s", h.helixAPIURL),
		fmt.Sprintf("OPENAI_BASE_URL=%s/v1", h.helixAPIURL),

		// Zed sync configuration
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		"ZED_ALLOW_EMULATED_GPU=1", // Allow software rendering with llvmpipe
		fmt.Sprintf("ZED_HELIX_URL=%s", zedHelixURL),
		fmt.Sprintf("ZED_HELIX_TLS=%t", zedHelixTLS),
		"ZED_HELIX_SKIP_TLS_VERIFY=true", // Enterprise internal CAs

		// Debug logging
		"RUST_LOG=info,gst_wayland_display=debug",
		"SHOW_ACP_DEBUG_LOGS=true",

		// Settings sync daemon port
		"SETTINGS_SYNC_PORT=9877",

		// ZED_WORK_DIR: Consistent cwd for ACP session storage
		"ZED_WORK_DIR=/home/retro/work",

		// Keep desktop alive when Zed restarts
		"SWAY_STOP_ON_APP_EXIT=no",
	}

	// SECURITY: Runner token is NOT passed to containers - users must never see it
	// All API authentication uses USER_API_TOKEN (set via agent.Env with dev container token)
	// Settings-sync-daemon also uses USER_API_TOKEN for API calls

	// Agent identification
	env = append(env,
		fmt.Sprintf("HELIX_AGENT_INSTANCE_ID=%s", agent.SessionID),
		"HELIX_SCOPE_TYPE=session",
		fmt.Sprintf("HELIX_SCOPE_ID=%s", agent.SessionID),
		fmt.Sprintf("HELIX_USER_ID=%s", agent.UserID),
	)

	// Helix session ID for WebSocket communication
	if agent.SessionID != "" {
		env = append(env, fmt.Sprintf("HELIX_SESSION_ID=%s", agent.SessionID))
	}

	// Add project path if provided
	if agent.ProjectPath != "" {
		env = append(env, fmt.Sprintf("HELIX_PROJECT_PATH=%s", agent.ProjectPath))
	}

	// Add Git repository URL for cloning
	if agent.GitRepoURL != "" {
		env = append(env, fmt.Sprintf("GIT_REPO_URL=%s", agent.GitRepoURL))
	}
	if agent.GitBranch != "" {
		env = append(env, fmt.Sprintf("GIT_BRANCH=%s", agent.GitBranch))
	}

	// Branch configuration
	if agent.BranchMode != "" {
		env = append(env, fmt.Sprintf("HELIX_BRANCH_MODE=%s", agent.BranchMode))
	}
	if agent.BaseBranch != "" {
		env = append(env, fmt.Sprintf("HELIX_BASE_BRANCH=%s", agent.BaseBranch))
	}
	if agent.WorkingBranch != "" {
		env = append(env, fmt.Sprintf("HELIX_WORKING_BRANCH=%s", agent.WorkingBranch))
	}

	// SpecTask info
	if agent.SpecTaskID != "" {
		env = append(env, fmt.Sprintf("HELIX_SPEC_TASK_ID=%s", agent.SpecTaskID))
	}
	if agent.ProjectID != "" {
		env = append(env, fmt.Sprintf("HELIX_PROJECT_ID=%s", agent.ProjectID))
	}
	// NOTE: HELIX_PRIMARY_REPO_NAME is set in StartDesktop after fetching actual repo name

	// Display settings for non-headless containers
	if containerType != "headless" {
		width, height, refreshRate := agent.GetEffectiveResolution()
		env = append(env,
			fmt.Sprintf("GAMESCOPE_WIDTH=%d", width),
			fmt.Sprintf("GAMESCOPE_HEIGHT=%d", height),
			fmt.Sprintf("GAMESCOPE_REFRESH=%d", refreshRate),
			fmt.Sprintf("HELIX_DESKTOP_TYPE=%s", containerType),
		)

		// Zoom level
		zoomLevel := 100
		if agent.ZoomLevel > 0 {
			zoomLevel = agent.ZoomLevel
		}
		env = append(env, fmt.Sprintf("HELIX_ZOOM_LEVEL=%d", zoomLevel))

		// Display scale for KDE/Qt
		if agent.DisplayScale > 0 {
			env = append(env, fmt.Sprintf("HELIX_DISPLAY_SCALE=%d", agent.DisplayScale))
		}
	}

	// Add GPU-specific environment variables
	switch h.gpuVendor {
	case "nvidia":
		env = append(env, "NVIDIA_VISIBLE_DEVICES=all")
		env = append(env, "NVIDIA_DRIVER_CAPABILITIES=all")
	case "amd":
		env = append(env, "GOW_REQUIRED_DEVICES=/dev/dri/card*:/dev/dri/renderD*")
	case "intel":
		env = append(env, "GOW_REQUIRED_DEVICES=/dev/dri/card*:/dev/dri/renderD*")
	}

	// Add custom env vars from agent request (includes USER_API_TOKEN for git + RevDial)
	// These come LAST so they can override defaults (e.g., use user's token instead of runner token)
	hasUserAPIToken := false
	for _, e := range agent.Env {
		if strings.HasPrefix(e, "USER_API_TOKEN=") {
			hasUserAPIToken = true
			break
		}
	}
	log.Info().
		Int("agent_env_count", len(agent.Env)).
		Bool("has_user_api_token", hasUserAPIToken).
		Str("session_id", agent.SessionID).
		Msg("buildEnvVars: Appending agent.Env (USER_API_TOKEN should be present for RevDial)")

	env = append(env, agent.Env...)

	return env
}

// buildMounts builds volume mounts for the container
// workspaceDir is already a sandbox-local path (e.g., /data/workspaces/spec-tasks/spt_xxx)
// containerType is "sway", "ubuntu", or "headless"
// useHostDocker: if true, also mount the host Docker socket for privileged mode
func (h *HydraExecutor) buildMounts(agent *types.DesktopAgent, workspaceDir string, containerType string, useHostDocker bool) []hydra.MountConfig {
	// CRITICAL: Mount workspace at MULTIPLE paths for compatibility:
	// 1. Same path (/data/workspaces/...) - for Docker wrapper hacks that resolve symlinks
	// 2. /home/retro/work - so agent tools see a real directory (not a symlink)
	// 3. workspaceBasePathForContainer (/workspace) - where startup.sh expects WORKSPACE_DIR
	// This eliminates path confusion in various tools.
	mounts := []hydra.MountConfig{
		// Mount 1: Same path for Docker wrapper hacks
		{
			Source:      workspaceDir,
			Destination: workspaceDir,
			ReadOnly:    false,
		},
		// Mount 2: /home/retro/work for agent tools (ZED_WORK_DIR)
		{
			Source:      workspaceDir,
			Destination: "/home/retro/work",
			ReadOnly:    false,
		},
		// Mount 3: /workspace for WORKSPACE_DIR (startup.sh expects this)
		{
			Source:      workspaceDir,
			Destination: h.workspaceBasePathForContainer,
			ReadOnly:    false,
		},
	}

	// Docker socket mount
	// Note: Hydra may use isolated dockerd - DockerSocket field in request handles this
	// For now, mount the default socket; Hydra's CreateDevContainerRequest.DockerSocket
	// overrides this when using isolated Docker instances
	mounts = append(mounts, hydra.MountConfig{
		Source:      "/var/run/docker.sock",
		Destination: "/var/run/docker.sock",
		ReadOnly:    false,
	})

	// For Ubuntu/GNOME containers, create a per-session pipewire directory
	// and mount it to /run/user/1000 where PipeWire daemon creates its socket
	if containerType == "ubuntu" {
		pipewireDir := filepath.Join("/data/sessions", agent.SessionID, "pipewire")
		mounts = append(mounts, hydra.MountConfig{
			Source:      pipewireDir,
			Destination: "/run/user/1000",
			ReadOnly:    false,
		})
	}

	// Crash dump directory - persists core dumps from compositor crashes (Sway/GNOME)
	// Mounted from sandbox's /data/sessions/{sessionID}/crash-dumps to container's /tmp/cores
	// This allows crash analysis even after container restarts
	crashDumpDir := filepath.Join("/data/sessions", agent.SessionID, "crash-dumps")
	mounts = append(mounts, hydra.MountConfig{
		Source:      crashDumpDir,
		Destination: "/tmp/cores",
		ReadOnly:    false,
	})

	// Host Docker socket for privileged mode (Helix-in-Helix development)
	// When enabled, allows the desktop container to create sandboxes on the host Docker
	// instead of trying to run DinD-in-DinD (which fails with overlay2 storage driver)
	if useHostDocker {
		mounts = append(mounts, hydra.MountConfig{
			Source:      "/var/run/host-docker.sock",
			Destination: "/var/run/host-docker.sock",
			ReadOnly:    false,
		})
	}

	return mounts
}

// waitForDesktopBridge polls the desktop-bridge health endpoint via RevDial until it's ready.
// Desktop-bridge startup includes: D-Bus wait, Wayland socket wait, portal wait, GStreamer init.
// This can take 10-30 seconds depending on the compositor and GPU.
// Uses RevDial connection because the container IP is inside the sandbox's DinD network
// and not directly reachable from the API container.
func (h *HydraExecutor) waitForDesktopBridge(ctx context.Context, sessionID string) error {
	// RevDial runner ID follows the pattern "desktop-{sessionID}"
	runnerID := fmt.Sprintf("desktop-%s", sessionID)

	// Poll for up to 60 seconds (desktop startup can be slow)
	maxAttempts := 60
	pollInterval := 1 * time.Second

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Msg("Waiting for desktop-bridge to be ready via RevDial...")

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to connect via RevDial and check health endpoint
		if h.checkDesktopBridgeHealth(ctx, runnerID, sessionID) {
			log.Info().
				Str("session_id", sessionID).
				Int("attempts", attempt).
				Msg("Desktop-bridge is ready")
			return nil
		}

		// Log progress every 10 attempts
		if attempt%10 == 0 {
			log.Debug().
				Str("session_id", sessionID).
				Int("attempt", attempt).
				Int("max_attempts", maxAttempts).
				Msg("Still waiting for desktop-bridge...")
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("desktop-bridge not ready after %d seconds", maxAttempts)
}

// checkDesktopBridgeHealth checks if the desktop-bridge is ready via RevDial
func (h *HydraExecutor) checkDesktopBridgeHealth(ctx context.Context, runnerID, sessionID string) bool {
	if h.connman == nil {
		log.Debug().Msg("Connection manager not available for health check")
		return false
	}

	// Create a context with timeout for this single check
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Try to dial the desktop container via RevDial
	conn, err := h.connman.Dial(checkCtx, runnerID)
	if err != nil {
		// RevDial not yet available - container still starting or registering
		return false
	}
	defer conn.Close()

	// Send health check request over RevDial tunnel
	healthReq, err := http.NewRequest("GET", "http://localhost:9876/health", nil)
	if err != nil {
		return false
	}

	if err := healthReq.Write(conn); err != nil {
		return false
	}

	// Read response
	resp, err := http.ReadResponse(bufio.NewReader(conn), healthReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// DiscoverContainersFromSandbox queries a sandbox for running dev containers and
// reconciles them with the in-memory sessions map and database state.
// This is called when a sandbox connects (via heartbeat) to recover state after
// API restart or when containers were started but the API didn't record them.
func (h *HydraExecutor) DiscoverContainersFromSandbox(ctx context.Context, sandboxID string) error {
	if h.connman == nil {
		return fmt.Errorf("connection manager not available")
	}

	// Hydra runner ID follows the pattern: hydra-{SANDBOX_INSTANCE_ID}
	hydraRunnerID := "hydra-" + sandboxID

	// Create RevDial client to query Hydra
	hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)

	// Query for running containers
	containerList, err := hydraClient.ListDevContainers(ctx)
	if err != nil {
		// Don't fail on connection errors - sandbox might not be ready yet
		log.Debug().Err(err).
			Str("sandbox_id", sandboxID).
			Msg("Failed to query containers from sandbox (may not be ready)")
		return nil
	}

	if len(containerList.Containers) == 0 {
		return nil
	}

	log.Info().
		Str("sandbox_id", sandboxID).
		Int("container_count", len(containerList.Containers)).
		Msg("Discovered running containers from sandbox")

	// Collect containers that need to be added to our map
	// We do this in two phases to avoid holding the lock during DB operations
	type containerToAdd struct {
		sessionID     string
		containerID   string
		containerName string
		containerIP   string
		containerType string
	}
	var containersToAdd []containerToAdd

	// Phase 1: Check which containers we don't have tracked (short lock)
	h.mutex.RLock()
	for _, container := range containerList.Containers {
		sessionID := container.SessionID
		if _, exists := h.sessions[sessionID]; !exists {
			containerType := "ubuntu" // Default to Ubuntu
			if strings.Contains(container.ContainerName, "sway") {
				containerType = "sway"
			}
			containersToAdd = append(containersToAdd, containerToAdd{
				sessionID:     sessionID,
				containerID:   container.ContainerID,
				containerName: container.ContainerName,
				containerIP:   container.IPAddress,
				containerType: containerType,
			})
		}
	}
	h.mutex.RUnlock()

	if len(containersToAdd) == 0 {
		return nil
	}

	// Phase 2: For each container, acquire per-session lock, update DB, then update map
	for _, container := range containersToAdd {
		sessionID := container.sessionID

		// Acquire per-session creation lock to prevent race with StartDesktop
		h.creationLocksMutex.Lock()
		sessionLock, exists := h.creationLocks[sessionID]
		if !exists {
			sessionLock = &sync.Mutex{}
			h.creationLocks[sessionID] = sessionLock
		}
		h.creationLocksMutex.Unlock()

		sessionLock.Lock()

		// Double-check we still need to add this (StartDesktop may have run)
		h.mutex.RLock()
		_, alreadyTracked := h.sessions[sessionID]
		h.mutex.RUnlock()

		if alreadyTracked {
			sessionLock.Unlock()
			continue
		}

		// Check if session exists in database
		dbSession, err := h.store.GetSession(ctx, sessionID)
		if err != nil {
			log.Debug().Err(err).
				Str("session_id", sessionID).
				Msg("Session not found in database during discovery (may have been deleted)")
			// TODO: Consider stopping orphaned container here
			sessionLock.Unlock()
			continue
		}

		// Update database session metadata (outside of sessions map lock)
		if dbSession.Metadata.ContainerName != container.containerName ||
			dbSession.Metadata.ExternalAgentStatus != "running" {
			dbSession.Metadata.ContainerName = container.containerName
			dbSession.Metadata.ContainerID = container.containerID
			dbSession.Metadata.ContainerIP = container.containerIP
			dbSession.Metadata.ExternalAgentStatus = "running"
			dbSession.Metadata.ExecutorMode = "hydra"
			dbSession.SandboxID = sandboxID

			if _, err := h.store.UpdateSession(ctx, *dbSession); err != nil {
				log.Warn().Err(err).
					Str("session_id", sessionID).
					Msg("Failed to update session metadata after container discovery")
				sessionLock.Unlock()
				continue
			}
		}

		// Add to in-memory sessions map
		h.mutex.Lock()
		h.sessions[sessionID] = &ZedSession{
			SessionID:     sessionID,
			ContainerID:   container.containerID,
			ContainerName: container.containerName,
			Status:        "running",
			ContainerIP:   container.containerIP,
			LastAccess:    time.Now(),
		}
		h.mutex.Unlock()

		log.Info().
			Str("session_id", sessionID).
			Str("container_id", container.containerID).
			Str("container_name", container.containerName).
			Str("container_type", container.containerType).
			Str("sandbox_id", sandboxID).
			Msg("Recovered container from sandbox discovery")

		sessionLock.Unlock()
	}

	return nil
}
