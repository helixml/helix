package publishing

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func (n *recNotifier) Notify(_ string, _ streaming.TopicID) { *n.log = append(*n.log, "notify") }

type recDispatcher struct{ log *[]string }

func (d *recDispatcher) Dispatch(_ context.Context, _ streaming.Event) {
	*d.log = append(*d.log, "dispatch")
}

type recDeliverer struct {
	calls int
	err   error
}

func (d *recDeliverer) Deliver(_ context.Context, _ streaming.Topic, _ streaming.Message) (DeliveryReceipt, error) {
	d.calls++
	return DeliveryReceipt{Status: "delivered", Provider: "slack", Destination: "C1", MessageID: "1.2"}, d.err
}

func seedTopic(t *testing.T, st *store.Store, orgID string, tr transport.Transport) {
	seedTopicID(t, st, orgID, "s-1", tr)
}

func seedTopicID(t *testing.T, st *store.Store, orgID string, id streaming.TopicID, tr transport.Transport) {
	t.Helper()
	s, err := streaming.NewTopic(id, string(id), "", "w-owner", fixedClock(), tr, orgID)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(context.Background(), s); err != nil {
		t.Fatalf("create topic: %v", err)
	}
}

type nestedPublishDispatcher struct {
	svc  *Publishing
	next map[streaming.TopicID]streaming.TopicID
	errs []error
}

func (d *nestedPublishDispatcher) Dispatch(ctx context.Context, event streaming.Event) {
	next := d.next[event.TopicID]
	if next == "" {
		return
	}
	if _, err := d.svc.Publish(ctx, event.OrganizationID, next, "", streaming.Message{Body: "nested"}); err != nil {
		d.errs = append(d.errs, err)
	}
}

