package pubsub

import "time"

type Provider string

const (
	ProviderMemory Provider = "inmemory"
	// TODO: NATS/Redis
)

func New() PubSub {
	// TODO: switch on the provider type
	return NewInMemory()
}

type Config struct {
	Namespace string

	Provider Provider

	HealthInterval time.Duration
	SendTimeout    time.Duration
	ChannelSize    int
}
