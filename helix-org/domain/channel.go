package domain

import (
	"errors"
	"time"
)

// Channel is a named source of events. Workers publish to Channels via tools
// and receive from Channels via Streams (subscriptions).
type Channel struct {
	ID          ChannelID
	Name        string
	Description string
	CreatedBy   WorkerID
	CreatedAt   time.Time
}

// NewChannel validates and constructs a Channel.
func NewChannel(id ChannelID, name, description string, createdBy WorkerID, createdAt time.Time) (Channel, error) {
	if id == "" {
		return Channel{}, errors.New("channel id is empty")
	}
	if name == "" {
		return Channel{}, errors.New("channel name is empty")
	}
	if createdBy == "" {
		return Channel{}, errors.New("channel createdBy is empty")
	}
	if createdAt.IsZero() {
		return Channel{}, errors.New("channel createdAt is zero")
	}
	return Channel{
		ID:          id,
		Name:        name,
		Description: description,
		CreatedBy:   createdBy,
		CreatedAt:   createdAt.UTC(),
	}, nil
}
