package streamcron

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

// recordingPublisher is the real publish use case wired to the same
// store, wrapped so tests can assert what the scheduler published
// without booting a dispatcher.
type recordingPublisher struct {
	inner *publishing.Publishing
	mu    sync.Mutex
	calls []string
}

func (p *recordingPublisher) PublishToTrigger(ctx context.Context, orgID, triggerID, from string, msg streaming.Message) (streaming.Event, error) {
	p.mu.Lock()
	p.calls = append(p.calls, triggerID)
	p.mu.Unlock()
	return p.inner.PublishToTrigger(ctx, orgID, triggerID, from, msg)
}

func (p *recordingPublisher) snapshot() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.calls))
	copy(out, p.calls)
	return out
}

// newTestScheduler wires a Scheduler against the memory store + a
// recording publisher. Returns the scheduler and the publisher (for
// assertions); the store is reachable as s.store.
func newTestScheduler(t *testing.T) (*Scheduler, *recordingPublisher) {
	t.Helper()
	st := memory.New()
	// Deterministic id + clock so events are reproducible.
	idCounter := 0
	newID := func() string {
		idCounter++
		return "test-id"
	}
	fixedNow := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return fixedNow }
	pub := &recordingPublisher{inner: publishing.New(publishing.Deps{Triggers: st.Triggers, Events: st.Events, Now: now, NewID: newID})}
	s, err := New(st, pub, newID, now)
	if err != nil {
		t.Fatalf("New scheduler: %v", err)
	}
	return s, pub
}

func makeCronTrigger(t *testing.T, s *Scheduler, orgID, triggerID, schedule string) {
	makeCronTriggerWithMessage(t, s, orgID, triggerID, schedule, "")
}

func makeCronTriggerWithMessage(t *testing.T, s *Scheduler, orgID, triggerID, schedule, message string) {
	t.Helper()
	cfg, err := json.Marshal(transport.CronConfig{Schedule: schedule, Message: message})
	if err != nil {
		t.Fatalf("marshal cron config: %v", err)
	}
	// Use triggerID as the name suffix so multiple Triggers in the same
	// org satisfy the per-org name uniqueness constraint.
	row, err := trigger.New(triggerID, orgID, "cron-"+triggerID, "scheduled trigger", transport.KindCron, cfg, "w-owner", time.Now().UTC())
	if err != nil {
		t.Fatalf("trigger.New: %v", err)
	}
	if err := s.store.Triggers.Create(context.Background(), row); err != nil {
		t.Fatalf("Triggers.Create: %v", err)
	}
}

// getTrigger reads one Trigger back for the mutate-and-write-back tests.
func getTrigger(t *testing.T, s *Scheduler, orgID, id string) trigger.Trigger {
	t.Helper()
	rows, err := s.store.Triggers.Find(context.Background(), store.WithOrg(orgID), store.WithID(id), store.WithLimit(1))
	if err != nil || len(rows) == 0 {
		t.Fatalf("get trigger %q: %v", id, err)
	}
	return rows[0]
}

func TestFireUsesConfiguredMessage(t *testing.T) {
	t.Parallel()

	s, pub := newTestScheduler(t)
	makeCronTriggerWithMessage(t, s, "org-test", "s-cron", "@daily", "Prepare the daily status report")

	if err := s.fire(context.Background(), "org-test", "s-cron"); err != nil {
		t.Fatalf("fire: %v", err)
	}
	events, err := s.store.Events.ListForStream(context.Background(), "org-test", "s-cron", 10)
	if err != nil {
		t.Fatalf("ListForStream: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events on stream = %d, want 1", len(events))
	}
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("Message: %v", err)
	}
	if msg.Body != "Prepare the daily status report" {
		t.Fatalf("Body = %q, want configured message", msg.Body)
	}
	if msg.BodyContentType != "text/plain" {
		t.Fatalf("BodyContentType = %q, want text/plain", msg.BodyContentType)
	}
	if got := pub.snapshot(); len(got) != 1 || got[0] != "s-cron" {
		t.Fatalf("published = %v, want [s-cron]", got)
	}
}

