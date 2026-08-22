package dispatch_test

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/internal/orgtest"
)

// newDispatcher returns a Dispatcher with a no-op spawner and a
// discard logger; callers wire in a fresh in-memory store.
func newDispatcher(t *testing.T) (*dispatch.Dispatcher, *store.Store) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)
	d := dispatch.New(s, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return d, s
}

// recordedActivation captures one Spawner invocation for assertions.
type recordedActivation struct {
	NodeID   orgchart.NodeID
	Triggers []activation.Trigger
}

// newDispatcherWithSpawner returns a Dispatcher whose Spawner records
// each activation onto a buffered channel. Tests use this to assert
// who was activated (and not activated) for a given Dispatch call.
func newDispatcherWithSpawner(t *testing.T) (*dispatch.Dispatcher, *store.Store, <-chan recordedActivation) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)
	rec := make(chan recordedActivation, 16)
	spawner := runtime.Spawner(func(_ context.Context, _ string, botID orgchart.NodeID, triggers []activation.Trigger) error {
		rec <- recordedActivation{NodeID: botID, Triggers: triggers}
		return nil
	})
	d := dispatch.New(s, spawner, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return d, s, rec
}

// drainActivations collects every recorded activation that lands within
// window, then returns them sorted by WorkerID for stable assertions.
// A negative timeout uses 200ms — enough for the dispatcher's
// goroutines to settle but short enough not to slow the suite.
func drainActivations(t *testing.T, rec <-chan recordedActivation, window time.Duration) []recordedActivation {
	t.Helper()
	if window <= 0 {
		window = 200 * time.Millisecond
	}
	deadline := time.After(window)
	var got []recordedActivation
	for {
		select {
		case r := <-rec:
			got = append(got, r)
		case <-deadline:
			sort.Slice(got, func(i, j int) bool { return got[i].NodeID < got[j].NodeID })
			return got
		}
	}
}

// seedBot creates a Bot and persists it.
func seedBot(t *testing.T, s *store.Store, botID orgchart.NodeID) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	b, err := orgchart.NewNode(botID, "# "+string(botID)+"\nTest persona.", nil, now, "org-test")
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	if err := s.Nodes.Create(ctx, b); err != nil {
		t.Fatalf("create bot: %v", err)
	}
}

// seedAttachment attaches a Worker to a Trigger.
func seedAttachment(t *testing.T, s *store.Store, botID orgchart.NodeID, triggerID string) {
	t.Helper()
	if _, err := s.Nodes.Get(context.Background(), "org-test", botID); err != nil {
		t.Fatalf("get bot %q for attachment: %v", botID, err)
	}
	orgtest.AttachTrigger(t, s, "org-test", botID, triggerID)
}

// seedTrigger creates a local Trigger with the given id.
func seedTrigger(t *testing.T, s *store.Store, id string) {
	t.Helper()
	orgtest.Trigger(t, s, "org-test", id)
}

// routeEvent builds the routed envelope for an appended event and hands
// it to the dispatcher, mirroring what publishing does in production.
func routeEvent(t *testing.T, d *dispatch.Dispatcher, triggerID string, e streaming.Event) {
	t.Helper()
	msg, err := e.Message()
	if err != nil {
		// The bad-body test deliberately routes a non-Message event; it
		// asserts on the fan-out, so pass an empty message through.
		msg = streaming.Message{}
	}
	routed, err := eventsource.NewEvent(e.ID, e.OrganizationID, eventsource.Trigger(triggerID), msg, e.Source, e.CreatedAt)
	if err != nil {
		t.Fatalf("build routed event: %v", err)
	}
	if err := d.Route(context.Background(), routed); err != nil {
		t.Fatalf("route: %v", err)
	}
}

// eventCounter monotonically generates unique IDs for test events,
// independent of the body. Bodies in some tests contain control bytes
// or non-ASCII that would otherwise leak into the X-Helix-Event header.
var eventCounter atomic.Uint64

// TestDispatchSkipsPublisher pins the rule that an AI Worker which
// publishes to a Topic they themselves are subscribed to is NOT
// re-activated on their own event. This is the cheapest available
// brake on broadcast cascades — without it, a single publish would
// activate the publisher in a loop. Other subscribers are still
// activated normally.
func TestDispatchSkipsPublisher(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)
	seedTrigger(t, s, "s-team")
	seedBot(t, s, "w-publisher")
	seedBot(t, s, "w-other")
	seedAttachment(t, s, "w-publisher", "s-team")
	seedAttachment(t, s, "w-other", "s-team")

	e, err := streaming.NewMessageEvent(
		"e-1", "s-team", "w-publisher",
		streaming.Message{From: "w-publisher", Body: "hello"},
		time.Now().UTC(),
		"org-test",
	)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	if err := s.Events.Append(context.Background(), e); err != nil {
		t.Fatalf("append event: %v", err)
	}
	routeEvent(t, d, "s-team", e)

	got := drainActivations(t, rec, 0)
	if len(got) != 1 {
		t.Fatalf("activations = %d, want 1; got %+v", len(got), got)
	}
	if got[0].NodeID != "w-other" {
		t.Fatalf("activated bot = %q, want w-other", got[0].NodeID)
	}
}

