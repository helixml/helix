package external_agent

import (
	"context"
	"fmt"
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
// This is an alternative to WolfExecutor that bypasses Wolf entirely for container lifecycle.
//
// Key differences from WolfExecutor:
// - Uses Hydra's dev container API instead of Wolf lobbies
// - No Moonlight protocol, uses WebSocket video streaming (ws_stream.go)
// - Simpler architecture: Helix API -> Hydra -> Docker -> Dev Container
type HydraExecutor struct {
	store    store.Store
	sessions map[string]*ZedSession
	mutex    sync.RWMutex

	// Configuration
	zedImage      string // e.g., "helix-sway:latest"
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

// connmanInterface is already defined in wolf_executor.go, we use the same interface
// type connmanInterface interface {
// 	Dial(ctx context.Context, deviceID string) (net.Conn, error)
// }

// HydraExecutorConfig holds configuration for creating a HydraExecutor
type HydraExecutorConfig struct {
	Store                         store.Store
	ZedImage                      string
	HelixAPIURL                   string
	HelixAPIToken                 string
	WorkspaceBasePathForContainer string
	WorkspaceBasePathForCloning   string
	Connman                       connmanInterface
	GPUVendor                     string
}

// NewHydraExecutor creates a new HydraExecutor instance
func NewHydraExecutor(cfg HydraExecutorConfig) *HydraExecutor {
	if cfg.ZedImage == "" {
		cfg.ZedImage = "helix-sway:latest"
	}

	return &HydraExecutor{
		store:                         cfg.Store,
		sessions:                      make(map[string]*ZedSession),
		zedImage:                      cfg.ZedImage,
		helixAPIURL:                   cfg.HelixAPIURL,
		helixAPIToken:                 cfg.HelixAPIToken,
		workspaceBasePathForContainer: cfg.WorkspaceBasePathForContainer,
		workspaceBasePathForCloning:   cfg.WorkspaceBasePathForCloning,
		connman:                       cfg.Connman,
		creationLocks:                 make(map[string]*sync.Mutex),
		gpuVendor:                     cfg.GPUVendor,
	}
}

// StartDesktop starts a dev container using Hydra instead of Wolf
func (h *HydraExecutor) StartDesktop(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
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
		Msg("Starting dev container via Hydra (Wolf-free)")

	// Check if session already exists and is running
	h.mutex.RLock()
	existingSession, exists := h.sessions[agent.SessionID]
	h.mutex.RUnlock()

	if exists && existingSession.Status == "running" {
		log.Info().
			Str("session_id", agent.SessionID).
			Msg("Dev container already running, returning existing session")
		return &types.ZedAgentResponse{
			SessionID:     agent.SessionID,
			ScreenshotURL: fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
			StreamURL:     fmt.Sprintf("/api/v1/sessions/%s/stream", agent.SessionID),
			Status:        "running",
		}, nil
	}

	// Get Hydra client via RevDial
	// Hydra runner ID follows pattern: hydra-{WOLF_INSTANCE_ID}
	// Hydra defaults WOLF_INSTANCE_ID to "local" (see api/cmd/hydra/main.go:112)
	sandboxID := agent.SandboxID
	if sandboxID == "" {
		// Use "local" to match Hydra's default WOLF_INSTANCE_ID
		sandboxID = "local"
	}
	hydraRunnerID := fmt.Sprintf("hydra-%s", sandboxID)
	hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)

	// Determine container type from desktop type
	containerType := h.parseContainerType(agent.DesktopType)

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

	// Build container image
	image := h.getContainerImage(containerType, agent)

	// Build environment variables
	env := h.buildEnvVars(agent, containerType, workspaceDir)

	// Build mounts
	mounts := h.buildMounts(agent, workspaceDir, containerType)

	// Create dev container request
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
		GPUVendor:     h.gpuVendor,
		UserID:        agent.UserID,
		Network:       "helix_default",
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

	// Track session
	session := &ZedSession{
		SessionID:      agent.SessionID,
		HelixSessionID: agent.HelixSessionID,
		UserID:         agent.UserID,
		Status:         "running",
		StartTime:      time.Now(),
		LastAccess:     time.Now(),
		ProjectPath:    agent.ProjectPath,
		ContainerName:  resp.ContainerName,
		ContainerID:    resp.ContainerID,
		ContainerIP:    resp.IPAddress,
		SandboxID:      sandboxID,
		// WolfLobbyID is not used in Hydra mode, but we store container info here
	}
	h.mutex.Lock()
	h.sessions[agent.SessionID] = session
	h.mutex.Unlock()

	// Update database session with container info
	if dbSession, err := h.store.GetSession(ctx, agent.SessionID); err == nil {
		dbSession.Metadata.ContainerName = resp.ContainerName
		dbSession.Metadata.ContainerID = resp.ContainerID
		dbSession.Metadata.ContainerIP = resp.IPAddress
		dbSession.Metadata.ExecutorMode = "hydra"
		if _, err := h.store.UpdateSession(ctx, *dbSession); err != nil {
			log.Warn().Err(err).Str("session_id", agent.SessionID).Msg("Failed to update session metadata with container info")
		}
	}

	return &types.ZedAgentResponse{
		SessionID:     agent.SessionID,
		ScreenshotURL: fmt.Sprintf("/api/v1/sessions/%s/screenshot", agent.SessionID),
		StreamURL:     fmt.Sprintf("/api/v1/sessions/%s/stream", agent.SessionID),
		Status:        "running",
		ContainerName: resp.ContainerName,
		ContainerIP:   resp.IPAddress,
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
	// Use WolfInstanceID as sandbox identifier for now (they're often the same or related)
	if sandboxID == "" {
		if dbSession, err := h.store.GetSessionIncludingDeleted(ctx, sessionID); err == nil {
			// Try WolfInstanceID first, which indicates which sandbox is handling this session
			sandboxID = dbSession.WolfInstanceID
		}
	}

	if sandboxID == "" {
		// Use "local" to match Hydra's default WOLF_INSTANCE_ID
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

	// Clean up creation lock
	h.creationLocksMutex.Lock()
	delete(h.creationLocks, sessionID)
	h.creationLocksMutex.Unlock()

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
func (h *HydraExecutor) StartZedInstance(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
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
		if session.HelixSessionID == helixSessionID || session.SessionID == helixSessionID {
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

// GetWolfClientForSession returns nil for Hydra executor (Wolf-free mode)
func (h *HydraExecutor) GetWolfClientForSession(wolfInstanceID string) WolfClientInterface {
	// Hydra executor doesn't use Wolf
	return nil
}

// FindExistingLobbyForSession returns error for Hydra executor (no lobbies)
func (h *HydraExecutor) FindExistingLobbyForSession(ctx context.Context, sessionID string) (string, error) {
	// Hydra executor doesn't use Wolf lobbies
	return "", fmt.Errorf("hydra executor does not use Wolf lobbies")
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

// ConfigurePendingSession is a no-op for Hydra executor
// Wolf uses this for Moonlight client attachment, but Hydra uses WebSocket streaming
func (h *HydraExecutor) ConfigurePendingSession(ctx context.Context, sessionID string, clientUniqueID string) error {
	// No-op for Hydra - we don't use Moonlight's pending session mechanism
	// WebSocket streaming connects directly to the container
	log.Debug().
		Str("session_id", sessionID).
		Str("client_unique_id", clientUniqueID).
		Msg("ConfigurePendingSession called (no-op for Hydra executor)")
	return nil
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
		return "sway" // Default to Sway
	}
}

// getContainerImage returns the appropriate container image for the given type
func (h *HydraExecutor) getContainerImage(containerType string, agent *types.ZedAgent) string {
	// Use custom image if provided
	if agent.CustomImage != "" {
		return agent.CustomImage
	}

	switch containerType {
	case "ubuntu":
		return "helix-ubuntu:latest"
	case "headless":
		return "helix-headless:latest"
	default:
		return h.zedImage // helix-sway:latest
	}
}

// buildEnvVars builds environment variables for the container
func (h *HydraExecutor) buildEnvVars(agent *types.ZedAgent, containerType, workspaceDir string) []string {
	env := []string{
		fmt.Sprintf("HELIX_API_URL=%s", h.helixAPIURL),
		fmt.Sprintf("HELIX_SESSION_ID=%s", agent.SessionID),
		fmt.Sprintf("HELIX_WORKSPACE_DIR=%s", h.workspaceBasePathForContainer),
	}

	// Add API token if available
	if h.helixAPIToken != "" {
		env = append(env, fmt.Sprintf("HELIX_API_TOKEN=%s", h.helixAPIToken))
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

	// Add display settings for non-headless containers
	if containerType != "headless" {
		if agent.DisplayWidth > 0 {
			env = append(env, fmt.Sprintf("GAMESCOPE_WIDTH=%d", agent.DisplayWidth))
		}
		if agent.DisplayHeight > 0 {
			env = append(env, fmt.Sprintf("GAMESCOPE_HEIGHT=%d", agent.DisplayHeight))
		}
		if agent.DisplayRefreshRate > 0 {
			env = append(env, fmt.Sprintf("GAMESCOPE_REFRESH=%d", agent.DisplayRefreshRate))
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

	return env
}

// buildMounts builds volume mounts for the container
// workspaceDir is already a sandbox-local path (e.g., /data/workspaces/spec-tasks/spt_xxx)
// containerType is "sway", "ubuntu", or "headless"
func (h *HydraExecutor) buildMounts(agent *types.ZedAgent, workspaceDir string, containerType string) []hydra.MountConfig {
	mounts := []hydra.MountConfig{
		// Workspace mount - workspaceDir is the sandbox-local path
		{
			Source:      workspaceDir,
			Destination: h.workspaceBasePathForContainer,
			ReadOnly:    false,
		},
	}

	// For Ubuntu/GNOME containers, create a per-session pipewire directory
	// and mount it to /run/user/1000 where PipeWire daemon creates its socket
	// This matches how Wolf handles pipewire mode (see docker.cpp:91-108)
	if containerType == "ubuntu" {
		pipewireDir := filepath.Join("/data/sessions", agent.SessionID, "pipewire")
		mounts = append(mounts, hydra.MountConfig{
			Source:      pipewireDir,
			Destination: "/run/user/1000",
			ReadOnly:    false,
		})
	}

	return mounts
}
