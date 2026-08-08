package openai

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runsWithin runs fn in a goroutine and fails the test if it has not returned
// within timeout. Used to catch lock-ordering bugs that would otherwise hang the
// whole test binary rather than reporting a useful failure.
func runsWithin(t *testing.T, timeout time.Duration, what string, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %s — the rate limiter mutex is stuck", what, timeout)
	}
}

// Handle429Error takes rl.mu and used to call the exported UpdateFromHeaders,
// which takes rl.mu again. sync.RWMutex is not reentrant, so the goroutine
// parked forever holding the write lock and every later request to that
// provider blocked in WaitForTokens — permanently, since Mutex.Lock cannot be
// cancelled by a context. This regressed all OpenAI inference in the process
// the first time a provider replied 429.
func TestHandle429ErrorDoesNotDeadlock(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	headers := http.Header{}
	headers.Set("x-ratelimit-limit-tokens", "150000")
	headers.Set("x-ratelimit-remaining-tokens", "0")

	runsWithin(t, 5*time.Second, "Handle429Error", func() {
		rl.Handle429Error(headers)
	})

	// The lock must have been released, so the limiter is still usable.
	runsWithin(t, 5*time.Second, "UpdateFromHeaders after Handle429Error", func() {
		rl.UpdateFromHeaders(headers)
	})

	runsWithin(t, 5*time.Second, "WaitForTokens after Handle429Error", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		// Backoff is in effect, so this is expected to end in context deadline
		// rather than block on the mutex.
		_ = rl.WaitForTokens(ctx, 1)
	})
}

func TestHandle429ErrorParsesRateLimitHeaders(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	headers := http.Header{}
	headers.Set("x-ratelimit-limit-tokens", "30000")
	headers.Set("x-ratelimit-limit-requests", "500")

	rl.Handle429Error(headers)

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// Limits come from the response headers...
	assert.Equal(t, int64(30000), rl.tokenLimit)
	assert.Equal(t, int64(30000), rl.tokensPerMinute)
	assert.Equal(t, int64(500), rl.requestLimit)

	// ...while the current buckets are zeroed to force a wait.
	assert.Equal(t, int64(0), rl.currentTokens)
	assert.Equal(t, int64(0), rl.currentRequests)
}

func TestHandle429ErrorUsesRetryAfterHeader(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	headers := http.Header{}
	headers.Set("retry-after", "42")

	rl.Handle429Error(headers)

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	assert.Equal(t, 42*time.Second, rl.backoffDuration)
}

func TestHandle429ErrorBacksOffExponentiallyAndCaps(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")
	headers := http.Header{} // no retry-after

	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		32 * time.Second,
		60 * time.Second, // capped
		60 * time.Second,
	}

	for i, want := range expected {
		rl.Handle429Error(headers)

		rl.mu.RLock()
		got := rl.backoffDuration
		rl.mu.RUnlock()

		assert.Equalf(t, want, got, "backoff after %d consecutive 429s", i+1)
	}
}

// WaitForTokens must not hold rl.mu while it sleeps: doing so serialises every
// other caller behind the one that is backing off, and callers blocked on
// Lock() cannot observe their own context cancellation.
func TestWaitForTokensDoesNotHoldLockWhileWaiting(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	headers := http.Header{}
	headers.Set("retry-after", "3600") // long enough that the waiter is definitely asleep
	rl.Handle429Error(headers)

	waiting := make(chan struct{})
	go func() {
		close(waiting)
		_ = rl.WaitForTokens(t.Context(), 1)
	}()

	<-waiting
	time.Sleep(100 * time.Millisecond) // let the waiter reach its sleep

	runsWithin(t, 5*time.Second, "UpdateFromHeaders while another caller is backing off", func() {
		rl.UpdateFromHeaders(http.Header{})
	})

	runsWithin(t, 5*time.Second, "Handle429Error while another caller is backing off", func() {
		rl.Handle429Error(http.Header{})
	})
}

func TestWaitForTokensCancelsDuringBackoff(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	headers := http.Header{}
	headers.Set("retry-after", "3600")
	rl.Handle429Error(headers)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- rl.WaitForTokens(ctx, 1)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForTokens ignored context cancellation during backoff")
	}
}

func TestWaitForTokensCancelsWhileWaitingForRefill(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	// Drain the bucket without setting a backoff, so the wait comes from the
	// token-refill path rather than the 429 path.
	rl.mu.Lock()
	rl.currentTokens = 0
	rl.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- rl.WaitForTokens(ctx, 150000) // a full minute of refill
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForTokens ignored context cancellation while waiting for refill")
	}
}

func TestWaitForTokensDeductsFromBucket(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	rl.mu.RLock()
	requestsBefore := rl.currentRequests
	rl.mu.RUnlock()

	require.NoError(t, rl.WaitForTokens(context.Background(), 1000))

	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// The bucket refills continuously, so assert it went down rather than
	// pinning an exact figure.
	assert.Less(t, rl.currentTokens, rl.maxTokens)
	assert.Equal(t, requestsBefore-1, rl.currentRequests)
}

// A 429 followed by a successful call is the sequence that stalled production:
// every request after the 429 must still get through once the backoff expires.
func TestWaitForTokensRecoversAfterBackoffExpires(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	headers := http.Header{}
	headers.Set("retry-after", "1")
	rl.Handle429Error(headers)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	require.NoError(t, rl.WaitForTokens(ctx, 1))
	assert.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond, "should have honoured the retry-after backoff")

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	assert.Zero(t, rl.backoffDuration, "backoff should be cleared once waited out")
}

// Concurrent traffic through a single limiter, with 429s interleaved — the
// shape of real usage, and the case the -race detector is most useful for.
func TestRateLimiterConcurrentAccess(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	headers := http.Header{}
	headers.Set("x-ratelimit-limit-tokens", "150000")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			rl.UpdateFromHeaders(headers)
			_ = rl.WaitForTokens(ctx, 10)
		}
	}()

	for range 50 {
		rl.Handle429Error(http.Header{})
	}

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("concurrent rate limiter access deadlocked")
	}
}
