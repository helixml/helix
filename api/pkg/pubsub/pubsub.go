package pubsub

import (
	"context"
	"time"
)

type Publisher interface {
	// Publish topic to message broker with payload.
	Publish(ctx context.Context, topic string, payload []byte, opts ...PublishOption) error
}

type PubSub interface {
	Publisher
	// Subscribe consumer to process the topic with payload, this should be
	// blocking operation.
	Subscribe(ctx context.Context, topic string, handler func(payload []byte) error, options ...SubscribeOption) Consumer
}

type Consumer interface {
	Subscribe(ctx context.Context, topics ...string) error
	Unsubscribe(ctx context.Context, topics ...string) error
	Close() error
}

type PublishConfig struct {
	namespace string
}

type PublishOption interface {
	Apply(*PublishConfig)
}

// PublishOptionFunc is a function that configures a publish config.
type PublishOptionFunc func(*PublishConfig)

// Apply calls f(publishConfig).
func (f PublishOptionFunc) Apply(config *PublishConfig) {
	f(config)
}

// WithPublishNamespace modifies publish config namespace.
func WithPublishNamespace(value string) PublishOption {
	return PublishOptionFunc(func(c *PublishConfig) {
		c.namespace = value
	})
}

func formatTopic(ns, topic string) string {
	return ns + ":" + topic
}

type SubscribeConfig struct {
	topics         []string
	namespace      string
	healthInterval time.Duration
	sendTimeout    time.Duration
	channelSize    int
}

// SubscribeOption configures a subscription config.
type SubscribeOption interface {
	Apply(*SubscribeConfig)
}

// SubscribeOptionFunc is a function that configures a subscription config.
type SubscribeOptionFunc func(*SubscribeConfig)

// Apply calls f(subscribeConfig).
func (f SubscribeOptionFunc) Apply(config *SubscribeConfig) {
	f(config)
}

// WithNamespace returns an channel option that configures namespace.
func WithChannelNamespace(value string) SubscribeOption {
	return SubscribeOptionFunc(func(c *SubscribeConfig) {
		c.namespace = value
	})
}
