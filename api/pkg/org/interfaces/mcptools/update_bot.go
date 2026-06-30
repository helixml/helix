package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// UpdateBot rewrites the mutable fields of a Bot — its markdown content
// and/or its MCP tool list. It is the merge of the former update_role
// (the job description) and update_identity (the per-hire persona): now
// that a Bot IS its own job description, there is one content to edit.
//
// It is a single DB write — the new content/tools take effect on the
// bot's next activation, when the Spawner projects current bot state
// into the Environment. A content-only patch preserves Tools and Topics
// (the service does a read-modify-write); a tools change propagates to
// the bot on its next MCP request.
//
// Bots can never modify their own definition — only the owner does, via
// this tool.
type UpdateBot struct {
	deps Deps
}

const UpdateBotName tool.Name = "update_bot"

var updateBotSchema = mustSchema[updateBotArgs]()

func (t *UpdateBot) Name() tool.Name                 { return UpdateBotName }
func (t *UpdateBot) InputSchema() *jsonschema.Schema { return updateBotSchema }
func (t *UpdateBot) Description() string {
	return "Update a Bot's markdown content and/or its MCP tool list. A content-only " +
		"patch preserves the bot's tools and topics. The change takes effect on the " +
		"bot's next activation, when the Spawner projects current bot state into its " +
		"Environment. Owner-only."
}

type updateBotArgs struct {
	ID      string      `json:"id"`
	Content string      `json:"content,omitempty"`
	Tools   []tool.Name `json:"tools,omitempty"`
}

func (t *UpdateBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args updateBotArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("update_bot: caller has no OrgID")
	}
	botID := orgchart.BotID(args.ID)

	// Build a patch: nil pointers leave the corresponding field
	// unchanged, so a content-only call keeps Tools/Topics intact.
	var params bots.UpdateParams
	if args.Content != "" {
		params.Content = &args.Content
	}
	if args.Tools != nil {
		params.Tools = &args.Tools
	}
	updated, err := t.deps.Bots.Update(ctx, orgID, botID, params)
	if err != nil {
		return nil, fmt.Errorf("update bot: %w", err)
	}

	// Mirror the new content into the bot's Environment so a running
	// session sees it without waiting for the next activation. This is a
	// workspace side-effect, not store state, so it stays in the MCP
	// adapter (the REST chart UI doesn't need it — the Spawner re-projects
	// current bot state at the start of every activation).
	_ = t.deps.Workspace.MirrorFile(ctx, orgID, botID, "role.md", updated.Content, fmt.Sprintf("update_bot: %s", botID))

	return json.Marshal(map[string]string{"id": string(botID)})
}
