package streaming

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Stream is a named source of events. Workers publish to a Stream
// via tools and receive from a Stream via Subscriptions.
//
// Every Stream has a Transport. The default — transport.KindLocal —
// keeps events inside the system: Postgres for storage, the
// in-process broadcaster for delivery, the dispatcher for waking
// subscribed AI Workers. Other transports (Slack, email, webhook,
// RSS, tick…) compose external I/O over the same local mechanism.
//
// CreatedBy is an orgchart.WorkerID stored as a plain string; the
// streaming aggregate intentionally does not import orgchart to keep
// the dependency DAG one-way.
type Stream struct {
	ID             StreamID
	OrganizationID string
	Name           string
	Description    string
	CreatedBy      string // orgchart.WorkerID
	CreatedAt      time.Time
	Transport      transport.Transport
}

// NewStream validates and constructs a Stream. orgID is required.
// If t.Kind is empty, the returned Stream uses
// transport.LocalTransport().
func NewStream(id StreamID, name, description string, createdBy string, createdAt time.Time, t transport.Transport, orgID string) (Stream, error) {
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
