package compute

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SandboxStore is the narrow slice of the Helix store that the Manager
// uses to read and persist SandboxInstance rows. Defined here so the
// Manager doesn't import the full store package (the concrete
// PostgresStore satisfies this interface implicitly).
type SandboxStore interface {
	ListSandboxInstances(ctx context.Context) ([]*types.SandboxInstance, error)
	GetSandboxInstance(ctx context.Context, id string) (*types.SandboxInstance, error)
	RegisterSandboxInstance(ctx context.Context, instance *types.SandboxInstance) error
	UpdateSandboxInstanceStatus(ctx context.Context, id, status string) error
	UpdateSandboxInstanceComputeState(ctx context.Context, id, computeState string) error
	UpdateSandboxInstanceProviderID(ctx context.Context, id, providerID string) error
	DeregisterSandboxInstance(ctx context.Context, id string) error
}

// ManagerConfig configures a single ComputeManager instance.
//
// One Manager per Helix install: it owns one Provider and reconciles
// the cloud's view against Helix's SandboxInstance rows on a timer.
// Per-org provider scoping is deliberately out of scope for v1 - the
// POC has one YD account per Helix deployment.
type ManagerConfig struct {
	// Floor is the minimum number of provisioned hosts the Manager
	// keeps available at all times. The Reconcile loop kicks off
	// Provision calls until the total (Ready + Provisioning) reaches
	// this count. Set to 0 to disable pre-warming (the Manager
	// remains constructed but its Reconcile loop is a no-op until
	// on-demand provisioning lands in a follow-up).
	Floor int

	// ReconcileInterval is how often the reconciliation loop runs.
	// Lower values respond faster to drift; higher values reduce
	// pressure on both Helix and the Provider's API. 30s is a
	// reasonable default - matches the existing sandbox heartbeat
	// cadence.
	ReconcileInterval time.Duration

	// HealthCheckTimeout caps how long one HealthCheck call can take
	// before the loop moves on. Provider implementations are expected
	// to honour ctx, but this is a safety net.
	HealthCheckTimeout time.Duration

	// MaxConcurrentProvisions caps how many Provision calls the
	// Manager fires off in a single Reconcile cycle when below Floor.
	// Defaults to 1 if unset. Operators with a large Floor on cold
	// boot can raise this to (e.g.) 5 so reaching the floor doesn't
	// take MaxConcurrentProvisions * ReconcileInterval. Each Provision
	// is still synchronous within the cycle; the cap is on per-cycle
	// fan-out, not on parallelism.
	MaxConcurrentProvisions int

	// MaxProvisioningAge bounds how long a row may sit in
	// ComputeState=provisioning before the Manager gives up and rolls
	// it back. Without this, a stuck upstream task (image pull
	// failure, capacity exhausted, scheduler hung) would hold a Floor
	// slot indefinitely and the Manager would believe it had capacity
	// it does not.
	//
	// Default 30m if unset. Picked from observed YD behaviour: the
	// happy path on g5.xlarge in eu-west-2 is ~10m, and one
	// cross-region fallback or a slow NVIDIA image pull can push it
	// to 20m+. Anything under 30m would time out legitimate
	// provisions in production. Operators with faster providers can
	// lower this explicitly.
	MaxProvisioningAge time.Duration

	// SpecTemplate is the Spec passed to Provider.Provision when the
	// Manager decides to bring up a new host. Operator-supplied at
	// startup; carries the helix-sandbox image tag, default
	// MaxSandboxes per host, and any provider-specific Labels.
	SpecTemplate Spec

	// Max is the hard ceiling on the number of Manager-owned hosts
	// (Ready + Provisioning combined). Zero (the default) disables
	// on-demand scale-up - the Manager only maintains Floor and
	// ignores demand pressure. Set Max > Floor to allow the Manager
	// to provision extra hosts when sandbox-session demand exhausts
	// the headroom on existing Ready hosts.
	//
	// Must be >= Floor when non-zero. The validation rejects Max
	// values that would prevent the Manager from satisfying Floor.
	Max int

	// ScaleUpHeadroomMin is the minimum number of free sandbox slots
	// the Manager tries to keep available across all Ready hosts.
	// When (sum(MaxSandboxes) - sum(ActiveSandboxes)) drops below
	// this value AND total owned is below Max, the Manager
	// provisions an additional host.
	//
	// Default 1 if unset (and Max > Floor) - "scale when 0 free slots
	// remain", which is the minimum-cost setting. Operators serving
	// bursty workloads can raise this (e.g. to 2 or 3) to provision
	// the next host before the last slot is claimed, hiding the
	// ~90s cold-start latency from the user.
	//
	// Ignored when Max == 0 (D3 disabled).
	ScaleUpHeadroomMin int

	// IdleTimeout is the duration a Ready host must have zero active
	// sandbox sessions before the Manager will deprovision it to
	// converge back toward Floor. Zero (the default in this struct;
	// envconfig sets a 10m default at the boundary) disables idle
	// deprovision (D4) entirely; in that mode the Manager only scales
	// UP from Floor, never DOWN.
	//
	// The idle timer is tracked in-memory and resets on Manager
	// restart. An operator restarting Helix while a fleet is in
	// scale-down may see one extra IdleTimeout window of grace before
	// the converge-back resumes. Acceptable trade-off for v1; if
	// the timer needs to survive restarts (multi-instance HA), the
	// "idle_since" timestamp moves to the sandbox_instance row.
	//
	// Hosts are never deprovisioned below Floor: the converge target
	// is Floor, not zero. An operator running Floor=0 and Max>0
	// (pure on-demand, no warm baseline) gets full scale-down to
	// zero hosts once all sandboxes drain.
	IdleTimeout time.Duration
}

