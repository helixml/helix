package slackrouting_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	"github.com/helixml/helix/api/pkg/org/application/topics"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

const org = "org-1"

func setup(t *testing.T) (*store.Store, *processors.Processors, *slackrouting.Reconciler) {
	t.Helper()
	var n int
	id := func() string { n++; return time.Now().Format("150405.000000") + "-" + string(rune('a'+n%26)) + string(rune('a'+n/26%26)) }
	s := memory.New()
	top := topics.New(topics.Deps{Topics: s.Topics, NewID: id})
	procs := processors.New(processors.Deps{Processors: s.Processors, Topics: top, NewID: id})
	rec := slackrouting.New(slackrouting.Deps{Workers: s.Workers, Subscriptions: s.Subscriptions, Processors: procs})
	return s, procs, rec
}

// makeRouter creates an Automated filter router on a Slack input topic.
func makeRouter(t *testing.T, ctx context.Context, s *store.Store, procs *processors.Processors) processor.Processor {
	t.Helper()
	in, err := streaming.NewTopic("s-slack", "Slack", "", "", time.Now().UTC(), transport.Transport{}, org)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Topics.Create(ctx, in); err != nil {
		t.Fatal(err)
	}
	p, err := procs.Create(ctx, org, processors.CreateParams{
		ID: "p-slack-router", Name: "Slack Router", InputTopicID: "s-slack", Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: processor.SystemActor,
	})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	return p
}

func addAIWorker(t *testing.T, ctx context.Context, s *store.Store, id string) {
	t.Helper()
	w, err := orgchart.NewAIWorker(id, "r-x", "", org)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Workers.Create(ctx, w); err != nil {
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

func TestReconcileAddsRoutePerAIWorkerAndSubscribes(t *testing.T) {
	ctx := context.Background()
	s, procs, rec := setup(t)
	makeRouter(t, ctx, s, procs)
	addAIWorker(t, ctx, s, "w-alice")
	addAIWorker(t, ctx, s, "w-bob")

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
		// Worker subscribed to the route's output topic.
		if _, err := s.Subscriptions.Find(ctx, org, orgchart.WorkerID(wid), o.TopicID); err != nil {
			t.Errorf("%s not subscribed to %s: %v", wid, o.TopicID, err)
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
	addAIWorker(t, ctx, s, "w-alice")

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
	addAIWorker(t, ctx, s, "w-alice")
	addAIWorker(t, ctx, s, "w-bob")
	_ = rec.Reconcile(ctx, org)

	// Bob leaves.
	if err := s.Workers.Delete(ctx, org, "w-bob"); err != nil {
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
	addAIWorker(t, ctx, s, "w-alice")
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
// maintain a managed route (and subscription) per AI Worker on BOTH, so a
// Worker is reachable by name from either workspace.
func TestReconcileMaintainsEveryRouterInOrg(t *testing.T) {
	ctx := context.Background()
	s, procs, rec := setup(t)
	makeRouter(t, ctx, s, procs) // p-slack-router on s-slack
	// A second workspace's router on its own input topic.
	in2, _ := streaming.NewTopic("s-slack-2", "Slack 2", "", "", time.Now().UTC(), transport.Transport{}, org)
	_ = s.Topics.Create(ctx, in2)
	if _, err := procs.Create(ctx, org, processors.CreateParams{
		ID: "p-slack-router-2", Name: "Slack Router 2", InputTopicID: "s-slack-2", Kind: processor.KindFilter,
		Outputs: []processors.OutputSpec{{Label: "default"}}, CreatedBy: processor.SystemActor,
	}); err != nil {
		t.Fatal(err)
	}
	addAIWorker(t, ctx, s, "w-alice")

	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	for _, rid := range []string{"p-slack-router", "p-slack-router-2"} {
		p, _ := procs.Get(ctx, org, rid)
		route, ok := routesByWorker(p)["w-alice"]
		if !ok {
			t.Fatalf("router %s has no route for w-alice", rid)
		}
		if _, err := s.Subscriptions.Find(ctx, org, "w-alice", route.TopicID); err != nil {
			t.Errorf("w-alice not subscribed to %s's route topic %s: %v", rid, route.TopicID, err)
		}
	}
}

func TestReconcileNoRouterIsNoOp(t *testing.T) {
	ctx := context.Background()
	s, _, rec := setup(t)
	addAIWorker(t, ctx, s, "w-alice")
	if err := rec.Reconcile(ctx, org); err != nil {
		t.Fatalf("reconcile with no router should be no-op, got %v", err)
	}
}
