package domain

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// Event is a single entry on a Stream. Events are markdown; the system
// does not impose a schema on content. Source is the Worker that
// emitted the event (empty means a system-emitted event such as a
// time tick).
type Event struct {
	ID        event.ID
	StreamID  stream.ID
	Source    worker.ID
	Body      string
	CreatedAt time.Time
}

// NewEvent validates and constructs an Event.
// Pass source = "" for system-emitted events.
func NewEvent(id event.ID, streamID stream.ID, source worker.ID, body string, createdAt time.Time) (Event, error) {
	if id == "" {
		return Event{}, errors.New("event id is empty")
	}
	if streamID == "" {
		return Event{}, errors.New("event streamId is empty")
	}
	if body == "" {
		return Event{}, errors.New("event body is empty")
	}
	if createdAt.IsZero() {
		return Event{}, errors.New("event createdAt is zero")
	}
	return Event{
		ID:        id,
		StreamID:  streamID,
		Source:    source,
		Body:      body,
		CreatedAt: createdAt.UTC(),
	}, nil
}
