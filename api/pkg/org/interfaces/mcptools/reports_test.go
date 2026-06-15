package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/topology"
)

// TestReports_TeamStreamAndDMStreams: jane has two reports; the reports
// tool returns a non-null team stream id plus each report's DM stream
// id. Neither report manages a sub-team here, so manages is false and no
// per-report teamStreamId is shown.
func TestReports_TeamStreamAndDMStreams(t *testing.T) {
	deps := seedReportingGraph(t)
	caller, _ := orgchart.NewAIWorker("w-jane", "r-x", "#", "org-test")
	tl := &Reports{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got reportsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TeamStreamID == nil || *got.TeamStreamID != topology.TeamStreamID("w-jane") {
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
		wantDM := topology.DMStreamID("w-jane", r.ID)
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
	tl := &Reports{deps: deps.Build()}

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
	if jane.TeamStreamID == nil || *jane.TeamStreamID != topology.TeamStreamID("w-jane") {
		t.Fatalf("w-jane teamStreamId = %v, want s-team-w-jane", jane.TeamStreamID)
	}
	if jane.DMStreamID != topology.DMStreamID("w-owner", "w-jane") {
		t.Fatalf("w-jane dmStreamId = %q, want %q", jane.DMStreamID, topology.DMStreamID("w-owner", "w-jane"))
	}
}

// TestReports_NoReportsNullTeamStream: a leaf worker has no reports —
// teamStreamId is null and reports is an empty array.
func TestReports_NoReportsNullTeamStream(t *testing.T) {
	deps := seedReportingGraph(t)
	caller, _ := orgchart.NewAIWorker("w-sam", "r-x", "#", "org-test")
	tl := &Reports{deps: deps.Build()}

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
