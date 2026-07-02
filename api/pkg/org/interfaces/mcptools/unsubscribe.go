package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Unsubscribe removes a Bot's subscription to one or more Topics. Pass
// the target bot's id and the topic ids; idempotent per topic.
type Unsubscribe struct {
	deps Deps
}

const UnsubscribeName tool.Name = "unsubscribe"

var unsubscribeSchema = mustSchema[unsubscribeArgs]()

func (t *Unsubscribe) Name() tool.Name { return UnsubscribeName }
func (t *Unsubscribe) Description() string {
	return "Unsubscribe a Bot from one or more Topics. Pass `botId` and `topicIds` as an " +
		"array of topic ids. Idempotent per topic (a topic the Bot isn't subscribed to is " +
		"a no-op). Use subscribe to add."
}
func (t *Unsubscribe) InputSchema() *jsonschema.Schema {
	return withProperty(unsubscribeSchema, "topicIds",
		stringArrayProperty("Topic ids to unsubscribe the Bot from (one or many)."))
}

type unsubscribeArgs struct {
	BotID    string   `json:"botId"`
	TopicIDs []string `json:"topicIds"`
}

func (t *Unsubscribe) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args unsubscribeArgs
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
		return nil, fmt.Errorf("unsubscribe: caller has no OrgID")
	}
	botID := orgchart.BotID(args.BotID)
	if err := t.deps.Subscriptions.UnsubscribeTopics(ctx, orgID, botID, args.TopicIDs); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"botId": string(botID), "topicIds": args.TopicIDs})
}
