package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// dmTestEnv seeds two workers and (optionally) a reporting line between
// them, reconciling topology so the DM channel exists exactly when a
// reporting relationship does. Returns Deps + the two workers.
func dmTestEnv(t *testing.T, wireLine bool) (Config, orgchart.Worker, orgchart.Worker) {
	t.Helper()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	role, _ := orgchart.NewRole("r-x", "# X", nil, nil, time.Now().UTC(), "org-test")
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	mgr, _ := orgchart.NewHumanWorker("w-mgr", "r-x", "#", "org-test")
	rep, _ := orgchart.NewAIWorker("w-rep", "r-x", "#", "org-test")
	for _, w := range []orgchart.Worker{mgr, rep} {
		if err := st.Workers.Create(ctx, w); err != nil {
			t.Fatalf("create %s: %v", w.ID(), err)
		}
	}
	deps := DefaultDeps(st)
	if wireLine {
		addReportingLine(t, st, "w-mgr", "w-rep")
		// Reconciler provisions the DM channel for the new edge.
		if err := deps.Reconciler.Reconcile(ctx, "org-test", "w-rep", "w-mgr"); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
	}
	return deps, mgr, rep
}

// TestDM_DeliversOverExistingChannel: with a reporting line wired (so
// topology provisioned s-dm-w-mgr-w-rep), the report can DM its manager
// — the event lands on the deterministic topic and both parties are
// subscribers (topology subscribed them, dm does NOT re-create).
func TestDM_DeliversOverExistingChannel(t *testing.T) {
	deps, _, rep := dmTestEnv(t, true)
	ctx := context.Background()
	tl := &DM{deps: deps.Build()}

	args, _ := json.Marshal(dmArgs{ToWorkerID: "w-mgr", Body: "blocked — need a decision"})
	raw, err := tl.Invoke(ctx, tool.Invocation{Caller: rep, Args: args})
	if err != nil {
		t.Fatalf("dm over existing channel: %v", err)
	}
	var out struct {
		TopicID string `json:"topicId"`
		To       string `json:"to"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := string(channels.DMTopicID("w-rep", "w-mgr"))
	if out.TopicID != want {
		t.Fatalf("topicId = %q, want %q", out.TopicID, want)
	}
	// The event landed on the channel.
	events, _ := deps.Store.Events.ListForTopic(ctx, "org-test", channels.DMTopicID("w-rep", "w-mgr"), 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	// Both parties are subscribers (provisioned by topology, not by dm).
	for _, id := range []orgchart.WorkerID{"w-mgr", "w-rep"} {
		if _, err := deps.Store.Subscriptions.Find(ctx, "org-test", id, channels.DMTopicID("w-rep", "w-mgr")); err != nil {
			t.Fatalf("%s not subscribed to DM channel: %v", id, err)
		}
	}
}

// TestDM_RefusesWithoutReportingLine: the load-bearing new behaviour. No
// reporting relationship → no DM channel → dm refuses noisily and writes
// nothing, rather than minting a channel the org never sanctioned.
func TestDM_RefusesWithoutReportingLine(t *testing.T) {
	deps, _, rep := dmTestEnv(t, false) // no line wired
	ctx := context.Background()
	tl := &DM{deps: deps.Build()}

	args, _ := json.Marshal(dmArgs{ToWorkerID: "w-mgr", Body: "hi"})
	_, err := tl.Invoke(ctx, tool.Invocation{Caller: rep, Args: args})
	if err == nil {
		t.Fatal("dm without a reporting line must error")
	}
	// The error points the agent at managers / reports.
	if !strings.Contains(err.Error(), "managers") || !strings.Contains(err.Error(), "reports") {
		t.Fatalf("err = %v, want it to mention `managers` and `reports`", err)
	}
	// No channel was created and no event written.
	if _, gerr := deps.Store.Topics.Get(ctx, "org-test", channels.DMTopicID("w-rep", "w-mgr")); gerr == nil {
		t.Fatal("dm must NOT create the channel on the refusal path")
	}
	events, _ := deps.Store.Events.ListForTopic(ctx, "org-test", channels.DMTopicID("w-rep", "w-mgr"), 10)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (nothing written on refusal)", len(events))
	}
}

// TestDM_RejectsSelfAndUnknownRecipient: the up-front guards.
func TestDM_RejectsSelfAndUnknownRecipient(t *testing.T) {
	deps, _, rep := dmTestEnv(t, true)
	ctx := context.Background()
	tl := &DM{deps: deps.Build()}

	self, _ := json.Marshal(dmArgs{ToWorkerID: "w-rep", Body: "x"})
	if _, err := tl.Invoke(ctx, tool.Invocation{Caller: rep, Args: self}); err == nil {
		t.Fatal("self-DM must be rejected")
	}

	ghost, _ := json.Marshal(dmArgs{ToWorkerID: "w-ghost", Body: "x"})
	_, err := tl.Invoke(ctx, tool.Invocation{Caller: rep, Args: ghost})
	if err == nil {
		t.Fatal("DM to a non-existent recipient must be rejected")
	}
}
