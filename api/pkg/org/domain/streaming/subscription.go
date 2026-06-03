package streaming

import (
	"errors"
	"time"
)

// Subscription is a Worker's link to a Stream. Events published on the
// Stream wake the Worker (via the dispatcher, for AI Workers) and show
// up when they read their events. The (WorkerID, StreamID) pair is the
// identity — there is no synthetic ID.
//
// WorkerID is an orgchart.WorkerID carried as a plain string; the
// streaming aggregate intentionally does not import orgchart.
type Subscription struct {
	OrganizationID string
	WorkerID       string // orgchart.WorkerID
	StreamID       StreamID
	CreatedAt      time.Time
}

// NewSubscription validates and constructs a Subscription. orgID is
// required — subscriptions are tenant-scoped.
func NewSubscription(workerID string, streamID StreamID, createdAt time.Time, orgID string) (Subscription, error) {
	if workerID == "" {
		return Subscription{}, errors.New("subscription workerId is empty")
	}
	if streamID == "" {
		return Subscription{}, errors.New("subscription streamId is empty")
	}
	if createdAt.IsZero() {
		return Subscription{}, errors.New("subscription createdAt is zero")
	}
	if orgID == "" {
		return Subscription{}, errors.New("subscription orgID is empty")
	}
	return Subscription{
		OrganizationID: orgID,
		WorkerID:       workerID,
		StreamID:       streamID,
		CreatedAt:      createdAt.UTC(),
	}, nil
}
