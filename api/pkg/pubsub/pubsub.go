package pubsub

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

type Publisher interface {
	// Publish topic to message broker with payload.
	Publish(ctx context.Context, topic string, payload []byte) error
}

type PubSub interface {
	Publisher
	Subscribe(ctx context.Context, topic string, handler func(payload []byte) error) (Subscription, error)

	// Request sends a request to a topic and waits for a response. Should be used for fast, ephemeral workloads
	Request(ctx context.Context, stream, sub string, payload []byte, timeout time.Duration) ([]byte, error)
	// QueueSubscribe subscribes to a topic with a queue group. Should be used for fast workloads, failed
	// messages will not be redelivered. Slow consumers will block the queue group.
	QueueSubscribe(ctx context.Context, stream, sub string, conc int, handler func(msg *Message) error) (Subscription, error)

	StreamRequest(ctx context.Context, stream, sub string, payload []byte, timeout time.Duration) ([]byte, error)
	StreamConsume(ctx context.Context, stream, sub string, conc int, handler func(msg *Message) error) (Subscription, error)
}

type Message struct {
	Type  string
	Reply string
	Data  []byte

	msg acker
}

type acker interface {
	Ack() error
	Nak() error
}

// natsMsgWrapper is used to wrap nats msg to ensure
// interface compatibility
type natsMsgWrapper struct {
	msg *nats.Msg
}

func (a *natsMsgWrapper) Ack() error {
	return a.msg.Ack()
}

func (a *natsMsgWrapper) Nak() error {
	return a.msg.Nak()
}

// Ack acknowledges a message
// This tells the server that the message was successfully processed and it can move on to the next message
func (m *Message) Ack() error {
	return m.msg.Ack()
}

// Nak negatively acknowledges a message
// This tells the server to redeliver the message
func (m *Message) Nak() error {
	return m.msg.Nak()
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

func getStreamSubV2(stream, sub, id string) string {
	return stream + "." + sub + "." + id
}
