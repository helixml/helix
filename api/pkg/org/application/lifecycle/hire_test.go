package lifecycle_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func hireClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

type fakeAgentCreator struct{}

func (fakeAgentCreator) CreateAgent(context.Context, string, string, string, lifecycle.AgentConfig) (string, error) {
	return "app-agent", nil
}

type failingNodeReconciler struct{}

func (failingNodeReconciler) Reconcile(context.Context, string, ...orgchart.NodeID) error {
	return errors.New("reconcile failed")
}

// newHireService builds a lifecycle.Service wired only for Create (the
// create half) against the in-memory store. Delete-only collaborators
// (Helix / Mirror) stay nil — these tests never delete. Create delegates
// the row creation to nodes.Nodes service, so one is wired over the same
// memory store.
func newHireService(st *store.Store) *lifecycle.Service {
	rec := reconcile.New(reconcile.Deps{Nodes: st.Nodes, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions, Now: hireClock})
	botSvc := nodes.New(nodes.Deps{
		Nodes:      st.Nodes,
		Lines:      st.ReportingLines,
		Reconciler: rec,
		Now:        hireClock,
		NewID:      func() string { return "id" },
	})
	return &lifecycle.Service{
		Store:           st,
		Nodes:           botSvc,
		Agents:          fakeAgentCreator{},
		NodeReconcilers: []lifecycle.NodeReconciler{rec},
		Now:             hireClock,
		NewID:           func() string { return "id" },
	}
}

// TestCreate_CreatesBotAndReconciles: Create creates the bot row, wires
// the reporting line to the parent, and reconciles topology (the new
// bot's transcript materialises with the manager subscribed).
func TestCreate_CreatesBotAndReconciles(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st)
	ctx := context.Background()

	boss, _ := orgchart.NewNode("w-boss", "# Eng", nil, hireClock(), "org-test")
	if err := st.Nodes.Create(ctx, boss); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Create(ctx, "org-test", lifecycle.CreateParams{
		ID: "w-new", Content: "a new hire", ParentID: "w-boss",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Node.ID != "w-new" {
		t.Fatalf("node id = %q", res.Node.ID)
	}
	if res.Node.AgentID != "app-agent" {
		t.Fatalf("agent app id = %q, want app-agent", res.Node.AgentID)
	}
	if _, err := st.Nodes.Get(ctx, "org-test", "w-new"); err != nil {
		t.Fatalf("bot not persisted: %v", err)
	}
	managers, _ := st.ReportingLines.ListManagers(ctx, "org-test", "w-new")
	if len(managers) != 1 || managers[0] != "w-boss" {
		t.Fatalf("reporting line not wired: %v", managers)
	}
	// The reconciler created the new bot's transcript.
	if _, err := st.Topics.Get(ctx, "org-test", "s-transcript-w-new"); err != nil {
		t.Fatalf("transcript not reconciled: %v", err)
	}
}

// TestBotsCreate_RejectsDuplicateID pins that the id is used exactly: a
// second bot with an id that already exists in the org is rejected rather
// than silently suffixed. Deterministic-id seeds rely on this collision to
// stay idempotent; user-facing creates surface it as "id already taken".
func TestBotsCreate_RejectsDuplicateID(t *testing.T) {
	t.Parallel()
	st := memory.New()
	botSvc := nodes.New(nodes.Deps{
		Nodes: st.Nodes,
		Now:   hireClock,
		NewID: func() string { return "id" },
	})
	ctx := context.Background()

	first, err := botSvc.Create(ctx, "org-test", nodes.CreateParams{ID: "chief-of-staff", Content: "# Chief of Staff"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.ID != "chief-of-staff" {
		t.Fatalf("first id = %q, want chief-of-staff", first.ID)
	}

	if _, err := botSvc.Create(ctx, "org-test", nodes.CreateParams{ID: "chief-of-staff", Content: "# Another"}); err == nil {
		t.Fatalf("second create with a duplicate id should error, got nil")
	}

	// No suffixed row was created.
	if _, err := st.Nodes.Get(ctx, "org-test", "chief-of-staff-1"); err == nil {
		t.Fatalf("a suffixed bot chief-of-staff-1 was created; expected none")
	}
}

// TestCreate_RejectsPathTraversalID pins the path-injection guard: a bot
// id that would escape the envs directory is rejected before any
// os.MkdirAll, and nothing is created under the temp envs root.
func TestCreate_RejectsPathTraversalID(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st)
	ctx := context.Background()
	_, err := svc.Create(ctx, "org-test", lifecycle.CreateParams{
		ID: "../../escape", Content: "x",
	})
	if err == nil {
		t.Fatal("Create with traversal id: want error")
	}
	// No bot row persisted.
	if _, gerr := st.Nodes.Get(ctx, "org-test", "../../escape"); gerr == nil {
		t.Fatal("traversal bot should not have been created")
	}
}

// TestCreate_UnknownParent: creating a bot whose parent does not exist
// fails (and does not persist the child).
func TestCreate_UnknownParent(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st)
	_, err := svc.Create(context.Background(), "org-test", lifecycle.CreateParams{
		ID: "w-new", Content: "x", ParentID: "w-missing",
	})
	if err == nil {
		t.Fatal("Create with unknown parent: want error")
	}
}

func TestCreate_RollsBackBotWhenReconcileFails(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st)
	svc.NodeReconcilers = []lifecycle.NodeReconciler{failingNodeReconciler{}}

	_, err := svc.Create(context.Background(), "org-test", lifecycle.CreateParams{ID: "w-new", Content: "x"})

	if err == nil {
		t.Fatal("Create: want error")
	}
	if _, err := st.Nodes.Get(context.Background(), "org-test", "w-new"); err == nil {
		t.Fatal("failed create left bot row")
	}
}

type recordingDispatcher struct{ hires int }

func (r *recordingDispatcher) DispatchHire(context.Context, string, orgchart.NodeID, activation.ID) {
	r.hires++
}

// TestCreate_AlwaysDispatchesActivation pins the creation contract: every
// newly-created agent bot gets an activation row and an immediate hire.
func TestCreate_AlwaysDispatchesActivation(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st)
	disp := &recordingDispatcher{}
	svc.Dispatcher = disp

	res, err := svc.Create(context.Background(), "org-test", lifecycle.CreateParams{
		ID: "w-new", Content: "x",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.ActivationID == "" {
		t.Fatal("create should return an ActivationID")
	}
	if disp.hires != 1 {
		t.Fatalf("create should dispatch exactly one hire, got %d", disp.hires)
	}
}

func TestCreate_DeferredDoesNotCreateOrDispatchActivation(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st)
	disp := &recordingDispatcher{}
	svc.Dispatcher = disp

	res, err := svc.Create(context.Background(), "org-test", lifecycle.CreateParams{
		ID: "w-new", Content: "x", DeferActivation: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.ActivationID != "" {
		t.Fatalf("deferred create activation = %q, want empty", res.ActivationID)
	}
	if disp.hires != 0 {
		t.Fatalf("deferred create dispatched %d hires", disp.hires)
	}
	rows, err := st.Activations.ListForWorker(context.Background(), "org-test", res.Node.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("deferred activation rows = %v, want none", rows)
	}
}
