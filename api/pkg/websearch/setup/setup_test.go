package setup

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/websearch/exa"
	"github.com/helixml/helix/api/pkg/websearch/native"
)

func TestNewProviderExa(t *testing.T) {
	res, err := NewProvider(config.WebSearch{Provider: ProviderExa, ExaAPIKey: "k", ExaBaseURL: "https://api.exa.ai"}, Deps{})
	require.NoError(t, err)
	require.IsType(t, &exa.Provider{}, res.Provider)
	require.Nil(t, res.EgressProxy)
}

func TestNewProviderExaRequiresKey(t *testing.T) {
	_, err := NewProvider(config.WebSearch{Provider: ProviderExa, ExaBaseURL: "https://api.exa.ai"}, Deps{})
	require.ErrorContains(t, err, "API key")
}

func TestNewProviderUnknown(t *testing.T) {
	_, err := NewProvider(config.WebSearch{Provider: "bing"}, Deps{})
	require.ErrorContains(t, err, `unknown provider "bing"`)
}

func TestNewProviderNativeWithoutLLMOrEmbeddings(t *testing.T) {
	res, err := NewProvider(config.WebSearch{
		Provider:         ProviderNative,
		SearXNGBaseURL:   "http://searxng:8080",
		SearXNGEngines:   []string{"yandex"},
		FetchConcurrency: 4,
	}, Deps{})
	require.NoError(t, err)
	require.IsType(t, &native.Provider{}, res.Provider)
	require.NotNil(t, res.EgressProxy)
}

func TestLLMModelsDefaultToMainModel(t *testing.T) {
	require.Equal(t, map[native.Tier]string{
		native.TierFast:    "main",
		native.TierDefault: "main",
		native.TierStrong:  "big",
	}, llmModels(config.WebSearch{LLMModel: "main", LLMStrongModel: "big"}))
}

func TestNewProviderNativeRejectsUnsupportedEngines(t *testing.T) {
	for _, engines := range [][]string{nil, {"yandex", "startpage"}} {
		_, err := NewProvider(config.WebSearch{
			Provider:         ProviderNative,
			SearXNGBaseURL:   "http://searxng:8080",
			SearXNGEngines:   engines,
			FetchConcurrency: 4,
		}, Deps{})
		require.ErrorContains(t, err, "SEARCH_SEARXNG_ENGINES", "engines %v", engines)
	}
}
