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
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	memorystore "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

const orgID = "org-test"

func fixedNow() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

// The pure Required / stream-id derivation tests live with the domain
// function in domain/channels. The tests below exercise the
// application-layer Reconciler and its ensureStreamWithMembers primitive
// against the memory store.

func ai(id orgchart.WorkerID) orgchart.Worker {
	w, err := orgchart.NewAIWorker(id, "r-x", "#", orgID)
	if err != nil {
		panic(err)
	}
	return w
}

func human(id orgchart.WorkerID) orgchart.Worker {
	w, err := orgchart.NewHumanWorker(id, "r-x", "#", orgID)
	if err != nil {
		panic(err)
	}
	return w
}

func line(manager, report orgchart.WorkerID) orgchart.ReportingLine {
	l, err := orgchart.NewReportingLine(orgID, manager, report)
	if err != nil {
		panic(err)
	}
	return l
}

func eq(a, b []orgchart.WorkerID) bool {
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
	return New(Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions, Now: fixedNow}), st
}

func seedWorker(t *testing.T, st *store.Store, w orgchart.Worker) {
	t.Helper()
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker %s: %v", w.ID(), err)
	}
}

func addLine(t *testing.T, st *store.Store, manager, report orgchart.WorkerID) {
	t.Helper()
	if err := st.ReportingLines.Add(context.Background(), line(manager, report)); err != nil {
		t.Fatalf("add line %s->%s: %v", manager, report, err)
	}
}

func streamMembers(t *testing.T, st *store.Store, sid streaming.StreamID) []orgchart.WorkerID {
	t.Helper()
	subs, err := st.Subscriptions.ListForStream(context.Background(), orgID, sid)
	if err != nil {
		t.Fatalf("list subscribers of %s: %v", sid, err)
	}
	out := make([]orgchart.WorkerID, 0, len(subs))
	for _, s := range subs {
		out = append(out, orgchart.WorkerID(s.WorkerID))
	}
	sort.Strings(out)
	return out
}

func streamExists(t *testing.T, st *store.Store, sid streaming.StreamID) bool {
	t.Helper()
	_, err := st.Streams.Get(context.Background(), orgID, sid)
	return err == nil
}

// TestReconcile_HireFirstAndSecondReport mirrors the first two TDD rows:
// hiring the first report creates the manager's team stream; hiring a
// second adds them.
func TestReconcile_HireFirstAndSecondReport(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))

	// First report.
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := streamMembers(t, st, channels.TeamStreamID("w-jane")); !eq(got, []orgchart.WorkerID{"w-jane", "w-li"}) {
		t.Fatalf("after first hire, s-team-w-jane = %v, want [w-jane w-li]", got)
	}
	// w-li's activation stream is observed by jane.
	if got := streamMembers(t, st, activation.StreamID("w-li")); !eq(got, []orgchart.WorkerID{"w-jane"}) {
		t.Fatalf("s-activations-w-li = %v, want [w-jane]", got)
	}

	// Second report.
	seedWorker(t, st, ai("w-sam"))
	addLine(t, st, "w-jane", "w-sam")
	if err := rec.Reconcile(ctx, orgID, "w-sam"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := streamMembers(t, st, channels.TeamStreamID("w-jane")); !eq(got, []orgchart.WorkerID{"w-jane", "w-li", "w-sam"}) {
		t.Fatalf("after second hire, s-team-w-jane = %v, want [w-jane w-li w-sam]", got)
	}
}

// TestReconcile_AddSecondManager mirrors the addWorkerParent TDD row:
// adding a second manager adds the report to the new team stream and
// the new manager to the report's activation stream, WITHOUT disturbing
// the first manager's membership (the many-to-many invariant).
func TestReconcile_AddSecondManager(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, human("w-bob"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile initial: %v", err)
	}

	// Now w-li ALSO reports to w-bob.
	addLine(t, st, "w-bob", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-bob"); err != nil {
		t.Fatalf("reconcile add manager: %v", err)
	}

	if got := streamMembers(t, st, channels.TeamStreamID("w-bob")); !eq(got, []orgchart.WorkerID{"w-bob", "w-li"}) {
		t.Fatalf("s-team-w-bob = %v, want [w-bob w-li]", got)
	}
	if got := streamMembers(t, st, channels.TeamStreamID("w-jane")); !eq(got, []orgchart.WorkerID{"w-jane", "w-li"}) {
		t.Fatalf("s-team-w-jane (unchanged) = %v, want [w-jane w-li]", got)
	}
	if got := streamMembers(t, st, activation.StreamID("w-li")); !eq(got, []orgchart.WorkerID{"w-bob", "w-jane"}) {
		t.Fatalf("s-activations-w-li = %v, want [w-bob w-jane]", got)
	}
}

