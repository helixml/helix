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
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// TestFire_RemovesWorkersTranscript pins the regression behind
// "we still see s-transcript-w-ai-1 and s-transcript-w-test-ai
// even though those workers are gone": the Fire cascade tore down
// subscriptions, environment, runtime state, and the worker
// row — but left the per-Worker transcript
// (s-transcript-<workerID>) lying around, so the Streams page kept
// rendering ghost rows for workers that no longer existed and the
// chart's orphan strip filled up with dashed pseudo-nodes.
//
// Activation events themselves are still audit-retained (the
// `org_events` rows survive); only the Stream row is cleaned up so
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
	streamID := activation.TranscriptID(worker.ID())
	stream, err := streaming.NewStream(
		streamID, "Activations: w-ghost", "test",
		worker.ID(), time.Now().UTC(),
		transport.Transport{}, orgID,
	)
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}

	// Sanity: the stream is there before we fire.
	if _, err := st.Streams.Get(ctx, orgID, streamID); err != nil {
		t.Fatalf("precondition: transcript not seeded: %v", err)
	}

	svc := &lifecycle.Service{Store: st, Reconciler: reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions})}
	if err := svc.Fire(ctx, orgID, worker.ID()); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	// The stream row must be gone.
	if _, err := st.Streams.Get(ctx, orgID, streamID); err == nil {
		t.Fatalf("transcript %q still exists after Fire — orphan regression", streamID)
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
//   - F5: firing a worker deleted its s-transcript-<id> stream but
//     left OTHER workers' subscriptions to that stream behind.
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
	mgrStream := activation.TranscriptID(mgr.ID())
	stream, err := streaming.NewStream(mgrStream, "Activations: w-mgr", "test", mgr.ID(), time.Now().UTC(), transport.Transport{}, orgID)
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	sub, err := streaming.NewSubscription("w-report", mgrStream, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new subscription: %v", err)
	}
	if err := st.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	svc := &lifecycle.Service{Store: st, Reconciler: reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions})}
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
	subs, err := st.Subscriptions.ListForStream(ctx, orgID, mgrStream)
	if err != nil {
		t.Fatalf("list subscriptions for stream: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("found %d subscription(s) to deleted stream %q, want 0 (F5 orphan-subscription regression)", len(subs), mgrStream)
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

	rec := reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions})
	// Provision the channels the edge implies (transcript observership,
	// team stream, and — the one under test — the 1:1 DM channel).
	if err := rec.Reconcile(ctx, orgID, "w-mgr", "w-report"); err != nil {
		t.Fatalf("reconcile (wire edge): %v", err)
	}
	dm := channels.DMStreamID("w-mgr", "w-report")
	if _, err := st.Streams.Get(ctx, orgID, dm); err != nil {
		t.Fatalf("precondition: DM channel %q should exist after wiring the edge: %v", dm, err)
	}

	svc := &lifecycle.Service{Store: st, Reconciler: rec}
	if err := svc.Fire(ctx, orgID, mgr.ID()); err != nil {
		t.Fatalf("Fire: %v", err)
	}

	// The DM channel must be gone — not left orphaned referencing the
	// deleted manager.
	if _, err := st.Streams.Get(ctx, orgID, dm); err == nil {
		t.Fatalf("DM channel %q still exists after firing the manager — orphan regression", dm)
	}
	// And the surviving report must not still be subscribed to it.
	subs, err := st.Subscriptions.ListForStream(ctx, orgID, dm)
	if err != nil {
		t.Fatalf("list subscriptions for DM stream: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("found %d subscription(s) to torn-down DM %q, want 0", len(subs), dm)
	}
}
