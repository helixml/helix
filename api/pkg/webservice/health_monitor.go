package webservice

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// HealthMonitor periodically probes every live project web service and
// auto-recovers any that stop responding, so a crashed or hung stack heals
// without a human. It follows the same ticker pattern as DomainVerifier and the
// sandbox reaper.
type HealthMonitor struct {
	store      store.Store
	controller *Controller

	interval      time.Duration
	probeTimeout  time.Duration
	failThreshold int           // consecutive failed probes before recovery fires
	cooldown      time.Duration // minimum gap between recoveries for one project
	buildTimeout  time.Duration // a build older than this is treated as stuck/interrupted

	mu        sync.Mutex
	fails     map[string]int       // projectID -> consecutive probe failures
	lastRecov map[string]time.Time // projectID -> last recovery trigger time
}

// NewHealthMonitor builds a monitor with production-sane defaults: probe every
// 30s, recover after ~90s of continuous failure, and don't re-recover the same
// project more than once per 5 minutes (a cold redeploy can take minutes).
func NewHealthMonitor(s store.Store, c *Controller) *HealthMonitor {
	return &HealthMonitor{
		store:         s,
		controller:    c,
		interval:      30 * time.Second,
		probeTimeout:  8 * time.Second,
		failThreshold: 3,
		cooldown:      5 * time.Minute,
		buildTimeout:  DeployBuildTimeout,
		fails:         map[string]int{},
		lastRecov:     map[string]time.Time{},
	}
}

// Start runs the monitor on a ticker until ctx is cancelled.
func (m *HealthMonitor) Start(ctx context.Context) {
	// A build cannot survive the process that was orchestrating it. On startup
	// (e.g. after a CD upgrade) fail any deploy still marked in-flight so a
	// web service whose build was interrupted recovers on the first tick,
	// instead of the stale in-flight row wedging recovery until buildTimeout.
	if n, err := m.store.FailInFlightWebServiceDeploys(ctx); err != nil {
		log.Warn().Err(err).Msg("web-service health-monitor: failing orphaned in-flight deploys on startup failed")
	} else if n > 0 {
		log.Info().Int64("count", n).Msg("web-service health-monitor: failed orphaned in-flight deploys on startup")
	}

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	log.Info().Dur("interval", m.interval).Msg("web-service health-monitor started")
	for {
		m.runOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *HealthMonitor) runOnce(ctx context.Context) {
	states, err := m.store.ListActiveWebServices(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("health-monitor: list active web services failed")
		return
	}
	active := make(map[string]struct{}, len(states))
	for _, st := range states {
		active[st.ProjectID] = struct{}{}
		// A deploy in flight (initial `docker compose up --build` or a redeploy)
		// legitimately won't answer for minutes — don't treat that as unhealthy.
		// Recovering mid-build needlessly restarts the stack, and for a slow build
		// could repeatedly interrupt it. Skip and clear any accrued failures.
		if m.deployInProgress(ctx, st.ProjectID) {
			m.reset(st.ProjectID)
			continue
		}
		if m.probe(ctx, st) {
			m.reset(st.ProjectID)
			continue
		}
		m.onFailure(st.ProjectID)
	}
	m.gc(active)
}

// deployInProgress reports whether the project's most recent web-service deploy
// is genuinely still pending or building. ListWebServiceDeploys returns
// newest-first.
//
// A build that has been pending/building for longer than buildTimeout was
// almost certainly interrupted — e.g. the API or runner restarted during a CD
// upgrade — and its status was never advanced to failed. Without the timeout
// such a deploy would be treated as "in progress" forever, so the health
// monitor would skip the project on every tick and never recover it (this is
// exactly how a live web service silently stayed down for days after an
// upgrade). When we see a stale in-flight deploy we mark it failed so it stops
// blocking recovery and the UI stops reporting a phantom build.
func (m *HealthMonitor) deployInProgress(ctx context.Context, projectID string) bool {
	deploys, err := m.store.ListWebServiceDeploys(ctx, projectID, 1)
	if err != nil || len(deploys) == 0 {
		return false
	}
	d := deploys[0]
	switch d.Status {
	case types.WebServiceDeployStatusPending, types.WebServiceDeployStatusBuilding:
		if time.Since(d.StartedAt) < m.buildTimeout {
			return true // genuinely in progress — don't disturb it
		}
		log.Warn().Str("project_id", projectID).Str("deploy_id", d.ID).
			Str("status", string(d.Status)).Dur("age", time.Since(d.StartedAt)).
			Msg("health-monitor: failing stale web-service deploy (interrupted build) so recovery can proceed")
		if err := m.store.UpdateWebServiceDeploy(ctx, d.ID, map[string]interface{}{
			"status": types.WebServiceDeployStatusFailed,
		}); err != nil {
			log.Warn().Err(err).Str("deploy_id", d.ID).
				Msg("health-monitor: could not mark stale deploy failed")
		}
		return false
	default:
		return false
	}
}

// probe returns true if the project's web service answers on its container
// port through the hydra proxy. Delegates to Controller.Probe — the single
// source of truth shared with the API's health reporting.
func (m *HealthMonitor) probe(ctx context.Context, st *types.ProjectWebServiceState) bool {
	return m.controller.Probe(ctx, st, m.probeTimeout)
}

func (m *HealthMonitor) reset(projectID string) {
	m.mu.Lock()
	delete(m.fails, projectID)
	m.mu.Unlock()
}

// onFailure records a failed probe and, once the consecutive-failure threshold
// is crossed (and we're not in cooldown), fires recovery in the background.
func (m *HealthMonitor) onFailure(projectID string) {
	m.mu.Lock()
	m.fails[projectID]++
	n := m.fails[projectID]
	inCooldown := time.Since(m.lastRecov[projectID]) < m.cooldown
	if n < m.failThreshold || inCooldown {
		m.mu.Unlock()
		return
	}
	m.lastRecov[projectID] = time.Now()
	m.fails[projectID] = 0
	m.mu.Unlock()

	log.Warn().Str("project_id", projectID).Int("consecutive_failures", n).
		Msg("health-monitor: web service unhealthy — triggering auto-recovery")
	go func() {
		// Detached from the tick ctx: recovery (a redeploy) can take minutes.
		rctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := m.controller.RecoverWebService(rctx, projectID); err != nil {
			log.Error().Err(err).Str("project_id", projectID).Msg("health-monitor: auto-recovery failed")
		}
	}()
}

// gc drops failure counters for projects that are no longer active so the maps
// don't grow unbounded.
func (m *HealthMonitor) gc(active map[string]struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for pid := range m.fails {
		if _, ok := active[pid]; !ok {
			delete(m.fails, pid)
		}
	}
}
