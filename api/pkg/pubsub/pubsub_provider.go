package pubsub

import "time"

type Provider string

const (
	ProviderMemory Provider = "inmemory"
	// TODO: NATS/Redis
)

func New() (PubSub, error) {
	// TODO: switch on the provider type
	return NewInMemoryNats()
}

type Config struct {
	Namespace string

	Provider Provider

	HealthInterval time.Duration
	SendTimeout    time.Duration
	ChannelSize    int
}
