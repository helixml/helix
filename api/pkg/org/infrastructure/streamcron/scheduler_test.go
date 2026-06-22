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
// inserting/updating topics), and the dispatcher (for assertions).
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

func makeCronTopic(t *testing.T, s *Scheduler, orgID string, topicID streaming.TopicID, schedule string) {
	t.Helper()
	cfg, err := json.Marshal(transport.CronConfig{Schedule: schedule})
	if err != nil {
		t.Fatalf("marshal cron config: %v", err)
	}
	tp := transport.Transport{Kind: transport.KindCron, Config: cfg}
	// Use topicID as the name suffix so multiple topics in the same
	// org satisfy the per-org name uniqueness constraint.
	topic, err := streaming.NewTopic(topicID, "cron-"+string(topicID), "scheduled trigger", "w-owner", time.Now().UTC(), tp, orgID)
	if err != nil {
		t.Fatalf("NewTopic: %v", err)
	}
	if err := s.store.Topics.Create(context.Background(), topic); err != nil {
		t.Fatalf("Topics.Create: %v", err)
	}
}

// TestFirePublishesEventAndDispatches proves the fire() path
// invariant — every tick appends an event and hands it to the
// dispatcher. This is what makes subscribed Workers wake up; if this
// breaks, cron topics stop activating.
func TestFirePublishesEventAndDispatches(t *testing.T) {
	t.Parallel()

	s, disp := newTestScheduler(t)
	makeCronTopic(t, s, "org-test", "s-cron", "@daily")

	if err := s.fire(context.Background(), "org-test", "s-cron"); err != nil {
		t.Fatalf("fire: %v", err)
	}

	// Event was appended to the store.
	events, err := s.store.Events.ListForTopic(context.Background(), "org-test", "s-cron", 10)
	if err != nil {
		t.Fatalf("ListForTopic: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events on topic = %d, want 1", len(events))
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
	if dispatched[0].TopicID != "s-cron" {
		t.Fatalf("dispatched TopicID = %q, want s-cron", dispatched[0].TopicID)
	}
}

// TestReconcileSchedulesCronTopics verifies that running reconcile
// against a cron topic creates a gocron.Job — the prerequisite for
// any future tick.
func TestReconcileSchedulesCronTopics(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTopic(t, s, "org-a", "s-1", "@hourly")
	makeCronTopic(t, s, "org-b", "s-2", "0 9 * * 1")

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

// TestReconcileRemovesDeletedTopics proves that when a cron topic
// disappears from the store, the next reconcile drops its gocron.Job —
// no zombie ticks after delete.
func TestReconcileRemovesDeletedTopics(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTopic(t, s, "org-a", "s-keep", "@hourly")
	makeCronTopic(t, s, "org-a", "s-drop", "@daily")
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if got := len(s.scheduler.Jobs()); got != 2 {
		t.Fatalf("jobs after seed = %d, want 2", got)
	}

	if err := s.store.Topics.Delete(context.Background(), "org-a", "s-drop"); err != nil {
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
// path — when a topic's TransportConfig changes, reconcile picks up
// the new cadence within one cycle (≤ 10s in production).
func TestReconcileUpdatesChangedSchedule(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTopic(t, s, "org-a", "s-1", "@hourly")
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if got := jobSchedule(s.scheduler.Jobs()[0]); got != "@hourly" {
		t.Fatalf("initial schedule = %q, want @hourly", got)
	}

	// Read, mutate, write back. Update replaces transport_config
	// wholesale so the new schedule appears on next reconcile.
	topic, err := s.store.Topics.Get(context.Background(), "org-a", "s-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	newCfg, err := json.Marshal(transport.CronConfig{Schedule: "@daily"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	topic.Transport.Config = newCfg
	if err := s.store.Topics.Update(context.Background(), topic); err != nil {
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
// registered. Constructed by mutating an existing topic's transport
// config in the memory store, since NewTopic guards the front door.
func TestReconcileSkipsInvalidSchedule(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTopic(t, s, "org-a", "s-bad", "@hourly")
	// Replace the row's transport config wholesale with a sub-90s
	// schedule. The memory Update doesn't re-validate the transport,
	// matching the production gorm Update — exactly the case
	// reconcile's defensive Validate guards against.
	topic, err := s.store.Topics.Get(context.Background(), "org-a", "s-bad")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	topic.Transport.Config = json.RawMessage(`{"schedule":"* * * * *"}`)
	if err := s.store.Topics.Update(context.Background(), topic); err != nil {
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
	makeCronTopic(t, s, "org-a", "s-1", "@hourly")

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
