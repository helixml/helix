// Package sandbox implements the lifecycle controller behind the Sandboxes API.
//
// A Sandbox is an ephemeral container the user creates via REST. We pick a
// hydra host with the right desktop image, ask it to launch a dev container
// in "no-agent" mode (HELIX_DISABLE_AGENT=1 skips the Zed/Qwen autoboot), and
// remember which host owns the container so subsequent commands can route
// through revdial.
//
// On delete we tear the container down and forget every cached command/log
// record on the hydra side. Nothing about the sandbox survives deletion.
package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// DefaultDisplayWidth/Height/FPS are applied when the request doesn't override.
const (
	DefaultDisplayWidth  = 1920
	DefaultDisplayHeight = 1080
	DefaultDisplayFPS    = 30
)

type resourcePreset struct {
	VCPUs    int
	MemoryMB int
}

var allowedResourcePresets = []resourcePreset{
	{VCPUs: 1, MemoryMB: 2048},
	{VCPUs: 4, MemoryMB: 8192},
	{VCPUs: 8, MemoryMB: 16384},
}

// Controller orchestrates user-facing sandbox lifecycle on top of hydra.
type Controller struct {
	store        store.Store
	connman      *connman.ConnectionManager
	runtimes     *RuntimeRegistry
	helixAPIURL  string // base URL desktop-bridge / RevDial-from-container should dial back to
	workspaceDir string // sandbox-host path under which per-sandbox dirs live (mounts, persistence)
}

// New builds a new controller. The runtime registry is required — callers
// build it from the server config via NewRuntimeRegistry. helixAPIURL is the
// base URL the in-container desktop-bridge will dial back to (used for the
// `desktop-{sandboxID}` RevDial registration that powers screenshots/streams);
// workspaceDir is the sandbox-host directory under which per-sandbox dirs are
// created (typically `/data/sandboxes`).
func New(s store.Store, cm *connman.ConnectionManager, runtimes *RuntimeRegistry, helixAPIURL, workspaceDir string) *Controller {
	if workspaceDir == "" {
		workspaceDir = "/data/sandboxes"
	}
	return &Controller{
		store:        s,
		connman:      cm,
		runtimes:     runtimes,
		helixAPIURL:  helixAPIURL,
		workspaceDir: workspaceDir,
	}
}

// Create persists a sandbox row and asynchronously schedules the container.
// The returned Sandbox is in status=pending; callers can poll Get() until
// status=running or status=failed.
func (c *Controller) Create(ctx context.Context, orgID, owner string, req *types.CreateSandboxRequest) (*types.Sandbox, error) {
	if orgID == "" {
		return nil, errors.New("organization_id is required")
	}
	if owner == "" {
		return nil, errors.New("owner is required")
	}
	if req == nil {
		req = &types.CreateSandboxRequest{}
	}

	// Resolve the runtime up front so we can reject bad requests synchronously
	// with a 400 instead of failing later in provision().
	spec, err := c.runtimes.Resolve(req)
	if err != nil {
		return nil, err
	}
	vcpus, memoryMB, err := resolveSandboxResources(req)
	if err != nil {
		return nil, err
	}
	settings, err := c.store.GetSystemSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get system settings: %w", err)
	}
	if err := c.ensureSandboxLimits(ctx, orgID, spec, settings); err != nil {
		return nil, err
	}
	if err := c.ensureSandboxCredits(ctx, orgID, spec, settings, vcpus); err != nil {
		return nil, err
	}
	// Stamp the row with the resolved runtime name and image so the UI/CLI
	// can show what's actually running, even when the caller used a custom
	// image override.
	resolvedRuntime := types.SandboxRuntime(spec.Name)
	resolvedImage := spec.Image

	envBytes, err := json.Marshal(req.Env)
	if err != nil {
		return nil, fmt.Errorf("marshal env: %w", err)
	}
	tagBytes, err := json.Marshal(req.Tags)
	if err != nil {
		return nil, fmt.Errorf("marshal tags: %w", err)
	}

	// timeout < 0 means "never expire". timeout == 0 falls back to the 1h default.
	timeout := req.TimeoutSeconds
	if timeout == 0 {
		timeout = 3600
	}
	width, height, fps := DefaultDisplayWidth, DefaultDisplayHeight, DefaultDisplayFPS
	if req.DisplayWidth > 0 {
		width = req.DisplayWidth
	}
	if req.DisplayHeight > 0 {
		height = req.DisplayHeight
	}
	if req.DisplayFPS > 0 {
		fps = req.DisplayFPS
	}

	sandbox := &types.Sandbox{
		Name:           req.Name,
		OrganizationID: orgID,
		ProjectID:      req.ProjectID,
		Owner:          owner,
		Runtime:        resolvedRuntime,
		Image:          resolvedImage,
		Status:         types.SandboxStatusPending,
		VCPUs:          vcpus,
		MemoryMB:       memoryMB,
		Persistent:     req.Persistent,
		TimeoutSeconds: timeout,
		DisplayWidth:   width,
		DisplayHeight:  height,
		DisplayFPS:     fps,
		Env:            envBytes,
		Tags:           tagBytes,
	}

	created, err := c.store.CreateSandbox(ctx, sandbox)
	if err != nil {
		return nil, fmt.Errorf("create sandbox row: %w", err)
	}

	// Provision asynchronously — don't block API caller while the desktop
	// container starts up.
	go c.provision(context.Background(), created.ID)

	return created, nil
}

