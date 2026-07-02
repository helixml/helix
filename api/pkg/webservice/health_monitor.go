package webservice

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
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

	mu         sync.Mutex
	fails      map[string]int       // projectID -> consecutive probe failures
	lastRecov  map[string]time.Time // projectID -> last recovery trigger time
	recovFails map[string]int       // projectID -> consecutive FAILED recoveries (drives backoff + looping alert)
	prevActive map[string]struct{}  // active project set from the last tick (for metric GC)
}

// loopingAlertThreshold is the number of consecutive failed recoveries after
// which we log a distinct, loud, alertable error: recovery is looping and needs
// a human. Prometheus alerts on helix_webservice_consecutive_recovery_failures.
const loopingAlertThreshold = 3

// maxRecoveryBackoff caps the exponential backoff between failed recoveries so a
// persistently-broken service is still retried periodically (not abandoned).
const maxRecoveryBackoff = 30 * time.Minute

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
		recovFails:    map[string]int{},
		prevActive:    map[string]struct{}{},
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
		// Guard the tick: a panic in runOnce must not kill the monitor
		// (which would silently stop ALL web-service recovery forever).
		func() {
			defer recoverGoroutine("healthMonitor.runOnce", nil)
			m.runOnce(ctx)
		}()
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
		pid := st.ProjectID
		active[pid] = struct{}{}
		// A deploy in flight (initial `docker compose up --build` or a redeploy)
		// legitimately won't answer for minutes — don't treat that as unhealthy.
		// Recovering mid-build needlessly restarts the stack, and for a slow build
		// could repeatedly interrupt it. Skip and clear any accrued failures.
		// Treat "deploying" as up so the down-alert doesn't fire during a build.
		if m.deployInProgress(ctx, pid) {
			metricUp.WithLabelValues(pid).Set(1)
			m.reset(pid)
			continue
		}
		if m.probe(ctx, st) {
			metricUp.WithLabelValues(pid).Set(1)
			m.onSuccess(pid)
			continue
		}
		metricUp.WithLabelValues(pid).Set(0)
		m.onFailure(pid)
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

// onSuccess is called when a project probes healthy: clear its probe-failure and
// recovery-failure counters so backoff resets and the looping alert clears.
func (m *HealthMonitor) onSuccess(projectID string) {
	m.mu.Lock()
	delete(m.fails, projectID)
	recovered := m.recovFails[projectID] > 0
	delete(m.recovFails, projectID)
	m.mu.Unlock()
	if recovered {
		log.Info().Str("project_id", projectID).Msg("health-monitor: web service healthy again after recovery")
	}
	metricConsecutiveRecoveryFailures.WithLabelValues(projectID).Set(0)
}

// onFailure records a failed probe and, once the consecutive-failure threshold
// is crossed (and we're past the — backoff-extended — cooldown), fires recovery
// in the background.
func (m *HealthMonitor) onFailure(projectID string) {
	m.mu.Lock()
	m.fails[projectID]++
	n := m.fails[projectID]
	// Exponential backoff after failed recoveries: don't hammer a persistently
	// broken service every cooldown (bad commit, image-pull failure, dead
	// runner). Grows cooldown by 2^recovFails, capped at maxRecoveryBackoff so
	// it's still retried periodically rather than abandoned.
	backoff := m.cooldown
	if rf := m.recovFails[projectID]; rf > 0 {
		shift := rf
		if shift > 5 {
			shift = 5
		}
		backoff = m.cooldown << uint(shift)
		if backoff > maxRecoveryBackoff {
			backoff = maxRecoveryBackoff
		}
	}
	inCooldown := time.Since(m.lastRecov[projectID]) < backoff
	if n < m.failThreshold || inCooldown {
		m.mu.Unlock()
		return
	}
	m.lastRecov[projectID] = time.Now()
	m.fails[projectID] = 0
	m.mu.Unlock()

	log.Warn().Str("project_id", projectID).Int("consecutive_failures", n).
		Msg("health-monitor: web service unhealthy — triggering auto-recovery")
	go m.doRecover(projectID)
}

// doRecover runs a recovery attempt detached from the tick, records the result
// (metrics + backoff counter), and — critically — recovers from any panic so a
// bug in the recovery path degrades one service instead of crashing the whole
// API (and with it every hosted service and the control plane).
func (m *HealthMonitor) doRecover(projectID string) {
	defer recoverGoroutine("healthMonitor.doRecover project="+projectID, func(any) {
		m.recordRecoveryResult(projectID, false)
	})
	// Detached from the tick ctx: recovery (a redeploy) can take minutes.
	rctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	timer := prometheus.NewTimer(metricRecoveryDuration.WithLabelValues(projectID))
	err := m.controller.RecoverWebService(rctx, projectID)
	timer.ObserveDuration()
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("health-monitor: auto-recovery failed")
		m.recordRecoveryResult(projectID, false)
		return
	}
	m.recordRecoveryResult(projectID, true)
}

// recordRecoveryResult updates recovery metrics + the consecutive-failure
// counter that drives backoff and the "recovery is looping" alert.
func (m *HealthMonitor) recordRecoveryResult(projectID string, ok bool) {
	if ok {
		metricRecoveryTotal.WithLabelValues(projectID, "success").Inc()
		m.mu.Lock()
		delete(m.recovFails, projectID)
		m.mu.Unlock()
		metricConsecutiveRecoveryFailures.WithLabelValues(projectID).Set(0)
		return
	}
	metricRecoveryTotal.WithLabelValues(projectID, "failure").Inc()
	m.mu.Lock()
	m.recovFails[projectID]++
	rf := m.recovFails[projectID]
	m.mu.Unlock()
	metricConsecutiveRecoveryFailures.WithLabelValues(projectID).Set(float64(rf))
	if rf >= loopingAlertThreshold {
		// Distinct, loud, alertable: auto-recovery cannot fix this on its own.
		log.Error().Str("project_id", projectID).Int("consecutive_recovery_failures", rf).
			Msg("health-monitor: web-service recovery is LOOPING — auto-recovery keeps failing, needs an operator")
	}
}

// gc drops counters and metric series for projects that are no longer active
// web services so the maps and Prometheus label cardinality stay bounded.
func (m *HealthMonitor) gc(active map[string]struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gone := func(pid string) bool { _, ok := active[pid]; return !ok }
	for pid := range m.fails {
		if gone(pid) {
			delete(m.fails, pid)
		}
	}
	for pid := range m.lastRecov {
		if gone(pid) {
			delete(m.lastRecov, pid)
		}
	}
	for pid := range m.recovFails {
		if gone(pid) {
			delete(m.recovFails, pid)
		}
	}
	// Forget metric series for projects we tracked last tick but are no longer
	// active (e.g. web-service disabled/deleted), so gauges don't go stale.
	for pid := range m.prevActive {
		if gone(pid) {
			forgetProjectMetrics(pid)
		}
	}
	m.prevActive = active
}
