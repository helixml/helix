package reconcile

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
	dm := channels.DMTopicID("w-jane", "w-li")
	if !topicExists(t, st, dm) {
		t.Fatalf("DM channel %q should exist after wiring the edge", dm)
	}
	if got := topicMembers(t, st, dm); !eq(got, []orgchart.WorkerID{"w-jane", "w-li"}) {
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
	dm := channels.DMTopicID("w-jane", "w-li")
	if !topicExists(t, st, dm) {
		t.Fatalf("precondition: DM channel should exist")
	}

	if err := st.ReportingLines.Remove(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("remove line: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile remove: %v", err)
	}
	if topicExists(t, st, dm) {
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
	dm := channels.DMTopicID("w-jane", "w-li")

	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err := st.Workers.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	affected := append([]orgchart.WorkerID{"w-li"}, managers...)
	if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}
	if topicExists(t, st, dm) {
		t.Fatalf("DM channel %q should be gone after firing the report", dm)
	}
}

// TestReconcile_LeavesForeignTopicsUntouched is the load-bearing
// safety assertion for the scoping comment: Reconcile only ever touches
// the activation / team / DM ids of the affected Workers and their
// one-hop neighbours — never an operator-created topic, even one whose
// members overlap the affected set.
func TestReconcile_LeavesForeignTopicsUntouched(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	seedWorker(t, st, ai("w-outsider"))

	// An operator-created topic with its own membership — nothing to do
	// with the reporting graph.
	foreign := streaming.TopicID("s-general")
	fs, err := streaming.NewTopic(foreign, "general", "", "w-jane", fixedNow(), transport.Transport{}, orgID)
	if err != nil {
		t.Fatalf("new foreign topic: %v", err)
	}
	if err := rec.ensureTopicWithMembers(ctx, fs, fixedNow(), "w-jane", "w-li", "w-outsider"); err != nil {
		t.Fatalf("seed foreign topic: %v", err)
	}

	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// The foreign topic still exists with its full, unmodified
	// membership — Reconcile never considered it.
	if !topicExists(t, st, foreign) {
		t.Fatalf("operator topic %q must survive reconcile", foreign)
	}
	if got := topicMembers(t, st, foreign); !eq(got, []orgchart.WorkerID{"w-jane", "w-li", "w-outsider"}) {
		t.Fatalf("foreign topic members = %v, want untouched [w-jane w-li w-outsider]", got)
	}
}

// TestReconcileAll_CatchesUpMissingTeamTopic simulates the case where
// Workers were hired before the topology reconciler was wired: the
// reporting lines and Workers exist in the store but no team topic was
// ever created. ReconcileAll must converge all Topics idempotently,
// including the team topic for a manager who already has direct
// reports.
func TestReconcileAll_CatchesUpMissingTeamTopic(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()

	// Seed the org graph directly (bypassing hire_worker) to simulate
	// Workers hired before the reconciler was wired — no topics exist.
	seedWorker(t, st, human("w-owner"))
	seedWorker(t, st, ai("w-alice"))
	seedWorker(t, st, ai("w-qa-1"))
	addLine(t, st, "w-owner", "w-alice")
	addLine(t, st, "w-owner", "w-qa-1")

	// ReconcileAll must create the team topic and subscribe all members.
	if err := rec.ReconcileAll(ctx, orgID); err != nil {
		t.Fatalf("ReconcileAll: %v", err)
	}

	team := channels.TeamTopicID("w-owner")
	if !topicExists(t, st, team) {
		t.Fatalf("s-team-w-owner should exist after ReconcileAll")
	}
	if got := topicMembers(t, st, team); !eq(got, []orgchart.WorkerID{"w-alice", "w-owner", "w-qa-1"}) {
		t.Fatalf("s-team-w-owner members = %v, want [w-alice w-owner w-qa-1]", got)
	}
}

// TestReconcileAll_ScopedToAffectedSubtree: reconciling one manager's
// subtree leaves an unrelated manager's team topic untouched.
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
	before := topicMembers(t, st, channels.TeamTopicID("w-bob"))

	// Now mutate jane's subtree only (fire li) and reconcile just it.
	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err := st.Workers.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, append([]orgchart.WorkerID{"w-li"}, managers...)...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}

	// bob's team topic is untouched.
	after := topicMembers(t, st, channels.TeamTopicID("w-bob"))
	if !eq(before, after) {
		t.Fatalf("unrelated subtree disturbed: s-team-w-bob before=%v after=%v", before, after)
	}
}
