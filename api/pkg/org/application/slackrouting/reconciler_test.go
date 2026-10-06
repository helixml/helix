package slackrouting_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/attachments"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/internal/orgtest"
)

const org = "org-1"

func setup(t *testing.T) (*store.Store, *processors.Processors, *slackrouting.Reconciler) {
	t.Helper()
	var n int
	id := func() string {
		n++
		return time.Now().Format("150405.000000") + "-" + string(rune('a'+n%26)) + string(rune('a'+n/26%26))
	}
	s := memory.New()
	procs := processors.New(processors.Deps{Processors: s.Processors, Triggers: s.Triggers, Attachments: s.WorkerAttachments, NewID: id})
	attach := attachments.New(attachments.Deps{Store: s, NewID: id})
	rec := slackrouting.New(slackrouting.Deps{Nodes: s.Nodes, Attachments: attach, Processors: procs})
	return s, procs, rec
}

// makeRouter creates an Automated filter router on a Slack Trigger.
func makeRouter(t *testing.T, ctx context.Context, s *store.Store, procs *processors.Processors) processor.Processor {
	t.Helper()
	orgtest.Trigger(t, s, org, "s-slack")
	p, err := procs.Create(ctx, org, processors.CreateParams{
		ID: "p-slack-router", Name: "Slack Router", InputSource: eventsource.Trigger("s-slack"), Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: processor.SystemActor,
	})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	return p
}

func addBot(t *testing.T, ctx context.Context, s *store.Store, id string) {
	t.Helper()
	now := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	b, err := orgchart.NewNode(id, "content", nil, now, org)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Nodes.Create(ctx, b); err != nil {
		t.Fatal(err)
	}
}

func routesByWorker(p processor.Processor) map[string]processor.Output {
	m := map[string]processor.Output{}
	for _, o := range p.Outputs {
		if o.ManagedFor != "" {
			m[o.ManagedFor] = o
		}
	}
	return m
}

