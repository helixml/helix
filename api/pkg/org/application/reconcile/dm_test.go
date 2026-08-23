package reconcile

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/internal/orgtest"
)

// TestReconcile_DMChannelCreatedPerEdge: wiring a reporting edge
// provisions the 1:1 DM channel with exactly the two endpoints — the
// channel the `dm` tool then assumes exists.
func TestReconcile_DMChannelCreatedPerEdge(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	dm := channels.DMTriggerID("w-jane", "w-li")
	if !channelExists(t, st, dm) {
		t.Fatalf("DM channel %q should exist after wiring the edge", dm)
	}
	if got := channelMembers(t, st, dm); !eq(got, []orgchart.NodeID{"w-jane", "w-li"}) {
		t.Fatalf("DM members = %v, want [w-jane w-li]", got)
	}
}

// TestReconcile_DMChannelTornDownOnEdgeRemoval: removing the reporting
// edge tears the DM channel down (the all-pairs-of-affected scoping is
// what reaches it once the two are no longer neighbours).
func TestReconcile_DMChannelTornDownOnEdgeRemoval(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile add: %v", err)
	}
	dm := channels.DMTriggerID("w-jane", "w-li")
	if !channelExists(t, st, dm) {
		t.Fatalf("precondition: DM channel should exist")
	}

	if err := st.ReportingLines.Remove(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("remove line: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile remove: %v", err)
	}
	if channelExists(t, st, dm) {
		t.Fatalf("DM channel %q should be torn down when the reporting edge is removed", dm)
	}
}

// TestReconcile_DMChannelTornDownOnFire: firing a report tears down its
// DM channel with the ex-manager (passed in affected so the all-pairs
// scoping reaches it after the lines cascade away).
func TestReconcile_DMChannelTornDownOnFire(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	dm := channels.DMTriggerID("w-jane", "w-li")

	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err := st.Nodes.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	affected := append([]orgchart.NodeID{"w-li"}, managers...)
	if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}
	if channelExists(t, st, dm) {
		t.Fatalf("DM channel %q should be gone after firing the report", dm)
	}
}

// TestReconcile_LeavesForeignTriggersUntouched is the load-bearing
// safety assertion for the scoping comment: Reconcile only ever touches
// the transcript / team / DM ids of the affected Workers and their
// one-hop neighbours — never an operator-created Trigger, even one whose
// members overlap the affected set.
func TestReconcile_LeavesForeignTriggersUntouched(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	seedBot(t, st, bot("w-outsider"))

	// An operator-created Trigger with its own membership — nothing to
	// do with the reporting graph.
	const foreign = "s-general"
	orgtest.Trigger(t, st, orgID, foreign)
	for _, w := range []orgchart.NodeID{"w-jane", "w-li", "w-outsider"} {
		orgtest.AttachTrigger(t, st, orgID, w, foreign)
	}

	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// The foreign Trigger still exists with its full, unmodified
	// membership — Reconcile never considered it.
	if !channelExists(t, st, foreign) {
		t.Fatalf("operator trigger %q must survive reconcile", foreign)
	}
	if got := channelMembers(t, st, foreign); !eq(got, []orgchart.NodeID{"w-jane", "w-li", "w-outsider"}) {
		t.Fatalf("foreign trigger members = %v, want untouched [w-jane w-li w-outsider]", got)
	}
}

// TestReconcileAll_CatchesUpMissingTeamChat simulates the case where
// Workers were hired before the topology reconciler was wired: the
// reporting lines and Workers exist in the store but no team topic was
// ever created. ReconcileAll must converge all Topics idempotently,
// including the team topic for a manager who already has direct
// reports.
func TestReconcileAll_CatchesUpMissingTeamChat(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()

	// Seed the org graph directly (bypassing hire_worker) to simulate
	// Workers hired before the reconciler was wired — no topics exist.
	seedBot(t, st, bot("w-owner"))
	seedBot(t, st, bot("w-alice"))
	seedBot(t, st, bot("w-qa-1"))
	addLine(t, st, "w-owner", "w-alice")
	addLine(t, st, "w-owner", "w-qa-1")

	// ReconcileAll must create the team topic and subscribe all members.
	if err := rec.ReconcileAll(ctx, orgID); err != nil {
		t.Fatalf("ReconcileAll: %v", err)
	}

	team := channels.TeamTriggerID("w-owner")
	if !channelExists(t, st, team) {
		t.Fatalf("s-team-w-owner should exist after ReconcileAll")
	}
	if got := channelMembers(t, st, team); !eq(got, []orgchart.NodeID{"w-alice", "w-owner", "w-qa-1"}) {
		t.Fatalf("s-team-w-owner members = %v, want [w-alice w-owner w-qa-1]", got)
	}
}

// TestReconcile_ScopedToAffectedSubtree: reconciling one manager's
// subtree leaves an unrelated manager's team chat untouched.
func TestReconcile_ScopedToAffectedSubtree(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	// Two independent subtrees: jane→li and bob→sam.
	for _, id := range []orgchart.NodeID{"w-jane", "w-bob"} {
		seedBot(t, st, bot(id))
	}
	for _, id := range []orgchart.NodeID{"w-li", "w-sam"} {
		seedBot(t, st, bot(id))
	}
	addLine(t, st, "w-jane", "w-li")
	addLine(t, st, "w-bob", "w-sam")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile jane subtree: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-sam", "w-bob"); err != nil {
		t.Fatalf("reconcile bob subtree: %v", err)
	}
	before := channelMembers(t, st, channels.TeamTriggerID("w-bob"))

	// Now mutate jane's subtree only (fire li) and reconcile just it.
	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err := st.Nodes.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, append([]orgchart.NodeID{"w-li"}, managers...)...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}

	// bob's team topic is untouched.
	after := channelMembers(t, st, channels.TeamTriggerID("w-bob"))
	if !eq(before, after) {
		t.Fatalf("unrelated subtree disturbed: s-team-w-bob before=%v after=%v", before, after)
	}
}
