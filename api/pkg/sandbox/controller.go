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
//
// Code is split across files by concern: this file holds the lifecycle
// CRUD (Create/Get/List/Update/Delete + reaper). The provisioning pipeline
// lives in controller_provision.go, billing in controller_billing.go, and
// stopped-sandbox cleanup in controller_cleanup.go.
package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Controller orchestrates user-facing sandbox lifecycle on top of hydra.
type Controller struct {
	store        store.Store
	connman      *connman.ConnectionManager
	runtimes     *RuntimeRegistry
	helixAPIURL  string // base URL desktop-bridge / RevDial-from-container should dial back to
	workspaceDir string // sandbox-host path under which per-sandbox dirs live (mounts, persistence)

	// newHydraClient is the factory used by provision()/Delete() to talk to a
	// host's hydra. Defaults to hydra.NewRevDialClient(...); tests inject a
	// fake to capture CreateDevContainer requests.
	newHydraClient func(hostID string) hydraProvisionClient

	// provisionWG tracks in-flight provision goroutines launched by Create().
	// Tests use waitProvisions() to wait for them to settle.
	provisionWG sync.WaitGroup
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
	c := &Controller{
		store:        s,
		connman:      cm,
		runtimes:     runtimes,
		helixAPIURL:  helixAPIURL,
		workspaceDir: workspaceDir,
	}
	c.newHydraClient = func(hostID string) hydraProvisionClient {
		return hydra.NewRevDialClient(c.connman, fmt.Sprintf("hydra-%s", hostID))
	}
	return c
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
	// container starts up. The WaitGroup lets tests wait for the goroutine
	// to settle without sleep-polling.
	c.provisionWG.Add(1)
	go func() {
		defer c.provisionWG.Done()
		c.provision(context.Background(), created.ID)
	}()

	return created, nil
}

// waitProvisions blocks until every in-flight provision goroutine has
// returned. Test-only helper. Production callers should not rely on this —
// provision() is intentionally fire-and-forget so the API stays responsive.
func (c *Controller) waitProvisions() {
	c.provisionWG.Wait()
}

// Runtimes returns the registered runtime registry. Used by the API layer to
// expose a discovery endpoint and validate requests synchronously.
func (c *Controller) Runtimes() *RuntimeRegistry { return c.runtimes }

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
		hydraClient := c.newHydraClient(sandbox.HostDeviceID)
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
