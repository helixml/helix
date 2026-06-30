package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// InviteBots subscribes one or more other Bots to a Topic. The
// counterpart to subscribe (which is self-only) — used to add others to
// a topic you've created, e.g. opening a DM by creating a topic and
// inviting both parties to it.
type InviteBots struct {
	deps Deps
}

const InviteBotsName tool.Name = "invite_bots"

var inviteBotsSchema = mustSchema[inviteBotsArgs]()

func (t *InviteBots) Name() tool.Name { return InviteBotsName }
func (t *InviteBots) Description() string {
	return "Subscribe one or more Bots to a Topic. Use this to add others " +
		"to a topic you control — e.g. opening a DM by creating a topic and " +
		"inviting both parties, or pulling a colleague into an existing thread. " +
		"Idempotent per bot: anyone already subscribed is a no-op."
}
func (t *InviteBots) InputSchema() *jsonschema.Schema { return inviteBotsSchema }

type inviteBotsArgs struct {
	TopicID string   `json:"topicId"`
	BotIDs  []string `json:"botIds"`
}

func (t *InviteBots) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args inviteBotsArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TopicID == "" {
		return nil, fmt.Errorf("topicId is required")
	}
	if len(args.BotIDs) == 0 {
		return nil, fmt.Errorf("botIds must contain at least one bot")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("invite_bots: caller has no OrgID")
	}
	topicID := streaming.TopicID(args.TopicID)
	botIDs := make([]orgchart.BotID, 0, len(args.BotIDs))
	for _, raw := range args.BotIDs {
		botIDs = append(botIDs, orgchart.BotID(raw))
	}
	// The service validates the topic + every bot up front and
	// subscribes each (idempotent per bot).
	if err := t.deps.Subscriptions.Invite(ctx, orgID, topicID, botIDs); err != nil {
		return nil, err
	}

	botIDStrings := make([]string, len(botIDs))
	for i, bid := range botIDs {
		botIDStrings[i] = string(bid)
	}
	return json.Marshal(map[string]any{
		"topicId": string(topicID),
		"botIds":  botIDStrings,
	})
}
