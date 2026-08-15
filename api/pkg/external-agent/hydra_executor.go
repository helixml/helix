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

type QuotaManager interface {
	LimitReached(ctx context.Context, req *types.QuotaLimitReachedRequest) (*types.QuotaLimitReachedResponse, error)
}

// ProjectSecretsGetter returns project secrets as `KEY=value` env-var strings.
// Wired to HelixAPIServer.GetProjectSecretsAsEnvVars at startup so HydraExecutor
// can inject them on every container start without each caller remembering.
type ProjectSecretsGetter func(ctx context.Context, projectID string) ([]string, error)

// agentBinaryCacheDir is the sandbox-host directory holding admin-pinned agent
// binaries. Shared by every container on the host and keyed by version, so a
// pinned release is downloaded once rather than once per session.
const agentBinaryCacheDir = "/data/agent-cache"

// HydraExecutor implements the Executor interface using Hydra for dev container management.
//
// Architecture: Helix API -> Hydra -> Docker -> Dev Container
// Video streaming: WebSocket streaming (ws_stream.go)
type HydraExecutor struct {
	store        store.Store
	quotaManager QuotaManager

	// Active Zed sessions indexed by session ID
	sessions map[string]*ZedSession
	mutex    sync.RWMutex

	// API configuration
	helixAPIURL   string
	helixAPIToken string

	// Workspace path configuration
	workspaceBasePathForContainer string // Path as seen from inside dev container
	workspaceBasePathForCloning   string // Path on sandbox filesystem (Hydra creates dirs)
	filestoreLocalPath            string // Local filestore root (e.g. /filestore)

	// RevDial connection manager for communicating with Hydra in sandbox
	connman connmanInterface

	// Per-session creation locks to prevent duplicate container creation
	creationLocks      map[string]*sync.Mutex
	creationLocksMutex sync.Mutex

	// GPU configuration
	gpuVendor string // "nvidia", "amd", "intel", ""

	// License key for nested Helix instances
	licenseKey string

	// Callback to fetch project secrets, set via SetProjectSecretsGetter
	// after HelixAPIServer is constructed (mirrors SetQuotaManager wiring).
	getProjectSecrets ProjectSecretsGetter

	// sandboxMeter opens/closes the sandbox row that bills and quota-checks
	// each desktop. Set via SetSandboxMeter after the sandbox controller is
	// constructed. Nil means desktops run unmetered.
	sandboxMeter SandboxMeter
}

// SandboxMeter is the billing/quota record behind a desktop container.
// Implemented by *sandbox.Controller; declared here so external-agent and the
// sandbox controller depend only on types, not on each other.
//
// StartDesktop is the single choke point every desktop passes through — spec
// tasks, exploratory sessions, session forks, subscription desktops and golden
// builds all land here — so metering at this level is what makes compute
// billing uniform.
type SandboxMeter interface {
	// BeginSession enforces the org's concurrency limit and credit floor and
	// opens a pending row. Returning an error aborts the desktop start.
	BeginSession(ctx context.Context, req *types.BeginSandboxSessionRequest) (*types.Sandbox, error)
	// MarkSessionRunning opens the billing window once the container is up.
	MarkSessionRunning(ctx context.Context, sessionID, hostDeviceID, containerID string) error
	// MarkSessionStopped settles the final partial minute and closes the row.
	MarkSessionStopped(ctx context.Context, sessionID string) error
	// MarkSessionFailed closes the row after a failed start.
	MarkSessionFailed(ctx context.Context, sessionID, reason string) error
	// EnsureSessionResizeCredits checks affordability before growing a container.
	EnsureSessionResizeCredits(ctx context.Context, sessionID string, vcpus int) error
	// ResizeSession settles charges at the old size, then records the new one.
	ResizeSession(ctx context.Context, sessionID string, vcpus, memoryMB int) error
}

// connmanInterface abstracts the connection manager for RevDial connections to sandboxes
type connmanInterface interface {
	Dial(ctx context.Context, deviceID string) (net.Conn, error)
}

// HydraExecutorConfig holds configuration for creating a HydraExecutor
type HydraExecutorConfig struct {
	Store                         store.Store
	QuotaManager                  QuotaManager
	HelixAPIURL                   string
	HelixAPIToken                 string
	WorkspaceBasePathForContainer string
	WorkspaceBasePathForCloning   string
	FilestoreLocalPath            string // Local filestore root for persisting paused screenshots
	Connman                       connmanInterface
	GPUVendor                     string
	LicenseKey                    string // License key to pass to nested Helix instances
}

// NewHydraExecutor creates a new HydraExecutor instance
func NewHydraExecutor(cfg HydraExecutorConfig) *HydraExecutor {
	return &HydraExecutor{
		store:                         cfg.Store,
		quotaManager:                  cfg.QuotaManager,
		sessions:                      make(map[string]*ZedSession),
		helixAPIURL:                   cfg.HelixAPIURL,
		helixAPIToken:                 cfg.HelixAPIToken,
		workspaceBasePathForContainer: cfg.WorkspaceBasePathForContainer,
		workspaceBasePathForCloning:   cfg.WorkspaceBasePathForCloning,
		filestoreLocalPath:            cfg.FilestoreLocalPath,
		connman:                       cfg.Connman,
		creationLocks:                 make(map[string]*sync.Mutex),
		gpuVendor:                     cfg.GPUVendor,
		licenseKey:                    cfg.LicenseKey,
	}
}

func (h *HydraExecutor) SetQuotaManager(quotaManager QuotaManager) {
	h.quotaManager = quotaManager
}

func (h *HydraExecutor) SetSandboxMeter(meter SandboxMeter) {
	h.sandboxMeter = meter
}

func (h *HydraExecutor) SetProjectSecretsGetter(getter ProjectSecretsGetter) {
	h.getProjectSecrets = getter
}

// getOrCreateSessionLock returns a per-session mutex, creating one if needed.
// Used by both StartDesktop and StopDesktop to serialize operations on the same
// session and prevent races (e.g. key revocation racing with container creation).
func (h *HydraExecutor) getOrCreateSessionLock(sessionID string) *sync.Mutex {
	h.creationLocksMutex.Lock()
	sessionLock, exists := h.creationLocks[sessionID]
	if !exists {
		sessionLock = &sync.Mutex{}
		h.creationLocks[sessionID] = sessionLock
	}
	h.creationLocksMutex.Unlock()
	return sessionLock
}

