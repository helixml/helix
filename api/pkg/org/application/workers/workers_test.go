package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/application/topology"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func fixedClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

func newService(st *store.Store) *Workers {
	return newServiceEnv(st, "")
}

func newServiceEnv(st *store.Store, envsDir string) *Workers {
	rolesSvc := roles.New(roles.Deps{Roles: st.Roles, Now: fixedClock, NewID: func() string { return "id" }})
	return New(Deps{
		Workers:      st.Workers,
		Roles:        rolesSvc,
		Lines:        st.ReportingLines,
		Topology:     &topology.Reconciler{Store: st, Now: fixedClock},
		Environments: st.Environments,
		Activations:  st.Activations,
		EnvsDir:      envsDir,
		Now:          fixedClock,
		NewID:        func() string { return "id" },
	})
}

// seedWorker creates a role + AI worker so the update tests have a
// target. Returns the worker id.
func seedWorker(t *testing.T, st *store.Store, orgID string) orgchart.WorkerID {
	t.Helper()
	ctx := context.Background()
	role, err := orgchart.NewRole("r-eng", "# Engineer", []tool.Name{"publish"}, []streaming.StreamID{"s-a"}, fixedClock(), orgID)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	wk, err := orgchart.NewAIWorker("w-mark", "r-eng", "original identity", orgID)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := st.Workers.Create(ctx, wk); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	return wk.ID()
}

func TestWorkersUpdateIdentity_HappyPath(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	id := seedWorker(t, st, "org-test")

	got, err := svc.UpdateIdentity(ctx, "org-test", id, "new identity")
	if err != nil {
		t.Fatalf("UpdateIdentity: %v", err)
	}
	if got.IdentityContent() != "new identity" {
		t.Fatalf("identity = %q, want 'new identity'", got.IdentityContent())
	}
	stored, _ := st.Workers.Get(ctx, "org-test", id)
	if stored.IdentityContent() != "new identity" {
		t.Fatalf("stored identity = %q", stored.IdentityContent())
	}
	// Role unchanged.
	if stored.RoleID() != "r-eng" {
		t.Fatalf("role changed: %q", stored.RoleID())
	}
}

func TestWorkersUpdateIdentity_NotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	_, err := svc.UpdateIdentity(context.Background(), "org-test", "w-missing", "x")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWorkersUpdateIdentity_OrgScoping(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	seedWorker(t, st, "org-a")
	_, err := svc.UpdateIdentity(context.Background(), "org-b", "w-mark", "hacked")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-org err = %v, want ErrNotFound", err)
	}
}

// TestWorkersUpdateRole_UpdatesHeldRoleContent: updating a worker's role
// rewrites the content of the role the worker holds, preserving the
// role's tools and streams (the same invariant the roles service owns).
func TestWorkersUpdateRole_UpdatesHeldRoleContent(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	id := seedWorker(t, st, "org-test")

	got, err := svc.UpdateRole(ctx, "org-test", id, "# Engineer v2")
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if got.Content != "# Engineer v2" {
		t.Fatalf("content = %q", got.Content)
	}
	// Tools + streams preserved.
	if len(got.Tools) == 0 || got.Streams[0] != "s-a" {
		t.Fatalf("tools/streams lost: tools=%v streams=%v", got.Tools, got.Streams)
	}
	stored, _ := st.Roles.Get(ctx, "org-test", "r-eng")
	if stored.Content != "# Engineer v2" {
		t.Fatalf("stored role content = %q", stored.Content)
	}
}

