package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// seedReportingGraph wires a small graph: owner → jane → {li, sam}, plus
// li also reporting to bob. Returns store-backed Deps. Shared by the
// managers_test and reports_test suites.
func seedReportingGraph(t *testing.T) Config {
	t.Helper()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)

	now := time.Now().UTC()
	for _, id := range []orgchart.BotID{"b-owner", "b-jane", "b-bob", "b-li", "b-sam"} {
		b, _ := orgchart.NewBot(id, "# "+string(id), nil, nil, now, "org-test")
		if err := st.Bots.Create(ctx, b); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	addReportingLine(t, st, "b-owner", "b-jane")
	addReportingLine(t, st, "b-jane", "b-li")
	addReportingLine(t, st, "b-jane", "b-sam")
	addReportingLine(t, st, "b-bob", "b-li") // li has two managers

	return DefaultDeps(st)
}

func addReportingLine(t *testing.T, st *orgstore.Store, manager, report orgchart.BotID) {
	t.Helper()
	line, err := orgchart.NewReportingLine("org-test", manager, report)
	if err != nil {
		t.Fatalf("new line %s->%s: %v", manager, report, err)
	}
	if err := st.ReportingLines.Add(context.Background(), line); err != nil {
		t.Fatalf("add line %s->%s: %v", manager, report, err)
	}
}

// callerBot builds a tool.Caller for the given bot id (the MCP server
// builds the real adapter at the boundary).
func callerBot(id orgchart.BotID) tool.Caller {
	return botCaller{id: string(id), orgID: "org-test"}
}

// TestManagers_ListsBothManagersWithDMTopics: b-li reports to jane and
// bob; the managers tool returns both, each with the deterministic DM
// topic id.
func TestManagers_ListsBothManagersWithDMTopics(t *testing.T) {
	deps := seedReportingGraph(t)
	tl := &Managers{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: callerBot("b-li"), Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got managersResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Managers) != 2 {
		t.Fatalf("managers = %+v, want 2 (b-bob, b-jane)", got.Managers)
	}
	byID := map[orgchart.BotID]managerView{}
	for _, m := range got.Managers {
		byID[m.ID] = m
	}
	jane, ok := byID["b-jane"]
	if !ok {
		t.Fatalf("b-jane missing from managers: %+v", got.Managers)
	}
	if jane.DMTopicID != channels.DMTopicID("b-li", "b-jane") {
		t.Fatalf("jane dmTopicId = %q, want %q", jane.DMTopicID, channels.DMTopicID("b-li", "b-jane"))
	}
	if _, ok := byID["b-bob"]; !ok {
		t.Fatalf("b-bob missing from managers: %+v", got.Managers)
	}
}

// TestManagers_OwnerHasNone: the owner reports to no one — managers is
// an empty (non-null) list.
func TestManagers_OwnerHasNone(t *testing.T) {
	deps := seedReportingGraph(t)
	tl := &Managers{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: callerBot("b-owner"), Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got managersResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Managers == nil {
		t.Fatalf("managers is null, want empty array")
	}
	if len(got.Managers) != 0 {
		t.Fatalf("managers = %+v, want empty", got.Managers)
	}
}

// TestManagers_SkipsDanglingManager: a reporting line that points at a
// manager who no longer exists is skipped rather than returned as a
// phantom (defensive — the line should have cascaded, but tolerate it).
func TestManagers_SkipsDanglingManager(t *testing.T) {
	deps := seedReportingGraph(t)
	// Add a line from a non-existent manager to b-sam directly in the
	// store (bypassing the bot-existence check a real create would do).
	addReportingLine(t, deps.Store, "b-ghost", "b-sam")
	tl := &Managers{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: callerBot("b-sam"), Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got managersResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// b-sam reports to b-jane (real) and b-ghost (dangling) — only jane
	// comes back.
	if len(got.Managers) != 1 || got.Managers[0].ID != "b-jane" {
		t.Fatalf("managers = %+v, want only [b-jane]", got.Managers)
	}
}