// TestFirePublishesEvent proves the fire() path invariant — every tick
// goes through the shared publish use case, which appends the event and
// routes it. This is what makes attached Workers wake up; if it breaks,
// cron Triggers stop activating.
func TestFirePublishesEvent(t *testing.T) {
	t.Parallel()

	s, pub := newTestScheduler(t)
	makeCronTrigger(t, s, "org-test", "s-cron", "@daily")

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

	// The publish went to the Trigger's own stream.
	published := pub.snapshot()
	if len(published) != 1 || published[0] != "s-cron" {
		t.Fatalf("published = %v, want [s-cron]", published)
	}
}

// TestReconcileSchedulesCronTriggers verifies that running reconcile
// against a cron Trigger creates a gocron.Job — the prerequisite for
// any future tick.
func TestReconcileSchedulesCronTriggers(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTrigger(t, s, "org-a", "s-1", "@hourly")
	makeCronTrigger(t, s, "org-b", "s-2", "0 9 * * 1")

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

// TestReconcileRemovesDeletedTriggers proves that when a cron Trigger
// disappears from the store, the next reconcile drops its gocron.Job —
// no zombie ticks after delete.
func TestReconcileRemovesDeletedTriggers(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTrigger(t, s, "org-a", "s-keep", "@hourly")
	makeCronTrigger(t, s, "org-a", "s-drop", "@daily")
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if got := len(s.scheduler.Jobs()); got != 2 {
		t.Fatalf("jobs after seed = %d, want 2", got)
	}

	if err := s.store.Triggers.Delete(context.Background(), "org-a", "s-drop"); err != nil {
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
// path — when a Trigger's config changes, reconcile picks up
// the new cadence within one cycle (≤ 10s in production).
func TestReconcileUpdatesChangedSchedule(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTrigger(t, s, "org-a", "s-1", "@hourly")
	if err := s.reconcile(context.Background()); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if got := jobSchedule(s.scheduler.Jobs()[0]); got != "@hourly" {
		t.Fatalf("initial schedule = %q, want @hourly", got)
	}

	// Read, mutate, write back. Update replaces transport_config
	// wholesale so the new schedule appears on next reconcile.
	row := getTrigger(t, s, "org-a", "s-1")
	newCfg, err := json.Marshal(transport.CronConfig{Schedule: "@daily"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	row.Config = newCfg
	if err := s.store.Triggers.Update(context.Background(), row); err != nil {
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
// config in the memory store, since trigger.New guards the front door.
func TestReconcileSkipsInvalidSchedule(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	makeCronTrigger(t, s, "org-a", "s-bad", "@hourly")
	// Replace the row's transport config wholesale with a sub-90s
	// schedule. The memory Update doesn't re-validate the transport,
	// matching the production gorm Update — exactly the case
	// reconcile's defensive Validate guards against.
	row := getTrigger(t, s, "org-a", "s-bad")
	row.Config = json.RawMessage(`{"schedule":"* * * * *"}`)
	if err := s.store.Triggers.Update(context.Background(), row); err != nil {
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
// publisher (or any panic in the fire path) does NOT take down the
// scheduler goroutine.
func TestFirePanicRecovery(t *testing.T) {
	t.Parallel()

	s, _ := newTestScheduler(t)
	s.publisher = panickyPublisher{}
	makeCronTrigger(t, s, "org-a", "s-1", "@hourly")

	// fireFn wraps fire in recover; invoking it directly must not
	// propagate the panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("fireFn propagated panic: %v", r)
		}
	}()
	s.fireFn("org-a", "s-1")()
}

type panickyPublisher struct{}

func (panickyPublisher) PublishToTrigger(_ context.Context, _, _, _ string, _ streaming.Message) (streaming.Event, error) {
	panic("simulated publisher failure")
}
