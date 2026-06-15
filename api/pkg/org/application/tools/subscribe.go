package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/store"
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
	if _, err := t.deps.Store.Streams.Get(ctx, orgID, streamID); err != nil {
		return nil, fmt.Errorf("stream %q: %w", streamID, err)
	}

	workerID := inv.Caller.ID()
	if _, err := t.deps.Store.Workers.Get(ctx, orgID, workerID); err != nil {
		return nil, fmt.Errorf("get caller worker %q: %w", workerID, err)
	}
	if _, err := t.deps.Store.Subscriptions.Find(ctx, orgID, workerID, streamID); err == nil {
		return json.Marshal(map[string]string{"workerId": string(workerID), "streamId": string(streamID)})
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	sub, err := streaming.NewSubscription(string(workerID), streamID, t.deps.Now(), orgID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Subscriptions.Create(ctx, sub); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"workerId": string(workerID), "streamId": string(streamID)})
}
