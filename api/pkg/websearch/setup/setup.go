// Package setup builds the deployment's websearch.Provider from
// configuration: the one place that knows how the native components fit
// together.
package setup

import (
	"fmt"
	"slices"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/websearch"
	"github.com/helixml/helix/api/pkg/websearch/exa"
	"github.com/helixml/helix/api/pkg/websearch/native"
	"github.com/helixml/helix/api/pkg/websearch/pagecache"
)

// Provider names accepted in SEARCH_PROVIDER.
const (
	ProviderNative = "native"
	ProviderExa    = "exa"
)

// SearchEngines are the SearXNG engines SEARCH_SEARXNG_ENGINES may name. Our
// SearXNG settings enable each of them.
var SearchEngines = []string{"google", "bing", "duckduckgo", "brave", "yandex"}

// Deps are the shared services the native provider is built from.
type Deps struct {
	Store     store.Store
	Providers manager.ProviderManager
	Documents extract.Extractor
	Browser   *browser.Browser
}

// Result is the configured provider and, for the native provider, the egress
// proxy Chrome's page fetches must go through. The caller serves EgressProxy
// on cfg.BrowserProxyListenAddr; it is nil for the exa provider.
type Result struct {
	Provider    websearch.Provider
	EgressProxy *netguard.Proxy
}

func NewProvider(cfg config.WebSearch, deps Deps) (*Result, error) {
	switch cfg.Provider {
	case ProviderExa:
		p, err := exa.New(cfg.ExaAPIKey, cfg.ExaBaseURL)
		if err != nil {
			return nil, fmt.Errorf("web search: %w", err)
		}
		return &Result{Provider: p}, nil
	case ProviderNative:
		return newNative(cfg, deps)
	default:
		return nil, fmt.Errorf("web search: unknown provider %q (want %q or %q)", cfg.Provider, ProviderNative, ProviderExa)
	}
}

func newNative(cfg config.WebSearch, deps Deps) (*Result, error) {
	engines, err := searchEngines(cfg.SearXNGEngines)
	if err != nil {
		return nil, err
	}
	policy := netguard.Policy{AllowPrivate: cfg.AllowPrivateNetworks}

	proxy, err := netguard.NewProxy(policy)
	if err != nil {
		return nil, fmt.Errorf("web search egress proxy: %w", err)
	}
	username, password := proxy.Credentials()

	fetcher, err := native.NewFetcher(native.FetcherOptions{
		Policy:        policy,
		Browser:       deps.Browser,
		ProxyURL:      cfg.BrowserProxyURL,
		ProxyUsername: username,
		ProxyPassword: password,
	})
	if err != nil {
		return nil, fmt.Errorf("web search fetcher: %w", err)
	}

	opts := native.Options{
		Discoverer:       native.NewSearXNGDiscoverer(searxng.NewClient(cfg.SearXNGBaseURL, engines)),
		Fetcher:          fetcher,
		Extractor:        native.NewExtractor(deps.Documents),
		Cache:            pagecache.New(deps.Store),
		FetchConcurrency: cfg.FetchConcurrency,
	}
	if cfg.EmbeddingsProvider != "" {
		opts.Ranker = native.NewEmbeddingRanker(deps.Providers, cfg.EmbeddingsProvider, cfg.EmbeddingsModel)
	}
	if cfg.LLMProvider != "" {
		synth, err := native.NewLLMSynthesizer(deps.Providers, cfg.LLMProvider, llmModels(cfg))
		if err != nil {
			return nil, fmt.Errorf("web search synthesizer: %w", err)
		}
		opts.Synthesizer = synth
	}

	provider, err := native.New(opts)
	if err != nil {
		return nil, fmt.Errorf("web search: %w", err)
	}
	return &Result{Provider: provider, EgressProxy: proxy}, nil
}

// llmModels maps each tier to its model. The fast and strong tiers use the
// default model unless configured.
func llmModels(cfg config.WebSearch) map[native.Tier]string {
	models := map[native.Tier]string{
		native.TierFast:    cfg.LLMFastModel,
		native.TierDefault: cfg.LLMModel,
		native.TierStrong:  cfg.LLMStrongModel,
	}
	for tier, model := range models {
		if model == "" {
			models[tier] = cfg.LLMModel
		}
	}
	return models
}

func searchEngines(names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("web search: SEARCH_SEARXNG_ENGINES is empty (choose from %s)", strings.Join(SearchEngines, ", "))
	}
	for _, n := range names {
		if !slices.Contains(SearchEngines, n) {
			return nil, fmt.Errorf("web search: unsupported search engine %q in SEARCH_SEARXNG_ENGINES (choose from %s)", n, strings.Join(SearchEngines, ", "))
		}
	}
	return names, nil
}
