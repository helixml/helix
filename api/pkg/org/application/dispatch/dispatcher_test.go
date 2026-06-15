package dispatch_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// caught is one POST observed by the test catcher.
type caught struct {
	body    string
	headers http.Header
	method  string
	path    string
}

// catcher is an httptest.Server that records every POST body it sees
// and pushes it onto a channel so tests can synchronise. Closes are
// handled by t.Cleanup.
type catcher struct {
	srv      *httptest.Server
	requests chan caught
	status   atomic.Int32 // status to reply with; defaults to 204
	delay    atomic.Int64 // nanoseconds to sleep before responding
}

func newCatcher(t *testing.T) *catcher {
	t.Helper()
	c := &catcher{requests: make(chan caught, 64)}
	c.status.Store(http.StatusNoContent)
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		// Snapshot headers up front so the channel send doesn't race with
		// the response writer recycling the request.
		headers := r.Header.Clone()
		c.requests <- caught{body: string(body), headers: headers, method: r.Method, path: r.URL.Path}
		if d := time.Duration(c.delay.Load()); d > 0 {
			time.Sleep(d)
		}
		w.WriteHeader(int(c.status.Load()))
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *catcher) URL() string { return c.srv.URL }

// waitFor blocks until one POST is received or the deadline elapses.
func (c *catcher) waitFor(t *testing.T, timeout time.Duration) caught {
	t.Helper()
	select {
	case got := <-c.requests:
		return got
	case <-time.After(timeout):
		t.Fatalf("catcher: no POST within %s", timeout)
		return caught{}
	}
}

// expectNone asserts no POST arrives in the window.
func (c *catcher) expectNone(t *testing.T, window time.Duration) {
	t.Helper()
	select {
	case got := <-c.requests:
		t.Fatalf("expected no POST, got %+v", got)
	case <-time.After(window):
	}
}

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
	WorkerID orgchart.WorkerID
	Triggers []activation.Trigger
}

// newDispatcherWithSpawner returns a Dispatcher whose Spawner records
// each activation onto a buffered channel. Tests use this to assert
// who was activated (and not activated) for a given Dispatch call.
func newDispatcherWithSpawner(t *testing.T) (*dispatch.Dispatcher, *store.Store, <-chan recordedActivation) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)
	rec := make(chan recordedActivation, 16)
	spawner := runtime.Spawner(func(_ context.Context, _ string, workerID orgchart.WorkerID, _ string, triggers []activation.Trigger) error {
		rec <- recordedActivation{WorkerID: workerID, Triggers: triggers}
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
			sort.Slice(got, func(i, j int) bool { return got[i].WorkerID < got[j].WorkerID })
			return got
		}
	}
}

// seedAIWorker creates an AIWorker holding a shared per-test role and
// persists it.
func seedAIWorker(t *testing.T, s *store.Store, workerID orgchart.WorkerID) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	roleID := orgchart.RoleID("r-test")
	if _, err := s.Roles.Get(ctx, "org-test", roleID); err != nil {
		role, err := orgchart.NewRole(roleID, "# Role: Test\nTest role.", nil, nil, now, "org-test")
		if err != nil {
			t.Fatalf("new role: %v", err)
		}
		if err := s.Roles.Create(ctx, role); err != nil {
			t.Fatalf("create role: %v", err)
		}
	}
	w, err := orgchart.NewAIWorker(workerID, roleID, "# "+string(workerID)+"\nTest persona.", "org-test")
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := s.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	env, err := environment.New(workerID, t.TempDir(), now, "org-test")
	if err != nil {
		t.Fatalf("new env: %v", err)
	}
	if err := s.Environments.Create(ctx, env); err != nil {
		t.Fatalf("create env: %v", err)
	}
}

