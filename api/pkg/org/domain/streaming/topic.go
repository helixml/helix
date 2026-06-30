package streaming

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Topic is a named source of events. Workers publish to a Topic
// via tools and receive from a Topic via Subscriptions.
//
// Every Topic has a Transport. The default — transport.KindLocal —
// keeps events inside the system: Postgres for storage, the
// in-process broadcaster for delivery, the dispatcher for waking
// subscribed AI Workers. Other transports (Slack, email, webhook,
// RSS, tick…) compose external I/O over the same local mechanism.
//
// CreatedBy is an orgchart.WorkerID stored as a plain string; the
// streaming aggregate intentionally does not import orgchart to keep
// the dependency DAG one-way.
type Topic struct {
	ID             TopicID
	OrganizationID string
	Name           string
	Description    string
	CreatedBy      string // orgchart.WorkerID (or processor.SystemActor for automation)
	CreatedAt      time.Time
	Transport      transport.Transport
}

// NewTopic validates and constructs a Topic. orgID is required.
// If t.Kind is empty, the returned Topic uses
// transport.LocalTransport().
func NewTopic(id TopicID, name, description string, createdBy string, createdAt time.Time, t transport.Transport, orgID string) (Topic, error) {
	if id == "" {
		return Topic{}, errors.New("topic id is empty")
	}
	if name == "" {
		return Topic{}, errors.New("topic name is empty")
	}
	// createdBy is intentionally optional: it is the Worker the human was
	// acting as when the topic was created, and it is purely cosmetic —
	// it only anchors the topic's node to a Worker on the chart. An
	// operator creating a topic from the Topics tab (no worker context)
	// leaves it empty, and the topic is simply unanchored. Requiring it
	// here broke UI topic creation after the owner concept was removed
	// (there is no implicit owner to default it to).
	if createdAt.IsZero() {
		return Topic{}, errors.New("topic createdAt is zero")
	}
	if orgID == "" {
		return Topic{}, errors.New("topic orgID is empty")
	}
	if t.Kind == "" {
		t = transport.LocalTransport()
	}
	if err := t.Validate(); err != nil {
		return Topic{}, err
	}
	return Topic{
		ID:             id,
		OrganizationID: orgID,
		Name:           name,
		Description:    description,
		CreatedBy:      createdBy,
		CreatedAt:      createdAt.UTC(),
		Transport:      t,
	}, nil
}