// validate returns an error if cfg is missing required fields or has
// values that would break the loop (negative floor, zero interval).
func (cfg ManagerConfig) validate() error {
	if cfg.Floor < 0 {
		return fmt.Errorf("compute.ManagerConfig.Floor must be >= 0, got %d", cfg.Floor)
	}
	if cfg.ReconcileInterval <= 0 {
		return errors.New("compute.ManagerConfig.ReconcileInterval must be > 0")
	}
	if cfg.HealthCheckTimeout <= 0 {
		return errors.New("compute.ManagerConfig.HealthCheckTimeout must be > 0")
	}
	if cfg.Max < 0 {
		return fmt.Errorf("compute.ManagerConfig.Max must be >= 0, got %d", cfg.Max)
	}
	if cfg.Max != 0 && cfg.Max <= cfg.Floor {
		// Max == Floor is a configuration mistake: it claims to enable
		// scale-up (Max != 0) but pins the ceiling at the warm-baseline,
		// so the demand branch (gated on Max > Floor) never fires. The
		// ultrareview flagged this as silent-broken: validation accepted
		// it, runtime gate refused to fire, no log explained why.
		// Operators who genuinely want "fixed warm-baseline, no scale-up"
		// set Max=0 (which preserves the original D1/D2 behaviour
		// exactly). Anyone setting Max means they want scale-up, so
		// Max == Floor is rejected with a clear hint.
		return fmt.Errorf("compute.ManagerConfig.Max=%d must be > Floor=%d (use Max=0 to disable scale-up while keeping the warm baseline)",
			cfg.Max, cfg.Floor)
	}
	if cfg.ScaleUpHeadroomMin < 0 {
		return fmt.Errorf("compute.ManagerConfig.ScaleUpHeadroomMin must be >= 0, got %d",
			cfg.ScaleUpHeadroomMin)
	}
	if cfg.IdleTimeout < 0 {
		return fmt.Errorf("compute.ManagerConfig.IdleTimeout must be >= 0, got %s",
			cfg.IdleTimeout)
	}
	return nil
}

// Manager runs the reconciliation loop that keeps Helix's view of
// provisioned hosts in sync with what the Provider says actually exists
// and with the Floor / on-demand demand signal.
//
// Lifecycle:
//   - NewManager returns a Manager that has not yet started reconciling.
//   - Run(ctx) blocks until ctx is cancelled, ticking Reconcile each
//     cfg.ReconcileInterval.
//   - Reconcile(ctx) can also be called directly (e.g. from tests, or
//     by an HTTP handler that wants to force an immediate cycle).
//
// One Manager is constructed per Helix install at boot time. The
// Manager is goroutine-safe: Reconcile holds an internal mutex so
// concurrent calls do not overlap.
type Manager struct {
	provider Provider
	store    SandboxStore
	cfg      ManagerConfig

	// mu serialises Reconcile calls. The loop runs at most one cycle
	// at a time; an external trigger that arrives while a cycle is
	// running will block on this mutex.
	mu sync.Mutex

	// idleSince maps a sandbox ID to the time it was first observed
	// with zero active sandbox sessions. Populated and cleared inside
	// Reconcile (under mu). Used by the D4 idle-deprovision arm to
	// enforce a hysteresis window: a host must be continuously idle
	// for IdleTimeout before it becomes a deprovision candidate.
	// On Manager restart the map starts empty; one IdleTimeout window
	// of grace before scale-down resumes is acceptable for v1.
	idleSince map[string]time.Time

	// deprovisionFailingSince maps a sandbox ID to the time its first
	// Deprovision call started failing. Caps the retry storm when the
	// upstream is permanently broken (instance already gone, IAM
	// revoked, region offline). After maxDeprovisionRetryAge of
	// continuous failure, the Manager gives up: logs error, removes
	// the Helix row, leaves the (possibly-orphaned) upstream for an
	// operator to investigate. Better than an indefinite
	// phantom-Ready row that blocks legitimate scale-up forever.
	deprovisionFailingSince map[string]time.Time

	// now is the clock source. Defaults to time.Now; tests inject a
	// fake clock to drive the idle-window logic without sleeping.
	now func() time.Time
}

// maxDeprovisionRetryAge bounds the D4 retry storm when Provider.Deprovision
// keeps failing for the same candidate (e.g. upstream 404 because the
// instance was deleted out-of-band; IAM revoked; region offline). After
// this duration of continuous failure, the Manager removes the Helix row
// even though the upstream Deprovision didn't succeed - the alternative
// is an indefinite phantom-Ready row that suppresses legitimate scale-up.
// 15m matches the existing MaxProvisioningAge default's order of magnitude
// (provisioning gives up after 30m; deprovisioning gives up after 15m so
// the cluster's bad-state windows are bounded similarly).
const maxDeprovisionRetryAge = 15 * time.Minute

