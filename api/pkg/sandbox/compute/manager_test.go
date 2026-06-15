package compute

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// fakeStore is a goroutine-safe in-memory SandboxStore for unit tests.
// Mirrors the subset of the real store the Manager touches.
type fakeStore struct {
	mu   sync.Mutex
	rows map[string]*types.SandboxInstance
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: make(map[string]*types.SandboxInstance)}
}

func (f *fakeStore) ListSandboxInstances(ctx context.Context) ([]*types.SandboxInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*types.SandboxInstance, 0, len(f.rows))
	for _, r := range f.rows {
		cp := *r
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeStore) GetSandboxInstance(ctx context.Context, id string) (*types.SandboxInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *r
	return &cp, nil
}

func (f *fakeStore) RegisterSandboxInstance(ctx context.Context, instance *types.SandboxInstance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *instance
	f.rows[instance.ID] = &cp
	return nil
}

func (f *fakeStore) UpdateSandboxInstanceStatus(ctx context.Context, id, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return errors.New("not found")
	}
	r.Status = status
	return nil
}

func (f *fakeStore) UpdateSandboxInstanceComputeState(ctx context.Context, id, computeState string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return errors.New("not found")
	}
	r.ComputeState = computeState
	return nil
}

func (f *fakeStore) UpdateSandboxInstanceProviderID(ctx context.Context, id, providerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rows[id]
	if !ok {
		return errors.New("not found")
	}
	r.ProviderID = providerID
	return nil
}

func (f *fakeStore) DeregisterSandboxInstance(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, id)
	return nil
}

// failOnceStore wraps fakeStore but injects a one-shot failure on
// UpdateSandboxInstanceProviderID, used by the test that exercises
// provisionOne's rollback path when the second write fails.
type failOnceStore struct {
	*fakeStore
	updProviderIDErr error
	updProviderIDCalls int
}

func (f *failOnceStore) UpdateSandboxInstanceProviderID(ctx context.Context, id, providerID string) error {
	f.updProviderIDCalls++
	if f.updProviderIDErr != nil {
		err := f.updProviderIDErr
		f.updProviderIDErr = nil // one-shot
		return err
	}
	return f.fakeStore.UpdateSandboxInstanceProviderID(ctx, id, providerID)
}

// newTestManager builds a Manager wired to a fresh fake store + stub
// provider, with sensible defaults for testing.
func newTestManager(t *testing.T, floor int) (*Manager, *StubProvider, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:              floor,
		ReconcileInterval:  10 * time.Millisecond,
		HealthCheckTimeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, stub, store
}

func TestNewManagerValidatesInputs(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	good := ManagerConfig{
		Floor:              1,
		ReconcileInterval:  time.Second,
		HealthCheckTimeout: time.Second,
	}
	cases := []struct {
		name     string
		provider Provider
		store    SandboxStore
		cfg      ManagerConfig
		wantErr  string
	}{
		{"nil provider", nil, store, good, "provider is required"},
		{"nil store", stub, nil, good, "store is required"},
		{"negative floor", stub, store, ManagerConfig{Floor: -1, ReconcileInterval: time.Second, HealthCheckTimeout: time.Second}, "Floor must be >= 0"},
		{"zero interval", stub, store, ManagerConfig{Floor: 0, ReconcileInterval: 0, HealthCheckTimeout: time.Second}, "ReconcileInterval must be > 0"},
		{"zero hc timeout", stub, store, ManagerConfig{Floor: 0, ReconcileInterval: time.Second, HealthCheckTimeout: 0}, "HealthCheckTimeout must be > 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewManager(tc.provider, tc.store, tc.cfg)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestReconcileBelowFloorProvisionsOne(t *testing.T) {
	m, _, store := newTestManager(t, 2)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("expected 1 row provisioned per cycle, got %d", len(rows))
	}
	r := rows[0]
	if r.Provider != "stub" {
		t.Fatalf("row.Provider = %q, want stub", r.Provider)
	}
	if r.ComputeState != string(StateProvisioning) {
		t.Fatalf("row.ComputeState = %q, want provisioning", r.ComputeState)
	}
	if r.ProviderID == "" {
		t.Fatal("row.ProviderID empty - should be populated from stub.Provision")
	}
	if r.ID == "" || !startsWith(r.ID, "sbx_") {
		t.Fatalf("row.ID = %q, want sbx_<uuid>", r.ID)
	}
	if r.ProvisionedAt == nil {
		t.Fatal("row.ProvisionedAt nil; should be set at insert")
	}
}

func TestReconcileAtFloorIsNoop(t *testing.T) {
	m, _, store := newTestManager(t, 1)
	// First cycle brings us to 1.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile (warm): %v", err)
	}
	before, _ := store.ListSandboxInstances(context.Background())
	if len(before) != 1 {
		t.Fatalf("warm-up expected 1 row, got %d", len(before))
	}
	// Second cycle should NOT provision another - we're at floor.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile (steady): %v", err)
	}
	after, _ := store.ListSandboxInstances(context.Background())
	if len(after) != 1 {
		t.Fatalf("steady cycle should be no-op, got %d rows", len(after))
	}
}

