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

// SetBotContent rewrites a Bot's markdown content (its prompt). Tools and
// subscriptions are untouched — use attach_tool/detach_tool for tools and
// subscribe/unsubscribe for streams. The change takes effect on the Bot's
// next activation; the running session sees it via the workspace mirror.
// Owner-only.
type SetBotContent struct {
	deps Deps
}

const SetBotContentName tool.Name = "set_bot_content"

var setBotContentSchema = mustSchema[setBotContentArgs]()

type setBotContentArgs struct {
	BotID   string `json:"botId"`
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
	if args.BotID == "" {
		return nil, fmt.Errorf("botId is required")
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("set_bot_content: caller has no OrgID")
	}
	botID := orgchart.BotID(args.BotID)
	updated, err := t.deps.Bots.Update(ctx, orgID, botID, bots.UpdateParams{Content: &args.Content})
	if err != nil {
		return nil, fmt.Errorf("set bot content: %w", err)
	}
	// Mirror the new content into the bot's Environment so a running
	// session sees it without waiting for the next activation.
	_ = t.deps.Workspace.MirrorFile(ctx, orgID, botID, "role.md", updated.Content, fmt.Sprintf("set_bot_content: %s", botID))
	return json.Marshal(map[string]string{"id": string(botID)})
}
