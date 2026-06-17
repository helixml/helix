package compute

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// TestReconcileReplacesReadyOfflineRow is the regression test for the
// yd.helix.ml outage on 2026-06-17. Before the fix, isAvailable counted
// a Ready+offline row toward the Floor, so when the sandbox-instance
// reaper flipped a dead runner from "online" to "offline" the
// ComputeManager kept thinking the Floor was satisfied and never
// provisioned a replacement. Result: Floor=1 but 0 healthy runners,
// indefinitely - until D4 happened to shed the dead row.
//
// Post-fix, isAliveForFloor excludes Ready+offline rows, so the very
// next Reconcile cycle sees the gap and submits a new Provision.
func TestReconcileReplacesReadyOfflineRow(t *testing.T) {
	m, _, store := newTestManager(t, 1)

	// Seed a row that the reaper has already flipped to offline. It's
	// still owned by our Provider (compute_state=ready) but its YD WR
	// is dead. This is exactly the state sbx_99c93f07 was in on yd.helix.ml.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "sbx-dead",
		Provider:     "stub",
		ProviderID:   "wr-dead",
		ComputeState: string(StateReady),
		Status:       "offline",
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	rows, _ := store.ListSandboxInstances(context.Background())
	stubProvisioning := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			stubProvisioning++
		}
	}
	if stubProvisioning != 1 {
		t.Fatalf("Floor=1 with 1 Ready+offline row should provision a replacement; "+
			"expected 1 provisioning stub row, got %d. all rows: %+v",
			stubProvisioning, rows)
	}
}

// TestReconcileDoesNotDoubleProvisionDuringD4Shed ensures the fix
// doesn't over-correct. If a Ready+offline row exists AND we've
// already provisioned a replacement (so Floor is met by the new
// provisioning row), Reconcile must NOT provision a SECOND replacement
// just because the offline row still sits there.
//
// This is the case the comments on isAvailable describe: keeping
// Ready+offline rows in the Max ceiling prevents churn while D4 sheds
// the dead row asynchronously.
func TestReconcileDoesNotDoubleProvisionDuringD4Shed(t *testing.T) {
	m, _, store := newTestManager(t, 1)

	// Dead row that's about to be shed.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "sbx-dead",
		Provider:     "stub",
		ProviderID:   "wr-dead",
		ComputeState: string(StateReady),
		Status:       "offline",
	})
	// Replacement already in flight (previous Reconcile fired this).
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "sbx-replacement",
		Provider:     "stub",
		ProviderID:   "wr-new",
		ComputeState: string(StateProvisioning),
		Status:       "offline", // not online until it registers
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	rows, _ := store.ListSandboxInstances(context.Background())
	provisioningCount := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			provisioningCount++
		}
	}
	if provisioningCount != 1 {
		t.Fatalf("with one dead row + one in-flight replacement, Reconcile must NOT "+
			"fire a third Provision; expected 1 provisioning row, got %d. all rows: %+v",
			provisioningCount, rows)
	}
}

// TestReconcileMaxCeilingHonoursOwnedOfflineRows confirms the
// counterpart of the Floor-predicate split: the Max ceiling still
// counts Ready+offline rows as "owned." Without this, an operator who
// set Floor=0 / Max=1 would see endless churn (Ready+offline row
// stops counting against Max -> Reconcile fires Provision -> D4
// fires Deprovision -> repeat).
//
// Distinct from TestReconcileDoesNotDoubleProvisionDuringD4Shed
// (which conflates the Floor-met-by-provisioning-row case): here Floor=0
// so no Floor pressure exists at all, isolating the Max-ceiling
// behaviour.
func TestReconcileMaxCeilingHonoursOwnedOfflineRows(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   0,
		Max:                     1,
		ScaleUpHeadroomMin:      1,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             time.Hour, // disable D4 for this test
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "sbx-dead",
		Provider:     "stub",
		ProviderID:   "wr-dead",
		ComputeState: string(StateReady),
		Status:       "offline",
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	rows, _ := store.ListSandboxInstances(context.Background())
	provisioningCount := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			provisioningCount++
		}
	}
	if provisioningCount != 0 {
		t.Fatalf("Floor=0 Max=1 with 1 Ready+offline row owned by us must NOT provision "+
			"a replacement (would breach Max during D4 shed); got %d provisioning rows. all: %+v",
			provisioningCount, rows)
	}
}

