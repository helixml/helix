package openai

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// UniversalRateLimiter implements a token bucket rate limiter that works with multiple AI providers
type UniversalRateLimiter struct {
	mu sync.RWMutex

	// Token bucket parameters
	tokensPerMinute int64
	maxTokens       int64
	currentTokens   int64
	lastRefill      time.Time

	// Request rate limiting
	requestsPerMinute int64
	maxRequests       int64
	currentRequests   int64
	lastRequestRefill time.Time

	// Configured limits from headers (latest values from server)
	tokenLimit            int64
	requestLimit          int64
	tokenRemainingLimit   int64
	requestRemainingLimit int64
	resetTime             time.Time

	// Rate limiting state
	lastRequestTime time.Time
	backoffDuration time.Duration

	// Provider identification
	provider string
}

// NewUniversalRateLimiter creates a new rate limiter that works with multiple providers
func NewUniversalRateLimiter(provider string) *UniversalRateLimiter {
	now := time.Now()

	// Set conservative default limits based on provider
	var defaultTokens, defaultRequests int64
	switch {
	case strings.Contains(provider, "anthropic"):
		defaultTokens = 100000 // 100k tokens per minute
		defaultRequests = 1000
	case strings.Contains(provider, "openai"):
		defaultTokens = 150000 // 150k tokens per minute
		defaultRequests = 60
	case strings.Contains(provider, "together"):
		defaultTokens = 180000 // 180k tokens per minute
		defaultRequests = 600
	default:
		// Conservative defaults for unknown providers
		defaultTokens = 50000
		defaultRequests = 100
	}

	return &UniversalRateLimiter{
		provider: provider,

		tokensPerMinute: defaultTokens,
		maxTokens:       defaultTokens,
		currentTokens:   defaultTokens,

		requestsPerMinute: defaultRequests,
		maxRequests:       defaultRequests,
		currentRequests:   defaultRequests,

		lastRefill:        now,
		lastRequestRefill: now,
		lastRequestTime:   now,
	}
}

// WaitForTokens waits until the specified number of tokens are available
func (rl *UniversalRateLimiter) WaitForTokens(ctx context.Context, tokensNeeded int64) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on elapsed time
	rl.refillTokens()

	// Check if we need to wait due to previous 429 errors
	if rl.backoffDuration > 0 {
		backoffEnd := rl.lastRequestTime.Add(rl.backoffDuration)
		if time.Now().Before(backoffEnd) {
			waitTime := time.Until(backoffEnd)
			log.Warn().
				Str("provider", rl.provider).
				Dur("wait_time", waitTime).
				Msg("Rate limiter waiting due to previous 429 error")

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
				rl.backoffDuration = 0 // Reset backoff
			}
		}
	}

	// Check if we have enough tokens
	if rl.currentTokens >= tokensNeeded && rl.currentRequests >= 1 {
		rl.currentTokens -= tokensNeeded
		rl.currentRequests--
		rl.lastRequestTime = time.Now()
		return nil
	}

	// Calculate wait time for token refill
	tokensShortfall := tokensNeeded - rl.currentTokens
	if tokensShortfall <= 0 {
		tokensShortfall = 1 // Need at least 1 token
	}

	waitSeconds := float64(tokensShortfall) / float64(rl.tokensPerMinute) * 60.0
	waitTime := time.Duration(waitSeconds * float64(time.Second))

	// Also check request rate limiting
	if rl.currentRequests < 1 {
		requestWaitTime := time.Duration(60.0/float64(rl.requestsPerMinute)) * time.Second
		if requestWaitTime > waitTime {
			waitTime = requestWaitTime
		}
	}

	log.Info().
		Str("provider", rl.provider).
		Int64("tokens_needed", tokensNeeded).
		Int64("tokens_available", rl.currentTokens).
		Dur("wait_time", waitTime).
		Msg("Rate limiter waiting for tokens")

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(waitTime):
		// Refill again after waiting
		rl.refillTokens()

		// Deduct tokens and requests
		if rl.currentTokens >= tokensNeeded {
			rl.currentTokens -= tokensNeeded
		}
		if rl.currentRequests >= 1 {
			rl.currentRequests--
		}
		rl.lastRequestTime = time.Now()
		return nil
	}
}

