package mcptools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// TestReports_TeamTopicAndDMTopics: jane has two reports; the reports
// tool returns a non-null team topic id plus each report's DM topic
// id. Neither report manages a sub-team here, so manages is false and no
// per-report teamTopicId is shown.
func TestReports_TeamTopicAndDMTopics(t *testing.T) {
	deps := seedReportingGraph(t)
	tl := &Reports{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: callerBot("b-jane"), Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got reportsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.TeamTopicID == nil || *got.TeamTopicID != channels.TeamTopicID("b-jane") {
		t.Fatalf("teamTopicId = %v, want s-team-b-jane", got.TeamTopicID)
	}
	if len(got.Reports) != 2 {
		t.Fatalf("reports = %+v, want 2 (b-li, b-sam)", got.Reports)
	}
	for _, r := range got.Reports {
		if r.Manages {
			t.Fatalf("report %s should not manage anyone (no sub-reports)", r.ID)
		}
		if r.TeamTopicID != nil {
			t.Fatalf("non-managing report %s must not carry a teamTopicId", r.ID)
		}
		wantDM := channels.DMTopicID("b-jane", r.ID)
		if r.DMTopicID != wantDM {
			t.Fatalf("report %s dmTopicId = %q, want %q", r.ID, r.DMTopicID, wantDM)
		}
	}
}

// TestReports_ManagesFlagSurfacesSubTeam: a report that leads its own
// sub-team is flagged manages:true and carries its sub-team topic id.
func TestReports_ManagesFlagSurfacesSubTeam(t *testing.T) {
	deps := seedReportingGraph(t)
	// b-owner's only report is b-jane, who manages li + sam.
	tl := &Reports{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: callerBot("b-owner"), Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var got reportsResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Reports) != 1 || got.Reports[0].ID != "b-jane" {
		t.Fatalf("reports = %+v, want [b-jane]", got.Reports)
	}
	jane := got.Reports[0]
	if !jane.Manages {
		t.Fatalf("b-jane should be flagged manages:true")
	}
	if jane.TeamTopicID == nil || *jane.TeamTopicID != channels.TeamTopicID("b-jane") {
		t.Fatalf("b-jane teamTopicId = %v, want s-team-b-jane", jane.TeamTopicID)
	}
	if jane.DMTopicID != channels.DMTopicID("b-owner", "b-jane") {
		t.Fatalf("b-jane dmTopicId = %q, want %q", jane.DMTopicID, channels.DMTopicID("b-owner", "b-jane"))
	}
}

// TestReports_NoReportsNullTeamTopic: a leaf bot has no reports —
// teamTopicId is null and reports is an empty array.
func TestReports_NoReportsNullTeamTopic(t *testing.T) {
	deps := seedReportingGraph(t)
	tl := &Reports{deps: deps.Build()}

	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: callerBot("b-sam"), Args: json.RawMessage(`{}`)})
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
