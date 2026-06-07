package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/topology"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// DM sends a direct message to a single other Worker. It bundles the
// underlying primitives — get-or-create a per-pair Stream, subscribe
// both parties, publish the body — into one Tool the agent can reach
// for from a "DM the fact-checker..." style instruction without having
// to chain four separate calls.
//
// The Stream ID is deterministic from the sorted (sender, recipient)
// pair, so subsequent DMs in either direction land on the same Stream
// and the back-and-forth stays ordered in one place.
type DM struct {
	deps Deps
}

const DMName tool.Name = "dm"

var dmSchema = mustSchema[dmArgs]()

func (t *DM) Name() tool.Name { return DMName }
func (t *DM) Description() string {
	return "Send a direct message (DM/PM/private message) to a single other Worker. " +
		"Reach for this whenever the user says to DM/message/ping a named colleague. " +
		"Transparently creates a per-pair Stream the first time, subscribes both " +
		"parties, and publishes the body; subsequent DMs to the same Worker reuse " +
		"the same Stream so the conversation stays in one ordered place. Use " +
		"list_workers first if you need to look up the recipient's ID. Returns the " +
		"streamId — read_events on it to wait for a reply."
}
func (t *DM) InputSchema() *jsonschema.Schema { return dmSchema }

type dmArgs struct {
	ToWorkerID string `json:"toWorkerId"`
	Body       string `json:"body"`
}

func (t *DM) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args dmArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ToWorkerID == "" || args.Body == "" {
		return nil, fmt.Errorf("toWorkerId and body are required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("dm: caller has no OrgID")
	}
	sender := inv.Caller.ID()
	recipient := orgchart.WorkerID(args.ToWorkerID)
	if sender == recipient {
		return nil, fmt.Errorf("cannot DM yourself")
	}
	if _, err := t.deps.Store.Workers.Get(ctx, orgID, recipient); err != nil {
		return nil, fmt.Errorf("recipient %q: %w", recipient, err)
	}

	streamID := DMStreamID(sender, recipient)

	// Get-or-create the per-pair Stream and subscribe both parties via
	// the shared topology primitive — one implementation of
	// "ensure stream exists with member set", used here and by the
	// reconciler. Subscriptions are worker-anchored.
	name := fmt.Sprintf("dm: %s ↔ %s", sender, recipient)
	stream, err := streaming.NewStream(streamID, name, "", sender, t.deps.Now(), transport.Transport{}, orgID)
	if err != nil {
		return nil, err
	}
	if err := topology.EnsureStreamWithMembers(ctx, t.deps.Store, stream, t.deps.Now(), sender, recipient); err != nil {
		return nil, err
	}

	msg := streaming.Message{
		From: string(sender),
		To:   []string{string(recipient)},
		Body: args.Body,
	}
	event, err := streaming.NewMessageEvent(
		streaming.EventID("e-"+t.deps.NewID()),
		streamID,
		sender,
		msg,
		t.deps.Now(),
		orgID,
	)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Events.Append(ctx, event); err != nil {
		return nil, err
	}
	if t.deps.Hub != nil {
		t.deps.Hub.Notify(streamID)
	}
	if t.deps.Dispatcher != nil {
		t.deps.Dispatcher.Dispatch(ctx, event)
	}

	return json.Marshal(map[string]string{
		"id":       string(event.ID),
		"streamId": string(streamID),
		"to":       string(recipient),
	})
}

// DMStreamID returns the deterministic Stream ID for a DM between two
// Workers, ordered by string compare so A→B and B→A share one Stream.
// Exported so the managers / reports read tools can hand back the DM
// stream id before the stream exists — the first dm lazily creates it.
func DMStreamID(a, b orgchart.WorkerID) streaming.StreamID {
	pair := []string{string(a), string(b)}
	sort.Strings(pair)
	return streaming.StreamID("s-dm-" + pair[0] + "-" + pair[1])
}
