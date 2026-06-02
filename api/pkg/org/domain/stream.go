package domain

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// Stream is a named source of events. Workers publish to a Stream via
// tools and receive from a Stream via Subscriptions.
//
// Every Stream has a Transport. The default — transport.KindLocal —
// keeps events inside the system: SQLite for storage, the in-process
// broadcaster for delivery, the dispatcher for waking subscribed AI
// Workers. Other transports (Slack, email, webhook, RSS, tick…)
// compose external I/O over the same local mechanism: events still
// land in SQLite for history and replay; the transport additionally
// ships them to or from the outside world.
type Stream struct {
	ID             stream.ID
	OrganizationID string
	Name           string
	Description    string
	CreatedBy      worker.ID
	CreatedAt      time.Time
	Transport      transport.Transport
}

// NewStream validates and constructs a Stream. orgID is required.
// If t.Kind is empty, the returned Stream uses transport.LocalTransport().
func NewStream(id stream.ID, name, description string, createdBy worker.ID, createdAt time.Time, t transport.Transport, orgID string) (Stream, error) {
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
	if orgID == "" {
		return Stream{}, errors.New("stream orgID is empty")
	}
	if t.Kind == "" {
		t = transport.LocalTransport()
	}
	if err := t.Validate(); err != nil {
		return Stream{}, err
	}
	return Stream{
		ID:             id,
		OrganizationID: orgID,
		Name:           name,
		Description:    description,
		CreatedBy:      createdBy,
		CreatedAt:      createdAt.UTC(),
		Transport:      t,
	}, nil
}