// StartDesktop starts a dev container using Hydra
func (h *HydraExecutor) StartDesktop(ctx context.Context, agent *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
	// Lock this specific session to prevent duplicate container creation
	// and to serialize with StopDesktop (prevents key revocation races).
	sessionLock := h.getOrCreateSessionLock(agent.SessionID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	log.Info().
		Str("organization_id", agent.OrganizationID).
		Str("session_id", agent.SessionID).
		Str("user_id", agent.UserID).
		Str("project_path", agent.ProjectPath).
		Msg("Starting dev container via Hydra")

	// Reject subscription-mode agents with no reachable subscription before
	// spending two minutes booting a desktop that can never create a thread.
	if err := h.verifySubscriptionCredentials(ctx, agent); err != nil {
		return nil, err
	}

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

	// Inject project secrets onto agent.Env so every fresh container picks up
	// the project's secrets without each call site (spec task launch, exploratory
	// session, resume, etc.) having to remember to load them.
	//
	// Ordering matters: secrets are appended BEFORE OnBeforeCreate so that the
	// API-token env vars OnBeforeCreate adds (ANTHROPIC_API_KEY, USER_API_TOKEN,
	// ...) appear later in agent.Env and therefore win duplicate-key resolution
	// in Docker. This preserves the long-standing invariant that a user-defined
	// project secret can't shadow the system-injected agent API tokens.
	if h.getProjectSecrets != nil && agent.ProjectID != "" {
		projectSecrets, err := h.getProjectSecrets(ctx, agent.ProjectID)
		if err != nil {
			log.Warn().Err(err).Str("project_id", agent.ProjectID).Str("session_id", agent.SessionID).Msg("Failed to load project secrets, continuing without them")
		} else if len(projectSecrets) > 0 {
			before := len(agent.Env)
			agent.Env = appendProjectSecrets(agent.Env, projectSecrets)
			injected := len(agent.Env) - before
			log.Info().Int("secret_count", injected).Str("project_id", agent.ProjectID).Str("session_id", agent.SessionID).Msg("Injected project secrets into desktop env")
		}
	}

	// Org-worker identity is session state, not project state: SpecTask and
	// human exploratory sessions can share the same project and must not inherit
	// it. Materialize both agent-native instruction files before the container
	// starts; the workspace bind mount persists them across desktop restarts.
	if err := h.attachSessionBootstrap(ctx, agent); err != nil {
		return nil, err
	}

	// Call OnBeforeCreate hook inside the lock to refresh API keys.
	// This prevents a race where StopDesktop revokes the key between
	// addUserAPITokenToAgent (outside the lock) and container creation (inside).
	if agent.OnBeforeCreate != nil {
		if err := agent.OnBeforeCreate(ctx, agent); err != nil {
			return nil, fmt.Errorf("pre-create hook failed: %w", err)
		}
	}

	// Resolve immutable task launch settings before quota checks and placement.
	// Resume, fork, and design-review paths rebuild DesktopAgent from session
	// metadata, so the owning task remains the source of truth.
	if err := h.resolveSpecTaskLaunchConfig(ctx, agent); err != nil {
		return nil, err
	}

	// Check legacy external-agent desktop limits for full desktops. Headless
	// tasks are enforced by the headless sandbox limit in beginSandboxMetering.
	limitReached, err := h.checkLimits(ctx, agent)
	if err != nil {
		return nil, fmt.Errorf("failed to check limits: %w", err)
	}
	if limitReached != nil && limitReached.LimitReached {
		return nil, fmt.Errorf("desktop limit reached (%d). Stop some of the existing sessions or upgrade your plan", limitReached.Limit)
	}

	// Open the sandbox row that bills and quota-checks this desktop. This
	// enforces the org's desktop concurrency cap and credit floor, so it must
	// happen before we spend minutes booting a container the org can't pay
	// for. Billing itself doesn't start until markSessionRunning below.
	meterOpened, err := h.beginSandboxMetering(ctx, agent)
	if err != nil {
		return nil, err
	}
	desktopStarted := false
	defer func() {
		if !meterOpened || desktopStarted {
			return
		}
		// Every failure path between here and the successful return must close
		// the row, otherwise a desktop that never booted keeps consuming a
		// concurrency slot forever (pending counts as active).
		if err := h.sandboxMeter.MarkSessionFailed(context.Background(), agent.SessionID, "desktop start failed"); err != nil {
			log.Warn().Err(err).Str("session_id", agent.SessionID).Msg("Failed to close sandbox billing row after failed desktop start")
		}
	}()

	// Get Hydra client via RevDial
	// Hydra runner ID follows pattern: hydra-{SANDBOX_INSTANCE_ID}
	// Determine container type first (needed for sandbox selection)
	containerType := h.parseContainerType(agent.DesktopType)

	// Determine sandbox ID - use agent's preference or find an available one
	sandboxID := agent.SandboxID
	if sandboxID == "" {
		// Headless agents use the helix-ubuntu toolchain image, so placement must
		// select a host advertising that image even though the created container
		// itself has no compositor or display devices.
		placementImage := containerType
		requiresDisplay := true
		if containerType == "headless" {
			placementImage = "ubuntu"
			// No compositor, no encoder — a CPU-only host is fine, it just
			// has to advertise the toolchain image.
			requiresDisplay = false
		}
		sandbox, err := h.store.FindAvailableSandboxInstance(ctx, placementImage, requiresDisplay)
		if err != nil {
			return nil, fmt.Errorf("failed to find available sandbox: %w", err)
		}
		if sandbox != nil {
			sandboxID = sandbox.ID
			log.Info().
				Str("sandbox_id", sandboxID).
				Str("container_type", containerType).
				Msg("Auto-selected available sandbox")
		} else {
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
			gitUserName = user.GitAuthorName()
			gitUserEmail = user.GitAuthorEmail()
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

	// Docker-in-desktop mode: the desktop container runs its own dockerd with a
	// volume-backed /var/lib/docker. No per-session sibling dockerd or network
	// bridging needed. This is simpler and supports arbitrary DinD nesting.
	//
	// The init script 17-start-dockerd.sh inside the desktop container detects
	// the volume mount and starts dockerd automatically.
	log.Info().
		Str("session_id", agent.SessionID).
		Msg("Docker-in-desktop mode: desktop will run its own dockerd")

	// Build mounts - includes Docker volume for /var/lib/docker
	mounts := h.buildMounts(agent, workspaceDir, containerType)

	// Create dev container request
	// NOTE: GPUVendor is empty - Hydra reads it from its own GPU_VENDOR env var
	// Privileged mode is required for the inner dockerd (overlay2 needs it)
	req := &hydra.CreateDevContainerRequest{
		SessionID:      agent.SessionID,
		Image:          image,
		ContainerName:  containerName,
		Hostname:       containerName,
		Env:            env,
		Mounts:         mounts,
		WorkspaceFiles: agent.WorkspaceFiles,
		DisplayWidth:   agent.DisplayWidth,
		DisplayHeight:  agent.DisplayHeight,
		DisplayFPS:     agent.DisplayRefreshRate,
		ContainerType:  hydra.DevContainerType(containerType),
		UserID:         agent.UserID,
		Network:        "bridge",
		Privileged:     true, // Required for inner dockerd (docker-in-desktop mode)
		ProjectID:      agent.ProjectID,
		GoldenBuild:    agent.GoldenBuild,
		VCPUs:          agent.VCPUs,
		MemoryMB:       agent.MemoryMB,
	}

	// Create dev container via Hydra
	log.Info().
		Str("session_id", agent.SessionID).
		Str("image", req.Image).
		Str("container_name", req.ContainerName).
		Str("container_type", string(req.ContainerType)).
		Msg("Creating dev container via Hydra")

	// Detach from the HTTP request context before any long-running work.
	// Container creation (ZFS clone + Docker pull) can take several minutes.
	// If the user navigates away the browser closes the connection, cancelling
	// the request context — we must NOT abort a ZFS clone or Docker start
	// mid-flight because of that. Use a background context with a hard upper
	// bound instead. All operations below this line use startCtx.
	startCtx, startCancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer startCancel()
	ctx = startCtx //nolint:govet // intentional shadow — request ctx no longer needed

	// Mark session as "starting" so the frontend shows the starting UI
	// (with progress messages) instead of the paused/absent state.
	h.setExternalAgentStatus(ctx, agent.SessionID, "starting")

	// Guarantee status cleanup on any error: if StartDesktop returns an error the
	// status must not remain "starting" permanently (issue #1 from ZFS deployment).
	startErr := error(nil)
	defer func() {
		if startErr != nil {
			h.setExternalAgentStatus(context.Background(), agent.SessionID, "")
			h.updateSessionStatusMessage(context.Background(), agent.SessionID, "")
		}
	}()

	// Poll golden cache copy progress in a goroutine while CreateDevContainer blocks.
	// If a golden cache exists, the Hydra side copies it before creating the container.
	// We poll a separate Hydra endpoint and update session.StatusMessage so the
	// frontend/CLI can show "Unpacking build cache (2.1/7.0 GB)" instead of just "Starting Desktop...".
	stopProgress := make(chan struct{})
	go func() {
		// Use a separate RevDial client for concurrent progress polling
		progressClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopProgress:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				progress, err := progressClient.GetGoldenCopyProgress(ctx, agent.ProjectID)
				if err != nil || progress == nil {
					continue // no copy in progress (yet or anymore)
				}
				copiedGB := float64(progress.CopiedBytes) / 1e9
				totalGB := float64(progress.TotalBytes) / 1e9
				msg := fmt.Sprintf("Unpacking build cache (%.1f/%.1f GB)", copiedGB, totalGB)
				h.updateSessionStatusMessage(ctx, agent.SessionID, msg)
			}
		}
	}()

	resp, err := hydraClient.CreateDevContainer(ctx, req)

	// Signal progress goroutine to stop and clear the status message
	close(stopProgress)
	h.updateSessionStatusMessage(ctx, agent.SessionID, "")

	if err != nil {
		startErr = fmt.Errorf("failed to create dev container via Hydra: %w", err)
		return nil, startErr
	}

	log.Info().
		Str("session_id", agent.SessionID).
		Str("container_id", resp.ContainerID).
		Str("container_name", resp.ContainerName).
		Str("ip_address", resp.IPAddress).
		Msg("Dev container created successfully via Hydra")

	// Increment moved below to the point where the session enters
	// h.sessions, so increment/decrement stay paired on the same
	// gate (membership in h.sessions). See the increment block right
	// after the h.sessions insert.

	// No bridging needed - desktop runs its own dockerd, so all containers
	// are on the same Docker network inside the desktop container.

	// Wait for the container bridge to be ready before returning. Desktop
	// runtimes initialize D-Bus, Wayland, portals, and GStreamer first; headless
	// runtimes expose only the workspace API from the same bridge binary.
	// Uses RevDial for health check since container IP is inside sandbox's DinD network.
	//
	// IMPORTANT: use an independent context here (issue #1 from ZFS deployment).
	// The caller's ctx may have been partially consumed by the ZFS clone (which can take
	// 10-90s). If we reuse it, the bridge wait budget is whatever is left over, which may
	// be far less than the 90s we need. Using context.Background() gives the full 90s.
	bridgeCtx, bridgeCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer bridgeCancel()
	if err := h.waitForDesktopBridge(bridgeCtx, agent.SessionID); err != nil {
		log.Warn().Err(err).
			Str("session_id", agent.SessionID).
			Msg("Container bridge not ready (continuing anyway, frontend may need to retry)")
		// Don't fail - the container is running, just not fully ready yet.
		// Frontend callers retry once the service becomes reachable.
	}

	// Track session
	session := &ZedSession{
		OrganizationID: agent.OrganizationID,
		ProjectID:      agent.ProjectID,
		SessionID:      agent.SessionID,
		UserID:         agent.UserID,
		Status:         "running",
		StartTime:      time.Now(),
		LastAccess:     time.Now(),
		ProjectPath:    agent.ProjectPath,
		ContainerName:  resp.ContainerName,
		ContainerID:    resp.ContainerID,
		ContainerIP:    resp.IPAddress,
		SandboxID:      sandboxID,
		GoldenBuild:    agent.GoldenBuild,
		// DevContainerID is not used in Hydra mode, but we store container info here
	}
	h.mutex.Lock()
	_, alreadyTracked := h.sessions[agent.SessionID]
	h.sessions[agent.SessionID] = session
	h.mutex.Unlock()

	// Increment active_sandboxes on the Runner row, paired with the
	// h.sessions insertion above. Gating on (!alreadyTracked) keeps
	// the increment idempotent if StartDesktop is somehow called twice
	// for the same session (counter would otherwise double-count).
	// Skipping golden builds: they aren't user-facing workload and the
	// hydra-side monitorGoldenBuild path doesn't go through StopDesktop,
	// so counting them would create a one-way drift up. Skipping the
	// "local" sentinel: no Runner row exists when SandboxID is unset.
	//
	// The matching decrement lives in StopDesktop, gated on the session
	// actually being present in h.sessions at stop time (so double-stop
	// can't over-decrement).
	if !alreadyTracked && !agent.GoldenBuild && sandboxID != "" && sandboxID != "local" {
		if incErr := h.store.IncrementSandboxContainerCount(ctx, sandboxID); incErr != nil {
			log.Warn().
				Err(incErr).
				Str("sandbox_id", sandboxID).
				Str("session_id", agent.SessionID).
				Msg("Failed to increment active_sandboxes on Runner; autoscaler may underestimate demand")
		}
	}

	// Update database session with container info and debug info
	if dbSession, err := h.store.GetSession(ctx, agent.SessionID); err == nil {
		dbSession.Metadata.ContainerName = resp.ContainerName
		dbSession.Metadata.ContainerID = resp.ContainerID
		dbSession.Metadata.ContainerIP = resp.IPAddress
		dbSession.Metadata.ExecutorMode = "hydra"
		// CRITICAL: Set DevContainerID - used by exploratory session to check if container is running
		dbSession.Metadata.DevContainerID = resp.ContainerID
		// Mark as running — StartDesktop sets "starting" earlier but never clears it on success.
		// The discovery loop only updates sessions not already in h.sessions, so without this
		// the DB stays "starting" permanently even though the container is up.
		dbSession.Metadata.ExternalAgentStatus = "running"

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

	// Container is up: record where it landed and open the billing window.
	if meterOpened {
		if err := h.sandboxMeter.MarkSessionRunning(ctx, agent.SessionID, sandboxID, resp.ContainerID); err != nil {
			// The desktop is running and usable; refusing to return it because
			// bookkeeping failed would be the worse outcome. Log loudly — an
			// unopened window means this desktop runs free until it restarts.
			log.Error().Err(err).Str("session_id", agent.SessionID).Msg("Failed to open sandbox billing window; desktop is running unmetered")
		}
	}
	desktopStarted = true

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

// resolveSpecTaskLaunchConfig applies the immutable runtime and fills a missing
// resource preset from the owning task. It is deliberately centralized here so
// every start path, including forks and reconciler resumes, behaves identically.
func (h *HydraExecutor) resolveSpecTaskLaunchConfig(ctx context.Context, agent *types.DesktopAgent) error {
	if agent.SpecTaskID == "" {
		return nil
	}
	task, err := h.store.GetSpecTask(ctx, agent.SpecTaskID)
	if err != nil {
		return fmt.Errorf("load spec task %s for sandbox configuration: %w", agent.SpecTaskID, err)
	}
	if agent.VCPUs <= 0 || agent.MemoryMB <= 0 {
		resources := types.EffectiveSpecTaskSandboxResources(task.SandboxResourceOverrides)
		agent.VCPUs = resources.VCPUs
		agent.MemoryMB = resources.MemoryMB
	}
	if types.EffectiveSpecTaskSandboxRuntime(task.SandboxRuntime) == types.SandboxRuntimeHeadlessUbuntu {
		agent.DesktopType = "headless"
	}
	return nil
}

// beginSandboxMetering opens the billing/quota row for a desktop that is about
// to start. Returns false when there is nothing to meter (no meter wired, or
// no owning organization — wallets are org-scoped).
func (h *HydraExecutor) beginSandboxMetering(ctx context.Context, agent *types.DesktopAgent) (bool, error) {
	if h.sandboxMeter == nil || agent.OrganizationID == "" {
		return false, nil
	}
	vcpus, memoryMB := desktopBillingResources(agent)
	runtime := types.SandboxRuntimeUbuntuDesktop
	if h.parseContainerType(agent.DesktopType) == "headless" {
		runtime = types.SandboxRuntimeHeadlessUbuntu
	}
	sb, err := h.sandboxMeter.BeginSession(ctx, &types.BeginSandboxSessionRequest{
		SessionID:      agent.SessionID,
		OrganizationID: agent.OrganizationID,
		Owner:          agent.UserID,
		ProjectID:      agent.ProjectID,
		SpecTaskID:     agent.SpecTaskID,
		Name:           h.desktopSandboxName(ctx, agent),
		Runtime:        runtime,
		VCPUs:          vcpus,
		MemoryMB:       memoryMB,
		DisplayWidth:   agent.DisplayWidth,
		DisplayHeight:  agent.DisplayHeight,
		DisplayFPS:     agent.DisplayRefreshRate,
	})
	if err != nil {
		return false, fmt.Errorf("cannot start desktop: %w", err)
	}
	return sb != nil, nil
}

// desktopBillingResources is what we charge for, which is not always what the
// container is capped at. Desktops started without an explicit preset run
// uncapped (hydra treats 0 as "no cap"), and charging those a single core
// would systematically undercharge the largest consumers. They are billed at
// the standard desktop preset instead — the same allocation an equivalent spec
// task gets. Capping those containers for real is a separate product change.
func desktopBillingResources(agent *types.DesktopAgent) (int, int) {
	if agent.VCPUs > 0 && agent.MemoryMB > 0 {
		return agent.VCPUs, agent.MemoryMB
	}
	standard := types.EffectiveSpecTaskSandboxResources(nil)
	return standard.VCPUs, standard.MemoryMB
}

// desktopSandboxName gives the row a label a human can recognise in the
// Sandboxes list. One indexed lookup on a path that is about to spend minutes
// booting a container.
func (h *HydraExecutor) desktopSandboxName(ctx context.Context, agent *types.DesktopAgent) string {
	if agent.SpecTaskID != "" {
		if task, err := h.store.GetSpecTask(ctx, agent.SpecTaskID); err == nil && task != nil && task.Name != "" {
			return task.Name
		}
		return agent.SpecTaskID
	}
	return fmt.Sprintf("Session %s", strings.TrimPrefix(agent.SessionID, "ses_"))
}

func (h *HydraExecutor) UpdateDesktopResources(ctx context.Context, sessionID string, resources *types.SandboxResourceOverrides) error {
	if resources == nil {
		return fmt.Errorf("sandbox resources are required")
	}
	sessionLock := h.getOrCreateSessionLock(sessionID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	sandboxID := ""
	h.mutex.RLock()
	if session := h.sessions[sessionID]; session != nil {
		sandboxID = session.SandboxID
	}
	h.mutex.RUnlock()
	if sandboxID == "" {
		dbSession, err := h.store.GetSession(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to load session: %w", err)
		}
		sandboxID = dbSession.SandboxID
	}
	if sandboxID == "" {
		sandboxID = "local"
	}

	// Refuse to grow a container the org can't pay to run at the new size.
	if h.sandboxMeter != nil {
		if err := h.sandboxMeter.EnsureSessionResizeCredits(ctx, sessionID, resources.VCPUs); err != nil {
			return fmt.Errorf("cannot resize desktop: %w", err)
		}
	}

	client := hydra.NewRevDialClient(h.connman, fmt.Sprintf("hydra-%s", sandboxID))
	_, err := client.UpdateDevContainerResources(ctx, sessionID, &hydra.UpdateDevContainerResourcesRequest{
		VCPUs:    resources.VCPUs,
		MemoryMB: resources.MemoryMB,
	})
	if err != nil {
		return fmt.Errorf("failed to update desktop resources: %w", err)
	}

	// Settle charges at the old core count before the row starts billing at
	// the new one — see sandbox.Controller.ResizeSession. Only after the
	// runtime resize actually succeeded, so a failed resize never reprices.
	if h.sandboxMeter != nil {
		if err := h.sandboxMeter.ResizeSession(ctx, sessionID, resources.VCPUs, resources.MemoryMB); err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Desktop resized but sandbox billing row not updated; charges will lag the real allocation")
		}
	}
	return nil
}

func (h *HydraExecutor) attachSessionBootstrap(ctx context.Context, agent *types.DesktopAgent) error {
	session, err := h.store.GetSession(ctx, agent.SessionID)
	if err != nil {
		return fmt.Errorf("load session bootstrap state for %s: %w", agent.SessionID, err)
	}
	return applySessionBootstrap(session.Metadata, agent)
}

func applySessionBootstrap(metadata types.SessionMetadata, agent *types.DesktopAgent) error {
	workerID := metadata.OrgWorkerID
	instructions := metadata.RuntimeInstructions
	if workerID == "" && instructions == "" {
		return nil
	}
	if workerID == "" || instructions == "" {
		return fmt.Errorf("incomplete org-worker bootstrap state for session %s", agent.SessionID)
	}
	agent.Env = append(agent.Env, "HELIX_WORKER_ID="+workerID)
	if agent.WorkspaceFiles == nil {
		agent.WorkspaceFiles = make(map[string][]byte, 2)
	}
	agent.WorkspaceFiles["AGENTS.md"] = []byte(instructions)
	agent.WorkspaceFiles["CLAUDE.md"] = []byte(instructions)
	return nil
}

func appendProjectSecrets(env, secrets []string) []string {
	for _, secret := range secrets {
		// HELIX_WORKER_ID used to be stored at project scope. Never propagate
		// that legacy value: only session bootstrap may identify a desktop as
		// an org worker.
		if strings.HasPrefix(secret, "HELIX_WORKER_ID=") {
			continue
		}
		env = append(env, secret)
	}
	return env
}

// StopDesktop stops a dev container using Hydra
func (h *HydraExecutor) StopDesktop(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required to stop desktop")
	}

	// Detach from the caller's context immediately. Stopping a container is a
	// state-machine transition that must run to completion regardless of whether
	// the triggering HTTP request is still alive (browser navigation, etc.).
	// A half-stopped container with a stale ZFS zvol is worse than a slow stop.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stopCancel()
	ctx = stopCtx //nolint:govet // intentional shadow — caller ctx not needed

	// Acquire the per-session lock to serialize with StartDesktop.
	// This prevents a race where StopDesktop revokes API keys while a concurrent
	// StartDesktop is creating a container with those same keys — leaving the
	// container with a revoked key and unable to authenticate via RevDial.
	sessionLock := h.getOrCreateSessionLock(sessionID)
	sessionLock.Lock()
	defer sessionLock.Unlock()

	log.Info().Str("session_id", sessionID).Msg("Stopping dev container via Hydra")

	h.mutex.Lock()
	session, sessionWasTracked := h.sessions[sessionID]
	var sandboxID string
	var sessionGoldenBuild bool
	if sessionWasTracked {
		sandboxID = session.SandboxID
		sessionGoldenBuild = session.GoldenBuild
		delete(h.sessions, sessionID)
	}
	h.mutex.Unlock()

	// Capture once for the decrement decision below. If the session
	// was never in h.sessions when this Stop fired (e.g. double-stop,
	// or a stop on a session that was never tracked), the matching
	// increment never happened either, so no decrement should fire.
	// Likewise for golden builds: StartDesktop deliberately skipped the
	// increment, so we must skip the decrement to keep the counter balanced.
	exists := sessionWasTracked && !sessionGoldenBuild

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

	// Capture a screenshot before tearing down the container so stopped desktops
	// can still show their last state in the Kanban card and session viewer.
	screenshotPath := h.capturePausedScreenshot(ctx, sessionID)

	// Delete dev container via Hydra
	resp, err := hydraClient.DeleteDevContainer(ctx, sessionID)
	deleteSucceeded := err == nil
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to delete dev container (may already be stopped)")
		// Don't return error - container might already be gone. We
		// also do NOT decrement here: if the delete failed but the
		// container is actually still alive on the host, decrementing
		// would create a phantom "free slot" that the dispatcher
		// would place new work onto. Operator visibility (a counter
		// stuck high) is the lesser harm. The periodic reconcile via
		// DiscoverContainersFromSandbox is the corrective path - it
		// SETs the counter to the actual container count.
	} else {
		log.Info().
			Str("session_id", sessionID).
			Str("container_id", resp.ContainerID).
			Msg("Dev container stopped successfully via Hydra")
	}

	// Decrement gated on TWO conditions:
	//   1. The session was actually in h.sessions when stop started
	//      (so the matching increment fired earlier; double-stop and
	//      stop-of-untracked-session both have exists=false here).
	//   2. The delete actually succeeded (so the container is really
	//      gone; on failure we keep the counter high to avoid phantom
	//      free slots, see comment above).
	// Skip the "local" sentinel (no Runner row to decrement).
	if exists && deleteSucceeded && sandboxID != "" && sandboxID != "local" {
		if decErr := h.store.DecrementSandboxContainerCount(ctx, sandboxID); decErr != nil {
			log.Warn().
				Err(decErr).
				Str("sandbox_id", sandboxID).
				Str("session_id", sessionID).
				Msg("Failed to decrement active_sandboxes on Runner; counter may drift high")
		}
	}

	// Settle the final partial minute and close the billing row. Done after
	// the container delete so we bill for every second it was actually up.
	if h.sandboxMeter != nil {
		if err := h.sandboxMeter.MarkSessionStopped(ctx, sessionID); err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to close sandbox billing row on desktop stop")
		}
	}

	// Revoke session-scoped ephemeral API keys
	// Keys are minted when desktop starts and should be revoked when it stops
	if err := h.revokeSessionAPIKeys(ctx, sessionID); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to revoke session API keys")
		// Don't fail the stop operation - key cleanup is best-effort
	}

	// Note: we intentionally do NOT delete the creation lock here.
	// The lock is still held (deferred unlock), and future StartDesktop calls
	// will reuse or create a new one. The map grows only by number of unique
	// sessions which is bounded.

	// Clear external_agent_status and persist the paused screenshot path together.
	if dbSession, err := h.store.GetSession(ctx, sessionID); err == nil {
		dbSession.Metadata.ExternalAgentStatus = ""
		dbSession.Metadata.PausedScreenshotPath = screenshotPath
		dbSession.Metadata.StatusMessage = ""
		if _, err := h.store.UpdateSession(ctx, *dbSession); err != nil {
			log.Debug().Err(err).Str("session_id", sessionID).Msg("Failed to update session after stop")
		}
	} else {
		// Fallback: just clear the status the old way
		h.setExternalAgentStatus(ctx, sessionID, "")
		h.updateSessionStatusMessage(ctx, sessionID, "")
	}

	return nil
}