// TestPublish_AppendNotifyDispatchOrder pins the trio that must stay
// atomic and ordered: the event is appended FIRST, then long-poll
// observers are notified, then subscribed AI workers are dispatched.
func TestPublish_AppendNotifyDispatchOrder(t *testing.T) {
	t.Parallel()
	st := memory.New()
	seedTopic(t, st, "org-test", transport.LocalTransport())

	var log []string
	svc := New(Deps{
		Topics:     st.Topics,
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
	events, _ := st.Events.ListForTopic(context.Background(), "org-test", "s-1", 10)
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

// TestPublish_GitHubRejected: github transport topics are inbound-only;
// publish returns ErrPublishToGitHub and appends nothing.
func TestPublish_GitHubRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ghCfg := []byte(`{"repo":"helixml/helix","events":["issues"]}`)
	seedTopic(t, st, "org-test", transport.Transport{Kind: transport.KindGitHub, Config: ghCfg})

	var log []string
	svc := New(Deps{
		Topics:     st.Topics,
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

func TestPublish_TopicNotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := New(Deps{Topics: st.Topics, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})
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
	seedTopic(t, st, "org-test", transport.LocalTransport())
	svc := New(Deps{Topics: st.Topics, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" }})
	if _, err := svc.Publish(context.Background(), "org-test", "s-1", "w-owner", streaming.Message{Body: "hi"}); err != nil {
		t.Fatalf("Publish without hub/dispatcher: %v", err)
	}
}

func TestPublish_SlackDeliversButInboundDoesNotEcho(t *testing.T) {
	st := memory.New()
	seedTopic(t, st, "org-test", transport.Transport{Kind: transport.KindSlack, Config: []byte(`{"service_connection_id":"sc-1","channel_id":"C1"}`)})
	deliverer := &recDeliverer{}
	id := 0
	svc := New(Deps{
		Topics: st.Topics, Events: st.Events, Now: fixedClock, NewID: func() string {
			id++
			return fmt.Sprint(id)
		},
		Deliverers: map[transport.Kind]Deliverer{transport.KindSlack: deliverer},
	})

	result, err := svc.PublishWithReceipt(context.Background(), "org-test", "s-1", "b-worker", streaming.Message{Body: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if deliverer.calls != 1 || result.Delivery == nil || result.Delivery.Status != "delivered" || result.Delivery.MessageID != "1.2" {
		t.Fatalf("delivery = %#v, calls = %d", result.Delivery, deliverer.calls)
	}
	if _, err := svc.Publish(context.Background(), "org-test", "s-1", "", streaming.Message{Body: "automated"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PublishInbound(context.Background(), "org-test", "s-1", "", streaming.Message{Body: "from Slack"}); err != nil {
		t.Fatal(err)
	}
	if deliverer.calls != 2 {
		t.Fatalf("inbound Slack event echoed outbound; calls = %d", deliverer.calls)
	}
}

func TestPublish_SlackWithoutChannelRejectedBeforeSideEffects(t *testing.T) {
	for _, useReceipt := range []bool{false, true} {
		name := "Publish"
		if useReceipt {
			name = "PublishWithReceipt"
		}
		t.Run(name, func(t *testing.T) {
			st := memory.New()
			seedTopic(t, st, "org-test", transport.Transport{Kind: transport.KindSlack, Config: []byte(`{"service_connection_id":"sc-1"}`)})
			var log []string
			idCalls := 0
			deliverer := &recDeliverer{}
			svc := New(Deps{
				Topics:     st.Topics,
				Events:     &recEvents{Events: st.Events, log: &log},
				Hub:        &recNotifier{log: &log},
				Dispatcher: &recDispatcher{log: &log},
				Now:        fixedClock,
				NewID: func() string {
					idCalls++
					return "fixed"
				},
				Deliverers: map[transport.Kind]Deliverer{transport.KindSlack: deliverer},
			})

			var result Result
			var err error
			if useReceipt {
				result, err = svc.PublishWithReceipt(context.Background(), "org-test", "s-1", "b-worker", streaming.Message{Body: "hello"})
			} else {
				result.Event, err = svc.Publish(context.Background(), "org-test", "s-1", "b-worker", streaming.Message{Body: "hello"})
			}
			if !errors.Is(err, ErrSlackChannelNotConfigured) {
				t.Fatalf("err = %v, want ErrSlackChannelNotConfigured", err)
			}
			if result.Event.ID != "" || result.Delivery != nil {
				t.Fatalf("result = %#v, want empty", result)
			}
			if idCalls != 0 || len(log) != 0 || deliverer.calls != 0 {
				t.Fatalf("side effects: id calls=%d log=%v delivery calls=%d", idCalls, log, deliverer.calls)
			}
			events, _ := st.Events.ListForTopic(context.Background(), "org-test", "s-1", 10)
			if len(events) != 0 {
				t.Fatalf("events = %d, want 0", len(events))
			}
		})
	}
}

func TestPublishInbound_SlackWithoutChannelAllowed(t *testing.T) {
	st := memory.New()
	seedTopic(t, st, "org-test", transport.Transport{Kind: transport.KindSlack, Config: []byte(`{"service_connection_id":"sc-1"}`)})
	var log []string
	deliverer := &recDeliverer{}
	svc := New(Deps{
		Topics:     st.Topics,
		Events:     &recEvents{Events: st.Events, log: &log},
		Dispatcher: &recDispatcher{log: &log},
		Now:        fixedClock,
		NewID:      func() string { return "fixed" },
		Deliverers: map[transport.Kind]Deliverer{transport.KindSlack: deliverer},
	})

	event, err := svc.PublishInbound(context.Background(), "org-test", "s-1", "", streaming.Message{Body: "from Slack"})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == "" || len(log) != 2 || log[0] != "append" || log[1] != "dispatch" || deliverer.calls != 0 {
		t.Fatalf("event=%#v log=%v delivery calls=%d", event, log, deliverer.calls)
	}
}

func TestPublish_InboundProvenanceSuppressesNestedDelivery(t *testing.T) {
	st := memory.New()
	tr := transport.Transport{Kind: transport.KindSlack, Config: []byte(`{"service_connection_id":"sc-1","channel_id":"C1"}`)}
	for _, id := range []streaming.TopicID{"s-1", "s-2", "s-3"} {
		seedTopicID(t, st, "org-test", id, tr)
	}
	deliverer := &recDeliverer{}
	dispatcher := &nestedPublishDispatcher{next: map[streaming.TopicID]streaming.TopicID{"s-1": "s-2", "s-2": "s-3"}}
	id := 0
	svc := New(Deps{
		Topics: st.Topics, Events: st.Events, Dispatcher: dispatcher, Now: fixedClock,
		NewID: func() string {
			id++
			return fmt.Sprint(id)
		},
		Deliverers: map[transport.Kind]Deliverer{transport.KindSlack: deliverer},
	})
	dispatcher.svc = svc

	if _, err := svc.PublishInbound(context.Background(), "org-test", "s-1", "", streaming.Message{Body: "inbound"}); err != nil {
		t.Fatal(err)
	}
	if len(dispatcher.errs) != 0 || deliverer.calls != 0 {
		t.Fatalf("inbound nested errors = %v, delivery calls = %d", dispatcher.errs, deliverer.calls)
	}
	for _, topicID := range []streaming.TopicID{"s-1", "s-2", "s-3"} {
		events, _ := st.Events.ListForTopic(context.Background(), "org-test", topicID, 10)
		if len(events) != 1 {
			t.Fatalf("%s inbound events = %d, want 1", topicID, len(events))
		}
	}

	if _, err := svc.Publish(context.Background(), "org-test", "s-1", "", streaming.Message{Body: "automated"}); err != nil {
		t.Fatal(err)
	}
	if deliverer.calls != 3 {
		t.Fatalf("ordinary empty-source publish delivery calls = %d, want 3", deliverer.calls)
	}
}

func TestPublish_SlackFailureIsExplicitAfterAuditAppend(t *testing.T) {
	st := memory.New()
	seedTopic(t, st, "org-test", transport.Transport{Kind: transport.KindSlack, Config: []byte(`{"service_connection_id":"sc-1","channel_id":"C1"}`)})
	deliverer := &recDeliverer{err: errors.New("not_in_channel")}
	svc := New(Deps{
		Topics: st.Topics, Events: st.Events, Now: fixedClock, NewID: func() string { return "x" },
		Deliverers: map[transport.Kind]Deliverer{transport.KindSlack: deliverer},
	})

	result, err := svc.PublishWithReceipt(context.Background(), "org-test", "s-1", "b-worker", streaming.Message{Body: "hello"})
	if err == nil || !strings.Contains(err.Error(), "not_in_channel") {
		t.Fatalf("err = %v", err)
	}
	if result.Event.ID == "" || result.Delivery == nil || result.Delivery.Status != "failed" || result.Delivery.Provider != "slack" || !strings.Contains(result.Delivery.Error, "do not retry publish: not_in_channel") {
		t.Fatalf("partial result = %#v", result)
	}
	events, _ := st.Events.ListForTopic(context.Background(), "org-test", "s-1", 10)
	if len(events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(events))
	}
}
