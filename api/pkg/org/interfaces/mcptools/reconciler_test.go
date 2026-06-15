package mcptools

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// TestRoleReconcilerBackfillsMissingBaseline simulates the production
// bug from helixml/helix#2546: an `r-qa-engineer`-shaped Role missing
// the baseline reads. After Reconcile the Role's tools must be the
// caller-order-preserving union with BaseReadTools.
func TestRoleReconcilerBackfillsMissingBaseline(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)

	created := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	role, err := orgchart.NewRole(
		"r-qa", "# QA",
		[]tool.Name{DMName, PublishName, ReadEventsName, SubscribeName},
		nil, created, "org-test",
	)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}

	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	rec := &RoleReconciler{Store: st, Now: func() time.Time { return now }}
	if err := rec.Reconcile(ctx, "org-test"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := st.Roles.Get(ctx, "org-test", "r-qa")
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	want := []tool.Name{
		// Caller-order preserved.
		DMName, PublishName, ReadEventsName, SubscribeName,
		// Baseline tail in BaseReadTools order, minus the already-present
		// `read_events`.
		ManagersName,
		ReportsName,
		ListWorkersName,
		GetWorkerName,
		ListRolesName,
		GetRoleName,
		ListStreamsName,
		GetStreamName,
		ListStreamEventsName,
		WorkerLogName,
		GetWorkerEnvironmentName,
		MintCredentialName,
	}
	if !reflect.DeepEqual(got.Tools, want) {
		t.Fatalf("reconciled tools drifted.\n got: %v\nwant: %v", got.Tools, want)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt should bump to reconcile clock.\n got: %v\nwant: %v", got.UpdatedAt, now)
	}
}

// TestRoleReconcilerIdempotent is the central correctness property: a
// second Reconcile against an already-baselined Role must not rewrite
// the row. We assert this by checking UpdatedAt — if the reconciler
// blindly wrote on every pass, the second run's now() would bump it.
func TestRoleReconcilerIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)

	created := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	role, err := orgchart.NewRole(
		"r-with-baseline", "# Role",
		// Pre-baselined: identical to what mergeBaseReadTools would
		// emit if seeded with []. The reconciler should see no drift.
		append([]tool.Name(nil), BaseReadTools...),
		nil, created, "org-test",
	)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}

	firstNow := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	rec := &RoleReconciler{Store: st, Now: func() time.Time { return firstNow }}
	if err := rec.Reconcile(ctx, "org-test"); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	afterFirst, err := st.Roles.Get(ctx, "org-test", "r-with-baseline")
	if err != nil {
		t.Fatalf("get after first: %v", err)
	}
	if !afterFirst.UpdatedAt.Equal(created) {
		t.Fatalf("first reconcile should be a no-op on pre-baselined role.\n got UpdatedAt: %v\nwant (created): %v",
			afterFirst.UpdatedAt, created)
	}

	secondNow := time.Date(2026, 6, 11, 13, 0, 0, 0, time.UTC)
	rec.Now = func() time.Time { return secondNow }
	if err := rec.Reconcile(ctx, "org-test"); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	afterSecond, err := st.Roles.Get(ctx, "org-test", "r-with-baseline")
	if err != nil {
		t.Fatalf("get after second: %v", err)
	}
	if !afterSecond.UpdatedAt.Equal(created) {
		t.Fatalf("second reconcile rewrote a baselined role.\n got UpdatedAt: %v\nwant (created): %v",
			afterSecond.UpdatedAt, created)
	}
}

// TestRoleReconcilerScopedToOrg guards against the easy mistake of
// reconciling roles outside the requested org. Two orgs, each with one
// drifted role; Reconcile("org-a") must touch r-a and leave r-b alone.
func TestRoleReconcilerScopedToOrg(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)

	created := time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC)
	for _, spec := range []struct {
		orgID string
		id    orgchart.RoleID
	}{
		{"org-a", "r-a"},
		{"org-b", "r-b"},
	} {
		role, err := orgchart.NewRole(spec.id, "#", []tool.Name{DMName}, nil, created, spec.orgID)
		if err != nil {
			t.Fatalf("new %s: %v", spec.id, err)
		}
		if err := st.Roles.Create(ctx, role); err != nil {
			t.Fatalf("create %s: %v", spec.id, err)
		}
	}

	rec := &RoleReconciler{Store: st, Now: func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }}
	if err := rec.Reconcile(ctx, "org-a"); err != nil {
		t.Fatalf("reconcile org-a: %v", err)
	}

	gotA, err := st.Roles.Get(ctx, "org-a", "r-a")
	if err != nil {
		t.Fatalf("get r-a: %v", err)
	}
	if len(gotA.Tools) != 1+len(BaseReadTools) {
		t.Fatalf("r-a should have been backfilled. got tools: %v", gotA.Tools)
	}

	gotB, err := st.Roles.Get(ctx, "org-b", "r-b")
	if err != nil {
		t.Fatalf("get r-b: %v", err)
	}
	if len(gotB.Tools) != 1 || gotB.Tools[0] != DMName {
		t.Fatalf("r-b should be untouched. got tools: %v", gotB.Tools)
	}
	if !gotB.UpdatedAt.Equal(created) {
		t.Fatalf("r-b UpdatedAt should not have moved. got: %v want: %v", gotB.UpdatedAt, created)
	}
}

// TestRoleReconcilerNilSafe documents the "best-effort" wiring story.
// helix_org_middleware.ensureBootstrap follows the topology reconciler
// pattern of constructing on the fly; if any field is nil the call
// should be a harmless no-op rather than a panic.
func TestRoleReconcilerNilSafe(t *testing.T) {
	t.Parallel()
	var rec *RoleReconciler
	if err := rec.Reconcile(context.Background(), "org-test"); err != nil {
		t.Fatalf("nil receiver should be a no-op, got: %v", err)
	}
	if err := (&RoleReconciler{}).Reconcile(context.Background(), "org-test"); err != nil {
		t.Fatalf("nil store should be a no-op, got: %v", err)
	}
}
