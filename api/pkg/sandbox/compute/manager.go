package compute

import (
	"context"
	"errors"
	"fmt"
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
// if provider or store is nil, or cfg is invalid.
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
// Idempotent: calling Reconcile twice in a row should at most kick
// off (extras_needed_first_call - extras_needed_second_call)
// additional Provision calls. The Manager never tries to "catch up"
// by submitting multiple Provisions per cycle; it submits at most
// one per cycle so a slow Provider can't be hammered.
func (m *Manager) Reconcile(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rows, err := m.ownedRows(ctx)
	if err != nil {
		return fmt.Errorf("list owned rows: %w", err)
	}

	// First pass: refresh provisioning rows. We do this before counting
	// available capacity so a host that just transitioned to Ready
	// counts toward the Floor in this same cycle.
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

	if available < m.cfg.Floor {
		// One Provision per cycle. The next cycle reassesses; if the
		// previous one is still in StateProvisioning, available will
		// have incremented and we may not need another.
		if err := m.provisionOne(ctx); err != nil {
			return fmt.Errorf("provision: %w", err)
		}
	}
	return nil
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

// refreshProvisioning calls HealthCheck on each row in
// compute_state=provisioning and updates the row's ComputeState in
// the store. Best effort - if HealthCheck fails (network blip,
// transient Provider error) the row stays in 'provisioning' and the
// next cycle retries.
func (m *Manager) refreshProvisioning(ctx context.Context, rows []*types.SandboxInstance) {
	for _, r := range rows {
		if r.ComputeState != string(StateProvisioning) {
			continue
		}
		if r.ProviderID == "" {
			// Row missing its upstream id - probably mid-create from a
			// previous cycle. Skip; next reconcile will catch up if
			// the Provision call eventually populated it.
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
		newState := string(handle.State)
		if newState == "" {
			continue
		}
		if newState == r.ComputeState {
			continue
		}
		// Persist the transition. We piggy-back on the existing
		// UpdateSandboxInstanceStatus method by mapping ComputeState
		// changes to the row's Status field too where the mapping is
		// unambiguous (Ready -> online, Failed/Terminated -> offline).
		// A dedicated UpdateSandboxInstanceComputeState method lands
		// in D2; for now we use what's there.
		switch handle.State {
		case StateReady:
			_ = m.store.UpdateSandboxInstanceStatus(ctx, r.ID, "online")
		case StateFailed, StateTerminated:
			_ = m.store.UpdateSandboxInstanceStatus(ctx, r.ID, "offline")
		}
		if err != nil {
			log.Debug().
				Err(err).
				Str("sandbox_id", r.ID).
				Str("provider_id", r.ProviderID).
				Str("new_state", newState).
				Msg("compute HealthCheck reported state change with error")
		}
	}
}

// provisionOne pre-decides the SandboxID, inserts a stub row in
// compute_state=provisioning, then calls Provider.Provision. On
// success the row is updated with the upstream provider_id. On
// failure the stub row is rolled back.
//
// The pre-decided SandboxID is the bridge that lets a registering
// host claim a pre-existing row (instead of inserting a new one):
// the YD task is launched with HELIX_SANDBOX_ID=<this id>, the
// helix-sandbox container reads it on startup, and the auto-register
// handler in api/server.go matches it against this row. That bridge
// lands in D2; the Manager populates the right field here so D2 can
// flip the switch without schema churn.
func (m *Manager) provisionOne(ctx context.Context) error {
	sandboxID := newSandboxID()
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
		// Roll back the stub row so we don't accumulate orphans.
		_ = m.store.DeregisterSandboxInstance(ctx, sandboxID)
		return fmt.Errorf("provider Provision: %w", err)
	}

	// Persist the upstream id so future Reconciles can look this
	// row's status up via HealthCheck. The existing store interface
	// doesn't have a typed method for this; we leverage Register's
	// upsert semantics, which is hacky but contained. Replaced with
	// a typed UpdateSandboxInstanceProvider method in D2.
	stub.ProviderID = handle.ProviderID
	if err := m.store.RegisterSandboxInstance(ctx, stub); err != nil {
		log.Error().
			Err(err).
			Str("sandbox_id", sandboxID).
			Str("provider_id", handle.ProviderID).
			Msg("failed to persist provider_id on stub row; reconciler will reconcile via List() next cycle")
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
func newSandboxID() string {
	return "sbx_" + uuid.New().String()
}

// defaultMaxSandboxes picks the per-host inner-desktop capacity from
// the spec, falling back to 20 (matching server.go's legacy default).
func defaultMaxSandboxes(spec Spec) int {
	if spec.MaxSandboxes > 0 {
		return spec.MaxSandboxes
	}
	return 20
}
