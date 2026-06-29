package compute

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeManager records its lifecycle and blocks until cancelled, standing
// in for a real *Manager.Run.
type fakeManager struct {
	started chan struct{}
	exitErr error // if set, Run returns it immediately instead of blocking
	mu      sync.Mutex
	stopped bool
}

func newFakeManager() *fakeManager { return &fakeManager{started: make(chan struct{})} }

func (m *fakeManager) Run(ctx context.Context) error {
	close(m.started)
	if m.exitErr != nil {
		return m.exitErr
	}
	<-ctx.Done()
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	return ctx.Err()
}

func (m *fakeManager) isStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

type fakeFactory struct {
	mu       sync.Mutex
	calls    int
	managers map[string]*fakeManager // keyed by pool key
	err      error
	exitErr  error // managers built return this immediately from Run
}

func newFakeFactory() *fakeFactory { return &fakeFactory{managers: map[string]*fakeManager{}} }

func (f *fakeFactory) NewPoolManager(p DiscoveredPool) (poolManager, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	m := newFakeManager()
	m.exitErr = f.exitErr
	f.managers[p.Key] = m
	return m, nil
}

func (f *fakeFactory) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeDiscoverer struct {
	mu    sync.Mutex
	pools []DiscoveredPool
	err   error
}

func (d *fakeDiscoverer) set(pools []DiscoveredPool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pools = pools
	d.err = err
}

func (d *fakeDiscoverer) DiscoverPools(_ context.Context) ([]DiscoveredPool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.pools, d.err
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}

func TestPoolSupervisorStartsAndStopsManagers(t *testing.T) {
	disc := &fakeDiscoverer{}
	fac := newFakeFactory()
	s, err := NewPoolSupervisor(disc, fac, time.Hour) // ticker irrelevant; we drive reconcile directly
	if err != nil {
		t.Fatalf("NewPoolSupervisor: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poolA := DiscoveredPool{Key: "worker-nvidia", WorkerTag: "worker-nvidia", InstanceType: "g5.xlarge", NodeCount: 1}
	poolB := DiscoveredPool{Key: "worker-inf2", WorkerTag: "worker-inf2", InstanceType: "inf2.8xlarge", NodeCount: 1}

	// Pool A appears -> one manager started.
	disc.set([]DiscoveredPool{poolA}, nil)
	s.reconcile(ctx)
	waitFor(t, func() bool { return fac.callCount() == 1 }, "manager A built")
	mgrA := fac.managers["worker-nvidia"]
	<-mgrA.started

	// Reconcile again with the same pool -> no new manager.
	s.reconcile(ctx)
	if got := fac.callCount(); got != 1 {
		t.Fatalf("stable pool rebuilt a manager: callCount=%d, want 1", got)
	}

	// Pool B also appears -> a second manager started, A untouched.
	disc.set([]DiscoveredPool{poolA, poolB}, nil)
	s.reconcile(ctx)
	waitFor(t, func() bool { return fac.callCount() == 2 }, "manager B built")
	mgrB := fac.managers["worker-inf2"]
	<-mgrB.started
	if mgrA.isStopped() {
		t.Fatal("manager A stopped when B appeared")
	}

	// Pool A disappears -> its manager stops, B keeps running.
	disc.set([]DiscoveredPool{poolB}, nil)
	s.reconcile(ctx)
	waitFor(t, func() bool { return mgrA.isStopped() }, "manager A stopped")
	if mgrB.isStopped() {
		t.Fatal("manager B stopped when only A was removed")
	}

	// Shut everything down.
	cancel()
	s.stopAll()
	waitFor(t, func() bool { return mgrB.isStopped() }, "manager B stopped on shutdown")
}

func TestPoolSupervisorDiscoveryErrorKeepsManagers(t *testing.T) {
	disc := &fakeDiscoverer{}
	fac := newFakeFactory()
	s, _ := NewPoolSupervisor(disc, fac, time.Hour)
	ctx := context.Background()

	disc.set([]DiscoveredPool{{Key: "p", WorkerTag: "p", InstanceType: "g5.xlarge", NodeCount: 1}}, nil)
	s.reconcile(ctx)
	waitFor(t, func() bool { return fac.callCount() == 1 }, "manager built")

	// Discovery now errors: the running manager must NOT be torn down.
	disc.set(nil, errors.New("yd down"))
	s.reconcile(ctx)
	if fac.managers["p"].isStopped() {
		t.Fatal("manager stopped on a transient discovery error")
	}
}

func TestPoolSupervisorRebuildsExitedManager(t *testing.T) {
	disc := &fakeDiscoverer{}
	fac := newFakeFactory()
	fac.exitErr = errors.New("manager crashed") // every built manager returns immediately
	s, _ := NewPoolSupervisor(disc, fac, time.Hour)
	ctx := context.Background()

	disc.set([]DiscoveredPool{{Key: "p", WorkerTag: "p", InstanceType: "g5.xlarge", NodeCount: 1}}, nil)

	s.reconcile(ctx)
	waitFor(t, func() bool { return fac.callCount() == 1 }, "first build")

	// A Manager that returns on its own must self-remove from the running
	// set, otherwise it is never rebuilt and the pool silently loses its floor.
	waitFor(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.running) == 0
	}, "exited manager removed from running set")

	// Next cycle, the still-present pool gets a fresh Manager.
	s.reconcile(ctx)
	waitFor(t, func() bool { return fac.callCount() == 2 }, "rebuilt after exit")
}

func TestPoolSupervisorSkipsUnbuildablePool(t *testing.T) {
	disc := &fakeDiscoverer{}
	fac := newFakeFactory()
	fac.err = errors.New("unclassifiable instance type")
	s, _ := NewPoolSupervisor(disc, fac, time.Hour)
	ctx := context.Background()

	disc.set([]DiscoveredPool{{Key: "weird", WorkerTag: "weird", InstanceType: "t3.small"}}, nil)
	s.reconcile(ctx)
	// Factory was called and errored; nothing should be tracked as running.
	if got := fac.callCount(); got != 1 {
		t.Fatalf("callCount=%d, want 1", got)
	}
	s.mu.Lock()
	n := len(s.running)
	s.mu.Unlock()
	if n != 0 {
		t.Fatalf("running managers=%d, want 0 (pool was unbuildable)", n)
	}
}

func TestAcceleratorForInstanceType(t *testing.T) {
	cases := map[string]string{
		"g5.xlarge":      "nvidia",
		"g4ad.xlarge":    "nvidia", // known-imperfect: g4ad is AMD; documents the heuristic ceiling
		"g6.12xlarge":    "nvidia",
		"p4d.24xlarge":   "nvidia",
		"inf2.8xlarge":   "neuron",
		"inf2.xlarge":    "neuron",
		"trn2.3xlarge":   "neuron",
		"trn1.32xlarge":  "neuron",
		"t3.small":       "",
		"m6g.large":      "",
		"":               "",
		" INF2.XLARGE  ": "neuron",
	}
	for in, want := range cases {
		if got := AcceleratorForInstanceType(in); got != want {
			t.Errorf("AcceleratorForInstanceType(%q) = %q, want %q", in, got, want)
		}
	}
}
