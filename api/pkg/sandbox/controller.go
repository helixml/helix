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
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// DefaultDisplayWidth/Height/FPS are applied when the request doesn't override.
const (
	DefaultDisplayWidth  = 1920
	DefaultDisplayHeight = 1080
	DefaultDisplayFPS    = 30
)

// Controller orchestrates user-facing sandbox lifecycle on top of hydra.
type Controller struct {
	store    store.Store
	connman  *connman.ConnectionManager
	runtimes *RuntimeRegistry
}

// New builds a new controller. The runtime registry is required — callers
// build it from the server config via NewRuntimeRegistry.
func New(s store.Store, cm *connman.ConnectionManager, runtimes *RuntimeRegistry) *Controller {
	return &Controller{store: s, connman: cm, runtimes: runtimes}
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
		VCPUs:          1,
		MemoryMB:       2048,
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

	// Pick a hydra host. Desktop runtimes need a host with a matching
	// versioned image advertised via the heartbeat blob. Headless runtimes
	// just need any online host that can pull the image.
	var host *types.SandboxInstance
	if spec.VersionKey != "" {
		host, err = c.store.FindAvailableSandboxInstance(provisionCtx, spec.VersionKey)
		if err != nil {
			_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, fmt.Sprintf("find available host: %s", err))
			return
		}
	} else {
		hosts, listErr := c.store.ListSandboxInstances(provisionCtx)
		if listErr != nil {
			_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, fmt.Sprintf("list hosts: %s", listErr))
			return
		}
		for _, h := range hosts {
			if h.Status == "online" {
				host = h
				break
			}
		}
	}
	if host == nil {
		_ = c.store.SetSandboxStatus(provisionCtx, sandboxID, types.SandboxStatusFailed, "no available sandbox host with the requested runtime")
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

	envSlice := make([]string, 0, len(envMap))
	for k, v := range envMap {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}

	containerName := fmt.Sprintf("sbx-%s", strings.TrimPrefix(sandbox.ID, "sbx_"))

	createReq := &hydra.CreateDevContainerRequest{
		SessionID:           sandbox.ID,
		Image:               image,
		ContainerName:       containerName,
		Hostname:            containerName,
		Env:                 envSlice,
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
		}
	}
}
