package webservice

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// fakeAlerter records the pages that would have been delivered to a human.
type fakeAlerter struct {
	mu        sync.Mutex
	down      []*types.WebServiceDownAlert
	recovered []*types.WebServiceDownAlert
}

func (f *fakeAlerter) SendWebServiceDownAlert(_ context.Context, d *types.WebServiceDownAlert) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = append(f.down, d)
}

func (f *fakeAlerter) SendWebServiceRecoveredAlert(_ context.Context, d *types.WebServiceDownAlert) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recovered = append(f.recovered, d)
}

func (f *fakeAlerter) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.down), len(f.recovered)
}

func newTestMonitor(st store.Store, alerter DownAlerter) *HealthMonitor {
	m := NewHealthMonitor(st, nil)
	m.alerter = alerter
	return m
}

// TestDownStreakStartsOnceAndIsMonotonic: the down-streak must be stamped on the
// FIRST failed probe and never refreshed while the service stays down. If it
// were refreshed, `time() - helix_webservice_unhealthy_since_seconds` would
// never grow past one tick and the alert could never fire.
func TestDownStreakStartsOnceAndIsMonotonic(t *testing.T) {
	m := newTestMonitor(nil, nil)

	m.onFailure("prj_1")
	first, ok := m.downSince["prj_1"]
	if !ok {
		t.Fatal("first failed probe did not start a down-streak")
	}

	time.Sleep(5 * time.Millisecond)
	m.onFailure("prj_1")
	m.onFailure("prj_1")

	if got := m.downSince["prj_1"]; !got.Equal(first) {
		t.Errorf("down-streak start was refreshed by later failures: %v -> %v", first, got)
	}
}

// TestDeployInProgressDoesNotClearDownStreak is the regression test for the
// we-find.ai outage. Auto-recovery redeploys a broken service every ~11 minutes;
// while that deploy is in flight the monitor reports helix_webservice_up = 1. If
// that also cleared the down-streak, the paging signal would reset on every
// recovery attempt and the alert would resolve itself forever — which is exactly
// what the operator saw ("down, then resolved") while the site served 503 for
// five days.
func TestDeployInProgressDoesNotClearDownStreak(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	m := newTestMonitor(st, nil)

	// Service goes down.
	m.onFailure("prj_1")
	started, ok := m.downSince["prj_1"]
	if !ok {
		t.Fatal("expected a down-streak after a failed probe")
	}

	// Auto-recovery kicks off a redeploy: runOnce takes the deployInProgress
	// branch, which resets probe-failure counters.
	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", 1).
		Return([]*types.WebServiceDeploy{{Status: types.WebServiceDeployStatusBuilding, StartedAt: time.Now()}}, nil)
	if !m.deployInProgress(context.Background(), "prj_1") {
		t.Fatal("expected the redeploy to count as in progress")
	}
	m.reset("prj_1")

	if got, ok := m.downSince["prj_1"]; !ok || !got.Equal(started) {
		t.Fatal("an in-flight recovery deploy cleared the down-streak — the down alert would resolve on every recovery attempt")
	}
}

// TestSuccessfulProbeClearsDownStreak: only a real success ends the streak.
func TestSuccessfulProbeClearsDownStreak(t *testing.T) {
	m := newTestMonitor(nil, nil)
	m.onFailure("prj_1")
	m.onSuccess(context.Background(), "prj_1")

	if _, ok := m.downSince["prj_1"]; ok {
		t.Error("a successful probe should have ended the down-streak")
	}
}

// TestPagesOnceThenRecoveryNotice: exactly one page per down-streak (no per-tick
// spam), and one recovery notice closing it out.
func TestPagesOnceThenRecoveryNotice(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	alerter := &fakeAlerter{}
	m := newTestMonitor(st, alerter)
	ctx := context.Background()

	// Down for longer than the threshold.
	m.onFailure("prj_1")
	m.downSince["prj_1"] = time.Now().Add(-DownAlertThreshold - time.Minute)

	// buildAlert enriches the page; every lookup is best-effort.
	st.EXPECT().GetProject(gomock.Any(), "prj_1").
		Return(&types.Project{ID: "prj_1", Name: "Find AI"}, nil).AnyTimes()
	st.EXPECT().ListVHostRoutesByTarget(gomock.Any(), types.VHostTargetProjectWebService, "prj_1").
		Return([]*types.VHostRoute{{Hostname: "we-find.ai"}, {Hostname: "www.we-find.ai"}}, nil).AnyTimes()
	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", 1).
		Return([]*types.WebServiceDeploy{{Error: "readiness: never bound"}}, nil).AnyTimes()

	// Several ticks while still down must produce exactly one page.
	m.evaluateDownAlert(ctx, "prj_1")
	m.evaluateDownAlert(ctx, "prj_1")
	m.evaluateDownAlert(ctx, "prj_1")

	waitFor(t, func() bool { d, _ := alerter.counts(); return d == 1 })
	if d, _ := alerter.counts(); d != 1 {
		t.Fatalf("expected exactly 1 page per down-streak, got %d", d)
	}

	alerter.mu.Lock()
	got := alerter.down[0]
	alerter.mu.Unlock()
	if got.ProjectName != "Find AI" || len(got.Domains) != 2 || got.DeployError == "" {
		t.Errorf("page payload is missing triage detail: %+v", got)
	}

	// Recovery closes the loop, once.
	m.onSuccess(ctx, "prj_1")
	waitFor(t, func() bool { _, r := alerter.counts(); return r == 1 })
	if _, r := alerter.counts(); r != 1 {
		t.Errorf("expected 1 recovery notice, got %d", r)
	}
}

// TestNoPageBeforeThreshold: a short blip, and a slow cold deploy, must not page.
func TestNoPageBeforeThreshold(t *testing.T) {
	alerter := &fakeAlerter{}
	m := newTestMonitor(nil, alerter)

	m.onFailure("prj_1") // down "now" — well inside the threshold
	m.evaluateDownAlert(context.Background(), "prj_1")

	if d, _ := alerter.counts(); d != 0 {
		t.Errorf("paged after a brief failure (threshold is %s), got %d pages", DownAlertThreshold, d)
	}
}

// TestDownAlertThresholdExceedsReadinessWait guards the one number that decides
// whether we page a human at 3am for a slow-but-fine cold deploy. A cold
// docker-compose build can legitimately take the full readiness window.
func TestDownAlertThresholdExceedsReadinessWait(t *testing.T) {
	readinessWait := New(nil, nil).readinessWait
	if DownAlertThreshold <= readinessWait {
		t.Fatalf("DownAlertThreshold (%s) must exceed readinessWait (%s) or slow first deploys will page",
			DownAlertThreshold, readinessWait)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
