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
	return "Subscribe the calling Worker to a Stream. Idempotent: a no-op if already subscribed."
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

	// Subscriptions are position-anchored: resolve the calling worker
	// to its position, then subscribe the position. The LLM still
	// talks in worker terms (the tool args mention "the calling
	// worker"); positions are an implementation detail that survives
	// firings.
	workerID := inv.Caller.ID()
	worker, err := t.deps.Store.Workers.Get(ctx, orgID, workerID)
	if err != nil {
		return nil, fmt.Errorf("get caller worker %q: %w", workerID, err)
	}
	positionID := worker.Position()
	if positionID == "" {
		return nil, fmt.Errorf("subscribe: caller worker %q is unassigned (no position)", workerID)
	}
	if _, err := t.deps.Store.Subscriptions.Find(ctx, orgID, positionID, streamID); err == nil {
		return json.Marshal(map[string]string{"workerId": string(workerID), "positionId": string(positionID), "streamId": string(streamID)})
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	sub, err := streaming.NewSubscription(string(positionID), streamID, t.deps.Now(), orgID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Subscriptions.Create(ctx, sub); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"workerId": string(workerID), "positionId": string(positionID), "streamId": string(streamID)})
}
