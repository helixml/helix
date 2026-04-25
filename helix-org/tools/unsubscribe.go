package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// Unsubscribe removes the caller's Stream for the given Channel.
type Unsubscribe struct {
	deps Deps
}

const UnsubscribeName domain.ToolName = "unsubscribe"

var unsubscribeSchema = mustSchema[unsubscribeArgs]()

func (t *Unsubscribe) Name() domain.ToolName           { return UnsubscribeName }
func (t *Unsubscribe) Description() string             { return "Unsubscribe the calling Worker from a Channel." }
func (t *Unsubscribe) InputSchema() *jsonschema.Schema { return unsubscribeSchema }

type unsubscribeArgs struct {
	ChannelID string `json:"channelId"`
}

func (t *Unsubscribe) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args unsubscribeArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ChannelID == "" {
		return nil, fmt.Errorf("channelId is required")
	}
	channelID := domain.ChannelID(args.ChannelID)
	stream, err := t.deps.Store.Streams.FindForWorkerAndChannel(ctx, inv.Caller.ID(), channelID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Streams.Delete(ctx, stream.ID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": string(stream.ID)})
}
