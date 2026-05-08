package pricing

import (
	"strconv"

	"github.com/helixml/helix/api/pkg/types"
)

// TokenUsage captures the four charge lines we bill for an LLM call.
//
// PromptTokens MUST include CacheReadTokens and CacheWriteTokens (i.e. total input
// tokens before any cache split). Callers normalize provider-specific shapes:
//   - OpenAI / Gemini: pass usage.prompt_tokens as-is (cached tokens are already inside it).
//   - Anthropic: pass input_tokens + cache_read_input_tokens + cache_creation_input_tokens
//     (cache tokens sit alongside input_tokens in Anthropic's response).
//
// CalculateTokenPrice derives non-cached input = PromptTokens - CacheReadTokens - CacheWriteTokens.
type TokenUsage struct {
	PromptTokens     int64
	CompletionTokens int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// TokenCost is the per-line cost breakdown.
type TokenCost struct {
	PromptCost     float64
	CompletionCost float64
	CacheReadCost  float64
	CacheWriteCost float64
}

// Total returns the sum of all cost lines.
func (c TokenCost) Total() float64 {
	return c.PromptCost + c.CompletionCost + c.CacheReadCost + c.CacheWriteCost
}

// CalculateTokenPrice calculates prompt/completion/cache-read/cache-write costs for an LLM call.
//
// Provider caching reporting differs:
//   - OpenAI / Gemini: cached input tokens are reported *inside* usage.prompt_tokens (via prompt_tokens_details.cached_tokens).
//   - Anthropic: cache_read_input_tokens and cache_creation_input_tokens are reported *alongside* input_tokens (not included in it).
//
// Callers pass PromptTokens as the provider-reported figure. We split the prompt bucket by subtracting
// the cached portion, clamped at zero for Anthropic-style reporting.
//
// Missing cache price = "this model doesn't price cache in that direction". Those tokens are billed
// at the prompt rate and rolled into PromptCost (so the separate CacheRead/WriteCost line stays 0
// and reporting truthfully reflects that cache wasn't priced separately).
func CalculateTokenPrice(mf *types.ModelInfo, usage TokenUsage) (TokenCost, error) {
	var cost TokenCost

	promptPrice, err := parsePriceField(mf.Pricing.Prompt)
	if err != nil {
		return cost, err
	}
	completionPrice, err := parsePriceField(mf.Pricing.Completion)
	if err != nil {
		return cost, err
	}
	cacheReadPrice, err := parsePriceField(mf.Pricing.InputCacheRead)
	if err != nil {
		return cost, err
	}
	cacheWritePrice, err := parsePriceField(mf.Pricing.InputCacheWrite)
	if err != nil {
		return cost, err
	}

	hasCacheReadPrice := mf.Pricing.InputCacheRead != ""
	hasCacheWritePrice := mf.Pricing.InputCacheWrite != ""

	// Non-cached prompt tokens = provider prompt_tokens minus the cached portion.
	// For OpenAI/Gemini the cached tokens are already inside PromptTokens, so we subtract.
	// For Anthropic the cached tokens sit outside input_tokens; callers pass PromptTokens
	// as input_tokens only, so the subtraction clamps at 0 via max.
	nonCachedPromptTokens := max(usage.PromptTokens-usage.CacheReadTokens-usage.CacheWriteTokens, 0)
	promptTokensForBilling := nonCachedPromptTokens

	// If a cache rate isn't published, roll those tokens back into the prompt bucket
	// so the CacheRead/WriteCost line stays 0 (truthful reporting) and billing is
	// equivalent to the provider not offering cache pricing for that direction.
	if hasCacheReadPrice {
		cost.CacheReadCost = cacheReadPrice * float64(usage.CacheReadTokens)
	} else {
		promptTokensForBilling += usage.CacheReadTokens
	}
	if hasCacheWritePrice {
		cost.CacheWriteCost = cacheWritePrice * float64(usage.CacheWriteTokens)
	} else {
		promptTokensForBilling += usage.CacheWriteTokens
	}

	cost.PromptCost = promptPrice * float64(promptTokensForBilling)
	cost.CompletionCost = completionPrice * float64(usage.CompletionTokens)

	return cost, nil
}

func parsePriceField(v string) (float64, error) {
	if v == "" {
		return 0, nil
	}
	return strconv.ParseFloat(v, 64)
}
