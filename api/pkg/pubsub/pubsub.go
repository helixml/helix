package pubsub

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type Publisher interface {
	// Publish topic to message broker with payload.
	Publish(ctx context.Context, topic string, payload []byte) error
}

type PubSub interface {
	Publisher
	Subscribe(ctx context.Context, topic string, handler func(payload []byte) error) (Subscription, error)

	StreamRequest(ctx context.Context, stream, sub string, payload []byte, timeout time.Duration) ([]byte, error)
	StreamConsume(ctx context.Context, stream, sub string, conc int, handler func(msg *Message) error) (Subscription, error)
}

type Message struct {
	Reply string
	Data  []byte

	msg jetstream.Msg
}

func (m *Message) Ack() error {
	return m.msg.Ack()
}

type Subscription interface {
	Unsubscribe() error
}

func GetSessionQueue(ownerID, sessionID string) string {
	return "session-updates." + ownerID + "." + sessionID
}

const (
	ScriptRunnerStream = "SCRIPTS"
	AppQueue           = "apps"
	ToolQueue          = "tools"
)

func getStreamSub(stream, sub string) string {
	return stream + "." + sub
}
