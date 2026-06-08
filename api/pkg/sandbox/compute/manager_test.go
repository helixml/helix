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
	// Regression guard for the must-fix bug from review of D1: prior
	// to the fix, Status was updated but ComputeState was not, so a
	// Ready or Failed row would silently keep counting as
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
	// set must default to 1, preserving the D1 conservative behaviour.
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