// revokeSessionAPIKeys revokes all ephemeral API keys associated with a session.
// This is called when a desktop shuts down to clean up session-scoped keys.
func (h *HydraExecutor) revokeSessionAPIKeys(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required to revoke session keys")
	}

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

// HasRunningContainer checks if a session has a running container.
// It verifies with the actual sandbox via RevDial to detect containers that
// were stopped internally by Hydra (e.g., golden builds completing).
func (h *HydraExecutor) HasRunningContainer(ctx context.Context, sessionID string) bool {
	h.mutex.RLock()
	session, exists := h.sessions[sessionID]
	h.mutex.RUnlock()

	if !exists {
		return false
	}

	if session.ContainerID == "" {
		return false
	}

	// Verify with the actual sandbox that the container is still running.
	// The in-memory sessions map can become stale when containers are stopped
	// internally by Hydra (e.g., golden builds completing via monitorGoldenBuild).
	if h.connman != nil {
		sandboxID := session.SandboxID
		if sandboxID == "" {
			sandboxID = "local"
		}
		hydraRunnerID := fmt.Sprintf("hydra-%s", sandboxID)
		hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)

		container, err := hydraClient.GetDevContainer(ctx, sessionID)
		if err != nil {
			// Container no longer exists on sandbox — clean up stale entry
			log.Info().
				Str("session_id", sessionID).
				Str("sandbox_id", sandboxID).
				Msg("Container no longer running on sandbox, cleaning up stale session entry")
			h.mutex.Lock()
			// Re-check membership under the write lock in case a concurrent
			// StopDesktop already removed the session (and decremented).
			_, stillTracked := h.sessions[sessionID]
			delete(h.sessions, sessionID)
			h.mutex.Unlock()
			// Mirror the decrement that StopDesktop would have fired:
			// the container is gone from the Runner, so the matching
			// counter slot is gone too. Without this, sessions evicted
			// via this stale-detection path would leak the counter up
			// (autoscaler would never see them released). Same guards
			// as the StopDesktop decrement: was-tracked, not a golden
			// build, real Runner row.
			if stillTracked && !session.GoldenBuild && sandboxID != "" && sandboxID != "local" {
				if decErr := h.store.DecrementSandboxContainerCount(ctx, sandboxID); decErr != nil {
					log.Warn().Err(decErr).
						Str("sandbox_id", sandboxID).
						Str("session_id", sessionID).
						Msg("Failed to decrement active_sandboxes on Runner after stale-session eviction")
				}
			}
			return false
		}
		return container.Status == hydra.DevContainerStatusRunning
	}

	return session.Status == "running"
}

