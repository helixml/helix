package processing_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/application/processing"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/topics"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

const org = "org-1"

// rig wires the full publish→dispatch→process→publish backbone against
// the in-memory store, with a spawner that records every activation.
type rig struct {
	store   *store.Store
	pub     *publishing.Publishing
	procSvc *processors.Processors
	topics  *topics.Topics

	mu          sync.Mutex
	activations []activation.Trigger
	gotAct      chan struct{}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{store: memory.New(), gotAct: make(chan struct{}, 64)}

	spawner := func(_ context.Context, _ string, _ orgchart.BotID, triggers []activation.Trigger) error {
		r.mu.Lock()
		r.activations = append(r.activations, triggers...)
		r.mu.Unlock()
		for range triggers {
			r.gotAct <- struct{}{}
		}
		return nil
	}

	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	disp := dispatch.New(r.store, spawner, logger)
	r.pub = publishing.New(publishing.Deps{
		Topics: r.store.Topics, Events: r.store.Events, Dispatcher: disp,
		Now: time.Now, NewID: incID(),
	})
	runner := processing.New(r.store.Processors, r.pub, logger)
	disp.RegisterProcessorRunner(runner)

	r.topics = topics.New(topics.Deps{Topics: r.store.Topics, Now: time.Now, NewID: incID()})
	r.procSvc = processors.New(processors.Deps{
		Processors: r.store.Processors, Topics: r.topics, Now: time.Now, NewID: incID(),
	})
	return r
}

func (r *rig) waitForActivation(t *testing.T) {
	t.Helper()
	select {
	case <-r.gotAct:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for worker activation")
	}
}

func (r *rig) lastActivation(t *testing.T) activation.Trigger {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.activations) == 0 {
		t.Fatal("no activations recorded")
	}
	return r.activations[len(r.activations)-1]
}

func (r *rig) mkTopic(t *testing.T, id, name string) streaming.TopicID {
	t.Helper()
	top, err := r.topics.Create(context.Background(), org, topics.CreateParams{ID: id, Name: name})
	if err != nil {
		t.Fatalf("create topic %s: %v", id, err)
	}
	return top.ID
}

func (r *rig) mkAIWorker(t *testing.T, id, subTopic streaming.TopicID) {
	t.Helper()
	w, err := orgchart.NewAIWorker(orgchart.BotID(id), "r-x", "# "+string(id), org)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := r.store.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	sub, err := streaming.NewSubscription(string(id), subTopic, time.Now().UTC(), org)
	if err != nil {
		t.Fatalf("new sub: %v", err)
	}
	if err := r.store.Subscriptions.Create(context.Background(), sub); err != nil {
		t.Fatalf("create sub: %v", err)
	}
}

func templateCfg(t *testing.T, tmpl string) json.RawMessage {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"template": tmpl})
	return b
}

// TestTemplateProcessorEndToEnd is the Phase-2 red test: a template
// processor between an input topic and a worker subscribed to the
// output topic. Publishing to the input must (a) place a transformed
// event on the output topic and (b) activate the worker with the
// rendered body.
func TestTemplateProcessorEndToEnd(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	in := r.mkTopic(t, "s-in", "Inbox")

	proc, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p-fmt", Name: "Formatter", InputTopicID: in, Kind: processor.KindTemplate,
		Config: templateCfg(t, "From {{ .Message.from }}: {{ .Message.body }}"),
	})
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}
	outTopic := proc.Outputs[0].TopicID
	if outTopic == "" {
		t.Fatal("processor output topic was not auto-provisioned")
	}

	// Worker subscribes to the OUTPUT topic.
	r.mkAIWorker(t, "w-triage", outTopic)

	// Publish to the INPUT topic.
	if _, err := r.pub.Publish(ctx, org, in, "alice", streaming.Message{Body: "hello"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	r.waitForActivation(t)

	// (a) transformed event landed on the output topic.
	evs, err := r.store.Events.ListForTopic(ctx, org, outTopic, 10)
	if err != nil {
		t.Fatalf("list output events: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("want 1 event on output topic, got %d", len(evs))
	}
	gotMsg, _ := evs[0].Message()
	if gotMsg.Body != "From alice: hello" {
		t.Errorf("output body = %q, want %q", gotMsg.Body, "From alice: hello")
	}

	// (b) worker activated with the rendered body.
	act := r.lastActivation(t)
	if act.Message.Body != "From alice: hello" {
		t.Errorf("activation body = %q, want %q", act.Message.Body, "From alice: hello")
	}
	if act.TopicID != outTopic {
		t.Errorf("activation topic = %q, want output topic %q", act.TopicID, outTopic)
	}
}

// TestFilterProcessorRouting: a filter with two outputs (a VIP predicate
// and an unconditional default) routes each message to exactly the
// outputs whose predicate matches, and the right workers activate.
func TestFilterProcessorRouting(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	in := r.mkTopic(t, "s-in", "Inbox")

	proc, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p-route", Name: "Router", InputTopicID: in, Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{
			{Label: "vip", Match: `{{ if hasSuffix "@vip.com" .Message.from }}1{{ end }}`},
			{Label: "default", Match: ``},
		},
	})
	if err != nil {
		t.Fatalf("create filter: %v", err)
	}
	vipTopic := proc.Outputs[0].TopicID
	genTopic := proc.Outputs[1].TopicID
	r.mkAIWorker(t, "w-senior", vipTopic)
	r.mkAIWorker(t, "w-triage", genTopic)

	// VIP message → both vip + default.
	if _, err := r.pub.Publish(ctx, org, in, "boss@vip.com", streaming.Message{Body: "urgent"}); err != nil {
		t.Fatalf("publish vip: %v", err)
	}
	r.waitForActivation(t)
	time.Sleep(150 * time.Millisecond)

	vipCount, _ := r.store.Events.CountForTopic(ctx, org, vipTopic)
	genCount, _ := r.store.Events.CountForTopic(ctx, org, genTopic)
	if vipCount != 1 {
		t.Errorf("vip topic events = %d, want 1", vipCount)
	}
	if genCount != 1 {
		t.Errorf("default topic events = %d, want 1 (default catches all)", genCount)
	}

	// Plain message → only the default.
	if _, err := r.pub.Publish(ctx, org, in, "joe@example.com", streaming.Message{Body: "hi"}); err != nil {
		t.Fatalf("publish plain: %v", err)
	}
	r.waitForActivation(t)
	time.Sleep(150 * time.Millisecond)

	vipCount2, _ := r.store.Events.CountForTopic(ctx, org, vipTopic)
	genCount2, _ := r.store.Events.CountForTopic(ctx, org, genTopic)
	if vipCount2 != 1 {
		t.Errorf("after plain publish vip topic = %d, want still 1 (no match)", vipCount2)
	}
	if genCount2 != 2 {
		t.Errorf("after plain publish default topic = %d, want 2", genCount2)
	}
}

