package slack

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeConnector records connect/stop calls so tests can assert the
// manager started/stopped the right apps.
type fakeConnector struct {
	mu      sync.Mutex
	started []SocketApp
	stopped []string
}

func (f *fakeConnector) connect(_ context.Context, app SocketApp) func() {
	f.mu.Lock()
	f.started = append(f.started, app)
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		f.stopped = append(f.stopped, app.ID)
		f.mu.Unlock()
	}
}

func (f *fakeConnector) startedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, len(f.started))
	for i, a := range f.started {
		ids[i] = a.ID
	}
	return ids
}

func listing(apps *[]SocketApp, err *error) func(context.Context) ([]SocketApp, error) {
	return func(context.Context) ([]SocketApp, error) {
		if err != nil && *err != nil {
			return nil, *err
		}
		return *apps, nil
	}
}

// syncApps is a race-free desired-set holder for tests that mutate the
// configured apps while the Run loop reads them concurrently.
type syncApps struct {
	mu   sync.Mutex
	apps []SocketApp
}

func (s *syncApps) set(a []SocketApp) {
	s.mu.Lock()
	s.apps = a
	s.mu.Unlock()
}

func (s *syncApps) list(context.Context) ([]SocketApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.apps, nil
}

// A newly-configured socket app gets a connection on the next reconcile —
// no restart. And reconcile is idempotent: a second pass with the same
// config does not reconnect.
func TestSocketManager_StartsNewAppAndIsIdempotent(t *testing.T) {
	fc := &fakeConnector{}
	apps := []SocketApp{{ID: "a1", AppToken: "xapp-1"}}
	m := NewSocketManager(listing(&apps, nil), fc.connect, nil)

	m.Reconcile(context.Background())
	if got := fc.startedIDs(); len(got) != 1 || got[0] != "a1" {
		t.Fatalf("first reconcile: started = %v, want [a1]", got)
	}

	m.Reconcile(context.Background())
	if got := fc.startedIDs(); len(got) != 1 {
		t.Fatalf("reconcile not idempotent: started = %v, want still [a1]", got)
	}
}

// A removed app's connection is stopped on the next reconcile.
func TestSocketManager_StopsRemovedApp(t *testing.T) {
	fc := &fakeConnector{}
	apps := []SocketApp{{ID: "a1", AppToken: "xapp-1"}}
	m := NewSocketManager(listing(&apps, nil), fc.connect, nil)

	m.Reconcile(context.Background())
	apps = nil // app deleted
	m.Reconcile(context.Background())

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.stopped) != 1 || fc.stopped[0] != "a1" {
		t.Fatalf("stopped = %v, want [a1]", fc.stopped)
	}
}

// An app-token change (operator edited the app) tears down the old
// connection and starts a fresh one with the new token.
func TestSocketManager_RestartsOnTokenChange(t *testing.T) {
	fc := &fakeConnector{}
	apps := []SocketApp{{ID: "a1", AppToken: "xapp-old"}}
	m := NewSocketManager(listing(&apps, nil), fc.connect, nil)

	m.Reconcile(context.Background())
	apps = []SocketApp{{ID: "a1", AppToken: "xapp-new"}}
	m.Reconcile(context.Background())

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.stopped) != 1 || fc.stopped[0] != "a1" {
		t.Fatalf("stopped = %v, want [a1] (old token torn down)", fc.stopped)
	}
	if len(fc.started) != 2 || fc.started[1].AppToken != "xapp-new" {
		t.Fatalf("started = %+v, want a second connect with xapp-new", fc.started)
	}
}

// Apps with no app token are skipped (a rest-mode or half-configured app
// must not get a socket connection).
func TestSocketManager_SkipsAppsWithoutToken(t *testing.T) {
	fc := &fakeConnector{}
	apps := []SocketApp{{ID: "a1", AppToken: ""}}
	m := NewSocketManager(listing(&apps, nil), fc.connect, nil)

	m.Reconcile(context.Background())
	if got := fc.startedIDs(); len(got) != 0 {
		t.Fatalf("started = %v, want none (no token)", got)
	}
}

// A transient list error must not tear down healthy connections.
func TestSocketManager_ListErrorLeavesRunningIntact(t *testing.T) {
	fc := &fakeConnector{}
	apps := []SocketApp{{ID: "a1", AppToken: "xapp-1"}}
	var listErr error
	m := NewSocketManager(listing(&apps, &listErr), fc.connect, nil)

	m.Reconcile(context.Background())
	listErr = errors.New("store down")
	m.Reconcile(context.Background())

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.stopped) != 0 {
		t.Fatalf("stopped = %v, want none (list error must not stop connections)", fc.stopped)
	}
}

// Kick triggers an immediate reconcile so a newly-created socket app
// connects without waiting for the (here, effectively infinite) tick —
// the "installing a socket app should just work, no restart" guarantee.
func TestSocketManager_KickPicksUpNewAppWithoutWaitingForTick(t *testing.T) {
	fc := &fakeConnector{}
	sa := &syncApps{} // none configured yet
	m := NewSocketManager(sa.list, fc.connect, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx, time.Hour) // only a Kick can drive reconcile within the test

	sa.set([]SocketApp{{ID: "a1", AppToken: "xapp-1"}}) // operator installs it
	m.Kick()

	waitFor(t, func() bool { return len(fc.startedIDs()) == 1 }, 2*time.Second)
	if got := fc.startedIDs(); len(got) != 1 || got[0] != "a1" {
		t.Fatalf("after Kick: started = %v, want [a1]", got)
	}
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if cond() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("condition not met before timeout")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Run stops every live connection when its context is cancelled.
func TestSocketManager_RunStopsAllOnShutdown(t *testing.T) {
	fc := &fakeConnector{}
	apps := []SocketApp{{ID: "a1", AppToken: "x1"}, {ID: "a2", AppToken: "x2"}}
	m := NewSocketManager(listing(&apps, nil), fc.connect, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, time.Hour); close(done) }()

	// Wait for the initial reconcile to start both, then shut down.
	deadline := time.After(2 * time.Second)
	for {
		if len(fc.startedIDs()) == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for initial reconcile")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()
	if len(fc.stopped) != 2 {
		t.Fatalf("stopped = %v, want both a1 and a2 stopped on shutdown", fc.stopped)
	}
}
