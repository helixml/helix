package reconcile

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	memorystore "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

const orgID = "org-test"

func fixedNow() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

// The pure Required / channel-id derivation tests live with the domain
// function in domain/channels. The tests below exercise the
// application-layer Reconciler against the memory store.

// bot builds a Bot for tests. The former Role/Worker (AI vs human)
// distinction is gone: there is one Bot aggregate, so every test fixture
// is just a Bot with non-empty content.
func bot(id orgchart.NodeID) orgchart.Node {
	b, err := orgchart.NewNode(id, "#", nil, fixedNow(), orgID)
	if err != nil {
		panic(err)
	}
	return b
}

func line(manager, report orgchart.NodeID) orgchart.ReportingLine {
	l, err := orgchart.NewReportingLine(orgID, manager, report)
	if err != nil {
		panic(err)
	}
	return l
}

func eq(a, b []orgchart.NodeID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---- Reconcile against the memory store --------------------------------

func newRec(t *testing.T) (*Reconciler, *store.Store) {
	t.Helper()
	st := memorystore.New()
	return New(Deps{Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers, Attachments: st.WorkerAttachments, Now: fixedNow}), st
}

func seedBot(t *testing.T, st *store.Store, b orgchart.Node) {
	t.Helper()
	if err := st.Nodes.Create(context.Background(), b); err != nil {
		t.Fatalf("create bot %s: %v", b.ID, err)
	}
}

func addLine(t *testing.T, st *store.Store, manager, report orgchart.NodeID) {
	t.Helper()
	if err := st.ReportingLines.Add(context.Background(), line(manager, report)); err != nil {
		t.Fatalf("add line %s->%s: %v", manager, report, err)
	}
}

func channelMembers(t *testing.T, st *store.Store, id string) []orgchart.NodeID {
	t.Helper()
	rows, err := st.WorkerAttachments.Find(context.Background(), store.WithOrg(orgID), store.WithTriggerID(id))
	if err != nil {
		t.Fatalf("list members of %s: %v", id, err)
	}
	out := make([]orgchart.NodeID, 0, len(rows))
	for _, a := range rows {
		out = append(out, a.WorkerID)
	}
	sort.Strings(out)
	return out
}

func channelExists(t *testing.T, st *store.Store, id string) bool {
	t.Helper()
	rows, err := st.Triggers.Find(context.Background(), store.WithOrg(orgID), store.WithID(id), store.WithLimit(1))
	return err == nil && len(rows) > 0
}

// TestReconcile_HireFirstAndSecondReport mirrors the first two TDD rows:
// hiring the first report creates the manager's team topic; hiring a
// second adds them.
func TestReconcile_HireFirstAndSecondReport(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))

	// First report.
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := channelMembers(t, st, channels.TeamTriggerID("w-jane")); !eq(got, []orgchart.NodeID{"w-jane", "w-li"}) {
		t.Fatalf("after first hire, s-team-w-jane = %v, want [w-jane w-li]", got)
	}
	// w-li's transcript is observed by jane.
	if got := channelMembers(t, st, activation.TranscriptID("w-li")); !eq(got, []orgchart.NodeID{"w-jane"}) {
		t.Fatalf("s-transcript-w-li = %v, want [w-jane]", got)
	}

	// Second report.
	seedBot(t, st, bot("w-sam"))
	addLine(t, st, "w-jane", "w-sam")
	if err := rec.Reconcile(ctx, orgID, "w-sam"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := channelMembers(t, st, channels.TeamTriggerID("w-jane")); !eq(got, []orgchart.NodeID{"w-jane", "w-li", "w-sam"}) {
		t.Fatalf("after second hire, s-team-w-jane = %v, want [w-jane w-li w-sam]", got)
	}
}

