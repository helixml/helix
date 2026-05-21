package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/helix-org/broadcast"
	"github.com/helixml/helix/helix-org/dispatch"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/server"
	"github.com/helixml/helix/helix-org/store"
	"github.com/helixml/helix/helix-org/store/sqlite"
	"github.com/helixml/helix/helix-org/tools"
)

// recordingDispatcher captures every Dispatch call so tests can assert
// the webhook handler fans events out to subscribed Workers. Safe for
// concurrent calls — httptest.Server runs each request on its own
// goroutine.
type recordingDispatcher struct {
	mu     sync.Mutex
	events []domain.Event
}

func (d *recordingDispatcher) Dispatch(_ context.Context, e domain.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
}

func (d *recordingDispatcher) snapshot() []domain.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]domain.Event, len(d.events))
	copy(out, d.events)
	return out
}

// newWebhookServer wires an in-memory store, a real broadcaster, and
// the supplied dispatcher (may be nil) into a Server. Returns the
// running httptest.Server plus the store + broadcaster so tests can
// seed streams and observe wakeups.
func newWebhookServer(t *testing.T, dispatcher server.Dispatcher) (*httptest.Server, *store.Store, *broadcast.Broadcaster) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	bc := broadcast.New()
	srv := httptest.NewServer(server.New(s, tools.NewRegistry(), bc, dispatcher, nil).Handler())
	t.Cleanup(srv.Close)
	return srv, s, bc
}

// seedStream creates a Stream with the given transport kind. The
// caller's createdBy is a fixed test sentinel; we don't seed a
// matching Worker because the webhook path doesn't read it.
func seedStream(t *testing.T, s *store.Store, id stream.ID, kind transport.Kind) {
	t.Helper()
	stream, err := domain.NewStream(id, string(id), "", "w-owner", time.Now().UTC(),
		transport.Transport{Kind: kind})
	if err != nil {
		t.Fatalf("new stream %q: %v", id, err)
	}
	if err := s.Streams.Create(context.Background(), stream); err != nil {
		t.Fatalf("seed stream %q: %v", id, err)
	}
}

// TestWebhookPostAppendsEvent walks the happy path: POSTing a body to
// /webhooks/<streamID> appends an event with empty source (system-
// emitted) and the raw body. The dispatcher receives the event, and a
// long-poll observer of that stream wakes.
func TestWebhookPostAppendsEvent(t *testing.T) {
	t.Parallel()
	rd := &recordingDispatcher{}
	srv, s, bc := newWebhookServer(t, rd)
	seedStream(t, s, "s-inbox", transport.KindWebhook)

	wake := bc.Subscribe([]stream.ID{"s-inbox"})
	t.Cleanup(func() { bc.Unsubscribe([]stream.ID{"s-inbox"}, wake) })

	body := "incoming text — anything goes here"
	resp, err := http.Post(srv.URL+"/webhooks/s-inbox", "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %q", resp.StatusCode, string(b))
	}

	events, err := s.Events.ListForStream(context.Background(), "s-inbox", 10)
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
	default:
		t.Fatal("broadcaster did not wake long-poll observer")
	}
}

