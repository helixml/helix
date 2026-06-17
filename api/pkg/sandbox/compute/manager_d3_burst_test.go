package compute

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// TestD3DoesNotDoubleProvisionWhileFirstStillBooting is the regression
// test for the demo-time observation on 2026-06-17: HEADROOM_MIN=1 with
// a single Ready Runner at-cap and 2 sessions placed on it produced
// 2 new Runners across two reconcile cycles instead of 1.
//
// Root cause: computeNeeded only counted Ready+online rows toward
// readyCapacity. The first cycle saw headroom=0 < 1 and fired a
// Provision. The second cycle (15-30s later, while the new Runner was
// still in StateProvisioning - EC2 boot + image pull take 60-90s)
// STILL saw readyCapacity=2 / readyDemand=2 / headroom=0 and fired a
// second Provision. The deficit was double-counted because in-flight
// capacity was invisible.
//
// Fix: count provisioning rows' MaxSandboxes toward `committed
// capacity` for the headroom math. The Ready-vs-Provisioning distinction
// is preserved for the readyOnlineCount gate (D3 still won't fire with
// zero serving capacity to measure against) but the deficit calc uses
// committed slots.
func TestD3DoesNotDoubleProvisionWhileFirstStillBooting(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      1,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             time.Hour, // disable D4 noise
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// One Ready Runner at full cap (the live demo scenario).
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-live",
		Provider:        "stub",
		ProviderID:      "wr-live",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 2,
	})

	// Cycle 1: headroom = (2 ready + 0 provisioning) - 2 = 0, fire 1 Provision.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile 1: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	provisioningAfter1 := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			provisioningAfter1++
		}
	}
	if provisioningAfter1 != 1 {
		t.Fatalf("cycle 1: expected exactly 1 new Provision, got %d. all rows: %+v",
			provisioningAfter1, rows)
	}

	// Cycle 2 (immediate; new Runner still in StateProvisioning,
	// MaxSandboxes=2 from the stub). Bug-on-old-code: headroom would
	// still be 0 because the provisioning row didn't count toward
	// readyCapacity -> a SECOND Provision fires. Post-fix: committed
	// capacity = 2 ready + 2 provisioning = 4; demand = 2; headroom =
	// 2 >= MIN=1, so NO additional Provision.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile 2: %v", err)
	}
	rows, _ = store.ListSandboxInstances(context.Background())
	provisioningAfter2 := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			provisioningAfter2++
		}
	}
	if provisioningAfter2 != 1 {
		t.Fatalf("cycle 2: expected SAME 1 provisioning row (no second fire), got %d. "+
			"this is the over-provision bug. all rows: %+v",
			provisioningAfter2, rows)
	}
}

// TestD3StillFiresWhenCommittedCapacityIsBelowMin guards against
// over-correction: the new committed-capacity math must STILL fire a
// Provision when the deficit is real (no in-flight provision yet).
func TestD3StillFiresWhenCommittedCapacityIsBelowMin(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      1,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// One Ready Runner at full cap. No provisioning row yet.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-live",
		Provider:        "stub",
		ProviderID:      "wr-live",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 2,
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	provisioning := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			provisioning++
		}
	}
	if provisioning != 1 {
		t.Fatalf("real deficit must still trigger D3; expected 1 new Provision, got %d. "+
			"all rows: %+v", provisioning, rows)
	}
}

// TestD3FiresAgainWhenLargerBurstOutpacesInFlightCapacity verifies that
// the fix doesn't go too far the other way: if committed capacity still
// can't satisfy HEADROOM_MIN, D3 must fire another Provision (subject
// to MaxConcurrentProvisions and Max ceiling).
//
// Scenario: Max=5, HEADROOM_MIN=3, one Ready Runner saturated (2/2),
// one provisioning (will give +2). committed = 4, demand = 2, headroom
// = 2, which is still < MIN=3 -> fire one more.
func TestD3FiresAgainWhenLargerBurstOutpacesInFlightCapacity(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     5,
		ScaleUpHeadroomMin:      3,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             time.Hour,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-live",
		Provider:        "stub",
		ProviderID:      "wr-live",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 2,
	})
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "sbx-inflight",
		Provider:     "stub",
		ProviderID:   "wr-inflight",
		ComputeState: string(StateProvisioning),
		Status:       "offline",
		MaxSandboxes: 2,
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	rows, _ := store.ListSandboxInstances(context.Background())
	provisioning := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			provisioning++
		}
	}
	// 1 pre-existing + 1 newly fired = 2 provisioning total.
	if provisioning != 2 {
		t.Fatalf("with MIN=3 and committed capacity still short, D3 must fire one more; "+
			"expected 2 provisioning rows total, got %d. all: %+v", provisioning, rows)
	}
}
