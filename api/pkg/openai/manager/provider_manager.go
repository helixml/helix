package manager

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/types"
)

type GetClientRequest struct {
	Provider types.Provider
}

//go:generate mockgen -source $GOFILE -destination manager_mocks.go -package $GOPACKAGE

// ProviderManager returns an OpenAI compatible client based on provider
type ProviderManager interface {
	// GetClient returns a client for the given provider
	GetClient(ctx context.Context, req *GetClientRequest) (openai.Client, error)
	// ListProviders returns a list of providers that are available
	ListProviders(ctx context.Context) ([]types.Provider, error)
}

type providerClient struct {
	client openai.Client
}

type MultiClientManager struct {
	cfg       *config.ServerConfig
	logStores []logger.LogStore
	clients   map[types.Provider]*providerClient
	clientsMu *sync.RWMutex
}

func NewProviderManager(cfg *config.ServerConfig, helixInference openai.Client, logStores ...logger.LogStore) *MultiClientManager {
	clients := make(map[types.Provider]*providerClient)

	if cfg.Providers.OpenAI.APIKey != "" {
		log.Info().
			Str("base_url", cfg.Providers.OpenAI.BaseURL).
			Msg("initializing OpenAI client")

		openaiClient := openai.New(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL)

		loggedClient := logger.Wrap(cfg, types.ProviderOpenAI, openaiClient, logStores...)

		clients[types.ProviderOpenAI] = &providerClient{client: loggedClient}
	}

	if cfg.Providers.TogetherAI.APIKey != "" {
		log.Info().
			Str("base_url", cfg.Providers.TogetherAI.BaseURL).
			Msg("using TogetherAI provider for controller inference")

		togetherAiClient := openai.New(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL)

		loggedClient := logger.Wrap(cfg, types.ProviderTogetherAI, togetherAiClient, logStores...)

		clients[types.ProviderTogetherAI] = &providerClient{client: loggedClient}
	}

	// Always configure Helix provider too

	loggedClient := logger.Wrap(cfg, types.ProviderHelix, helixInference, logStores...)

	clients[types.ProviderHelix] = &providerClient{client: loggedClient}

	return &MultiClientManager{
		cfg:       cfg,
		logStores: logStores,
		clients:   clients,
		clientsMu: &sync.RWMutex{},
	}
}

func (m *MultiClientManager) StartRefresh(ctx context.Context) {
	if m.cfg.Providers.OpenAI.APIKeyFromFile != "" {
		go func() {
			err := m.watchAndUpdateClient(ctx, types.ProviderOpenAI, m.cfg.Providers.OpenAI.APIKeyRefreshInterval, m.cfg.Providers.OpenAI.BaseURL, m.cfg.Providers.OpenAI.APIKeyFromFile)
			if err != nil {
				log.Error().Err(err).Msg("error watching and updating OpenAI client")
			}
		}()
	}

	if m.cfg.Providers.TogetherAI.APIKeyFromFile != "" {
		go func() {
			err := m.watchAndUpdateClient(ctx, types.ProviderTogetherAI, m.cfg.Providers.TogetherAI.APIKeyRefreshInterval, m.cfg.Providers.TogetherAI.BaseURL, m.cfg.Providers.TogetherAI.APIKeyFromFile)
			if err != nil {
				log.Error().Err(err).Msg("error watching and updating TogetherAI client")
			}
		}()
	}

	<-ctx.Done()
}

func (m *MultiClientManager) watchAndUpdateClient(ctx context.Context, provider types.Provider, interval time.Duration, baseURL, keyFile string) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var apiKey string

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			bts, err := os.ReadFile(keyFile)
			if err != nil {
				log.Error().
					Str("file", keyFile).
					Err(err).
					Msg("error reading API key file")
				continue
			}

			newKey := strings.TrimSpace(string(bts))
			if newKey == apiKey {
				continue
			}

			// Recreate the client with the new key
			openaiClient := openai.New(newKey, baseURL)

			loggedClient := logger.Wrap(m.cfg, provider, openaiClient, m.logStores...)

			m.clientsMu.Lock()
			m.clients[provider] = &providerClient{client: loggedClient}
			m.clientsMu.Unlock()

			apiKey = newKey
		}
	}
}

func (m *MultiClientManager) ListProviders(_ context.Context) ([]types.Provider, error) {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	providers := make([]types.Provider, 0, len(m.clients))
	for provider := range m.clients {
		providers = append(providers, provider)
	}

	return providers, nil
}

func (m *MultiClientManager) GetClient(_ context.Context, req *GetClientRequest) (openai.Client, error) {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	client, ok := m.clients[req.Provider]
	if !ok {
		return nil, fmt.Errorf("no client found for provider: %s", req.Provider)
	}

	return client.client, nil
}