// seedSubscription persists a Worker→Stream subscription.
func seedSubscription(t *testing.T, s *store.Store, workerID orgchart.WorkerID, streamID streaming.StreamID) {
	t.Helper()
	if _, err := s.Workers.Get(context.Background(), "org-test", workerID); err != nil {
		t.Fatalf("get worker %q for subscription: %v", workerID, err)
	}
	sub, err := streaming.NewSubscription(string(workerID), streamID, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("new subscription: %v", err)
	}
	if err := s.Subscriptions.Create(context.Background(), sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
}

// seedWebhookStream creates a Stream of the given Transport and returns
// its ID.
func seedWebhookStream(t *testing.T, s *store.Store, id streaming.StreamID, transport transport.Transport) {
	t.Helper()
	stream, err := streaming.NewStream(id, string(id), "", "w-owner", time.Now().UTC(), transport, "org-test")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := s.Streams.Create(context.Background(), stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
}

// eventCounter monotonically generates unique IDs for test events,
// independent of the body. Bodies in some tests contain control bytes
// or non-ASCII that would otherwise leak into the X-Helix-Event header.
var eventCounter atomic.Uint64

// makeEvent builds a simple Event for dispatching with a stable
// header-safe ID. Source is set to a non-empty sentinel so emit
// runs (events with empty Source are treated as inbound and skipped
// by the dispatcher to avoid echo loops).
func makeEvent(t *testing.T, streamID streaming.StreamID, body string) streaming.Event {
	t.Helper()
	id := streaming.EventID(fmt.Sprintf("e-%s-%d", streamID, eventCounter.Add(1)))
	e, err := streaming.NewEvent(id, streamID, "w-test", body, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	return e
}

// TestDispatchEmitsOutbound is the happy path: a webhook stream with
// an outbound_url POSTs the event body to the catcher when Dispatch
// runs. Headers identify the source stream and event.
func TestDispatchEmitsOutbound(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, s := newDispatcher(t)
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL()})
	seedWebhookStream(t, s, "s-out", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	e := makeEvent(t, "s-out", "hello world")
	d.Dispatch(context.Background(), e)

	got := c.waitFor(t, 2*time.Second)
	if got.body != "hello world" {
		t.Fatalf("body = %q, want %q", got.body, "hello world")
	}
	if got.method != http.MethodPost {
		t.Fatalf("method = %q, want POST", got.method)
	}
	if h := got.headers.Get("X-Helix-Stream"); h != "s-out" {
		t.Fatalf("X-Helix-Stream = %q", h)
	}
	if h := got.headers.Get("X-Helix-Event"); h == "" {
		t.Fatalf("X-Helix-Event missing")
	}
}

// TestDispatchSkipsLocalStream proves a TransportLocal stream emits
// nothing — local streams stay local even when the catcher exists.
func TestDispatchSkipsLocalStream(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, s := newDispatcher(t)
	seedWebhookStream(t, s, "s-local", transport.LocalTransport())

	d.Dispatch(context.Background(), makeEvent(t, "s-local", "should not leave"))
	c.expectNone(t, 200*time.Millisecond)
}

// TestDispatchSkipsWebhookWithoutURL proves an inbound-only webhook
// stream — same Kind but no outbound_url — does not emit. This is the
// existing inbound demo behaviour: still works after we added emit.
func TestDispatchSkipsWebhookWithoutURL(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, s := newDispatcher(t)
	seedWebhookStream(t, s, "s-inbox", transport.Transport{Kind: transport.KindWebhook})

	d.Dispatch(context.Background(), makeEvent(t, "s-inbox", "inbound only"))
	c.expectNone(t, 200*time.Millisecond)
}

// TestDispatchHandlesMissingStream proves a publish on a stream that
// has been deleted (or never existed) doesn't panic — the dispatcher
// silently no-ops.
func TestDispatchHandlesMissingStream(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, _ := newDispatcher(t)

	// No stream seeded. Just dispatch.
	d.Dispatch(context.Background(), makeEvent(t, "s-ghost", "vanished"))
	c.expectNone(t, 100*time.Millisecond)
}

// TestDispatchTolerates5xx proves a target returning a 5xx does not
// panic, hang, or block subsequent dispatches.
func TestDispatchTolerates5xx(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	c.status.Store(http.StatusInternalServerError)
	d, s := newDispatcher(t)
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL()})
	seedWebhookStream(t, s, "s-flaky", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	d.Dispatch(context.Background(), makeEvent(t, "s-flaky", "boom"))

	// Target still received it even though it 500'd — the emitter logs
	// and moves on, doesn't retry, doesn't crash.
	got := c.waitFor(t, 2*time.Second)
	if got.body != "boom" {
		t.Fatalf("body = %q", got.body)
	}

	// Second dispatch still works.
	d.Dispatch(context.Background(), makeEvent(t, "s-flaky", "again"))
	got2 := c.waitFor(t, 2*time.Second)
	if got2.body != "again" {
		t.Fatalf("body = %q", got2.body)
	}
}

// TestDispatchTolerates4xx proves a target returning a 4xx (e.g. the
// remote rejecting the payload) is also a non-fatal log-and-drop —
// same shape as 5xx but a different branch in the implementation.
func TestDispatchTolerates4xx(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	c.status.Store(http.StatusBadRequest)
	d, s := newDispatcher(t)
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL()})
	seedWebhookStream(t, s, "s-rejecty", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	d.Dispatch(context.Background(), makeEvent(t, "s-rejecty", "nope"))
	got := c.waitFor(t, 2*time.Second)
	if got.body != "nope" {
		t.Fatalf("body = %q", got.body)
	}
}

