package pricing

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
)

func TestCalculateTokenPrice(t *testing.T) {
	mf := &types.ModelInfo{
		Pricing: types.Pricing{
			Prompt:     "0.000000075",
			Completion: "0.00000015",
		},
	}

	cost, err := CalculateTokenPrice(mf, TokenUsage{
		PromptTokens:     10000000,
		CompletionTokens: 10000000,
	})
	assert.NoError(t, err)
	assert.Equal(t, 0.75, cost.PromptCost)
	assert.Equal(t, 1.5, cost.CompletionCost)
	assert.Equal(t, 0.0, cost.CacheReadCost)
	assert.Equal(t, 0.0, cost.CacheWriteCost)
	assert.Equal(t, 2.25, cost.Total())
}

func TestCalculateTokenPrice_CacheRead_OpenAIStyle(t *testing.T) {
	// OpenAI/Gemini report cached tokens inside prompt_tokens.
	mf := &types.ModelInfo{
		Pricing: types.Pricing{
			Prompt:         "0.0000025",
			Completion:     "0.000015",
			InputCacheRead: "0.00000025", // 10% of prompt
		},
	}

	cost, err := CalculateTokenPrice(mf, TokenUsage{
		PromptTokens:     1_000_000, // 800k new + 200k cached
		CompletionTokens: 100_000,
		CacheReadTokens:  200_000,
	})
	assert.NoError(t, err)
	// 800k * 2.5e-6 = 2.0
	assert.InDelta(t, 2.0, cost.PromptCost, 1e-9)
	// 100k * 15e-6 = 1.5
	assert.InDelta(t, 1.5, cost.CompletionCost, 1e-9)
	// 200k * 0.25e-6 = 0.05
	assert.InDelta(t, 0.05, cost.CacheReadCost, 1e-9)
}

func TestCalculateTokenPrice_CacheWriteAndRead_AnthropicStyle(t *testing.T) {
	// Anthropic reports cache creation/read separately from input_tokens,
	// but callers normalize by passing PromptTokens = input_tokens + cache_read + cache_write.
	mf := &types.ModelInfo{
		Pricing: types.Pricing{
			Prompt:          "0.000003",
			Completion:      "0.000015",
			InputCacheRead:  "0.0000003",  // 10% of prompt (hit)
			InputCacheWrite: "0.00000375", // 125% of prompt (5m ephemeral)
		},
	}

	// Anthropic response: input_tokens=50k (non-cached), cache_read=100k, cache_write=20k.
	// Caller folds: PromptTokens = 50k + 100k + 20k = 170k.
	cost, err := CalculateTokenPrice(mf, TokenUsage{
		PromptTokens:     170_000,
		CompletionTokens: 1_000,
		CacheReadTokens:  100_000,
		CacheWriteTokens: 20_000,
	})
	assert.NoError(t, err)
	// Non-cached prompt: 170k - 100k - 20k = 50k * 3e-6 = 0.15
	assert.InDelta(t, 0.15, cost.PromptCost, 1e-9)
	assert.InDelta(t, 0.015, cost.CompletionCost, 1e-9)
	// 100k * 0.3e-6 = 0.03
	assert.InDelta(t, 0.03, cost.CacheReadCost, 1e-9)
	// 20k * 3.75e-6 = 0.075
	assert.InDelta(t, 0.075, cost.CacheWriteCost, 1e-9)
}

func TestCalculateTokenPrice_MissingCacheRate_RollsIntoPrompt(t *testing.T) {
	// No cache rate published → we treat the model as not supporting cache billing
	// in that direction. Cached tokens are billed at the prompt rate, but reported
	// inside PromptCost (CacheReadCost stays 0) so downstream dashboards don't
	// misleadingly show a cache_read line for a model that isn't priced for it.
	mf := &types.ModelInfo{
		Pricing: types.Pricing{
			Prompt:     "0.000001",
			Completion: "0.000002",
		},
	}

	cost, err := CalculateTokenPrice(mf, TokenUsage{
		PromptTokens:     100_000, // OpenAI-style: 90k new + 10k cached inside
		CompletionTokens: 5_000,
		CacheReadTokens:  10_000,
	})
	assert.NoError(t, err)
	// All 100k input billed at prompt rate.
	assert.InDelta(t, 0.1, cost.PromptCost, 1e-9)
	assert.InDelta(t, 0.0, cost.CacheReadCost, 1e-9)
	assert.InDelta(t, 0.01, cost.CompletionCost, 1e-9)
	assert.InDelta(t, 0.11, cost.Total(), 1e-9)
}

func TestCalculateTokenPrice_OnlyOneCacheRateSet(t *testing.T) {
	// Only cache_read priced; cache_write has no rate → those tokens roll into prompt bucket.
	mf := &types.ModelInfo{
		Pricing: types.Pricing{
			Prompt:         "0.000003",
			Completion:     "0.000015",
			InputCacheRead: "0.0000003",
			// InputCacheWrite intentionally empty
		},
	}

	// Anthropic-shaped: input_tokens=3k (non-cached), cache_read=5k, cache_write=2k
	// → caller folds: PromptTokens = 3k + 5k + 2k = 10k.
	cost, err := CalculateTokenPrice(mf, TokenUsage{
		PromptTokens:     10_000,
		CompletionTokens: 1_000,
		CacheReadTokens:  5_000,
		CacheWriteTokens: 2_000,
	})
	assert.NoError(t, err)
	// Prompt bucket: 3k non-cached + 2k cache-write (no rate) = 5k * 3e-6 = 0.015
	assert.InDelta(t, 0.015, cost.PromptCost, 1e-9)
	// Cache read priced: 5k * 0.3e-6 = 0.0015
	assert.InDelta(t, 0.0015, cost.CacheReadCost, 1e-9)
	assert.InDelta(t, 0.0, cost.CacheWriteCost, 1e-9)
	assert.InDelta(t, 0.015, cost.CompletionCost, 1e-9)
}
