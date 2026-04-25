package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

// Subscribe adds a Stream between the caller and the given Channel. A Worker
// subscribes themselves; subscribing other Workers is not an operation the
// tool layer exposes (default subscriptions flow through Roles at hire time).
type Subscribe struct {
	deps Deps
}

const SubscribeName domain.ToolName = "subscribe"

var subscribeSchema = mustSchema[subscribeArgs]()

func (t *Subscribe) Name() domain.ToolName { return SubscribeName }
func (t *Subscribe) Description() string {
	return "Subscribe the calling Worker to a Channel. Idempotent: returns the existing stream if already subscribed."
}
func (t *Subscribe) InputSchema() *jsonschema.Schema { return subscribeSchema }

type subscribeArgs struct {
	ChannelID string `json:"channelId"`
}

func (t *Subscribe) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args subscribeArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ChannelID == "" {
		return nil, fmt.Errorf("channelId is required")
	}
	channelID := domain.ChannelID(args.ChannelID)
	if _, err := t.deps.Store.Channels.Get(ctx, channelID); err != nil {
		return nil, fmt.Errorf("channel %q: %w", channelID, err)
	}

	// Idempotent: if already subscribed, return the existing stream.
	if existing, err := t.deps.Store.Streams.FindForWorkerAndChannel(ctx, inv.Caller.ID(), channelID); err == nil {
		return json.Marshal(map[string]string{"id": string(existing.ID)})
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	s, err := domain.NewStream(
		domain.StreamID("s-"+t.deps.NewID()),
		inv.Caller.ID(),
		channelID,
		t.deps.Now(),
	)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Streams.Create(ctx, s); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(s.ID)})
}
