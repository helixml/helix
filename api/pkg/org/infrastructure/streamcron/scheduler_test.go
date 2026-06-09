package streamcron

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// recordingDispatcher captures every event passed to Dispatch so tests
// can assert on fan-out without booting a real dispatcher.
type recordingDispatcher struct {
	mu     sync.Mutex
	events []streaming.Event
}

func (d *recordingDispatcher) Dispatch(_ context.Context, e streaming.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
}

func (d *recordingDispatcher) snapshot() []streaming.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]streaming.Event, len(d.events))
	copy(out, d.events)
	return out
}

// newTestScheduler wires a Scheduler against the memory store + a
// recording dispatcher. Returns the scheduler, the store (for
// inserting/updating streams), and the dispatcher (for assertions).
func newTestScheduler(t *testing.T) (*Scheduler, *recordingDispatcher) {
	t.Helper()
	st := memory.New()
	disp := &recordingDispatcher{}
	// Deterministic id + clock so events are reproducible.
	idCounter := 0
	newID := func() string {
		idCounter++
		return "test-id"
	}
	fixedNow := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return fixedNow }
	s, err := New(st, nil, disp, newID, now)
	if err != nil {
		t.Fatalf("New scheduler: %v", err)
	}
	return s, disp
}

func makeCronStream(t *testing.T, s *Scheduler, orgID string, streamID streaming.StreamID, schedule string) {
	t.Helper()
	cfg, err := json.Marshal(transport.CronConfig{Schedule: schedule})
	if err != nil {
		t.Fatalf("marshal cron config: %v", err)
	}
	tp := transport.Transport{Kind: transport.KindCron, Config: cfg}
	// Use streamID as the name suffix so multiple streams in the same
	// org satisfy the per-org name uniqueness constraint.
	stream, err := streaming.NewStream(streamID, "cron-"+string(streamID), "scheduled trigger", "w-owner", time.Now().UTC(), tp, orgID)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := s.store.Streams.Create(context.Background(), stream); err != nil {
		t.Fatalf("Streams.Create: %v", err)
	}
}

// TestFirePublishesEventAndDispatches proves the fire() path
// invariant — every tick appends an event and hands it to the
// dispatcher. This is what makes subscribed Workers wake up; if this
// breaks, cron streams stop activating.
func TestFirePublishesEventAndDispatches(t *testing.T) {
	t.Parallel()

	s, disp := newTestScheduler(t)
	makeCronStream(t, s, "org-test", "s-cron", "@daily")

	if err := s.fire(context.Background(), "org-test", "s-cron"); err != nil {
		t.Fatalf("fire: %v", err)
	}

	// Event was appended to the store.
	events, err := s.store.Events.ListForStream(context.Background(), "org-test", "s-cron", 10)
	if err != nil {
		t.Fatalf("ListForStream: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events on stream = %d, want 1", len(events))
	}
	// System-emitted events have empty Source.
	if events[0].Source != "" {
		t.Fatalf("Source = %q, want empty (system-emitted)", events[0].Source)
	}
	// Body parses as a Message whose body is the canonical scheduledBody JSON.
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if !strings.Contains(msg.Body, `"kind":"scheduled"`) {
		t.Fatalf("Body = %q, want kind:scheduled", msg.Body)
	}

	// Dispatcher saw the same event.
	dispatched := disp.snapshot()
	if len(dispatched) != 1 {
		t.Fatalf("dispatched events = %d, want 1", len(dispatched))
	}
	if dispatched[0].StreamID != "s-cron" {
		t.Fatalf("dispatched StreamID = %q, want s-cron", dispatched[0].StreamID)
	}
}

// TestReconcileSchedulesCronStreams verifies that running reconcile
// against a cron stream creates a gocron.Job — the prerequisite for
// any future tick.
func TestReconcileSchedulesCronStreams(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronStream(t, s, "org-a", "s-1", "@hourly")
	makeCronStream(t, s, "org-b", "s-2", "0 9 * * 1")

	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	jobs := s.scheduler.Jobs()
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	names := map[string]bool{}
	for _, j := range jobs {
		names[j.Name()] = true
	}
	if !names["org-a:s-1"] || !names["org-b:s-2"] {
		t.Fatalf("missing expected jobs: %v", names)
	}
}