func TestWorkersUpdateRole_WorkerNotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	_, err := svc.UpdateRole(context.Background(), "org-test", "w-missing", "x")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// seedTwoWorkers creates manager + report workers (no line yet).
func seedTwoWorkers(t *testing.T, st *store.Store, orgID string) {
	t.Helper()
	ctx := context.Background()
	role, _ := orgchart.NewRole("r-eng", "# Engineer", []tool.Name{"publish"}, nil, fixedClock(), orgID)
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	for _, id := range []orgchart.WorkerID{"w-boss", "w-report"} {
		wk, _ := orgchart.NewAIWorker(id, "r-eng", "id", orgID)
		if err := st.Workers.Create(ctx, wk); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
}

func TestWorkersAddParent_WiresLineAndReconciles(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seedTwoWorkers(t, st, "org-test")

	if err := svc.AddParent(ctx, "org-test", "w-report", "w-boss"); err != nil {
		t.Fatalf("AddParent: %v", err)
	}
	managers, _ := st.ReportingLines.ListManagers(ctx, "org-test", "w-report")
	if len(managers) != 1 || managers[0] != "w-boss" {
		t.Fatalf("managers = %v, want [w-boss]", managers)
	}
	// Idempotent re-add.
	if err := svc.AddParent(ctx, "org-test", "w-report", "w-boss"); err != nil {
		t.Fatalf("AddParent (repeat): %v", err)
	}
}

func TestWorkersAddParent_CycleRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seedTwoWorkers(t, st, "org-test")
	if err := svc.AddParent(ctx, "org-test", "w-report", "w-boss"); err != nil {
		t.Fatalf("AddParent: %v", err)
	}
	// Now make w-boss report to w-report → cycle.
	err := svc.AddParent(ctx, "org-test", "w-boss", "w-report")
	if !errors.Is(err, ErrReportingCycle) {
		t.Fatalf("err = %v, want ErrReportingCycle", err)
	}
}

func TestWorkersAddParent_UnknownWorker(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seedTwoWorkers(t, st, "org-test")
	err := svc.AddParent(ctx, "org-test", "w-report", "w-ghost")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWorkersRemoveParent(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	seedTwoWorkers(t, st, "org-test")
	if err := svc.AddParent(ctx, "org-test", "w-report", "w-boss"); err != nil {
		t.Fatalf("AddParent: %v", err)
	}
	if err := svc.RemoveParent(ctx, "org-test", "w-report", "w-boss"); err != nil {
		t.Fatalf("RemoveParent: %v", err)
	}
	managers, _ := st.ReportingLines.ListManagers(ctx, "org-test", "w-report")
	if len(managers) != 0 {
		t.Fatalf("managers = %v, want empty", managers)
	}
	// Removing again → not found.
	if err := svc.RemoveParent(ctx, "org-test", "w-report", "w-boss"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// TestWorkersHire_CreatesWorkerEnvAndReconciles: Hire creates the worker
// + environment row, wires the reporting line to the parent, and
// reconciles topology (the hire's activation stream materialises with
// the manager subscribed).
func TestWorkersHire_CreatesWorkerEnvAndReconciles(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newServiceEnv(st, t.TempDir())
	ctx := context.Background()

	role, _ := orgchart.NewRole("r-eng", "# Eng", []tool.Name{"publish"}, nil, fixedClock(), "org-test")
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatal(err)
	}
	boss, _ := orgchart.NewAIWorker("w-boss", "r-eng", "id", "org-test")
	if err := st.Workers.Create(ctx, boss); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Hire(ctx, "org-test", HireParams{
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

func TestWorkersHire_RejectsUnknownKind(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newServiceEnv(st, t.TempDir())
	_, err := svc.Hire(context.Background(), "org-test", HireParams{
		RoleID: "r-eng", Kind: "claude", IdentityContent: "x",
	})
	if err == nil {
		t.Fatal("Hire with unknown kind: want error")
	}
}

// TestWorkersHire_RejectsPathTraversalID pins the path-injection guard:
// a worker id that would escape the envs directory is rejected before
// any os.MkdirAll, and nothing is created under the temp envs root.
func TestWorkersHire_RejectsPathTraversalID(t *testing.T) {
	t.Parallel()
	st := memory.New()
	envs := t.TempDir()
	svc := newServiceEnv(st, envs)
	ctx := context.Background()
	role, _ := orgchart.NewRole("r-eng", "# Eng", []tool.Name{"publish"}, nil, fixedClock(), "org-test")
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Hire(ctx, "org-test", HireParams{
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

func TestWorkersHire_UnknownRole(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newServiceEnv(st, t.TempDir())
	_, err := svc.Hire(context.Background(), "org-test", HireParams{
		RoleID: "r-missing", Kind: orgchart.WorkerKindAI, IdentityContent: "x",
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestWorkersAddParent_NoLinesRepo(t *testing.T) {
	t.Parallel()
	st := memory.New()
	rolesSvc := roles.New(roles.Deps{Roles: st.Roles, Now: fixedClock, NewID: func() string { return "id" }})
	svc := New(Deps{Workers: st.Workers, Roles: rolesSvc}) // no Lines
	err := svc.AddParent(context.Background(), "org-test", "w-report", "w-boss")
	if !errors.Is(err, ErrReportingLinesUnavailable) {
		t.Fatalf("err = %v, want ErrReportingLinesUnavailable", err)
	}
}