// Helper methods

// GetGoldenBuildResult queries a specific sandbox for the latest golden build result.
func (h *HydraExecutor) GetGoldenBuildResult(ctx context.Context, sandboxID, projectID string) (*hydra.GoldenBuildResult, error) {
	if h.connman == nil {
		return nil, fmt.Errorf("connection manager not available")
	}
	if sandboxID == "" {
		sandboxID = "local"
	}
	hydraRunnerID := fmt.Sprintf("hydra-%s", sandboxID)
	hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)
	return hydraClient.GetGoldenBuildResult(ctx, projectID)
}

// updateSessionStatusMessage updates the session's transient status message in the database.
// This is polled by the frontend (every 3s) and CLI (every 2s) to show startup progress
// like "Unpacking build cache (2.1/7.0 GB)" instead of a generic "Starting Desktop...".
// Pass empty string to clear the message.
func (h *HydraExecutor) updateSessionStatusMessage(ctx context.Context, sessionID, message string) {
	dbSession, err := h.store.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	if dbSession.Metadata.StatusMessage == message {
		return // no change needed
	}
	dbSession.Metadata.StatusMessage = message
	if _, err := h.store.UpdateSession(ctx, *dbSession); err != nil {
		log.Debug().Err(err).Str("session_id", sessionID).Msg("Failed to update session status message")
	}
}

