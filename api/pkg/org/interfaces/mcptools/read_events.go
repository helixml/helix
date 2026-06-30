package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
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
	ID        streaming.EventID  `json:"id"`
	TopicID   streaming.TopicID  `json:"topicId"`
	Source    orgchart.BotID     `json:"source"`
	Body      string             `json:"body"`
	Message   *streaming.Message `json:"message,omitempty"`
	CreatedAt time.Time          `json:"createdAt"`
}

func eventViewOf(e streaming.Event) eventView {
	view := eventView{
		ID:        e.ID,
		TopicID:   e.TopicID,
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

// ReadEvents returns the events on the Topics the calling Worker
// subscribes to, newest-first. With wait>0, blocks up to that many
// seconds for new events on any subscribed Topic.
type ReadEvents struct {
	deps Deps
}

const ReadEventsName tool.Name = "read_events"

var readEventsSchema = mustSchema[readEventsArgs]()

type readEventsArgs struct {
	Limit int    `json:"limit,omitempty"`
	Since string `json:"since,omitempty"`
	Wait  int    `json:"wait,omitempty"`
}

// UnmarshalJSON tolerates string-encoded ints for limit and wait.
// The declared JSON schema is `"type": "integer"` for these, but
// some LLM tool-call implementations (notably Claude Code in the
// in-sandbox Anthropic SDK) emit them as JSON strings — `"60"`
// instead of `60`. Accept either rather than failing the activation
// loop with "cannot unmarshal string into Go struct field".
func (a *readEventsArgs) UnmarshalJSON(data []byte) error {
	type plain readEventsArgs
	type tolerant struct {
		*plain
		Limit json.RawMessage `json:"limit,omitempty"`
		Wait  json.RawMessage `json:"wait,omitempty"`
	}
	t := tolerant{plain: (*plain)(a)}
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	if v, err := decodeFlexInt(t.Limit); err != nil {
		return fmt.Errorf("limit: %w", err)
	} else {
		a.Limit = v
	}
	if v, err := decodeFlexInt(t.Wait); err != nil {
		return fmt.Errorf("wait: %w", err)
	} else {
		a.Wait = v
	}
	return nil
}

// decodeFlexInt unmarshals an int that may have been encoded as
// either a JSON number or a JSON string. Empty/null is 0.
func decodeFlexInt(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, err
	}
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func (t *ReadEvents) Name() tool.Name                 { return ReadEventsName }
func (t *ReadEvents) InputSchema() *jsonschema.Schema { return readEventsSchema }
func (t *ReadEvents) Description() string {
	return "Read events on the Topics you subscribe to, newest first. Pass since=<eventId> " +
		"to skip everything up to and including a previously-seen event. Pass wait=<seconds> " +
		"(0..60) to block for new events when nothing is currently waiting after applying " +
		"`since`. limit defaults to 50, capped at 200."
}

func (t *ReadEvents) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
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
	since := streaming.EventID(args.Since)
	botID := inv.Caller.ID()
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("read_events: caller has no OrgID")
	}

	fresh, err := t.fresh(ctx, orgID, botID, limit, since)
	if err != nil {
		return nil, err
	}
	if len(fresh) > 0 || wait == 0 || t.deps.Hub == nil {
		return marshalEvents(fresh), nil
	}

	// Resolve bot → subscribed topics.
	if _, err := t.deps.Queries.GetBot(ctx, orgID, botID); err != nil {
		return nil, fmt.Errorf("get bot %q: %w", botID, err)
	}
	subs, err := t.deps.Queries.BotSubscriptions(ctx, orgID, botID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions for bot %q: %w", botID, err)
	}
	topicIDs := make([]streaming.TopicID, 0, len(subs))
	for _, sub := range subs {
		topicIDs = append(topicIDs, sub.TopicID)
	}
	wake := t.deps.Hub.Subscribe(orgID, topicIDs)
	defer t.deps.Hub.Unsubscribe(topicIDs, wake)

	timer := time.NewTimer(time.Duration(wait) * time.Second)
	defer timer.Stop()

	select {
	case <-wake:
	case <-timer.C:
	case <-ctx.Done():
		return marshalEvents(nil), nil
	}

	fresh, err = t.fresh(ctx, orgID, botID, limit, since)
	if err != nil {
		return nil, err
	}
	return marshalEvents(fresh), nil
}

// fresh returns events newer than `since` (exclusive), newest-first, up
// to `limit`. An empty `since` means "return everything".
func (t *ReadEvents) fresh(ctx context.Context, orgID string, botID orgchart.BotID, limit int, since streaming.EventID) ([]streaming.Event, error) {
	events, err := t.deps.Queries.BotEvents(ctx, orgID, botID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events for %q: %w", botID, err)
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

func marshalEvents(events []streaming.Event) json.RawMessage {
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
