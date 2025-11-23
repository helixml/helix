package services

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"
)

const (
	// HealthCheckInterval is how often the health monitor runs
	HealthCheckInterval = 60 * time.Second

	// HeartbeatTimeout is the maximum age of a heartbeat before marking Wolf as offline
	HeartbeatTimeout = 2 * time.Minute
)

// WolfHealthMonitor monitors Wolf instance health and marks stale instances as offline
type WolfHealthMonitor struct {
	store     store.Store
	scheduler *WolfScheduler
}

// NewWolfHealthMonitor creates a new Wolf health monitor
func NewWolfHealthMonitor(store store.Store, scheduler *WolfScheduler) *WolfHealthMonitor {
	return &WolfHealthMonitor{
		store:     store,
		scheduler: scheduler,
	}
}

// Start starts the health monitor background goroutine
func (m *WolfHealthMonitor) Start(ctx context.Context) {
	log.Info().Msg("Starting Wolf health monitor")

	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()

	// Run once immediately on startup
	m.runHealthCheck(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping Wolf health monitor")
			return
		case <-ticker.C:
			m.runHealthCheck(ctx)
		}
	}
}

// runHealthCheck performs a single health check pass
func (m *WolfHealthMonitor) runHealthCheck(ctx context.Context) {
	cutoffTime := time.Now().Add(-HeartbeatTimeout)

	// Get Wolf instances with stale heartbeats
	staleInstances, err := m.store.GetWolfInstancesOlderThanHeartbeat(ctx, cutoffTime)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get stale Wolf instances")
		return
	}

	if len(staleInstances) == 0 {
		log.Debug().Msg("Wolf health check: all instances healthy")
		return
	}

	log.Warn().
		Int("count", len(staleInstances)).
		Dur("timeout", HeartbeatTimeout).
		Msg("Found Wolf instances with stale heartbeats")

	// Mark each stale instance as offline
	for _, instance := range staleInstances {
		log.Warn().
			Str("wolf_id", instance.ID).
			Str("wolf_name", instance.Name).
			Time("last_heartbeat", instance.LastHeartbeat).
			Str("previous_status", instance.Status).
			Msg("Marking Wolf instance as offline due to stale heartbeat")

		err := m.scheduler.MarkWolfOffline(ctx, instance.ID)
		if err != nil {
			log.Error().
				Err(err).
				Str("wolf_id", instance.ID).
				Msg("Failed to mark Wolf instance as offline")
			continue
		}

		log.Info().
			Str("wolf_id", instance.ID).
			Str("wolf_name", instance.Name).
			Msg("Successfully marked Wolf instance as offline")
	}
}

// MarkWolfDegradedOnError marks a Wolf as degraded if sandbox operations fail
// This should be called by external code when sandbox creation fails
func (m *WolfHealthMonitor) MarkWolfDegradedOnError(ctx context.Context, wolfID string, err error) {
	log.Error().
		Err(err).
		Str("wolf_id", wolfID).
		Msg("Sandbox operation failed, marking Wolf as degraded")

	markErr := m.scheduler.MarkWolfDegraded(ctx, wolfID)
	if markErr != nil {
		log.Error().
			Err(markErr).
			Str("wolf_id", wolfID).
			Msg("Failed to mark Wolf as degraded")
	}
}