func resolveSandboxResources(req *types.CreateSandboxRequest) (int, int, error) {
	vcpus := req.VCPUs
	memoryMB := req.MemoryMB
	if vcpus == 0 && memoryMB == 0 {
		return allowedResourcePresets[0].VCPUs, allowedResourcePresets[0].MemoryMB, nil
	}
	for _, preset := range allowedResourcePresets {
		if vcpus == preset.VCPUs && memoryMB == preset.MemoryMB {
			return vcpus, memoryMB, nil
		}
	}
	return 0, 0, fmt.Errorf("invalid sandbox resources: choose one of 1 CPU / 2GB RAM, 4 CPU / 8GB RAM, or 8 CPU / 16GB RAM")
}

// provision picks a hydra host and asks it to launch the container.
func (c *Controller) provision(ctx context.Context, sandboxID string) {
	provisionCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	sandbox, err := c.store.GetSandbox(provisionCtx, sandboxID)
	if err != nil {
		log.Error().Err(err).Str("sandbox_id", sandboxID).Msg("provision: failed to load sandbox row")
		return
	}

	// Resolve the runtime spec for the row. For rows created with a custom
	// image override the spec is reconstructed from the persisted Image field
	// so re-provisions / restarts still work.
	spec, err := c.specForSandbox(sandbox)
	if err != nil {
		_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, err.Error())
		return
	}

	// Pick a hydra host. Three rules apply, in order:
	//
	//  1. If the sandbox already has a HostDeviceID stamped from a previous
	//     provision, prefer that host. The persistent volume (and even
	//     ephemeral docker-data for desktop runtimes) lives on its local
	//     disk; scheduling onto a different host would silently lose all
	//     of it. We do this for non-persistent sandboxes too — sticky
	//     placement is cheap and reduces user surprise on restart.
	//
	//  2. If the previously-bound host is offline AND the sandbox is
	//     persistent, fail loudly rather than silently moving — the user's
	//     data is on that other machine. They'll need the host back, or
	//     to delete-and-recreate accepting data loss.
	//
	//  3. Only when no prior host is set do we pick a fresh one
	//     (heartbeat-matched for desktop, any-online for headless).
	host, hostErr := c.pickHostForSandbox(provisionCtx, sandbox, spec)
	if hostErr != nil {
		_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, hostErr.Error())
		return
	}

	// Final image: heartbeat-versioned for desktop, fixed for headless/custom.
	skipValidation := spec.VersionKey == ""
	image := spec.Image
	entrypoint := spec.Entrypoint
	cmd := spec.Cmd
	if spec.VersionKey != "" {
		imageName := "helix-" + spec.VersionKey
		image, err = resolveImage(host, imageName, spec.VersionKey)
		if err != nil {
			_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, err.Error())
			return
		}
	}

	envMap := map[string]string{}
	if len(sandbox.Env) > 0 {
		_ = json.Unmarshal(sandbox.Env, &envMap)
	}
	if envMap == nil {
		// JSON `null` unmarshals into a nil map; re-init so writes succeed.
		envMap = map[string]string{}
	}
	envMap["HELIX_DISABLE_AGENT"] = "1"
	envMap["HELIX_SANDBOX_ID"] = sandbox.ID
	envMap["HELIX_SESSION_ID"] = sandbox.ID
	envMap["HELIX_USER_ID"] = sandbox.Owner
	envMap["HELIX_ORGANIZATION_ID"] = sandbox.OrganizationID

	// Desktop runtimes need the same suite of env vars that spec-task
	// desktops use so the GNOME/Sway boot sequence can find its inputs and
	// reach the API for the desktop-bridge / RevDial registration. Without
	// these the gnome-shell startup falls over and the container exits.
	containerType := spec.ContainerType
	if containerType == hydra.DevContainerTypeUbuntu || containerType == hydra.DevContainerTypeSway {
		envMap["HELIX_DESKTOP_TYPE"] = string(containerType)
		envMap["GAMESCOPE_WIDTH"] = fmt.Sprintf("%d", sandbox.DisplayWidth)
		envMap["GAMESCOPE_HEIGHT"] = fmt.Sprintf("%d", sandbox.DisplayHeight)
		envMap["GAMESCOPE_REFRESH"] = fmt.Sprintf("%d", sandbox.DisplayFPS)
		envMap["GOW_REQUIRED_DEVICES"] = "/dev/dri/card*:/dev/dri/renderD*:/dev/uinput:/dev/input/event*:/dev/input/js*:/dev/input/mice"
		envMap["XDG_RUNTIME_DIR"] = "/run/user/1000"
		envMap["UMASK"] = "022"
		envMap["HELIX_API_URL"] = c.helixAPIURL
		envMap["HELIX_API_BASE_URL"] = c.helixAPIURL
		envMap["SWAY_STOP_ON_APP_EXIT"] = "no"
		// startup-app.sh hard-requires WORKSPACE_DIR to exist as a directory
		// inside the container. We point it at /home/retro/work, which is
		// where buildMounts() bind-mounts the per-sandbox workspace.
		envMap["WORKSPACE_DIR"] = "/home/retro/work"
		// NVIDIA passthrough — matches spec-task hydra executor so the GPU
		// is actually visible to the inner compositor.
		envMap["NVIDIA_VISIBLE_DEVICES"] = "all"
		envMap["NVIDIA_DRIVER_CAPABILITIES"] = "compute,utility,video,graphics,display"

		// Mint a sandbox-scoped ephemeral API key. The desktop-bridge needs
		// it (as USER_API_TOKEN) to register a RevDial connection back to
		// the API, which is how screenshots and the streaming pipeline reach
		// the in-container HTTP server. Revoked on Delete.
		token, tokenErr := c.ensureSandboxAPIToken(provisionCtx, sandbox)
		if tokenErr != nil {
			_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, fmt.Sprintf("mint sandbox api token: %s", tokenErr))
			return
		}
		for _, kv := range types.DesktopAgentAPIEnvVars(token) {
			if eq := strings.Index(kv, "="); eq > 0 {
				envMap[kv[:eq]] = kv[eq+1:]
			}
		}
	}

	envSlice := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}

	containerName := fmt.Sprintf("sbx-%s", strings.TrimPrefix(sandbox.ID, "sbx_"))

	mounts := c.buildMounts(sandbox, spec)

	createReq := &hydra.CreateDevContainerRequest{
		SessionID:           sandbox.ID,
		Image:               image,
		ContainerName:       containerName,
		Hostname:            containerName,
		Env:                 envSlice,
		Mounts:              mounts,
		ContainerType:       spec.ContainerType,
		DisplayWidth:        sandbox.DisplayWidth,
		DisplayHeight:       sandbox.DisplayHeight,
		DisplayFPS:          sandbox.DisplayFPS,
		Network:             "bridge",
		Privileged:          spec.Privileged,
		UserID:              sandbox.Owner,
		VCPUs:               sandbox.VCPUs,
		MemoryMB:            sandbox.MemoryMB,
		Entrypoint:          entrypoint,
		Cmd:                 cmd,
		SkipImageValidation: skipValidation,
	}

	hydraClient := hydra.NewRevDialClient(c.connman, fmt.Sprintf("hydra-%s", host.ID))
	resp, err := hydraClient.CreateDevContainer(provisionCtx, createReq)
	if err != nil {
		_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, fmt.Sprintf("hydra create: %s", err))
		return
	}

	if err := c.store.SetSandboxContainer(provisionCtx, sandboxID, host.ID, resp.ContainerID); err != nil {
		log.Error().Err(err).Str("sandbox_id", sandboxID).Msg("failed to persist host/container ids")
	}

	status := types.SandboxStatusRunning
	if resp.Status != hydra.DevContainerStatusRunning {
		status = types.SandboxStatusPending
	}
	_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, status, "")
}

