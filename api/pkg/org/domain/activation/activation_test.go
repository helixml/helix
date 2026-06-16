package activation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// TestNewActivationFromHireTrigger pins the happy-path constructor
// shape. New(...) sets StartedAt, derives TranscriptID from
// WorkerID, copies Triggers, leaves Outcome zero and EndedAt nil so
// callers can tell the row is "still running."
func TestNewActivationFromHireTrigger(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	wid := orgchart.WorkerID("w-alice")
	triggers := []activation.Trigger{{Kind: activation.TriggerHire}}

	a, err := activation.New("a-1", wid, triggers, started, "org-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.ID != "a-1" {
		t.Errorf("ID = %q, want %q", a.ID, "a-1")
	}
	if a.WorkerID != wid {
		t.Errorf("WorkerID = %q, want %q", a.WorkerID, wid)
	}
	if a.StartedAt != started {
		t.Errorf("StartedAt = %v, want %v", a.StartedAt, started)
	}
	if a.EndedAt != nil {
		t.Errorf("EndedAt = %v, want nil for a still-running activation", a.EndedAt)
	}
	if a.Outcome != (activation.Outcome{}) {
		t.Errorf("Outcome = %+v, want zero (Complete hasn't fired)", a.Outcome)
	}
	if a.TranscriptID != activation.TranscriptID(wid) {
		t.Errorf("TranscriptID = %q, want %q (derived from WorkerID)", a.TranscriptID, activation.TranscriptID(wid))
	}
	if len(a.Triggers) != 1 || a.Triggers[0].Kind != activation.TriggerHire {
		t.Errorf("Triggers = %+v, want one hire trigger", a.Triggers)
	}
}

// TestNewRejectsEmptyTriggers — Activations are by definition the
// response to "something woke this Worker." Allowing an empty slice
// would let the Spawner be called with no context, which makes the
// transcript meaningless.
func TestNewRejectsEmptyTriggers(t *testing.T) {
	t.Parallel()
	_, err := activation.New("a-1", "w-alice", nil, time.Now(), "org-test")
	if err == nil {
		t.Fatal("New with nil triggers = nil; want error")
	}
	_, err = activation.New("a-1", "w-alice", []activation.Trigger{}, time.Now(), "org-test")
	if err == nil {
		t.Fatal("New with empty triggers = nil; want error")
	}
}

// TestNewRejectsEmptyID protects against silent collisions. Empty IDs
// would all key to the same row in storage; reject at construction.
func TestNewRejectsEmptyID(t *testing.T) {
	t.Parallel()
	_, err := activation.New("", "w-alice", []activation.Trigger{{Kind: activation.TriggerHire}}, time.Now(), "org-test")
	if err == nil {
		t.Fatal("New with empty ID = nil; want error")
	}
}

// TestNewRejectsEmptyWorker — every activation belongs to exactly one
// Worker. Empty WorkerID is meaningless.
func TestNewRejectsEmptyWorker(t *testing.T) {
	t.Parallel()
	_, err := activation.New("a-1", "", []activation.Trigger{{Kind: activation.TriggerHire}}, time.Now(), "org-test")
	if err == nil {
		t.Fatal("New with empty WorkerID = nil; want error")
	}
}

// TestCompleteSetsEndedAtAndOutcome — Spawner finishes, calls
// Complete; the aggregate moves to the terminal state.
func TestCompleteSetsEndedAtAndOutcome(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	ended := started.Add(2 * time.Minute)
	a, err := activation.New("a-1", "w-alice", []activation.Trigger{{Kind: activation.TriggerHire}}, started, "org-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Complete(activation.Outcome{Status: activation.StatusOK}, ended); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if a.EndedAt == nil || !a.EndedAt.Equal(ended) {
		t.Errorf("EndedAt = %v, want %v", a.EndedAt, ended)
	}
	if a.Outcome.Status != activation.StatusOK {
		t.Errorf("Outcome.Status = %q, want ok", a.Outcome.Status)
	}
}

// TestCompleteIsIdempotentOnSameOutcome — pin the "exactly once"
// invariant. Two Complete calls with the same outcome should be a
// no-op (helps reconciliation paths); calling with a *different*
// outcome is an error.
func TestCompleteRefusesSecondCallWithDifferentOutcome(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	ended := started.Add(time.Minute)
	a, _ := activation.New("a-1", "w-alice", []activation.Trigger{{Kind: activation.TriggerHire}}, started, "org-test")
	if err := a.Complete(activation.Outcome{Status: activation.StatusOK}, ended); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	err := a.Complete(activation.Outcome{Status: activation.StatusError, Error: "late failure"}, ended.Add(time.Second))
	if err == nil {
		t.Fatal("second Complete with different outcome = nil; want error")
	}
}

// TestCompleteRejectsEndedAtBeforeStartedAt — typed guard against
// clock skew or test wiring bugs where the end time is recorded
// before the start.
func TestCompleteRejectsEndedAtBeforeStartedAt(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	a, _ := activation.New("a-1", "w-alice", []activation.Trigger{{Kind: activation.TriggerHire}}, started, "org-test")
	if err := a.Complete(activation.Outcome{Status: activation.StatusOK}, started.Add(-time.Second)); err == nil {
		t.Fatal("Complete with endedAt before startedAt = nil; want error")
	}
}

// TestIsCompletedReportsLifecycleState — convenience for repository
// impls and consumers that want to filter "still running" vs "done."
func TestIsCompletedReportsLifecycleState(t *testing.T) {
	t.Parallel()
	a, _ := activation.New("a-1", "w-alice", []activation.Trigger{{Kind: activation.TriggerHire}}, time.Now(), "org-test")
	if a.IsCompleted() {
		t.Fatal("fresh activation reports IsCompleted=true; want false")
	}
	_ = a.Complete(activation.Outcome{Status: activation.StatusOK}, time.Now().Add(time.Second))
	if !a.IsCompleted() {
		t.Fatal("post-Complete activation reports IsCompleted=false; want true")
	}
}

// Test_Repository_IsAPort — guard against accidental concrete-type
// drift. Anything that compiles against this assertion will satisfy
// the storage seam.
func Test_Repository_IsAPort(t *testing.T) {
	t.Parallel()
	var r activation.Repository = fakeRepo{}
	_ = r
}

type fakeRepo struct{}

func (fakeRepo) Create(_ context.Context, _ *activation.Activation) error { return nil }
func (fakeRepo) Complete(_ context.Context, _ string, _ activation.ID, _ activation.Outcome, _ time.Time) error {
	return nil
}
func (fakeRepo) Get(_ context.Context, _ string, _ activation.ID) (*activation.Activation, error) {
	return nil, errors.New("not found")
}
func (fakeRepo) ListForWorker(_ context.Context, _ string, _ orgchart.WorkerID, _ int) ([]*activation.Activation, error) {
	return nil, nil
}