// TestReconcile_RemoveManager mirrors the removeWorkerParent TDD row:
// dropping w-li→w-jane removes w-li from s-team-w-jane, tears the stream
// down if jane now has no reports, and unsubscribes jane from
// s-activations-w-li.
func TestReconcile_RemoveManager(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile initial: %v", err)
	}
	if !streamExists(t, st, channels.TeamStreamID("w-jane")) {
		t.Fatalf("precondition: s-team-w-jane should exist")
	}

	// Drop the line, then reconcile both endpoints.
	if err := st.ReportingLines.Remove(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("remove line: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li", "w-jane"); err != nil {
		t.Fatalf("reconcile remove: %v", err)
	}

	if streamExists(t, st, channels.TeamStreamID("w-jane")) {
		t.Fatalf("s-team-w-jane should be torn down (jane has 0 reports)")
	}
	// jane no longer observes w-li's transcript.
	if got := streamMembers(t, st, activation.StreamID("w-li")); len(got) != 0 {
		t.Fatalf("s-activations-w-li observers = %v, want none after losing its only manager", got)
	}
}

// TestReconcile_RemoveManager_KeepsStreamWhenOtherReports: dropping one
// report from a manager that still has others keeps the team stream and
// only removes the departing report.
func TestReconcile_RemoveManager_KeepsStreamWhenOtherReports(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	seedWorker(t, st, ai("w-sam"))
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

	if got := streamMembers(t, st, channels.TeamStreamID("w-jane")); !eq(got, []orgchart.WorkerID{"w-jane", "w-sam"}) {
		t.Fatalf("s-team-w-jane = %v, want [w-jane w-sam]", got)
	}
}

// TestReconcile_FireReport mirrors the fire-a-report TDD row: after the
// worker row + its lines are gone, Reconcile(firedID, exManagers…) tears
// down the now-empty team stream.
func TestReconcile_FireReport(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile initial: %v", err)
	}

	// Capture managers BEFORE deletion (lines cascade with the row).
	managers, err := st.ReportingLines.ListManagers(ctx, orgID, "w-li")
	if err != nil {
		t.Fatalf("list managers: %v", err)
	}
	if err := st.Workers.Delete(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	affected := append([]orgchart.WorkerID{"w-li"}, managers...)
	if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}

	if streamExists(t, st, channels.TeamStreamID("w-jane")) {
		t.Fatalf("s-team-w-jane should be gone after firing its only report")
	}
	if streamExists(t, st, activation.StreamID("w-li")) {
		t.Fatalf("s-activations-w-li should be gone after firing w-li")
	}
}

// TestReconcile_FireManager mirrors the fire-a-manager TDD row: firing
// the manager removes the team stream; the reports keep their own
// subtrees.
func TestReconcile_FireManager(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-owner"))
	seedWorker(t, st, ai("w-jane"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-owner", "w-jane")
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-jane"); err != nil {
		t.Fatalf("reconcile jane: %v", err)
	}
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile li: %v", err)
	}
	if !streamExists(t, st, channels.TeamStreamID("w-jane")) {
		t.Fatalf("precondition: s-team-w-jane should exist")
	}

	managers, _ := st.ReportingLines.ListManagers(ctx, orgID, "w-jane")
	if err := st.Workers.Delete(ctx, orgID, "w-jane"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}
	affected := append([]orgchart.WorkerID{"w-jane"}, managers...)
	if err := rec.Reconcile(ctx, orgID, affected...); err != nil {
		t.Fatalf("reconcile fire: %v", err)
	}

	if streamExists(t, st, channels.TeamStreamID("w-jane")) {
		t.Fatalf("s-team-w-jane should be torn down")
	}
	// w-li still exists and keeps its own activation stream.
	if !streamExists(t, st, activation.StreamID("w-li")) {
		t.Fatalf("w-li should keep its own activation stream")
	}
}

