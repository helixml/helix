package streaming

import (
	"errors"
	"time"
)

// Event is a single entry on a Stream. Events are markdown; the system
// does not impose a schema on content. Source is the Worker that
// emitted the event (empty means a system-emitted event such as a
// time tick).
//
// Source is an orgchart.WorkerID carried as a plain string; the
// streaming aggregate intentionally does not import orgchart.
type Event struct {
	ID             EventID
	OrganizationID string
	StreamID       StreamID
	Source         string // orgchart.WorkerID
	Body           string
	CreatedAt      time.Time
}

// NewEvent validates and constructs an Event. orgID is required.
// Pass source = "" for system-emitted events.
func NewEvent(id EventID, streamID StreamID, source string, body string, createdAt time.Time, orgID string) (Event, error) {
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
	if orgID == "" {
		return Event{}, errors.New("event orgID is empty")
	}
	return Event{
		ID:             id,
		OrganizationID: orgID,
		StreamID:       streamID,
		Source:         source,
		Body:           body,
		CreatedAt:      createdAt.UTC(),
	}, nil
}

// SourcePrincipal returns Event.Source as a typed Principal.
// Inference rules:
//
//   - empty Source → zero-Principal (system-emitted / inbound
//     transport without a resolved sender)
//   - non-empty Source → KindWorker (today's only populated case)
func (e Event) SourcePrincipal() Principal {
	if e.Source == "" {
		return Principal{}
	}
	return NewPrincipalWorker(e.Source)
}

// Message parses the Event's Body as a canonical Message. Every Event
// in the system carries Message JSON in its Body, so this should
// always succeed; an error indicates a bug or a hand-poked database.
func (e Event) Message() (Message, error) {
	return DecodeMessage(e.Body)
}

// NewMessageEvent is the standard way to construct an Event whose
// Body holds a Message. It encodes the Message and delegates field
// validation to NewEvent.
func NewMessageEvent(id EventID, streamID StreamID, source string, msg Message, createdAt time.Time, orgID string) (Event, error) {
	body, err := msg.Encode()
	if err != nil {
		return Event{}, err
	}
	return NewEvent(id, streamID, source, body, createdAt, orgID)
}
