package activations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/seedprompts"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// fakeEnsurer / fakeDispatcher are the minimal Activate collaborators the
// tests wire so the audit-row pre-allocation can be exercised through the
// public Activate command rather than a standalone helper.
type fakeEnsurer struct{}

func (fakeEnsurer) Ensure(_ context.Context, _ string, _ orgchart.NodeID) (string, string, string, error) {
	return "prj-1", "app-1", "repo-1", nil
}

type fakeDispatcher struct {
	gotID       activation.ID
	hireCalls   int
	manualCalls int
	events      *[]string
}

func (f *fakeDispatcher) DispatchHire(_ context.Context, _ string, _ orgchart.NodeID, activationID activation.ID) {
	f.gotID = activationID
	f.hireCalls++
	if f.events != nil {
		*f.events = append(*f.events, "dispatch")
	}
}

func (f *fakeDispatcher) DispatchManual(_ context.Context, _ string, _ orgchart.NodeID, activationID activation.ID) {
	f.gotID = activationID
	f.manualCalls++
	if f.events != nil {
		*f.events = append(*f.events, "dispatch")
	}
}

// TestActivate_PreAllocatesAuditRow: with a wired repo, Activate mints the
// `a-<id>` audit row, persists it, surfaces the id in the result, and
// hands that same id to the dispatcher.
func TestActivate_PreAllocatesAuditRow(t *testing.T) {
	t.Parallel()
	st := memory.New()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		Repo:       st.Activations,
		Now:        func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID:      func() string { return "fixed" },
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
	})

	res, err := svc.Activate(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if res.ActivationID != "a-fixed" {
		t.Fatalf("activation id = %q, want a-fixed", res.ActivationID)
	}
	if res.ProjectID != "prj-1" || res.AgentID != "app-1" {
		t.Fatalf("project/agent ids = %q/%q, want prj-1/app-1", res.ProjectID, res.AgentID)
	}
	if disp.gotID != "a-fixed" {
		t.Fatalf("dispatcher got id %q, want a-fixed", disp.gotID)
	}
	got, err := st.Activations.Get(context.Background(), "org-test", res.ActivationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("activation row not persisted")
	}
}

// TestActivate_NoRepoMintsNoRow: with no repo wired, Activate skips the
// pre-allocation — the result and the dispatcher both get an empty id, so
// the Spawner mints its own (the previous inline behaviour).
func TestActivate_NoRepoMintsNoRow(t *testing.T) {
	t.Parallel()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		NewID:      func() string { return "x" }, // no Repo
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
	})

	res, err := svc.Activate(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if res.ActivationID != "" {
		t.Fatalf("activation id = %q, want empty (no pre-allocation)", res.ActivationID)
	}
	if disp.gotID != "" {
		t.Fatalf("dispatcher got id %q, want empty", disp.gotID)
	}
}

type fakeSessions struct {
	id  string
	err error
}

func (f fakeSessions) SessionID(_ context.Context, _ string, _ orgchart.NodeID) (string, error) {
	return f.id, f.err
}

func TestActivate_FirstChiefOfStaffSessionUsesHire(t *testing.T) {
	t.Parallel()
	st := memory.New()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		Repo:       st.Activations,
		Now:        func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID:      func() string { return "first" },
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
		Sessions:   fakeSessions{},
	})

	res, err := svc.Activate(context.Background(), "org-test", seedprompts.ChiefOfStaffBotID)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if disp.hireCalls != 1 || disp.manualCalls != 0 {
		t.Fatalf("dispatch calls hire/manual = %d/%d, want 1/0", disp.hireCalls, disp.manualCalls)
	}
	row, err := st.Activations.Get(context.Background(), "org-test", res.ActivationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(row.Triggers) != 1 || row.Triggers[0].Kind != activation.TriggerHire {
		t.Fatalf("triggers = %+v, want [hire]", row.Triggers)
	}
}

