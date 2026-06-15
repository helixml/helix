package mcptools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/topology"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// seedReportingGraph wires a small graph: owner → jane → {li, sam}, plus
// li also reporting to bob. Returns store-backed Deps. Shared by the
// managers_test and reports_test suites.
func seedReportingGraph(t *testing.T) Config {
	t.Helper()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)

	role, _ := orgchart.NewRole("r-x", "# X", nil, nil, time.Now().UTC(), "org-test")
	if err := st.Roles.Create(ctx, role); err != nil {
		t.Fatalf("create role: %v", err)
	}
	for _, id := range []orgchart.WorkerID{"w-owner", "w-jane", "w-bob", "w-li", "w-sam"} {
		w, _ := orgchart.NewAIWorker(id, "r-x", "#", "org-test")
		if err := st.Workers.Create(ctx, w); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	addReportingLine(t, st, "w-owner", "w-jane")
	addReportingLine(t, st, "w-jane", "w-li")
	addReportingLine(t, st, "w-jane", "w-sam")
	addReportingLine(t, st, "w-bob", "w-li") // li has two managers

	return DefaultDeps(st)
}

func addReportingLine(t *testing.T, st *orgstore.Store, manager, report orgchart.WorkerID) {
	t.Helper()
	line, err := orgchart.NewReportingLine("org-test", manager, report)
	if err != nil {
		t.Fatalf("new line %s->%s: %v", manager, report, err)
	}
	if err := st.ReportingLines.Add(context.Background(), line); err != nil {
		t.Fatalf("add line %s->%s: %v", manager, report, err)
	}
}

// TestManagers_ListsBothManagersWithDMStreams: w-li reports to jane and
// bob; the managers tool returns both, each with the deterministic DM
// stream id.
func TestManagers_ListsBothManagersWithDMStreams(t *testing.T) {
	deps := seedReportingGraph(t)
	caller, _ := orgchart.NewAIWorker("w-li", "r-x", "#", "org-test")
	tl := &Managers{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got managersResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Managers) != 2 {
		t.Fatalf("managers = %+v, want 2 (w-bob, w-jane)", got.Managers)
	}
	byID := map[orgchart.WorkerID]managerView{}
	for _, m := range got.Managers {
		byID[m.ID] = m
	}
	jane, ok := byID["w-jane"]
	if !ok {
		t.Fatalf("w-jane missing from managers: %+v", got.Managers)
	}
	if jane.DMStreamID != topology.DMStreamID("w-li", "w-jane") {
		t.Fatalf("jane dmStreamId = %q, want %q", jane.DMStreamID, topology.DMStreamID("w-li", "w-jane"))
	}
	if jane.Role != "r-x" {
		t.Fatalf("jane role = %q, want r-x", jane.Role)
	}
	if _, ok := byID["w-bob"]; !ok {
		t.Fatalf("w-bob missing from managers: %+v", got.Managers)
	}
}

// TestManagers_OwnerHasNone: the owner reports to no one — managers is
// an empty (non-null) list.
func TestManagers_OwnerHasNone(t *testing.T) {
	deps := seedReportingGraph(t)
	caller, _ := orgchart.NewHumanWorker("w-owner", "r-x", "#", "org-test")
	tl := &Managers{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
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
	// Add a line from a non-existent manager to w-sam directly in the
	// store (bypassing the worker-existence check a real hire would do).
	addReportingLine(t, deps.Store, "w-ghost", "w-sam")
	caller, _ := orgchart.NewAIWorker("w-sam", "r-x", "#", "org-test")
	tl := &Managers{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got managersResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// w-sam reports to w-jane (real) and w-ghost (dangling) — only jane
	// comes back.
	if len(got.Managers) != 1 || got.Managers[0].ID != "w-jane" {
		t.Fatalf("managers = %+v, want only [w-jane]", got.Managers)
	}
}
