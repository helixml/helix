package domain

import (
	"errors"
	"time"
)

// Stream is a named source of events. Workers publish to a Stream via
// tools and receive from a Stream via Subscriptions.
//
// Every Stream has a Transport. The default — TransportLocal — keeps
// events inside the system: SQLite for storage, the in-process
// broadcaster for delivery, the dispatcher for waking subscribed AI
// Workers. Other transports (Slack, email, webhook, RSS, tick…)
// compose external I/O over the same local mechanism: events still
// land in SQLite for history and replay; the transport additionally
// ships them to or from the outside world.
type Stream struct {
	ID          StreamID
	Name        string
	Description string
	CreatedBy   WorkerID
	CreatedAt   time.Time
	Transport   Transport
}

// NewStream validates and constructs a Stream. If transport.Kind is
// empty, the returned Stream uses LocalTransport.
func NewStream(id StreamID, name, description string, createdBy WorkerID, createdAt time.Time, transport Transport) (Stream, error) {
	if id == "" {
		return Stream{}, errors.New("stream id is empty")
	}
	if name == "" {
		return Stream{}, errors.New("stream name is empty")
	}
	if createdBy == "" {
		return Stream{}, errors.New("stream createdBy is empty")
	}
	if createdAt.IsZero() {
		return Stream{}, errors.New("stream createdAt is zero")
	}
	if transport.Kind == "" {
		transport = LocalTransport()
	}
	if err := transport.Validate(); err != nil {
		return Stream{}, err
	}
	return Stream{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedBy:   createdBy,
		CreatedAt:   createdAt.UTC(),
		Transport:   transport,
	}, nil
}
