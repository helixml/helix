package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Subscribe adds a Subscription between the caller and the given
// Stream. A Worker subscribes themselves; see invite_workers for
// adding other Workers to a Stream.
type Subscribe struct {
	deps Deps
}

const SubscribeName tool.Name = "subscribe"

var subscribeSchema = mustSchema[subscribeArgs]()

func (t *Subscribe) Name() tool.Name { return SubscribeName }
func (t *Subscribe) Description() string {
	return "Subscribe the calling Worker to a Stream. Idempotent: a no-op if already subscribed. " +
		"Subscriptions are per-Worker: firing this Worker drops the subscription, and a new " +
		"hire into the same Role does not automatically inherit it."
}
func (t *Subscribe) InputSchema() *jsonschema.Schema { return subscribeSchema }

type subscribeArgs struct {
	StreamID string `json:"streamId"`
}

func (t *Subscribe) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args subscribeArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" {
		return nil, fmt.Errorf("streamId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("subscribe: caller has no OrgID")
	}
	streamID := streaming.StreamID(args.StreamID)
	workerID := inv.Caller.ID()
	if _, _, err := t.deps.subscriptionsService().Subscribe(ctx, orgID, workerID, streamID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"workerId": string(workerID), "streamId": string(streamID)})
}
