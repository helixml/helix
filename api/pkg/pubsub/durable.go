package pubsub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/rs/zerolog/log"
)

func (n *Nats) EnsurePersistentStream(ctx context.Context, name string, subjects []string) error {
	_, err := n.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      name,
		Subjects:  subjects,
		Retention: jetstream.WorkQueuePolicy,
		Discard:   jetstream.DiscardOld,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		return fmt.Errorf("ensure persistent stream %q: %w", name, err)
	}
	return nil
}

func (n *Nats) PublishDurable(ctx context.Context, stream, subject string, payload []byte) error {
	if _, err := n.js.Publish(ctx, subject, payload, jetstream.WithExpectStream(stream)); err != nil {
		return fmt.Errorf("publish to persistent stream %q: %w", stream, err)
	}
	return nil
}

func (n *Nats) ConsumeDurable(ctx context.Context, streamName, consumerName, subject string, ackWait time.Duration, handler func(msg *Message) error) (Subscription, error) {
	stream, err := n.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("get persistent stream %q: %w", streamName, err)
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          consumerName,
		Durable:       consumerName,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxAckPending: 1,
		FilterSubject: subject,
		ReplayPolicy:  jetstream.ReplayInstantPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure durable consumer %q: %w", consumerName, err)
	}
	messages, err := consumer.Messages(jetstream.PullMaxMessages(1))
	if err != nil {
		return nil, fmt.Errorf("open durable consumer %q: %w", consumerName, err)
	}
	sub := &durableSubscription{messages: messages, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			sub.Unsubscribe()
		case <-sub.done:
		}
	}()
	go func() {
		defer sub.Unsubscribe()
		for {
			msg, err := messages.Next()
			if err != nil {
				if ctx.Err() == nil {
					log.Error().Err(err).Str("consumer", consumerName).Msg("durable consumer stopped")
				}
				return
			}
			if err := handler(&Message{Data: msg.Data(), Header: msg.Headers(), msg: msg}); err != nil {
				log.Error().Err(err).Str("consumer", consumerName).Msg("durable consumer handler failed")
			}
		}
	}()
	return sub, nil
}

func (n *Nats) ListDurableConsumers(ctx context.Context, streamName string) ([]DurableConsumer, error) {
	stream, err := n.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("get persistent stream %q: %w", streamName, err)
	}
	list := stream.ListConsumers(ctx)
	consumers := make([]DurableConsumer, 0)
	for info := range list.Info() {
		consumers = append(consumers, DurableConsumer{Name: info.Name, Subject: info.Config.FilterSubject})
	}
	if err := list.Err(); err != nil {
		return nil, fmt.Errorf("list durable consumers for %q: %w", streamName, err)
	}
	return consumers, nil
}

func (n *Nats) DeleteDurableConsumer(ctx context.Context, streamName, consumerName string) error {
	stream, err := n.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("get persistent stream %q: %w", streamName, err)
	}
	if err := stream.DeleteConsumer(ctx, consumerName); err != nil && !errors.Is(err, jetstream.ErrConsumerNotFound) {
		return fmt.Errorf("delete durable consumer %q: %w", consumerName, err)
	}
	return nil
}

func (n *Nats) PurgeDurableSubject(ctx context.Context, streamName, subject string) error {
	stream, err := n.js.Stream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("get persistent stream %q: %w", streamName, err)
	}
	if err := stream.Purge(ctx, jetstream.WithPurgeSubject(subject)); err != nil {
		return fmt.Errorf("purge durable subject %q: %w", subject, err)
	}
	return nil
}

type durableSubscription struct {
	messages jetstream.MessagesContext
	done     chan struct{}
	once     sync.Once
}

func (s *durableSubscription) Unsubscribe() error {
	s.once.Do(func() {
		close(s.done)
		s.messages.Stop()
	})
	return nil
}
