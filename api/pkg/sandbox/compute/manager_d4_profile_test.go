package compute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// TestD4SkipsProfileAssignedRunner is the priority correctness fix:
// a Runner with a Runner Profile assigned may be serving inference
// (e.g. vllm-tiny via compose-manager) even with active_sandboxes = 0.
// D4 must not shed it - tearing it down kills the inference engine.
func TestD4SkipsProfileAssignedRunner(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      0,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Millisecond,

	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// R1: keeps the floor met so D4 is allowed to consider others.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r1-keepalive",
		Provider:        "stub",
		ProviderID:      "wr-r1",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})
	// R2: profile-assigned. 0 sandboxes but presumed serving inference
	// via the assigned profile. MUST be protected from D4.
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r2-inference",
		Provider:        "stub",
		ProviderID:      "wr-r2",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})
	store.setAssignment("sbx-r2-inference", "rprof_vllm_tiny")

	// Arm idle tracker, then trigger a shed cycle.
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile arm: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile shed: %v", err)
	}

	// Per-cycle MaxConcurrent=1: at most one shed. The protected R2
	// must NOT be the one. R1 was the keepalive; D4 should have shed R1
	// instead (it's the only unprotected candidate).
	if _, err := store.GetSandboxInstance(context.Background(), "sbx-r2-inference"); err != nil {
		t.Fatalf("R2 (profile-assigned) was shed despite the protection; "+
			"this is the bug fixed here. error: %v", err)
	}
	// R1 deregistered. (Floor=1: this leaves 1 ready row, the profile-
	// assigned R2.)
	if _, err := store.GetSandboxInstance(context.Background(), "sbx-r1-keepalive"); err == nil {
		t.Fatalf("R1 (unprotected, idle) should have been shed; still present")
	}
}

// TestD4ShedsUnassignedRunnerNormally guards against false positives:
// when a Runner has NO profile assignment, the new code path must NOT
// inhibit shedding. Same shape as TestD4SkipsProfileAssignedRunner but
// with no assignment set.
func TestD4ShedsUnassignedRunnerNormally(t *testing.T) {
	store := newFakeStore()
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      0,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Millisecond,

	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r1",
		Provider:        "stub",
		ProviderID:      "wr-r1",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})
	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r2-noprofile",
		Provider:        "stub",
		ProviderID:      "wr-r2",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
	})
	// no assignment set on either row

	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile arm: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile shed: %v", err)
	}

	// Exactly one shed (MaxConcurrent=1). Which one doesn't matter.
	left := 0
	for _, id := range []string{"sbx-r1", "sbx-r2-noprofile"} {
		if _, err := store.GetSandboxInstance(context.Background(), id); err == nil {
			left++
		}
	}
	if left != 1 {
		t.Fatalf("expected exactly 1 of 2 idle unassigned Runners shed; %d still present", left)
	}
}

// TestD4SkipsShedOnAssignmentLookupError verifies the conservative
// failure mode: if ListRunnerAssignments fails (DB hiccup, timeout),
// D4 must NOT shed - we'd rather hold an idle Runner alive for one
// cycle than risk killing inference because we didn't know the
// assignments.
func TestD4SkipsShedOnAssignmentLookupError(t *testing.T) {
	store := &assignmentErrorStore{fakeStore: newFakeStore()}
	stub := NewStubProvider("stub")
	m, err := NewManager(stub, store, ManagerConfig{
		Floor:                   1,
		Max:                     3,
		ScaleUpHeadroomMin:      0,
		MaxConcurrentProvisions: 1,
		ReconcileInterval:       10 * time.Millisecond,
		HealthCheckTimeout:      100 * time.Millisecond,
		IdleTimeout:             1 * time.Millisecond,

	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_ = store.RegisterSandboxInstance(context.Background(), &types.SandboxInstance{
		ID:              "sbx-r1",
		Provider:        "stub",
		ProviderID:      "wr-r1",
		ComputeState:    string(StateReady),
		Status:          "online",
		MaxSandboxes:    2,
		ActiveSandboxes: 0,
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

	// Both rows should still be present - the shed was skipped.
	for _, id := range []string{"sbx-r1", "sbx-r2"} {
		if _, err := store.GetSandboxInstance(context.Background(), id); err != nil {
			t.Fatalf("row %s shed despite the assignments-lookup error; "+
				"D4 should have bailed conservatively. error: %v", id, err)
		}
	}
}

// assignmentErrorStore wraps fakeStore and always errors on
// ListRunnerAssignments. Used to verify the conservative-skip path.
type assignmentErrorStore struct{ *fakeStore }

func (s *assignmentErrorStore) ListRunnerAssignments(_ context.Context) ([]*types.RunnerAssignment, error) {
	return nil, errors.New("simulated DB timeout")
}
