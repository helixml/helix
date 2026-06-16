// These tests pin the slack ingest — the one shared inbound path
// (design/2026-06-16-helix-org-slack-stream.md §9.1). They drive
// Ingest.Receive with synthetic events against a seeded GORM test DB:
//
//   - team id → correct org; unknown team id → dropped, no dispatch (FR-17)
//   - message in a bound channel → one Event appended + dispatched (FR-18)
//   - two orgs, same channel name → strict org isolation (FR-5/FR-17)
//   - event with a bot_id → dropped (self-echo guard, FR-20)
//   - envelope mapping: From=user, Body=text, ThreadID=thread_ts, MessageID=ts
package slack_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// recordingDispatcher captures Dispatch / DispatchTo calls so tests can
// assert the dispatcher was woken and with which targets.
type recordingDispatcher struct {
	mu        sync.Mutex
	events    []streaming.Event
	targets   [][]orgchart.WorkerID
	dispatchN int
}

func (d *recordingDispatcher) Dispatch(_ context.Context, e streaming.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
	d.targets = append(d.targets, nil)
	d.dispatchN++
}

func (d *recordingDispatcher) DispatchTo(_ context.Context, e streaming.Event, targets []orgchart.WorkerID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
	d.targets = append(d.targets, targets)
	d.dispatchN++
}

func (d *recordingDispatcher) snapshot() ([]streaming.Event, [][]orgchart.WorkerID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ev := make([]streaming.Event, len(d.events))
	copy(ev, d.events)
	tg := make([][]orgchart.WorkerID, len(d.targets))
	copy(tg, d.targets)
	return ev, tg
}

func newTestIngest(t *testing.T) (*slacktransport.Ingest, *store.Store, *recordingDispatcher, *configregistry.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("NewInMemoryNats: %v", err)
	}
	bc := wakebus.New(ps)
	rd := &recordingDispatcher{}
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{
		Key:     "transport.slack",
		Type:    configregistry.TypeObject,
		Secrets: []string{"bot_token"},
	})
	reg.Register(configregistry.Spec{
		Key:     "slack.router",
		Type:    configregistry.TypeString,
		Default: `"broadcast"`,
	})
	in := slacktransport.NewIngest(reg, st, bc, rd, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return in, st, rd, reg
}

func setSlackInstall(t *testing.T, reg *configregistry.Registry, orgID, botToken, teamID string) {
	t.Helper()
	val, _ := json.Marshal(map[string]string{"bot_token": botToken, "team_id": teamID})
	if err := reg.Set(context.Background(), orgID, "transport.slack", string(val)); err != nil {
		t.Fatalf("set slack config for %s: %v", orgID, err)
	}
}

func seedSlackStream(t *testing.T, st *store.Store, orgID string, id streaming.StreamID, channel string) streaming.Stream {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{"channel": channel})
	stream, err := streaming.NewStream(id, string(id), "", "w-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindSlack, Config: cfg}, orgID)
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(context.Background(), stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	return stream
}

func TestIngest_RoutesToOrgByTeamID(t *testing.T) {
	in, st, rd, reg := newTestIngest(t)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	ev := slacktransport.Event{Channel: "C123", User: "U999", Text: "hello", TS: "1700000000.000100"}
	if err := in.Receive(context.Background(), "TAAA", ev); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	events, _ := rd.snapshot()
	if len(events) != 1 {
		t.Fatalf("dispatch count = %d, want 1", len(events))
	}
	if events[0].StreamID != stream.ID {
		t.Fatalf("dispatched to stream %q, want %q", events[0].StreamID, stream.ID)
	}
	if events[0].OrganizationID != "org-a" {
		t.Fatalf("event org = %q, want org-a", events[0].OrganizationID)
	}
	// Event was appended.
	stored, err := st.Events.ListForStream(context.Background(), "org-a", stream.ID, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored events = %d, want 1", len(stored))
	}
}

func TestIngest_UnknownTeamDropped(t *testing.T) {
	in, st, rd, reg := newTestIngest(t)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	seedSlackStream(t, st, "org-a", "str-a", "C123")

	ev := slacktransport.Event{Channel: "C123", User: "U999", Text: "hello", TS: "1.1"}
	if err := in.Receive(context.Background(), "TZZZ", ev); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	events, _ := rd.snapshot()
	if len(events) != 0 {
		t.Fatalf("dispatch count = %d, want 0 for unknown team", len(events))
	}
}

func TestIngest_OrgIsolationSameChannelName(t *testing.T) {
	in, st, rd, reg := newTestIngest(t)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	setSlackInstall(t, reg, "org-b", "xoxb-b", "TBBB")
	streamA := seedSlackStream(t, st, "org-a", "str-a", "Cshared")
	seedSlackStream(t, st, "org-b", "str-b", "Cshared")

	ev := slacktransport.Event{Channel: "Cshared", User: "U1", Text: "hi", TS: "1.2"}
	if err := in.Receive(context.Background(), "TAAA", ev); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	events, _ := rd.snapshot()
	if len(events) != 1 {
		t.Fatalf("dispatch count = %d, want 1 (org-a only)", len(events))
	}
	if events[0].StreamID != streamA.ID || events[0].OrganizationID != "org-a" {
		t.Fatalf("leaked to wrong stream/org: %+v", events[0])
	}
	// org-b stream must have no events.
	bEvents, _ := st.Events.ListForStream(context.Background(), "org-b", "str-b", 10)
	if len(bEvents) != 0 {
		t.Fatalf("org-b stream got %d events, want 0 (isolation breach)", len(bEvents))
	}
}

func TestIngest_BotEventDropped(t *testing.T) {
	in, st, rd, reg := newTestIngest(t)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	seedSlackStream(t, st, "org-a", "str-a", "C123")

	ev := slacktransport.Event{Channel: "C123", User: "U999", Text: "echo", TS: "1.3", BotID: "B0001"}
	if err := in.Receive(context.Background(), "TAAA", ev); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	events, _ := rd.snapshot()
	if len(events) != 0 {
		t.Fatalf("dispatch count = %d, want 0 for bot event (self-echo)", len(events))
	}
}

// seedAISubscriber creates an AI worker (with a role carrying matchable
// text) subscribed to the stream, so routing has candidates to choose
// among.
func seedAISubscriber(t *testing.T, st *store.Store, orgID string, workerID orgchart.WorkerID, roleID orgchart.RoleID, roleText string, streamID streaming.StreamID) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.Roles.Get(ctx, orgID, roleID); err != nil {
		role, err := orgchart.NewRole(roleID, roleText, nil, nil, time.Now().UTC(), orgID)
		if err != nil {
			t.Fatalf("new role: %v", err)
		}
		if err := st.Roles.Create(ctx, role); err != nil {
			t.Fatalf("create role: %v", err)
		}
	}
	w, err := orgchart.NewAIWorker(workerID, roleID, "# "+string(workerID), orgID)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := st.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	sub, err := streaming.NewSubscription(string(workerID), streamID, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new subscription: %v", err)
	}
	if err := st.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
}

