package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// TestReports_TeamTopicAndDMTopics: jane has two reports; the reports
// tool returns a non-null team topic id plus each report's DM topic
// id. Neither report manages a sub-team here, so manages is false and no
// per-report teamTopicId is shown.
func TestReports_TeamTopicAndDMTopics(t *testing.T) {
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
	if got.TeamTopicID == nil || *got.TeamTopicID != channels.TeamTopicID("w-jane") {
		t.Fatalf("teamTopicId = %v, want s-team-w-jane", got.TeamTopicID)
	}
	if len(got.Reports) != 2 {
		t.Fatalf("reports = %+v, want 2 (w-li, w-sam)", got.Reports)
	}
	for _, r := range got.Reports {
		if r.Manages {
			t.Fatalf("report %s should not manage anyone (no sub-reports)", r.ID)
		}
		if r.TeamTopicID != nil {
			t.Fatalf("non-managing report %s must not carry a teamTopicId", r.ID)
		}
		wantDM := channels.DMTopicID("w-jane", r.ID)
		if r.DMTopicID != wantDM {
			t.Fatalf("report %s dmTopicId = %q, want %q", r.ID, r.DMTopicID, wantDM)
		}
	}
}

// TestReports_ManagesFlagSurfacesSubTeam: a report that leads its own
// sub-team is flagged manages:true and carries its sub-team topic id.
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
	if jane.TeamTopicID == nil || *jane.TeamTopicID != channels.TeamTopicID("w-jane") {
		t.Fatalf("w-jane teamTopicId = %v, want s-team-w-jane", jane.TeamTopicID)
	}
	if jane.DMTopicID != channels.DMTopicID("w-owner", "w-jane") {
		t.Fatalf("w-jane dmTopicId = %q, want %q", jane.DMTopicID, channels.DMTopicID("w-owner", "w-jane"))
	}
}

// TestReports_NoReportsNullTeamTopic: a leaf worker has no reports —
// teamTopicId is null and reports is an empty array.
func TestReports_NoReportsNullTeamTopic(t *testing.T) {
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
	if got.TeamTopicID != nil {
		t.Fatalf("teamTopicId = %v, want null", *got.TeamTopicID)
	}
	if got.Reports == nil || len(got.Reports) != 0 {
		t.Fatalf("reports = %+v, want empty array", got.Reports)
	}
}