// TestReconcile_Idempotent: a second Reconcile with no graph change is a
// no-op — same streams, same members.
func TestReconcile_Idempotent(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-jane"))
	seedWorker(t, st, ai("w-li"))
	addLine(t, st, "w-jane", "w-li")
	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	before := streamMembers(t, st, channels.TeamStreamID("w-jane"))

	if err := rec.Reconcile(ctx, orgID, "w-li"); err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	after := streamMembers(t, st, channels.TeamStreamID("w-jane"))
	if !eq(before, after) {
		t.Fatalf("idempotency broken: before=%v after=%v", before, after)
	}
}

// TestEnsureStreamWithMembers_ConcurrentCreateRace proves the helper is
// safe against two callers racing on the same deterministic stream id —
// the TOCTOU the topology design must tolerate (two simultaneous DMs
// between the same pair, two reconciles touching one team stream). Many
// goroutines released from a barrier all get-or-create the SAME stream
// and subscribe the SAME member; the memory store runs Get and Create
// under separate locks, so the loser of each race takes the
// re-check-after-Create path. Every call must succeed, and the end state
// must be exactly one stream with exactly one subscription.
func TestEnsureStreamWithMembers_ConcurrentCreateRace(t *testing.T) {
	st := memorystore.New()
	rec := New(Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions, Now: fixedNow})
	ctx := context.Background()
	const sid = streaming.StreamID("s-team-w-race")
	stream, err := streaming.NewStream(sid, "Team: race", "", "w-race", fixedNow(), transport.Transport{}, orgID)
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}

	const n = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // release all at once to maximise interleaving
			errs[idx] = rec.ensureStreamWithMembers(ctx, stream, fixedNow(), "w-member")
		}(i)
	}
	close(start)
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("goroutine %d errored on concurrent ensure: %v", i, e)
		}
	}
	// Exactly one stream, exactly one subscription — no duplicates, no
	// spurious unique-constraint failures.
	if got := streamMembers(t, st, sid); !eq(got, []orgchart.WorkerID{"w-member"}) {
		t.Fatalf("subscribers = %v, want exactly [w-member]", got)
	}
}

// TestReconcile_OwnerWithReport mirrors the owner TDD row: the owner with
// a direct report has both a self-observed activation stream and a team
// stream containing the owner + report.
func TestReconcile_OwnerWithReport(t *testing.T) {
	rec, st := newRec(t)
	ctx := context.Background()
	seedWorker(t, st, human("w-owner"))
	// Owner bootstrap reconcile mints its self-observed activation stream.
	if err := rec.Reconcile(ctx, orgID, "w-owner"); err != nil {
		t.Fatalf("reconcile owner: %v", err)
	}
	if got := streamMembers(t, st, activation.StreamID("w-owner")); !eq(got, []orgchart.WorkerID{"w-owner"}) {
		t.Fatalf("owner activation observers = %v, want [w-owner]", got)
	}

	seedWorker(t, st, ai("w-jane"))
	addLine(t, st, "w-owner", "w-jane")
	if err := rec.Reconcile(ctx, orgID, "w-jane"); err != nil {
		t.Fatalf("reconcile jane: %v", err)
	}
	if got := streamMembers(t, st, channels.TeamStreamID("w-owner")); !eq(got, []orgchart.WorkerID{"w-jane", "w-owner"}) {
		t.Fatalf("s-team-w-owner = %v, want [w-jane w-owner]", got)
	}
	// Owner's own activation stream survived the team-stream creation.
	if got := streamMembers(t, st, activation.StreamID("w-owner")); !eq(got, []orgchart.WorkerID{"w-owner"}) {
		t.Fatalf("owner activation observers after report = %v, want [w-owner]", got)
	}
}