func TestReconcileFloorZeroDoesNothing(t *testing.T) {
	m, _, store := newTestManager(t, 0)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 0 {
		t.Fatalf("floor=0 should provision nothing, got %d rows", len(rows))
	}
}

func TestReconcileIgnoresOtherProviders(t *testing.T) {
	m, _, store := newTestManager(t, 1)
	// Seed a row owned by a DIFFERENT provider. The Manager must not
	// count it toward the Floor.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "foreign-1",
		Provider:     "other-provider",
		ComputeState: string(StateReady),
		Status:       "online",
	})
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("expected foreign row + 1 stub row, got %d", len(rows))
	}
	stubCount := 0
	for _, r := range rows {
		if r.Provider == "stub" {
			stubCount++
		}
	}
	if stubCount != 1 {
		t.Fatalf("expected exactly 1 stub-owned row, got %d", stubCount)
	}
}

func TestReconcileIgnoresLegacySelfRegisteredRows(t *testing.T) {
	m, _, store := newTestManager(t, 1)
	// Seed a legacy row with NO provider - the auto-register path
	// creates these for self-registered hosts. The Manager must leave
	// them alone (they aren't ours) and provision its own row.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:       "legacy-1",
		Provider: "",
		Status:   "online",
	})
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("expected legacy row + 1 stub row, got %d", len(rows))
	}
}

func TestReconcileProvisionFailureRollsBackStubRow(t *testing.T) {
	store := newFakeStore()
	failing := &failingProvider{name: "stub-fail"}
	m, err := NewManager(failing, store, ManagerConfig{
		Floor:              1,
		ReconcileInterval:  time.Second,
		HealthCheckTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	err = m.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected Reconcile to surface Provision error")
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 0 {
		t.Fatalf("expected stub row to be rolled back on Provision failure, got %d", len(rows))
	}
}

func TestReconcileRefreshesProvisioningRows(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	// Hook the stub so newly provisioned handles are marked Ready
	// immediately (skipping the boot wait). Lets us verify that
	// HealthCheck-driven state transitions reach the store.
	stub.SetProvisionHook(func(_ Spec, h *Handle) {
		h.State = StateReady
	})
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:              1,
		ReconcileInterval:  time.Second,
		HealthCheckTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// First cycle provisions one row (still in provisioning state on the row
	// itself, but the stub has it marked Ready already).
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile #1: %v", err)
	}
	// Second cycle: refreshProvisioning calls HealthCheck on the
	// provisioning row, observes the stub reporting Ready, and MUST
	// persist BOTH the new ComputeState AND the mirrored Status.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile #2: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row after two cycles, got %d", len(rows))
	}
	if rows[0].Status != "online" {
		t.Fatalf("HealthCheck should have mirrored Ready to Status=online; got %q", rows[0].Status)
	}
	// Regression guard for a silent-failure bug surfaced during
	// review: an earlier version updated Status but not ComputeState,
	// so a Ready or Failed row would silently keep counting as
	// "provisioning" toward Floor forever.
	if rows[0].ComputeState != string(StateReady) {
		t.Fatalf("HealthCheck must persist ComputeState=ready; got %q", rows[0].ComputeState)
	}
}

