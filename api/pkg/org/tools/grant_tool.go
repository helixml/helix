package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
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

func (t *GrantTool) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args grantToolArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if _, err := t.deps.Store.Workers.Get(ctx, worker.ID(args.WorkerID)); err != nil {
		return nil, fmt.Errorf("worker %q: %w", args.WorkerID, err)
	}
	id := grant.ID(args.ID)
	if id == "" {
		id = grant.ID("g-" + t.deps.NewID())
	}
	grant, err := domain.NewToolGrant(id, worker.ID(args.WorkerID), tool.Name(args.ToolName))
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Grants.Create(ctx, grant); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(id)})
}
