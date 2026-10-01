package webservice

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// TestAwaitDeployOutcomes: recovery counts as a success only when the deploy it
// kicked off actually went live. Redeploy returns as soon as it has spawned its
// goroutine, so treating that as success reported every failed deploy as a
// successful recovery — which is why backoff never grew and
// helix_webservice_consecutive_recovery_failures stayed pinned at 0 through
// ~28h of continuously failing recoveries on 2026-09-27.
func TestAwaitDeployOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		deploys  []*types.WebServiceDeploy
		wantErr  bool
		contains string
	}{
		{
			name:    "live is a real success",
			deploys: []*types.WebServiceDeploy{{ID: "wsd_1", Status: types.WebServiceDeployStatusLive}},
		},
		{
			name:    "superseded is not this recovery's failure",
			deploys: []*types.WebServiceDeploy{{ID: "wsd_1", Status: types.WebServiceDeployStatusSuperseded}},
		},
		{
			name:     "failed deploy is a failed recovery",
			deploys:  []*types.WebServiceDeploy{{ID: "wsd_1", Status: types.WebServiceDeployStatusFailed, Error: "ensure sandbox: no host"}},
			wantErr:  true,
			contains: "ensure sandbox: no host",
		},
		{
			name:     "failed deploy with no detail still fails",
			deploys:  []*types.WebServiceDeploy{{ID: "wsd_1", Status: types.WebServiceDeployStatusFailed}},
			wantErr:  true,
			contains: "wsd_1 failed",
		},
		{
			name:    "a newer deploy took over, so it accounts for itself",
			deploys: []*types.WebServiceDeploy{{ID: "wsd_2", Status: types.WebServiceDeployStatusBuilding}},
		},
		{
			name:    "no deploy rows at all",
			deploys: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			st := store.NewMockStore(ctrl)
			st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", 1).Return(c.deploys, nil)

			err := (&HealthMonitor{store: st}).awaitDeploy(context.Background(), "prj_1", "wsd_1")

			switch {
			case c.wantErr && err == nil:
				t.Fatal("expected an error, got nil")
			case !c.wantErr && err != nil:
				t.Fatalf("expected success, got %v", err)
			case c.contains != "" && !strings.Contains(err.Error(), c.contains):
				t.Fatalf("error %v does not mention %q", err, c.contains)
			}
		})
	}
}

// A deploy that never reaches a terminal state surfaces as a failed recovery
// when the context expires, rather than leaking this goroutine forever.
func TestAwaitDeployGivesUpWithContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", 1).
		Return([]*types.WebServiceDeploy{{ID: "wsd_1", Status: types.WebServiceDeployStatusBuilding}}, nil).
		AnyTimes()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := (&HealthMonitor{store: st}).awaitDeploy(ctx, "prj_1", "wsd_1"); err == nil {
		t.Fatal("expected a timeout error for a deploy that never finishes")
	}
}

// recovFails drives BOTH the exponential backoff and
// helix_webservice_consecutive_recovery_failures (the looping alert). Prove it
// climbs to the threshold the Prometheus rule fires on and clears only on a
// genuinely healthy probe — it never climbed at all before, because every
// recovery was recorded as a success.
func TestConsecutiveRecoveryFailuresReachLoopingThreshold(t *testing.T) {
	m := &HealthMonitor{
		fails:      map[string]int{},
		lastRecov:  map[string]time.Time{},
		recovFails: map[string]int{},
		prevActive: map[string]struct{}{},
	}
	const pid = "prj_looping_test"

	for i := 1; i <= loopingAlertThreshold; i++ {
		m.recordRecoveryResult(pid, false)
		if got := m.recovFails[pid]; got != i {
			t.Fatalf("after %d failed recoveries counter = %d, want %d", i, got, i)
		}
	}

	m.onSuccess(pid)
	if got := m.recovFails[pid]; got != 0 {
		t.Fatalf("counter = %d after a healthy probe, want 0", got)
	}
}

// Backoff must actually grow with consecutive failures, so a persistently
// broken service is not re-recovered every cooldown for hours (22-24 deploy
// attempts per hour, for 28 hours, is what this prevents).
func TestBackoffGrowsWithConsecutiveFailures(t *testing.T) {
	m := NewHealthMonitor(nil, nil)
	const pid = "prj_backoff"

	// Fresh: recovery is allowed once the probe threshold is crossed.
	m.lastRecov[pid] = time.Now().Add(-m.cooldown - time.Second)
	m.recovFails[pid] = 0
	if got := m.backoffFor(pid); got != m.cooldown {
		t.Fatalf("clean backoff = %v, want %v", got, m.cooldown)
	}

	// Doubles per consecutive failure...
	m.recovFails[pid] = 2
	if got, want := m.backoffFor(pid), m.cooldown<<2; got != want {
		t.Fatalf("backoff after 2 failures = %v, want %v", got, want)
	}

	// ...but never past the cap, so a broken service is retried periodically
	// rather than abandoned. With the 5m cooldown that bites from 3 failures on.
	for _, rf := range []int{3, 99} {
		m.recovFails[pid] = rf
		if got := m.backoffFor(pid); got != maxRecoveryBackoff {
			t.Fatalf("backoff at %d failures = %v, want the %v cap", rf, got, maxRecoveryBackoff)
		}
	}
}
