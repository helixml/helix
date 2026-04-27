package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
)

// Defaults and caps for read_events pagination and long-polling.
const (
	readEventsDefaultLimit = 50
	readEventsMaxLimit     = 200
	readEventsMaxWaitSecs  = 60
)

// eventView is the on-the-wire shape returned by read_events /
// worker_log. Body is the visible text — for messaging events, the
// parsed Message.Body; for legacy events that fail to parse, the raw
// stored Body. Message carries the full canonical envelope when it
// parses cleanly, letting Roles inspect From/To/threading without
// re-parsing.
type eventView struct {
	ID        domain.EventID  `json:"id"`
	StreamID  domain.StreamID `json:"streamId"`
	Source    domain.WorkerID `json:"source"`
	Body      string          `json:"body"`
	Message   *domain.Message `json:"message,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
}

func eventViewOf(e domain.Event) eventView {
	view := eventView{
		ID:        e.ID,
		StreamID:  e.StreamID,
		Source:    e.Source,
		Body:      e.Body,
		CreatedAt: e.CreatedAt,
	}
	if msg, err := e.Message(); err == nil {
		view.Body = msg.Body
		view.Message = &msg
	}
	return view
}

// ReadEvents returns the events on the Streams the calling Worker
// subscribes to, newest-first. With wait>0, blocks up to that many
// seconds for new events on any subscribed Stream.
type ReadEvents struct {
	deps Deps
}

const ReadEventsName domain.ToolName = "read_events"

var readEventsSchema = mustSchema[readEventsArgs]()

type readEventsArgs struct {
	Limit int    `json:"limit,omitempty"`
	Since string `json:"since,omitempty"`
	Wait  int    `json:"wait,omitempty"`
}

func (t *ReadEvents) Name() domain.ToolName           { return ReadEventsName }
func (t *ReadEvents) InputSchema() *jsonschema.Schema { return readEventsSchema }
func (t *ReadEvents) Description() string {
	return "Read events on the Streams you subscribe to, newest first. Pass since=<eventId> " +
		"to skip everything up to and including a previously-seen event. Pass wait=<seconds> " +
		"(0..60) to block for new events when nothing is currently waiting after applying " +
		"`since`. limit defaults to 50, capped at 200."
}

func (t *ReadEvents) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args readEventsArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	limit := args.Limit
	if limit <= 0 {
		limit = readEventsDefaultLimit
	}
	if limit > readEventsMaxLimit {
		limit = readEventsMaxLimit
	}
	wait := args.Wait
	if wait < 0 {
		wait = 0
	}
	if wait > readEventsMaxWaitSecs {
		wait = readEventsMaxWaitSecs
	}
	since := domain.EventID(args.Since)
	workerID := inv.Caller.ID()

	fresh, err := t.fresh(ctx, workerID, limit, since)
	if err != nil {
		return nil, err
	}
	if len(fresh) > 0 || wait == 0 || t.deps.Broadcaster == nil {
		return marshalEvents(fresh), nil
	}

	subs, err := t.deps.Store.Subscriptions.ListForWorker(ctx, workerID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions for %q: %w", workerID, err)
	}
	streamIDs := make([]domain.StreamID, 0, len(subs))
	for _, sub := range subs {
		streamIDs = append(streamIDs, sub.StreamID)
	}
	wake := t.deps.Broadcaster.Subscribe(streamIDs)
	defer t.deps.Broadcaster.Unsubscribe(streamIDs, wake)

	timer := time.NewTimer(time.Duration(wait) * time.Second)
	defer timer.Stop()

	select {
	case <-wake:
	case <-timer.C:
	case <-ctx.Done():
		return marshalEvents(nil), nil
	}

	fresh, err = t.fresh(ctx, workerID, limit, since)
	if err != nil {
		return nil, err
	}
	return marshalEvents(fresh), nil
}

// fresh returns events newer than `since` (exclusive), newest-first, up
// to `limit`. An empty `since` means "return everything".
func (t *ReadEvents) fresh(ctx context.Context, workerID domain.WorkerID, limit int, since domain.EventID) ([]domain.Event, error) {
	events, err := t.deps.Store.Events.ListForWorker(ctx, workerID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events for %q: %w", workerID, err)
	}
	if since == "" {
		return events, nil
	}
	for i, e := range events {
		if e.ID == since {
			return events[:i], nil
		}
	}
	return events, nil
}

func marshalEvents(events []domain.Event) json.RawMessage {
	out := make([]eventView, 0, len(events))
	for _, e := range events {
		out = append(out, eventViewOf(e))
	}
	body, err := json.Marshal(map[string]any{"events": out})
	if err != nil {
		// All inputs are simple structs of primitives; a marshal failure
		// here is a programming error, not a runtime condition.
		panic(fmt.Sprintf("marshal events: %v", err))
	}
	return body
}
