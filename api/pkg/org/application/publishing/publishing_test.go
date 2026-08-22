package publishing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
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

type recRouter struct {
	log  *[]string
	seen []eventsource.Event
	err  error
}

func (d *recRouter) Route(_ context.Context, e eventsource.Event) error {
	*d.log = append(*d.log, "route")
	d.seen = append(d.seen, e)
	return d.err
}

func seedTrigger(t *testing.T, st *store.Store, orgID, id string, kind transport.Kind, config []byte) {
	t.Helper()
	row, err := trigger.New(id, orgID, id, "", kind, config, "w-owner", fixedClock())
	if err != nil {
		t.Fatalf("new trigger: %v", err)
	}
	if err := st.Triggers.Create(context.Background(), row); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
}

// TestPublish_AppendNotifyRouteOrder pins the trio that must stay atomic
// and ordered: the event is appended FIRST, then long-poll observers are
// notified, then attached AI Workers are routed to.
func TestPublish_AppendNotifyRouteOrder(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedTrigger(t, st, "org-test", "s-1", transport.KindLocal, nil)

	var log []string
	router := &recRouter{log: &log}
	svc := New(Deps{
		Triggers: st.Triggers,
		Events:   &recEvents{Events: st.Events, log: &log},
		Hub:      &recNotifier{log: &log},
		Router:   router,
		Now:      fixedClock,
		NewID:    func() string { return "fixed" },
	})

	ev, err := svc.PublishToTrigger(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "hello"})
	if err != nil {
		t.Fatalf("PublishToTrigger: %v", err)
	}
	if ev.ID != "e-fixed" {
		t.Fatalf("event id = %q", ev.ID)
	}
	want := []string{"append", "notify", "route"}
	if len(log) != 3 || log[0] != want[0] || log[1] != want[1] || log[2] != want[2] {
		t.Fatalf("call order = %v, want %v", log, want)
	}
	if len(router.seen) != 1 || router.seen[0].Source != eventsource.Trigger("s-1") {
		t.Fatalf("routed source = %+v", router.seen)
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

// TestSendToChannel_InboundTriggerRejected: a Worker cannot send back out
// through an inbound transport. The rejection happens before any write.
func TestSendToChannel_InboundTriggerRejected(t *testing.T) {
	t.Parallel()
	for _, kind := range []transport.Kind{transport.KindGitHub, transport.KindGitLab, transport.KindWebhook, transport.KindCron} {
		t.Run(string(kind), func(t *testing.T) {
			st := memory.New()
			seedTrigger(t, st, "org-test", "s-1", kind, configFor(kind))

			var log []string
			idCalls := 0
			svc := New(Deps{
				Triggers: st.Triggers,
				Events:   &recEvents{Events: st.Events, log: &log},
				Hub:      &recNotifier{log: &log},
				Router:   &recRouter{log: &log},
				Now:      fixedClock,
				NewID: func() string {
					idCalls++
					return "fixed"
				},
			})
			_, err := svc.SendToChannel(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "x"})
			if !errors.Is(err, ErrNotAnInternalChannel) {
				t.Fatalf("err = %v, want ErrNotAnInternalChannel", err)
			}
			if idCalls != 0 || len(log) != 0 {
				t.Fatalf("nothing should fire on rejection: id calls=%d log=%v", idCalls, log)
			}
			events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-1", 10)
			if len(events) != 0 {
				t.Fatalf("events = %d, want 0", len(events))
			}
		})
	}
}

// TestSendToChannel_LocalAllowed: an internal channel is exactly what a
// Worker may send into.
func TestSendToChannel_LocalAllowed(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedTrigger(t, st, "org-test", "s-dm-a-b", transport.KindLocal, nil)
	svc := New(Deps{Triggers: st.Triggers, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})
	if _, err := svc.SendToChannel(context.Background(), "org-test", "s-dm-a-b", "w-a", streaming.Message{Body: "hi"}); err != nil {
		t.Fatalf("SendToChannel: %v", err)
	}
}

func TestPublish_TriggerNotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := New(Deps{Triggers: st.Triggers, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})
	_, err := svc.PublishToTrigger(context.Background(), "org-test", "s-missing", "w-owner", streaming.Message{Body: "x"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestPublish_NoHubNoRouter: the trio degrades gracefully when Hub and
// Router are unwired (tests / runtimes without them).
func TestPublish_NoHubNoRouter(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedTrigger(t, st, "org-test", "s-1", transport.KindLocal, nil)
	svc := New(Deps{Triggers: st.Triggers, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})
	if _, err := svc.PublishToTrigger(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "hi"}); err != nil {
		t.Fatalf("Publish without hub/router: %v", err)
	}
}

// TestPublishDelivery_DuplicateRejected: a provider redelivery under the
// same derived event id collides on the events key instead of being
// appended twice, which is what makes ingress idempotent.
func TestPublishDelivery_DuplicateRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedTrigger(t, st, "org-test", "s-gh", transport.KindGitHub, []byte(`{"repo":"helixml/helix","events":["issues"]}`))
	svc := New(Deps{Triggers: st.Triggers, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})

	if _, err := svc.PublishDelivery(context.Background(), "org-test", "s-gh", "e-delivery-1", streaming.Message{Body: "first"}); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	_, err := svc.PublishDelivery(context.Background(), "org-test", "s-gh", "e-delivery-1", streaming.Message{Body: "retry"})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-gh", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

// TestPublish_ProcessorBranchUsesBranchStream: a branch event lands on
// the stream recorded on the branch, and routes from the branch's source
// — the pair that preserves a converted branch's history while routing
// on its durable id.
func TestPublish_ProcessorBranchUsesBranchStream(t *testing.T) {
	t.Parallel()
	st := memory.New()
	var log []string
	router := &recRouter{log: &log}
	svc := New(Deps{Triggers: st.Triggers, Events: st.Events, Router: router, Now: fixedClock, NewID: func() string { return "x" }})

	src := eventsource.ProcessorOutput("p-1", "po-vip")
	if _, err := svc.Publish(context.Background(), "org-test", src, "s-legacy-out", "", streaming.Message{Body: "routed"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-legacy-out", 10)
	if len(events) != 1 {
		t.Fatalf("events on branch stream = %d, want 1", len(events))
	}
	if len(router.seen) != 1 || router.seen[0].Source != src {
		t.Fatalf("routed source = %+v, want %s", router.seen, src.Key())
	}
}

// TestPublish_RouteFailureSurfacesAfterAppend: the event is durable even
// when routing fails, and the caller is told rather than silently losing
// the fan-out.
func TestPublish_RouteFailureSurfacesAfterAppend(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedTrigger(t, st, "org-test", "s-1", transport.KindLocal, nil)
	var log []string
	svc := New(Deps{
		Triggers: st.Triggers, Events: st.Events,
		Router: &recRouter{log: &log, err: errors.New("queue down")},
		Now:    fixedClock, NewID: func() string { return "x" },
	})
	ev, err := svc.PublishToTrigger(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "hi"})
	if err == nil {
		t.Fatal("want a routing error")
	}
	if ev.ID == "" {
		t.Fatal("append should still have happened")
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-1", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func configFor(kind transport.Kind) []byte {
	switch kind {
	case transport.KindGitHub:
		return []byte(`{"repo":"helixml/helix","events":["issues"]}`)
	case transport.KindGitLab:
		return []byte(`{"repository_id":"1","repo":"helixml/helix","events":["Push Hook"]}`)
	case transport.KindCron:
		return []byte(`{"schedule":"0 9 * * *"}`)
	default:
		return nil
	}
}
