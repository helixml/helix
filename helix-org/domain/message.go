package domain

import (
	"time"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// The Message envelope and the Encode/Decode helpers moved to
// api/pkg/org/message in B3b. The two remaining functions in this
// file are constructors/accessors that bridge Message to Event —
// they live with Event until Event itself lifts.

// Message parses the Event's Body as a canonical message.Message.
// Every Event in the system carries Message JSON in its Body, so this
// should always succeed; an error indicates a bug or a hand-poked
// database.
func (e Event) Message() (message.Message, error) {
	return message.Decode(e.Body)
}

// NewMessageEvent is the standard way to construct an Event whose
// Body holds a Message. It encodes the Message and delegates field
// validation to NewEvent.
func NewMessageEvent(id event.ID, streamID stream.ID, source worker.ID, msg message.Message, createdAt time.Time) (Event, error) {
	body, err := msg.Encode()
	if err != nil {
		return Event{}, err
	}
	return NewEvent(id, streamID, source, body, createdAt)
}
