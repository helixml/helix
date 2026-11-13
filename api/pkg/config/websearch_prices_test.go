package config

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"
)

func TestWebSearchPricesDefaultToExaListPrices(t *testing.T) {
	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))
	require.Equal(t, WebSearchPrices{
		SearchLite: 0.004, Search: 0.007, SearchDeep: 0.012, ResultOver10: 0.001,
		ContentPage: 0.001, Answer: 0.005, FindSimilar: 0.007,
	}, cfg.WebSearch.Prices)
}

func TestWebSearchPricesFromEnv(t *testing.T) {
	for name, value := range map[string]string{
		"SEARCH_PRICE_SEARCH_LITE":    "1",
		"SEARCH_PRICE_SEARCH":         "2",
		"SEARCH_PRICE_SEARCH_DEEP":    "3",
		"SEARCH_PRICE_RESULT_OVER_10": "4",
		"SEARCH_PRICE_CONTENT_PAGE":   "5",
		"SEARCH_PRICE_ANSWER":         "6",
		"SEARCH_PRICE_FIND_SIMILAR":   "7",
	} {
		t.Setenv(name, value)
	}
	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))
	require.Equal(t, WebSearchPrices{
		SearchLite: 1, Search: 2, SearchDeep: 3, ResultOver10: 4,
		ContentPage: 5, Answer: 6, FindSimilar: 7,
	}, cfg.WebSearch.Prices)
}
