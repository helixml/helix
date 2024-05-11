package pubsub

import (
	"context"
	"time"
)

type Publisher interface {
	// Publish topic to message broker with payload.
	Publish(ctx context.Context, topic string, payload []byte) error
}

type PubSub interface {
	Publisher
	Subscribe(ctx context.Context, topic string, handler func(payload []byte) error) (Subscription, error)
	Request(ctx context.Context, topic string, payload []byte, timeout time.Duration) ([]byte, error)
	QueueSubscribe(ctx context.Context, topic, queue string, handler func(reply string, payload []byte) error) (Subscription, error)
}

type Subscription interface {
	Unsubscribe() error
}

func GetSessionQueue(ownerID, sessionID string) string {
	return "session-updates." + ownerID + "." + sessionID
}

func GetGPTScriptQueue() string {
	return "gptscript"
}