// setExternalAgentStatus updates the session's external agent status in the database.
// This controls which UI state the frontend shows (starting, running, stopped, etc.).
// capturePausedScreenshot fetches a screenshot from the running desktop container
// via RevDial and saves it to disk so it can be served after the container stops.
// Returns the file path on success, empty string on any failure (best-effort).
func (h *HydraExecutor) capturePausedScreenshot(ctx context.Context, sessionID string) string {
	captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := h.connman.Dial(captureCtx, fmt.Sprintf("desktop-%s", sessionID))
	if err != nil {
		log.Debug().Err(err).Str("session_id", sessionID).Msg("Could not connect to desktop for paused screenshot")
		return ""
	}
	defer conn.Close()

	req, err := http.NewRequestWithContext(captureCtx, "GET", "http://localhost:9876/screenshot?quality=80", nil)
	if err != nil {
		return ""
	}
	if err := req.Write(conn); err != nil {
		return ""
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()

	data := make([]byte, 0, 512*1024)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	if len(data) == 0 {
		return ""
	}

	filestoreRoot := h.filestoreLocalPath
	if filestoreRoot == "" {
		filestoreRoot = "/filestore"
	}
	dir := filepath.Join(filestoreRoot, "workspaces", "paused-screenshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}
	path := filepath.Join(dir, sessionID+".jpg")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return ""
	}
	log.Info().Str("session_id", sessionID).Str("path", path).Msg("Saved paused screenshot")
	return path
}

func (h *HydraExecutor) setExternalAgentStatus(ctx context.Context, sessionID, status string) {
	dbSession, err := h.store.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	if dbSession.Metadata.ExternalAgentStatus == status {
		return
	}
	dbSession.Metadata.ExternalAgentStatus = status
	if _, err := h.store.UpdateSession(ctx, *dbSession); err != nil {
		log.Debug().Err(err).Str("session_id", sessionID).Msg("Failed to update external agent status")
	}
}

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
	case "headless":
		// Headless spec tasks still need the Helix agent toolchain baked into
		// helix-ubuntu; the container type suppresses the compositor and GPU path.
		imageName = "helix-ubuntu"
		versionKey = "ubuntu"
	default:
		imageName = "helix-sway"
		versionKey = "sway"
	}

	// Look up desktop_versions from sandbox's database record
	// The sandbox heartbeat daemon updates this with versions from /opt/images/*.version
	sandbox, err := h.store.GetSandboxInstance(ctx, sandboxID)
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

		// LLM proxy configuration for Zed's built-in agents
		// SECURITY: ANTHROPIC_API_KEY, OPENAI_API_KEY are set via agent.Env with session-scoped token
		// (see addUserAPITokenToAgent). Only set the base URLs here - NOT the runner token.
		fmt.Sprintf("ANTHROPIC_BASE_URL=%s", h.helixAPIURL),
		fmt.Sprintf("OPENAI_BASE_URL=%s/v1", h.helixAPIURL),

		// Zed sync configuration
		"ZED_EXTERNAL_SYNC_ENABLED=true",
		fmt.Sprintf("ZED_HELIX_URL=%s", zedHelixURL),
		fmt.Sprintf("ZED_HELIX_TLS=%t", zedHelixTLS),
		"ZED_HELIX_SKIP_TLS_VERIFY=true", // Enterprise internal CAs

		// Debug logging
		"RUST_LOG=info",
		"SHOW_ACP_DEBUG_LOGS=true",

		// Settings sync daemon port
		"SETTINGS_SYNC_PORT=9877",

		// ZED_WORK_DIR: Consistent cwd for ACP session storage
		"ZED_WORK_DIR=/home/retro/work",

		// Keep desktop alive when Zed restarts
		"SWAY_STOP_ON_APP_EXIT=no",
	}

	// Pass license key to nested Helix instances (Helix-in-Helix development)
	// This hides the "Get your free Community License Key" banner
	if h.licenseKey != "" {
		env = append(env, fmt.Sprintf("LICENSE_KEY=%s", h.licenseKey))
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
		env = setContainerEnv(env, "RUST_LOG", "info,gst_wayland_display=debug")
		env = append(env,
			"GOW_REQUIRED_DEVICES=/dev/dri/card*:/dev/dri/renderD*:/dev/uinput:/dev/input/event*:/dev/input/js*:/dev/input/mice",
			"GST_DEBUG=vsockenc:5",
			"ZED_ALLOW_EMULATED_GPU=1",
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

		// Add GPU-specific environment variables.
		switch h.gpuVendor {
		case "nvidia":
			env = append(env, "NVIDIA_VISIBLE_DEVICES=all")
			// Use explicit capabilities instead of "all" for GKE/cloud compatibility
			env = append(env, "NVIDIA_DRIVER_CAPABILITIES=compute,utility,video,graphics,display")
		case "amd":
			env = append(env, "GOW_REQUIRED_DEVICES=/dev/dri/card*:/dev/dri/renderD*")
		case "intel":
			env = append(env, "GOW_REQUIRED_DEVICES=/dev/dri/card*:/dev/dri/renderD*")
		}
	}

	// NOTE: BUILDKIT_HOST env var is injected by Hydra server side (devcontainer.go buildEnv)
	// which runs inside the sandbox where it can query the helix-buildkit container IP.

	// Forward desktop-bridge tunables from the controlplane env into dev
	// containers. The desktop-bridge binary reads these inside the container,
	// so without explicit forwarding here an operator setting them on
	// `controlplane.extraEnv` would have no effect.
	//
	//   HELIX_ENCODER     - H.264 encoder (nvenc | vaapi | openh264 | x264 | ...)
	//   HELIX_VIDEO_MODE  - PipeWire capture path (zerocopy | native | shm).
	//                       Skip "scanout" - devcontainer.go sets that
	//                       explicitly for the macOS QEMU virtio-gpu path.
	//   HELIX_GOP_SIZE    - GOP size in frames (default 120 = 2s at 60fps)
	//   HELIX_RENDER_NODE - VA-API render device (e.g. /dev/dri/renderD129)
	if containerType != "headless" {
		for _, name := range []string{"HELIX_ENCODER", "HELIX_VIDEO_MODE", "HELIX_GOP_SIZE", "HELIX_RENDER_NODE"} {
			val := os.Getenv(name)
			if val == "" {
				continue
			}
			if name == "HELIX_VIDEO_MODE" && val == "scanout" {
				continue
			}
			env = append(env, fmt.Sprintf("%s=%s", name, val))
		}
	}

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
	if containerType == "headless" {
		env = setContainerEnv(env, "HELIX_HEADLESS", "1")
	}

	return env
}

func setContainerEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// buildMounts builds volume mounts for the container.
// In docker-in-desktop mode, we mount a Docker named volume for /var/lib/docker
// instead of mounting a docker.sock from a sibling dockerd.
// workspaceDir is already a sandbox-local path (e.g., /data/workspaces/spec-tasks/spt_xxx)
// containerType is "sway", "ubuntu", or "headless"
func (h *HydraExecutor) buildMounts(agent *types.DesktopAgent, workspaceDir string, containerType string) []hydra.MountConfig {
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

	// Docker-in-desktop: mount a named volume for the inner dockerd's data.
	// The desktop's 17-start-dockerd.sh init script detects this mountpoint
	// and starts dockerd automatically. No docker.sock mount needed.
	mounts = append(mounts, hydra.MountConfig{
		Source:      fmt.Sprintf("docker-data-%s", agent.SessionID),
		Destination: "/var/lib/docker",
		Type:        "volume", // Docker named volume, backed by host ext4
	})

	// Shared cache for admin-pinned agent binaries (currently opencode).
	// Host-level, not per-session: the opencode release archive is ~60MB, so
	// without this every container on the host would re-download the same
	// pinned version. Entries are version-keyed and written atomically, which
	// makes concurrent readers and writers safe.
	mounts = append(mounts, hydra.MountConfig{
		Source:      agentBinaryCacheDir,
		Destination: "/opt/helix/agent-cache",
		ReadOnly:    false,
	})

	// NOTE: Shared BuildKit cache mount (/buildkit-cache) and BUILDKIT_HOST env var
	// are injected by the Hydra server side (devcontainer.go buildMounts/buildEnv)
	// which runs inside the sandbox where it can access the correct paths.

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

	return mounts
}

