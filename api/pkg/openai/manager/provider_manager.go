package manager

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type GetClientRequest struct {
	Provider string
	Owner    string
	AppID    string
}

// RunnerControllerStatus defines the minimum interface needed to check runner status
type RunnerControllerStatus interface {
	RunnerIDs() []string
}

//go:generate mockgen -source $GOFILE -destination manager_mocks.go -package $GOPACKAGE

// ProviderManager returns an OpenAI compatible client based on provider
type ProviderManager interface {
	// GetClient returns a client for the given provider
	GetClient(ctx context.Context, req *GetClientRequest) (openai.Client, error)
	// ListProviders returns a list of providers that are available
	ListProviders(ctx context.Context, owner string) ([]types.Provider, error)
	// ListProviderEndpoints returns the full provider records visible to the
	// owner: synthetic entries for env-baked global providers (ID="",
	// Name=canonical) plus DB-backed user/org provider records. Used by
	// agent-config code paths that need to resolve an agent's stored
	// provider reference (an immutable ID for DB-backed, the canonical name
	// for globals) to the current canonical name.
	ListProviderEndpoints(ctx context.Context, owner string) ([]*types.ProviderEndpoint, error)
	// SetRunnerController sets the runner controller for checking runner availability
	SetRunnerController(controller RunnerControllerStatus)
}

type providerClient struct {
	client openai.Client
}

type MultiClientManager struct {
	cfg               *config.ServerConfig
	store             store.Store
	modelInfoProvider model.ModelInfoProvider
	billingLogger     logger.LogStore
	logStores         []logger.LogStore
	globalClients     map[types.Provider]*providerClient
	globalClientsMu   *sync.RWMutex
	wg                sync.WaitGroup
	mu                sync.RWMutex
}

