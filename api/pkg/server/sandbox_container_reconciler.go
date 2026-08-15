package server

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
)

// perSandboxReconcileTimeout bounds one sandbox's discovery round trip so a
// wedged RevDial connection can't stall the whole sweep. Discovery is a list
// call plus a bounded number of per-session probes; 30s is generous for both.
const perSandboxReconcileTimeout = 30 * time.Second

// startSandboxContainerReconciler periodically asks every online sandbox which
// dev containers are actually running and reconciles session state against the
// answer.
//
// Container discovery previously ran in exactly two places: when a sandbox
// POSTs to /sandboxes/register, and when its RevDial control connection is
// established. Both are *sandbox* lifecycle events. A dev container that dies
// while its sandbox stays happily connected — the dominant failure mode, e.g.
// helix-workspace-setup hanging on a git fetch until start-zed-core's 300s
// timeout kills the container's PID 1 — fires neither. Nothing then downgraded
// the session, so `external_agent_status` stayed "running" indefinitely: the
// task chat showed a green "Sandbox running" dot and a ticking "Working for…"
// timer against a container that had exited hours earlier, and the billing row
// stayed open.
//
// This loop closes that gap by making reconciliation time-driven rather than
// connection-driven. It reuses DiscoverContainersFromSandbox, which is already
// idempotent and correctly locked (per-session creation locks, authoritative
// per-session probes before any downgrade), so a periodic call is safe to race
// against StartDesktop/StopDesktop.
//
// Interval is configurable via HELIX_SANDBOX_CONTAINER_RECONCILE_INTERVAL.
func (apiServer *HelixAPIServer) startSandboxContainerReconciler(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = config.DefaultSandboxContainerReconcileInterval
	}
	if apiServer.externalAgentExecutor == nil {
		log.Info().Msg("sandbox-container reconciler: no external agent executor configured, not starting")
		return
	}

	log.Info().
		Dur("interval", interval).
		Msg("🔁 [SANDBOX_RECONCILE] Started periodic dev-container reconciliation")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			apiServer.reconcileSandboxContainers(ctx)
		}
	}
}

// reconcileSandboxContainers runs one sweep across every online sandbox.
//
// Offline sandboxes are skipped deliberately: their RevDial connection is gone,
// so discovery would fail for every session on them and tell us nothing new.
// Their sessions are handled by the reconnect path (DiscoverContainersFromSandbox
// on RevDial connect), which runs the same downgrade logic the moment the
// sandbox comes back.
//
// Errors are logged and never returned — a transient store or RevDial failure
// must not kill the ticker.
func (apiServer *HelixAPIServer) reconcileSandboxContainers(ctx context.Context) {
	instances, err := apiServer.Store.ListSandboxInstances(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("sandbox-container reconciler: failed to list sandbox instances")
		return
	}

	for _, instance := range instances {
		// Bail on shutdown rather than working through a long instance list.
		if ctx.Err() != nil {
			return
		}
		if instance.Status != sandboxInstanceStatusOnline {
			continue
		}

		sweepCtx, cancel := context.WithTimeout(ctx, perSandboxReconcileTimeout)
		err := apiServer.externalAgentExecutor.DiscoverContainersFromSandbox(sweepCtx, instance.ID)
		cancel()
		if err != nil {
			log.Debug().Err(err).
				Str("sandbox_id", instance.ID).
				Msg("sandbox-container reconciler: discovery failed for sandbox")
		}
	}
}