// buildMounts assembles the host-side bind/volume mounts for a sandbox.
//
// All sandboxes get a sandbox-host workspace dir bind-mounted at
// `/home/retro/work` so user-installed binaries / scratch data have a single,
// predictable home. When the sandbox row sets Persistent=true the workspace
// dir lives under `<workspaceDir>/persist/<sandboxID>` and survives across
// restarts and helix-side reaping (we don't rm it on container delete);
// non-persistent sandboxes use `<workspaceDir>/ephem/<sandboxID>` which can
// be cleaned up by GC.
//
// Desktop runtimes additionally get the named volume + per-session helper
// dirs that the spec-task path uses, otherwise the GNOME shell init can't
// find PipeWire/dbus/dockerd state and exits before the desktop comes up.
// Headless runtimes don't need those — `tail -f /dev/null` is enough.
func (c *Controller) buildMounts(sandbox *types.Sandbox, spec *RuntimeSpec) []hydra.MountConfig {
	var mounts []hydra.MountConfig

	// Workspace mount — see func comment. We mount at /home/retro/work
	// (matches spec-task convention) so any tool that already knows that path
	// "just works".
	subdir := "ephem"
	if sandbox.Persistent {
		subdir = "persist"
	}
	workspaceHostDir := filepath.Join(c.workspaceDir, subdir, sandbox.ID)
	mounts = append(mounts, hydra.MountConfig{
		Source:      workspaceHostDir,
		Destination: "/home/retro/work",
		ReadOnly:    false,
	})

	// Desktop-only mounts — same as the spec-task hydra executor, minus the
	// Zed-specific paths since HELIX_DISABLE_AGENT=1 keeps the agent stack
	// off. Without /var/lib/docker as a volume the desktop init script
	// errors out and the container exits.
	if spec.ContainerType == hydra.DevContainerTypeUbuntu || spec.ContainerType == hydra.DevContainerTypeSway {
		mounts = append(mounts,
			hydra.MountConfig{
				Source:      fmt.Sprintf("docker-data-%s", sandbox.ID),
				Destination: "/var/lib/docker",
				Type:        "volume",
			},
			hydra.MountConfig{
				Source:      filepath.Join(c.workspaceDir, "runtime", sandbox.ID, "pipewire"),
				Destination: "/run/user/1000",
				ReadOnly:    false,
			},
			hydra.MountConfig{
				Source:      filepath.Join(c.workspaceDir, "runtime", sandbox.ID, "crash-dumps"),
				Destination: "/tmp/cores",
				ReadOnly:    false,
			},
		)
	}

	return mounts
}

