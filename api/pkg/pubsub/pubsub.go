package pubsub

import (
	"context"
)

type Publisher interface {
	// Publish topic to message broker with payload.
	Publish(ctx context.Context, topic string, payload []byte) error
}

type PubSub interface {
	Publisher
	Subscribe(ctx context.Context, topic string, handler func(payload []byte) error) (Subscription, error)
}

type Subscription interface {
	Unsubscribe() error
}

func GetSessionQueue(ownerID, sessionID string) string {
	return "session-updates." + ownerID + "." + sessionID
}
