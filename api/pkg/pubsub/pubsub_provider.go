package pubsub

import (
	"github.com/helixml/helix/api/pkg/config"
)

type Provider string

const (
	ProviderMemory Provider = "inmemory"
	ProviderNATS   Provider = "nats"
)

func New(cfg *config.ServerConfig) (PubSub, error) {
	switch cfg.PubSub.Provider {
	case string(ProviderMemory):
		return NewInMemoryNats()
	case string(ProviderNATS):
		return NewNats(cfg)
	default:
		return NewNats(cfg) // Default to NATS
	}
}
