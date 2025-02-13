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
	wg        sync.WaitGroup
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

	// For VLLM, as long as the base URL is set, we can use it
	if cfg.Providers.VLLM.BaseURL != "" {
		log.Info().
			Str("base_url", cfg.Providers.VLLM.BaseURL).
			Msg("using VLLM provider for controller inference")

		vllmClient := openai.New(
			cfg.Providers.VLLM.APIKey,
			cfg.Providers.VLLM.BaseURL)

		loggedClient := logger.Wrap(cfg, types.ProviderVLLM, vllmClient, logStores...)

		clients[types.ProviderVLLM] = &providerClient{client: loggedClient}
	}

	// Always configure Helix provider too

	loggedClient := logger.Wrap(cfg, types.ProviderHelix, helixInference, logStores...)

	clients[types.ProviderHelix] = &providerClient{client: loggedClient}

	mcm := &MultiClientManager{
		cfg:       cfg,
		logStores: logStores,
		clients:   clients,
		clientsMu: &sync.RWMutex{},
	}

	return mcm
}

func (m *MultiClientManager) StartRefresh(ctx context.Context) {
	if m.cfg.Providers.OpenAI.APIKeyFromFile != "" {
		err := m.watchAndUpdateClient(ctx, types.ProviderOpenAI, m.cfg.Providers.OpenAI.APIKeyRefreshInterval, m.cfg.Providers.OpenAI.BaseURL, m.cfg.Providers.OpenAI.APIKeyFromFile)
		if err != nil {
			log.Error().Err(err).Msg("error watching and updating OpenAI client")
		}
	}

	if m.cfg.Providers.TogetherAI.APIKeyFromFile != "" {
		err := m.watchAndUpdateClient(ctx, types.ProviderTogetherAI, m.cfg.Providers.TogetherAI.APIKeyRefreshInterval, m.cfg.Providers.TogetherAI.BaseURL, m.cfg.Providers.TogetherAI.APIKeyFromFile)
		if err != nil {
			log.Error().Err(err).Msg("error watching and updating TogetherAI client")
		}
	}

	if m.cfg.Providers.VLLM.APIKeyFromFile != "" {
		err := m.watchAndUpdateClient(ctx, types.ProviderVLLM, m.cfg.Providers.VLLM.APIKeyRefreshInterval, m.cfg.Providers.VLLM.BaseURL, m.cfg.Providers.VLLM.APIKeyFromFile)
		if err != nil {
			log.Error().Err(err).Msg("error watching and updating VLLM client")
		}
	}
}

func (m *MultiClientManager) watchAndUpdateClient(ctx context.Context, provider types.Provider, interval time.Duration, baseURL, keyFile string) error {

	// Initialize the client
	err := m.updateClientAPIKeyFromFile(provider, baseURL, keyFile)
	if err != nil {
		log.Warn().Str("provider", string(provider)).Err(err).Msg("error getting client API key from file, provider will not be available until it's available")
	}

	m.wg.Add(1)

	// Start watching for changes
	go func() {
		defer m.wg.Done()

		log.Info().Str("provider", string(provider)).Str("path", keyFile).Msg("starting to watch and update client")
		defer log.Info().Str("provider", string(provider)).Str("path", keyFile).Msg("stopped watching and updating client")

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				err := m.updateClientAPIKeyFromFile(provider, baseURL, keyFile)
				if err != nil {
					log.Error().Str("provider", string(provider)).Err(err).Msg("error updating client API key")
					continue
				}
			}
		}
	}()

	return nil
}

func (m *MultiClientManager) updateClientAPIKeyFromFile(provider types.Provider, baseURL, keyFile string) error {
	bts, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("error reading API key file '%s': %w", keyFile, err)
	}

	newKey := strings.TrimSpace(string(bts))

	m.clientsMu.RLock()
	client, ok := m.clients[provider]
	m.clientsMu.RUnlock()

	if ok && client.client.APIKey() == newKey {
		// Nothing to do
		return nil
	}

	// Log if we're creating a new client or updating an existing one
	if client == nil {
		log.Info().
			Str("provider", string(provider)).
			Str("path", keyFile).
			Msg("creating new OpenAI compatible client")
	} else {
		log.Info().
			Str("provider", string(provider)).
			Str("path", keyFile).
			Msg("API key updated, recreating OpenAI compatible client")
	}

	// Recreate the client with the new key
	openaiClient := openai.New(newKey, baseURL)

	loggedClient := logger.Wrap(m.cfg, provider, openaiClient, m.logStores...)

	m.clientsMu.Lock()
	m.clients[provider] = &providerClient{client: loggedClient}
	m.clientsMu.Unlock()

	return nil
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
		availableProviders := make([]string, 0, len(m.clients))
		for provider := range m.clients {
			availableProviders = append(availableProviders, string(provider))
		}
		return nil, fmt.Errorf("no client found for provider: %s, available providers: [%s]", req.Provider, strings.Join(availableProviders, ", "))
	}

	return client.client, nil
}