// NewManager validates cfg and constructs a Manager. Returns an error
// if provider or store is nil, or cfg is invalid. Defaults are applied
// for MaxConcurrentProvisions (1) and MaxProvisioningAge (15m) so
// operators only need to set values they want to override.
func NewManager(provider Provider, store SandboxStore, cfg ManagerConfig) (*Manager, error) {
	if provider == nil {
		return nil, errors.New("compute.NewManager: provider is required")
	}
	if store == nil {
		return nil, errors.New("compute.NewManager: store is required")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.MaxConcurrentProvisions <= 0 {
		cfg.MaxConcurrentProvisions = 1
	}
	if cfg.MaxProvisioningAge <= 0 {
		cfg.MaxProvisioningAge = 30 * time.Minute
	}
	if cfg.Max > 0 && cfg.ScaleUpHeadroomMin <= 0 {
		// Default the headroom-min only when scale-up is enabled.
		// Leaving it at zero with Max>Floor would mean "scale when
		// headroom < 0", which can never trigger - effectively
		// disabling scale-up despite the operator setting Max.
		cfg.ScaleUpHeadroomMin = 1
	}
	// IdleTimeout is NOT defaulted here. The envconfig binding on
	// config.Compute.IdleTimeout sets the operator-facing 10m default
	// at the env-loading boundary, so production deployments get
	// HELIX_COMPUTE_IDLE_TIMEOUT=10m unless overridden. Honouring 0
	// here as "disabled" matches the docstring (the original commit's
	// silent rewrite to 10m was a defect caught by ultrareview - an
	// operator setting HELIX_COMPUTE_IDLE_TIMEOUT=0 during an incident
	// to freeze the fleet was getting D4 silently re-enabled).
	return &Manager{
		provider:                provider,
		store:                   store,
		cfg:                     cfg,
		idleSince:               make(map[string]time.Time),
		deprovisionFailingSince: make(map[string]time.Time),
		now:                     time.Now,
	}, nil
}

// Run blocks until ctx is cancelled, calling Reconcile every
// cfg.ReconcileInterval. The first reconcile happens immediately
// rather than after one full interval - operators starting Helix
// with Floor>0 expect pre-warming to begin right away.
//
// A Reconcile error does NOT terminate the loop. The error is logged
// and the next tick proceeds normally. This is intentional: a
// transient Provider hiccup must not freeze pre-warming forever.
func (m *Manager) Run(ctx context.Context) error {
	t := time.NewTicker(m.cfg.ReconcileInterval)
	defer t.Stop()

	// First reconcile immediately so Floor pre-warming starts at boot.
	m.runOneCycle(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			m.runOneCycle(ctx)
		}
	}
}

func (m *Manager) runOneCycle(ctx context.Context) {
	if err := m.Reconcile(ctx); err != nil {
		log.Warn().
			Err(err).
			Str("provider", m.provider.Name()).
			Msg("compute manager reconcile cycle failed; will retry on next tick")
	}
}

// Reconcile runs one pass: refresh state of in-flight provisions,
// then bring the cluster up to Floor if it's below.
//
// Idempotent. The Manager fires off at most MaxConcurrentProvisions
// Provision calls per cycle so a slow Provider cannot be hammered
// and operators get a knob to trade cold-start latency against
// upstream load. Default is 1.
func (m *Manager) Reconcile(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rows, err := m.ownedRows(ctx)
	if err != nil {
		return fmt.Errorf("list owned rows: %w", err)
	}

	// First pass: refresh provisioning rows. Real state changes get
	// persisted to the row's ComputeState column; rows that have
	// exceeded MaxProvisioningAge are rolled back. We do this before
	// counting available capacity so a host that just transitioned
	// to Ready (or got rolled back) counts correctly in this cycle.
	m.refreshProvisioning(ctx, rows)

	// Re-read after refresh so state changes are reflected in the
	// count. Cheap (single DB query) and correct.
	rows, err = m.ownedRows(ctx)
	if err != nil {
		return fmt.Errorf("re-list owned rows after refresh: %w", err)
	}

	needed := m.computeNeeded(rows)
	var firstErr error
	provisionedThisCycle := 0
	// Each Provision is independent; we surface the first error but
	// keep going so a transient upstream blip doesn't permanently
	// stall the cluster at a partial fill.
	for i := 0; i < needed; i++ {
		if err := m.provisionOne(ctx); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("provision: %w", err)
			}
		} else {
			provisionedThisCycle++
		}
	}

	// D4: idle deprovision. Skipped under two conditions, both
	// fixes from the ultrareview:
	//
	//   1. A Provision actually succeeded this cycle - racing the
	//      new host into Terminating before it reaches Ready would
	//      waste work. Original "needed == 0" skipped on attempts;
	//      we now skip only on actual successes.
	//   2. Demand pressure exists (headroom below threshold) even
	//      if the Max ceiling clamped needed to 0. Otherwise D4
	//      sheds an idle host while D3 wanted to ADD one,
	//      oscillating the cluster between Floor and Max forever
	//      (the original commit's "churn loop" footgun).
	if provisionedThisCycle == 0 && !m.demandPressureExists(rows) {
		if err := m.tryDeprovisionIdle(ctx, rows); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("deprovision idle: %w", err)
		}
	}
	return firstErr
}

// demandPressureExists reports whether free sandbox slots across Ready
// hosts have fallen below ScaleUpHeadroomMin. Used by Reconcile to
// gate D4 - even when D3 was blocked from acting (e.g. Max ceiling
// reached, Provision failed), shedding an idle host under demand
// pressure creates an unproductive scale-down/scale-up oscillation.
//
// Returns false when D3 is disabled (Max == 0) or no Ready hosts exist.
func (m *Manager) demandPressureExists(rows []*types.SandboxInstance) bool {
	if m.cfg.Max <= m.cfg.Floor {
		return false // D3 disabled; no concept of demand pressure
	}
	readyCapacity := 0
	readyDemand := 0
	readyAny := false
	for _, r := range rows {
		// Capacity math uses only REACHABLE hosts. A Ready+offline
		// row contributes no live sandbox slots, so counting it would
		// hide real demand pressure.
		if !isReadyAndOnline(r) {
			continue
		}
		readyAny = true
		readyCapacity += int(r.MaxSandboxes)
		readyDemand += int(r.ActiveSandboxes)
	}
	if !readyAny {
		return false
	}
	return (readyCapacity - readyDemand) < m.cfg.ScaleUpHeadroomMin
}

