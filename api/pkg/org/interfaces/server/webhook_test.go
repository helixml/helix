package server_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	"github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// recordingDispatcher captures every Dispatch call so tests can assert
// the webhook handler fans events out to subscribed Workers. Safe for
// concurrent calls — httptest.Server runs each request on its own
// goroutine.
// recordingRouter captures the events publishing routed, so tests can
// assert the webhook reached the routing layer at all.
type recordingRouter struct {
	mu     sync.Mutex
	events []eventsource.Event
}

func (d *recordingRouter) Route(_ context.Context, e eventsource.Event) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
	return nil
}

func (d *recordingRouter) snapshot() []eventsource.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]eventsource.Event, len(d.events))
	copy(out, d.events)
	return out
}

// newWebhookServer wires an in-memory store, a real broadcaster, and
// the supplied router (may be nil) into a Server. Returns the running
// httptest.Server plus the store + broadcaster so tests can seed
// Triggers and observe wakeups.
func newWebhookServer(t *testing.T, router publishing.Router) (*httptest.Server, *store.Store, *wakebus.Bus) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)
	bc := newTopichub(t)
	srv := httptest.NewServer(server.NewFromStore(s, mcptools.NewRegistry(), bc, router, nil).Handler())
	t.Cleanup(srv.Close)
	return srv, s, bc
}

// newTopichub spins up an in-memory NATS-backed wakebus. The
// embedded NATS server is cleaned up at test exit via the test's
// natural goroutine teardown — the in-memory provider doesn't expose
// an explicit Close hook.
func newTopichub(t *testing.T) *wakebus.Bus {
	t.Helper()
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("NewInMemoryNats: %v", err)
	}
	return wakebus.New(ps)
}

// seedTrigger creates a Trigger with the given transport kind. The
// caller's createdBy is a fixed test sentinel; we don't seed a
// matching Worker because the webhook path doesn't read it.
func seedTrigger(t *testing.T, s *store.Store, id string, kind transport.Kind) {
	t.Helper()
	row, err := trigger.New(id, "org-test", id, "", kind, nil, "w-owner", time.Now().UTC())
	if err != nil {
		t.Fatalf("new trigger %q: %v", id, err)
	}
	if err := s.Triggers.Create(context.Background(), row); err != nil {
		t.Fatalf("seed trigger %q: %v", id, err)
	}
}

// TestWebhookPostAppendsEvent walks the happy path: POSTing a body to
// /webhooks/<triggerID> appends an event with empty source (system-
// emitted) and the raw body. The dispatcher receives the event, and a
// long-poll observer of that stream wakes.
func TestWebhookPostAppendsEvent(t *testing.T) {
	t.Parallel()
	rd := &recordingRouter{}
	srv, s, bc := newWebhookServer(t, rd)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)

	wake := bc.Subscribe("org-test", []streaming.StreamID{"s-inbox"})
	t.Cleanup(func() { bc.Unsubscribe([]streaming.StreamID{"s-inbox"}, wake) })
	// wakebus is pubsub-backed (NATS); the SUB has to round-trip to
	// the embedded server before Publish can route to us. Give it a
	// short window — the wake check at the bottom of the test then
	// waits up to a second for the asynchronous delivery to land.
	time.Sleep(100 * time.Millisecond)

	body := "incoming text — anything goes here"
	resp, err := http.Post(srv.URL+"/webhooks/org-test/s-inbox", "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %q", resp.StatusCode, string(b))
	}

	events, err := s.Events.ListForStream(context.Background(), "org-test", "s-inbox", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("parse message body: %v", err)
	}
	if msg.Body != body {
		t.Fatalf("message body = %q, want %q", msg.Body, body)
	}
	if msg.From != "" {
		t.Fatalf("message from = %q, want empty (no helix originator)", msg.From)
	}
	if events[0].Source != "" {
		t.Fatalf("source = %q, want empty (system-emitted)", events[0].Source)
	}
	if events[0].StreamID != "s-inbox" {
		t.Fatalf("streamID = %q, want s-inbox", events[0].StreamID)
	}

	dispatched := rd.snapshot()
	if len(dispatched) != 1 || dispatched[0].ID != events[0].ID {
		t.Fatalf("dispatched = %+v, want one event matching the appended one", dispatched)
	}

	select {
	case <-wake:
	case <-time.After(time.Second):
		t.Fatal("broadcaster did not wake long-poll observer")
	}
}

