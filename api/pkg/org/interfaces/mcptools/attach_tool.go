package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// AttachTool grants one or more tools to a Bot — the counterpart to
// detach_tool. Tools are the Bot's live MCP surface (Bot.Tools); the
// change takes effect on the Bot's next MCP request. Owner-only.
type AttachTool struct {
	deps Deps
}

const AttachToolName tool.Name = "attach_tool"

// attachToolSchema is the reflected base (object shape + required, since
// neither field is omitempty). InputSchema swaps in the dynamic `tools`
// enum at serve time.
var attachToolSchema = mustSchema[attachToolArgs]()

type attachToolArgs struct {
	NodeID string   `json:"botId"`
	Tools  []string `json:"tools"`
}

func (t *AttachTool) Name() tool.Name { return AttachToolName }
func (t *AttachTool) Description() string {
	return "Grant MCP tools to a Bot. Pass `tools` as an array of tool names (one or " +
		"many) chosen from the enum. Idempotent: names the Bot already has are ignored. " +
		"The change takes effect on the Bot's next MCP request. Use detach_tool to remove."
}
func (t *AttachTool) InputSchema() *jsonschema.Schema {
	return withProperty(attachToolSchema, "tools",
		enumStringArrayProperty(t.deps.ToolNames(), "MCP tool names to grant to the Bot (one or many)."))
}

func (t *AttachTool) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args attachToolArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.NodeID == "" {
		return nil, fmt.Errorf("botId is required")
	}
	if len(args.Tools) == 0 {
		return nil, fmt.Errorf("tools must contain at least one tool name")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("attach_tool: caller has no OrgID")
	}
	updated, err := t.deps.Nodes.AttachTools(ctx, orgID, orgchart.NodeID(args.NodeID), args.Tools)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"id": string(updated.ID), "tools": updated.Tools})
}