func TestActivate_OrdinaryFirstSessionUsesManual(t *testing.T) {
	t.Parallel()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
		Sessions:   fakeSessions{},
	})

	if _, err := svc.Activate(context.Background(), "org-test", "w-mark"); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if disp.hireCalls != 0 || disp.manualCalls != 1 {
		t.Fatalf("dispatch calls hire/manual = %d/%d, want 0/1", disp.hireCalls, disp.manualCalls)
	}
}

func TestActivate_SessionlessChiefOfStaffWithHistoryUsesManual(t *testing.T) {
	t.Parallel()
	st := memory.New()
	prior, err := activation.New(
		"a-prior",
		seedprompts.ChiefOfStaffBotID,
		[]activation.Trigger{{Kind: activation.TriggerHire}},
		time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC),
		"org-test",
	)
	if err != nil {
		t.Fatalf("build prior activation: %v", err)
	}
	if err := st.Activations.Create(context.Background(), prior); err != nil {
		t.Fatalf("create prior activation: %v", err)
	}
	disp := &fakeDispatcher{}
	svc := New(Deps{
		Repo:       st.Activations,
		Now:        func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID:      func() string { return "retry" },
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
		Sessions:   fakeSessions{},
	})

	res, err := svc.Activate(context.Background(), "org-test", seedprompts.ChiefOfStaffBotID)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if disp.hireCalls != 0 || disp.manualCalls != 1 {
		t.Fatalf("dispatch calls hire/manual = %d/%d, want 0/1", disp.hireCalls, disp.manualCalls)
	}
	row, err := st.Activations.Get(context.Background(), "org-test", res.ActivationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(row.Triggers) != 1 || row.Triggers[0].Kind != activation.TriggerManual {
		t.Fatalf("triggers = %+v, want [manual]", row.Triggers)
	}
}

func TestActivate_EstablishedChiefOfStaffSessionUsesManual(t *testing.T) {
	t.Parallel()
	st := memory.New()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		Repo:       st.Activations,
		Now:        func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID:      func() string { return "manual" },
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
		Sessions:   fakeSessions{id: "ses-1"},
	})

	res, err := svc.Activate(context.Background(), "org-test", seedprompts.ChiefOfStaffBotID)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if res.SessionID != "ses-1" {
		t.Fatalf("session id = %q, want ses-1", res.SessionID)
	}
	if disp.hireCalls != 0 || disp.manualCalls != 1 {
		t.Fatalf("dispatch calls hire/manual = %d/%d, want 0/1", disp.hireCalls, disp.manualCalls)
	}
	row, err := st.Activations.Get(context.Background(), "org-test", res.ActivationID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(row.Triggers) != 1 || row.Triggers[0].Kind != activation.TriggerManual {
		t.Fatalf("triggers = %+v, want [manual]", row.Triggers)
	}
}

func TestActivate_SessionLookupErrorUsesManual(t *testing.T) {
	t.Parallel()
	disp := &fakeDispatcher{}
	svc := New(Deps{
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
		Sessions:   fakeSessions{err: errors.New("lookup failed")},
	})

	if _, err := svc.Activate(context.Background(), "org-test", seedprompts.ChiefOfStaffBotID); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if disp.hireCalls != 0 || disp.manualCalls != 1 {
		t.Fatalf("dispatch calls hire/manual = %d/%d, want 0/1", disp.hireCalls, disp.manualCalls)
	}
}

type fakeStopper struct{ called string }

func (f *fakeStopper) StopDesktop(_ context.Context, sessionID string) error {
	f.called = sessionID
	return nil
}

type fakeResetter struct {
	called string
	org    string
	bot    orgchart.NodeID
}

type fakeCanceller struct {
	called bool
	events *[]string
}

func (f *fakeCanceller) CancelOutstanding(_ context.Context, _ string, _ orgchart.NodeID) error {
	f.called = true
	if f.events != nil {
		*f.events = append(*f.events, "cancel")
	}
	return nil
}

