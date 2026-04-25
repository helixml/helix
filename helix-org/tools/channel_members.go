package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// ChannelMembers returns the Worker IDs subscribed to a Channel right
// now. Read-only and non-blocking — the manager-style use case is "is
// the worker I'm about to message actually listening?". Composes with
// any outstanding-task tracking the caller does: see who's listening,
// and if the right party isn't, defer the work and reconcile later.
type ChannelMembers struct {
	deps Deps
}

const ChannelMembersName domain.ToolName = "channel_members"

var channelMembersSchema = mustSchema[channelMembersArgs]()

func (t *ChannelMembers) Name() domain.ToolName           { return ChannelMembersName }
func (t *ChannelMembers) InputSchema() *jsonschema.Schema { return channelMembersSchema }
func (t *ChannelMembers) Description() string {
	return "List the Worker IDs currently subscribed to a Channel. Returns immediately. " +
		"Use this before publishing if you need to know whether a particular Worker is listening — " +
		"e.g. before sending the first recruiting brief, check that the recruiter is subscribed."
}

type channelMembersArgs struct {
	ChannelID string `json:"channelId"`
}

func (t *ChannelMembers) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args channelMembersArgs
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
	streams, err := t.deps.Store.Streams.ListForChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	members := make([]domain.WorkerID, 0, len(streams))
	for _, s := range streams {
		members = append(members, s.WorkerID)
	}
	return json.Marshal(map[string]any{
		"channelId": string(channelID),
		"members":   members,
	})
}
