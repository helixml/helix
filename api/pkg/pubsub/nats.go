package pubsub

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog/log"
)

type Nats struct {
	conn *nats.Conn
	js   jetstream.JetStream

	stream jetstream.Stream
}

func NewInMemoryNats(storeDir string) (*Nats, error) {
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      server.RANDOM_PORT,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  storeDir,
	}

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

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream context: %w", err)
	}

	stream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:     ScriptRunnerStream,
		Subjects: []string{"SCRIPTS.*"},
		// ConsumerLimits: jetstream.StreamConsumerLimits{
		// 	MaxAckPending: 20,
		// },
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream stream: %w", err)
	}

	return &Nats{
		conn:   nc,
		js:     js,
		stream: stream,
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

const jetstreamReplyHeader = "helix-reply"

// Request publish a message to the given subject and creates an inbox to receive the response. If response is not
// received within the timeout, an error is returned.
func (n *Nats) StreamRequest(ctx context.Context, stream, subject string, payload []byte, timeout time.Duration) ([]byte, error) {
	replyInbox := nats.NewInbox()
	var dataCh = make(chan []byte)

	sub, err := n.conn.Subscribe(replyInbox, func(msg *nats.Msg) {
		dataCh <- msg.Data
	})
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	hdr := nats.Header{}
	hdr.Set(jetstreamReplyHeader, replyInbox)

	streamTopic := getStreamSub(stream, subject)

	fmt.Printf("XX sending message to %s\n", streamTopic)
	fmt.Println(string(payload))

	// Publish the message to the JetStream stream,
	// one of the consumer will pick it up
	_, err = n.js.PublishMsg(ctx, &nats.Msg{
		Subject: streamTopic,
		Data:    payload,
		Header:  hdr,
	},
		jetstream.WithRetryWait(100*time.Millisecond),
		jetstream.WithRetryAttempts(10),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to publish message to jetstream: %w", err)
	}

	data := <-dataCh

	return data, nil
}

// QueueSubscribe is similar to Subscribe, but it will only deliver a message to one subscriber in the group. This way you can
// have multiple subscribers to the same subject, but only one gets it.
func (n *Nats) StreamConsume(ctx context.Context, stream, subject string, conc int, handler func(msg *Message) error) (Subscription, error) {
	s, err := n.js.Stream(ctx, stream)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	filter := getStreamSub(stream, subject)

	fmt.Println("XX filter", filter)

	c, err := s.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		// Durable:        "durable",
		AckPolicy:      jetstream.AckExplicitPolicy,
		FilterSubjects: []string{filter},
		AckWait:        5 * time.Second,
		// MemoryStorage:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer")
	}

	cons, err := c.Consume(func(msg jetstream.Msg) {
		err := handler(&Message{
			Reply: msg.Headers().Get(jetstreamReplyHeader),
			Data:  msg.Data(),
			msg:   msg,
		})
		if err != nil {
			log.Err(err).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start msg consumer: %w", err)
	}

	return &consumerWrapper{consumer: cons}, nil
}

type consumerWrapper struct {
	consumer jetstream.ConsumeContext
}

func (c *consumerWrapper) Unsubscribe() error {
	c.consumer.Stop()
	return nil
}
