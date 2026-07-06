package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
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

// TestBotsCreate_SuffixesDuplicateID pins the name-collision fix in
// bots.Create: two bots whose ids collide (e.g. a second "Chief of Staff"
// slugifying to the same handle) don't fail on the composite (id, org)
// primary key — the second is suffixed base-1 rather than erroring.
func TestBotsCreate_SuffixesDuplicateID(t *testing.T) {
	t.Parallel()
	st := memory.New()
	botSvc := bots.New(bots.Deps{
		Bots:  st.Bots,
		Now:   hireClock,
		NewID: func() string { return "id" },
	})
	ctx := context.Background()

	first, err := botSvc.Create(ctx, "org-test", bots.CreateParams{ID: "chief-of-staff", Content: "# Chief of Staff"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.ID != "chief-of-staff" {
		t.Fatalf("first id = %q, want chief-of-staff", first.ID)
	}

	second, err := botSvc.Create(ctx, "org-test", bots.CreateParams{ID: "chief-of-staff", Content: "# Another"})
	if err != nil {
		t.Fatalf("second create should suffix, not error: %v", err)
	}
	if second.ID != "chief-of-staff-1" {
		t.Fatalf("second id = %q, want chief-of-staff-1", second.ID)
	}

	// Both rows exist independently.
	if _, err := st.Bots.Get(ctx, "org-test", "chief-of-staff"); err != nil {
		t.Fatalf("first bot missing: %v", err)
	}
	if _, err := st.Bots.Get(ctx, "org-test", "chief-of-staff-1"); err != nil {
		t.Fatalf("suffixed bot missing: %v", err)
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

type recordingDispatcher struct{ hires int }

func (r *recordingDispatcher) DispatchHire(context.Context, string, orgchart.BotID, activation.ID) {
	r.hires++
}

// TestCreate_DeferActivation: DeferActivation creates the bot row (and its
// topology) but skips the hire — no activation row, no dispatch, empty
// ActivationID. This is the "org has no runtime configured yet" path that
// keeps a seeded bot from being provisioned on the gpt default.
func TestCreate_DeferActivation(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st)
	disp := &recordingDispatcher{}
	svc.Dispatcher = disp
	ctx := context.Background()

	res, err := svc.Create(ctx, "org-test", lifecycle.CreateParams{
		ID: "w-new", Content: "x", DeferActivation: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := st.Bots.Get(ctx, "org-test", "w-new"); err != nil {
		t.Fatalf("bot row should still exist when deferred: %v", err)
	}
	if res.ActivationID != "" {
		t.Fatalf("deferred create should return empty ActivationID, got %q", res.ActivationID)
	}
	if disp.hires != 0 {
		t.Fatalf("deferred create must not dispatch a hire, got %d", disp.hires)
	}
}

// TestCreate_DispatchesWhenNotDeferred: the default path (runtime already
// configured) still dispatches the hire so the bot provisions immediately.
func TestCreate_DispatchesWhenNotDeferred(t *testing.T) {
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
		t.Fatal("non-deferred create should return an ActivationID")
	}
	if disp.hires != 1 {
		t.Fatalf("non-deferred create should dispatch exactly one hire, got %d", disp.hires)
	}
}
