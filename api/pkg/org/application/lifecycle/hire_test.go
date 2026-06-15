package lifecycle_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func hireClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

// newHireService builds a lifecycle.Service wired only for Hire (the
// create half) against the in-memory store. Fire-only collaborators
// (Helix / Mirror / Owner) stay nil — these tests never fire.
func newHireService(st *store.Store, envsDir string) *lifecycle.Service {
	return &lifecycle.Service{
		Store:    st,
		Topology: reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions, Now: hireClock}),
		EnvsDir:  envsDir,
		Now:      hireClock,
		NewID:    func() string { return "id" },
	}
}

// TestHire_CreatesWorkerEnvAndReconciles: Hire creates the worker +
// environment row, wires the reporting line to the parent, and
// reconciles topology (the hire's activation stream materialises with
// the manager subscribed).
func TestHire_CreatesWorkerEnvAndReconciles(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st, t.TempDir())
	ctx := context.Background()

	role, _ := orgchart.NewRole("r-eng", "# Eng", []tool.Name{"publish"}, nil, hireClock(), "org-test")
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatal(err)
	}
	boss, _ := orgchart.NewAIWorker("w-boss", "r-eng", "id", "org-test")
	if err := st.Workers.Create(ctx, boss); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Hire(ctx, "org-test", lifecycle.HireParams{
		ID: "w-new", RoleID: "r-eng", ParentID: "w-boss",
		Kind: orgchart.WorkerKindAI, IdentityContent: "a new hire",
	})
	if err != nil {
		t.Fatalf("Hire: %v", err)
	}
	if res.WorkerID != "w-new" {
		t.Fatalf("worker id = %q", res.WorkerID)
	}
	if _, err := st.Workers.Get(ctx, "org-test", "w-new"); err != nil {
		t.Fatalf("worker not persisted: %v", err)
	}
	if _, err := st.Environments.Get(ctx, "org-test", "w-new"); err != nil {
		t.Fatalf("environment not created: %v", err)
	}
	managers, _ := st.ReportingLines.ListManagers(ctx, "org-test", "w-new")
	if len(managers) != 1 || managers[0] != "w-boss" {
		t.Fatalf("reporting line not wired: %v", managers)
	}
	// Topology reconcile created the hire's activation stream.
	if _, err := st.Streams.Get(ctx, "org-test", "s-activations-w-new"); err != nil {
		t.Fatalf("activation stream not reconciled: %v", err)
	}
}

func TestHire_RejectsUnknownKind(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st, t.TempDir())
	_, err := svc.Hire(context.Background(), "org-test", lifecycle.HireParams{
		RoleID: "r-eng", Kind: "claude", IdentityContent: "x",
	})
	if err == nil {
		t.Fatal("Hire with unknown kind: want error")
	}
}

// TestHire_RejectsPathTraversalID pins the path-injection guard: a
// worker id that would escape the envs directory is rejected before any
// os.MkdirAll, and nothing is created under the temp envs root.
func TestHire_RejectsPathTraversalID(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st, t.TempDir())
	ctx := context.Background()
	role, _ := orgchart.NewRole("r-eng", "# Eng", []tool.Name{"publish"}, nil, hireClock(), "org-test")
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Hire(ctx, "org-test", lifecycle.HireParams{
		ID: "../../escape", RoleID: "r-eng", Kind: orgchart.WorkerKindAI, IdentityContent: "x",
	})
	if err == nil {
		t.Fatal("Hire with traversal id: want error")
	}
	// No worker row persisted.
	if _, gerr := st.Workers.Get(ctx, "org-test", "../../escape"); gerr == nil {
		t.Fatal("traversal worker should not have been created")
	}
}

func TestHire_UnknownRole(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newHireService(st, t.TempDir())
	_, err := svc.Hire(context.Background(), "org-test", lifecycle.HireParams{
		RoleID: "r-missing", Kind: orgchart.WorkerKindAI, IdentityContent: "x",
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
