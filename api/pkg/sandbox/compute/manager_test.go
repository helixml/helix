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

func (f *fakeStore) DeregisterSandboxInstance(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, id)
	return nil
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
	// Manually mark the row state=provisioning to mirror what'd happen
	// in production: row.ComputeState=provisioning, stub.handle.State=Ready.
	// The next Reconcile must call HealthCheck and propagate the
	// stub's view onto the row.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile #2: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 row after two cycles, got %d", len(rows))
	}
	if rows[0].Status != "online" {
		t.Fatalf("HealthCheck should have promoted row to online; got Status=%q", rows[0].Status)
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
