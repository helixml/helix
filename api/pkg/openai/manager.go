package openai

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
)

type GetClientRequest struct {
	Provider types.Provider
	Model    string
}

// Manager returns an OpenAI compatible client based on provider
type Manager interface {
	GetClient(ctx context.Context, req *GetClientRequest) (Client, error)
	ListModels(ctx context.Context, provider types.Provider) ([]model.OpenAIModel, error)
}

type providerClient struct {
	client Client
}

type MultiClientManager struct {
	clients   map[types.Provider]*providerClient
	clientsMu *sync.RWMutex
}

func NewManager(cfg *config.ServerConfig, helixInference Client) *MultiClientManager {
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

	if cfg.Inference.Provider == types.ProviderHelix {
		clients[types.ProviderHelix] = &providerClient{client: helixInference}
	}

	return &MultiClientManager{
		clients:   clients,
		clientsMu: &sync.RWMutex{},
	}
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
