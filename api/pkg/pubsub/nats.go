package pubsub

import (
	"context"
	"fmt"
	"os"
	"sync"
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

	consumerMu sync.Mutex
	consumer   jetstream.Consumer
}

func NewInMemoryNats(storeDir string) (*Nats, error) {
	tmpDir, _ := os.MkdirTemp(os.TempDir(), "helix-nats")
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      server.RANDOM_PORT,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  tmpDir,
		// Setting payload to 32 MB
		MaxPayload: 32 * 1024 * 1024,
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
		Name:      "SCRIPTS_STREAM",
		Subjects:  []string{"SCRIPTS.*"},
		Retention: jetstream.WorkQueuePolicy,
		// Storage:   jetstream.MemoryStorage,
		Discard: jetstream.DiscardOld,
		MaxAge:  5 * time.Minute, // Discard messages older than 5 minutes
		// ConsumerLimits: jetstream.StreamConsumerLimits{
		// 	MaxAckPending: 20,
		// },
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create jetstream stream: %w", err)
	}

	ctx := context.Background()
	c, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		AckPolicy:      jetstream.AckExplicitPolicy,
		FilterSubjects: []string{getStreamSub(ScriptRunnerStream, AppQueue)},
		AckWait:        5 * time.Second,
		// MemoryStorage:  true,
		ReplayPolicy: jetstream.ReplayInstantPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Basic monitoring of the stream
	go func() {
		for {
			info, err := stream.Info(ctx)
			if err != nil {
				log.Err(err).Msg("failed to get stream info")
				continue
			}
			log.Debug().
				Int("messages", int(info.State.Msgs)).
				Int("consumers", int(info.State.Consumers)).
				Time("oldest_message", info.State.FirstTime).
				Time("newest_message", info.State.LastTime).
				Msg("Stream info")
			time.Sleep(10 * time.Second)
		}
	}()

	return &Nats{
		conn:     nc,
		js:       js,
		stream:   stream,
		consumer: c,
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

func (n *Nats) Request(ctx context.Context, _, subject string, payload []byte, header map[string]string, timeout time.Duration) ([]byte, error) {
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

	for k, v := range header {
		hdr.Set(k, v)
	}

	hdr.Set(helixNatsReplyHeader, replyInbox)
	hdr.Set(helixNatsSubjectHeader, subject)

	// Publish the message to NATS
	err = n.conn.PublishMsg(&nats.Msg{
		Subject: subject,
		Data:    payload,
		Header:  hdr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to publish message to jetstream: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case data := <-dataCh:
		return data, nil
	}
}

func (n *Nats) QueueSubscribe(ctx context.Context, queue, subject string, conc int, handler func(msg *Message) error) (Subscription, error) {
	sub, err := n.conn.QueueSubscribe(subject, queue, func(msg *nats.Msg) {
		err := handler(&Message{
			Reply:  msg.Header.Get(helixNatsReplyHeader),
			Data:   msg.Data,
			Type:   msg.Header.Get(helixNatsSubjectHeader),
			Header: msg.Header,
			msg:    &natsMsgWrapper{msg},
		})
		if err != nil {
			log.Err(err).Msg("error handling message")
		}
	})
	if err != nil {
		return nil, err
	}

	return sub, nil
}

const (
	helixNatsReplyHeader   = "helix-reply"
	helixNatsSubjectHeader = "helix-subject"
)

// Request publish a message to the given subject and creates an inbox to receive the response. If response is not
// received within the timeout, an error is returned.
func (n *Nats) StreamRequest(ctx context.Context, stream, subject string, payload []byte, header map[string]string, timeout time.Duration) ([]byte, error) {
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

	for k, v := range header {
		hdr.Set(k, v)
	}

	hdr.Set(helixNatsReplyHeader, replyInbox)
	hdr.Set(helixNatsSubjectHeader, subject)

	// streamTopic := getStreamSub(stream, subject) + "." + nuid.Next()
	streamTopic := getStreamSub(stream, subject)

	// Publish the message to the JetStream stream,
	// one of the consumer will pick it up
	_, err = n.js.PublishMsg(ctx, &nats.Msg{
		Subject: streamTopic,
		Data:    payload,
		Header:  hdr,
	},
		jetstream.WithRetryWait(50*time.Millisecond),
		jetstream.WithRetryAttempts(10),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to publish message to jetstream: %w", err)
	}

	select {
	case <-ctx.Done():
		info, err := n.stream.Info(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to get stream info: %w", err)
		}

		if info.State.Consumers == 0 {
			return nil, fmt.Errorf("no consumers available to process the request, are there any runner connected?")
		}

		return nil, ctx.Err()
	case data := <-dataCh:
		return data, nil
	}
}

// QueueSubscribe is similar to Subscribe, but it will only deliver a message to one subscriber in the group. This way you can
// have multiple subscribers to the same subject, but only one gets it.
func (n *Nats) StreamConsume(ctx context.Context, stream, subject string, conc int, handler func(msg *Message) error) (Subscription, error) {
	n.consumerMu.Lock()
	defer n.consumerMu.Unlock()

	info, err := n.stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	if info.State.Consumers == 0 {
		// Creating consumer
		ctx := context.Background()
		c, err := n.stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			AckPolicy:      jetstream.AckExplicitPolicy,
			FilterSubjects: []string{getStreamSub(stream, subject)},
			AckWait:        5 * time.Second,
			ReplayPolicy:   jetstream.ReplayInstantPolicy,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer: %w", err)
		}
		n.consumer = c
	}

	mc, err := n.consumer.Messages()
	if err != nil {
		return nil, fmt.Errorf("failed to get messages context: %w", err)
	}

	go func() {
		<-ctx.Done()
		mc.Stop()
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := mc.Next()
				if err != nil {
					log.Err(err).Msg("failed to fetch messages")
					continue
				}

				err = handler(&Message{
					Type:   msg.Headers().Get(helixNatsSubjectHeader),
					Reply:  msg.Headers().Get(helixNatsReplyHeader),
					Data:   msg.Data(),
					Header: msg.Headers(),
					msg:    msg,
				})
				if err != nil {
					log.Err(err).Msg("error handling message")
				}
			}
		}

	}()

	// Consumer wrapper noop
	return &consumerWrapper{}, nil
}

type consumerWrapper struct{}

func (c *consumerWrapper) Unsubscribe() error {
	return nil
}