func (f *fakeResetter) ResetSession(_ context.Context, orgID string, workerID orgchart.NodeID, sessionID string) error {
	f.called = sessionID
	f.org = orgID
	f.bot = workerID
	return nil
}

func TestStop_NoSessionIsNoop(t *testing.T) {
	t.Parallel()
	stopper := &fakeStopper{}
	canceller := &fakeCanceller{}
	svc := New(Deps{
		Sessions:  fakeSessions{id: ""},
		Stopper:   stopper,
		Canceller: canceller,
	})
	res, err := svc.Stop(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if res.Stopped {
		t.Fatal("expected Stopped=false when no session")
	}
	if stopper.called != "" {
		t.Fatalf("StopDesktop called with %q, want no call", stopper.called)
	}
	if !canceller.called {
		t.Fatal("outstanding activations were not cancelled")
	}
}

func TestStop_StopsDesktop(t *testing.T) {
	t.Parallel()
	stopper := &fakeStopper{}
	svc := New(Deps{
		Sessions: fakeSessions{id: "ses_1"},
		Stopper:  stopper,
	})
	res, err := svc.Stop(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !res.Stopped || res.SessionID != "ses_1" {
		t.Fatalf("got %+v, want stopped ses_1", res)
	}
	if stopper.called != "ses_1" {
		t.Fatalf("StopDesktop called with %q, want ses_1", stopper.called)
	}
}

func TestRestart_ResetsThenActivates(t *testing.T) {
	t.Parallel()
	events := []string{}
	disp := &fakeDispatcher{events: &events}
	resetter := &fakeResetter{}
	canceller := &fakeCanceller{events: &events}
	svc := New(Deps{
		Repo:       memory.New().Activations,
		Now:        func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID:      func() string { return "restart" },
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
		Sessions:   fakeSessions{id: "ses_old"},
		Resetter:   resetter,
		Canceller:  canceller,
	})
	res, err := svc.Restart(context.Background(), "org-test", seedprompts.ChiefOfStaffBotID)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if resetter.called != "ses_old" || resetter.bot != seedprompts.ChiefOfStaffBotID {
		t.Fatalf("resetter = %+v, want ses_old / chief-of-staff", resetter)
	}
	if !canceller.called {
		t.Fatal("outstanding activations were not cancelled before restart")
	}
	if len(events) != 2 || events[0] != "cancel" || events[1] != "dispatch" {
		t.Fatalf("restart order = %v, want [cancel dispatch]", events)
	}
	if res.ActivationID != "a-restart" {
		t.Fatalf("activation id = %q, want a-restart", res.ActivationID)
	}
	if disp.gotID != "a-restart" {
		t.Fatalf("dispatcher got %q, want a-restart", disp.gotID)
	}
	if disp.manualCalls != 1 || disp.hireCalls != 0 {
		t.Fatalf("dispatch calls manual/hire = %d/%d, want 1/0", disp.manualCalls, disp.hireCalls)
	}
}

func TestRestart_NeverStartedUsesHire(t *testing.T) {
	t.Parallel()
	disp := &fakeDispatcher{}
	resetter := &fakeResetter{}
	svc := New(Deps{
		Repo:       memory.New().Activations,
		Now:        func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID:      func() string { return "restart-first" },
		Ensurer:    fakeEnsurer{},
		Dispatcher: disp,
		Sessions:   fakeSessions{},
		Resetter:   resetter,
	})

	if _, err := svc.Restart(context.Background(), "org-test", seedprompts.ChiefOfStaffBotID); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if resetter.called != "" {
		t.Fatalf("reset session = %q, want no reset", resetter.called)
	}
	if disp.hireCalls != 1 || disp.manualCalls != 0 {
		t.Fatalf("dispatch calls hire/manual = %d/%d, want 1/0", disp.hireCalls, disp.manualCalls)
	}
}
