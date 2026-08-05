package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// SetBotContent rewrites a Bot's markdown content (its prompt). Tools and
// subscriptions are untouched — use attach_tool/detach_tool for tools and
// subscribe/unsubscribe for streams. The change takes effect on the Bot's
// next activation, when the spawner refreshes the session-scoped profile.
// Owner-only.
type SetBotContent struct {
	deps Deps
}

const SetBotContentName tool.Name = "set_bot_content"

var setBotContentSchema = mustSchema[setBotContentArgs]()

type setBotContentArgs struct {
	NodeID  string `json:"botId"`
	Content string `json:"content"`
}

func (t *SetBotContent) Name() tool.Name                 { return SetBotContentName }
func (t *SetBotContent) InputSchema() *jsonschema.Schema { return setBotContentSchema }
func (t *SetBotContent) Description() string {
	return "Replace a Bot's markdown content (its prompt). Tools and subscriptions are " +
		"left unchanged. Takes effect on the Bot's next activation. Owner-only."
}

func (t *SetBotContent) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args setBotContentArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.NodeID == "" {
		return nil, fmt.Errorf("botId is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("set_bot_content: caller has no OrgID")
	}
	botID := orgchart.NodeID(args.NodeID)
	existing, err := t.deps.Queries.GetBot(ctx, orgID, botID)
	if err != nil {
		return nil, fmt.Errorf("get bot: %w", err)
	}
	if existing.AgentID != "" && t.deps.AgentContentUpdater == nil {
		return nil, fmt.Errorf("update linked agent content: updater is not wired")
	}
	updated, err := t.deps.Nodes.Update(ctx, orgID, botID, nodes.UpdateParams{Content: &args.Content})
	if err != nil {
		return nil, fmt.Errorf("set bot content: %w", err)
	}
	if updated.AgentID != "" {
		if err := t.deps.AgentContentUpdater.UpdateAgentContent(ctx, updated.AgentID, args.Content); err != nil {
			_, rollbackErr := t.deps.Nodes.Update(ctx, orgID, botID, nodes.UpdateParams{Content: &existing.Content})
			if rollbackErr != nil {
				return nil, fmt.Errorf("update linked agent content: %v; rollback bot content: %w", err, rollbackErr)
			}
			return nil, fmt.Errorf("update linked agent content: %w", err)
		}
	}
	return json.Marshal(map[string]string{"id": string(botID)})
}
