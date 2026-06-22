package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Subscribe adds a Subscription between the caller and the given
// Topic. A Worker subscribes themselves; see invite_workers for
// adding other Workers to a Topic.
type Subscribe struct {
	deps Deps
}

const SubscribeName tool.Name = "subscribe"

var subscribeSchema = mustSchema[subscribeArgs]()

func (t *Subscribe) Name() tool.Name { return SubscribeName }
func (t *Subscribe) Description() string {
	return "Subscribe the calling Worker to a Topic. Idempotent: a no-op if already subscribed. " +
		"Subscriptions are per-Worker: firing this Worker drops the subscription, and a new " +
		"hire into the same Role does not automatically inherit it."
}
func (t *Subscribe) InputSchema() *jsonschema.Schema { return subscribeSchema }

type subscribeArgs struct {
	TopicID string `json:"topicId"`
}

func (t *Subscribe) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args subscribeArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TopicID == "" {
		return nil, fmt.Errorf("topicId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("subscribe: caller has no OrgID")
	}
	topicID := streaming.TopicID(args.TopicID)
	workerID := inv.Caller.ID()
	if _, _, err := t.deps.Subscriptions.Subscribe(ctx, orgID, workerID, topicID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"workerId": string(workerID), "topicId": string(topicID)})
}
