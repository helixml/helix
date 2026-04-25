package domain

import (
	"errors"
	"time"
)

// Stream is a Worker's subscription to a Channel. Events published on the
// Channel flow into the Worker's Feed while the Stream exists.
type Stream struct {
	ID        StreamID
	WorkerID  WorkerID
	ChannelID ChannelID
	CreatedAt time.Time
}

// NewStream validates and constructs a Stream.
func NewStream(id StreamID, workerID WorkerID, channelID ChannelID, createdAt time.Time) (Stream, error) {
	if id == "" {
		return Stream{}, errors.New("stream id is empty")
	}
	if workerID == "" {
		return Stream{}, errors.New("stream workerId is empty")
	}
	if channelID == "" {
		return Stream{}, errors.New("stream channelId is empty")
	}
	if createdAt.IsZero() {
		return Stream{}, errors.New("stream createdAt is zero")
	}
	return Stream{
		ID:        id,
		WorkerID:  workerID,
		ChannelID: channelID,
		CreatedAt: createdAt.UTC(),
	}, nil
}
