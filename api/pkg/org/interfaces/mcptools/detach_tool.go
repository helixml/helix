package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// DetachTool removes one or more tools from a Bot — the counterpart to
// attach_tool. It refuses to remove a universal read-baseline tool (the
// service enforces this): those are mandatory and would be re-added by
// the reconciler anyway. Owner-only.
type DetachTool struct {
	deps Deps
}

const DetachToolName tool.Name = "detach_tool"

var detachToolSchema = mustSchema[detachToolArgs]()

type detachToolArgs struct {
	BotID string   `json:"botId"`
	Tools []string `json:"tools"`
}

func (t *DetachTool) Name() tool.Name { return DetachToolName }
func (t *DetachTool) Description() string {
	return "Remove MCP tools from a Bot. Pass `tools` as an array of tool names (one or " +
		"many) chosen from the enum. Idempotent: names the Bot lacks are ignored. " +
		"Universal read-baseline tools cannot be removed. Use attach_tool to grant."
}
func (t *DetachTool) InputSchema() *jsonschema.Schema {
	return withProperty(detachToolSchema, "tools",
		enumStringArrayProperty(t.deps.ToolNames(), "MCP tool names to remove from the Bot (one or many)."))
}

func (t *DetachTool) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args detachToolArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, fmt.Errorf("botId is required")
	}
	if len(args.Tools) == 0 {
		return nil, fmt.Errorf("tools must contain at least one tool name")
	}
	if err := validateRegisteredTools(args.Tools, t.deps.ToolNames); err != nil {
		return nil, err
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("detach_tool: caller has no OrgID")
	}
	updated, err := t.deps.Bots.DetachTools(ctx, orgID, orgchart.BotID(args.BotID), args.Tools)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"id": string(updated.ID), "tools": updated.Tools})
}
