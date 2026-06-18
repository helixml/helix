package publishing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func fixedClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

// recEvents wraps the real Events repo and records "append" in the
// shared call-order log so the test can assert append-before-notify.
type recEvents struct {
	store.Events
	log *[]string
}

func (r *recEvents) Append(ctx context.Context, e streaming.Event) error {
	*r.log = append(*r.log, "append")
	return r.Events.Append(ctx, e)
}

type recNotifier struct{ log *[]string }

func (n *recNotifier) Notify(_ string, _ streaming.StreamID) { *n.log = append(*n.log, "notify") }

type recDispatcher struct{ log *[]string }

func (d *recDispatcher) Dispatch(_ context.Context, _ streaming.Event) {
	*d.log = append(*d.log, "dispatch")
}

func seedStream(t *testing.T, st *store.Store, orgID string, tr transport.Transport) {
	t.Helper()
	s, err := streaming.NewStream("s-1", "s-1", "", "w-owner", fixedClock(), tr, orgID)
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(context.Background(), s); err != nil {
		t.Fatalf("create stream: %v", err)
	}
}

// TestPublish_AppendNotifyDispatchOrder pins the trio that must stay
// atomic and ordered: the event is appended FIRST, then long-poll
// observers are notified, then subscribed AI workers are dispatched.
func TestPublish_AppendNotifyDispatchOrder(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedStream(t, st, "org-test", transport.LocalTransport())

	var log []string
	svc := New(Deps{
		Streams:    st.Streams,
		Events:     &recEvents{Events: st.Events, log: &log},
		Hub:        &recNotifier{log: &log},
		Dispatcher: &recDispatcher{log: &log},
		Now:        fixedClock,
		NewID:      func() string { return "fixed" },
	})

	ev, err := svc.Publish(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "hello"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if ev.ID != "e-fixed" {
		t.Fatalf("event id = %q", ev.ID)
	}
	want := []string{"append", "notify", "dispatch"}
	if len(log) != 3 || log[0] != want[0] || log[1] != want[1] || log[2] != want[2] {
		t.Fatalf("call order = %v, want %v", log, want)
	}

	// Event persisted with the caller as source + From.
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-1", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	if msg.From != "w-owner" || msg.Body != "hello" {
		t.Fatalf("message = %+v", msg)
	}
}

// TestPublish_GitHubRejected: github transport streams are inbound-only;
// publish returns ErrPublishToGitHub and appends nothing.
func TestPublish_GitHubRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ghCfg := []byte(`{"repo":"helixml/helix","events":["issues"]}`)
	seedStream(t, st, "org-test", transport.Transport{Kind: transport.KindGitHub, Config: ghCfg})

	var log []string
	svc := New(Deps{
		Streams:    st.Streams,
		Events:     &recEvents{Events: st.Events, log: &log},
		Hub:        &recNotifier{log: &log},
		Dispatcher: &recDispatcher{log: &log},
		Now:        fixedClock,
		NewID:      func() string { return "fixed" },
	})
	_, err := svc.Publish(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "x"})
	if !errors.Is(err, ErrPublishToGitHub) {
		t.Fatalf("err = %v, want ErrPublishToGitHub", err)
	}
	if len(log) != 0 {
		t.Fatalf("nothing should fire on rejection, got %v", log)
	}
}

func TestPublish_StreamNotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := New(Deps{Streams: st.Streams, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})
	_, err := svc.Publish(context.Background(), "org-test", "s-missing", "w-owner", streaming.Message{Body: "x"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestPublish_NoHubNoDispatcher: the trio degrades gracefully when Hub
// and Dispatcher are unwired (tests / runtimes without them).
func TestPublish_NoHubNoDispatcher(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedStream(t, st, "org-test", transport.LocalTransport())
	svc := New(Deps{Streams: st.Streams, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})
	if _, err := svc.Publish(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "hi"}); err != nil {
		t.Fatalf("Publish without hub/dispatcher: %v", err)
	}
}
