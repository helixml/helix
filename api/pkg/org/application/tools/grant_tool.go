package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// GrantTool creates a new ToolGrant for a Worker — boolean permission
// to call the named tool. Owner-only. Granularity comes from the
// design of tools; there is no per-grant scope.
type GrantTool struct {
	deps Deps
}

const GrantToolName tool.Name = "grant_tool"

var grantToolSchema = mustSchema[grantToolArgs]()

func (t *GrantTool) Name() tool.Name                 { return GrantToolName }
func (t *GrantTool) InputSchema() *jsonschema.Schema { return grantToolSchema }
func (t *GrantTool) Description() string {
	return "Grant a tool to a Worker. The grant is a boolean permission — holding it lets the " +
		"Worker call that tool however the tool's input schema allows."
}

type grantToolArgs struct {
	ID       string `json:"id,omitempty"`
	WorkerID string `json:"workerId"`
	ToolName string `json:"toolName"`
}

func (t *GrantTool) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args grantToolArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("grant_tool: caller has no OrgID")
	}
	if _, err := t.deps.Store.Workers.Get(ctx, orgID, orgchart.WorkerID(args.WorkerID)); err != nil {
		return nil, fmt.Errorf("worker %q: %w", args.WorkerID, err)
	}
	id := orgchart.GrantID(args.ID)
	if id == "" {
		id = orgchart.GrantID("g-" + t.deps.NewID())
	}
	g, err := orgchart.NewToolGrant(id, orgchart.WorkerID(args.WorkerID), tool.Name(args.ToolName), orgID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Grants.Create(ctx, g); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
