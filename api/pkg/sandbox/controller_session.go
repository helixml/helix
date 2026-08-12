package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Session-backed sandboxes.
//
// Spec-task desktops, exploratory sessions and subscription desktops are
// provisioned by the external-agent executor, not by this controller — that
// path owns repo checkout, branch mode, golden builds, ZFS clones and the Zed
// configuration, none of which belong here. But they consume exactly the same
// hydra host as a user-created sandbox, so they must be metered, quota-checked
// and visible on the same terms.
//
// The functions below give the executor a way to open, resize and close a
// billing record without handing it the provisioning pipeline. The row is the
// meter; the executor stays the provisioner. `Sandbox.SessionID` is the
// discriminator — see types.Sandbox.SessionBacked().

// DesktopStopper tears down a session-backed container. The sandbox package
// cannot import external-agent (that dependency already runs the other way),
// so the executor's StopDesktop is injected as a callback at wiring time.
type DesktopStopper func(ctx context.Context, sessionID string) error

// SetDesktopStopper wires the callback used to tear down session-backed
// containers, both when a user deletes the row from the Sandboxes UI and when
// the org runs out of credits mid-session.
func (c *Controller) SetDesktopStopper(stop DesktopStopper) {
	c.stopDesktop = stop
}

// BeginSession enforces the org's concurrency limit and credit floor, then
// upserts the sandbox row that meters this session. It returns a nil sandbox
// (and no error) when there is nothing to meter — a desktop with no owning
// organization has no wallet to bill.
//
// The returned row is in status=pending. Billing does not start until
// MarkSessionRunning flips it to running, so a desktop that fails to boot is
// never charged.
func (c *Controller) BeginSession(ctx context.Context, req *types.BeginSandboxSessionRequest) (*types.Sandbox, error) {
	if req == nil || req.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if req.OrganizationID == "" {
		// Wallets are org-scoped; a personal desktop has nothing to debit.
		return nil, nil
	}
	runtime := req.Runtime
	if runtime == "" {
		runtime = types.SandboxRuntimeUbuntuDesktop
	}
	vcpus, memoryMB := req.VCPUs, req.MemoryMB
	if vcpus <= 0 || memoryMB <= 0 {
		return nil, fmt.Errorf("sandbox resources are required for session %s (got %d vcpus / %d MB)", req.SessionID, vcpus, memoryMB)
	}

	settings, err := c.store.GetSystemSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("get system settings: %w", err)
	}

	existing, err := c.sandboxForSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	pricingType := sandboxPricingTypeForRuntime(runtime)
	// A restart reuses the session's existing row, so exclude it from the
	// concurrency count — resuming a paused task must not be blocked by the
	// slot it is itself about to reoccupy.
	excludeID := ""
	if existing != nil {
		excludeID = existing.ID
	}
	if err := c.ensureSandboxLimitsForType(ctx, req.OrganizationID, pricingType, excludeID, settings); err != nil {
		return nil, err
	}
	if err := c.ensureCreditsForType(ctx, req.OrganizationID, pricingType, settings, vcpus); err != nil {
		return nil, err
	}

	if existing != nil {
		// Reuse the row so a paused-then-resumed task keeps one identity (and
		// one charge history) across restarts.
		existing.VCPUs = vcpus
		existing.MemoryMB = memoryMB
		existing.SpecTaskID = req.SpecTaskID
		existing.ProjectID = req.ProjectID
		existing.Status = types.SandboxStatusPending
		existing.StatusMessage = ""
		// Reopen the billing window at the restart: the gap while the desktop
		// was stopped must not be charged.
		existing.BillingLastChargedAt = nil
		existing.StartedAt = nil
		existing.StoppedAt = nil
		updated, err := c.store.UpdateSandbox(ctx, existing)
		if err != nil {
			return nil, fmt.Errorf("reopen session sandbox row: %w", err)
		}
		return updated, nil
	}

	created, err := c.store.CreateSandbox(ctx, &types.Sandbox{
		Name:           req.Name,
		OrganizationID: req.OrganizationID,
		ProjectID:      req.ProjectID,
		Owner:          req.Owner,
		SessionID:      req.SessionID,
		SpecTaskID:     req.SpecTaskID,
		Runtime:        runtime,
		Status:         types.SandboxStatusPending,
		VCPUs:          vcpus,
		MemoryMB:       memoryMB,
		DisplayWidth:   req.DisplayWidth,
		DisplayHeight:  req.DisplayHeight,
		DisplayFPS:     req.DisplayFPS,
		// The task owns the desktop's lifetime, not a sandbox TTL. Negative
		// means "never expire", which leaves expires_at NULL so ReapExpired
		// skips the row.
		TimeoutSeconds: -1,
	})
	if err != nil {
		return nil, fmt.Errorf("create session sandbox row: %w", err)
	}
	return created, nil
}

