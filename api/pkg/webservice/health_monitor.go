package webservice

import (
	"context"
	"fmt"
	"strings"
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

	// alerter delivers the down alert to a human directly (Slack via janitor +
	// admin email), independent of Prometheus. Optional — nil disables the
	// in-process alert path, leaving only the metrics.
	alerter DownAlerter
	// appURL is the control plane's public base URL, used to build a deep link
	// to the failing project's deploy log in the alert.
	appURL string

	mu         sync.Mutex
	fails      map[string]int       // projectID -> consecutive probe failures
	lastRecov  map[string]time.Time // projectID -> last recovery trigger time
	recovFails map[string]int       // projectID -> consecutive FAILED recoveries (drives backoff + looping alert)
	downSince  map[string]time.Time // projectID -> start of the current down-streak (drives the page)
	alerted    map[string]struct{}  // projectID -> already paged for the current down-streak
	prevActive map[string]struct{}  // active project set from the last tick (for metric GC)
}

// DownAlerter delivers a hosted-web-service-down page to a human. Implemented by
// notification.AdminAlerter (Slack webhook via janitor + admin email).
type DownAlerter interface {
	SendWebServiceDownAlert(ctx context.Context, data *types.WebServiceDownAlert)
	SendWebServiceRecoveredAlert(ctx context.Context, data *types.WebServiceDownAlert)
}

// loopingAlertThreshold is the number of consecutive failed recoveries after
// which we log a distinct, loud, alertable error: recovery is looping and needs
// a human. Prometheus alerts on helix_webservice_consecutive_recovery_failures.
const loopingAlertThreshold = 3

// maxRecoveryBackoff caps the exponential backoff between failed recoveries so a
// persistently-broken service is still retried periodically (not abandoned).
const maxRecoveryBackoff = 30 * time.Minute

// DownAlertThreshold is how long a web service must be continuously down before
// we page a human. It must stay above readinessWait (10 min) so a legitimately
// slow cold deploy — which can take that long to bind its port — never pages
// anyone. The Prometheus rule in deploy/monitoring/helix-webservice.rules.yml
// uses the same 15 minutes; keep the two in step.
const DownAlertThreshold = 15 * time.Minute

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
		downSince:     map[string]time.Time{},
		alerted:       map[string]struct{}{},
		prevActive:    map[string]struct{}{},
	}
}

