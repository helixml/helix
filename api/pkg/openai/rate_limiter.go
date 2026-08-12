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

	// Rate limiting state. backoffDuration is the current step of the
	// exponential ladder — it survives until a request succeeds — while
	// backoffUntil is the deadline of the window opened by the last 429.
	backoffDuration time.Duration
	backoffUntil    time.Time

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
	}
}

// minRateLimiterWait floors each wait so that a caller a few tokens short of
// the bucket re-checks on a timer rather than spinning on the mutex.
const minRateLimiterWait = 10 * time.Millisecond

// WaitForTokens blocks until the request has been admitted against the token
// bucket, or ctx is cancelled.
//
// The mutex is never held across a sleep: waiters would otherwise be serialised
// behind whichever caller is backing off, and — because sync.Mutex.Lock is not
// cancellable — they could not observe their own context being cancelled.
//
// Every pass re-evaluates the bucket under the lock and only returns once the
// deduction has actually succeeded. Callers must not compute a wait, sleep, and
// then admit themselves on the strength of that stale snapshot: concurrent
// callers all see the same empty bucket, so they would wake together and admit
// N requests' worth of tokens in one instant.
func (rl *UniversalRateLimiter) WaitForTokens(ctx context.Context, tokensNeeded int64) error {
	for {
		rl.mu.Lock()
		rl.refillTokens()

		// Honour the window opened by the last 429 before looking at the bucket.
		// Re-read on every pass, so a 429 that lands while we sleep pushes the
		// deadline out rather than being missed.
		//
		// Nothing here mutates the ladder. It is reset by HandleSuccess when the
		// provider accepts a request: clearing it on the way past a window would
		// restart it at 1s on every retry, so it would never escalate during
		// exactly the sustained-429 storm it exists for.
		if remaining := time.Until(rl.backoffUntil); remaining > 0 {
			provider := rl.provider
			rl.mu.Unlock()

			log.Warn().
				Str("provider", provider).
				Dur("wait_time", remaining).
				Msg("Rate limiter waiting due to previous 429 error")

			if err := sleepWithContext(ctx, remaining); err != nil {
				return err
			}
			continue
		}

		if rl.currentRequests >= 1 {
			// Normal case: the bucket covers the request.
			if rl.currentTokens >= tokensNeeded {
				rl.currentTokens -= tokensNeeded
				rl.currentRequests--
				rl.mu.Unlock()
				return nil
			}

			// A request larger than the entire budget can never be covered.
			// Admit it once the bucket has refilled to the brim and charge it
			// everything, rather than looping until ctx expires.
			if tokensNeeded > rl.maxTokens && rl.currentTokens >= rl.maxTokens {
				log.Warn().
					Str("provider", rl.provider).
					Int64("tokens_needed", tokensNeeded).
					Int64("max_tokens", rl.maxTokens).
					Msg("Rate limiter admitting request larger than the provider's entire token budget")

				rl.currentTokens = 0
				rl.currentRequests--
				rl.mu.Unlock()
				return nil
			}
		}

		waitTime := rl.waitTimeLocked(tokensNeeded)

		log.Info().
			Str("provider", rl.provider).
			Int64("tokens_needed", tokensNeeded).
			Int64("tokens_available", rl.currentTokens).
			Dur("wait_time", waitTime).
			Msg("Rate limiter waiting for tokens")
		rl.mu.Unlock()

		if err := sleepWithContext(ctx, waitTime); err != nil {
			return err
		}
	}
}

// waitTimeLocked estimates how long until the bucket can cover tokensNeeded.
// Callers must hold rl.mu.
func (rl *UniversalRateLimiter) waitTimeLocked(tokensNeeded int64) time.Duration {
	// A request bigger than the whole budget waits for a full bucket, not for
	// an amount the limiter will never accumulate.
	target := min(tokensNeeded, rl.maxTokens)

	tokensShortfall := target - rl.currentTokens
	if tokensShortfall <= 0 {
		tokensShortfall = 1 // Need at least 1 token
	}

	var waitTime time.Duration
	if rl.tokensPerMinute > 0 {
		waitSeconds := float64(tokensShortfall) / float64(rl.tokensPerMinute) * 60.0
		waitTime = time.Duration(waitSeconds * float64(time.Second))
	} else {
		waitTime = time.Second
	}

	// Also check request rate limiting
	if rl.currentRequests < 1 {
		requestWaitTime := time.Second
		if rl.requestsPerMinute > 0 {
			// Scale before converting. time.Duration(60.0/rpm) * time.Second
			// truncates the float to integer nanoseconds first, so every rate
			// that doesn't divide 60 exactly collapses to zero — and the caller
			// then polls at the minimum wait without ever accruing a slot.
			requestWaitTime = time.Duration(float64(time.Minute) / float64(rl.requestsPerMinute))
		}
		if requestWaitTime > waitTime {
			waitTime = requestWaitTime
		}
	}

	return max(waitTime, minRateLimiterWait)
}

