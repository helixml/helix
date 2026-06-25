package webservice

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/sandbox"
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
	sandboxes  *sandbox.Controller

	interval      time.Duration
	probeTimeout  time.Duration
	failThreshold int           // consecutive failed probes before recovery fires
	cooldown      time.Duration // minimum gap between recoveries for one project

	mu        sync.Mutex
	fails     map[string]int       // projectID -> consecutive probe failures
	lastRecov map[string]time.Time // projectID -> last recovery trigger time
}

// NewHealthMonitor builds a monitor with production-sane defaults: probe every
// 30s, recover after ~90s of continuous failure, and don't re-recover the same
// project more than once per 5 minutes (a cold redeploy can take minutes).
func NewHealthMonitor(s store.Store, c *Controller, sb *sandbox.Controller) *HealthMonitor {
	return &HealthMonitor{
		store:         s,
		controller:    c,
		sandboxes:     sb,
		interval:      30 * time.Second,
		probeTimeout:  8 * time.Second,
		failThreshold: 3,
		cooldown:      5 * time.Minute,
		fails:         map[string]int{},
		lastRecov:     map[string]time.Time{},
	}
}

// Start runs the monitor on a ticker until ctx is cancelled.
func (m *HealthMonitor) Start(ctx context.Context) {
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
// is still pending or building. ListWebServiceDeploys returns newest-first.
func (m *HealthMonitor) deployInProgress(ctx context.Context, projectID string) bool {
	deploys, err := m.store.ListWebServiceDeploys(ctx, projectID, 1)
	if err != nil || len(deploys) == 0 {
		return false
	}
	switch deploys[0].Status {
	case types.WebServiceDeployStatusPending, types.WebServiceDeployStatusBuilding:
		return true
	default:
		return false
	}
}

// probe returns true if the project's web service answers on its container
// port through the hydra proxy. ProbeDevContainerPort errors on transport
// failure / 4xx / 5xx (see hydra doRequest), so any of those count as down.
func (m *HealthMonitor) probe(ctx context.Context, st *types.ProjectWebServiceState) bool {
	sb, err := m.sandboxes.Get(ctx, st.ActiveSandboxID)
	if err != nil || sb == nil || sb.Status != types.SandboxStatusRunning {
		return false
	}
	hc, err := m.sandboxes.HydraClient(sb)
	if err != nil {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, m.probeTimeout)
	defer cancel()
	return hc.ProbeDevContainerPort(pctx, sb.ID, st.ContainerPort) == nil
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