func NewProviderManager(cfg *config.ServerConfig, store store.Store, helixInference openai.Client, modelInfoProvider model.ModelInfoProvider, logStores ...logger.LogStore) *MultiClientManager {
	clients := make(map[types.Provider]*providerClient)

	billingLogger, err := logger.NewBillingLogger(store, cfg.Stripe.BillingEnabled)
	if err != nil {
		log.Error().Err(err).Msg("failed to initialize billing logger")
	}

	// TLS options for all OpenAI-compatible clients
	tlsOpts := openai.ClientOptions{
		TLSSkipVerify: cfg.Tools.TLSSkipVerify,
	}

	// Log TLS configuration prominently for debugging enterprise deployments
	log.Info().
		Bool("tls_skip_verify", cfg.Tools.TLSSkipVerify).
		Str("env_var", "TOOLS_TLS_SKIP_VERIFY").
		Str("how_to_set", "set in .env for Docker Compose, or extraEnv in Helm chart").
		Msg("Provider manager TLS configuration loaded")

	if cfg.Providers.OpenAI.APIKey != "" {
		log.Info().
			Str("base_url", cfg.Providers.OpenAI.BaseURL).
			Bool("tls_skip_verify", cfg.Tools.TLSSkipVerify).
			Msg("initializing OpenAI client")

		openaiClient := openai.NewWithOptions(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL,
			cfg.Stripe.BillingEnabled,
			tlsOpts,
			cfg.Providers.OpenAI.Models...)

		loggedClient := logger.Wrap(cfg, types.ProviderOpenAI, openaiClient, modelInfoProvider, billingLogger, logStores...)

		clients[types.ProviderOpenAI] = &providerClient{client: loggedClient}
	}

	if cfg.Providers.TogetherAI.APIKey != "" {
		log.Info().
			Str("base_url", cfg.Providers.TogetherAI.BaseURL).
			Msg("using TogetherAI provider for controller inference")

		togetherAiClient := openai.NewWithOptions(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL,
			cfg.Stripe.BillingEnabled,
			tlsOpts,
			cfg.Providers.TogetherAI.Models...)

		loggedClient := logger.Wrap(cfg, types.ProviderTogetherAI, togetherAiClient, modelInfoProvider, billingLogger, logStores...)

		clients[types.ProviderTogetherAI] = &providerClient{client: loggedClient}
	}

	if cfg.Providers.Anthropic.APIKey != "" {
		log.Info().
			Str("base_url", cfg.Providers.Anthropic.BaseURL).
			Msg("using Anthropic provider for controller inference")

		// The go-openai client sends chat completions to {baseURL}/chat/completions.
		// ANTHROPIC_BASE_URL is typically set without /v1 (e.g., "https://api.anthropic.com"
		// or "http://outer-helix:8081") because the Anthropic SDK appends /v1/messages itself.
		// For the go-openai client path (used by agents), we need /v1 in the base URL.
		anthropicBaseURL := cfg.Providers.Anthropic.BaseURL
		if anthropicBaseURL != "" && !strings.HasSuffix(anthropicBaseURL, "/v1") {
			anthropicBaseURL = strings.TrimSuffix(anthropicBaseURL, "/") + "/v1"
		}

		anthropicClient := openai.NewWithOptions(
			cfg.Providers.Anthropic.APIKey,
			anthropicBaseURL,
			cfg.Stripe.BillingEnabled,
			tlsOpts,
			cfg.Providers.Anthropic.Models...)

		// Mark as Anthropic provider so ListModels uses the correct API format
		anthropicClient.SetIsAnthropic(true)

		loggedClient := logger.Wrap(cfg, types.ProviderAnthropic, anthropicClient, modelInfoProvider, billingLogger, logStores...)

		clients[types.ProviderAnthropic] = &providerClient{client: loggedClient}
	}

	// For VLLM, as long as the base URL is set, we can use it
	if cfg.Providers.VLLM.BaseURL != "" {
		log.Info().
			Str("base_url", cfg.Providers.VLLM.BaseURL).
			Msg("using VLLM provider for controller inference")

		vllmClient := openai.NewWithOptions(
			cfg.Providers.VLLM.APIKey,
			cfg.Providers.VLLM.BaseURL,
			cfg.Stripe.BillingEnabled,
			tlsOpts,
			cfg.Providers.VLLM.Models...)

		loggedClient := logger.Wrap(cfg, types.ProviderVLLM, vllmClient, modelInfoProvider, billingLogger, logStores...)

		clients[types.ProviderVLLM] = &providerClient{client: loggedClient}
	}

	// Always configure Helix provider too
	loggedClient := logger.Wrap(cfg, types.ProviderHelix, helixInference, modelInfoProvider, billingLogger, logStores...)

	clients[types.ProviderHelix] = &providerClient{client: loggedClient}

	mcm := &MultiClientManager{
		cfg:               cfg,
		store:             store,
		modelInfoProvider: modelInfoProvider,
		logStores:         logStores,
		billingLogger:     billingLogger,
		globalClients:     clients,
		globalClientsMu:   &sync.RWMutex{},
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

	if m.cfg.Providers.Anthropic.APIKeyFromFile != "" {
		err := m.watchAndUpdateClient(ctx, types.ProviderAnthropic, m.cfg.Providers.Anthropic.APIKeyRefreshInterval, m.cfg.Providers.Anthropic.BaseURL, m.cfg.Providers.Anthropic.APIKeyFromFile)
		if err != nil {
			log.Error().Err(err).Msg("error watching and updating Anthropic client")
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

	m.globalClientsMu.RLock()
	client, ok := m.globalClients[provider]
	m.globalClientsMu.RUnlock()

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
	openaiClient := openai.NewWithOptions(newKey, baseURL, m.cfg.Stripe.BillingEnabled, openai.ClientOptions{
		TLSSkipVerify: m.cfg.Tools.TLSSkipVerify,
	})

	// Preserve x-api-key auth across key rotation for the Anthropic global provider.
	if provider == types.ProviderAnthropic {
		openaiClient.SetIsAnthropic(true)
	}

	loggedClient := logger.Wrap(m.cfg, provider, openaiClient, m.modelInfoProvider, m.billingLogger, m.logStores...)

	m.globalClientsMu.Lock()
	m.globalClients[provider] = &providerClient{client: loggedClient}
	m.globalClientsMu.Unlock()

	return nil
}

// SetRunnerController implements ProviderManager.SetRunnerController.
// Sandbox-absorbs-runner pivot: this is now a no-op. Runner availability
// is checked by the inferencerouter at request-routing time, not at
// provider-listing time. The helix provider is always listed; individual
// model availability flows via /v1/models from the router.
func (m *MultiClientManager) SetRunnerController(_ RunnerControllerStatus) {
	// no-op
}

func (m *MultiClientManager) ListProviders(ctx context.Context, owner string) ([]types.Provider, error) {
	m.globalClientsMu.RLock()
	defer m.globalClientsMu.RUnlock()

	providers := make([]types.Provider, 0, len(m.globalClients))
	for provider := range m.globalClients {
		providers = append(providers, provider)
	}

	// If no owner is provided, return only the global providers configured
	// from the environment
	if owner == "" {
		return providers, nil
	}

	userProviders, err := m.store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      owner,
		WithGlobal: true,
	})
	if err != nil {
		return nil, err
	}

	for _, provider := range userProviders {
		providers = append(providers, types.Provider(provider.Name))
	}

	return providers, nil
}

// ListProviderEndpoints returns full provider records visible to the owner.
// Env-baked globals are returned as synthetic *ProviderEndpoint values with
// ID="" and Name=canonical; DB-backed user/org providers are returned with
// their real ID and current admin-set Name. Use this when callers need to
// resolve an agent's stored provider reference to its current canonical name
// — the agent record stores the immutable ID, settings.json carries the
// current name. Renames flow into running sessions on the next sync poll.
func (m *MultiClientManager) ListProviderEndpoints(ctx context.Context, owner string) ([]*types.ProviderEndpoint, error) {
	// Always list global providers including Helix — runner availability is
	// checked by the inferencerouter at request-routing time post sandbox-
	// absorbs-runner pivot, not at provider-listing time. Matches
	// ListProviders and the SetRunnerController no-op contract.
	m.globalClientsMu.RLock()
	endpoints := make([]*types.ProviderEndpoint, 0, len(m.globalClients))
	for provider := range m.globalClients {
		endpoints = append(endpoints, &types.ProviderEndpoint{
			Name:         string(provider),
			EndpointType: types.ProviderEndpointTypeGlobal,
		})
	}
	m.globalClientsMu.RUnlock()

	if owner == "" {
		return endpoints, nil
	}

	userProviders, err := m.store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      owner,
		WithGlobal: true,
	})
	if err != nil {
		return nil, err
	}
	endpoints = append(endpoints, userProviders...)
	return endpoints, nil
}

