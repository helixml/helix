package pubsub

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
)

// NoopPubSub is a no-op implementation of PubSub for testing.
// All publishes are silently discarded. All subscriptions return
// immediately with a no-op subscription that can be safely unsubscribed.
type NoopPubSub struct{}

var _ PubSub = &NoopPubSub{}

func NewNoop() *NoopPubSub {
	return &NoopPubSub{}
}

func (n *NoopPubSub) Publish(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (n *NoopPubSub) PublishWithHeader(_ context.Context, _ string, _ map[string]string, _ []byte) error {
	return nil
}

func (n *NoopPubSub) Subscribe(_ context.Context, _ string, _ func(payload []byte) error) (Subscription, error) {
	return &noopSubscription{}, nil
}

func (n *NoopPubSub) SubscribeWithCtx(_ context.Context, _ string, _ func(ctx context.Context, msg *nats.Msg) error) (Subscription, error) {
	return &noopSubscription{}, nil
}

func (n *NoopPubSub) Request(_ context.Context, _ string, _ map[string]string, _ []byte, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (n *NoopPubSub) QueueRequest(_ context.Context, _, _ string, _ []byte, _ map[string]string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (n *NoopPubSub) QueueSubscribe(_ context.Context, _, _ string, _ func(msg *Message) error) (Subscription, error) {
	return &noopSubscription{}, nil
}

func (n *NoopPubSub) StreamRequest(_ context.Context, _, _ string, _ []byte, _ map[string]string, _ time.Duration) ([]byte, error) {
	return nil, nil
}

func (n *NoopPubSub) StreamConsume(_ context.Context, _, _ string, _ func(msg *Message) error) (Subscription, error) {
	return &noopSubscription{}, nil
}

func (n *NoopPubSub) OnConnectionStatus(_ ConnectionStatusHandler) {}

type noopSubscription struct{}

func (s *noopSubscription) Unsubscribe() error { return nil }
