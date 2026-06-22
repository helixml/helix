package streaming

import (
	"errors"
	"time"
)

// Subscription is a Worker's link to a Topic. The (WorkerID, TopicID)
// pair is the identity — there is no synthetic ID.
//
// Subscriptions are WORKER-anchored: firing a Worker drops its
// subscriptions. The hiring playbook re-subscribes new hires
// explicitly, which lets two Workers in the same Role consume
// different topics (specialisation) or only the on-call subset of a
// role wake up on a given event (load patterns).
//
// WorkerID is an orgchart.WorkerID carried as a plain string; the
// streaming aggregate intentionally does not import orgchart to keep
// the dependency DAG one-way.
type Subscription struct {
	OrganizationID string
	WorkerID       string // orgchart.WorkerID
	TopicID       TopicID
	CreatedAt      time.Time
}

// NewSubscription validates and constructs a Subscription. orgID is
// required — subscriptions are tenant-scoped.
func NewSubscription(workerID string, topicID TopicID, createdAt time.Time, orgID string) (Subscription, error) {
	if workerID == "" {
		return Subscription{}, errors.New("subscription workerId is empty")
	}
	if topicID == "" {
		return Subscription{}, errors.New("subscription topicId is empty")
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
		TopicID:       topicID,
		CreatedAt:      createdAt.UTC(),
	}, nil
}
