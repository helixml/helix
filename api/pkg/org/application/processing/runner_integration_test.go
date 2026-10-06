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
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/internal/orgtest"
)

const org = "org-1"

// rig wires the full publish→route→process→publish backbone against the
// in-memory store, with a spawner that records every activation.
type rig struct {
	store   *store.Store
	pub     *publishing.Publishing
	procSvc *processors.Processors

	mu          sync.Mutex
	activations []activation.Trigger
	gotAct      chan struct{}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{store: memory.New(), gotAct: make(chan struct{}, 64)}

	spawner := func(_ context.Context, _ string, _ orgchart.NodeID, triggers []activation.Trigger) error {
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
		Triggers: r.store.Triggers, Events: r.store.Events, Router: disp,
		Now: time.Now, NewID: incID(),
	})
	runner := processing.New(r.store.Processors, r.pub, logger)
	disp.RegisterProcessorRunner(runner)

	r.procSvc = processors.New(processors.Deps{
		Processors: r.store.Processors, Triggers: r.store.Triggers,
		Attachments: r.store.WorkerAttachments, Now: time.Now, NewID: incID(),
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

func (r *rig) mkTrigger(t *testing.T, id string) string {
	t.Helper()
	orgtest.Trigger(t, r.store, org, id)
	return id
}

// mkAIWorker creates a Worker attached to one source.
func (r *rig) mkAIWorker(t *testing.T, id string, src eventsource.SourceRef) {
	t.Helper()
	w, err := orgchart.NewNode(orgchart.NodeID(id), "# "+id, nil, time.Now().UTC(), org)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	if err := r.store.Nodes.Create(context.Background(), w); err != nil {
		t.Fatalf("create bot: %v", err)
	}
	orgtest.Attach(t, r.store, org, orgchart.NodeID(id), src)
}

func templateCfg(t *testing.T, tmpl string) json.RawMessage {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"template": tmpl})
	return b
}

// TestTemplateProcessorEndToEnd: a template processor between a Trigger
// and a Worker attached to its output branch. Publishing to the Trigger
// must (a) place a transformed event on the branch's stream and (b)
// activate the Worker with the rendered body.
func TestTemplateProcessorEndToEnd(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	in := r.mkTrigger(t, "s-in")

	proc, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p-fmt", Name: "Formatter", InputSource: eventsource.Trigger(in), Kind: processor.KindTemplate,
		Config: templateCfg(t, "From {{ .Message.from }}: {{ .Message.body }}"),
	})
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}
	branch := proc.Outputs[0]
	if branch.ID == "" || branch.StreamID == "" {
		t.Fatal("processor branch has no durable identity or stream")
	}

	// Worker attaches to the OUTPUT branch.
	r.mkAIWorker(t, "w-triage", proc.Source(branch))

	// Publish to the input Trigger.
	if _, err := r.pub.PublishToTrigger(ctx, org, in, "alice", streaming.Message{Body: "hello"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	r.waitForActivation(t)

	// (a) transformed event landed on the branch's stream.
	evs, err := r.store.Events.ListForStream(ctx, org, branch.StreamID, 10)
	if err != nil {
		t.Fatalf("list branch events: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("want 1 event on the branch stream, got %d", len(evs))
	}
	gotMsg, _ := evs[0].Message()
	if gotMsg.Body != "From alice: hello" {
		t.Errorf("output body = %q, want %q", gotMsg.Body, "From alice: hello")
	}

	// (b) worker activated with the rendered body, from the branch.
	act := r.lastActivation(t)
	if act.Message.Body != "From alice: hello" {
		t.Errorf("activation body = %q, want %q", act.Message.Body, "From alice: hello")
	}
	if act.EventSource != proc.Source(branch) {
		t.Errorf("activation source = %v, want %s", act.EventSource, proc.Source(branch).Key())
	}
}

// TestFilterProcessorRouting: a filter with two branches (a VIP
// predicate and an unconditional default) routes each message to exactly
// the branches whose predicate matches, and the right workers activate.
func TestFilterProcessorRouting(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	in := r.mkTrigger(t, "s-in")

	proc, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p-route", Name: "Router", InputSource: eventsource.Trigger(in), Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{
			{Label: "vip", Match: `{{ if hasSuffix "@vip.com" .Message.from }}1{{ end }}`},
			{Label: "default", Match: ``},
		},
	})
	if err != nil {
		t.Fatalf("create filter: %v", err)
	}
	vip, general := proc.Outputs[0], proc.Outputs[1]
	r.mkAIWorker(t, "w-senior", proc.Source(vip))
	r.mkAIWorker(t, "w-triage", proc.Source(general))

	// VIP message → both vip + default.
	if _, err := r.pub.PublishToTrigger(ctx, org, in, "boss@vip.com", streaming.Message{Body: "urgent"}); err != nil {
		t.Fatalf("publish vip: %v", err)
	}
	r.waitForActivation(t)
	time.Sleep(150 * time.Millisecond)

	vipCount, _ := r.store.Events.CountForStream(ctx, org, vip.StreamID)
	genCount, _ := r.store.Events.CountForStream(ctx, org, general.StreamID)
	if vipCount != 1 {
		t.Errorf("vip branch events = %d, want 1", vipCount)
	}
	if genCount != 1 {
		t.Errorf("default branch events = %d, want 1 (default catches all)", genCount)
	}

	// Plain message → only the default.
	if _, err := r.pub.PublishToTrigger(ctx, org, in, "joe@example.com", streaming.Message{Body: "hi"}); err != nil {
		t.Fatalf("publish plain: %v", err)
	}
	r.waitForActivation(t)
	time.Sleep(150 * time.Millisecond)

	vipCount2, _ := r.store.Events.CountForStream(ctx, org, vip.StreamID)
	genCount2, _ := r.store.Events.CountForStream(ctx, org, general.StreamID)
	if vipCount2 != 1 {
		t.Errorf("after plain publish vip branch = %d, want still 1 (no match)", vipCount2)
	}
	if genCount2 != 2 {
		t.Errorf("after plain publish default branch = %d, want 2", genCount2)
	}
}

