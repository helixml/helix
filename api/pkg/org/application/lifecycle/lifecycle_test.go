package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// TestFire_RemovesWorkersTranscript pins the regression behind
// "we still see s-transcript-w-ai-1 and s-transcript-w-test-ai
// even though those workers are gone": the Fire cascade tore down
// subscriptions, environment, runtime state, and the worker
// row — but left the per-Worker transcript
// (s-transcript-<workerID>) lying around, so the Topics page kept
// rendering ghost rows for workers that no longer existed and the
// chart's orphan strip filled up with dashed pseudo-nodes.
//
// Activation events themselves are still audit-retained (the
// `org_events` rows survive); only the Topic row is cleaned up so
// the UI surfaces stop showing it as an active channel.
func TestFire_RemovesWorkersTranscript(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-test"

	// Seed a role + worker + their transcript the same way
	// hire_worker would.
	role, err := orgchart.NewRole("r-owner", "# Owner", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	worker, err := orgchart.NewAIWorker("w-ghost", role.ID, "# Ghost", orgID)
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := st.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	topicID := activation.TranscriptID(worker.ID())
	topic, err := streaming.NewTopic(
		topicID, "Activations: w-ghost", "test",
		worker.ID(), time.Now().UTC(),
		transport.Transport{}, orgID,
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// Sanity: the topic is there before we fire.
	if _, err := st.Topics.Get(ctx, orgID, topicID); err != nil {
		t.Fatalf("precondition: transcript not seeded: %v", err)
	}

	svc := &lifecycle.Service{Store: st, Reconciler: reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions})}
	if err := svc.Fire(ctx, orgID, worker.ID()); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	// The topic row must be gone.
	if _, err := st.Topics.Get(ctx, orgID, topicID); err == nil {
		t.Fatalf("transcript %q still exists after Fire — orphan regression", topicID)
	}
}

// TestFire_CascadesReportingLinesAndSubscriptions pins the two cascade
// bugs found in the 2026-06-06 QA run, now handled structurally by the
// store:
//
//   - F8: firing a manager left their direct reports pointing at the
//     now-deleted worker. With reporting lines, firing the manager must
//     drop every line that references it (the gorm store does this with
//     ON DELETE CASCADE; the memory store mirrors it).
//   - F5: firing a worker deleted its s-transcript-<id> topic but
//     left OTHER workers' subscriptions to that topic behind.
func TestFire_CascadesReportingLinesAndSubscriptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-cascade"

	role, err := orgchart.NewRole("r-owner", "# Owner", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}

	mgr, err := orgchart.NewAIWorker("w-mgr", role.ID, "# Mgr", orgID)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := st.Workers.Create(ctx, mgr); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	report, err := orgchart.NewAIWorker("w-report", role.ID, "# Report", orgID)
	if err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := st.Workers.Create(ctx, report); err != nil {
		t.Fatalf("create report: %v", err)
	}
	// w-report reports to w-mgr.
	line, err := orgchart.NewReportingLine(orgID, "w-mgr", "w-report")
	if err != nil {
		t.Fatalf("new reporting line: %v", err)
	}
	if err := st.ReportingLines.Add(ctx, line); err != nil {
		t.Fatalf("add reporting line: %v", err)
	}

	// The manager's transcript + an outside subscriber (mirrors
	// the hiring caller auto-subscribed to a new hire's activations).
	mgrTopic := activation.TranscriptID(mgr.ID())
	topic, err := streaming.NewTopic(mgrTopic, "Activations: w-mgr", "test", mgr.ID(), time.Now().UTC(), transport.Transport{}, orgID)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}
	sub, err := streaming.NewSubscription("w-report", mgrTopic, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new subscription: %v", err)
	}
	if err := st.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	svc := &lifecycle.Service{Store: st, Reconciler: reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions})}
	if err := svc.Fire(ctx, orgID, mgr.ID()); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	// F8: no reporting line may reference the deleted manager.
	managers, err := st.ReportingLines.ListManagers(ctx, orgID, "w-report")
	if err != nil {
		t.Fatalf("list managers after fire: %v", err)
	}
	if len(managers) != 0 {
		t.Fatalf("w-report still reports to %v after firing its manager, want none (F8 dangling-line regression)", managers)
	}

	// F5: no subscription may reference the deleted transcript.
	subs, err := st.Subscriptions.ListForTopic(ctx, orgID, mgrTopic)
	if err != nil {
		t.Fatalf("list subscriptions for topic: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("found %d subscription(s) to deleted topic %q, want 0 (F5 orphan-subscription regression)", len(subs), mgrTopic)
	}
}