// specForSandbox returns the RuntimeSpec for an existing row. For runtimes
// registered in the registry it returns the registered spec verbatim. For
// rows that were created via a custom-image override (Runtime=="custom") it
// reconstructs an ad-hoc spec from the persisted Image, since the request
// itself is no longer available.
func (c *Controller) specForSandbox(sandbox *types.Sandbox) (*RuntimeSpec, error) {
	name := string(sandbox.Runtime)
	if name == "custom" {
		if sandbox.Image == "" {
			return nil, errors.New("custom runtime row has no image recorded")
		}
		return &RuntimeSpec{
			Name:          "custom",
			Image:         sandbox.Image,
			Entrypoint:    []string{"/bin/sh", "-c"},
			Cmd:           []string{"tail -f /dev/null"},
			ContainerType: hydra.DevContainerTypeHeadless,
		}, nil
	}
	spec, err := c.runtimes.Resolve(&types.CreateSandboxRequest{Runtime: types.SandboxRuntime(name)})
	if err != nil {
		return nil, err
	}
	return spec, nil
}

// Runtimes returns the registered runtime registry. Used by the API layer to
// expose a discovery endpoint and validate requests synchronously.
func (c *Controller) Runtimes() *RuntimeRegistry { return c.runtimes }

// instanceAdvertisesVersion mirrors the version-matching logic in
// store.FindAvailableSandboxInstance — it returns true iff the host's
// heartbeat blob lists a non-empty image tag for desktopType. Used when
// re-binding a sandbox to its previously chosen host.
func instanceAdvertisesVersion(instance *types.SandboxInstance, desktopType string) bool {
	if len(instance.DesktopVersions) == 0 {
		return false
	}
	var versions map[string]string
	if err := json.Unmarshal(instance.DesktopVersions, &versions); err != nil {
		return false
	}
	v, ok := versions[desktopType]
	return ok && v != ""
}