// TestWebhookPostErrors covers the rejection paths the handler must
// turn into HTTP errors: unknown streams, wrong-transport streams,
// empty bodies, and wrong HTTP methods.
func TestWebhookPostErrors(t *testing.T) {
	t.Parallel()
	srv, s, _ := newWebhookServer(t, nil)
	seedStream(t, s, "s-inbox", transport.KindWebhook)
	seedStream(t, s, "s-local", transport.KindLocal)

	cases := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
	}{
		{"unknown stream", "POST", "/webhooks/s-ghost", "x", http.StatusNotFound},
		{"wrong-transport stream (local) is not a webhook", "POST", "/webhooks/s-local", "x", http.StatusNotFound},
		{"empty body", "POST", "/webhooks/s-inbox", "", http.StatusBadRequest},
		{"GET not allowed", "GET", "/webhooks/s-inbox", "", http.StatusMethodNotAllowed},
		{"PUT not allowed", "PUT", "/webhooks/s-inbox", "x", http.StatusMethodNotAllowed},
		{"DELETE not allowed", "DELETE", "/webhooks/s-inbox", "", http.StatusMethodNotAllowed},
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
	seedStream(t, s, "s-inbox", transport.KindWebhook)

	// Empty body → 400. No event should land.
	resp, err := http.Post(srv.URL+"/webhooks/s-inbox", "text/plain", strings.NewReader(""))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	events, err := s.Events.ListForStream(context.Background(), "s-inbox", 10)
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
	seedStream(t, s, "s-inbox", transport.KindWebhook)

	atLimit := bytes.Repeat([]byte("a"), 1<<20)
	resp, err := http.Post(srv.URL+"/webhooks/s-inbox", "text/plain", bytes.NewReader(atLimit))
	if err != nil {
		t.Fatalf("POST at limit: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("at-limit status = %d, want 200", resp.StatusCode)
	}

	overLimit := bytes.Repeat([]byte("a"), (1<<20)+1)
	resp, err = http.Post(srv.URL+"/webhooks/s-inbox", "text/plain", bytes.NewReader(overLimit))
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
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	seedStream(t, s, "s-inbox", transport.KindWebhook)
	srv := httptest.NewServer(server.New(s, tools.NewRegistry(), nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/webhooks/s-inbox", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	events, _ := s.Events.ListForStream(context.Background(), "s-inbox", 10)
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
	seedStream(t, s, "s-inbox", transport.KindWebhook)

	body := "line one\nline two\n\ttabbed → emoji 🚀 — UTF-8 preserved"
	resp, err := http.Post(srv.URL+"/webhooks/s-inbox", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	events, _ := s.Events.ListForStream(context.Background(), "s-inbox", 10)
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
// Stream and asserts every one lands as a distinct event with a
// matching dispatch.
func TestWebhookConcurrentPosts(t *testing.T) {
	t.Parallel()
	rd := &recordingDispatcher{}
	srv, s, _ := newWebhookServer(t, rd)
	seedStream(t, s, "s-inbox", transport.KindWebhook)

	const N = 25
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf("payload %02d", i)
			resp, err := http.Post(srv.URL+"/webhooks/s-inbox", "text/plain", strings.NewReader(body))
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

	events, err := s.Events.ListForStream(context.Background(), "s-inbox", 100)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != N {
		t.Fatalf("events = %d, want %d", len(events), N)
	}
	if got := len(rd.snapshot()); got != N {
		t.Fatalf("dispatched = %d, want %d", got, N)
	}

	seen := make(map[event.ID]bool, N)
	for _, e := range events {
		if seen[e.ID] {
			t.Fatalf("duplicate event ID %q", e.ID)
		}
		seen[e.ID] = true
	}
}

// TestWebhookDoesNotLeakAcrossStreams verifies that a POST to one
// webhook stream lands only on that stream — not on a sibling
// webhook stream that happens to exist.
func TestWebhookDoesNotLeakAcrossStreams(t *testing.T) {
	t.Parallel()
	srv, s, _ := newWebhookServer(t, nil)
	seedStream(t, s, "s-inbox", transport.KindWebhook)
	seedStream(t, s, "s-other", transport.KindWebhook)

	resp, err := http.Post(srv.URL+"/webhooks/s-inbox", "text/plain", strings.NewReader("for inbox"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	inboxEvents, _ := s.Events.ListForStream(context.Background(), "s-inbox", 10)
	otherEvents, _ := s.Events.ListForStream(context.Background(), "s-other", 10)
	if len(inboxEvents) != 1 {
		t.Fatalf("inbox events = %d, want 1", len(inboxEvents))
	}
	if len(otherEvents) != 0 {
		t.Fatalf("other events = %d, want 0 (no leakage)", len(otherEvents))
	}
}

// TestWebhookInboundDoesNotEcho proves that a bidirectional webhook
// Stream (one with both inbound and outbound configured) does *not*
// echo inbound POSTs back out to its own outbound URL. The dispatcher
// skips emit for events with empty Source — i.e. events that came
// from this transport's own inbound — so a stream that's
// bidirectional doesn't loop. Only Worker-published events
// (Source != "") emit outbound.
func TestWebhookInboundDoesNotEcho(t *testing.T) {
	t.Parallel()
	caught := make(chan string, 1)
	catcher := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		caught <- string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(catcher.Close)

	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	d := dispatch.New(st, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(server.New(st, tools.NewRegistry(), broadcast.New(), d, nil).Handler())
	t.Cleanup(srv.Close)

	cfg, _ := json.Marshal(transport.WebhookConfig{OutboundURL: catcher.URL})
	stream, err := domain.NewStream("s-bridge", "bridge", "", "w-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindWebhook, Config: cfg})
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(context.Background(), stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}

	body := "round-trip text"
	resp, err := http.Post(srv.URL+"/webhooks/s-bridge", "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inbound status = %d", resp.StatusCode)
	}

	select {
	case got := <-caught:
		t.Fatalf("inbound event echoed to outbound: %q", got)
	case <-time.After(500 * time.Millisecond):
		// Expected: nothing arrives at the catcher because inbound
		// events have empty Source and the dispatcher skips emit.
	}
}