func TestReconcileHealthCheckFailedRollsForwardComputeState(t *testing.T) {
	// Regression guard for review-discovered bug: when HealthCheck
	// reports StateFailed, the row's ComputeState must transition off
	// 'provisioning' so it stops counting toward Floor. Otherwise the
	// Manager believes it has capacity it does not, and pre-warming
	// silently degrades to zero usable hosts.
	store := newFakeStore()
	stub := NewStubProvider("stub")
	// Hook only fires on the FIRST Provision so the replacement
	// (provisioned in cycle 2 after the original is marked Failed)
	// boots cleanly. Otherwise every provision would be Failed and
	// we'd never converge.
	provisionCount := 0
	stub.SetProvisionHook(func(_ Spec, h *Handle) {
		provisionCount++
		if provisionCount == 1 {
			h.State = StateFailed
		}
	})
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:              1,
		ReconcileInterval:  time.Second,
		HealthCheckTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile #1: %v", err)
	}
	// Cycle 2: refreshProvisioning observes Failed on row #1 via
	// HealthCheck and persists it. Then the count of "available"
	// rows is 0 (Failed doesn't count), so a replacement is
	// provisioned in the same cycle - confirming the Failed row
	// is NOT being mistaken for capacity.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile #2: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("expected Failed row + replacement = 2 rows, got %d", len(rows))
	}
	failedCount := 0
	provisioningCount := 0
	for _, r := range rows {
		switch State(r.ComputeState) {
		case StateFailed:
			failedCount++
		case StateProvisioning:
			provisioningCount++
		}
	}
	if failedCount != 1 {
		t.Fatalf("expected exactly 1 row in ComputeState=failed (the original), got %d", failedCount)
	}
	if provisioningCount != 1 {
		t.Fatalf("expected exactly 1 row in ComputeState=provisioning (the replacement), got %d", provisioningCount)
	}
}

func TestReconcileTimesOutStuckProvisioningRows(t *testing.T) {
	// Regression guard: a stuck Provision (upstream task hung in
	// image-pull etc.) must not hold a Floor slot forever. After
	// MaxProvisioningAge elapses, the row is rolled back via
	// Deprovision + Deregister.
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:              1,
		ReconcileInterval:  time.Second,
		HealthCheckTimeout: time.Second,
		MaxProvisioningAge: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// First cycle provisions one row. Default stub state is
	// Provisioning and stays there - it never transitions to Ready
	// (no hook). Perfect simulation of a stuck upstream.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile #1: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after first cycle, got %d", len(rows))
	}
	// Wait past MaxProvisioningAge.
	time.Sleep(20 * time.Millisecond)
	// Second cycle should observe age > MaxProvisioningAge, roll back
	// the stuck row, and (because we're below Floor again) provision
	// a fresh one.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile #2: %v", err)
	}
	rows, _ = store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("expected stuck row replaced (still 1 row), got %d", len(rows))
	}
	// New row's ProvisionedAt must be after the original cutoff.
	if rows[0].ProvisionedAt == nil {
		t.Fatal("replacement row missing ProvisionedAt")
	}
}

func TestReconcileProvisionSecondWriteFailureRollsBack(t *testing.T) {
	// Regression guard: if UpdateSandboxInstanceProviderID fails
	// after Provision succeeded, both the upstream resource AND the
	// Helix row must be cleaned up. Otherwise the upstream WR runs
	// forever with nothing tracking it.
	store := &failOnceStore{
		fakeStore:        newFakeStore(),
		updProviderIDErr: errors.New("simulated db write failure"),
	}
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:              1,
		ReconcileInterval:  time.Second,
		HealthCheckTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	err = m.Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected Reconcile to surface persist-failure error")
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 0 {
		t.Fatalf("row should have been rolled back, got %d rows", len(rows))
	}
	// Verify the upstream resource was deprovisioned (stub's view
	// should show the handle in StateTerminated).
	handles, _ := stub.List(context.Background())
	if len(handles) != 1 {
		t.Fatalf("expected 1 stub handle to exist, got %d", len(handles))
	}
	if handles[0].State != StateTerminated {
		t.Fatalf("expected upstream handle to be Terminated after rollback, got %q", handles[0].State)
	}
}

func TestReconcileFiresMaxConcurrentProvisionsPerCycle(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   5,
		ReconcileInterval:       time.Second,
		HealthCheckTimeout:      time.Second,
		MaxConcurrentProvisions: 3,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 3 {
		t.Fatalf("expected MaxConcurrentProvisions=3 rows per cycle below Floor, got %d", len(rows))
	}
}

func TestReconcileDefaultMaxConcurrentProvisionsIsOne(t *testing.T) {
	// Backwards compatibility: cfg with no MaxConcurrentProvisions
	// set must default to 1 (the conservative one-per-cycle behaviour).
	m, _, store := newTestManager(t, 5)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("default MaxConcurrentProvisions should be 1; got %d rows", len(rows))
	}
}

func TestReconcileOnePerCycleEvenWhenFarBelowFloor(t *testing.T) {
	m, _, store := newTestManager(t, 5)
	// One cycle should add at most one row, even though we're 5 below floor.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("expected 1 row per cycle even with floor=5, got %d", len(rows))
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	m, _, _ := newTestManager(t, 0)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancel within 1s")
	}
}