// pickHostForSandbox is the host scheduler. See the call site in provision()
// for the placement rules. Returns a non-nil host on success, or a typed
// error describing why no host could be assigned.
//
// The single most important guarantee: when a sandbox already has a
// HostDeviceID and is persistent, this function will NEVER return a different
// host. Doing so would orphan the user's persisted workspace on the original
// host's local disk.
func (c *Controller) pickHostForSandbox(ctx context.Context, sandbox *types.Sandbox, spec *RuntimeSpec) (*types.SandboxInstance, error) {
	// Re-bind to the previously chosen host when one is recorded.
	if sandbox.HostDeviceID != "" {
		prev, err := c.store.GetSandboxInstance(ctx, sandbox.HostDeviceID)
		if err == nil && prev != nil && prev.Status == "online" {
			// Desktop runtimes also need the heartbeat-versioned image;
			// confirm the host still advertises the right version key.
			if spec.VersionKey != "" {
				if !instanceAdvertisesVersion(prev, spec.VersionKey) {
					return nil, fmt.Errorf("sticky host %s no longer advertises image %q for runtime %s; cannot safely re-bind",
						prev.ID, spec.VersionKey, spec.Name)
				}
			}
			return prev, nil
		}
		// Previous host is gone or offline. For persistent sandboxes the
		// data is on that host's disk, so silently moving would mean data
		// loss. Refuse and tell the user.
		if sandbox.Persistent {
			return nil, fmt.Errorf("sandbox is bound to host %s which is offline; refusing to move a persistent sandbox to a different host (data is on the original host)", sandbox.HostDeviceID)
		}
		// Non-persistent: fall through and pick a fresh host. We log it
		// because the user might still notice their /home/retro/work
		// scratch is empty.
		log.Warn().
			Str("sandbox_id", sandbox.ID).
			Str("previous_host", sandbox.HostDeviceID).
			Msg("previously bound host is offline; rescheduling non-persistent sandbox to a new host")
	}

	// First-time placement (or non-persistent reschedule).
	if spec.VersionKey != "" {
		host, err := c.store.FindAvailableSandboxInstance(ctx, spec.VersionKey)
		if err != nil {
			return nil, fmt.Errorf("find available host: %w", err)
		}
		if host == nil {
			return nil, fmt.Errorf("no online sandbox host advertises image for runtime %s", spec.Name)
		}
		return host, nil
	}
	hosts, err := c.store.ListSandboxInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list hosts: %w", err)
	}
	for _, h := range hosts {
		if h.Status == "online" {
			return h, nil
		}
	}
	return nil, errors.New("no available sandbox host with the requested runtime")
}

