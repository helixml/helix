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
	rolesSvc := roles.New(roles.Deps{Roles: st.Roles, Now: fixedClock, NewID: func() string { return "id" }})
	return New(Deps{
		Workers:  st.Workers,
		Roles:    rolesSvc,
		Lines:    st.ReportingLines,
		Topology: topology.NewReconciler(topology.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions, Now: fixedClock}),
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