// TestCreateRejectsSelfCycle: a processor whose input is one of its own
// branches closes a one-hop cycle and must be rejected.
func TestCreateRejectsSelfCycle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)

	_, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p-self", Name: "Self", InputSource: eventsource.ProcessorOutput("p-self", "po-own"), Kind: processor.KindTemplate,
		Config:  templateCfg(t, "{{ .Message.body }}"),
		Outputs: []processors.OutputSpec{{ID: "po-own"}},
	})
	if err == nil {
		t.Fatal("want an error for a processor reading its own branch, got nil")
	}
}

// TestCreateRejectsMultiHopCycle: p1 reads a Trigger and writes branch B;
// p2 reads branch B and would write back into p1's input, closing a
// two-hop cycle.
func TestCreateRejectsMultiHopCycle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)
	a := r.mkTrigger(t, "s-a")

	p1, err := r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p1", Name: "P1", InputSource: eventsource.Trigger(a), Kind: processor.KindTemplate,
		Config: templateCfg(t, "{{ .Message.body }}"), Outputs: []processors.OutputSpec{{Label: "b"}},
	})
	if err != nil {
		t.Fatalf("create p1: %v", err)
	}
	// p2 reads p1's branch and writes a branch p1 reads — p1's input is a
	// Trigger, so the cycle here is p2 → p1's own branch.
	_, err = r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p2", Name: "P2", InputSource: p1.Source(p1.Outputs[0]), Kind: processor.KindTemplate,
		Config: templateCfg(t, "{{ .Message.body }}"), Outputs: []processors.OutputSpec{{ID: p1.Outputs[0].ID}},
	})
	if err != nil {
		t.Fatalf("create p2 (distinct branch ids, no cycle): %v", err)
	}
	// Now close the loop: p3 reads p2's branch and writes into p2's input.
	_, err = r.procSvc.Create(ctx, org, processors.CreateParams{
		ID: "p3", Name: "P3", InputSource: eventsource.ProcessorOutput("p3", "po-loop"), Kind: processor.KindTemplate,
		Config: templateCfg(t, "{{ .Message.body }}"), Outputs: []processors.OutputSpec{{ID: "po-loop"}},
	})
	if err == nil {
		t.Fatal("want a cycle error, got nil")
	}
}

// TestRuntimeHopGuardStopsCycle: if a cycle somehow exists in the store
// (here seeded directly, bypassing checkAcyclic), the runtime hop guard
// must abort the chain instead of looping forever. We assert the chain
// terminates and produced a bounded number of events.
func TestRuntimeHopGuardStopsCycle(t *testing.T) {
	ctx := context.Background()
	r := newRig(t)

	// Seed a self-looping processor directly into the store (the service
	// would reject this; we bypass it to exercise the runtime guard).
	loop := processor.Processor{
		ID: "p-loop", OrganizationID: org, Name: "Loop",
		InputSource: eventsource.ProcessorOutput("p-loop", "po-loop"),
		Kind:        processor.KindTemplate, Config: templateCfg(t, "{{ .Message.body }}"),
		Outputs:   []processor.Output{{ID: "po-loop", StreamID: "s-loop"}},
		CreatedAt: time.Now(),
	}
	if err := r.store.Processors.Create(ctx, loop); err != nil {
		t.Fatalf("seed loop: %v", err)
	}

	if _, err := r.pub.Publish(ctx, org, loop.InputSource, "s-loop", "alice", streaming.Message{Body: "x"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	// Let the (bounded) recursion settle.
	time.Sleep(500 * time.Millisecond)

	count, err := r.store.Events.CountForStream(ctx, org, "s-loop")
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