// TestReconcile_AddSecondManager mirrors the addWorkerParent TDD row:
// adding a second manager adds the report to the new team topic and
// the new manager to the report's transcript, WITHOUT disturbing
// the first manager's membership (the many-to-many invariant).
func TestReconcile_AddSecondManager(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-bob"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile initial: %v", err)
	}

	// Now w-li ALSO reports to w-bob.
	addLine(t, st, "w-bob", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-bob"); err != nil {
		t.Fatalf("reconcile add manager: %v", err)
	}

	if got := channelMembers(t, st, channels.TeamTriggerID("w-bob")); !eq(got, []orgchart.NodeID{"w-bob", "w-li"}) {
		t.Fatalf("s-team-w-bob = %v, want [w-bob w-li]", got)
	}
	if got := channelMembers(t, st, channels.TeamTriggerID("w-jane")); !eq(got, []orgchart.NodeID{"w-jane", "w-li"}) {
		t.Fatalf("s-team-w-jane (unchanged) = %v, want [w-jane w-li]", got)
	}
	if got := channelMembers(t, st, activation.TranscriptID("w-li")); !eq(got, []orgchart.NodeID{"w-bob", "w-jane"}) {
		t.Fatalf("s-transcript-w-li = %v, want [w-bob w-jane]", got)
	}
}

// TestReconcile_RemoveManager mirrors the removeWorkerParent TDD row:
// dropping w-li→w-jane removes w-li from s-team-w-jane, tears the topic
// down if jane now has no reports, and unsubscribes jane from
// s-transcript-w-li.
func TestReconcile_RemoveManager(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile initial: %v", err)
	}
	if !channelExists(t, st, channels.TeamTriggerID("w-jane")) {
		t.Fatalf("precondition: s-team-w-jane should exist")
	}

	// Drop the line, then reconcile both endpoints.
	if err := st.ReportingLines.Remove(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("remove line: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile remove: %v", err)
	}

	if channelExists(t, st, channels.TeamTriggerID("w-jane")) {
		t.Fatalf("s-team-w-jane should be torn down (jane has 0 reports)")
	}
	// jane no longer observes w-li's transcript.
	if got := channelMembers(t, st, activation.TranscriptID("w-li")); len(got) != 0 {
		t.Fatalf("s-transcript-w-li observers = %v, want none after losing its only manager", got)
	}
}

// TestReconcile_RemoveManager_KeepsTopicWhenOtherReports: dropping one
// report from a manager that still has others keeps the team topic and
// only removes the departing report.
func TestReconcile_RemoveManager_KeepsTopicWhenOtherReports(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	seedBot(t, st, bot("w-sam"))
	addLine(t, st, "w-jane", "w-li")
	addLine(t, st, "w-jane", "w-sam")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-sam"); err != nil {
		t.Fatalf("reconcile initial: %v", err)
	}

	if err := st.ReportingLines.Remove(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("remove line: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile remove: %v", err)
	}

	if got := channelMembers(t, st, channels.TeamTriggerID("w-jane")); !eq(got, []orgchart.NodeID{"w-jane", "w-sam"}) {
		t.Fatalf("s-team-w-jane = %v, want [w-jane w-sam]", got)
	}
}

// TestReconcile_FireReport mirrors the fire-a-report TDD row: after the
// worker row + its lines are gone, Reconcile(firedID, exManagers…) tears
// down the now-empty team topic.
func TestReconcile_FireReport(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile initial: %v", err)
	}

	// Capture managers BEFORE deletion (lines cascade with the row).
	managers, err := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err != nil {
		t.Fatalf("list managers: %v", err)
	}
	if err := st.Nodes.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	affected := append([]orgchart.NodeID{"w-li"}, managers...)
	if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}

	if channelExists(t, st, channels.TeamTriggerID("w-jane")) {
		t.Fatalf("s-team-w-jane should be gone after firing its only report")
	}
	if channelExists(t, st, activation.TranscriptID("w-li")) {
		t.Fatalf("s-transcript-w-li should be gone after firing w-li")
	}
}