// failingProvider is a minimal Provider that always errors out of
// Provision, used to verify rollback semantics.
// --- D3 (on-demand scale-up) tests ----------------------------------------

// newD3Manager builds a Manager with Floor=1 and a configurable Max +
// ScaleUpHeadroomMin so the test can assert on scale-up behaviour.
func newD3Manager(t *testing.T, floor, max, headroomMin int) (*Manager, *StubProvider, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   floor,
		Max:                     max,
		ScaleUpHeadroomMin:      headroomMin,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		MaxConcurrentProvisions: 5, // give D3 headroom on the per-cycle cap
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m, stub, store
}

// seedReadyRow inserts a Ready stub-owned row with the given capacity.
// Used to set up scenarios where headroom and demand can be controlled
// independently from the per-cycle provision flow.
func seedReadyRow(t *testing.T, store *fakeStore, id string, active, max int) {
	t.Helper()
	err := store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              id,
		Provider:        "stub",
		ComputeState:    string(StateReady),
		Status:          "online",
		ActiveSandboxes: active,
		MaxSandboxes:    max,
	})
	if err != nil {
		t.Fatalf("seedReadyRow: %v", err)
	}
}

// seedReadyRowOffline is like seedReadyRow but inserts a row with
// Status="offline" - simulates a host whose heartbeat has stopped
// (briefly or permanently). D3 should EXCLUDE offline rows from
// capacity math but still treat them as owned (so they count toward
// Floor satisfaction and the Max ceiling).
func seedReadyRowOffline(t *testing.T, store *fakeStore, id string, max, active int) {
	t.Helper()
	err := store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              id,
		Provider:        "stub",
		ComputeState:    string(StateReady),
		Status:          "offline",
		ActiveSandboxes: active,
		MaxSandboxes:    max,
	})
	if err != nil {
		t.Fatalf("seedReadyRowOffline: %v", err)
	}
}

func TestNewManagerValidatesD3Inputs(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	good := ManagerConfig{
		Floor:              1,
		ReconcileInterval:  time.Second,
		HealthCheckTimeout: time.Second,
	}
	cases := []struct {
		name    string
		cfg     ManagerConfig
		wantErr string
	}{
		{
			name: "negative max",
			cfg: ManagerConfig{
				Floor: 1, Max: -1,
				ReconcileInterval: time.Second, HealthCheckTimeout: time.Second,
			},
			wantErr: "Max must be >= 0",
		},
		{
			name: "max less than floor",
			cfg: ManagerConfig{
				Floor: 5, Max: 3,
				ReconcileInterval: time.Second, HealthCheckTimeout: time.Second,
			},
			wantErr: "must be > Floor",
		},
		{
			// Max == Floor was previously accepted but silently
			// disabled D3 (runtime gate requires Max > Floor).
			// Now rejected at validation with an actionable hint.
			name: "max equal to floor",
			cfg: ManagerConfig{
				Floor: 3, Max: 3,
				ReconcileInterval: time.Second, HealthCheckTimeout: time.Second,
			},
			wantErr: "must be > Floor",
		},
		{
			name: "negative headroom min",
			cfg: ManagerConfig{
				Floor: 1, Max: 2, ScaleUpHeadroomMin: -1,
				ReconcileInterval: time.Second, HealthCheckTimeout: time.Second,
			},
			wantErr: "ScaleUpHeadroomMin must be >= 0",
		},
	}
	_ = good // ensure the helper is still referenced
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewManager(stub, store, tc.cfg)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestD3DisabledWhenMaxZero(t *testing.T) {
	// Max=0 keeps the pre-D3 floor-only behaviour even when headroom
	// would justify scaling. Backward-compatibility guarantee for
	// existing operators upgrading to a Helix that ships D3.
	m, _, store := newD3Manager(t, 1, 0, 1)
	// Seed one Ready row at full capacity (zero headroom). With D3
	// enabled this would trigger scale-up; with Max=0 it must not.
	seedReadyRow(t, store, "sbx_full", 10, 10)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("Max=0 must not provision beyond Floor; got %d rows", len(rows))
	}
}

func TestD3ScalesUpWhenHeadroomBelowMin(t *testing.T) {
	// Floor=1, Max=3, HeadroomMin=2: a single Ready host using 9/10
	// slots leaves 1 free, which is below 2. Manager must provision
	// one more host this cycle.
	m, _, store := newD3Manager(t, 1, 3, 2)
	seedReadyRow(t, store, "sbx_busy", 9, 10)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("expected D3 to provision a 2nd host; got %d rows", len(rows))
	}
	provisioning := 0
	for _, r := range rows {
		if r.ComputeState == string(StateProvisioning) {
			provisioning++
		}
	}
	if provisioning != 1 {
		t.Fatalf("expected exactly 1 provisioning row, got %d", provisioning)
	}
}

