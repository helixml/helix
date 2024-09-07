package openai

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type GetClientRequest struct {
	Provider types.Provider
}

//go:generate mockgen -source $GOFILE -destination manager_mocks.go -package $GOPACKAGE

// ProviderManager returns an OpenAI compatible client based on provider
type ProviderManager interface {
	// GetClient returns a client for the given provider
	GetClient(ctx context.Context, req *GetClientRequest) (Client, error)
	// ListProviders returns a list of providers that are available
	ListProviders(ctx context.Context) ([]types.Provider, error)
}

type providerClient struct {
	client Client
}

type MultiClientManager struct {
	clients   map[types.Provider]*providerClient
	clientsMu *sync.RWMutex
}

func NewProviderManager(cfg *config.ServerConfig, helixInference Client) *MultiClientManager {
	clients := make(map[types.Provider]*providerClient)

	if cfg.Providers.OpenAI.APIKey != "" {
		log.Info().
			Str("base_url", cfg.Providers.OpenAI.BaseURL).
			Msg("initializing OpenAI client")

		openaiClient := New(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL)

		clients[types.ProviderOpenAI] = &providerClient{client: openaiClient}
	}

	if cfg.Providers.TogetherAI.APIKey != "" {
		log.Info().
			Str("base_url", cfg.Providers.TogetherAI.BaseURL).
			Msg("using TogetherAI provider for controller inference")

		togetherAiClient := New(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL)

		clients[types.ProviderTogetherAI] = &providerClient{client: togetherAiClient}
	}

	// Always configure Helix provider too
	clients[types.ProviderHelix] = &providerClient{client: helixInference}

	return &MultiClientManager{
		clients:   clients,
		clientsMu: &sync.RWMutex{},
	}
}

func (m *MultiClientManager) ListProviders(ctx context.Context) ([]types.Provider, error) {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	providers := make([]types.Provider, 0, len(m.clients))
	for provider := range m.clients {
		providers = append(providers, provider)
	}

	return providers, nil
}

func (m *MultiClientManager) GetClient(ctx context.Context, req *GetClientRequest) (Client, error) {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	client, ok := m.clients[req.Provider]
	if !ok {
		return nil, fmt.Errorf("no client found for provider: %s", req.Provider)
	}

	return client.client, nil
}