// TestDispatchDeliversEventsOneAtATime pins the context-bounding rule
// that drove this design: while one activation is in flight for a
// Worker, any further events that arrive on Topics that Worker
// subscribes to are appended to a per-Worker queue and delivered to
// the Spawner one trigger per activation, in arrival order — never
// folded into one oversized batch that would blow the Worker's context
// budget.
//
// Shape of the test: the spawner blocks on the very first call so we
// can publish more events behind it, then we release it and assert
// each queued event drains in its own Spawner call, FIFO.
func TestDispatchDeliversEventsOneAtATime(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)
	rec := make(chan recordedActivation, 8)

	// First Spawner call gates on `release` so the test can stack more
	// events behind it; subsequent calls return immediately. The atomic
	// counter is what makes "first" deterministic across the runner's
	// retry loop.
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	spawner := runtime.Spawner(func(_ context.Context, _ string, botID orgchart.NodeID, triggers []activation.Trigger) error {
		n := calls.Add(1)
		if n == 1 {
			close(started)
			<-release
		}
		// Copy the slice so a later mutation in the dispatcher (it doesn't
		// today, but defensive) can't race with the assertion read.
		copied := make([]activation.Trigger, len(triggers))
		copy(copied, triggers)
		rec <- recordedActivation{NodeID: botID, Triggers: copied}
		return nil
	})
	d := dispatch.New(s, spawner, slog.New(slog.NewTextHandler(io.Discard, nil)))

	seedTrigger(t, s, "s-team")
	seedBot(t, s, "w-eng")
	seedAttachment(t, s, "w-eng", "s-team")

	publish := func(id, body string) {
		ev, err := streaming.NewMessageEvent(
			streaming.EventID(id), "s-team", "w-other",
			streaming.Message{From: "w-other", Body: body},
			time.Now().UTC(),
			"org-test",
		)
		if err != nil {
			t.Fatalf("new event: %v", err)
		}
		if err := s.Events.Append(context.Background(), ev); err != nil {
			t.Fatalf("append event: %v", err)
		}
		routeEvent(t, d, "s-team", ev)
	}

	// First event kicks off activation #1; the spawner blocks inside it.
	publish("e-1", "first")
	<-started

	// Three more events while activation #1 is held. Each should drain
	// in its own Spawner call once activation #1 returns — never pooled
	// into one batch.
	publish("e-2", "two")
	publish("e-3", "three")
	publish("e-4", "four")

	// Give the dispatcher's enqueue goroutines a tick to land. The lock
	// inside enqueue is uncontended once Dispatch returns, but the
	// goroutines that resolve subs/env can still be in flight.
	time.Sleep(100 * time.Millisecond)

	// Release the first activation; the runner now drains the queue one
	// trigger at a time.
	close(release)

	// Four Spawner calls total, each carrying a single event in FIFO
	// order: [e-1], [e-2], [e-3], [e-4].
	wantIDs := []streaming.EventID{"e-1", "e-2", "e-3", "e-4"}
	for i, want := range wantIDs {
		a := waitForActivation(t, rec, 2*time.Second)
		if len(a.Triggers) != 1 {
			t.Fatalf("activation #%d = %d triggers %+v, want exactly 1", i+1, len(a.Triggers), eventIDs(a.Triggers))
		}
		if a.Triggers[0].EventID != want {
			t.Fatalf("activation #%d event = %q, want %q (FIFO order broken)", i+1, a.Triggers[0].EventID, want)
		}
	}

	// And no fifth activation is fired — the runner exits cleanly when
	// the queue drains.
	select {
	case extra := <-rec:
		t.Fatalf("unexpected fifth activation: %+v", extra)
	case <-time.After(150 * time.Millisecond):
	}

	if got := calls.Load(); got != 4 {
		t.Fatalf("Spawner calls = %d, want 4 (one per event)", got)
	}
}

// waitForActivation pulls one recordedActivation off rec or fails the
// test on timeout. Centralised so the one-at-a-time test reads cleanly.
func waitForActivation(t *testing.T, rec <-chan recordedActivation, timeout time.Duration) recordedActivation {
	t.Helper()
	select {
	case got := <-rec:
		return got
	case <-time.After(timeout):
		t.Fatalf("no activation within %s", timeout)
		return recordedActivation{}
	}
}

func eventIDs(ts []activation.Trigger) []streaming.EventID {
	out := make([]streaming.EventID, len(ts))
	for i, t := range ts {
		out[i] = t.EventID
	}
	return out
}

// TestRouteRejectsUnroutableEvent pins where the "don't fan out a
// half-formed event" guard lives after the cutover. It moved off the
// dispatcher: publishing decodes the canonical Message and builds the
// routed envelope, and eventsource.NewEvent refuses an empty one — so a
// malformed body can never reach Route at all. What Route still owns is
// refusing an event whose source names nothing routable.
func TestRouteRejectsUnroutableEvent(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)
	seedTrigger(t, s, "s-bad")
	seedBot(t, s, "w-listener")
	seedAttachment(t, s, "w-listener", "s-bad")

	err := d.Route(context.Background(), eventsource.Event{
		ID:             "e-bad",
		OrganizationID: "org-test",
		Message:        streaming.Message{Body: "hello"},
		CreatedAt:      time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("Route with no source kind must fail")
	}
	if got := drainActivations(t, rec, 100*time.Millisecond); len(got) != 0 {
		t.Fatalf("activations = %d, want 0; got %+v", len(got), got)
	}
}

// TestEventsourceRejectsNonMessageBody pins the guard that replaced the
// dispatcher's strict-parse: an event whose body is not canonical
// Message JSON produces an empty Message, and building the routed
// envelope from it fails rather than fanning out a half-rendered
// activation prompt.
func TestEventsourceRejectsNonMessageBody(t *testing.T) {
	t.Parallel()
	raw, err := streaming.NewEvent("e-bad", "s-bad", "w-author", "not-json-payload", time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	msg, _ := raw.Message()
	if _, err := eventsource.NewEvent(raw.ID, raw.OrganizationID, eventsource.Trigger("s-bad"), msg, raw.Source, raw.CreatedAt); err == nil {
		t.Fatal("a non-Message body must not produce a routable event")
	}
}
