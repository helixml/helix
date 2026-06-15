package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Unsubscribe removes the caller's Subscription from the given Stream.
type Unsubscribe struct {
	deps Deps
}

const UnsubscribeName tool.Name = "unsubscribe"

var unsubscribeSchema = mustSchema[unsubscribeArgs]()

func (t *Unsubscribe) Name() tool.Name                 { return UnsubscribeName }
func (t *Unsubscribe) Description() string             { return "Unsubscribe the calling Worker from a Stream." }
func (t *Unsubscribe) InputSchema() *jsonschema.Schema { return unsubscribeSchema }

type unsubscribeArgs struct {
	StreamID string `json:"streamId"`
}

func (t *Unsubscribe) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args unsubscribeArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" {
		return nil, fmt.Errorf("streamId is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("unsubscribe: caller has no OrgID")
	}
	streamID := streaming.StreamID(args.StreamID)
	workerID := inv.Caller.ID()
	if err := t.deps.Store.Subscriptions.Delete(ctx, orgID, workerID, streamID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"workerId": string(workerID), "streamId": string(streamID)})
}