// TestReconcile_FireManager mirrors the fire-a-manager TDD row: firing
// the manager removes the team topic; the reports keep their own
// subtrees.
func TestReconcile_FireManager(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-owner"))
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-owner", "w-jane")
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-jane"); err != nil {
		t.Fatalf("reconcile jane: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile li: %v", err)
	}
	if !channelExists(t, st, channels.TeamTriggerID("w-jane")) {
		t.Fatalf("precondition: s-team-w-jane should exist")
	}

	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-jane")
	if err := st.Nodes.Delete(ctx, orgID, "w-jane"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	affected := append([]orgchart.NodeID{"w-jane"}, managers...)
	if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}

	if channelExists(t, st, channels.TeamTriggerID("w-jane")) {
		t.Fatalf("s-team-w-jane should be torn down")
	}
	// w-li still exists and keeps its own transcript.
	if !channelExists(t, st, activation.TranscriptID("w-li")) {
		t.Fatalf("w-li should keep its own transcript")
	}
}

// TestReconcile_Idempotent: a second Reconcile with no graph change is a
// no-op — same topics, same members.
func TestReconcile_Idempotent(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-jane"))
	seedBot(t, st, bot("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	before := channelMembers(t, st, channels.TeamTriggerID("w-jane"))

	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	after := channelMembers(t, st, channels.TeamTriggerID("w-jane"))
	if !eq(before, after) {
		t.Fatalf("idempotency broken: before=%v after=%v", before, after)
	}
}

// TestConvergeChannel_ConcurrentRace proves the reconciler is safe
// against two callers racing on the same deterministic channel id — the
// TOCTOU the topology design must tolerate (two simultaneous DMs between
// the same pair, two reconciles touching one team chat). Many goroutines
// released from a barrier all reconcile the SAME graph; the memory store
// runs Find and Create under separate locks, so the loser of each race
// takes the re-check-after-Create path. Every call must succeed, and the
// end state must be exactly one channel with exactly one member.
func TestConvergeChannel_ConcurrentRace(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-race"))
	seedBot(t, st, bot("w-member"))
	addLine(t, st, "w-race", "w-member")

	const n = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // release all at once to maximise interleaving
			errs[idx] = rec.Reconcile(ctx, orgID, "w-member", "w-race")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("goroutine %d errored on concurrent reconcile: %v", i, e)
		}
	}
	// Exactly one team chat with exactly its two members — no
	// duplicates, no spurious unique-constraint failures.
	if got := channelMembers(t, st, channels.TeamTriggerID("w-race")); !eq(got, []orgchart.NodeID{"w-member", "w-race"}) {
		t.Fatalf("members = %v, want exactly [w-member w-race]", got)
	}
}

// TestReconcile_RootWithReport: a manager-less root worker with a direct
// report has a transcript (unobserved — never self-attached) and a team
// chat containing the root + report.
func TestReconcile_RootWithReport(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedBot(t, st, bot("w-root"))
	// The root's first reconcile mints its (unobserved) transcript.
	if err := rec.Reconcile(ctx, orgID, "w-root"); err != nil {
		t.Fatalf("reconcile root: %v", err)
	}
	if got := channelMembers(t, st, activation.TranscriptID("w-root")); len(got) != 0 {
		t.Fatalf("root transcript observers = %v, want none (never self-attached)", got)
	}

	seedBot(t, st, bot("w-jane"))
	addLine(t, st, "w-root", "w-jane")
	if err := rec.Reconcile(ctx, orgID, "w-jane"); err != nil {
		t.Fatalf("reconcile jane: %v", err)
	}
	if got := channelMembers(t, st, channels.TeamTriggerID("w-root")); !eq(got, []orgchart.NodeID{"w-jane", "w-root"}) {
		t.Fatalf("s-team-w-root = %v, want [w-jane w-root]", got)
	}
	// The root's own transcript survived the team-chat creation.
	if !channelExists(t, st, activation.TranscriptID("w-root")) {
		t.Fatalf("root transcript should still exist after report")
	}
}
