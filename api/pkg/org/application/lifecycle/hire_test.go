package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func hireClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

// newHireService builds a lifecycle.Service wired only for Create (the
// create half) against the in-memory store. Delete-only collaborators
// (Helix / Mirror) stay nil — these tests never delete. Create delegates
// the row creation to a bots.Bots service, so one is wired over the same
// memory store.
func newHireService(st *store.Store) *lifecycle.Service {
	rec := reconcile.New(reconcile.Deps{Bots: st.Bots, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions, Now: hireClock})
	botSvc := bots.New(bots.Deps{
		Bots:       st.Bots,
		Lines:      st.ReportingLines,
		Reconciler: rec,
		Now:        hireClock,
		NewID:      func() string { return "id" },
	})
	return &lifecycle.Service{
		Store:          st,
		Bots:           botSvc,
		BotReconcilers: []lifecycle.BotReconciler{rec},
		Now:            hireClock,
		NewID:          func() string { return "id" },
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

	boss, _ := orgchart.NewBot("w-boss", "# Eng", nil, hireClock(), "org-test")
	if err := st.Bots.Create(ctx, boss); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Create(ctx, "org-test", lifecycle.CreateParams{
		ID: "w-new", Content: "a new hire", ParentID: "w-boss",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if res.Bot.ID != "w-new" {
		t.Fatalf("bot id = %q", res.Bot.ID)
	}
	if _, err := st.Bots.Get(ctx, "org-test", "w-new"); err != nil {
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
	if _, gerr := st.Bots.Get(ctx, "org-test", "../../escape"); gerr == nil {
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