// ensureSandboxAPIToken returns an ephemeral API key scoped to the sandbox.
// We re-use the api_keys table's SessionID column as the sandbox handle so the
// existing GORM filters Just Work — sandbox IDs are globally unique and don't
// collide with real session IDs. The token grants the desktop-bridge enough
// auth to register a RevDial connection back to the API; it's revoked when
// the sandbox is deleted.
func (c *Controller) ensureSandboxAPIToken(ctx context.Context, sandbox *types.Sandbox) (string, error) {
	existing, err := c.store.GetAPIKey(ctx, &types.ApiKey{
		OrganizationID: sandbox.OrganizationID,
		Owner:          sandbox.Owner,
		OwnerType:      types.OwnerTypeUser,
		SessionID:      sandbox.ID,
	})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if existing != nil {
		return existing.Key, nil
	}

	newKey, err := system.GenerateAPIKey()
	if err != nil {
		return "", err
	}
	created, err := c.store.CreateAPIKey(ctx, &types.ApiKey{
		OrganizationID: sandbox.OrganizationID,
		Owner:          sandbox.Owner,
		OwnerType:      types.OwnerTypeUser,
		Key:            newKey,
		Name:           fmt.Sprintf("Sandbox key - %s", sandbox.ID),
		Type:           types.APIkeytypeAPI,
		SessionID:      sandbox.ID,
	})
	if err != nil {
		return "", err
	}
	return created.Key, nil
}

// revokeSandboxAPIToken deletes the ephemeral key minted in
// ensureSandboxAPIToken. Best effort — a leaked key is bounded by the sandbox
// row being gone, but we still try to remove it for hygiene.
func (c *Controller) revokeSandboxAPIToken(ctx context.Context, sandbox *types.Sandbox) {
	existing, err := c.store.GetAPIKey(ctx, &types.ApiKey{
		OrganizationID: sandbox.OrganizationID,
		Owner:          sandbox.Owner,
		OwnerType:      types.OwnerTypeUser,
		SessionID:      sandbox.ID,
	})
	if err != nil || existing == nil {
		return
	}
	if err := c.store.DeleteAPIKey(ctx, existing.Key); err != nil {
		log.Warn().Err(err).Str("sandbox_id", sandbox.ID).Msg("failed to revoke sandbox api token")
	}
}

// resolveImage looks up the image tag from the host's desktop_versions blob.
func resolveImage(host *types.SandboxInstance, imageName, versionKey string) (string, error) {
	versions := map[string]string{}
	if len(host.DesktopVersions) > 0 {
		if err := json.Unmarshal(host.DesktopVersions, &versions); err != nil {
			return "", fmt.Errorf("parse desktop_versions: %w", err)
		}
	}
	v, ok := versions[versionKey]
	if !ok || v == "" {
		return "", fmt.Errorf("host %q does not advertise %q image version", host.ID, versionKey)
	}
	return imageName + ":" + v, nil
}

// Get returns a sandbox by id. Soft-deleted rows are not returned.
func (c *Controller) Get(ctx context.Context, id string) (*types.Sandbox, error) {
	return c.store.GetSandbox(ctx, id)
}

// List returns the sandboxes for an organization, optionally narrowed to a
// single project. Empty projectID matches all sandboxes (project-scoped or
// not).
func (c *Controller) List(ctx context.Context, orgID, projectID string) ([]*types.Sandbox, error) {
	return c.store.ListSandboxes(ctx, &store.ListSandboxesQuery{
		OrganizationID: orgID,
		ProjectID:      projectID,
	})
}