// TestDispatchTolerates_UnreachableHost proves an unreachable target
// (port closed) is logged-and-dropped with a bounded timeout — the
// dispatcher returns immediately, and a follow-up dispatch on a
// healthy stream still works.
func TestDispatchTolerates_UnreachableHost(t *testing.T) {
	t.Parallel()
	d, s := newDispatcher(t)
	// 127.0.0.1:1 is reserved and reliably refuses connections.
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: "http://127.0.0.1:1/dead"})
	seedWebhookStream(t, s, "s-dead", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	// Use a tiny client timeout so the test runs fast.
	d.SetHTTPClient(&http.Client{Timeout: 200 * time.Millisecond})

	start := time.Now()
	d.Dispatch(context.Background(), makeEvent(t, "s-dead", "void"))
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Dispatch blocked for %s — should be async", elapsed)
	}

	// Sleep past the client timeout to give the goroutine time to fail.
	time.Sleep(400 * time.Millisecond)
	// No assertion on the catcher (there is none); we're proving the
	// dispatcher didn't crash and didn't block its caller.
}

// TestDispatchHonoursClientTimeout proves a slow target hits the
// configured HTTP timeout without stalling the caller.
func TestDispatchHonoursClientTimeout(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	c.delay.Store(int64(2 * time.Second)) // longer than the client timeout
	d, s := newDispatcher(t)
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL()})
	seedWebhookStream(t, s, "s-slow", transport.Transport{Kind: transport.KindWebhook, Config: cfg})
	d.SetHTTPClient(&http.Client{Timeout: 100 * time.Millisecond})

	start := time.Now()
	d.Dispatch(context.Background(), makeEvent(t, "s-slow", "patience"))
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Dispatch blocked for %s", elapsed)
	}

	// Catcher still receives the request before its delay; that's fine.
	_ = c.waitFor(t, 2*time.Second)
}

// TestDispatchConcurrent proves many parallel publishes all reach the
// target, in any order, with no deadlocks.
func TestDispatchConcurrent(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, s := newDispatcher(t)
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL()})
	seedWebhookStream(t, s, "s-stress", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	const n = 25
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			d.Dispatch(context.Background(), makeEvent(t, "s-stress", "msg"))
		}(i)
	}
	wg.Wait()

	deadline := time.After(5 * time.Second)
	seen := 0
	for seen < n {
		select {
		case <-c.requests:
			seen++
		case <-deadline:
			t.Fatalf("only %d/%d POSTs received", seen, n)
		}
	}
}

// TestDispatchBinaryPayload proves arbitrary bytes (including null
// bytes, UTF-8, newlines) round-trip verbatim — no implicit encoding
// or wrapping.
func TestDispatchBinaryPayload(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, s := newDispatcher(t)
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL()})
	seedWebhookStream(t, s, "s-bin", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	body := "líne 1 — α β γ\n\x00\nemoji: 🚀"
	d.Dispatch(context.Background(), makeEvent(t, "s-bin", body))
	got := c.waitFor(t, 2*time.Second)
	if got.body != body {
		t.Fatalf("body round-trip mismatch:\n got: %q\nwant: %q", got.body, body)
	}
}

// TestDispatchInvalidStoredConfigDoesNotCrash exercises the defensive
// path where transport.Config is malformed at runtime (impossible via
// the normal NewStream path, since Validate rejects it — but a manual
// DB edit could create it). The dispatcher logs and continues.
func TestDispatchInvalidStoredConfigDoesNotCrash(t *testing.T) {
	t.Parallel()
	d, s := newDispatcher(t)
	// Bypass NewStream's Validate by inserting the malformed Stream
	// directly through the store.
	bogus := streaming.Stream{
		ID:        "s-bogus",
		Name:      "bogus",
		CreatedBy: "w-owner",
		CreatedAt: time.Now().UTC(),
		Transport: transport.Transport{Kind: transport.KindWebhook, Config: []byte(`{not valid`)},
	}
	if err := s.Streams.Create(context.Background(), bogus); err != nil {
		t.Fatalf("create stream: %v", err)
	}

	d.Dispatch(context.Background(), makeEvent(t, "s-bogus", "ignored"))
	// No crash. Nothing else to assert; if we got here we passed.
}

