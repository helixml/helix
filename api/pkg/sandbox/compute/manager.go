package compute

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
	// this count. Set to 0 to disable pre-warming (the Manager then
	// behaves on-demand only - feature added in D3).
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
}

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
	return &Manager{
		provider: provider,
		store:    store,
		cfg:      cfg,
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

	available := 0
	for _, r := range rows {
		if isAvailable(r) {
			available++
		}
	}

	needed := m.cfg.Floor - available
	if needed <= 0 {
		return nil
	}
	if needed > m.cfg.MaxConcurrentProvisions {
		needed = m.cfg.MaxConcurrentProvisions
	}
	// Each Provision is independent; we surface the first error but
	// keep going so a transient upstream blip doesn't permanently
	// stall the cluster at a partial fill.
	var firstErr error
	for i := 0; i < needed; i++ {
		if err := m.provisionOne(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("provision: %w", err)
		}
	}
	return firstErr
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
// handler in api/server.go matches it against this row. That bridge
// lands in D2; the Manager populates the right field here so D2 can
// flip the switch without schema churn.
func (m *Manager) provisionOne(ctx context.Context) error {
	sandboxID := newSandboxID()
	// Defence-in-depth: refuse to proceed if the generator ever drifts
	// to something that could escape the shell when D2's bash-script
	// interpolates HELIX_SANDBOX_ID. UUIDs are safe; this guards
	// against future changes to newSandboxID.
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
	// where bash-script.sh reads it and launches helix-sandbox with
	// the matching ID. Wired up in D2.
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
	// these two writes (possible in D2), Save would overwrite its
	// fresh heartbeat fields with stale defaults from the stub.
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

// isAvailable reports whether a row counts toward the Floor.
//
// Ready rows count (host is up and registered). Provisioning rows
// also count (host is on its way) so we don't double-provision while
// the first one is still booting. Failed and Terminated rows do not
// count - they are dead and should be cleaned up (D4).
func isAvailable(r *types.SandboxInstance) bool {
	switch State(r.ComputeState) {
	case StateReady, StateProvisioning:
		return true
	default:
		return false
	}
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