// waitForDesktopBridge polls the bridge health endpoint via RevDial until it's ready.
// Desktop startup includes D-Bus, Wayland, portal, and GStreamer initialization;
// headless startup serves the workspace APIs without those dependencies.
// Uses RevDial connection because the container IP is inside the sandbox's DinD network
// and not directly reachable from the API container.
func (h *HydraExecutor) waitForDesktopBridge(ctx context.Context, sessionID string) error {
	// RevDial runner ID follows the pattern "desktop-{sessionID}"
	runnerID := fmt.Sprintf("desktop-%s", sessionID)

	// Poll for up to 120 seconds (desktop startup can be slow, especially
	// when multiple desktops boot in parallel and contend for GPU/CPU/disk)
	maxAttempts := 120
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

	// Resync active_sandboxes from ground truth FIRST, before any
	// early-return on the empty-list case. hydra is the source of truth:
	// if it reports 0 containers, the DB MUST become 0 too (otherwise a
	// Runner that previously drifted up stays drifted forever after
	// becoming empty - the exact failure mode this resync was added to
	// prevent). SET (not Increment) writes the exact count, recovering
	// from any drift accumulated by missed Increment/Decrement calls
	// (API restart, hydra-side internal deletes, failed StopDesktop).
	if sandboxID != "" && sandboxID != "local" {
		if setErr := h.store.SetSandboxContainerCount(ctx, sandboxID, len(containerList.Containers)); setErr != nil {
			log.Warn().
				Err(setErr).
				Str("sandbox_id", sandboxID).
				Int("container_count", len(containerList.Containers)).
				Msg("Failed to set active_sandboxes from discovery; counter may be stale")
		}
	}

	runningContainers := filterRunningContainers(containerList.Containers)
	if exited := len(containerList.Containers) - len(runningContainers); exited > 0 {
		log.Info().
			Str("sandbox_id", sandboxID).
			Int("exited_containers", exited).
			Msg("Sandbox reports containers that are no longer running; excluding them from the live set")
	}

	// Reconcile the inverse of discovery: any session this control-plane still
	// believes is "running" on this sandbox but that hydra no longer reports as
	// running is stale (e.g. a CD redeploy destroyed its dev containers, or the
	// container's entrypoint exited on its own). Mark those sessions stopped so
	// the Kanban / task page stop showing a dead container as running. This MUST
	// run even when hydra reports zero running containers (the full-wipe case),
	// so it sits before the empty-list early return below.
	liveSessionIDs := make(map[string]bool, len(runningContainers))
	for _, container := range runningContainers {
		if container.SessionID != "" {
			liveSessionIDs[container.SessionID] = true
		}
	}
	h.markMissingSessionsStopped(ctx, sandboxID, liveSessionIDs, hydraClient)

	if len(runningContainers) == 0 {
		return nil
	}

	log.Info().
		Str("sandbox_id", sandboxID).
		Int("container_count", len(runningContainers)).
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
	for _, container := range runningContainers {
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

		// Add to in-memory sessions map. SandboxID populated so a
		// later StopDesktop for this session can resolve the right
		// Runner for the decrement (previously left blank, which
		// meant the decrement fell back to "local" and skipped).
		h.mutex.Lock()
		h.sessions[sessionID] = &ZedSession{
			OrganizationID: dbSession.OrganizationID,
			ProjectID:      dbSession.ProjectID,
			SessionID:      sessionID,
			ContainerID:    container.containerID,
			ContainerName:  container.containerName,
			Status:         "running",
			ContainerIP:    container.containerIP,
			SandboxID:      sandboxID,
			LastAccess:     time.Now(),
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

// filterRunningContainers keeps only the containers Docker currently reports as
// running.
//
// hydra tracks a container until it is explicitly removed, so its list includes
// containers that have exited (workspace-setup FATAL, OOM kill, crashed
// compositor). Everything downstream of discovery — the "did this session die"
// reconcile, the recovery of untracked containers into the in-memory map, the
// `external_agent_status = "running"` DB write — keys off this set. Treating
// "hydra still tracks it" as "alive" is what left dead sessions showing a green
// "Sandbox running" dot indefinitely, and what let a reconnect resurrect a
// session whose container had already exited.
func filterRunningContainers(containers []hydra.DevContainerResponse) []hydra.DevContainerResponse {
	running := make([]hydra.DevContainerResponse, 0, len(containers))
	for _, container := range containers {
		if container.Status == hydra.DevContainerStatusRunning {
			running = append(running, container)
		}
	}
	return running
}

// devContainerProber is the slice of the hydra client that
// markMissingSessionsStopped needs: an authoritative, per-session liveness
// check. Narrowing it to an interface lets the reconcile tests exercise the
// "hydra still tracks the container but Docker says it exited" case, which is
// exactly the state a crashed sandbox leaves behind.
type devContainerProber interface {
	GetDevContainer(ctx context.Context, sessionID string) (*hydra.DevContainerResponse, error)
}

// markMissingSessionsStopped downgrades sessions on the given sandbox that this
// control-plane still marks "running" but that are absent from hydra's live set
// (liveSessionIDs — containers Docker currently reports as running). For each
// candidate it authoritatively confirms the container is not running with a
// per-session GetDevContainer probe — taken under the session's creation lock so
// a concurrent StartDesktop that (re)created the container after hydra's snapshot
// can't be wrongly torn down. Confirmed-dead sessions have their container
// metadata cleared, status set to "stopped", and their stale in-memory entry
// evicted, so the derived SandboxState becomes "absent" instead of showing a dead
// container as running.
func (h *HydraExecutor) markMissingSessionsStopped(ctx context.Context, sandboxID string, liveSessionIDs map[string]bool, prober devContainerProber) {
	// Unlike the active_sandboxes counter (a multi-tenant autoscaler concern that
	// skips "local"), session-status reconciliation applies to every sandbox
	// including the single-node "local" one — a self-hosted CD redeploy destroys
	// its containers just the same. Only skip the ambiguous empty id.
	if sandboxID == "" {
		return
	}

	sessions, err := h.store.ListSessionsBySandbox(ctx, sandboxID)
	if err != nil {
		log.Warn().Err(err).Str("sandbox_id", sandboxID).Msg("Failed to list sessions for stale-container reconcile")
		return
	}

	for _, session := range sessions {
		if liveSessionIDs[session.ID] {
			continue // hydra confirms this container is alive
		}
		// Only downgrade sessions we believe are actively running with a
		// container. Skip "starting" (StartDesktop may be mid-flight and not yet
		// visible to hydra) and already-terminal states.
		if session.Metadata.ExternalAgentStatus != "running" || session.Metadata.ContainerName == "" {
			continue
		}

		// Serialize against StartDesktop for this session before making a
		// decision, so the probe + downgrade is atomic wrt a concurrent start.
		h.creationLocksMutex.Lock()
		sessionLock, exists := h.creationLocks[session.ID]
		if !exists {
			sessionLock = &sync.Mutex{}
			h.creationLocks[session.ID] = sessionLock
		}
		h.creationLocksMutex.Unlock()

		sessionLock.Lock()

		// Re-read under the lock: StartDesktop may have just (re)created it.
		current, err := h.store.GetSession(ctx, session.ID)
		if err != nil {
			sessionLock.Unlock()
			continue
		}
		if current.Metadata.ExternalAgentStatus != "running" || current.Metadata.ContainerName == "" {
			sessionLock.Unlock()
			continue
		}

		// Authoritatively confirm the container is not running before
		// downgrading. hydra's list snapshot may pre-date a just-started
		// container, so a direct live probe is the source of truth for the
		// decision.
		//
		// The probe must check the reported status, not merely that the call
		// succeeded: GetDevContainer returns a container hydra still tracks even
		// when Docker reports it exited, so an `err == nil` check here treated a
		// dead container as alive and skipped the downgrade forever.
		if prober != nil {
			if container, err := prober.GetDevContainer(ctx, session.ID); err == nil &&
				container != nil && container.Status == hydra.DevContainerStatusRunning {
				sessionLock.Unlock()
				continue // container really is running; snapshot was just stale
			}
		}

		current.Metadata.ContainerName = ""
		current.Metadata.ContainerID = ""
		current.Metadata.ContainerIP = ""
		current.Metadata.ExternalAgentStatus = "stopped"
		// Keep DesiredState + SandboxID so a reconciler can restart it here.

		if _, err := h.store.UpdateSession(ctx, *current); err != nil {
			log.Warn().Err(err).
				Str("session_id", session.ID).
				Str("sandbox_id", sandboxID).
				Msg("Failed to mark stale dev container session stopped during discovery")
			sessionLock.Unlock()
			continue
		}

		// Evict the stale in-memory entry so GetSession/HasRunningContainer agree.
		h.mutex.Lock()
		delete(h.sessions, session.ID)
		h.mutex.Unlock()

		// Close the billing row too. Without this, a container destroyed
		// outside StopDesktop (host redeploy, OOM kill, manual docker rm)
		// leaves a row in `running` and the reaper keeps charging the org
		// every minute for a container that no longer exists.
		if h.sandboxMeter != nil {
			if err := h.sandboxMeter.MarkSessionStopped(ctx, session.ID); err != nil {
				log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to close sandbox billing row for stale container")
			}
		}

		log.Info().
			Str("session_id", session.ID).
			Str("sandbox_id", sandboxID).
			Msg("Marked stale dev container session stopped (not reported by hydra after reconnect)")

		sessionLock.Unlock()
	}
}

// ReconcileSandboxResources posts the DB-derived live-set to a connected
// sandbox's hydra over RevDial and returns hydra's report of reaped / skipped
// orphan resources. Mirrors the DiscoverContainersFromSandbox RevDial wiring.
func (h *HydraExecutor) ReconcileSandboxResources(ctx context.Context, sandboxID string, req *hydra.GCReconcileRequest) (*hydra.GCReconcileResponse, error) {
	if h.connman == nil {
		return nil, fmt.Errorf("connection manager not available")
	}

	// Hydra runner ID follows the pattern: hydra-{SANDBOX_INSTANCE_ID}
	hydraRunnerID := "hydra-" + sandboxID

	hydraClient := hydra.NewRevDialClient(h.connman, hydraRunnerID)

	return hydraClient.ReconcileGC(ctx, req)
}

// checkLimits checks desktop limits for the user/org
func (h *HydraExecutor) checkLimits(ctx context.Context, agent *types.DesktopAgent) (*types.QuotaLimitReachedResponse, error) {
	if h.parseContainerType(agent.DesktopType) == "headless" {
		return &types.QuotaLimitReachedResponse{LimitReached: false}, nil
	}
	// Get system settings
	systemSettings, err := h.store.GetSystemSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get system settings: %w", err)
	}

	// Check if limits are disabled
	if !systemSettings.EnforceQuotas {
		return &types.QuotaLimitReachedResponse{LimitReached: false}, nil
	}

	limitReached, err := h.quotaManager.LimitReached(ctx, &types.QuotaLimitReachedRequest{
		UserID:         agent.UserID,
		OrganizationID: agent.OrganizationID,
		Resource:       types.ResourceDesktop,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to check limits: %w", err)
	}

	return limitReached, nil
}

// OnSandboxDisconnected is called when a sandbox's grace period expires (definitively disconnected).
// It clears stale container metadata from sessions on that sandbox so the frontend shows
// "stopped" state instead of an endless connection loop.
func (h *HydraExecutor) OnSandboxDisconnected(sandboxID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Info().Str("sandbox_id", sandboxID).Msg("Sandbox disconnected, clearing session metadata")

	// Get all sessions associated with this sandbox
	sessions, err := h.store.ListSessionsBySandbox(ctx, sandboxID)
	if err != nil {
		log.Error().Err(err).Str("sandbox_id", sandboxID).Msg("Failed to list sessions for sandbox")
		return
	}

	if len(sessions) == 0 {
		log.Debug().Str("sandbox_id", sandboxID).Msg("No sessions found for disconnected sandbox")
		return
	}

	log.Info().Str("sandbox_id", sandboxID).Int("session_count", len(sessions)).Msg("Clearing metadata for sessions on disconnected sandbox")

	// Clear container metadata for each session
	for _, session := range sessions {
		// Only clear if the session had container metadata
		if session.Metadata.ContainerName == "" && session.Metadata.ContainerID == "" {
			continue
		}

		// Clear stale container metadata
		session.Metadata.ContainerName = ""
		session.Metadata.ContainerID = ""
		session.Metadata.ContainerIP = ""
		session.Metadata.ExternalAgentStatus = "stopped"
		// Keep DesiredState unchanged so reconciler can restart when sandbox returns
		session.SandboxID = "" // Clear sandbox association

		if _, err := h.store.UpdateSession(ctx, *session); err != nil {
			log.Warn().Err(err).
				Str("session_id", session.ID).
				Str("sandbox_id", sandboxID).
				Msg("Failed to clear session metadata after sandbox disconnect")
		} else {
			log.Debug().
				Str("session_id", session.ID).
				Str("sandbox_id", sandboxID).
				Msg("Cleared session metadata after sandbox disconnect")
		}
	}

	// Also clear in-memory sessions map
	h.clearSessionsBySandbox(sandboxID)
}

// clearSessionsBySandbox removes all in-memory session entries for a given sandbox
func (h *HydraExecutor) clearSessionsBySandbox(sandboxID string) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	var cleared []string
	for sessionID, session := range h.sessions {
		// Check if this session was on the disconnected sandbox
		// We need to check the DB session's SandboxID, but since we already updated the DB,
		// we check if the session doesn't have a valid container anymore
		if session.ContainerID == "" || session.Status != "running" {
			continue
		}

		// Remove from in-memory map - the container no longer exists
		delete(h.sessions, sessionID)
		cleared = append(cleared, sessionID)
	}

	if len(cleared) > 0 {
		log.Info().
			Str("sandbox_id", sandboxID).
			Int("cleared_count", len(cleared)).
			Strs("session_ids", cleared).
			Msg("Cleared in-memory sessions for disconnected sandbox")
	}
}
