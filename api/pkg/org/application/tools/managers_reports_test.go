package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// seedReportingGraph wires a small graph: owner → jane → {li, sam}, plus
// li also reporting to bob. Returns store-backed Deps.
func seedReportingGraph(t *testing.T) Deps {
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
	tl := &Managers{deps: deps}

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
	if jane.DMStreamID != DMStreamID("w-li", "w-jane") {
		t.Fatalf("jane dmStreamId = %q, want %q", jane.DMStreamID, DMStreamID("w-li", "w-jane"))
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
	tl := &Managers{deps: deps}

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

// TestReports_TeamStreamAndManagesFlag: jane has two reports; li itself
// manages a sub-team (none here) — verify team stream id, dm streams,
// and the manages flag.
func TestReports_TeamStreamAndManagesFlag(t *testing.T) {
	deps := seedReportingGraph(t)
	caller, _ := orgchart.NewAIWorker("w-jane", "r-x", "#", "org-test")
	tl := &Reports{deps: deps}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got reportsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TeamStreamID == nil || *got.TeamStreamID != "s-team-w-jane" {
		t.Fatalf("teamStreamId = %v, want s-team-w-jane", got.TeamStreamID)
	}
	if len(got.Reports) != 2 {
		t.Fatalf("reports = %+v, want 2 (w-li, w-sam)", got.Reports)
	}
	for _, r := range got.Reports {
		if r.Manages {
			t.Fatalf("report %s should not manage anyone (no sub-reports)", r.ID)
		}
		if r.TeamStreamID != nil {
			t.Fatalf("non-managing report %s must not carry a teamStreamId", r.ID)
		}
		wantDM := DMStreamID("w-jane", r.ID)
		if r.DMStreamID != wantDM {
			t.Fatalf("report %s dmStreamId = %q, want %q", r.ID, r.DMStreamID, wantDM)
		}
	}
}

// TestReports_ManagesFlagSurfacesSubTeam: a report that leads its own
// sub-team is flagged manages:true and carries its sub-team stream id.
func TestReports_ManagesFlagSurfacesSubTeam(t *testing.T) {
	deps := seedReportingGraph(t)
	// w-owner's only report is w-jane, who manages li + sam.
	caller, _ := orgchart.NewHumanWorker("w-owner", "r-x", "#", "org-test")
	tl := &Reports{deps: deps}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got reportsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Reports) != 1 || got.Reports[0].ID != "w-jane" {
		t.Fatalf("reports = %+v, want [w-jane]", got.Reports)
	}
	jane := got.Reports[0]
	if !jane.Manages {
		t.Fatalf("w-jane should be flagged manages:true")
	}
	if jane.TeamStreamID == nil || *jane.TeamStreamID != "s-team-w-jane" {
		t.Fatalf("w-jane teamStreamId = %v, want s-team-w-jane", jane.TeamStreamID)
	}
}

// TestReports_NoReportsNullTeamStream: a leaf worker has no reports —
// teamStreamId is null and reports is an empty array.
func TestReports_NoReportsNullTeamStream(t *testing.T) {
	deps := seedReportingGraph(t)
	caller, _ := orgchart.NewAIWorker("w-sam", "r-x", "#", "org-test")
	tl := &Reports{deps: deps}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got reportsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TeamStreamID != nil {
		t.Fatalf("teamStreamId = %v, want null", *got.TeamStreamID)
	}
	if got.Reports == nil || len(got.Reports) != 0 {
		t.Fatalf("reports = %+v, want empty array", got.Reports)
	}
}
