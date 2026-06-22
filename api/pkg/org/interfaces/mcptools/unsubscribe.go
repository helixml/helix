package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Unsubscribe removes the caller's Subscription from the given Topic.
type Unsubscribe struct {
	deps Deps
}

const UnsubscribeName tool.Name = "unsubscribe"

var unsubscribeSchema = mustSchema[unsubscribeArgs]()

func (t *Unsubscribe) Name() tool.Name                 { return UnsubscribeName }
func (t *Unsubscribe) Description() string             { return "Unsubscribe the calling Worker from a Topic." }
func (t *Unsubscribe) InputSchema() *jsonschema.Schema { return unsubscribeSchema }

type unsubscribeArgs struct {
	TopicID string `json:"topicId"`
}

func (t *Unsubscribe) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args unsubscribeArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.TopicID == "" {
		return nil, fmt.Errorf("topicId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("unsubscribe: caller has no OrgID")
	}
	topicID := streaming.TopicID(args.TopicID)
	workerID := inv.Caller.ID()
	if err := t.deps.Subscriptions.Unsubscribe(ctx, orgID, workerID, topicID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"workerId": string(workerID), "topicId": string(topicID)})
}