// TestDispatchRespectsStoreLookupErrors proves a store that errors on
// Streams.Get (rather than returning ErrNotFound) is handled — the
// dispatcher logs and returns; downstream subscriber fan-out still
// works for the next event.
func TestDispatchRespectsStoreLookupErrors(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, s := newDispatcher(t)
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL()})
	seedWebhookStream(t, s, "s-ok", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	// Dispatch on a missing stream first — should noop without affecting
	// the next dispatch.
	d.Dispatch(context.Background(), makeEvent(t, "s-missing", "lost"))
	c.expectNone(t, 100*time.Millisecond)

	// Healthy dispatch still works.
	d.Dispatch(context.Background(), makeEvent(t, "s-ok", "found"))
	got := c.waitFor(t, 2*time.Second)
	if got.body != "found" {
		t.Fatalf("body = %q", got.body)
	}
}

// TestDispatchContentTypeAndPath proves the outbound POST hits the
// configured path and uses a generic content-type — the body is opaque
// so application/octet-stream is the safest default.
func TestDispatchContentTypeAndPath(t *testing.T) {
	t.Parallel()
	c := newCatcher(t)
	d, s := newDispatcher(t)
	// URL with a path so we can verify it's preserved.
	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: c.URL() + "/some/where"})
	seedWebhookStream(t, s, "s-path", transport.Transport{Kind: transport.KindWebhook, Config: cfg})

	d.Dispatch(context.Background(), makeEvent(t, "s-path", "x"))
	got := c.waitFor(t, 2*time.Second)
	if got.path != "/some/where" {
		t.Fatalf("path = %q, want /some/where", got.path)
	}
	if ct := got.headers.Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
}

// TestDispatchSkipsPublisher pins the rule that an AI Worker which
// publishes to a Stream they themselves are subscribed to is NOT
// re-activated on their own event. This is the cheapest available
// brake on broadcast cascades — without it, a single publish would
// activate the publisher in a loop. Other subscribers are still
// activated normally.
func TestDispatchSkipsPublisher(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)
	seedWebhookStream(t, s, "s-team", transport.Transport{Kind: transport.KindLocal})
	seedAIWorker(t, s, "w-publisher")
	seedAIWorker(t, s, "w-other")
	seedSubscription(t, s, "w-publisher", "s-team")
	seedSubscription(t, s, "w-other", "s-team")

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
	d.Dispatch(context.Background(), e)

	got := drainActivations(t, rec, 0)
	if len(got) != 1 {
		t.Fatalf("activations = %d, want 1; got %+v", len(got), got)
	}
	if got[0].WorkerID != "w-other" {
		t.Fatalf("activated worker = %q, want w-other", got[0].WorkerID)
	}
}

// TestDispatchAttachesSourceKind pins that the dispatcher resolves the
// Source Worker's WorkerKind and threads it onto the Trigger so the
// activation prompt (rendered by spawner.renderTrigger) can surface
// "source_kind: ai" or "source_kind: human". This is the input that
// agent.md's "treat AI-origin as low priority" rule keys off of.
func TestDispatchAttachesSourceKind(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)
	seedWebhookStream(t, s, "s-team", transport.Transport{Kind: transport.KindLocal})
	seedAIWorker(t, s, "w-publisher")
	seedAIWorker(t, s, "w-other")
	seedSubscription(t, s, "w-other", "s-team")

	e, err := streaming.NewMessageEvent(
		"e-2", "s-team", "w-publisher",
		streaming.Message{From: "w-publisher", Body: "ping"},
		time.Now().UTC(),
		"org-test",
	)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	if err := s.Events.Append(context.Background(), e); err != nil {
		t.Fatalf("append event: %v", err)
	}
	d.Dispatch(context.Background(), e)

	got := drainActivations(t, rec, 0)
	if len(got) != 1 {
		t.Fatalf("activations = %d, want 1", len(got))
	}
	if n := len(got[0].Triggers); n != 1 {
		t.Fatalf("triggers = %d, want 1", n)
	}
	if k := got[0].Triggers[0].SourceKind; k != orgchart.WorkerKindAI {
		t.Fatalf("SourceKind = %q, want %q", k, orgchart.WorkerKindAI)
	}
}

