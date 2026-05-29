package server

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// startSandboxInstanceReaper periodically flips sandbox_instances rows to
// "offline" when their last_seen timestamp falls outside the stale window.
//
// The underlying RevDial heartbeat path keeps last_seen fresh while a Runner
// is alive. When a Runner dies — process crash, network drop, container
// stop — the heartbeat just stops; nothing transitions the row to "offline"
// on its own. Without this reaper, the admin UI shows phantom-online
// Runners forever and FindAvailableSandboxInstance can hand them to the
// scheduler.
//
// Interval and staleThreshold are independent: the ticker fires every
// `interval` and marks anything older than `staleThreshold` offline. The
// scheduler's selection-filter uses its own (tighter) threshold, so this
// reaper's job is reflecting state in the DB, not gating dispatch.
func (apiServer *HelixAPIServer) startSandboxInstanceReaper(
	ctx context.Context,
	interval time.Duration,
	staleThreshold time.Duration,
) {
	if interval == 0 {
		interval = time.Minute
	}
	if staleThreshold == 0 {
		staleThreshold = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			apiServer.reapStaleSandboxInstances(ctx, staleThreshold)
		}
	}
}

// reapStaleSandboxInstances runs a single sweep: query rows older than the
// threshold, flip the ones still marked online to offline. Errors are logged
// but never returned — a transient DB hiccup must not kill the ticker.
func (apiServer *HelixAPIServer) reapStaleSandboxInstances(
	ctx context.Context,
	staleThreshold time.Duration,
) {
	cutoff := time.Now().Add(-staleThreshold)
	stale, err := apiServer.Store.GetSandboxInstancesOlderThanHeartbeat(ctx, cutoff)
	if err != nil {
		log.Warn().Err(err).Msg("sandbox-instance reaper: query failed")
		return
	}

	var flipped int
	for _, inst := range stale {
		if inst.Status == "offline" {
			continue
		}
		if err := apiServer.Store.UpdateSandboxInstanceStatus(ctx, inst.ID, "offline"); err != nil {
			log.Warn().
				Err(err).
				Str("sandbox_id", inst.ID).
				Msg("sandbox-instance reaper: mark-offline failed")
			continue
		}
		flipped++
		log.Info().
			Str("sandbox_id", inst.ID).
			Str("last_seen", inst.LastSeen.Format(time.RFC3339)).
			Msg("sandbox-instance reaper: marked stale runner offline")
	}

	if flipped > 0 {
		log.Info().
			Int("flipped", flipped).
			Int("considered", len(stale)).
			Msg("sandbox-instance reaper: sweep complete")
	}
}
