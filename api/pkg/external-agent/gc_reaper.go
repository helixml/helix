package external_agent

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// specTaskTerminalStatuses are statuses where a spec-task is finished and its
// ephemeral on-disk resources (workspace checkout) are no longer needed. Tasks
// in these states are NOT in the live-set (so they become reap candidates once
// they age past hydra's grace period), unless they were updated very recently.
var specTaskTerminalStatuses = map[types.SpecTaskStatus]bool{
	types.TaskStatusDone:                 true,
	types.TaskStatusSpecFailed:           true,
	types.TaskStatusImplementationFailed: true,
}

// RunOrphanResourceReaper is a durable, DB-driven garbage-collection ticker. On
// each tick it computes the live-set from Postgres (sessions + spec-tasks) and
// fans it out to every recently-seen sandbox's hydra, which lists on-disk
// session zvols and per-task/session workspace dirs, subtracts the live-set,
// applies the grace period, and reaps the rest.
//
// Unlike the legacy in-memory hydra GC (DevContainerManager.GCOrphanedSessions),
// this survives host reboots / API restarts because the live-set comes from the
// database, not from hydra's in-memory container map.
//
// It blocks until ctx is cancelled and is intended to run in a goroutine.
// Errors are logged, never fatal.
func RunOrphanResourceReaper(ctx context.Context, executor Executor, st store.Store, interval, gracePeriod time.Duration, dryRun bool) {
	log.Info().
		Dur("interval", interval).
		Dur("grace_period", gracePeriod).
		Bool("dry_run", dryRun).
		Msg("starting orphan-resource reaper")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reapOrphanResources(ctx, executor, st, gracePeriod, dryRun)
		}
	}
}

func reapOrphanResources(ctx context.Context, executor Executor, st store.Store, gracePeriod time.Duration, dryRun bool) {
	// Use a cutoff generous enough that anything created/updated within the
	// grace window is still considered live, so we never race a fresh resource.
	cutoff := time.Now().Add(-gracePeriod)

	liveSessionIDs, err := st.ListExternalAgentSessionIDs(ctx, cutoff)
	if err != nil {
		log.Error().Err(err).Msg("orphan reaper: failed to list live session ids")
		return
	}

	liveSpecTaskIDs, err := liveSpecTaskIDsForReaper(ctx, st, cutoff)
	if err != nil {
		log.Error().Err(err).Msg("orphan reaper: failed to list live spec-task ids")
		return
	}

	sandboxes, err := st.ListSandboxInstances(ctx)
	if err != nil {
		log.Error().Err(err).Msg("orphan reaper: failed to list sandbox instances")
		return
	}

	req := &hydra.GCReconcileRequest{
		LiveSessionIDs:     liveSessionIDs,
		LiveSpecTaskIDs:    liveSpecTaskIDs,
		GracePeriodSeconds: int(gracePeriod.Seconds()),
		DryRun:             dryRun,
	}

	for _, sandbox := range sandboxes {
		// Only reconcile against sandboxes we've heard from recently — a stale
		// row is likely a dead host that can't be reached over RevDial anyway.
		if sandbox.LastSeen.Before(cutoff) {
			continue
		}

		// Per-sandbox timeout so a slow or hung reconcile (e.g. a wedged RevDial
		// call) can't wedge the reaper ticker indefinitely. Don't defer cancel
		// in the loop — call it before the next iteration via the closure.
		resp, err := func() (*hydra.GCReconcileResponse, error) {
			cctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
			defer cancel()
			return executor.ReconcileSandboxResources(cctx, sandbox.ID, req)
		}()
		if err != nil {
			log.Warn().Err(err).
				Str("sandbox_id", sandbox.ID).
				Msg("orphan reaper: failed to reconcile sandbox resources")
			continue
		}

		log.Info().
			Str("sandbox_id", sandbox.ID).
			Bool("dry_run", dryRun).
			Strs("zvols_reaped", resp.ZvolsReaped).
			Int("zvols_skipped", len(resp.ZvolsSkipped)).
			Strs("workspaces_reaped", resp.WorkspacesReaped).
			Int("workspaces_skipped", len(resp.WorkspacesSkipped)).
			Int64("bytes_freed", resp.BytesFreed).
			Msg("orphan reaper: sandbox reconciled")
	}
}

// liveSpecTaskIDsForReaper returns the IDs of spec-tasks that should be treated
// as live: those in a non-terminal status OR updated at/after cutoff. Archived
// tasks are included in the scan but are live only if recently updated, so a
// long-archived task's workspace eventually reaps.
func liveSpecTaskIDsForReaper(ctx context.Context, st store.Store, cutoff time.Time) ([]string, error) {
	tasks, err := st.ListSpecTasks(ctx, &types.SpecTaskFilters{IncludeArchived: true})
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, t := range tasks {
		live := !specTaskTerminalStatuses[t.Status] && !t.Archived
		if !live && !t.UpdatedAt.Before(cutoff) {
			live = true // recently touched — keep its workspace a bit longer
		}
		if live {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}