// refillTokens refills the token bucket based on elapsed time
func (rl *UniversalRateLimiter) refillTokens() {
	now := time.Now()

	// Refill tokens
	tokenElapsed := now.Sub(rl.lastRefill)
	if tokenElapsed > 0 {
		newTokens := int64(float64(rl.tokensPerMinute) * tokenElapsed.Seconds() / 60.0)
		rl.currentTokens = min(rl.maxTokens, rl.currentTokens+newTokens)
		rl.lastRefill = now
	}

	// Refill requests
	requestElapsed := now.Sub(rl.lastRequestRefill)
	if requestElapsed > 0 {
		newRequests := int64(float64(rl.requestsPerMinute) * requestElapsed.Seconds() / 60.0)
		rl.currentRequests = min(rl.maxRequests, rl.currentRequests+newRequests)
		rl.lastRequestRefill = now
	}
}

// UpdateFromHeaders updates the rate limiter state from provider response headers
func (rl *UniversalRateLimiter) UpdateFromHeaders(headers http.Header) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Try to parse headers from different providers
	rl.parseOpenAIHeaders(headers)
	rl.parseAnthropicHeaders(headers)
	rl.parseTogetherAIHeaders(headers)

	log.Debug().
		Str("provider", rl.provider).
		Int64("token_limit", rl.tokenLimit).
		Int64("request_limit", rl.requestLimit).
		Int64("tokens_remaining", rl.tokenRemainingLimit).
		Int64("requests_remaining", rl.requestRemainingLimit).
		Time("reset_time", rl.resetTime).
		Msg("Updated rate limiter from provider headers")
}

// parseOpenAIHeaders parses OpenAI-style rate limit headers
func (rl *UniversalRateLimiter) parseOpenAIHeaders(headers http.Header) {
	// Request limits
	if requestLimitStr := headers.Get("x-ratelimit-limit-requests"); requestLimitStr != "" {
		if limit, err := strconv.ParseInt(requestLimitStr, 10, 64); err == nil {
			rl.requestLimit = limit
			rl.requestsPerMinute = limit
			rl.maxRequests = limit
		}
	}

	if requestRemainingStr := headers.Get("x-ratelimit-remaining-requests"); requestRemainingStr != "" {
		if remaining, err := strconv.ParseInt(requestRemainingStr, 10, 64); err == nil {
			rl.requestRemainingLimit = remaining
			rl.currentRequests = remaining
		}
	}

	// Token limits
	if tokenLimitStr := headers.Get("x-ratelimit-limit-tokens"); tokenLimitStr != "" {
		if limit, err := strconv.ParseInt(tokenLimitStr, 10, 64); err == nil {
			rl.tokenLimit = limit
			rl.tokensPerMinute = limit
			rl.maxTokens = limit
		}
	}

	if tokenRemainingStr := headers.Get("x-ratelimit-remaining-tokens"); tokenRemainingStr != "" {
		if remaining, err := strconv.ParseInt(tokenRemainingStr, 10, 64); err == nil {
			rl.tokenRemainingLimit = remaining
			rl.currentTokens = remaining
		}
	}

	// Reset time
	if resetTimeStr := headers.Get("x-ratelimit-reset-tokens"); resetTimeStr != "" {
		if resetTime, err := time.Parse(time.RFC3339, resetTimeStr); err == nil {
			rl.resetTime = resetTime
		}
	}
}

// parseAnthropicHeaders parses Anthropic-style rate limit headers
func (rl *UniversalRateLimiter) parseAnthropicHeaders(headers http.Header) {
	// Request limits
	if requestLimitStr := headers.Get("anthropic-ratelimit-requests-limit"); requestLimitStr != "" {
		if limit, err := strconv.ParseInt(requestLimitStr, 10, 64); err == nil {
			rl.requestLimit = limit
			rl.requestsPerMinute = limit
			rl.maxRequests = limit
		}
	}

	if requestRemainingStr := headers.Get("anthropic-ratelimit-requests-remaining"); requestRemainingStr != "" {
		if remaining, err := strconv.ParseInt(requestRemainingStr, 10, 64); err == nil {
			rl.requestRemainingLimit = remaining
			rl.currentRequests = remaining
		}
	}

	// Token limits (try both tokens and input-tokens headers)
	if tokenLimitStr := headers.Get("anthropic-ratelimit-tokens-limit"); tokenLimitStr != "" {
		if limit, err := strconv.ParseInt(tokenLimitStr, 10, 64); err == nil {
			rl.tokenLimit = limit
			rl.tokensPerMinute = limit
			rl.maxTokens = limit
		}
	} else if inputTokenLimitStr := headers.Get("anthropic-ratelimit-input-tokens-limit"); inputTokenLimitStr != "" {
		if limit, err := strconv.ParseInt(inputTokenLimitStr, 10, 64); err == nil {
			rl.tokenLimit = limit
			rl.tokensPerMinute = limit
			rl.maxTokens = limit
		}
	}

	if tokenRemainingStr := headers.Get("anthropic-ratelimit-tokens-remaining"); tokenRemainingStr != "" {
		if remaining, err := strconv.ParseInt(tokenRemainingStr, 10, 64); err == nil {
			rl.tokenRemainingLimit = remaining
			rl.currentTokens = remaining
		}
	} else if inputTokenRemainingStr := headers.Get("anthropic-ratelimit-input-tokens-remaining"); inputTokenRemainingStr != "" {
		if remaining, err := strconv.ParseInt(inputTokenRemainingStr, 10, 64); err == nil {
			rl.tokenRemainingLimit = remaining
			rl.currentTokens = remaining
		}
	}

	// Reset time
	if resetTimeStr := headers.Get("anthropic-ratelimit-tokens-reset"); resetTimeStr != "" {
		if resetTime, err := time.Parse(time.RFC3339, resetTimeStr); err == nil {
			rl.resetTime = resetTime
		}
	}
}