// TestReconcileFloor2MixedRows locks in the count math when the input
// is a mix of Ready+online and Ready+offline rows. Floor=2, one online
// row, one offline row -> aliveForFloor=1, floorNeed=1, exactly one
// Provision (not two, not zero).
func TestReconcileFloor2MixedRows(t *testing.T) {
	m, _, store := newTestManager(t, 2)

	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "sbx-healthy",
		Provider:     "stub",
		ProviderID:   "wr-healthy",
		ComputeState: string(StateReady),
		Status:       "online",
	})
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:           "sbx-dead",
		Provider:     "stub",
		ProviderID:   "wr-dead",
		ComputeState: string(StateReady),
		Status:       "offline",
	})

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	rows, _ := store.ListSandboxInstances(context.Background())
	provisioningCount := 0
	for _, r := range rows {
		if r.Provider == "stub" && r.ComputeState == string(StateProvisioning) {
			provisioningCount++
		}
	}
	if provisioningCount != 1 {
		t.Fatalf("Floor=2 with 1 online + 1 offline should fire exactly 1 Provision; "+
			"got %d. all rows: %+v", provisioningCount, rows)
	}
}

// TestD4StillShedsReadyOfflineRow confirms the Floor-predicate split
// doesn't strand dead rows. D4 keys off isReadyState (broad), which
// is unchanged. So a Ready+offline row that's also idle past
// IdleTimeout still gets shed - this test pins that interaction so
// any future tightening of isReadyState catches the regression.
func TestD4StillShedsReadyOfflineRow(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   0,
		Max:                     0,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Millisecond,
		MaxConcurrentProvisions: 1,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-dead",
		Provider:        "stub",
		ProviderID:      "wr-dead",
		ComputeState:    string(StateReady),
		Status:          "offline",
		ActiveSandboxes: 0,
	})

	// First Reconcile arms the idle tracker. Second (after IdleTimeout
	// expires) sheds.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile arm: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile shed: %v", err)
	}

	rows, _ := store.ListSandboxInstances(context.Background())
	for _, r := range rows {
		if r.ID == "sbx-dead" && r.ComputeState == string(StateReady) {
			t.Fatalf("Ready+offline row should have been shed by D4; still Ready. all: %+v", rows)
		}
	}
}

// TestComputeNeededFloorIgnoresOfflineRows is a direct unit test on
// computeNeeded covering the predicate boundary. Tested:
//
//   - Ready+online row    -> counts toward Floor          (the healthy case)
//   - Ready+offline row   -> does NOT count toward Floor  (the bug fix)
//   - Provisioning row    -> counts toward Floor          (in-flight, don't double-provision)
//   - Failed row          -> does not count               (dead, D4 will clean it up)
func TestComputeNeededFloorIgnoresOfflineRows(t *testing.T) {
	cases := []struct {
		name string
		row  *types.SandboxInstance
		// wantNeed is the result of computeNeeded with Floor=1 and just this row.
		// 0 = row covers the floor; 1 = floor still needs filling.
		wantNeed int
	}{
		{
			name:     "ready+online satisfies floor",
			row:      &types.SandboxInstance{Provider: "stub", ComputeState: string(StateReady), Status: "online"},
			wantNeed: 0,
		},
		{
			name:     "ready+offline does NOT satisfy floor (regression)",
			row:      &types.SandboxInstance{Provider: "stub", ComputeState: string(StateReady), Status: "offline"},
			wantNeed: 1,
		},
		{
			name:     "provisioning row satisfies floor (in-flight)",
			row:      &types.SandboxInstance{Provider: "stub", ComputeState: string(StateProvisioning), Status: "offline"},
			wantNeed: 0,
		},
		{
			name:     "failed row does NOT satisfy floor",
			row:      &types.SandboxInstance{Provider: "stub", ComputeState: string(StateFailed), Status: "offline"},
			wantNeed: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := newTestManager(t, 1)
			got := m.computeNeeded([]*types.SandboxInstance{tc.row})
			if got != tc.wantNeed {
				t.Fatalf("computeNeeded(%s) = %d, want %d", tc.name, got, tc.wantNeed)
			}
		})
	}
}
