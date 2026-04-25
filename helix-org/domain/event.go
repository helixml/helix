package domain

import (
	"errors"
	"time"
)

// Event is a single entry on a Channel. Events are markdown; the system does
// not impose a schema on content. Source is the Worker that emitted the event
// (empty means a system-emitted event such as a time tick).
type Event struct {
	ID        EventID
	ChannelID ChannelID
	Source    WorkerID
	Body      string
	CreatedAt time.Time
}

// NewEvent validates and constructs an Event.
// Pass source = "" for system-emitted events.
func NewEvent(id EventID, channelID ChannelID, source WorkerID, body string, createdAt time.Time) (Event, error) {
	if id == "" {
		return Event{}, errors.New("event id is empty")
	}
	if channelID == "" {
		return Event{}, errors.New("event channelId is empty")
	}
	if body == "" {
		return Event{}, errors.New("event body is empty")
	}
	if createdAt.IsZero() {
		return Event{}, errors.New("event createdAt is zero")
	}
	return Event{
		ID:        id,
		ChannelID: channelID,
		Source:    source,
		Body:      body,
		CreatedAt: createdAt.UTC(),
	}, nil
}