// MarkSessionRunning records where the container landed and opens the billing
// window. Called once the executor has a live container.
func (c *Controller) MarkSessionRunning(ctx context.Context, sessionID, hostDeviceID, containerID string) error {
	sb, err := c.sandboxForSession(ctx, sessionID)
	if err != nil || sb == nil {
		return err
	}
	if hostDeviceID != "" || containerID != "" {
		if err := c.store.SetSandboxContainer(ctx, sb.ID, hostDeviceID, containerID); err != nil {
			return fmt.Errorf("record session sandbox container: %w", err)
		}
	}
	// SetSandboxStatus(running) stamps started_at AND billing_last_charged_at,
	// so the first billed second is the first second the container was up.
	return c.store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, "")
}

// MarkSessionStopped settles the final partial minute and closes the row.
// Safe to call for sessions that were never metered or never started.
func (c *Controller) MarkSessionStopped(ctx context.Context, sessionID string) error {
	return c.closeSession(ctx, sessionID, types.SandboxStatusStopped, "")
}

// MarkSessionFailed closes the row after a failed start. Nothing is charged
// unless the container actually reached running.
func (c *Controller) MarkSessionFailed(ctx context.Context, sessionID, reason string) error {
	return c.closeSession(ctx, sessionID, types.SandboxStatusFailed, reason)
}

func (c *Controller) closeSession(ctx context.Context, sessionID string, status types.SandboxStatus, message string) error {
	sb, err := c.sandboxForSession(ctx, sessionID)
	if err != nil || sb == nil {
		return err
	}
	if err := c.billSandboxFinal(ctx, sb, time.Now()); err != nil {
		// A failed final charge must not block teardown — the container is
		// going away either way, and leaving the row "running" would keep the
		// reaper billing for a container that no longer exists.
		log.Warn().Err(err).Str("sandbox_id", sb.ID).Str("session_id", sessionID).
			Msg("final charge for session sandbox failed; closing row anyway")
	}
	return c.store.SetSandboxStatus(ctx, sb.ID, status, message)
}

// EnsureSessionResizeCredits checks the org can afford a minute at the
// requested size before we ask hydra to resize the container.
func (c *Controller) EnsureSessionResizeCredits(ctx context.Context, sessionID string, vcpus int) error {
	sb, err := c.sandboxForSession(ctx, sessionID)
	if err != nil || sb == nil {
		return err
	}
	settings, err := c.store.GetSystemSettings(ctx)
	if err != nil {
		return fmt.Errorf("get system settings: %w", err)
	}
	return c.ensureCreditsForType(ctx, sb.OrganizationID, sandboxPricingTypeForRuntime(sb.Runtime), settings, vcpus)
}

// ResizeSession records a new allocation after the container has actually been
// resized.
//
// Order matters: billSandbox multiplies the ENTIRE unbilled window by the row's
// current VCPUs, so the outstanding window is settled at the old core count
// first. Without the flush, resizing 1→8 vCPUs would retroactively reprice up
// to a minute of already-elapsed single-core usage at eight cores.
func (c *Controller) ResizeSession(ctx context.Context, sessionID string, vcpus, memoryMB int) error {
	sb, err := c.sandboxForSession(ctx, sessionID)
	if err != nil || sb == nil {
		return err
	}
	if vcpus <= 0 || memoryMB <= 0 {
		return fmt.Errorf("invalid sandbox resources: %d vcpus / %d MB", vcpus, memoryMB)
	}
	if sb.VCPUs == vcpus && sb.MemoryMB == memoryMB {
		return nil
	}

	settings, err := c.store.GetSystemSettings(ctx)
	if err != nil {
		return fmt.Errorf("get system settings: %w", err)
	}
	if settings.SandboxBillingEnabled && sb.Status == types.SandboxStatusRunning {
		if err := c.billSandbox(ctx, settings, sb, time.Now(), true, false); err != nil {
			return fmt.Errorf("settle charges at previous size before resize: %w", err)
		}
	}
	return c.store.SetSandboxResources(ctx, sb.ID, vcpus, memoryMB)
}

// stopSessionDesktop hands teardown of a session-backed container back to the
// external-agent executor. Called from Delete once the final charge has been
// settled.
func (c *Controller) stopSessionDesktop(ctx context.Context, sb *types.Sandbox) error {
	// Nothing to tear down if the desktop never came up or is already gone.
	if sb.Status != types.SandboxStatusRunning && sb.Status != types.SandboxStatusPending {
		return nil
	}
	if c.stopDesktop == nil {
		return fmt.Errorf("cannot stop session-backed sandbox %s: no desktop stopper wired", sb.ID)
	}
	// Detach from the caller's context: a half-stopped desktop is worse than a
	// slow stop, and this path also runs from the billing reaper.
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := c.stopDesktop(stopCtx, sb.SessionID); err != nil {
		return fmt.Errorf("stop desktop for session %s: %w", sb.SessionID, err)
	}
	return nil
}

// sandboxForSession returns the live row metering a session, or (nil, nil)
// when the session has none — desktops started before this feature existed,
// and desktops with no owning org, have no row and must not break teardown.
func (c *Controller) sandboxForSession(ctx context.Context, sessionID string) (*types.Sandbox, error) {
	if sessionID == "" {
		return nil, nil
	}
	sb, err := c.store.GetSandboxBySession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("look up sandbox for session %s: %w", sessionID, err)
	}
	return sb, nil
}