// Delete tears down the underlying container (best-effort) and soft-deletes
// the row. After this call the sandbox is unreachable.
func (c *Controller) Delete(ctx context.Context, id string) error {
	sandbox, err := c.store.GetSandbox(ctx, id)
	if err != nil {
		return err
	}

	if err := c.billSandboxFinal(ctx, sandbox, time.Now()); err != nil {
		return err
	}

	_ = c.store.SetSandboxStatus(ctx, id, types.SandboxStatusStopping, "")

	if sandbox.HostDeviceID != "" {
		hydraClient := hydra.NewRevDialClient(c.connman, fmt.Sprintf("hydra-%s", sandbox.HostDeviceID))
		// Delete container — best effort, log but don't block the row deletion.
		if _, err := hydraClient.DeleteDevContainer(ctx, sandbox.ID); err != nil {
			log.Warn().Err(err).Str("sandbox_id", id).Msg("hydra DeleteDevContainer failed; continuing with row deletion")
		}
		// Forget cached command records on hydra.
		if err := hydraClient.ForgetSandboxOps(ctx, sandbox.ID); err != nil {
			log.Debug().Err(err).Str("sandbox_id", id).Msg("hydra ForgetSandboxOps failed")
		}
	}

	c.revokeSandboxAPIToken(ctx, sandbox)
	return c.store.DeleteSandbox(ctx, id)
}

// Update applies user-supplied changes (name, tags, ttl extension).
func (c *Controller) Update(ctx context.Context, id string, req *types.UpdateSandboxRequest) (*types.Sandbox, error) {
	sandbox, err := c.store.GetSandbox(ctx, id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return sandbox, nil
	}
	if req.Name != nil {
		sandbox.Name = *req.Name
	}
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds > 0 {
		sandbox.TimeoutSeconds = *req.TimeoutSeconds
		newExp := sandbox.CreatedAt.Add(time.Duration(*req.TimeoutSeconds) * time.Second)
		sandbox.ExpiresAt = &newExp
	}
	if req.Tags != nil {
		b, err := json.Marshal(*req.Tags)
		if err != nil {
			return nil, fmt.Errorf("marshal tags: %w", err)
		}
		sandbox.Tags = b
	}
	return c.store.UpdateSandbox(ctx, sandbox)
}

// HydraClient returns a RevDial client targeting the host that owns the given
// sandbox. Used by the REST handlers to forward exec/files/terminal calls.
func (c *Controller) HydraClient(sandbox *types.Sandbox) (*hydra.RevDialClient, error) {
	if sandbox.HostDeviceID == "" {
		return nil, fmt.Errorf("sandbox %s has no host assigned yet (status=%s)", sandbox.ID, sandbox.Status)
	}
	return hydra.NewRevDialClient(c.connman, fmt.Sprintf("hydra-%s", sandbox.HostDeviceID)), nil
}

// ReapExpired stops sandboxes whose TTL has elapsed. Designed to be called by
// a periodic worker.
func (c *Controller) ReapExpired(ctx context.Context) error {
	expired, err := c.store.ListExpiredSandboxes(ctx, time.Now())
	if err != nil {
		return err
	}
	for _, sb := range expired {
		log.Info().Str("sandbox_id", sb.ID).Msg("reaping expired sandbox")
		if err := c.Delete(ctx, sb.ID); err != nil {
			log.Warn().Err(err).Str("sandbox_id", sb.ID).Msg("failed to reap sandbox")
		}
	}
	return nil
}

// StartReaper runs ReapExpired on a ticker until ctx is canceled.
func (c *Controller) StartReaper(ctx context.Context, interval time.Duration) {
	if interval == 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.ReapExpired(ctx); err != nil {
				log.Warn().Err(err).Msg("sandbox reaper iteration failed")
			}
			if err := c.ReapBilling(ctx); err != nil {
				log.Warn().Err(err).Msg("sandbox billing iteration failed")
			}
			if err := c.CleanupStoppedNonPersistent(ctx); err != nil {
				log.Warn().Err(err).Msg("sandbox stopped cleanup iteration failed")
			}
		}
	}
}
