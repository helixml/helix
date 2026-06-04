package streaming

import (
	"errors"
	"time"
)

// Subscription is a Position's link to a Stream. The (PositionID,
// StreamID) pair is the identity — there is no synthetic ID.
//
// Subscriptions are POSITION-anchored, not Worker-anchored: the
// subscription represents "whoever fills this slot in the org chart
// will see events on this stream". Hiring or firing a Worker into the
// position doesn't change which Streams the position consumes;
// dispatch walks (stream → subscribing positions → current workers in
// those positions) when an event arrives.
//
// PositionID is an orgchart.PositionID carried as a plain string; the
// streaming aggregate intentionally does not import orgchart to keep
// the dependency DAG one-way.
type Subscription struct {
	OrganizationID string
	PositionID     string // orgchart.PositionID
	StreamID       StreamID
	CreatedAt      time.Time
}

// NewSubscription validates and constructs a Subscription. orgID is
// required — subscriptions are tenant-scoped.
func NewSubscription(positionID string, streamID StreamID, createdAt time.Time, orgID string) (Subscription, error) {
	if positionID == "" {
		return Subscription{}, errors.New("subscription positionId is empty")
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
		PositionID:     positionID,
		StreamID:       streamID,
		CreatedAt:      createdAt.UTC(),
	}, nil
}
