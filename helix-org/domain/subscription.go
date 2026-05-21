package domain

import (
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// Subscription is a Worker's link to a Stream. Events published on the
// Stream wake the Worker (via the dispatcher, for AI Workers) and show
// up when they read their events. The (worker.ID, stream.ID) pair is the
// identity — there is no synthetic ID.
type Subscription struct {
	WorkerID  worker.ID
	StreamID  stream.ID
	CreatedAt time.Time
}

// NewSubscription validates and constructs a Subscription.
func NewSubscription(workerID worker.ID, streamID stream.ID, createdAt time.Time) (Subscription, error) {
	if workerID == "" {
		return Subscription{}, errors.New("subscription workerId is empty")
	}
	if streamID == "" {
		return Subscription{}, errors.New("subscription streamId is empty")
	}
	if createdAt.IsZero() {
		return Subscription{}, errors.New("subscription createdAt is zero")
	}
	return Subscription{
		WorkerID:  workerID,
		StreamID:  streamID,
		CreatedAt: createdAt.UTC(),
	}, nil
}