// sleepWithContext sleeps for d, returning early with ctx.Err() if the context
// is cancelled first.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// refillTokens refills the token and request buckets based on elapsed time
func (rl *UniversalRateLimiter) refillTokens() {
	now := time.Now()

	rl.currentTokens, rl.lastRefill = accrue(rl.currentTokens, rl.maxTokens, rl.tokensPerMinute, rl.lastRefill, now)
	rl.currentRequests, rl.lastRequestRefill = accrue(rl.currentRequests, rl.maxRequests, rl.requestsPerMinute, rl.lastRequestRefill, now)
}

// accrue adds whole units earned at ratePerMinute since last, returning the new
// count and an updated timestamp.
//
// The timestamp only advances by the time the granted units actually consumed.
// Advancing it to now would discard the remaining fraction, so a caller polling
// faster than one unit's interval — which WaitForTokens does whenever it is
// waiting on the request bucket — would throw away every partial interval and
// never accrue anything at all.
func accrue(current, maximum, ratePerMinute int64, last, now time.Time) (int64, time.Time) {
	elapsed := now.Sub(last)
	if elapsed <= 0 || ratePerMinute <= 0 {
		return current, last
	}

	// Already full: nothing to earn, and idle time must not bank credit that
	// would let a later burst exceed the bucket. Clamp on the way past, since a
	// remaining-header without a matching limit-header can seed a count above
	// the cap, and this branch is the only one such a count ever reaches.
	if current >= maximum {
		return maximum, now
	}

	newUnits := int64(float64(ratePerMinute) * elapsed.Seconds() / 60.0)
	if newUnits <= 0 {
		// Less than one whole unit so far — carry the elapsed fraction into the
		// next call rather than dropping it.
		return current, last
	}

	consumed := time.Duration(float64(newUnits) / float64(ratePerMinute) * float64(time.Minute))
	return min(maximum, current+newUnits), last.Add(consumed)
}

// UpdateFromHeaders updates the rate limiter state from provider response headers
func (rl *UniversalRateLimiter) UpdateFromHeaders(headers http.Header) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.updateFromHeadersLocked(headers)
}

// updateFromHeadersLocked is UpdateFromHeaders without the locking, for callers
// that already hold rl.mu. sync.RWMutex is not reentrant, so a lock holder that
// called UpdateFromHeaders would deadlock itself permanently.
//
// Only positive *limits* are applied: a zero limit would leave a bucket that can
// never accrue, so WaitForTokens would poll until its caller's context expired.
// A zero *remaining* is honoured — that is a provider legitimately saying the
// bucket is empty right now.
func (rl *UniversalRateLimiter) updateFromHeadersLocked(headers http.Header) {
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
		if limit, err := strconv.ParseInt(requestLimitStr, 10, 64); err == nil && limit > 0 {
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
		if limit, err := strconv.ParseInt(tokenLimitStr, 10, 64); err == nil && limit > 0 {
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
		if limit, err := strconv.ParseInt(requestLimitStr, 10, 64); err == nil && limit > 0 {
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
		if limit, err := strconv.ParseInt(tokenLimitStr, 10, 64); err == nil && limit > 0 {
			rl.tokenLimit = limit
			rl.tokensPerMinute = limit
			rl.maxTokens = limit
		}
	} else if inputTokenLimitStr := headers.Get("anthropic-ratelimit-input-tokens-limit"); inputTokenLimitStr != "" {
		if limit, err := strconv.ParseInt(inputTokenLimitStr, 10, 64); err == nil && limit > 0 {
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
		if limit, err := strconv.ParseInt(requestLimitStr, 10, 64); err == nil && limit > 0 {
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
		if limit, err := strconv.ParseInt(tokenLimitStr, 10, 64); err == nil && limit > 0 {
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
	rl.updateFromHeadersLocked(headers)

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

	// Anchor the window to this 429. Deriving it from the last request instead
	// would let a slow request that 429s after the previous window elapsed
	// compute a zero wait and skip the server's retry-after entirely; and any
	// admission would silently re-arm the window for another full step.
	rl.backoffUntil = time.Now().Add(rl.backoffDuration)

	log.Warn().
		Str("provider", rl.provider).
		Dur("backoff_duration", rl.backoffDuration).
		Int64("tokens_remaining", rl.tokenRemainingLimit).
		Int64("requests_remaining", rl.requestRemainingLimit).
		Msg("Handling 429 error with backoff")
}

// HandleSuccess resets the exponential backoff ladder after the provider
// accepts a request. Backoff escalates across consecutive 429s and is cleared
// only on success — never merely by waiting a window out, which would restart
// the ladder at its base on every retry.
//
// Any window already open is deliberately left to expire on its own. Under load
// some requests succeed while others 429, and a success on one is not licence to
// cancel a retry-after the server explicitly asked for on another.
func (rl *UniversalRateLimiter) HandleSuccess() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.backoffDuration == 0 {
		return
	}

	log.Debug().
		Str("provider", rl.provider).
		Dur("previous_backoff", rl.backoffDuration).
		Msg("Provider accepted a request, resetting rate limiter backoff")

	rl.backoffDuration = 0
}

// EstimateTokens estimates the number of tokens in a request (very rough estimate)
func EstimateTokens(text string) int64 {
	// Very rough estimate: ~4 characters per token for English text
	return int64(len(text) / 4)
}