// TestCreateRejectsSelfCycle: a processor whose explicit output topic is
// also its input topic closes a one-hop cycle and must be rejected.
func TestCreateRejectsSelfCycle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	a := r.mkTopic(t, "s-a", "A")

	_, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p-self", Name: "Self", InputTopicID: a, Kind: processor.KindTemplate,
		Config:  templateCfg(t, "{{ .Message.body }}"),
		Outputs: []processors.OutputSpec{{TopicID: a}}, // explicit: output == input
	})
	if err == nil {
		t.Fatal("want cycle error for output==input, got nil")
	}
}

// TestCreateRejectsMultiHopCycle: p1 a→b (explicit b), then a processor
// reading b whose explicit output is a closes a two-hop cycle.
func TestCreateRejectsMultiHopCycle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	a := r.mkTopic(t, "s-a", "A")
	b := r.mkTopic(t, "s-b", "B")

	if _, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p1", Name: "P1", InputTopicID: a, Kind: processor.KindTemplate,
		Config: templateCfg(t, "{{ .Message.body }}"), Outputs: []processors.OutputSpec{{TopicID: b}},
	}); err != nil {
		t.Fatalf("create p1 (a->b): %v", err)
	}
	// p2 reads b, outputs a → a→b→a cycle.
	_, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p2", Name: "P2", InputTopicID: b, Kind: processor.KindTemplate,
		Config: templateCfg(t, "{{ .Message.body }}"), Outputs: []processors.OutputSpec{{TopicID: a}},
	})
	if err == nil {
		t.Fatal("want cycle error for a->b->a, got nil")
	}
}

// TestRuntimeHopGuardStopsCycle: if a cycle somehow exists in the store
// (here seeded directly, bypassing checkAcyclic), the runtime hop guard
// must abort the chain instead of looping forever. We assert the chain
// terminates and produced a bounded number of events.
func TestRuntimeHopGuardStopsCycle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	a := r.mkTopic(t, "s-a", "A")

	// Seed a self-looping processor directly into the store (the service
	// would reject this; we bypass it to exercise the runtime guard).
	loop := processor.Processor{
		ID: "p-loop", OrganizationID: org, Name: "Loop", InputTopicID: a,
		Kind: processor.KindTemplate, Config: templateCfg(t, "{{ .Message.body }}"),
		Outputs: []processor.Output{{TopicID: a, Owned: false}}, CreatedAt: time.Now(),
	}
	if err := r.store.Processors.Create(ctx, loop); err != nil {
		t.Fatalf("seed loop: %v", err)
	}

	if _, err := r.pub.Publish(ctx, org, a, "alice", streaming.Message{Body: "x"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// Let the (bounded) recursion settle.
	time.Sleep(500 * time.Millisecond)

	count, err := r.store.Events.CountForTopic(ctx, org, a)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	// 1 original publish + at most maxHops re-publishes before the guard
	// trips. The exact number depends on the guard boundary; the point is
	// it is bounded and small, not unbounded.
	if count < 2 {
		t.Errorf("expected the chain to re-publish at least once, got %d events", count)
	}
	if count > 15 {
		t.Errorf("hop guard did not bound the chain: %d events", count)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) { w.t.Logf("%s", p); return len(p), nil }

func incID() func() string {
	var mu sync.Mutex
	var n int
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return time.Now().Format("150405.000000000") + "-" + itoa(n)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
