package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

// StreamMembers returns the Worker IDs subscribed to a Stream right
// now. Read-only and non-blocking — the manager-style use case is "is
// the worker I'm about to message actually listening?". Composes with
// any outstanding-task tracking the caller does: see who's listening,
// and if the right party isn't, defer the work and reconcile later.
type StreamMembers struct {
	deps Deps
}

const StreamMembersName domain.ToolName = "stream_members"

var streamMembersSchema = mustSchema[streamMembersArgs]()

func (t *StreamMembers) Name() domain.ToolName           { return StreamMembersName }
func (t *StreamMembers) InputSchema() *jsonschema.Schema { return streamMembersSchema }
func (t *StreamMembers) Description() string {
	return "List the Worker IDs currently subscribed to a Stream. Returns immediately. " +
		"Use this before publishing if you need to know whether a particular Worker is listening — " +
		"e.g. before sending the first recruiting brief, check that the recruiter is subscribed."
}

type streamMembersArgs struct {
	StreamID string `json:"streamId"`
}

func (t *StreamMembers) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args streamMembersArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" {
		return nil, fmt.Errorf("streamId is required")
	}
	streamID := domain.StreamID(args.StreamID)
	if _, err := t.deps.Store.Streams.Get(ctx, streamID); err != nil {
		return nil, fmt.Errorf("stream %q: %w", streamID, err)
	}
	subs, err := t.deps.Store.Subscriptions.ListForStream(ctx, streamID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	members := make([]domain.WorkerID, 0, len(subs))
	for _, sub := range subs {
		members = append(members, sub.WorkerID)
	}
	return json.Marshal(map[string]any{
		"streamId": string(streamID),
		"members":  members,
	})
}