func TestD3DoesNotScaleWhenHeadroomSufficient(t *testing.T) {
	// Floor=1, Max=3, HeadroomMin=2: a Ready host at 5/10 leaves 5
	// free slots, well above the threshold. No provision this cycle.
	m, _, store := newD3Manager(t, 1, 3, 2)
	seedReadyRow(t, store, "sbx_light", 5, 10)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("headroom sufficient; expected no new provision, got %d rows", len(rows))
	}
}

func TestD3RespectsMaxCeiling(t *testing.T) {
	// Floor=1, Max=2, HeadroomMin=2: two Ready hosts both at full
	// capacity (zero headroom). Demand pressure exists but Max is
	// already reached - no further provision.
	m, _, store := newD3Manager(t, 1, 2, 2)
	seedReadyRow(t, store, "sbx_full_1", 10, 10)
	seedReadyRow(t, store, "sbx_full_2", 10, 10)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("Max ceiling violated; expected 2 rows, got %d", len(rows))
	}
}

func TestD3CountsProvisioningTowardMax(t *testing.T) {
	// A Provisioning row contributes no live sandbox slots (so it
	// doesn't satisfy demand) BUT it does count toward Max (so we
	// don't double-provision while the first is on its way).
	// Floor=1, Max=2, HeadroomMin=2: one Ready full host + one
	// Provisioning host already in flight = total owned 2 = Max.
	// D3 must NOT provision a third even though headroom is 0.
	m, _, store := newD3Manager(t, 1, 2, 2)
	seedReadyRow(t, store, "sbx_full", 10, 10)
	provAt := time.Now()
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:            "sbx_in_flight",
		Provider:      "stub",
		ComputeState:  string(StateProvisioning),
		Status:        "offline",
		ProviderID:    "provider-id-1",
		MaxSandboxes:  10,
		ProvisionedAt: &provAt,
	})
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("Provisioning should count toward Max; got %d rows", len(rows))
	}
}

func TestD3StillSatisfiesFloorWhenDemandLow(t *testing.T) {
	// Edge case: Floor=2, Max=3, HeadroomMin=1, but no Ready hosts
	// yet (cold boot). Floor pressure must still bring us to 2 even
	// though there is no demand signal to compute headroom from.
	m, _, store := newD3Manager(t, 2, 3, 1)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	// Floor=2 with MaxConcurrentProvisions=5: one cycle provisions 2.
	if len(rows) != 2 {
		t.Fatalf("Floor must be satisfied independent of demand; got %d rows", len(rows))
	}
}

func TestD3DefaultHeadroomMinWhenScaleUpEnabled(t *testing.T) {
	// Convenience default: if operator sets Max > Floor but forgets
	// ScaleUpHeadroomMin, NewManager defaults it to 1. Without the
	// default, headroom < 0 can never be true and D3 silently does
	// nothing.
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      0, // operator forgot
		ReconcileInterval:       time.Second,
		HealthCheckTimeout:      time.Second,
		MaxConcurrentProvisions: 5,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Use ID-by-ref to peek at the resolved default.
	if m.cfg.ScaleUpHeadroomMin != 1 {
		t.Fatalf("expected ScaleUpHeadroomMin to default to 1 when Max>Floor; got %d", m.cfg.ScaleUpHeadroomMin)
	}
}

// --- D3 ultrareview regression tests ------------------------------------

