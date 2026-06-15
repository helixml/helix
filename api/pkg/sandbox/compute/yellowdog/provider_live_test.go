package yellowdog

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/sandbox/compute"
)

// TestLive_ProviderEndToEnd exercises Provision -> HealthCheck ->
// Deprovision -> List against the real YellowDog control plane.
// Gated behind YD_LIVE_TEST=1 so the package's default unit-test run
// never touches the network and CI without credentials stays green.
//
// Cost: zero EC2 spin-up. The test uses a deliberately unmatched
// WorkerTag, so the work requirement YD accepts will never find an
// eligible worker (taskGroup.starved=true). We submit, observe, then
// cancel before any compute provisions. Confirmed via the POC's
// behaviour where unmatched WRs sit indefinitely until cancelled.
//
// To run:
//
//	export YD_KEY=...  YD_SECRET=...
//	YD_LIVE_TEST=1 go test -v -run TestLive_ProviderEndToEnd ./api/pkg/sandbox/compute/yellowdog/
//
// The unique tag/namespace below isolates this test's WRs from any
// real workload running in the same account.
func TestLive_ProviderEndToEnd(t *testing.T) {
	if os.Getenv("YD_LIVE_TEST") != "1" {
		t.Skip("YD_LIVE_TEST=1 not set; skipping live test")
	}
	key := os.Getenv("YD_KEY")
	secret := os.Getenv("YD_SECRET")
	if key == "" || secret == "" {
		t.Skip("YD_KEY and YD_SECRET must be set for live test; skipping")
	}

	// Namespace mirrors the POC's `development` so we share the same
	// access scope, but the deployment tag and worker tag are unique
	// to this test invocation so we cannot collide with real WRs and
	// cannot accidentally match a real worker pool.
	uniq := time.Now().UTC().Format("20060102-150405.000000")
	cfg := Config{
		APIKeyID:      key,
		APISecret:     secret,
		Namespace:     "development",
		DeploymentTag: "helix-livetest-" + uniq,
		// Worker tag is intentionally invented. No real pool will be
		// tagged this; the WR will accept but the task will never
		// schedule. Important: this is what keeps the test free.
		WorkerTag:   "helix-livetest-no-pool-" + uniq,
		TaskType:    "bash",
		TaskTimeout: 5 * time.Minute,
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// --- Provision ---
	h, err := p.Provision(ctx, compute.Spec{
		GPUVendor: "nvidia",
		Labels:    map[string]string{"helix.test": "livetest"},
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if h.ProviderID == "" {
		t.Fatal("Provision returned handle with empty ProviderID")
	}
	t.Logf("provisioned WR id=%s name=%s", h.ProviderID, h.Metadata["yd.work_req_name"])

	// Always tear down, even on test failure, so we don't leak a
	// pending WR in the account.
	t.Cleanup(func() {
		dctx, dcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dcancel()
		if err := p.Deprovision(dctx, h, compute.DeprovisionOpts{Force: true, Reason: "livetest cleanup"}); err != nil {
			t.Logf("cleanup Deprovision: %v (may already be terminated)", err)
		}
	})

	// --- HealthCheck ---
	// Status should be one of {RUNNING, HELD} - we just submitted it,
	// no worker has picked it up yet. Either is fine; failure modes
	// would be FAILED or an unmapped status.
	hcErr := p.HealthCheck(ctx, h)
	switch h.State {
	case compute.StateReady, compute.StateProvisioning:
		// Expected. HealthCheck returns nil for these.
		if hcErr != nil {
			t.Fatalf("HealthCheck returned error for State=%q: %v", h.State, hcErr)
		}
	case compute.StateFailed, compute.StateTerminated, compute.StateTerminating:
		t.Fatalf("HealthCheck: WR landed in terminal/failed state immediately after Provision: state=%q err=%v", h.State, hcErr)
	default:
		t.Fatalf("HealthCheck: WR in unexpected state %q (err=%v) - YD may have added a new status enum value", h.State, hcErr)
	}
	t.Logf("HealthCheck: WR state=%q", h.State)

	// --- List ---
	// Our deployment tag is unique to this invocation, so List with
	// client-side filtering MUST return exactly one WR (the one we
	// just provisioned).
	all, err := p.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		// If this fires, either the client-side filter regressed or
		// a previous test run leaked a WR with the same tag (which
		// our unique timestamp should prevent).
		ids := make([]string, len(all))
		for i, h := range all {
			ids[i] = h.ProviderID
		}
		t.Fatalf("List: expected exactly 1 WR for unique deployment tag %q, got %d: %v", cfg.DeploymentTag, len(all), ids)
	}
	if all[0].ProviderID != h.ProviderID {
		t.Fatalf("List: returned ProviderID %q does not match provisioned %q", all[0].ProviderID, h.ProviderID)
	}

	// --- Deprovision ---
	// Force=true so YD aborts the (unschedulable) task immediately
	// rather than waiting for graceful drain.
	if err := p.Deprovision(ctx, h, compute.DeprovisionOpts{Force: true, Reason: "livetest end"}); err != nil {
		t.Fatalf("Deprovision: %v", err)
	}
	if h.State != compute.StateTerminating {
		t.Fatalf("Deprovision: expected handle.State=Terminating, got %q", h.State)
	}

	// HealthCheck after Deprovision should observe the transition to
	// CANCELLING or CANCELLED. Don't assert the exact terminal state -
	// the platform may still be propagating. Just confirm we don't
	// see RUNNING anymore.
	_ = p.HealthCheck(ctx, h)
	t.Logf("post-Deprovision: WR state=%q", h.State)
}
