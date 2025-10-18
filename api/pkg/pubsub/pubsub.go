package pubsub

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type Publisher interface {
	// Publish topic to message broker with payload.
	Publish(ctx context.Context, topic string, payload []byte) error
	PublishWithHeader(ctx context.Context, topic string, header map[string]string, payload []byte) error
}

type PubSub interface {
	Publisher
	Subscribe(ctx context.Context, topic string, handler func(payload []byte) error) (Subscription, error)
	SubscribeWithCtx(ctx context.Context, topic string, handler func(ctx context.Context, msg *nats.Msg) error) (Subscription, error)
	Request(ctx context.Context, sub string, header map[string]string, payload []byte, timeout time.Duration) ([]byte, error)
	QueueRequest(ctx context.Context, stream, sub string, payload []byte, header map[string]string, timeout time.Duration) ([]byte, error)
	QueueSubscribe(ctx context.Context, stream, sub string, handler func(msg *Message) error) (Subscription, error)
	StreamRequest(ctx context.Context, stream, sub string, payload []byte, header map[string]string, timeout time.Duration) ([]byte, error)
	StreamConsume(ctx context.Context, stream, sub string, handler func(msg *Message) error) (Subscription, error)
	OnConnectionStatus(handler ConnectionStatusHandler)
}

type Message struct {
	Type   string
	Reply  string
	Data   []byte
	Header nats.Header

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
	ZedAgentRunnerStream = "ZED_AGENTS"
	ZedAgentQueue        = "zed_agents"
	ScriptRunnerStream   = "SCRIPT_RUNNERS"
	AppQueue             = "app"
	RunnerQueue          = "runner"
	HelixNatsReplyHeader = "helix-nats-reply"
)

func getStreamSub(stream, sub string) string {
	return stream + "." + sub
}

func GetRunnerResponsesQueue(ownerID, reqID string) string {
	return "runner-responses." + ownerID + "." + reqID
}

func GetRunnerQueue(runnerID string) string {
	return RunnerQueue + "." + runnerID
}

func GetRunnerConnectedQueue(subject string) string {
	return "runner.connected." + subject
}

func ParseRunnerID(subject string) (string, error) {
	parts := strings.Split(subject, ".")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid subject: %s", subject)
	}
	return parts[2], nil
}

// External agent specific queue functions
func GetExternalAgentRegistrationQueue() string {
	return "external_agents.register"
}

func GetExternalAgentHeartbeatQueue() string {
	return "external_agents.heartbeat"
}

func GetExternalAgentResponseQueue() string {
	return "external_agents.response"
}

func GetExternalAgentStreamForMessageType(messageType string) string {
	switch messageType {
	case "agent.register", "agent.heartbeat", "agent.response":
		return ZedAgentRunnerStream
	default:
		return ZedAgentRunnerStream
	}
}