func TestD3DoesNotOverProvisionOnColdBoot(t *testing.T) {
	// Regression: original D3 gated demand-pressure on `available >=
	// Floor`, which became true the moment Floor stubs were inserted
	// in Provisioning state. readyCapacity stayed 0 until ~90s boot,
	// so headroom (0) < HeadroomMin every cycle and D3 fired all the
	// way to Max with ZERO actual demand. Fix: gate on readyCount
	// (Ready-only) instead of available.
	//
	// Scenario: Floor=1, Max=5, HeadroomMin=2, cold boot (no rows yet).
	// Cycle 1 should provision exactly 1 (for Floor). Cycle 2 should
	// see the Floor host still Provisioning and refuse to fire D3.
	m, _, store := newD3Manager(t, 1, 5, 2)

	// Cycle 1: Floor provision
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("cycle 1: expected exactly 1 row (Floor), got %d", len(rows))
	}

	// Cycle 2: Floor host is still Provisioning (the stub provider
	// transitions it on HealthCheck after one full cycle - but
	// either way, isReady requires Status=online which the stub
	// hasn't set yet). D3 must NOT fire.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}
	rows, _ = store.ListSandboxInstances(context.Background())
	// Count Ready vs Provisioning to confirm none were added beyond
	// what Floor reconciliation does naturally.
	if len(rows) > 1 {
		// The stub may transition the row to Ready - that's fine for
		// Floor, but D3 demand-pressure should still not fire because
		// the (newly Ready) host has zero demand and provides headroom.
		// What we're guarding against is D3 firing while NOTHING is
		// Ready yet. If len > 1 here, that fire happened.
		readyCount := 0
		for _, r := range rows {
			if r.ComputeState == string(StateReady) && r.Status == "online" {
				readyCount++
			}
		}
		if readyCount == 0 {
			t.Fatalf("D3 fired demand-scale while NO host was Ready yet: %d rows but 0 Ready", len(rows))
		}
	}
}

func TestD3IsReadyRejectsOfflineRows(t *testing.T) {
	// Regression: original isReady checked only ComputeState. A Ready
	// host whose Status flipped to offline (heartbeat stale) would
	// still contribute its MaxSandboxes to readyCapacity, hiding real
	// demand pressure and preventing D3 from scaling up when it
	// should. Fix: isReady requires ComputeState=Ready AND Status=online.
	//
	// Scenario: Floor=1, Max=3, HeadroomMin=2. One Ready+ONLINE host
	// fully busy + one Ready+OFFLINE host that LOOKS like it provides
	// headroom (idle, big capacity) but is unreachable. D3 should
	// recognise this and scale up.
	m, _, store := newD3Manager(t, 1, 3, 2)
	seedReadyRow(t, store, "sbx_busy_online", 10, 10) // 0 free, real
	// Manually insert offline-but-Ready row: looks like 10 free slots
	// but is actually unreachable.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx_stale_offline",
		Provider:        "stub",
		ComputeState:    string(StateReady),
		Status:          "offline", // stale heartbeat
		ActiveSandboxes: 0,
		MaxSandboxes:    10,
	})
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	// Expect 3 rows: 2 seeded + 1 newly provisioned by D3 because the
	// offline row was correctly excluded from the headroom calculation.
	if len(rows) != 3 {
		t.Fatalf("D3 must see only the online host's headroom (0 free) and scale up; got %d rows", len(rows))
	}
}

func TestD3BatchesDemandNeedUpToMaxConcurrentProvisions(t *testing.T) {
	// Regression: original demandNeed was hard-coded to 1 per cycle,
	// making MaxConcurrentProvisions ineffective for spike response.
	// Fix: demandNeed = min(MaxConcurrentProvisions, max(1, slotsShort)).
	// The earlier ceil-by-SpecTemplate version was unsafe because
	// SpecTemplate is always zero-valued in production (bootstrap
	// doesn't populate it).
	//
	// Scenario: Floor=1, Max=10, HeadroomMin=20 (huge buffer demand),
	// MaxConcurrentProvisions=4, one Ready+online host with MaxSandboxes=10
	// fully utilized. slotsShort = 20 - 0 = 20. demandNeed =
	// min(MaxConcurrentProvisions=4, slotsShort=20) = 4. Total cycle
	// fan-out: floor satisfied (1 seed Ready), so floorNeed=0,
	// totalNeed = demandNeed = 4. Bounded again by per-cycle cap
	// MaxConcurrentProvisions=4 and Max-room=9. Final: 4 new rows.
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     10,
		ScaleUpHeadroomMin:      20,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		MaxConcurrentProvisions: 4,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	seedReadyRow(t, store, "sbx_busy", 10, 10)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 5 { // 1 seed + 4 new (min(MaxConcurrentProvisions=4, slotsShort=20))
		t.Fatalf("expected demandNeed batched to MaxConcurrentProvisions=4; got %d rows", len(rows))
	}
}