func (m *MultiClientManager) GetClient(_ context.Context, req *GetClientRequest) (openai.Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if req == nil {
		req = &GetClientRequest{}
	}

	if req.AppID != "" {
		log.Info().
			Str("app_id", req.AppID).
			Str("provider", req.Provider).
			Str("owner", req.Owner).
			Msg("TRACE: Provider manager GetClient called with app ID")
	}

	m.globalClientsMu.RLock()
	defer m.globalClientsMu.RUnlock()

	client, ok := m.globalClients[types.Provider(req.Provider)]
	if ok {
		return client.client, nil
	}

	userProviders, err := m.store.ListProviderEndpoints(context.Background(), &store.ListProviderEndpointsQuery{
		Owner:      req.Owner,
		WithGlobal: true,
	})
	if err != nil {
		return nil, err
	}

	for _, provider := range userProviders {
		if provider.Name == req.Provider || provider.ID == req.Provider {
			return m.initializeClient(provider)
		}
	}

	// Build list of all available providers for error message
	availableProviders := make([]string, 0, len(m.globalClients)+len(userProviders))
	for provider := range m.globalClients {
		availableProviders = append(availableProviders, string(provider))
	}
	for _, provider := range userProviders {
		availableProviders = append(availableProviders, provider.Name)
	}
	return nil, fmt.Errorf("no client found for provider: %s, available providers: [%s]", req.Provider, strings.Join(availableProviders, ", "))
}

// isAnthropicAPIEndpoint reports whether a DB/UI-configured provider endpoint
// targets the direct Anthropic API and therefore needs x-api-key auth (not
// Bearer) for /v1/models. Detection keys off the canonical provider name
// (matching the frontend's `name === 'anthropic'` convention) or an
// api.anthropic.com base URL. Vertex endpoints are excluded — they authenticate
// via OAuth, not x-api-key.
// ponytail: name/URL heuristic; the proper fix is an explicit provider-kind /
// api-format field on ProviderEndpoint, which would also catch Anthropic
// endpoints renamed and pointed at a proxy (not matched here today).
func isAnthropicAPIEndpoint(endpoint *types.ProviderEndpoint) bool {
	if endpoint.VertexProjectID != "" {
		return false
	}
	if strings.EqualFold(endpoint.Name, string(types.ProviderAnthropic)) {
		return true
	}
	// Host-exact match so a lookalike like api.anthropic.com.proxy.evil.com
	// does not match.
	if u, err := url.Parse(endpoint.BaseURL); err == nil {
		return strings.EqualFold(u.Hostname(), "api.anthropic.com")
	}
	return false
}

func (m *MultiClientManager) initializeClient(endpoint *types.ProviderEndpoint) (openai.Client, error) {
	apiKey := endpoint.APIKey
	if endpoint.APIKeyFromFile != "" {
		bts, err := os.ReadFile(endpoint.APIKeyFromFile)
		if err != nil {
			return nil, fmt.Errorf("error reading API key file '%s': %w", endpoint.APIKeyFromFile, err)
		}
		apiKey = strings.TrimSpace(string(bts))
	}

	// Log TLS configuration for database-configured providers (user/org endpoints)
	// This helps debug enterprise TLS issues with providers configured via web UI
	log.Trace().
		Str("provider_id", endpoint.ID).
		Str("provider_name", endpoint.Name).
		Str("base_url", endpoint.BaseURL).
		Str("endpoint_type", string(endpoint.EndpointType)).
		Bool("tls_skip_verify", m.cfg.Tools.TLSSkipVerify).
		Msg("Initializing client for database-configured provider with TLS config")

	openaiClient := openai.NewWithOptions(apiKey, endpoint.BaseURL, endpoint.BillingEnabled, openai.ClientOptions{
		TLSSkipVerify: m.cfg.Tools.TLSSkipVerify,
	}, endpoint.Models...)

	// DB/UI-configured Anthropic providers must use x-api-key auth (not Bearer) for
	// /v1/models. See isAnthropicAPIEndpoint.
	if isAnthropicAPIEndpoint(endpoint) {
		openaiClient.SetIsAnthropic(true)
	}

	// If it's a personal endpoint, replace the billing logger with a NoopBillingLogger
	billingLogger := m.billingLogger
	if !endpoint.BillingEnabled && (endpoint.EndpointType == types.ProviderEndpointTypeUser || endpoint.EndpointType == types.ProviderEndpointTypeOrg) {
		billingLogger = &logger.NoopBillingLogger{}
	}

	loggedClient := logger.Wrap(m.cfg, types.Provider(endpoint.ID), openaiClient, m.modelInfoProvider, billingLogger, m.logStores...)

	return loggedClient, nil
}