// TestFire_TearsDownDMChannelToReports pins the 2026-06-16 QA finding:
// firing a manager left the 1:1 DM channel (`s-dm-<mgr>-<report>`) it
// shared with each direct report lying around — the report stayed
// subscribed to a DM with a now-deleted worker. The reconciler's
// DM-channel teardown is an all-pairs-of-affected scan, so to settle
// `s-dm-<mgr>-<report>` BOTH endpoints have to be in the affected set;
// Fire only fed itself + its ex-managers, never its ex-reports. The fix
// adds the fired Worker's reports to the reconcile set. (The edge-removal
// path — removeWorkerParent — never had this bug because it reconciles
// both endpoints of the dropped edge.)
func TestFire_TearsDownDMChannelToReports(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-dm-fire"

	role, err := orgchart.NewRole("r-owner", "# Owner", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	mgr, err := orgchart.NewAIWorker("w-mgr", role.ID, "# Mgr", orgID)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := st.Workers.Create(ctx, mgr); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	report, err := orgchart.NewAIWorker("w-report", role.ID, "# Report", orgID)
	if err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := st.Workers.Create(ctx, report); err != nil {
		t.Fatalf("create report: %v", err)
	}
	line, err := orgchart.NewReportingLine(orgID, "w-mgr", "w-report")
	if err != nil {
		t.Fatalf("new reporting line: %v", err)
	}
	if err := st.ReportingLines.Add(ctx, line); err != nil {
		t.Fatalf("add reporting line: %v", err)
	}

	rec := reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions})
	// Provision the channels the edge implies (transcript observership,
	// team topic, and — the one under test — the 1:1 DM channel).
	if err := rec.Reconcile(ctx, orgID, "w-mgr", "w-report"); err != nil {
		t.Fatalf("reconcile (wire edge): %v", err)
	}
	dm := channels.DMTopicID("w-mgr", "w-report")
	if _, err := st.Topics.Get(ctx, orgID, dm); err != nil {
		t.Fatalf("precondition: DM channel %q should exist after wiring the edge: %v", dm, err)
	}

	svc := &lifecycle.Service{Store: st, Reconciler: rec}
	if err := svc.Fire(ctx, orgID, mgr.ID()); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	// The DM channel must be gone — not left orphaned referencing the
	// deleted manager.
	if _, err := st.Topics.Get(ctx, orgID, dm); err == nil {
		t.Fatalf("DM channel %q still exists after firing the manager — orphan regression", dm)
	}
	// And the surviving report must not still be subscribed to it.
	subs, err := st.Subscriptions.ListForTopic(ctx, orgID, dm)
	if err != nil {
		t.Fatalf("list subscriptions for DM topic: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("found %d subscription(s) to torn-down DM %q, want 0", len(subs), dm)
	}
}

// newLifecycleSvc builds a Service wired to a reconciler against the same
// store, with nil Helix runtime (the memory-store tests don't provision a
// Helix project/app, so the Fire cascade never calls into Helix).
func newLifecycleSvc(st *store.Store) *lifecycle.Service {
	return &lifecycle.Service{
		Store:      st,
		Reconciler: reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions}),
	}
}

func seedRole(t *testing.T, st *store.Store, orgID, id string) {
	t.Helper()
	r, err := orgchart.NewRole(id, "# "+id, nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new role %s: %v", id, err)
	}
	if err := st.Roles.Create(context.Background(), r); err != nil {
		t.Fatalf("create role %s: %v", id, err)
	}
}

func seedAIWorker(t *testing.T, st *store.Store, orgID, id, roleID string) {
	t.Helper()
	w, err := orgchart.NewAIWorker(id, roleID, "# "+id, orgID)
	if err != nil {
		t.Fatalf("new worker %s: %v", id, err)
	}
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker %s: %v", id, err)
	}
}

// TestDeleteRole_FiresMatchingWorkersOnly pins the core DeleteRole
// contract: it fires *every* Worker holding the doomed Role and deletes
// the Role row, while Workers in other Roles (and those Roles) are left
// completely untouched. Over-firing here would silently delete unrelated
// Workers.
func TestDeleteRole_FiresMatchingWorkersOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delrole"

	seedRole(t, st, orgID, "r-doomed")
	seedRole(t, st, orgID, "r-keep")
	seedAIWorker(t, st, orgID, "w-a", "r-doomed")
	seedAIWorker(t, st, orgID, "w-b", "r-doomed")
	seedAIWorker(t, st, orgID, "w-c", "r-keep")

	svc := newLifecycleSvc(st)
	if err := svc.DeleteRole(ctx, orgID, "r-doomed"); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}

	if _, err := st.Roles.Get(ctx, orgID, "r-doomed"); err == nil {
		t.Fatal("r-doomed should be deleted")
	}
	for _, id := range []string{"w-a", "w-b"} {
		if _, err := st.Workers.Get(ctx, orgID, id); err == nil {
			t.Fatalf("%s should have been fired by DeleteRole", id)
		}
	}
	// Bystanders survive — DeleteRole must not touch other Roles' Workers.
	if _, err := st.Workers.Get(ctx, orgID, "w-c"); err != nil {
		t.Fatalf("w-c (r-keep) should survive DeleteRole(r-doomed): %v", err)
	}
	if _, err := st.Roles.Get(ctx, orgID, "r-keep"); err != nil {
		t.Fatalf("r-keep should survive: %v", err)
	}
}

