package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/topology"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Reports is the downward, delegation-direction read: who reports to
// you, and every channel to reach them — the team stream to brief them
// all at once, plus each report's DM stream for 1:1. Computed live from
// the reporting graph so the fixed worker-policy prompt can refer to
// "your reports" / "your team" abstractly.
type Reports struct {
	deps Deps
}

const ReportsName tool.Name = "reports"

var reportsSchema = mustSchema[reportsArgs]()

type reportsArgs struct{}

type reportView struct {
	ID         orgchart.WorkerID  `json:"id"`
	Role       orgchart.RoleID    `json:"role"`
	DMStreamID streaming.StreamID `json:"dmStreamId"`
	// Manages is true when this report leads their own sub-team. When
	// true, TeamStreamID is shown for context — but you delegate the
	// workstream to the report and let them cascade rather than posting
	// into their sub-team yourself (one-hop rule).
	Manages      bool                `json:"manages"`
	TeamStreamID *streaming.StreamID `json:"teamStreamId,omitempty"`
}

type reportsResult struct {
	// TeamStreamID is the broadcast channel for all your direct reports;
	// null until you have at least one report.
	TeamStreamID *streaming.StreamID `json:"teamStreamId"`
	Reports      []reportView        `json:"reports"`
}

func (t *Reports) Name() tool.Name                 { return ReportsName }
func (t *Reports) InputSchema() *jsonschema.Schema { return reportsSchema }
func (t *Reports) Description() string {
	return "List who reports to you and how to reach them — your delegation surface. " +
		"Takes no arguments; resolves your own reporting lines live. Returns " +
		"`teamStreamId` (publish there once to brief ALL your reports — one post, " +
		"not N DMs; null until you have reports) and a `reports` array, each with the " +
		"DM stream for a 1:1. A report flagged `manages: true` leads their own " +
		"sub-team: delegate the workstream to them and let them cascade — don't post " +
		"into their sub-team yourself."
}

func (t *Reports) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("reports: caller has no OrgID")
	}
	caller := orgchart.WorkerID(inv.Caller.ID())
	result := reportsResult{Reports: []reportView{}}
	if t.deps.Store.ReportingLines == nil {
		return json.Marshal(result)
	}
	reportIDs, err := t.deps.Store.ReportingLines.ListReports(ctx, orgID, caller)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	for _, r := range reportIDs {
		w, err := t.deps.Store.Workers.Get(ctx, orgID, r)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("get report %q: %w", r, err)
		}
		subReports, err := t.deps.Store.ReportingLines.ListReports(ctx, orgID, r)
		if err != nil {
			return nil, fmt.Errorf("list sub-reports of %q: %w", r, err)
		}
		view := reportView{
			ID:         w.ID(),
			Role:       w.RoleID(),
			DMStreamID: topology.DMStreamID(caller, r),
			Manages:    len(subReports) > 0,
		}
		if view.Manages {
			ts := topology.TeamStreamID(r)
			view.TeamStreamID = &ts
		}
		result.Reports = append(result.Reports, view)
	}
	// Only advertise a team stream once there is someone to brief.
	if len(result.Reports) > 0 {
		ts := topology.TeamStreamID(caller)
		result.TeamStreamID = &ts
	}
	return json.Marshal(result)
}
