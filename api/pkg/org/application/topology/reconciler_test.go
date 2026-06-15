package topology

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestReconcile_DMChannelCreatedPerEdge: wiring a reporting edge
// provisions the 1:1 DM channel with exactly the two endpoints — the
// channel the `dm` tool then assumes exists.
func TestReconcile_DMChannelCreatedPerEdge(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	dm := channels.DMStreamID("w-jane", "w-li")
	if !streamExists(t, st, dm) {
		t.Fatalf("DM channel %q should exist after wiring the edge", dm)
	}
	if got := streamMembers(t, st, dm); !eq(got, []orgchart.WorkerID{"w-jane", "w-li"}) {
		t.Fatalf("DM members = %v, want [w-jane w-li]", got)
	}
}

// TestReconcile_DMChannelTornDownOnEdgeRemoval: removing the reporting
// edge tears the DM channel down (the all-pairs-of-affected scoping is
// what reaches it once the two are no longer neighbours).
func TestReconcile_DMChannelTornDownOnEdgeRemoval(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile add: %v", err)
	}
	dm := channels.DMStreamID("w-jane", "w-li")
	if !streamExists(t, st, dm) {
		t.Fatalf("precondition: DM channel should exist")
	}

	if err := st.ReportingLines.Remove(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("remove line: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile remove: %v", err)
	}
	if streamExists(t, st, dm) {
		t.Fatalf("DM channel %q should be torn down when the reporting edge is removed", dm)
	}
}

// TestReconcile_DMChannelTornDownOnFire: firing a report tears down its
// DM channel with the ex-manager (passed in affected so the all-pairs
// scoping reaches it after the lines cascade away).
func TestReconcile_DMChannelTornDownOnFire(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	dm := channels.DMStreamID("w-jane", "w-li")

	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err := st.Workers.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	affected := append([]orgchart.WorkerID{"w-li"}, managers...)
	if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}
	if streamExists(t, st, dm) {
		t.Fatalf("DM channel %q should be gone after firing the report", dm)
	}
}

// TestReconcile_LeavesForeignStreamsUntouched is the load-bearing
// safety assertion for the scoping comment: Reconcile only ever touches
// the activation / team / DM ids of the affected Workers and their
// one-hop neighbours — never an operator-created stream, even one whose
// members overlap the affected set.
func TestReconcile_LeavesForeignStreamsUntouched(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	seedWorker(t, st, ai("w-outsider"))

	// An operator-created stream with its own membership — nothing to do
	// with the reporting graph.
	foreign := streaming.StreamID("s-general")
	fs, err := streaming.NewStream(foreign, "general", "", "w-jane", fixedNow(), transport.Transport{}, orgID)
	if err != nil {
		t.Fatalf("new foreign stream: %v", err)
	}
	if err := rec.ensureStreamWithMembers(ctx, fs, fixedNow(), "w-jane", "w-li", "w-outsider"); err != nil {
		t.Fatalf("seed foreign stream: %v", err)
	}

	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// The foreign stream still exists with its full, unmodified
	// membership — Reconcile never considered it.
	if !streamExists(t, st, foreign) {
		t.Fatalf("operator stream %q must survive reconcile", foreign)
	}
	if got := streamMembers(t, st, foreign); !eq(got, []orgchart.WorkerID{"w-jane", "w-li", "w-outsider"}) {
		t.Fatalf("foreign stream members = %v, want untouched [w-jane w-li w-outsider]", got)
	}
}

// TestReconcileAll_CatchesUpMissingTeamStream simulates the case where
// Workers were hired before the topology reconciler was wired: the
// reporting lines and Workers exist in the store but no team stream was
// ever created. ReconcileAll must converge all Streams idempotently,
// including the team stream for a manager who already has direct
// reports.
func TestReconcileAll_CatchesUpMissingTeamStream(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()

	// Seed the org graph directly (bypassing hire_worker) to simulate
	// Workers hired before the reconciler was wired — no streams exist.
	seedWorker(t, st, human("w-owner"))
	seedWorker(t, st, ai("w-alice"))
	seedWorker(t, st, ai("w-qa-1"))
	addLine(t, st, "w-owner", "w-alice")
	addLine(t, st, "w-owner", "w-qa-1")

	// ReconcileAll must create the team stream and subscribe all members.
	if err := rec.ReconcileAll(ctx, orgID); err != nil {
		t.Fatalf("ReconcileAll: %v", err)
	}

	team := channels.TeamStreamID("w-owner")
	if !streamExists(t, st, team) {
		t.Fatalf("s-team-w-owner should exist after ReconcileAll")
	}
	if got := streamMembers(t, st, team); !eq(got, []orgchart.WorkerID{"w-alice", "w-owner", "w-qa-1"}) {
		t.Fatalf("s-team-w-owner members = %v, want [w-alice w-owner w-qa-1]", got)
	}
}

// TestReconcileAll_ScopedToAffectedSubtree: reconciling one manager's
// subtree leaves an unrelated manager's team stream untouched.
func TestReconcile_ScopedToAffectedSubtree(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	// Two independent subtrees: jane→li and bob→sam.
	for _, id := range []orgchart.WorkerID{"w-jane", "w-bob"} {
		seedWorker(t, st, human(id))
	}
	for _, id := range []orgchart.WorkerID{"w-li", "w-sam"} {
		seedWorker(t, st, ai(id))
	}
	addLine(t, st, "w-jane", "w-li")
	addLine(t, st, "w-bob", "w-sam")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile jane subtree: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-sam", "w-bob"); err != nil {
		t.Fatalf("reconcile bob subtree: %v", err)
	}
	before := streamMembers(t, st, channels.TeamStreamID("w-bob"))

	// Now mutate jane's subtree only (fire li) and reconcile just it.
	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err := st.Workers.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, append([]orgchart.WorkerID{"w-li"}, managers...)...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}

	// bob's team stream is untouched.
	after := streamMembers(t, st, channels.TeamStreamID("w-bob"))
	if !eq(before, after) {
		t.Fatalf("unrelated subtree disturbed: s-team-w-bob before=%v after=%v", before, after)
	}
}