// parseTogetherAIHeaders parses Together AI-style rate limit headers
func (rl *UniversalRateLimiter) parseTogetherAIHeaders(headers http.Header) {
	// Request limits
	if requestLimitStr := headers.Get("x-ratelimit-limit"); requestLimitStr != "" {
		if limit, err := strconv.ParseInt(requestLimitStr, 10, 64); err == nil {
			rl.requestLimit = limit
			rl.requestsPerMinute = limit * 60 // Together AI reports per-second, convert to per-minute
			rl.maxRequests = rl.requestsPerMinute
		}
	}

	if requestRemainingStr := headers.Get("x-ratelimit-remaining"); requestRemainingStr != "" {
		if remaining, err := strconv.ParseInt(requestRemainingStr, 10, 64); err == nil {
			rl.requestRemainingLimit = remaining
			rl.currentRequests = remaining * 60 // Convert to per-minute equivalent
		}
	}

	// Token limits
	if tokenLimitStr := headers.Get("x-tokenlimit-limit"); tokenLimitStr != "" {
		if limit, err := strconv.ParseInt(tokenLimitStr, 10, 64); err == nil {
			rl.tokenLimit = limit
			rl.tokensPerMinute = limit * 60 // Together AI reports per-second, convert to per-minute
			rl.maxTokens = rl.tokensPerMinute
		}
	}

	if tokenRemainingStr := headers.Get("x-tokenlimit-remaining"); tokenRemainingStr != "" {
		if remaining, err := strconv.ParseInt(tokenRemainingStr, 10, 64); err == nil {
			rl.tokenRemainingLimit = remaining
			rl.currentTokens = remaining * 60 // Convert to per-minute equivalent
		}
	}
}

// Handle429Error handles a 429 Too Many Requests error by implementing exponential backoff
func (rl *UniversalRateLimiter) Handle429Error(headers http.Header) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Update from headers first
	rl.UpdateFromHeaders(headers)

	// Check for retry-after header first (standard)
	if retryAfterStr := headers.Get("retry-after"); retryAfterStr != "" {
		if retryAfter, err := strconv.ParseInt(retryAfterStr, 10, 64); err == nil {
			rl.backoffDuration = time.Duration(retryAfter) * time.Second
			log.Warn().
				Str("provider", rl.provider).
				Dur("retry_after", rl.backoffDuration).
				Msg("Using server-provided retry-after value")
		}
	} else {
		// Implement exponential backoff if no retry-after header
		baseBackoff := 1 * time.Second
		if rl.backoffDuration == 0 {
			rl.backoffDuration = baseBackoff
		} else {
			newBackoff := rl.backoffDuration * 2
			maxBackoff := 60 * time.Second
			if newBackoff > maxBackoff {
				rl.backoffDuration = maxBackoff
			} else {
				rl.backoffDuration = newBackoff
			}
		}
	}

	// Zero out current tokens and requests to force waiting
	rl.currentTokens = 0
	rl.currentRequests = 0

	log.Warn().
		Str("provider", rl.provider).
		Dur("backoff_duration", rl.backoffDuration).
		Int64("tokens_remaining", rl.tokenRemainingLimit).
		Int64("requests_remaining", rl.requestRemainingLimit).
		Msg("Handling 429 error with backoff")
}

// EstimateTokens estimates the number of tokens in a request (very rough estimate)
func EstimateTokens(text string) int64 {
	// Very rough estimate: ~4 characters per token for English text
	return int64(len(text) / 4)
}