// TestDeleteRole_NonExistentRoleFiresNobody is the safety test. The
// not-found guard MUST short-circuit before fireWorkersWithRole runs —
// otherwise a typo'd / stale role id would cascade-fire any Worker that
// happens to reference that id. We plant a Worker whose RoleID points at a
// role with no row (an orphan), so the test bites if the guard regresses:
// without it, fireWorkersWithRole would match and fire the orphan.
func TestDeleteRole_NonExistentRoleFiresNobody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delrole-safety"

	// No r-ghost role row exists; w-zombie references it anyway.
	seedAIWorker(t, st, orgID, "w-zombie", "r-ghost")

	svc := newLifecycleSvc(st)
	if err := svc.DeleteRole(ctx, orgID, "r-ghost"); err == nil {
		t.Fatal("DeleteRole on a non-existent role must return an error")
	}
	if _, err := st.Workers.Get(ctx, orgID, "w-zombie"); err != nil {
		t.Fatalf("a failed DeleteRole must fire NOBODY — w-zombie was destroyed: %v", err)
	}
}

// TestDeleteRole_ReconcilesSurvivingCrossRoleReport covers the dangerous
// cascade: a Worker in the doomed Role *manages* a Worker in a different
// Role. Deleting the role fires the manager; the surviving report must not
// be left pointing at a deleted manager, and the comms channels that edge
// implied (team topic + 1:1 DM) must be torn down rather than orphaned.
// This is the ISSUE-2 teardown class reached via DeleteRole.
func TestDeleteRole_ReconcilesSurvivingCrossRoleReport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delrole-cascade"

	seedRole(t, st, orgID, "r-mgmt")
	seedRole(t, st, orgID, "r-ic")
	seedAIWorker(t, st, orgID, "w-mgr", "r-mgmt") // doomed role, root manager
	seedAIWorker(t, st, orgID, "w-ic", "r-ic")    // survivor, reports to w-mgr

	line, err := orgchart.NewReportingLine(orgID, "w-mgr", "w-ic")
	if err != nil {
		t.Fatalf("new reporting line: %v", err)
	}
	if err := st.ReportingLines.Add(ctx, line); err != nil {
		t.Fatalf("add reporting line: %v", err)
	}

	svc := newLifecycleSvc(st)
	// Provision the channels the edge implies (team topic + DM channel).
	if err := svc.Reconciler.Reconcile(ctx, orgID, "w-mgr", "w-ic"); err != nil {
		t.Fatalf("reconcile (wire edge): %v", err)
	}
	dm := channels.DMTopicID("w-mgr", "w-ic")
	if _, err := st.Topics.Get(ctx, orgID, dm); err != nil {
		t.Fatalf("precondition: DM channel %q should exist: %v", dm, err)
	}
	if _, err := st.Topics.Get(ctx, orgID, "s-team-w-mgr"); err != nil {
		t.Fatalf("precondition: team topic s-team-w-mgr should exist: %v", err)
	}

	if err := svc.DeleteRole(ctx, orgID, "r-mgmt"); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}

	// Manager fired, report survives.
	if _, err := st.Workers.Get(ctx, orgID, "w-mgr"); err == nil {
		t.Fatal("w-mgr should be fired by DeleteRole(r-mgmt)")
	}
	if _, err := st.Workers.Get(ctx, orgID, "w-ic"); err != nil {
		t.Fatalf("w-ic (r-ic) should survive: %v", err)
	}
	// The surviving report no longer points at the deleted manager.
	mgrs, err := st.ReportingLines.ListManagers(ctx, orgID, "w-ic")
	if err != nil {
		t.Fatalf("list managers: %v", err)
	}
	if len(mgrs) != 0 {
		t.Fatalf("w-ic still reports to %v after its manager's role was deleted", mgrs)
	}
	// No orphaned comms channels referencing the deleted manager.
	if _, err := st.Topics.Get(ctx, orgID, dm); err == nil {
		t.Fatalf("DM channel %q orphaned after DeleteRole", dm)
	}
	if _, err := st.Topics.Get(ctx, orgID, "s-team-w-mgr"); err == nil {
		t.Fatal("team topic s-team-w-mgr orphaned after DeleteRole")
	}
}

// TestDeleteRole_Guards pins the cheap input guards.
func TestDeleteRole_Guards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)

	if err := (&lifecycle.Service{Store: st}).DeleteRole(ctx, "org", ""); err == nil {
		t.Fatal("empty role id should error")
	}
	if err := (&lifecycle.Service{}).DeleteRole(ctx, "org", "r-x"); err == nil {
		t.Fatal("nil store should error")
	}
}

// TestFire_MissingWorkerErrorsWithNoSideEffects pins that firing a worker
// that doesn't exist errors at the get-guard and leaves the graph alone —
// no bystander is swept up by a no-op fire.
func TestFire_MissingWorkerErrorsWithNoSideEffects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-fire-missing"

	seedRole(t, st, orgID, "r-x")
	seedAIWorker(t, st, orgID, "w-bystander", "r-x")

	svc := newLifecycleSvc(st)
	if err := svc.Fire(ctx, orgID, "w-missing"); err == nil {
		t.Fatal("Fire on a non-existent worker should error")
	}
	if _, err := st.Workers.Get(ctx, orgID, "w-bystander"); err != nil {
		t.Fatalf("bystander must be untouched by a failed Fire: %v", err)
	}
}