// TestReconcileRemovesDeletedStreams proves that when a cron stream
// disappears from the store, the next reconcile drops its gocron.Job —
// no zombie ticks after delete.
func TestReconcileRemovesDeletedStreams(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronStream(t, s, "org-a", "s-keep", "@hourly")
	makeCronStream(t, s, "org-a", "s-drop", "@daily")
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if got := len(s.scheduler.Jobs()); got != 2 {
		t.Fatalf("jobs after seed = %d, want 2", got)
	}

	if err := s.store.Streams.Delete(context.Background(), "org-a", "s-drop"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("post-delete reconcile: %v", err)
	}

	jobs := s.scheduler.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("jobs after drop = %d, want 1", len(jobs))
	}
	if jobs[0].Name() != "org-a:s-keep" {
		t.Fatalf("remaining job = %q, want org-a:s-keep", jobs[0].Name())
	}
}

// TestReconcileUpdatesChangedSchedule verifies the schedule-change
// path — when a stream's TransportConfig changes, reconcile picks up
// the new cadence within one cycle (≤ 10s in production).
func TestReconcileUpdatesChangedSchedule(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronStream(t, s, "org-a", "s-1", "@hourly")
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if got := jobSchedule(s.scheduler.Jobs()[0]); got != "@hourly" {
		t.Fatalf("initial schedule = %q, want @hourly", got)
	}

	// Read, mutate, write back. Update replaces transport_config
	// wholesale so the new schedule appears on next reconcile.
	stream, err := s.store.Streams.Get(context.Background(), "org-a", "s-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	newCfg, err := json.Marshal(transport.CronConfig{Schedule: "@daily"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	stream.Transport.Config = newCfg
	if err := s.store.Streams.Update(context.Background(), stream); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("post-update reconcile: %v", err)
	}
	jobs := s.scheduler.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("jobs after update = %d, want 1", len(jobs))
	}
	if got := jobSchedule(jobs[0]); got != "@daily" {
		t.Fatalf("updated schedule = %q, want @daily", got)
	}
}

// TestReconcileSkipsInvalidSchedule shows that a row whose schedule
// no longer validates (e.g. a sub-90s config snuck in via SQL after
// initial creation) is logged and skipped — no panic, no job
// registered. Constructed by mutating an existing stream's transport
// config in the memory store, since NewStream guards the front door.
func TestReconcileSkipsInvalidSchedule(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronStream(t, s, "org-a", "s-bad", "@hourly")
	// Replace the row's transport config wholesale with a sub-90s
	// schedule. The memory Update doesn't re-validate the transport,
	// matching the production gorm Update — exactly the case
	// reconcile's defensive Validate guards against.
	stream, err := s.store.Streams.Get(context.Background(), "org-a", "s-bad")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	stream.Transport.Config = json.RawMessage(`{"schedule":"* * * * *"}`)
	if err := s.store.Streams.Update(context.Background(), stream); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := len(s.scheduler.Jobs()); got != 0 {
		t.Fatalf("jobs after invalid schedule = %d, want 0", got)
	}
}

// TestFirePanicRecovery proves the recover() in fireFn — a bad
// dispatcher (or any panic in the fire path) does NOT take down the
// scheduler goroutine.
func TestFirePanicRecovery(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	s.dispatcher = panickyDispatcher{}
	makeCronStream(t, s, "org-a", "s-1", "@hourly")

	// fireFn wraps fire in recover; invoking it directly must not
	// propagate the panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("fireFn propagated panic: %v", r)
		}
	}()
	s.fireFn("org-a", "s-1")()
}

type panickyDispatcher struct{}

func (panickyDispatcher) Dispatch(_ context.Context, _ streaming.Event) {
	panic("simulated dispatcher failure")
}