func TestReconcileAddsRoutePerAIWorkerAndAttaches(t *testing.T) {
	ctx := context.Background()
	s, procs, rec := setup(t)
	makeRouter(t, ctx, s, procs)
	addBot(t, ctx, s, "w-alice")
	addBot(t, ctx, s, "w-bob")

	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	p, _ := procs.Get(ctx, org, "p-slack-router")
	routes := routesByWorker(p)
	if len(routes) != 2 {
		t.Fatalf("want 2 managed routes, got %d (%+v)", len(routes), p.Outputs)
	}
	for _, wid := range []string{"w-alice", "w-bob"} {
		o, ok := routes[wid]
		if !ok {
			t.Fatalf("no route for %s", wid)
		}
		// The predicate matches on the FULL worker id (w-alice), not the slug.
		want := `{{ mentions "` + wid + `" .Message.body }}`
		if o.Match != want {
			t.Errorf("%s route predicate = %q, want %q", wid, o.Match, want)
		}
		// Worker attached to the route's output branch.
		if !attached(t, ctx, s, orgchart.NodeID(wid), eventsource.ProcessorOutput(p.ID, o.ID)) {
			t.Errorf("%s not attached to %s", wid, o.ID)
		}
	}
	// The default (unconditional) route is untouched.
	if len(p.Outputs) != 3 {
		t.Errorf("want 3 outputs (default + 2 managed), got %d", len(p.Outputs))
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, procs, rec := setup(t)
	makeRouter(t, ctx, s, procs)
	addBot(t, ctx, s, "w-alice")

	_ = rec.Reconcile(ctx, org)
	_ = rec.Reconcile(ctx, org)
	p, _ := procs.Get(ctx, org, "p-slack-router")
	if got := len(routesByWorker(p)); got != 1 {
		t.Fatalf("want 1 managed route after double reconcile, got %d", got)
	}
}

func TestReconcileRemovesRouteForDepartedWorker(t *testing.T) {
	ctx := context.Background()
	s, procs, rec := setup(t)
	makeRouter(t, ctx, s, procs)
	addBot(t, ctx, s, "w-alice")
	addBot(t, ctx, s, "w-bob")
	_ = rec.Reconcile(ctx, org)

	// Bob leaves.
	if err := s.Nodes.Delete(ctx, org, "w-bob"); err != nil {
		t.Fatal(err)
	}
	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	p, _ := procs.Get(ctx, org, "p-slack-router")
	routes := routesByWorker(p)
	if _, ok := routes["w-bob"]; ok {
		t.Errorf("bob's route should be gone")
	}
	if _, ok := routes["w-alice"]; !ok {
		t.Errorf("alice's route should remain")
	}
}

func TestReconcilePreservesManualRoutesAndEdits(t *testing.T) {
	ctx := context.Background()
	s, procs, rec := setup(t)
	makeRouter(t, ctx, s, procs)
	addBot(t, ctx, s, "w-alice")
	_ = rec.Reconcile(ctx, org)

	// User adds a manual route (empty ManagedFor) and edits alice's predicate.
	if _, err := procs.AddOutput(ctx, org, "p-slack-router", processors.OutputSpec{Label: "manual", Match: "x"}); err != nil {
		t.Fatal(err)
	}
	p, _ := procs.Get(ctx, org, "p-slack-router")
	for i := range p.Outputs {
		if p.Outputs[i].ManagedFor == "w-alice" {
			p.Outputs[i].Match = "EDITED"
		}
	}
	if err := s.Processors.Update(ctx, p); err != nil {
		t.Fatal(err)
	}

	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	p, _ = procs.Get(ctx, org, "p-slack-router")
	// Manual route still present.
	manual := 0
	for _, o := range p.Outputs {
		if o.Label == "manual" && o.ManagedFor == "" {
			manual++
		}
	}
	if manual != 1 {
		t.Errorf("manual route should be preserved, found %d", manual)
	}
	// Alice's edited predicate untouched.
	if routesByWorker(p)["w-alice"].Match != "EDITED" {
		t.Errorf("alice's edited predicate was overwritten: %q", routesByWorker(p)["w-alice"].Match)
	}
}

// Two workspaces in one org → two automated routers. The reconciler must
// maintain a managed route (and attachment) per AI Worker on BOTH, so a
// Worker is reachable by name from either workspace.
func TestReconcileMaintainsEveryRouterInOrg(t *testing.T) {
	ctx := context.Background()
	s, procs, rec := setup(t)
	makeRouter(t, ctx, s, procs) // p-slack-router on s-slack
	// A second workspace's router on its own Trigger.
	orgtest.Trigger(t, s, org, "s-slack-2")
	if _, err := procs.Create(ctx, org, processors.CreateParams{
		ID: "p-slack-router-2", Name: "Slack Router 2", InputSource: eventsource.Trigger("s-slack-2"), Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: processor.SystemActor,
	}); err != nil {
		t.Fatal(err)
	}
	addBot(t, ctx, s, "w-alice")

	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, rid := range []string{"p-slack-router", "p-slack-router-2"} {
		p, _ := procs.Get(ctx, org, rid)
		route, ok := routesByWorker(p)["w-alice"]
		if !ok {
			t.Fatalf("router %s has no route for w-alice", rid)
		}
		if !attached(t, ctx, s, "w-alice", eventsource.ProcessorOutput(p.ID, route.ID)) {
			t.Errorf("w-alice not attached to %s's route branch %s", rid, route.ID)
		}
	}
}

func TestReconcileNoRouterIsNoOp(t *testing.T) {
	ctx := context.Background()
	s, _, rec := setup(t)
	addBot(t, ctx, s, "w-alice")
	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("reconcile with no router should be no-op, got %v", err)
	}
}

// attached reports whether the Worker has an attachment to the source.
func attached(t *testing.T, ctx context.Context, s *store.Store, worker orgchart.NodeID, src eventsource.SourceRef) bool {
	t.Helper()
	rows, err := s.WorkerAttachments.Find(ctx, store.WithOrg(org), store.WithWorkerID(worker),
		store.WithProcessorID(src.ProcessorID), store.WithOutputID(src.OutputID), store.WithLimit(1))
	if err != nil {
		t.Fatalf("find attachment: %v", err)
	}
	return len(rows) > 0
}
