package server

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
)

// Sandbox instance status values. The SandboxInstance type uses a plain
// string for Status (`types/types.go:3083`); these constants keep callers
// honest about the spelling. A typo like "offfline" would silently produce
// phantom-online rows forever.
const (
	sandboxInstanceStatusOnline  = "online"
	sandboxInstanceStatusOffline = "offline"
)

// startSandboxInstanceReaper periodically flips sandbox_instances rows to
// "offline" when their last_seen timestamp falls outside the stale window.
//
// The underlying RevDial heartbeat path keeps last_seen fresh while a Runner
// is alive (see sandbox_handlers.go:84 / store_sandbox.go:19). When a Runner
// dies — process crash, network drop, container stop — the heartbeat just
// stops; nothing transitions the row to "offline" on its own. The admin UI
// shows phantom-online Runners forever until this reaper sweeps them.
//
// Dispatch protection is independent: FindAvailableSandboxInstance applies
// its own 90s freshness filter (store_sandbox.go:148), so a freshly-dead
// Runner is excluded from new dispatch well before this reaper fires. The
// reaper's role is reflecting state in the DB for the admin UI and for
// reporting, not gating dispatch.
//
// Interval (default 60s) and staleThreshold (default 5min) are configurable
// via config.ServerConfig.SandboxReaperInterval and
// .SandboxStaleThreshold.
func (apiServer *HelixAPIServer) startSandboxInstanceReaper(
	ctx context.Context,
	interval time.Duration,
	staleThreshold time.Duration,
) {
	if interval == 0 {
		interval = config.DefaultSandboxReaperInterval
	}
	if staleThreshold == 0 {
		staleThreshold = config.DefaultSandboxStaleThreshold
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
// threshold, attempt to flip each still-online row to offline. The UPDATE
// is conditional on last_seen still being older than the cutoff at write
// time, so a heartbeat that lands between our SELECT and our UPDATE wins
// and the Runner stays online. Errors are logged but never returned — a
// transient DB hiccup must not kill the ticker.
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

	var flipped, raceLost int
	for _, inst := range stale {
		// Bail early on shutdown so a large stale list doesn't keep
		// the ticker goroutine alive past ctx cancel.
		if ctx.Err() != nil {
			return
		}
		if inst.Status != sandboxInstanceStatusOnline {
			// Already offline — skip without an UPDATE round trip.
			// (The query intentionally has no status predicate so
			// it remains usable for other diagnostic callers; we
			// filter here.)
			continue
		}
		rows, err := apiServer.Store.MarkSandboxInstanceOfflineIfStale(ctx, inst.ID, cutoff)
		if err != nil {
			log.Warn().
				Err(err).
				Str("sandbox_id", inst.ID).
				Msg("sandbox-instance reaper: mark-offline failed")
			continue
		}
		if rows == 0 {
			// A concurrent heartbeat refreshed the row between our
			// SELECT and UPDATE. Healthy Runner, no transition.
			raceLost++
			continue
		}
		flipped++
		log.Info().
			Str("sandbox_id", inst.ID).
			Str("last_seen", inst.LastSeen.Format(time.RFC3339)).
			Msg("sandbox-instance reaper: marked stale runner offline")
	}

	if flipped > 0 || raceLost > 0 {
		log.Info().
			Int("flipped", flipped).
			Int("race_lost_to_heartbeat", raceLost).
			Int("considered", len(stale)).
			Msg("sandbox-instance reaper: sweep complete")
	}
}