func TestIngest_DefaultBroadcastsToAllSubscribers(t *testing.T) {
	in, st, rd, reg := newTestIngest(t)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")
	seedAISubscriber(t, st, "org-a", "w-billing", "r-billing", "# Billing\nHandles invoices and refunds.", stream.ID)
	seedAISubscriber(t, st, "org-a", "w-eng", "r-eng", "# Engineering\nFixes bugs in code.", stream.ID)

	ev := slacktransport.Event{Channel: "C123", User: "U1", Text: "I need a refund on my invoice", TS: "1.9"}
	if err := in.Receive(context.Background(), "TAAA", ev); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	_, targets := rd.snapshot()
	if len(targets) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(targets))
	}
	if len(targets[0]) != 2 {
		t.Fatalf("default routed to %v, want both subscribers (broadcast)", targets[0])
	}
}

func TestIngest_FuzzyRouterActivatesChosenWorker(t *testing.T) {
	in, st, rd, reg := newTestIngest(t)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	if err := reg.Set(context.Background(), "org-a", "slack.router", `"fuzzy"`); err != nil {
		t.Fatalf("set slack.router: %v", err)
	}
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")
	seedAISubscriber(t, st, "org-a", "w-billing", "r-billing", "# Billing\nHandles invoices refunds payments.", stream.ID)
	seedAISubscriber(t, st, "org-a", "w-eng", "r-eng", "# Engineering\nFixes bugs writes code reviews.", stream.ID)

	ev := slacktransport.Event{Channel: "C123", User: "U1", Text: "I need a refund on my invoice, the payment failed", TS: "2.0"}
	if err := in.Receive(context.Background(), "TAAA", ev); err != nil {
		t.Fatalf("Receive: %v", err)
	}
	_, targets := rd.snapshot()
	if len(targets) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(targets))
	}
	if len(targets[0]) != 1 || targets[0][0] != "w-billing" {
		t.Fatalf("fuzzy routed to %v, want [w-billing]", targets[0])
	}
}

func TestIngest_EnvelopeMapping(t *testing.T) {
	in, st, _, reg := newTestIngest(t)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	ev := slacktransport.Event{
		Channel:  "C123",
		User:     "U999",
		Text:     "the body",
		TS:       "1700000000.000100",
		ThreadTS: "1699999999.000001",
	}
	if err := in.Receive(context.Background(), "TAAA", ev); err != nil {
		t.Fatalf("Receive: %v", err)
	}

	stored, _ := st.Events.ListForStream(context.Background(), "org-a", stream.ID, 10)
	if len(stored) != 1 {
		t.Fatalf("stored events = %d, want 1", len(stored))
	}
	msg, err := stored[0].Message()
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if msg.From != "U999" {
		t.Errorf("From = %q, want U999", msg.From)
	}
	if msg.Body != "the body" {
		t.Errorf("Body = %q, want 'the body'", msg.Body)
	}
	if msg.ThreadID != "1699999999.000001" {
		t.Errorf("ThreadID = %q, want thread_ts", msg.ThreadID)
	}
	if msg.MessageID != "1700000000.000100" {
		t.Errorf("MessageID = %q, want ts", msg.MessageID)
	}
	// Inbound events are system-sourced (external sender, no helix Worker).
	if stored[0].Source != "" {
		t.Errorf("Source = %q, want empty", stored[0].Source)
	}
}
