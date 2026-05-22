package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
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

func (t *DM) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args dmArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ToWorkerID == "" || args.Body == "" {
		return nil, fmt.Errorf("toWorkerId and body are required")
	}
	sender := inv.Caller.ID()
	recipient := worker.ID(args.ToWorkerID)
	if sender == recipient {
		return nil, fmt.Errorf("cannot DM yourself")
	}
	if _, err := t.deps.Store.Workers.Get(ctx, recipient); err != nil {
		return nil, fmt.Errorf("recipient %q: %w", recipient, err)
	}

	streamID := dmStreamID(sender, recipient)

	// Get-or-create the per-pair Stream. Reuse it across DMs so the
	// conversation stays ordered in one place.
	if _, err := t.deps.Store.Streams.Get(ctx, streamID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("lookup stream %q: %w", streamID, err)
		}
		name := fmt.Sprintf("dm: %s ↔ %s", sender, recipient)
		s, err := domain.NewStream(streamID, name, "", sender, t.deps.Now(), transport.Transport{})
		if err != nil {
			return nil, err
		}
		if err := t.deps.Store.Streams.Create(ctx, s); err != nil {
			return nil, fmt.Errorf("create stream %q: %w", streamID, err)
		}
	}

	// Make sure both parties are subscribed (idempotent). The recipient
	// might have unsubscribed since the last DM; re-subscribe them so
	// the message actually reaches them.
	for _, wid := range []worker.ID{sender, recipient} {
		if _, err := t.deps.Store.Subscriptions.Find(ctx, wid, streamID); err == nil {
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		sub, err := domain.NewSubscription(wid, streamID, t.deps.Now())
		if err != nil {
			return nil, err
		}
		if err := t.deps.Store.Subscriptions.Create(ctx, sub); err != nil {
			return nil, err
		}
	}

	msg := message.Message{
		From: string(sender),
		To:   []string{string(recipient)},
		Body: args.Body,
	}
	event, err := domain.NewMessageEvent(
		event.ID("e-"+t.deps.NewID()),
		streamID,
		sender,
		msg,
		t.deps.Now(),
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

// dmStreamID returns the deterministic Stream ID for a DM between two
// Workers, ordered by string compare so A→B and B→A share one Stream.
func dmStreamID(a, b worker.ID) stream.ID {
	pair := []string{string(a), string(b)}
	sort.Strings(pair)
	return stream.ID("s-dm-" + pair[0] + "-" + pair[1])
}
