package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

type Nats struct {
	conn *nats.Conn
}

func NewInMemoryNats() (*Nats, error) {
	opts := &server.Options{Host: "127.0.0.1", Port: server.RANDOM_PORT, NoSigs: true}

	// Initialize new server with options
	ns, err := server.NewServer(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create in-memory nats server: %w", err)
	}

	// Start the server via goroutine
	go ns.Start()

	// Wait for server to be ready for connections
	if !ns.ReadyForConnections(4 * time.Second) {
		return nil, fmt.Errorf("failed to start in-memory nats server")
	}

	// Connect to server
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to nats: %w", err)
	}

	return &Nats{
		conn: nc,
	}, nil
}

func (n *Nats) Subscribe(ctx context.Context, topic string, handler func(payload []byte) error) (Subscription, error) {
	sub, err := n.conn.Subscribe(topic, func(msg *nats.Msg) {
		err := handler(msg.Data)
		if err != nil {
			log.Err(err).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (n *Nats) Publish(ctx context.Context, topic string, payload []byte) error {
	return n.conn.Publish(topic, payload)
}

// Request publish a message to the given subject and creates an inbox to receive the response. If response is not
// received within the timeout, an error is returned.
func (n *Nats) Request(ctx context.Context, topic string, payload []byte, timeout time.Duration) ([]byte, error) {
	msg, err := n.conn.Request(topic, payload, timeout)
	if err != nil {
		return nil, err
	}

	return msg.Data, nil
}

// QueueSubscribe is similar to Subscribe, but it will only deliver a message to one subscriber in the group. This way you can
// have multiple subscribers to the same subject, but only one gets it.
func (n *Nats) QueueSubscribe(ctx context.Context, topic, queue string, handler func(reply string, payload []byte) error) (Subscription, error) {
	sub, err := n.conn.QueueSubscribe(topic, queue, func(msg *nats.Msg) {
		err := handler(msg.Reply, msg.Data)
		if err != nil {
			log.Err(err).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, err
	}

	return sub, nil
}