// SetAlerter wires the direct (non-Prometheus) page delivery path. Optional:
// with no alerter the monitor still maintains every metric, but a down service
// relies entirely on Prometheus scraping this process — which is exactly the
// assumption that failed during the we-find.ai outage.
func (m *HealthMonitor) SetAlerter(a DownAlerter, appURL string) {
	m.alerter = a
	m.appURL = appURL
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
		switch {
		// A deploy in flight (initial `docker compose up --build` or a redeploy)
		// legitimately won't answer for minutes — don't treat that as unhealthy.
		// Recovering mid-build needlessly restarts the stack, and for a slow build
		// could repeatedly interrupt it. Skip and clear any accrued probe failures.
		//
		// This sets helix_webservice_up to 1, which is why that gauge must never
		// be paged on: auto-recovery redeploys a broken service every ~11 minutes,
		// so a permanently-down site spends much of its life "deploying". The
		// down-streak (metricUnhealthySince) is deliberately NOT cleared here — a
		// recovery attempt is an attempt to fix, not evidence of health.
		case m.deployInProgress(ctx, pid):
			metricUp.WithLabelValues(pid).Set(1)
			m.reset(pid)
		case m.probe(ctx, st):
			metricUp.WithLabelValues(pid).Set(1)
			m.onSuccess(ctx, pid)
		default:
			metricUp.WithLabelValues(pid).Set(0)
			m.onFailure(pid)
		}
		// Evaluated on every tick, including mid-deploy, so a service that is
		// down but perpetually redeploying still pages.
		m.evaluateDownAlert(ctx, pid)
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
// recovery-failure counters so backoff resets and the looping alert clears, and
// end the down-streak. A successful probe is the ONLY thing that clears the
// down-streak — that is what makes it safe to page on.
func (m *HealthMonitor) onSuccess(ctx context.Context, projectID string) {
	m.mu.Lock()
	delete(m.fails, projectID)
	recovered := m.recovFails[projectID] > 0
	delete(m.recovFails, projectID)
	downFor := time.Duration(0)
	if since, ok := m.downSince[projectID]; ok {
		downFor = time.Since(since)
	}
	delete(m.downSince, projectID)
	_, wasAlerted := m.alerted[projectID]
	delete(m.alerted, projectID)
	m.mu.Unlock()

	if recovered {
		log.Info().Str("project_id", projectID).Msg("health-monitor: web service healthy again after recovery")
	}
	metricConsecutiveRecoveryFailures.WithLabelValues(projectID).Set(0)
	metricUnhealthySince.DeleteLabelValues(projectID)

	// Close the loop on a page we sent, so the operator knows it ended without
	// having to go and check.
	if wasAlerted && m.alerter != nil {
		data := m.buildAlert(ctx, projectID, downFor)
		log.Info().Str("project_id", projectID).Dur("down_for", downFor).
			Msg("health-monitor: web service recovered — sending recovery notification")
		m.alerter.SendWebServiceRecoveredAlert(ctx, data)
	}
}

// evaluateDownAlert pages a human once a project has been continuously down for
// DownAlertThreshold. Exactly one page per down-streak; the streak (and so the
// page) is only cleared by a successful probe.
func (m *HealthMonitor) evaluateDownAlert(ctx context.Context, projectID string) {
	m.mu.Lock()
	since, down := m.downSince[projectID]
	if !down {
		m.mu.Unlock()
		return
	}
	downFor := time.Since(since)
	_, already := m.alerted[projectID]
	if already || downFor < DownAlertThreshold {
		m.mu.Unlock()
		return
	}
	m.alerted[projectID] = struct{}{}
	recovFails := m.recovFails[projectID]
	m.mu.Unlock()

	data := m.buildAlert(ctx, projectID, downFor)
	data.ConsecutiveRecoveryFailures = recovFails

	// Loud regardless of whether a delivery channel is configured: this line is
	// the last-resort record that the platform knew the site was down.
	log.Error().
		Str("project_id", projectID).
		Str("project_name", data.ProjectName).
		Strs("domains", data.Domains).
		Dur("down_for", downFor).
		Int("consecutive_recovery_failures", recovFails).
		Str("deploy_error", data.DeployError).
		Msg("health-monitor: hosted web service is DOWN past the alert threshold — paging")

	if m.alerter == nil {
		log.Warn().Str("project_id", projectID).
			Msg("health-monitor: no alerter configured — down alert not delivered to a human")
		return
	}
	m.alerter.SendWebServiceDownAlert(ctx, data)
}

// buildAlert assembles the page payload. Every lookup is best-effort: a missing
// project row or domain list must never stop the alert going out.
func (m *HealthMonitor) buildAlert(ctx context.Context, projectID string, downFor time.Duration) *types.WebServiceDownAlert {
	data := &types.WebServiceDownAlert{
		ProjectID: projectID,
		DownFor:   downFor,
	}
	if project, err := m.store.GetProject(ctx, projectID); err == nil && project != nil {
		data.ProjectName = project.Name
	}
	if routes, err := m.store.ListVHostRoutesByTarget(ctx, types.VHostTargetProjectWebService, projectID); err == nil {
		for _, r := range routes {
			data.Domains = append(data.Domains, r.Hostname)
		}
	}
	if deploys, err := m.store.ListWebServiceDeploys(ctx, projectID, 1); err == nil && len(deploys) > 0 {
		data.DeployError = deploys[0].Error
	}
	if m.appURL != "" {
		data.DeployLogURL = fmt.Sprintf("%s/projects/%s/web-service", strings.TrimSuffix(m.appURL, "/"), projectID)
	}
	return data
}

// onFailure records a failed probe and, once the consecutive-failure threshold
// is crossed (and we're past the — backoff-extended — cooldown), fires recovery
// in the background.
func (m *HealthMonitor) onFailure(projectID string) {
	m.mu.Lock()
	m.fails[projectID]++
	n := m.fails[projectID]
	// Start the down-streak on the first failed probe and never refresh it while
	// the service stays down, so `time() - gauge` grows monotonically for as long
	// as the outage lasts.
	if _, ok := m.downSince[projectID]; !ok {
		m.downSince[projectID] = time.Now()
		metricUnhealthySince.WithLabelValues(projectID).Set(float64(time.Now().Unix()))
	}
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
	for pid := range m.downSince {
		if gone(pid) {
			delete(m.downSince, pid)
		}
	}
	for pid := range m.alerted {
		if gone(pid) {
			delete(m.alerted, pid)
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