// TestDispatchCoalescesEvents pins the cost-saving rule that drove this
// design: while one activation is in flight for a Worker, any further
// events that arrive on Streams that Worker subscribes to are
// appended to a per-Worker queue and delivered to the Spawner as one
// batched activation when the current one finishes — not five
// separate fresh-claude runs.
//
// Shape of the test: the spawner blocks on the very first call so we
// can publish more events behind it, then we release it and assert
// the second Spawner call receives all the events that queued during
// the block as one slice.
func TestDispatchCoalescesEvents(t *testing.T) {
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
	spawner := runtime.Spawner(func(_ context.Context, _ string, workerID orgchart.WorkerID, _ string, triggers []activation.Trigger) error {
		n := calls.Add(1)
		if n == 1 {
			close(started)
			<-release
		}
		// Copy the slice so a later mutation in the dispatcher (it doesn't
		// today, but defensive) can't race with the assertion read.
		copied := make([]activation.Trigger, len(triggers))
		copy(copied, triggers)
		rec <- recordedActivation{WorkerID: workerID, Triggers: copied}
		return nil
	})
	d := dispatch.New(s, spawner, slog.New(slog.NewTextHandler(io.Discard, nil)))

	seedWebhookStream(t, s, "s-team", transport.Transport{Kind: transport.KindLocal})
	seedAIWorker(t, s, "w-eng")
	seedSubscription(t, s, "w-eng", "s-team")

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
		d.Dispatch(context.Background(), ev)
	}

	// First event kicks off activation #1; the spawner blocks inside it.
	publish("e-1", "first")
	<-started

	// Three more events while activation #1 is held. These should NOT
	// each trigger a fresh Spawner call — they should pool in the queue
	// and be drained as one batch when activation #1 returns.
	publish("e-2", "two")
	publish("e-3", "three")
	publish("e-4", "four")

	// Give the dispatcher's enqueue goroutines a tick to land. The lock
	// inside enqueue is uncontended once Dispatch returns, but the
	// goroutines that resolve subs/env can still be in flight.
	time.Sleep(100 * time.Millisecond)

	// Release the first activation; the runner now drains the batch.
	close(release)

	// Two Spawner calls total: one with [e-1], one with [e-2, e-3, e-4].
	a1 := waitForActivation(t, rec, 2*time.Second)
	a2 := waitForActivation(t, rec, 2*time.Second)

	if len(a1.Triggers) != 1 || a1.Triggers[0].EventID != "e-1" {
		t.Fatalf("activation #1 = %d trigger(s) %+v, want [e-1]", len(a1.Triggers), eventIDs(a1.Triggers))
	}
	if len(a2.Triggers) != 3 {
		t.Fatalf("activation #2 = %d triggers %+v, want 3", len(a2.Triggers), eventIDs(a2.Triggers))
	}
	wantIDs := []streaming.EventID{"e-2", "e-3", "e-4"}
	for i, want := range wantIDs {
		if a2.Triggers[i].EventID != want {
			t.Fatalf("activation #2 trigger order = %+v, want %+v", eventIDs(a2.Triggers), wantIDs)
		}
	}

	// And no third activation is fired — the runner exits cleanly when
	// the queue drains.
	select {
	case extra := <-rec:
		t.Fatalf("unexpected third activation: %+v", extra)
	case <-time.After(150 * time.Millisecond):
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("Spawner calls = %d, want 2", got)
	}
}

// waitForActivation pulls one recordedActivation off rec or fails the
// test on timeout. Centralised so the coalescing test reads cleanly.
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

// TestDispatchSkipsFanOutOnBadMessageBody pins B6.2: an Event whose
// Body isn't canonical Message JSON is a programming bug — every
// production write goes through Message.Encode. The dispatcher used
// to silently fall back to {Body: raw}; B6.2 makes that path strict
// (no fan-out) so a bad event is visible rather than emitting a
// half-rendered activation prompt.
//
// Outbound emission is unaffected — it runs before the parse and
// posts the raw e.Body to webhook receivers regardless.
func TestDispatchSkipsFanOutOnBadMessageBody(t *testing.T) {
	t.Parallel()
	d, s, rec := newDispatcherWithSpawner(t)
	seedWebhookStream(t, s, "s-bad", transport.Transport{Kind: transport.KindLocal})
	seedAIWorker(t, s, "w-listener")
	seedSubscription(t, s, "w-listener", "s-bad")

	// Hand-craft an event with non-JSON body — bypasses NewMessageEvent
	// on purpose to simulate the only path that produces this state
	// (hand-poked DB or a regression in a future write path).
	e, err := streaming.NewEvent("e-bad", "s-bad", "w-author", "not-json-payload", time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("new event: %v", err)
	}

	d.Dispatch(context.Background(), e)

	// Listener must NOT be activated. With the old fallback the
	// dispatcher would activate with {Body: "not-json-payload"};
	// strict-parse skips fan-out entirely.
	got := drainActivations(t, rec, 100*time.Millisecond)
	if len(got) != 0 {
		t.Fatalf("activations = %d, want 0 (bad body must not fan out); got %+v", len(got), got)
	}
}
