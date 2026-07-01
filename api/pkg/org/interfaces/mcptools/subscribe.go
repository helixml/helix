package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Subscribe subscribes a Bot to one or more Topics. Pass the target
// bot's id and the topic ids; a Bot subscribes itself by passing its own
// id. Reuses the same subscription use case create_bot drives at creation.
type Subscribe struct {
	deps Deps
}

const SubscribeName tool.Name = "subscribe"

var subscribeSchema = mustSchema[subscribeArgs]()

func (t *Subscribe) Name() tool.Name { return SubscribeName }
func (t *Subscribe) Description() string {
	return "Subscribe a Bot to one or more Topics. Pass `botId` (the bot to subscribe — " +
		"pass your own id to subscribe yourself) and `topicIds` as an array of existing " +
		"topic ids. Idempotent per topic. Use unsubscribe to remove."
}
func (t *Subscribe) InputSchema() *jsonschema.Schema {
	return withProperty(subscribeSchema, "topicIds",
		stringArrayProperty("Existing Topic ids to subscribe the Bot to (one or many)."))
}

type subscribeArgs struct {
	BotID    string   `json:"botId"`
	TopicIDs []string `json:"topicIds"`
}

func (t *Subscribe) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args subscribeArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, fmt.Errorf("botId is required")
	}
	if len(args.TopicIDs) == 0 {
		return nil, fmt.Errorf("topicIds must contain at least one topic id")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("subscribe: caller has no OrgID")
	}
	botID := orgchart.BotID(args.BotID)
	if err := t.deps.Subscriptions.SubscribeTopics(ctx, orgID, botID, args.TopicIDs); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"botId": string(botID), "topicIds": args.TopicIDs})
}
