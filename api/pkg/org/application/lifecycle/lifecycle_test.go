package lifecycle_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/internal/orgtest"
)

type recordingAgentDeliveryCleaner struct {
	orgID    string
	agentID  orgchart.NodeID
	restored bool
}

func (c *recordingAgentDeliveryCleaner) CleanupAgent(_ context.Context, orgID string, agentID orgchart.NodeID) error {
	c.orgID = orgID
	c.agentID = agentID
	return nil
}

func (c *recordingAgentDeliveryCleaner) RestoreAgent(orgID string, agentID orgchart.NodeID) {
	c.orgID = orgID
	c.agentID = agentID
	c.restored = true
}

// TestDelete_RemovesBotsTranscript pins the regression behind "we still
// see s-transcript-w-ai-1 and s-transcript-w-test-ai even though those
// bots are gone": the Delete cascade tore down subscriptions, runtime
// state, and the bot row — but left the per-Bot transcript
// (s-transcript-<botID>) lying around, so the Triggers page kept
// rendering ghost rows for bots that no longer existed and the chart's
// orphan strip filled up with dashed pseudo-nodes.
//
// Activation events themselves are still audit-retained (the
// `org_events` rows survive); only the Trigger row is cleaned up so the
// UI surfaces stop showing it as an active channel.
func TestDelete_RemovesBotsTranscript(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-test"

	// Seed a bot + its transcript the same way Create would.
	bot, err := orgchart.NewNode("w-ghost", "# Ghost", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatalf("create bot: %v", err)
	}
	transcriptID := activation.TranscriptID(bot.ID)
	orgtest.Trigger(t, st, orgID, transcriptID)

	// Sanity: the channel is there before we delete.
	if !triggerExists(t, st, orgID, transcriptID) {
		t.Fatal("precondition: transcript not seeded")
	}

	svc := &lifecycle.Service{Store: st, NodeReconcilers: []lifecycle.NodeReconciler{reconcile.New(reconcile.Deps{Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers, Attachments: st.WorkerAttachments})}}
	if err := svc.Delete(ctx, orgID, bot.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// The Trigger row must be gone.
	if triggerExists(t, st, orgID, transcriptID) {
		t.Fatalf("transcript %q still exists after Delete — orphan regression", transcriptID)
	}
}

// TestDelete_CascadesReportingLinesAndAttachments pins the two cascade
// bugs found in the 2026-06-06 QA run, now handled structurally by the
// store:
//
//   - F8: deleting a manager left their direct reports pointing at the
//     now-deleted bot. With reporting lines, deleting the manager must
//     drop every line that references it (the gorm store does this with
//     ON DELETE CASCADE; the memory store mirrors it).
//   - F5: deleting a bot deleted its s-transcript-<id> channel but
//     left OTHER bots' attachments to it behind.
func TestDelete_CascadesReportingLinesAndAttachments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-cascade"

	mgr, err := orgchart.NewNode("w-mgr", "# Mgr", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := st.Nodes.Create(ctx, mgr); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	report, err := orgchart.NewNode("w-report", "# Report", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := st.Nodes.Create(ctx, report); err != nil {
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

	// The manager's transcript + an outside observer (mirrors the caller
	// auto-attached to a new bot's activations).
	mgrTranscript := activation.TranscriptID(mgr.ID)
	orgtest.Trigger(t, st, orgID, mgrTranscript)
	orgtest.AttachTrigger(t, st, orgID, "w-report", mgrTranscript)

	svc := &lifecycle.Service{Store: st, NodeReconcilers: []lifecycle.NodeReconciler{reconcile.New(reconcile.Deps{Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers, Attachments: st.WorkerAttachments})}}
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

	// F5: no attachment may reference the deleted transcript.
	if got := triggerMembers(t, st, orgID, mgrTranscript); len(got) != 0 {
		t.Fatalf("found %d attachment(s) to deleted channel %q, want 0 (F5 orphan regression)", len(got), mgrTranscript)
	}
}

// TestDelete_TearsDownDMChannelToReports pins the 2026-06-16 QA finding:
// deleting a manager left the 1:1 DM channel (`s-dm-<mgr>-<report>`) it
// shared with each direct report lying around — the report stayed
// attached to a DM with a now-deleted bot. The reconciler's DM-channel
// teardown is an all-pairs-of-affected scan, so to settle
// `s-dm-<mgr>-<report>` BOTH endpoints have to be in the affected set;
// Delete must feed itself + its ex-managers + its ex-reports.
func TestDelete_TearsDownDMChannelToReports(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-dm-delete"

	mgr, err := orgchart.NewNode("w-mgr", "# Mgr", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := st.Nodes.Create(ctx, mgr); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	report, err := orgchart.NewNode("w-report", "# Report", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := st.Nodes.Create(ctx, report); err != nil {
		t.Fatalf("create report: %v", err)
	}
	line, err := orgchart.NewReportingLine(orgID, "w-mgr", "w-report")
	if err != nil {
		t.Fatalf("new reporting line: %v", err)
	}
	if err := st.ReportingLines.Add(ctx, line); err != nil {
		t.Fatalf("add reporting line: %v", err)
	}

	rec := reconcile.New(reconcile.Deps{Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers, Attachments: st.WorkerAttachments})
	// Provision the channels the edge implies (transcript observership,
	// team chat, and — the one under test — the 1:1 DM channel).
	if err := rec.Reconcile(ctx, orgID, "w-mgr", "w-report"); err != nil {
		t.Fatalf("reconcile (wire edge): %v", err)
	}
	dm := channels.DMTriggerID("w-mgr", "w-report")
	if !triggerExists(t, st, orgID, dm) {
		t.Fatalf("precondition: DM channel %q should exist after wiring the edge", dm)
	}

	svc := &lifecycle.Service{Store: st, NodeReconcilers: []lifecycle.NodeReconciler{rec}}
	if err := svc.Delete(ctx, orgID, mgr.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// The DM channel must be gone — not left orphaned referencing the
	// deleted manager.
	if triggerExists(t, st, orgID, dm) {
		t.Fatalf("DM channel %q still exists after deleting the manager — orphan regression", dm)
	}
	// And the surviving report must not still be attached to it.
	if got := triggerMembers(t, st, orgID, dm); len(got) != 0 {
		t.Fatalf("found %d attachment(s) to torn-down DM %q, want 0", len(got), dm)
	}
}

// newLifecycleSvc builds a Service wired to a reconciler against the same
// store, with nil Helix runtime (the memory-store tests don't provision a
// Helix project/app, so the Delete cascade never calls into Helix).
func newLifecycleSvc(st *store.Store) *lifecycle.Service {
	return &lifecycle.Service{
		Store:           st,
		NodeReconcilers: []lifecycle.NodeReconciler{reconcile.New(reconcile.Deps{Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers, Attachments: st.WorkerAttachments})},
	}
}

func seedBot(t *testing.T, st *store.Store, orgID, id string) {
	t.Helper()
	b, err := orgchart.NewNode(id, "# "+id, nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new bot %s: %v", id, err)
	}
	if err := st.Nodes.Create(context.Background(), b); err != nil {
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
	// Provision the channels the edge implies (team chat + DM channel).
	if err := svc.NodeReconcilers[0].Reconcile(ctx, orgID, "w-mgr", "w-ic"); err != nil {
		t.Fatalf("reconcile (wire edge): %v", err)
	}
	dm := channels.DMTriggerID("w-mgr", "w-ic")
	if !triggerExists(t, st, orgID, dm) {
		t.Fatalf("precondition: DM channel %q should exist", dm)
	}
	if !triggerExists(t, st, orgID, "s-team-w-mgr") {
		t.Fatal("precondition: team chat s-team-w-mgr should exist")
	}

	if err := svc.Delete(ctx, orgID, "w-mgr"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Manager deleted, report survives.
	if _, err := st.Nodes.Get(ctx, orgID, "w-mgr"); err == nil {
		t.Fatal("w-mgr should be deleted")
	}
	if _, err := st.Nodes.Get(ctx, orgID, "w-ic"); err != nil {
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
	if triggerExists(t, st, orgID, dm) {
		t.Fatalf("DM channel %q orphaned after Delete", dm)
	}
	if triggerExists(t, st, orgID, "s-team-w-mgr") {
		t.Fatal("team chat s-team-w-mgr orphaned after Delete")
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

func TestDelete_CleansUpAgentDelivery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delete-delivery"
	seedBot(t, st, orgID, "w-delete")

	cleaner := &recordingAgentDeliveryCleaner{}
	if err := (&lifecycle.Service{Store: st, AgentDelivery: cleaner}).Delete(ctx, orgID, "w-delete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if cleaner.orgID != orgID || cleaner.agentID != "w-delete" {
		t.Fatalf("cleaned delivery for (%q, %q), want (%q, %q)", cleaner.orgID, cleaner.agentID, orgID, "w-delete")
	}
}

func TestDelete_RestoresAgentDeliveryWhenRuntimeDeleteFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	const orgID = "org-delete-delivery-failure"
	bot, err := orgchart.NewNode("w-delete", "# w-delete", nil, time.Now().UTC(), orgID)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	bot.AgentID = "app-delete"
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatalf("create bot: %v", err)
	}

	cleaner := &recordingAgentDeliveryCleaner{}
	svc := &lifecycle.Service{Store: st, Helix: &lifecycleRuntime{linkedErr: errors.New("delete failed")}, AgentDelivery: cleaner}
	if err := svc.Delete(ctx, orgID, bot.ID); err == nil {
		t.Fatal("Delete should fail")
	}
	if !cleaner.restored {
		t.Fatal("agent delivery was not restored after failed deletion")
	}
	if _, err := st.Nodes.Get(ctx, orgID, bot.ID); err != nil {
		t.Fatalf("bot should survive failed deletion: %v", err)
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
	if _, err := st.Nodes.Get(ctx, orgID, "w-bystander"); err != nil {
		t.Fatalf("bystander must be untouched by a failed Delete: %v", err)
	}
}

// triggerExists reports whether a Trigger row is present.
func triggerExists(t *testing.T, st *store.Store, orgID, id string) bool {
	t.Helper()
	rows, err := st.Triggers.Find(context.Background(), store.WithOrg(orgID), store.WithID(id), store.WithLimit(1))
	if err != nil {
		t.Fatalf("find trigger %q: %v", id, err)
	}
	return len(rows) > 0
}

// triggerMembers returns the Workers attached to a Trigger.
func triggerMembers(t *testing.T, st *store.Store, orgID, id string) []orgchart.NodeID {
	t.Helper()
	rows, err := st.WorkerAttachments.Find(context.Background(), store.WithOrg(orgID), store.WithTriggerID(id))
	if err != nil {
		t.Fatalf("find attachments for %q: %v", id, err)
	}
	out := make([]orgchart.NodeID, 0, len(rows))
	for _, a := range rows {
		out = append(out, a.WorkerID)
	}
	return out
}