// TestWebhookPostErrors covers the rejection paths the handler must
// turn into HTTP errors: unknown Triggers, wrong-transport Triggers,
// empty bodies, and wrong HTTP methods.
func TestWebhookPostErrors(t *testing.T) {
	t.Parallel()
	srv, s, _ := newWebhookServer(t, nil)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)
	seedTrigger(t, s, "s-local", transport.KindLocal)

	cases := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{"unknown trigger", "POST", "/webhooks/org-test/s-ghost", "x", http.StatusNotFound},
		{"wrong-transport trigger (local) is not a webhook", "POST", "/webhooks/org-test/s-local", "x", http.StatusNotFound},
		{"empty body", "POST", "/webhooks/org-test/s-inbox", "", http.StatusBadRequest},
		{"GET not allowed", "GET", "/webhooks/org-test/s-inbox", "", http.StatusMethodNotAllowed},
		{"PUT not allowed", "PUT", "/webhooks/org-test/s-inbox", "x", http.StatusMethodNotAllowed},
		{"DELETE not allowed", "DELETE", "/webhooks/org-test/s-inbox", "", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do request: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.wantCode {
				b, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d, want %d (body = %q)", resp.StatusCode, tc.wantCode, string(b))
			}
		})
	}
}

// TestWebhookErrorsLeaveStoreClean asserts that error paths don't
// half-create state. After a failed POST, the stream's event list
// must be empty.
func TestWebhookErrorsLeaveStoreClean(t *testing.T) {
	t.Parallel()
	srv, s, _ := newWebhookServer(t, nil)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)

	// Empty body → 400. No event should land.
	resp, err := http.Post(srv.URL+"/webhooks/org-test/s-inbox", "text/plain", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	events, err := s.Events.ListForStream(context.Background(), "org-test", "s-inbox", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 — failed POST should not append", len(events))
	}
}