// tryDeprovisionIdle is the D4 scale-down arm. Updates the idle-since
// tracker against the current Ready rows, then deprovisions one host
// per cycle if a Ready host has been continuously idle for >= IdleTimeout
// AND deprovisioning won't drop ready_count below Floor.
//
// One host per cycle (mirrors D3's single-step) so a fleet doesn't
// drain abruptly; sustained low demand walks the cluster down toward
// Floor over multiple cycles.
//
// Uses isReadyState (ComputeState only, not Status) for both the
// candidate set and the idleSince tracker. This is intentional:
//
//   - Heartbeat-flap tolerance: a host that briefly flickers offline
//     and back should NOT lose its accumulated idle time. If the
//     prune loop used isReadyAndOnline, the flap-offline cycle would
//     drop the idleSince entry, then the next cycle (back online)
//     would re-insert at NOW, restarting the timer. A noisy network
//     would prevent any host from ever crossing the IdleTimeout
//     window.
//
//   - Orphan-rot prevention: Ready+offline rows that genuinely died
//     (host crashed, network gone) accumulate forever without a
//     reclaim path otherwise. Letting D4 shed them (when they're
//     also idle) is the simplest cleanup.
//
// The map is still pruned of rows that have left the Ready state
// entirely (Failed/Terminated by another path).
func (m *Manager) tryDeprovisionIdle(ctx context.Context, rows []*types.SandboxInstance) error {
	if m.cfg.IdleTimeout <= 0 {
		// Disabled by config.
		return nil
	}

	now := m.now()
	readyByID := make(map[string]*types.SandboxInstance)
	readyCount := 0
	for _, r := range rows {
		if !isReadyState(r) {
			continue
		}
		readyByID[r.ID] = r
		readyCount++
	}

	// Update tracker: mark currently-idle Ready rows; clear busy ones;
	// prune entries whose row is no longer in Ready state (Failed,
	// Terminated, or removed by another code path - heartbeat-flap
	// to offline does NOT count as "no longer Ready").
	for _, r := range rows {
		if !isReadyState(r) {
			continue
		}
		if r.ActiveSandboxes == 0 {
			if _, tracked := m.idleSince[r.ID]; !tracked {
				m.idleSince[r.ID] = now
			}
		} else {
			delete(m.idleSince, r.ID)
			// Host picked up work mid-shed-retry: clear the
			// deprovision-failing tracker too. If it later goes idle
			// again, retry budget resets from zero.
			delete(m.deprovisionFailingSince, r.ID)
		}
	}
	for id := range m.idleSince {
		if _, stillReady := readyByID[id]; !stillReady {
			delete(m.idleSince, id)
			delete(m.deprovisionFailingSince, id)
		}
	}
	// Also prune deprovisionFailingSince of any entries whose host has
	// stopped being a shed candidate entirely (e.g. row went to Failed
	// state before D4 finished its retry budget).
	for id := range m.deprovisionFailingSince {
		if _, stillReady := readyByID[id]; !stillReady {
			delete(m.deprovisionFailingSince, id)
		}
	}

	// Can we shed any? Only above Floor.
	if readyCount <= m.cfg.Floor {
		return nil
	}

	// Pick the longest-idle candidate that has crossed the IdleTimeout
	// window. Single deprovision per cycle.
	//
	// Stable tie-break by sandbox_id: when multiple hosts went idle
	// in the same Reconcile cycle (e.g. a batch job finished and
	// released all sandboxes in one tick), their idleSince entries
	// share the same timestamp. Without a tie-break the Go map's
	// randomized iteration order would shed them non-deterministically.
	// Iterate over a sorted ID slice instead.
	ids := make([]string, 0, len(m.idleSince))
	for id := range m.idleSince {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var candidate *types.SandboxInstance
	var candidateIdleSince time.Time
	for _, id := range ids {
		idleAt := m.idleSince[id]
		if now.Sub(idleAt) < m.cfg.IdleTimeout {
			continue
		}
		r := readyByID[id]
		if r == nil {
			continue
		}
		// `<` is strict, so equal timestamps keep the candidate
		// already chosen - which is the first match in sorted-ID
		// order. Stable across runs.
		if candidate == nil || idleAt.Before(candidateIdleSince) {
			candidate = r
			candidateIdleSince = idleAt
		}
	}
	if candidate == nil {
		return nil
	}

	idleFor := now.Sub(candidateIdleSince)
	log.Info().
		Str("sandbox_id", candidate.ID).
		Str("provider_id", candidate.ProviderID).
		Dur("idle_for", idleFor).
		Int("ready_count_before", readyCount).
		Int("floor", m.cfg.Floor).
		Msg("compute manager deprovisioning idle host (D4)")

	if candidate.ProviderID != "" {
		handle := &Handle{
			ProviderName: candidate.Provider,
			ProviderID:   candidate.ProviderID,
			SandboxID:    candidate.ID,
		}
		// Force=true: for the YD-as-runner-host model the "task body"
		// is a persistent supervisor process - there's no in-flight
		// work to gracefully drain. Force=false maps to YD's "drain"
		// semantic which waits for the task to exit on its own; since
		// our task body never exits (it's a long-running helix-sandbox
		// container), drain mode leaves the WR in CANCELLING
		// indefinitely. Force=true maps to "abort tasks", which is
		// the only correct cancel semantic here.
		//
		// The Provider.Deprovision contract is "shut this host down";
		// the differentiation graceful-vs-force came from a generic
		// VM-pool mental model that doesn't fit YD. Future Providers
		// with genuine drainable work can opt back into Force=false.
		if err := m.provider.Deprovision(ctx, handle, DeprovisionOpts{
			Force:  true,
			Reason: "idle deprovision (D4)",
		}); err != nil {
			// Provider.Deprovision failed. The earlier ultrareview-1
			// fixup left the row in place + propagated the error so a
			// transient could be retried. The ultrareview-2 follow-up
			// pointed out that an INDEFINITE retry on a permanent
			// failure (upstream already 404-gone, IAM revoked, region
			// offline) creates a stuck phantom Ready row forever -
			// blocking legitimate scale-up via the readyOnlineCount > 0
			// gate in computeNeeded.
			//
			// Bounded retry: track when failure started for this
			// candidate. Within maxDeprovisionRetryAge, retry next
			// cycle. After that, give up - log loudly, fall through
			// to Deregister (the row goes away; upstream may be an
			// orphan, but it's logged explicitly with the retry
			// duration so an operator can investigate).
			if _, tracked := m.deprovisionFailingSince[candidate.ID]; !tracked {
				m.deprovisionFailingSince[candidate.ID] = now
			}
			failingFor := now.Sub(m.deprovisionFailingSince[candidate.ID])
			if failingFor < maxDeprovisionRetryAge {
				log.Warn().
					Err(err).
					Str("sandbox_id", candidate.ID).
					Str("provider_id", candidate.ProviderID).
					Dur("failing_for", failingFor).
					Dur("retry_budget", maxDeprovisionRetryAge).
					Msg("compute manager Deprovision during idle-shed failed; retrying next cycle")
				return fmt.Errorf("provider Deprovision for %s (failing for %s): %w", candidate.ID, failingFor, err)
			}
			// Retry budget exhausted. Give up on the upstream and
			// reclaim the Helix-side row. Log at error level so the
			// orphan-leak (if upstream is genuinely still alive) is
			// visible to operators - they can manually reap the
			// upstream resource by ID from the log.
			log.Error().
				Err(err).
				Str("sandbox_id", candidate.ID).
				Str("provider_id", candidate.ProviderID).
				Dur("failing_for", failingFor).
				Dur("retry_budget", maxDeprovisionRetryAge).
				Msg("compute manager giving up on Deprovision retry; removing row (upstream may be orphaned - investigate provider_id manually)")
			// Fall through to Deregister.
		}
	}

	// Provider.Deprovision succeeded, OR we gave up retrying after
	// maxDeprovisionRetryAge. Either way, remove the Helix row.
	//
	// Do NOT delete idleSince before DeregisterSandboxInstance: if
	// Deregister fails AND we already deleted the tracker, the next
	// cycle would re-arm idle-since to NOW, restarting the IdleTimeout
	// window from scratch (caught by ultrareview-1).
	if err := m.store.DeregisterSandboxInstance(ctx, candidate.ID); err != nil {
		return fmt.Errorf("deregister %s after Deprovision: %w", candidate.ID, err)
	}
	delete(m.idleSince, candidate.ID)
	delete(m.deprovisionFailingSince, candidate.ID)
	return nil
}

// computeNeeded returns how many additional Provision calls this cycle
// should fire. Considers two pressures:
//
//   - Floor: keep at least Floor (Ready + Provisioning) hosts at all
//     times. The original D1/D2 behaviour.
//   - Demand (D3): when Max > Floor AND free sandbox slots across
//     Ready hosts fall below ScaleUpHeadroomMin, provision more hosts
//     to close the gap. Bounded by Max and by MaxConcurrentProvisions.
//
// Returns the smaller of (sum of pressures, MaxConcurrentProvisions,
// Max - available). Zero or negative means "no provisioning this cycle".
//
// Pre-condition: rows reflects the state AFTER refreshProvisioning -
// so a row that transitioned to Ready this cycle is counted as Ready,
// and a row that timed out is no longer counted.
func (m *Manager) computeNeeded(rows []*types.SandboxInstance) int {
	available := 0
	aliveForFloor := 0
	readyOnlineCount := 0
	readyCapacity := 0
	readyDemand := 0
	// provisioningCapacity is the MaxSandboxes total across rows we've
	// ALREADY submitted a Provision for but that haven't reached Ready
	// yet. Counted toward "committed capacity" in the D3 headroom
	// decision so we don't fire a second Provision in the next cycle
	// for the same demand the first one is on its way to satisfy.
	// Without this, a 2-session burst on a single-Runner fleet with
	// HEADROOM_MIN=1 produces 2 new Runners instead of 1: cycle 1
	// sees headroom=0 and fires Provision A; cycle 2 (15-30s later,
	// A still booting since EC2+image pull takes 60-90s) STILL sees
	// readyCapacity=2/readyDemand=2/headroom=0 and fires Provision B.
	// The deficit gets double-counted because future-but-not-yet-Ready
	// capacity is invisible to the math.
	provisioningCapacity := 0
	for _, r := range rows {
		if isAvailable(r) {
			available++
		}
		if isAliveForFloor(r) {
			aliveForFloor++
		}
		if isReadyAndOnline(r) {
			readyOnlineCount++
			readyCapacity += int(r.MaxSandboxes)
			readyDemand += int(r.ActiveSandboxes)
		}
		if State(r.ComputeState) == StateProvisioning {
			provisioningCapacity += int(r.MaxSandboxes)
		}
	}

	// Floor pressure: how many short of Floor are we? Floor is the
	// operator's guarantee of HEALTHY capacity, so Ready+offline rows
	// (whose YD WR is dying and waiting for D4 to shed) do not count -
	// see isAliveForFloor for the rationale. The Max ceiling further
	// down still uses `available` so we don't double-provision during
	// D4's shed cycle.
	floorNeed := m.cfg.Floor - aliveForFloor
	if floorNeed < 0 {
		floorNeed = 0
	}

	// Demand pressure (D3): fires only when there's at least one
	// REACHABLE Ready+online host to base the demand judgement on.
	//
	// Why "at least one online host" and not "Floor satisfied":
	//   - Cold boot (Floor=1, no hosts): readyOnlineCount=0 -> blocked.
	//     Floor pressure fires instead. Once the first host comes
	//     online, the gate flips and D3 can act. Prevents the original
	//     cold-boot over-provisioning bug.
	//   - Heartbeat flap (Floor=2, 1 host briefly offline):
	//     readyOnlineCount=1, gate is true, D3 continues to act on
	//     the remaining online host's headroom. The previous
	//     "readyCount >= Floor" gate would have disabled D3 here,
	//     blocking scale-up exactly when needed.
	//   - Floor=0 cold boot: readyOnlineCount=0 -> blocked. Without
	//     a Ready host to measure demand against, D3 has no signal.
	//     Operators wanting "true cold-start scale on first request"
	//     need either Floor>=1 or an event-driven provisioning path
	//     (not implemented; would be a follow-up).
	//
	// demandNeed batches up to MaxConcurrentProvisions when the slot
	// shortage is large, but never exceeds the actual shortage (so
	// 1-slot pressure doesn't spawn N hosts). The earlier
	// SpecTemplate-based ceil-div was unsafe because SpecTemplate is
	// always zero-valued in production (bootstrap doesn't populate
	// it), so the fallback constant 20 was always used - under-
	// provisioning 4x on smaller-capacity hosts.
	demandNeed := 0
	if m.cfg.Max > m.cfg.Floor && readyOnlineCount > 0 {
		// Committed capacity: Ready+online (real serving) PLUS in-flight
		// provisioning (will serve soon). Both contribute slots toward
		// the headroom we already have on order. The readyOnlineCount
		// gate above still uses ONLY real-online so D3 doesn't fire
		// with zero serving capacity - but for the deficit math, count
		// committed slots so we don't double-fire across cycles while
		// a Provision is still in flight.
		headroom := readyCapacity + provisioningCapacity - readyDemand
		if headroom < m.cfg.ScaleUpHeadroomMin {
			slotsShort := m.cfg.ScaleUpHeadroomMin - headroom
			demandNeed = slotsShort
			if demandNeed > m.cfg.MaxConcurrentProvisions {
				demandNeed = m.cfg.MaxConcurrentProvisions
			}
			if demandNeed < 1 {
				demandNeed = 1
			}
		}
	}

	totalNeed := floorNeed + demandNeed

	// Hard ceiling: never let total owned (Ready + Provisioning + this
	// cycle's plans) exceed Max. Max=0 means "unbounded except by
	// Floor" - i.e. D3 disabled, only Floor provisions.
	if m.cfg.Max > 0 {
		room := m.cfg.Max - available
		if room < 0 {
			room = 0
		}
		if totalNeed > room {
			totalNeed = room
		}
	}

	// Per-cycle provisioning fan-out cap (D1/D2 behaviour preserved).
	if totalNeed > m.cfg.MaxConcurrentProvisions {
		totalNeed = m.cfg.MaxConcurrentProvisions
	}
	return totalNeed
}

// ownedRows returns all SandboxInstance rows that belong to this
// Manager's Provider. Rows with empty Provider are legacy
// self-registered hosts and are NOT touched by the Manager.
func (m *Manager) ownedRows(ctx context.Context) ([]*types.SandboxInstance, error) {
	all, err := m.store.ListSandboxInstances(ctx)
	if err != nil {
		return nil, err
	}
	pname := m.provider.Name()
	out := make([]*types.SandboxInstance, 0, len(all))
	for _, r := range all {
		if r.Provider == pname {
			out = append(out, r)
		}
	}
	return out, nil
}

// refreshProvisioning iterates rows in ComputeState=provisioning and
// either (a) persists a real state transition observed via
// Provider.HealthCheck, or (b) rolls back the row if its provisioning
// has been in flight for longer than MaxProvisioningAge.
//
// The age-based rollback is what prevents the silent-degradation
// failure mode: without it, a row whose upstream task is stuck
// (image pull failure, scheduler hang, capacity exhausted) sits in
// 'provisioning' forever and continues to count toward the Floor,
// so the Manager believes it has capacity it does not.
func (m *Manager) refreshProvisioning(ctx context.Context, rows []*types.SandboxInstance) {
	now := time.Now()
	for _, r := range rows {
		if r.ComputeState != string(StateProvisioning) {
			continue
		}

		// (a) Age-based rollback. We check this BEFORE HealthCheck so
		// a row stuck at ProviderID="" (upstream Provision never
		// returned an id, or the second Save lost a race) still gets
		// cleaned up rather than living forever.
		if r.ProvisionedAt != nil && now.Sub(*r.ProvisionedAt) > m.cfg.MaxProvisioningAge {
			m.rollbackStuckRow(ctx, r, "exceeded MaxProvisioningAge")
			continue
		}

		if r.ProviderID == "" {
			// Mid-create from a previous cycle whose persist hadn't
			// landed yet. The age check above eventually catches
			// genuinely-orphaned rows; for now, skip this cycle.
			continue
		}

		hcCtx, cancel := context.WithTimeout(ctx, m.cfg.HealthCheckTimeout)
		handle := &Handle{
			ProviderName: r.Provider,
			ProviderID:   r.ProviderID,
			SandboxID:    r.ID,
		}
		err := m.provider.HealthCheck(hcCtx, handle)
		cancel()

		// HealthCheck failure with no state set: log and move on. The
		// row stays in provisioning; next cycle retries. If the
		// upstream is permanently gone, MaxProvisioningAge eventually
		// catches it.
		if handle.State == "" {
			if err != nil {
				log.Debug().
					Err(err).
					Str("sandbox_id", r.ID).
					Str("provider_id", r.ProviderID).
					Msg("compute HealthCheck returned no state; will retry next cycle")
			}
			continue
		}

		newState := string(handle.State)
		if newState == r.ComputeState {
			continue
		}

		// Persist the real ComputeState transition. Use the targeted
		// column update so we don't race the heartbeat path that
		// owns Status/LastSeen.
		if updErr := m.store.UpdateSandboxInstanceComputeState(ctx, r.ID, newState); updErr != nil {
			log.Warn().
				Err(updErr).
				Str("sandbox_id", r.ID).
				Str("new_state", newState).
				Msg("compute manager failed to persist ComputeState transition")
			continue
		}

		// Mirror the unambiguous transitions onto Status so the rest
		// of Helix's existing online/offline machinery agrees.
		switch handle.State {
		case StateReady:
			_ = m.store.UpdateSandboxInstanceStatus(ctx, r.ID, "online")
		case StateFailed, StateTerminated:
			_ = m.store.UpdateSandboxInstanceStatus(ctx, r.ID, "offline")
		}

		log.Info().
			Str("sandbox_id", r.ID).
			Str("provider_id", r.ProviderID).
			Str("from", r.ComputeState).
			Str("to", newState).
			Msg("compute manager observed sandbox state transition")
	}
}

// rollbackStuckRow best-effort cancels the upstream resource and then
// removes the SandboxInstance row. Used by refreshProvisioning's
// age-based timeout path and by provisionOne's persist-failure path.
// Idempotent: if the row is already gone, succeeds silently.
func (m *Manager) rollbackStuckRow(ctx context.Context, r *types.SandboxInstance, reason string) {
	if r.ProviderID != "" {
		handle := &Handle{
			ProviderName: r.Provider,
			ProviderID:   r.ProviderID,
			SandboxID:    r.ID,
		}
		// Force=true: the row was stuck precisely because graceful
		// drain wasn't happening. No reason to wait now.
		if err := m.provider.Deprovision(ctx, handle, DeprovisionOpts{Force: true, Reason: reason}); err != nil {
			log.Warn().
				Err(err).
				Str("sandbox_id", r.ID).
				Str("provider_id", r.ProviderID).
				Str("reason", reason).
				Msg("compute manager Deprovision during rollback failed; row will be removed anyway")
		}
	}
	if err := m.store.DeregisterSandboxInstance(ctx, r.ID); err != nil {
		log.Warn().
			Err(err).
			Str("sandbox_id", r.ID).
			Str("reason", reason).
			Msg("compute manager failed to deregister stuck row; next cycle will retry")
		return
	}
	log.Info().
		Str("sandbox_id", r.ID).
		Str("provider_id", r.ProviderID).
		Str("reason", reason).
		Msg("compute manager rolled back stuck sandbox row")
}

// provisionOne pre-decides the SandboxID, inserts a stub row in
// compute_state=provisioning, calls Provider.Provision, then persists
// the upstream ID onto the row. Any failure rolls back both Helix-side
// state (the row) and upstream state (the Provider task) so we don't
// leak resources.
//
// The pre-decided SandboxID is the bridge that lets a registering
// host claim a pre-existing row (instead of inserting a new one):
// the upstream task is launched with HELIX_SANDBOX_ID=<this id>, the
// helix-sandbox container reads it on startup, and the auto-register
// handler in api/pkg/server/server.go matches it against this row.
// The Manager populates the row's identifying fields here so the
// bridge can promote it without schema churn.
func (m *Manager) provisionOne(ctx context.Context) error {
	sandboxID := newSandboxID()
	// Defence-in-depth: refuse to proceed if the generator ever drifts
	// to something that could escape the shell when a downstream
	// bash-script interpolates HELIX_SANDBOX_ID. UUIDs are safe; this
	// guards against future changes to newSandboxID.
	if !sandboxIDPattern.MatchString(sandboxID) {
		return fmt.Errorf("compute: generated sandbox id %q does not match %s", sandboxID, sandboxIDPattern)
	}
	now := time.Now().UTC()

	stub := &types.SandboxInstance{
		ID:            sandboxID,
		Hostname:      fmt.Sprintf("sandbox-%s", sandboxID),
		Status:        "offline", // becomes "online" when host registers
		Provider:      m.provider.Name(),
		ComputeState:  string(StateProvisioning),
		ProvisionedAt: &now,
		MaxSandboxes:  defaultMaxSandboxes(m.cfg.SpecTemplate),
	}
	if err := m.store.RegisterSandboxInstance(ctx, stub); err != nil {
		return fmt.Errorf("insert stub row: %w", err)
	}

	spec := m.cfg.SpecTemplate
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	// The Provider passes this label through to the task environment,
	// where the bash script reads it and launches helix-sandbox with
	// the matching ID. The bash script integration is downstream
	// work; for now the label travels and is ignored if no script
	// references it.
	spec.Labels["helix.sandbox_id"] = sandboxID

	handle, err := m.provider.Provision(ctx, spec)
	if err != nil {
		// Upstream rejected the request: clean up the Helix-side row
		// only. There is nothing to deprovision upstream.
		if delErr := m.store.DeregisterSandboxInstance(ctx, sandboxID); delErr != nil {
			log.Warn().
				Err(delErr).
				Str("sandbox_id", sandboxID).
				Msg("compute manager failed to roll back stub row after Provision failure; refreshProvisioning will catch it via MaxProvisioningAge")
		}
		return fmt.Errorf("provider Provision: %w", err)
	}

	// Persist the upstream id with a TARGETED column update. Going
	// through RegisterSandboxInstance again would be an upsert that
	// races the auto-register path: if a host registered between
	// these two writes, Save would overwrite its fresh heartbeat
	// fields with stale defaults from the stub.
	if updErr := m.store.UpdateSandboxInstanceProviderID(ctx, sandboxID, handle.ProviderID); updErr != nil {
		// Helix-side persist failed but upstream accepted. We MUST
		// roll back the upstream resource or it'll burn cloud spend
		// forever with nothing tracking it. The Helix-side row also
		// goes; otherwise refreshProvisioning would short-circuit on
		// empty ProviderID and the row would only be cleaned up by
		// MaxProvisioningAge (15m of wasted upstream resource).
		log.Error().
			Err(updErr).
			Str("sandbox_id", sandboxID).
			Str("provider_id", handle.ProviderID).
			Msg("compute manager failed to persist provider_id; rolling back upstream resource and Helix row")

		if depErr := m.provider.Deprovision(ctx, handle, DeprovisionOpts{Force: true, Reason: "rollback: failed to persist provider_id"}); depErr != nil {
			log.Warn().
				Err(depErr).
				Str("sandbox_id", sandboxID).
				Str("provider_id", handle.ProviderID).
				Msg("compute manager Deprovision during rollback failed; upstream may have orphan")
		}
		if delErr := m.store.DeregisterSandboxInstance(ctx, sandboxID); delErr != nil {
			log.Warn().
				Err(delErr).
				Str("sandbox_id", sandboxID).
				Msg("compute manager failed to deregister stuck row after persist failure")
		}
		return fmt.Errorf("persist provider_id: %w", updErr)
	}

	log.Info().
		Str("sandbox_id", sandboxID).
		Str("provider", m.provider.Name()).
		Str("provider_id", handle.ProviderID).
		Msg("compute manager provisioned new sandbox host")
	return nil
}

// isAvailable reports whether a row counts as "owned by us, do not
// double-provision while D4 sheds." Used for the Max ceiling, NOT for
// the Floor calculation - see isAliveForFloor for that.
//
// Ready rows count (host is up and registered). Provisioning rows
// also count (host is on its way) so we don't double-provision while
// the first one is still booting. Failed and Terminated rows do not
// count - they are dead and should be cleaned up by a follow-up
// idle-deprovision pass.
//
// Note: Ready+offline rows count here even though they aren't
// session-routable, because they're still consuming a YD WR (and the
// AWS EC2 instance underneath it) until D4 sheds them. Provisioning
// a replacement on top would temporarily push us past Max.
func isAvailable(r *types.SandboxInstance) bool {
	switch State(r.ComputeState) {
	case StateReady, StateProvisioning:
		return true
	default:
		return false
	}
}

// isAliveForFloor reports whether a row should count toward the Floor.
//
// Unlike isAvailable (which is owner-scoped), this is health-scoped:
// the Floor is the operator's guarantee of available capacity for
// sandbox sessions, so an offline row - even one we own - does NOT
// satisfy it. The sandbox-instance reaper (server/sandbox_instance_reaper.go)
// flips status to "offline" once last_seen drifts past
// HELIX_SANDBOX_STALE_THRESHOLD (default 5m). At that point the
// underlying compute (YD WR + EC2) is either dead or unreachable, so
// counting it toward Floor leaves the operator below their requested
// healthy-capacity guarantee for an unbounded amount of time (until
// D4 happens to shed it).
//
// Provisioning rows still count - the host is on its way and the
// next cycle will see it Ready, so firing another Provision now would
// be the same double-provision-during-boot bug isAvailable was
// originally written to prevent.
//
// Why a separate predicate, not a tighter isAvailable:
//
//	A previous tightening of isAvailable to require Status=="online"
//	broke the Max ceiling: D4's shed cycle takes one or two reconciles
//	to run, and during that window the offline-Ready row stops counting
//	against Max - so Reconcile fires a fresh Provision, then D4 fires
//	a Deprovision, and we churn. Keeping isAvailable broad for the
//	ceiling and using this narrower predicate for the Floor gets both
//	behaviours: prompt replacement of dead hosts, no churn while D4
//	is in flight.
//
// The reaper's 5-minute stale threshold IS the heartbeat-flap defence:
// by the time status flips to "offline" the row has already missed
// every heartbeat in that window. A real flap (one missed beat) is
// well inside the window and never triggers this.
func isAliveForFloor(r *types.SandboxInstance) bool {
	// Compose from isReadyAndOnline rather than re-inlining the
	// (ready AND online) check so future tightening of one predicate
	// propagates automatically instead of silently drifting.
	return State(r.ComputeState) == StateProvisioning || isReadyAndOnline(r)
}

// isReadyState reports whether the row's ComputeState is Ready, ignoring
// Status. Used by D4 to identify shed candidates (Ready+offline rows
// should also be shed - they're a form of orphan) and to prune the
// idleSince tracker (a host that flaps offline briefly should NOT lose
// its accumulated idle time when Status flickers back online).
func isReadyState(r *types.SandboxInstance) bool {
	return State(r.ComputeState) == StateReady
}

// isReadyAndOnline reports whether the row represents a host that is
// REACHABLE for sandbox-session traffic - both Ready (registered) and
// online (heartbeat fresh). Used by D3 for capacity and demand math:
// Ready+offline rows must NOT contribute to readyCapacity or
// readyDemand (they can't accept new sessions), but they MAY still be
// owned by us and waiting for D4 to shed them.
//
// The split between isReadyState (broad) and isReadyAndOnline (narrow)
// is the ultrareview-fixup-of-the-fixup: an earlier attempt collapsed
// both into a single tightened isReady, which broke D4's idle-tracker
// prune on heartbeat flap (entry deleted -> next cycle re-armed at
// NOW -> chronic-idle host never crossed IdleTimeout).
func isReadyAndOnline(r *types.SandboxInstance) bool {
	return State(r.ComputeState) == StateReady && r.Status == "online"
}

// newSandboxID returns a new globally-unique sandbox ID. Used as both
// the SandboxInstance.ID and the env var the YD task reads on startup.
// We use a "sbx_" prefix so SandboxIDs are visually distinguishable
// from other ID types in logs and the admin UI.
//
// Source of randomness: google/uuid.New() uses crypto/rand (panics if
// unavailable, never falls back to math/rand). 122 bits of entropy.
func newSandboxID() string {
	return "sbx_" + uuid.New().String()
}

// sandboxIDPattern guards against a future newSandboxID() change that
// could produce values unsafe to interpolate into a shell command.
// Asserted in provisionOne before the id leaves the package.
var sandboxIDPattern = regexp.MustCompile(`^sbx_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// defaultMaxSandboxes picks the per-host inner-desktop capacity from
// the spec, falling back to 20 (matching server.go's legacy default).
func defaultMaxSandboxes(spec Spec) int {
	if spec.MaxSandboxes > 0 {
		return spec.MaxSandboxes
	}
	return 20
}
