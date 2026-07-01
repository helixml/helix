package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// DeleteBot tears a Bot down via the lifecycle service — the same use
// case the REST DELETE /bots/{id} handler drives. It cascades: stops the
// Bot's sessions, deletes its Helix project + agent app, clears runtime
// state, drops its subscriptions and reporting lines, deletes the bot
// row, and reconciles team/DM topics. Activations are preserved as audit.
// Owner-only.
type DeleteBot struct {
	deps Deps
}

const DeleteBotName tool.Name = "delete_bot"

var deleteBotSchema = mustSchema[deleteBotArgs]()

type deleteBotArgs struct {
	BotID string `json:"botId"`
}

func (t *DeleteBot) Name() tool.Name                 { return DeleteBotName }
func (t *DeleteBot) InputSchema() *jsonschema.Schema { return deleteBotSchema }
func (t *DeleteBot) Description() string {
	return "Delete a Bot. Cascades: stops its sessions, deletes its Helix project + agent " +
		"app, clears runtime state, drops its subscriptions and reporting lines, then the " +
		"bot row. Bots that reported to it become parentless. Owner-only."
}

func (t *DeleteBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args deleteBotArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, fmt.Errorf("botId is required")
	}
	if t.deps.Lifecycle == nil {
		return nil, fmt.Errorf("delete_bot: lifecycle service not wired")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("delete_bot: caller has no OrgID")
	}
	botID := orgchart.BotID(args.BotID)
	if err := t.deps.Lifecycle.Delete(ctx, orgID, botID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(botID)})
}
