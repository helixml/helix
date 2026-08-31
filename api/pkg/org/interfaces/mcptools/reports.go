package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Reports is the downward, delegation-direction read: who reports to
// you, and every channel to reach them — the team topic to brief them
// all at once, plus each report's DM topic for 1:1. Computed live from
// the reporting graph so the fixed worker-policy prompt can refer to
// "your reports" / "your team" abstractly.
type Reports struct {
	deps Deps
}

const ReportsName tool.Name = "reports"

var reportsSchema = mustSchema[reportsArgs]()

type reportsArgs struct{}

type reportView struct {
	ID          orgchart.NodeID `json:"id"`
	DMChannelID string          `json:"dmChannelId"`
	// Manages is true when this report leads their own sub-team. When
	// true, TeamChatID is shown for context — but you delegate the
	// workstream to the report and let them cascade rather than posting
	// into their sub-team yourself (one-hop rule).
	Manages    bool    `json:"manages"`
	TeamChatID *string `json:"teamChatId,omitempty"`
}

type reportsResult struct {
	// TeamChatID is the broadcast channel for all your direct reports;
	// null until you have at least one report.
	TeamChatID *string      `json:"teamChatId"`
	Reports    []reportView `json:"reports"`
}

func (t *Reports) Name() tool.Name                 { return ReportsName }
func (t *Reports) InputSchema() *jsonschema.Schema { return reportsSchema }
func (t *Reports) Description() string {
	return "List who reports to you and how to reach them — your delegation surface. " +
		"Takes no arguments; resolves your own reporting lines live. Returns " +
		"`teamChatId` (call `chat` there once to brief ALL your reports — one message, " +
		"not N DMs; null until you have reports) and a `reports` array, each with the " +
		"DM channel for a 1:1. A report flagged `manages: true` leads their own " +
		"sub-team: delegate the workstream to them and let them cascade — don't send " +
		"into their sub-team yourself."
}

func (t *Reports) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("reports: caller has no OrgID")
	}
	caller, err := SubjectForCaller(ctx, inv.Caller)
	if err != nil {
		return nil, fmt.Errorf("reports: %w", err)
	}
	result := reportsResult{Reports: []reportView{}}
	if !t.deps.Queries.ReportingLinesWired() {
		return json.Marshal(result)
	}
	reportIDs, err := t.deps.Queries.ListReports(ctx, orgID, caller)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	for _, r := range reportIDs {
		b, err := t.deps.Queries.GetBot(ctx, orgID, r)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("get report %q: %w", r, err)
		}
		subReports, err := t.deps.Queries.ListReports(ctx, orgID, r)
		if err != nil {
			return nil, fmt.Errorf("list sub-reports of %q: %w", r, err)
		}
		view := reportView{
			ID:          b.ID,
			DMChannelID: channels.DMTriggerID(caller, r),
			Manages:     len(subReports) > 0,
		}
		if view.Manages {
			ts := channels.TeamTriggerID(r)
			view.TeamChatID = &ts
		}
		result.Reports = append(result.Reports, view)
	}
	// Only advertise a team topic once there is someone to brief.
	if len(result.Reports) > 0 {
		ts := channels.TeamTriggerID(caller)
		result.TeamChatID = &ts
	}
	return json.Marshal(result)
}
