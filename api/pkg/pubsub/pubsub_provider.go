package pubsub

import "time"

type Provider string

const (
	ProviderMemory Provider = "inmemory"
	// TODO: NATS/Redis
)

func New(storeDir string) (PubSub, error) {
	// TODO: switch on the provider type
	return NewInMemoryNats(storeDir)
}

type Config struct {
	Namespace string

	Provider Provider

	HealthInterval time.Duration
	SendTimeout    time.Duration
	ChannelSize    int
}