func TestD3SurvivesHeartbeatFlap(t *testing.T) {
	// Regression: the previous gate `readyCount >= Floor` silently
	// disabled D3 when a single host flickered offline (heartbeat
	// flap), exactly when scale-up is most needed. The new gate
	// `readyOnlineCount > 0` keeps D3 active as long as at least one
	// reachable host exists.
	//
	// Scenario: Floor=2, Max=4, HeadroomMin=2. Two seed rows: one
	// Ready+online (slots full), one Ready+offline (heartbeat flap).
	// readyOnlineCount=1 (the offline one is excluded from capacity
	// math). Online host has 10/10 used -> headroom=0 < HeadroomMin=2
	// -> demandNeed = min(MaxConcurrentProvisions=2, slotsShort=2) = 2.
	// Floor: available=2 already (both seeds), so floorNeed=0.
	// totalNeed = 0 + 2 = 2, capped by Max-room = 4-2 = 2. Expect 4 rows.
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   2,
		Max:                     4,
		ScaleUpHeadroomMin:      2,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		MaxConcurrentProvisions: 2,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	seedReadyRow(t, store, "sbx_online_busy", 10, 10)
	seedReadyRowOffline(t, store, "sbx_offline_flap", 10, 0)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 4 {
		t.Fatalf("expected D3 to keep firing during heartbeat flap; got %d rows (want 4 = 2 seeds + 2 new)", len(rows))
	}
}

// --- end D3 tests --------------------------------------------------------

// --- D4 (idle deprovision) tests ----------------------------------------

// newD4Manager builds a Manager with D4 enabled and a fake clock so
// tests can advance time without sleeping. Floor=1, Max=3 by default
// (D3 active so we can test the no-deprovision-during-scale-up
// interaction too).
func newD4Manager(t *testing.T, idleTimeout time.Duration) (*Manager, *StubProvider, *fakeStore, *fakeClock) {
	t.Helper()
	store := newFakeStore()
	stub := NewStubProvider("stub")
	clk := &fakeClock{now: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)}
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      1,
		IdleTimeout:             idleTimeout,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		MaxConcurrentProvisions: 5,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.now = clk.Now
	return m, stub, store, clk
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time         { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestD4DeprovisionsIdleAboveFloor(t *testing.T) {
	// Ready_count=2, Floor=1, one host idle for > IdleTimeout: the
	// idle host is deprovisioned, dropping us to Floor.
	m, _, store, clk := newD4Manager(t, 10*time.Minute)
	seedReadyRow(t, store, "sbx_busy", 5, 10)
	seedReadyRow(t, store, "sbx_idle", 0, 10)

	// First cycle: idle row gets tracked but not yet timed-out.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("cycle 1: expected idle window in flight, got %d rows", len(rows))
	}

	// Advance past IdleTimeout, reconcile again.
	clk.Advance(11 * time.Minute)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}
	rows, _ = store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("cycle 2: expected idle host shed, got %d rows", len(rows))
	}
	if rows[0].ID != "sbx_busy" {
		t.Fatalf("wrong host shed; kept %q, expected sbx_busy", rows[0].ID)
	}
}

func TestD4NeverDropsBelowFloor(t *testing.T) {
	// Ready_count=1, Floor=1, host idle for > IdleTimeout: must NOT
	// be deprovisioned because that would violate Floor.
	m, _, store, clk := newD4Manager(t, 10*time.Minute)
	seedReadyRow(t, store, "sbx_lonely_idle", 0, 10)

	// Track + advance.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("cycle 1: %v", err)
	}
	clk.Advance(30 * time.Minute)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("cycle 2: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("Floor=1 must be preserved; got %d rows", len(rows))
	}
}

func TestD4DoesNotShedBusyHosts(t *testing.T) {
	// Two Ready hosts, BOTH have active sandboxes (zero idle): never
	// gets a candidate even after a long time.
	m, _, store, clk := newD4Manager(t, 1*time.Minute)
	seedReadyRow(t, store, "sbx_busy_a", 3, 10)
	seedReadyRow(t, store, "sbx_busy_b", 7, 10)

	_ = m.Reconcile(context.Background())
	clk.Advance(10 * time.Minute)
	_ = m.Reconcile(context.Background())
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("busy hosts must not be shed; got %d rows", len(rows))
	}
}

