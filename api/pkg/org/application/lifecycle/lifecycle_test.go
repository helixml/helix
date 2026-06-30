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

// TestDelete_RemovesBotsTranscript pins the regression behind "we still
// see s-transcript-w-ai-1 and s-transcript-w-test-ai even though those
// bots are gone": the Delete cascade tore down subscriptions, runtime
// state, and the bot row — but left the per-Bot transcript
// (s-transcript-<botID>) lying around, so the Topics page kept rendering
// ghost rows for bots that no longer existed and the chart's orphan
// strip filled up with dashed pseudo-nodes.
//
// Activation events themselves are still audit-retained (the
// `org_events` rows survive); only the Topic row is cleaned up so the
// UI surfaces stop showing it as an active channel.
func TestDelete_RemovesBotsTranscript(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-test"

	// Seed a bot + its transcript the same way Create would.
	bot, err := orgchart.NewBot("w-ghost", "# Ghost", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	if err := st.Bots.Create(ctx, bot); err != nil {
		t.Fatalf("create bot: %v", err)
	}
	topicID := activation.TranscriptID(bot.ID)
	topic, err := streaming.NewTopic(
		topicID, "Activations: w-ghost", "test",
		bot.ID, time.Now().UTC(),
		transport.Transport{}, orgID,
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// Sanity: the topic is there before we delete.
	if _, err := st.Topics.Get(ctx, orgID, topicID); err != nil {
		t.Fatalf("precondition: transcript not seeded: %v", err)
	}

	svc := &lifecycle.Service{Store: st, BotReconcilers: []lifecycle.BotReconciler{reconcile.New(reconcile.Deps{Bots: st.Bots, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions})}}
	if err := svc.Delete(ctx, orgID, bot.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// The topic row must be gone.
	if _, err := st.Topics.Get(ctx, orgID, topicID); err == nil {
		t.Fatalf("transcript %q still exists after Delete — orphan regression", topicID)
	}
}

// TestDelete_CascadesReportingLinesAndSubscriptions pins the two cascade
// bugs found in the 2026-06-06 QA run, now handled structurally by the
// store:
//
//   - F8: deleting a manager left their direct reports pointing at the
//     now-deleted bot. With reporting lines, deleting the manager must
//     drop every line that references it (the gorm store does this with
//     ON DELETE CASCADE; the memory store mirrors it).
//   - F5: deleting a bot deleted its s-transcript-<id> topic but
//     left OTHER bots' subscriptions to that topic behind.
func TestDelete_CascadesReportingLinesAndSubscriptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-cascade"

	mgr, err := orgchart.NewBot("w-mgr", "# Mgr", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := st.Bots.Create(ctx, mgr); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	report, err := orgchart.NewBot("w-report", "# Report", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := st.Bots.Create(ctx, report); err != nil {
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

	// The manager's transcript + an outside subscriber (mirrors the
	// caller auto-subscribed to a new bot's activations).
	mgrTopic := activation.TranscriptID(mgr.ID)
	topic, err := streaming.NewTopic(mgrTopic, "Activations: w-mgr", "test", mgr.ID, time.Now().UTC(), transport.Transport{}, orgID)
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

	svc := &lifecycle.Service{Store: st, BotReconcilers: []lifecycle.BotReconciler{reconcile.New(reconcile.Deps{Bots: st.Bots, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions})}}
	if err := svc.Delete(ctx, orgID, mgr.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// F8: no reporting line may reference the deleted manager.
	managers, err := st.ReportingLines.ListManagers(ctx, orgID, "w-report")
	if err != nil {
		t.Fatalf("list managers after delete: %v", err)
	}
	if len(managers) != 0 {
		t.Fatalf("w-report still reports to %v after deleting its manager, want none (F8 dangling-line regression)", managers)
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

// TestDelete_TearsDownDMChannelToReports pins the 2026-06-16 QA finding:
// deleting a manager left the 1:1 DM channel (`s-dm-<mgr>-<report>`) it
// shared with each direct report lying around — the report stayed
// subscribed to a DM with a now-deleted bot. The reconciler's DM-channel
// teardown is an all-pairs-of-affected scan, so to settle
// `s-dm-<mgr>-<report>` BOTH endpoints have to be in the affected set;
// Delete must feed itself + its ex-managers + its ex-reports.
func TestDelete_TearsDownDMChannelToReports(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-dm-delete"

	mgr, err := orgchart.NewBot("w-mgr", "# Mgr", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := st.Bots.Create(ctx, mgr); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	report, err := orgchart.NewBot("w-report", "# Report", nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := st.Bots.Create(ctx, report); err != nil {
		t.Fatalf("create report: %v", err)
	}
	line, err := orgchart.NewReportingLine(orgID, "w-mgr", "w-report")
	if err != nil {
		t.Fatalf("new reporting line: %v", err)
	}
	if err := st.ReportingLines.Add(ctx, line); err != nil {
		t.Fatalf("add reporting line: %v", err)
	}

	rec := reconcile.New(reconcile.Deps{Bots: st.Bots, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions})
	// Provision the channels the edge implies (transcript observership,
	// team topic, and — the one under test — the 1:1 DM channel).
	if err := rec.Reconcile(ctx, orgID, "w-mgr", "w-report"); err != nil {
		t.Fatalf("reconcile (wire edge): %v", err)
	}
	dm := channels.DMTopicID("w-mgr", "w-report")
	if _, err := st.Topics.Get(ctx, orgID, dm); err != nil {
		t.Fatalf("precondition: DM channel %q should exist after wiring the edge: %v", dm, err)
	}

	svc := &lifecycle.Service{Store: st, BotReconcilers: []lifecycle.BotReconciler{rec}}
	if err := svc.Delete(ctx, orgID, mgr.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// The DM channel must be gone — not left orphaned referencing the
	// deleted manager.
	if _, err := st.Topics.Get(ctx, orgID, dm); err == nil {
		t.Fatalf("DM channel %q still exists after deleting the manager — orphan regression", dm)
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
// Helix project/app, so the Delete cascade never calls into Helix).
func newLifecycleSvc(st *store.Store) *lifecycle.Service {
	return &lifecycle.Service{
		Store:          st,
		BotReconcilers: []lifecycle.BotReconciler{reconcile.New(reconcile.Deps{Bots: st.Bots, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions})},
	}
}

func seedBot(t *testing.T, st *store.Store, orgID, id string) {
	t.Helper()
	b, err := orgchart.NewBot(id, "# "+id, nil, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new bot %s: %v", id, err)
	}
	if err := st.Bots.Create(context.Background(), b); err != nil {
		t.Fatalf("create bot %s: %v", id, err)
	}
}

// TestDelete_ReconcilesSurvivingReport covers the dangerous cascade: a
// bot *manages* another bot. Deleting the manager must not leave the
// surviving report pointing at a deleted manager, and the comms channels
// that edge implied (team topic + 1:1 DM) must be torn down rather than
// orphaned. This is the ISSUE-2 teardown class.
func TestDelete_ReconcilesSurvivingReport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delete-cascade"

	seedBot(t, st, orgID, "w-mgr") // root manager (doomed)
	seedBot(t, st, orgID, "w-ic")  // survivor, reports to w-mgr

	line, err := orgchart.NewReportingLine(orgID, "w-mgr", "w-ic")
	if err != nil {
		t.Fatalf("new reporting line: %v", err)
	}
	if err := st.ReportingLines.Add(ctx, line); err != nil {
		t.Fatalf("add reporting line: %v", err)
	}

	svc := newLifecycleSvc(st)
	// Provision the channels the edge implies (team topic + DM channel).
	if err := svc.BotReconcilers[0].Reconcile(ctx, orgID, "w-mgr", "w-ic"); err != nil {
		t.Fatalf("reconcile (wire edge): %v", err)
	}
	dm := channels.DMTopicID("w-mgr", "w-ic")
	if _, err := st.Topics.Get(ctx, orgID, dm); err != nil {
		t.Fatalf("precondition: DM channel %q should exist: %v", dm, err)
	}
	if _, err := st.Topics.Get(ctx, orgID, "s-team-w-mgr"); err != nil {
		t.Fatalf("precondition: team topic s-team-w-mgr should exist: %v", err)
	}

	if err := svc.Delete(ctx, orgID, "w-mgr"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Manager deleted, report survives.
	if _, err := st.Bots.Get(ctx, orgID, "w-mgr"); err == nil {
		t.Fatal("w-mgr should be deleted")
	}
	if _, err := st.Bots.Get(ctx, orgID, "w-ic"); err != nil {
		t.Fatalf("w-ic should survive: %v", err)
	}
	// The surviving report no longer points at the deleted manager.
	mgrs, err := st.ReportingLines.ListManagers(ctx, orgID, "w-ic")
	if err != nil {
		t.Fatalf("list managers: %v", err)
	}
	if len(mgrs) != 0 {
		t.Fatalf("w-ic still reports to %v after its manager was deleted", mgrs)
	}
	// No orphaned comms channels referencing the deleted manager.
	if _, err := st.Topics.Get(ctx, orgID, dm); err == nil {
		t.Fatalf("DM channel %q orphaned after Delete", dm)
	}
	if _, err := st.Topics.Get(ctx, orgID, "s-team-w-mgr"); err == nil {
		t.Fatal("team topic s-team-w-mgr orphaned after Delete")
	}
}

// TestDelete_Guards pins the cheap input guards.
func TestDelete_Guards(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)

	if err := (&lifecycle.Service{Store: st}).Delete(ctx, "org", ""); err == nil {
		t.Fatal("empty bot id should error")
	}
	if err := (&lifecycle.Service{}).Delete(ctx, "org", "b-x"); err == nil {
		t.Fatal("nil store should error")
	}
}

// TestDelete_MissingBotErrorsWithNoSideEffects pins that deleting a bot
// that doesn't exist errors at the get-guard and leaves the graph alone —
// no bystander is swept up by a no-op delete.
func TestDelete_MissingBotErrorsWithNoSideEffects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delete-missing"

	seedBot(t, st, orgID, "w-bystander")

	svc := newLifecycleSvc(st)
	if err := svc.Delete(ctx, orgID, "w-missing"); err == nil {
		t.Fatal("Delete on a non-existent bot should error")
	}
	if _, err := st.Bots.Get(ctx, orgID, "w-bystander"); err != nil {
		t.Fatalf("bystander must be untouched by a failed Delete: %v", err)
	}
}
