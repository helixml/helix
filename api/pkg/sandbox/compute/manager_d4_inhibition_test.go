package compute

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// TestD4InhibitedWhileFleetAtCap is the regression test for the
// oscillation observed on yd.helix.ml on 2026-06-17:
//
//   1. R1 at cap (2/2)
//   2. D3 fires R2 (pre-warm)
//   3. R2 boots, 0 active
//   4. D4 sheds R2 (old behavior) -> R1 still at cap -> D3 re-fires -> loop
//
// Option E fix: D4 must inhibit shedding while ANY other Ready Runner
// in the fleet is at-or-over its MaxSandboxes cap. The pre-warm R2 is
// real demand-pressure relief; pulling it triggers immediate re-fire.
func TestD4InhibitedWhileFleetAtCap(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      1,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Millisecond, // fire D4 immediately
		HardIdleTimeout:         4 * time.Hour,        // safety net far away
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// R1: at cap. R2: pre-warmed by an earlier D3, now idle.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r1",
		Provider:        "stub",
		ProviderID:      "wr-r1",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 2, // at cap
	})
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r2",
		Provider:        "stub",
		ProviderID:      "wr-r2",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})

	// Arm: tracker registers R2 as idle.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile arm: %v", err)
	}
	// Past IdleTimeout for R2, but R1 still at cap -> D4 inhibited.
	time.Sleep(10 * time.Millisecond)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile shed-attempt: %v", err)
	}

	rows, _ := store.ListSandboxInstances(context.Background())
	ids := make(map[string]bool)
	for _, r := range rows {
		ids[r.ID] = true
	}
	if !ids["sbx-r2"] {
		t.Fatalf("R2 was shed despite R1 being at cap; inhibition not working. rows: %+v", rows)
	}
}

// TestD4ShedAfterFleetDropsBelowCap verifies the convergence half:
// once R1 returns to under-cap (sessions end), the inhibition lifts
// and D4 sheds the idle R2 as it would in a steady-state shrink.
func TestD4ShedAfterFleetDropsBelowCap(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      1,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Millisecond,
		HardIdleTimeout:         4 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// R1 starts at cap; R2 idle.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r1",
		Provider:        "stub",
		ProviderID:      "wr-r1",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 2,
	})
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r2",
		Provider:        "stub",
		ProviderID:      "wr-r2",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile arm: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// R1's sessions end - simulate by mutating ActiveSandboxes on the row.
	_ = store.UpdateSandboxInstanceStatus(context.Background(), "sbx-r1", "online")
	r1, _ := store.GetSandboxInstance(context.Background(), "sbx-r1")
	r1.ActiveSandboxes = 0
	_ = store.RegisterSandboxInstance(context.Background(), r1) // re-store with new value

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile shed: %v", err)
	}

	rows, _ := store.ListSandboxInstances(context.Background())
	// Exactly one of R1 or R2 should remain (Floor=1, MaxConcurrent=1).
	stubReady := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateReady) {
			stubReady++
		}
	}
	if stubReady != 1 {
		t.Fatalf("after fleet drops below cap, D4 should converge to Floor=1; "+
			"got %d Ready rows. all: %+v", stubReady, rows)
	}
}

// TestD4HardIdleTimeoutOverride guards the stuck-session edge case:
// if a Runner is permanently at cap (e.g. a hung session never reports
// ActiveSandboxes=0), the idle peer can't be pinned forever. After
// HardIdleTimeout the override fires and D4 sheds anyway.
func TestD4HardIdleTimeoutOverride(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      1,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       time.Second,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Minute,
		HardIdleTimeout:         10 * time.Minute, // tight for the test
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.now = clk.Now

	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r1-stuck",
		Provider:        "stub",
		ProviderID:      "wr-r1",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 2, // PERMANENTLY at cap (stuck session)
	})
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r2-idle",
		Provider:        "stub",
		ProviderID:      "wr-r2",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})

	// Arm idle tracker.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile arm: %v", err)
	}

	// Advance to past IdleTimeout but well within HardIdleTimeout:
	// inhibition should hold.
	clk.Advance(5 * time.Minute)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile pre-hard: %v", err)
	}
	if _, err := store.GetSandboxInstance(context.Background(), "sbx-r2-idle"); err != nil {
		t.Fatalf("R2 shed too early (before HardIdleTimeout); inhibition not working")
	}

	// Advance past HardIdleTimeout: override fires.
	clk.Advance(6 * time.Minute) // total 11m, > 10m HardIdleTimeout
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile post-hard: %v", err)
	}
	if _, err := store.GetSandboxInstance(context.Background(), "sbx-r2-idle"); err == nil {
		t.Fatalf("R2 should have been shed by HardIdleTimeout override; still present")
	}
}

// TestD4ShedsNormallyWhenFleetUnderCap guards against false inhibition:
// if the only at-cap signal is gone (under-cap or zero load), D4 sheds
// the usual way.
func TestD4ShedsNormallyWhenFleetUnderCap(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      1,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Millisecond,
		HardIdleTimeout:         4 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// R1 under cap, R2 idle. Both Ready.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r1",
		Provider:        "stub",
		ProviderID:      "wr-r1",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 1, // under cap
	})
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r2",
		Provider:        "stub",
		ProviderID:      "wr-r2",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile arm: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile shed: %v", err)
	}

	if _, err := store.GetSandboxInstance(context.Background(), "sbx-r2"); err == nil {
		t.Fatalf("R2 should have been shed (fleet under-cap, no inhibition); still present")
	}
}

// fakeClock helper already declared in manager_test.go.