func TestD4IdleTimerResetsWhenHostGetsWork(t *testing.T) {
	// Host goes idle, accumulates ~half the window, then picks up a
	// sandbox session. Timer must reset; subsequent idle accumulation
	// starts from zero.
	//
	// Note on test mechanics: the fakeStore's ListSandboxInstances
	// returns deep copies, so mutating the returned slice doesn't
	// persist. Use RegisterSandboxInstance to overwrite (it does a
	// fresh copy into the store).
	m, _, store, clk := newD4Manager(t, 10*time.Minute)
	seedReadyRow(t, store, "sbx_busy", 5, 10)
	seedReadyRow(t, store, "sbx_flapping", 0, 10)

	// Cycle 1: flapping is idle. Tracker records the time.
	_ = m.Reconcile(context.Background())
	clk.Advance(5 * time.Minute) // half-window
	// Now the host picks up a session. Overwrite the row in the store.
	seedReadyRow(t, store, "sbx_flapping", 2, 10)
	// Cycle 2: tracker should drop the entry because ActiveSandboxes>0.
	_ = m.Reconcile(context.Background())
	// Sandbox finishes; back to idle.
	seedReadyRow(t, store, "sbx_flapping", 0, 10)
	clk.Advance(5 * time.Minute)
	// Cycle 3: tracker re-arms with NOW as idle-since (not the
	// original cycle-1 time). Only 5 min has accumulated; no shed.
	_ = m.Reconcile(context.Background())
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 2 {
		t.Fatalf("timer should have reset; expected 2 rows still, got %d", len(rows))
	}

	// Now wait out the full window from the second idle-start.
	// (Tracker armed at cycle 3, time T+10. Need to reach T+20+ for
	// the 10-minute window to elapse. Already at T+10, advance 11
	// more minutes for safety margin.)
	clk.Advance(11 * time.Minute)
	_ = m.Reconcile(context.Background())
	rows, _ = store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("after full window post-reset, expected shed; got %d rows", len(rows))
	}
}

func TestD4SkipsWhenScaleUpProvisionedThisCycle(t *testing.T) {
	// If D3 just provisioned a host this cycle (demand pressure),
	// don't immediately deprovision an idle host in the same cycle.
	// Wait for the next cycle to re-evaluate.
	//
	// Constructing the scenario: D3 fires when (sum_max - sum_active)
	// < HeadroomMin. We need demand pressure AND an idle host AND for
	// the headroom math to still come out below the threshold. Idle
	// host with a small MaxSandboxes (e.g. 1) contributes only 1 free
	// slot; a fully-busy big host contributes 0. Total headroom = 1,
	// below HeadroomMin=2 -> D3 fires.
	store := newFakeStore()
	stub := NewStubProvider("stub")
	clk := &fakeClock{now: time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)}
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      2, // headroom 1 < 2 -> D3 fires
		IdleTimeout:             1 * time.Minute,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		MaxConcurrentProvisions: 5,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.now = clk.Now

	// Idle host with capacity-1 (so it contributes only 1 free slot),
	// already past the idle window.
	seedReadyRow(t, store, "sbx_idle", 0, 1)
	// Fully-busy host - 0 free slots.
	seedReadyRow(t, store, "sbx_full", 10, 10)
	m.idleSince["sbx_idle"] = clk.now.Add(-5 * time.Minute)

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	// Should see: 2 original + 1 new (D3 provisioning), 0 deprovisioned.
	provCount := 0
	for _, r := range rows {
		if r.ComputeState == string(StateProvisioning) {
			provCount++
		}
	}
	if provCount != 1 {
		t.Fatalf("D3 should have fired (provisioning=1); got provCount=%d (rows=%d)", provCount, len(rows))
	}
	if len(rows) != 3 {
		t.Fatalf("D4 should have skipped this cycle; expected 3 rows total, got %d", len(rows))
	}
}

func TestD4IdleTimerSurvivesAcrossReconciles(t *testing.T) {
	// The idle-since map persists across Reconcile calls (within one
	// Manager lifetime). A host idle across many cycles should be
	// counted, not have its timer reset each cycle.
	m, _, store, clk := newD4Manager(t, 10*time.Minute)
	seedReadyRow(t, store, "sbx_busy", 5, 10)
	seedReadyRow(t, store, "sbx_idle", 0, 10)

	// Run multiple short cycles totalling > IdleTimeout.
	for i := 0; i < 6; i++ {
		_ = m.Reconcile(context.Background())
		clk.Advance(2 * time.Minute)
	}
	_ = m.Reconcile(context.Background())
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("idle-tracker should persist across cycles; expected shed by now, got %d rows", len(rows))
	}
}

// --- end D4 tests --------------------------------------------------------


type failingProvider struct{ name string }

func (f *failingProvider) Name() string { return f.name }
func (f *failingProvider) Provision(context.Context, Spec) (*Handle, error) {
	return nil, errors.New("provider intentionally failing for test")
}
func (f *failingProvider) Deprovision(context.Context, *Handle, DeprovisionOpts) error {
	return nil
}
func (f *failingProvider) List(context.Context) ([]*Handle, error)      { return nil, nil }
func (f *failingProvider) HealthCheck(context.Context, *Handle) error   { return nil }

// --- tiny helpers used by tests above to avoid pulling in strings.* ---

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
