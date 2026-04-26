package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// Publish appends an Event to a named Stream, attributed to the caller.
// It does exactly one thing: append an event to an existing Stream. It
// does not create Streams, manage subscriptions, or implement DM sugar;
// callers who want to direct-message another Worker are expected to
// create_stream and subscribe themselves, then publish.
type Publish struct {
	deps Deps
}

const PublishName domain.ToolName = "publish"

var publishSchema = mustSchema[publishArgs]()

func (t *Publish) Name() domain.ToolName { return PublishName }
func (t *Publish) Description() string {
	return "Append an Event with the given body to a Stream. Wakes long-poll observers and " +
		"activates every subscribed AI Worker."
}
func (t *Publish) InputSchema() *jsonschema.Schema { return publishSchema }

type publishArgs struct {
	StreamID string `json:"streamId"`
	Body     string `json:"body"`
}

func (t *Publish) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args publishArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" || args.Body == "" {
		return nil, fmt.Errorf("streamId and body are required")
	}
	streamID := domain.StreamID(args.StreamID)
	if _, err := t.deps.Store.Streams.Get(ctx, streamID); err != nil {
		return nil, fmt.Errorf("stream %q: %w", streamID, err)
	}
	event, err := domain.NewEvent(
		domain.EventID("e-"+t.deps.NewID()),
		streamID,
		inv.Caller.ID(),
		args.Body,
		t.deps.Now(),
	)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Events.Append(ctx, event); err != nil {
		return nil, err
	}
	// Wake HTTP long-poll observers (humans curling /tail).
	if t.deps.Broadcaster != nil {
		t.deps.Broadcaster.Notify(streamID)
	}
	// Activate every subscribed AI Worker. Background; returns immediately.
	if t.deps.Dispatcher != nil {
		t.deps.Dispatcher.Dispatch(ctx, event)
	}
	return json.Marshal(map[string]string{"id": string(event.ID), "streamId": string(streamID)})
}
