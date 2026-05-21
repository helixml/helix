package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

type streamView struct {
	ID            domain.StreamID `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	CreatedBy     domain.WorkerID `json:"createdBy"`
	CreatedAt     time.Time       `json:"createdAt"`
	TransportKind string          `json:"transportKind"`
}

func streamViewOf(s domain.Stream) streamView {
	return streamView{
		ID:            s.ID,
		Name:          s.Name,
		Description:   s.Description,
		CreatedBy:     s.CreatedBy,
		CreatedAt:     s.CreatedAt,
		TransportKind: string(s.Transport.Kind),
	}
}

// ListStreams returns every Stream.
type ListStreams struct {
	deps Deps
}

const ListStreamsName domain.ToolName = "list_streams"

var listStreamsSchema = mustSchema[listStreamsArgs]()

type listStreamsArgs struct{}

func (t *ListStreams) Name() domain.ToolName           { return ListStreamsName }
func (t *ListStreams) InputSchema() *jsonschema.Schema { return listStreamsSchema }
func (t *ListStreams) Description() string {
	return "List every Stream: id, name, description, creator, transport kind, and created-at."
}

func (t *ListStreams) Invoke(ctx context.Context, _ domain.Invocation) (json.RawMessage, error) {
	streams, err := t.deps.Store.Streams.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	out := make([]streamView, 0, len(streams))
	for _, s := range streams {
		out = append(out, streamViewOf(s))
	}
	return json.Marshal(map[string]any{"streams": out})
}

// GetStream returns one Stream by ID.
type GetStream struct {
	deps Deps
}

const GetStreamName domain.ToolName = "get_stream"

var getStreamSchema = mustSchema[getStreamArgs]()

type getStreamArgs struct {
	ID string `json:"id"`
}

func (t *GetStream) Name() domain.ToolName           { return GetStreamName }
func (t *GetStream) InputSchema() *jsonschema.Schema { return getStreamSchema }
func (t *GetStream) Description() string {
	return "Fetch one Stream by id."
}

func (t *GetStream) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args getStreamArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	s, err := t.deps.Store.Streams.Get(ctx, domain.StreamID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get stream %q: %w", args.ID, err)
	}
	return json.Marshal(streamViewOf(s))
}

// ListStreamEvents returns recent Events on one Stream, newest first.
// Non-blocking — callers who want to wait for new events use read_events.
type ListStreamEvents struct {
	deps Deps
}

const ListStreamEventsName domain.ToolName = "list_stream_events"

var listStreamEventsSchema = mustSchema[listStreamEventsArgs]()

const (
	listStreamEventsDefaultLimit = 50
	listStreamEventsMaxLimit     = 200
)

type listStreamEventsArgs struct {
	StreamID string `json:"streamId"`
	Limit    int    `json:"limit,omitempty"`
}

// UnmarshalJSON tolerates a string-encoded Limit — same LLM-quirk
// fix as read_events. See decodeFlexInt comment.
func (a *listStreamEventsArgs) UnmarshalJSON(data []byte) error {
	type plain listStreamEventsArgs
	type tolerant struct {
		*plain
		Limit json.RawMessage `json:"limit,omitempty"`
	}
	t := tolerant{plain: (*plain)(a)}
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	v, err := decodeFlexInt(t.Limit)
	if err != nil {
		return fmt.Errorf("limit: %w", err)
	}
	a.Limit = v
	return nil
}

func (t *ListStreamEvents) Name() domain.ToolName           { return ListStreamEventsName }
func (t *ListStreamEvents) InputSchema() *jsonschema.Schema { return listStreamEventsSchema }
func (t *ListStreamEvents) Description() string {
	return "List recent Events on a Stream, newest first. Returns immediately. limit defaults " +
		"to 50, capped at 200."
}

func (t *ListStreamEvents) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args listStreamEventsArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.StreamID == "" {
		return nil, fmt.Errorf("streamId is required")
	}
	limit := args.Limit
	if limit <= 0 {
		limit = listStreamEventsDefaultLimit
	}
	if limit > listStreamEventsMaxLimit {
		limit = listStreamEventsMaxLimit
	}
	streamID := domain.StreamID(args.StreamID)
	if _, err := t.deps.Store.Streams.Get(ctx, streamID); err != nil {
		return nil, fmt.Errorf("stream %q: %w", streamID, err)
	}
	events, err := t.deps.Store.Events.ListForStream(ctx, streamID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events for %q: %w", streamID, err)
	}
	out := make([]eventView, 0, len(events))
	for _, e := range events {
		out = append(out, eventViewOf(e))
	}
	return json.Marshal(map[string]any{"events": out})
}