// TestWebhookBodySizeBoundary verifies the 1 MiB cap is enforced
// exactly: a body at the limit is accepted, a body one byte over is
// rejected.
func TestWebhookBodySizeBoundary(t *testing.T) {
	t.Parallel()
	srv, s, _ := newWebhookServer(t, nil)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)

	atLimit := bytes.Repeat([]byte("a"), 1<<20)
	resp, err := http.Post(srv.URL+"/webhooks/org-test/s-inbox", "text/plain", bytes.NewReader(atLimit))
	if err != nil {
		t.Fatalf("POST at limit: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("at-limit status = %d, want 200", resp.StatusCode)
	}

	overLimit := bytes.Repeat([]byte("a"), (1<<20)+1)
	resp, err = http.Post(srv.URL+"/webhooks/org-test/s-inbox", "text/plain", bytes.NewReader(overLimit))
	if err != nil {
		t.Fatalf("POST over limit: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("over-limit status = %d, want 400", resp.StatusCode)
	}
}

// TestWebhookWithNilCollaborators verifies the handler tolerates a
// Server constructed without a broadcaster or dispatcher — common in
// tests and in degraded modes where one or both are deliberately
// unwired. The event still lands; nothing panics.
func TestWebhookWithNilCollaborators(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)
	srv := httptest.NewServer(server.NewFromStore(s, mcptools.NewRegistry(), nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/webhooks/org-test/s-inbox", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	events, _ := s.Events.ListForStream(context.Background(), "org-test", "s-inbox", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

// TestWebhookPreservesBodyExactly verifies the handler stores bodies
// verbatim — newlines, multibyte UTF-8, special characters all round-
// trip without normalisation.
func TestWebhookPreservesBodyExactly(t *testing.T) {
	t.Parallel()
	srv, s, _ := newWebhookServer(t, nil)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)

	body := "line one\nline two\n\ttabbed → emoji 🚀 — UTF-8 preserved"
	resp, err := http.Post(srv.URL+"/webhooks/org-test/s-inbox", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	events, _ := s.Events.ListForStream(context.Background(), "org-test", "s-inbox", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	if msg.Body != body {
		t.Fatalf("message body mismatch:\n got: %q\nwant: %q", msg.Body, body)
	}
}

// TestWebhookConcurrentPosts fires many parallel POSTs to the same
// Topic and asserts every one lands as a distinct event with a
// matching dispatch.
func TestWebhookConcurrentPosts(t *testing.T) {
	t.Parallel()
	rd := &recordingRouter{}
	srv, s, _ := newWebhookServer(t, rd)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)

	const N = 25
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf("payload %02d", i)
			resp, err := http.Post(srv.URL+"/webhooks/org-test/s-inbox", "text/plain", strings.NewReader(body))
			if err != nil {
				t.Errorf("POST %d: %v", i, err)
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("POST %d: status = %d", i, resp.StatusCode)
			}
		}(i)
	}
	wg.Wait()

	events, err := s.Events.ListForStream(context.Background(), "org-test", "s-inbox", 100)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != N {
		t.Fatalf("events = %d, want %d", len(events), N)
	}
	if got := len(rd.snapshot()); got != N {
		t.Fatalf("dispatched = %d, want %d", got, N)
	}

	seen := make(map[streaming.EventID]bool, N)
	for _, e := range events {
		if seen[e.ID] {
			t.Fatalf("duplicate event ID %q", e.ID)
		}
		seen[e.ID] = true
	}
}

// TestWebhookDoesNotLeakAcrossTriggers verifies that a POST to one
// webhook Trigger lands only on that Trigger — not on a sibling
// webhook Trigger that happens to exist.
func TestWebhookDoesNotLeakAcrossTriggers(t *testing.T) {
	t.Parallel()
	srv, s, _ := newWebhookServer(t, nil)
	seedTrigger(t, s, "s-inbox", transport.KindWebhook)
	seedTrigger(t, s, "s-other", transport.KindWebhook)

	resp, err := http.Post(srv.URL+"/webhooks/org-test/s-inbox", "text/plain", strings.NewReader("for inbox"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	inboxEvents, _ := s.Events.ListForStream(context.Background(), "org-test", "s-inbox", 10)
	otherEvents, _ := s.Events.ListForStream(context.Background(), "org-test", "s-other", 10)
	if len(inboxEvents) != 1 {
		t.Fatalf("inbox events = %d, want 1", len(inboxEvents))
	}
	if len(otherEvents) != 0 {
		t.Fatalf("other events = %d, want 0 (no leakage)", len(otherEvents))
	}
}

// TestWebhookTriggerIsInboundOnly pins what replaced the old
// "inbound must not echo outbound" guard: after the cutover a Trigger
// has no outbound side at all, so a webhook Trigger's config carries no
// destination and nothing can loop. The event lands, routes, and stops.
func TestWebhookTriggerIsInboundOnly(t *testing.T) {
	t.Parallel()
	router := &recordingRouter{}
	srv, s, _ := newWebhookServer(t, router)
	seedTrigger(t, s, "s-bridge", transport.KindWebhook)

	resp, err := http.Post(srv.URL+"/webhooks/org-test/s-bridge", "text/plain", strings.NewReader("round-trip text"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inbound status = %d", resp.StatusCode)
	}

	routed := router.snapshot()
	if len(routed) != 1 {
		t.Fatalf("routed events = %d, want 1", len(routed))
	}
	if routed[0].Source != eventsource.Trigger("s-bridge") {
		t.Fatalf("routed source = %+v, want trigger:s-bridge", routed[0].Source)
	}
	// Nobody authored it: an inbound delivery has no originating Worker.
	if routed[0].OriginatingWorkerID != "" {
		t.Fatalf("originating worker = %q, want empty", routed[0].OriginatingWorkerID)
	}
}

// Embedded in helix the org is resolved by middleware into the request
// context and the mount prefix supplies the {org} segment, so the public
// URL carries only the Trigger id. Standalone, the org is a path
// segment. Both must reach the same Trigger.
func TestWebhookPostResolvesOrgFromContextWhenPathHasNoOrgSegment(t *testing.T) {
	t.Parallel()
	rd := &recordingRouter{}
	st := orggorm.GetOrgTestDB(t)
	bc := newTopichub(t)
	h := server.NewFromStore(st, mcptools.NewRegistry(), bc, rd, nil).Handler()
	seedTrigger(t, st, "s-inbox", transport.KindWebhook)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/s-inbox", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "text/plain")
	req = req.WithContext(server.WithOrgID(req.Context(), "org-test"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := len(rd.snapshot()); got != 1 {
		t.Fatalf("routed %d events, want 1", got)
	}
}
