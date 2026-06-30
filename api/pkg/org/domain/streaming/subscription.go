package streaming

import (
	"errors"
	"time"
)

// Subscription is a Bot's link to a Topic. The (BotID, TopicID) pair is
// the identity — there is no synthetic ID.
//
// Subscriptions are BOT-anchored: deleting a Bot drops its
// subscriptions. They are driven explicitly (subscribe / unsubscribe),
// letting each Bot consume exactly the topics it should.
//
// BotID is an orgchart.BotID carried as a plain string; the streaming
// aggregate intentionally does not import orgchart to keep the
// dependency DAG one-way.
type Subscription struct {
	OrganizationID string
	BotID          string // orgchart.BotID
	TopicID        TopicID
	CreatedAt      time.Time
}

// NewSubscription validates and constructs a Subscription. orgID is
// required — subscriptions are tenant-scoped.
func NewSubscription(botID string, topicID TopicID, createdAt time.Time, orgID string) (Subscription, error) {
	if botID == "" {
		return Subscription{}, errors.New("subscription botId is empty")
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
		BotID:          botID,
		TopicID:        topicID,
		CreatedAt:      createdAt.UTC(),
	}, nil
}
