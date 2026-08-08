package openai

import (
	"context"
	"net/http"
	"slices"
	"sync"
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

	// Waiting the window out lets the caller through but deliberately leaves the
	// ladder standing — only a successful request resets it.
	rl.mu.RLock()
	assert.Equal(t, time.Second, rl.backoffDuration)
	rl.mu.RUnlock()

	rl.HandleSuccess()

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	assert.Zero(t, rl.backoffDuration, "backoff should be cleared once the provider accepts a request")
}

// Backoff must escalate across consecutive 429s even though callers wait each
// window out in between — that is the sustained-outage case it exists for — and
// must reset once the provider accepts a request.
func TestBackoffEscalatesAcrossRetriesAndResetsOnSuccess(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")
	headers := http.Header{} // no retry-after, so the exponential ladder applies

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Two 429s, each with the window fully waited out in between, as the
	// client's retry loop does.
	rl.Handle429Error(headers)
	require.NoError(t, rl.WaitForTokens(ctx, 1))

	rl.mu.RLock()
	afterFirst := rl.backoffDuration
	rl.mu.RUnlock()
	assert.Equal(t, 1*time.Second, afterFirst)

	rl.Handle429Error(headers)

	rl.mu.RLock()
	afterSecond := rl.backoffDuration
	rl.mu.RUnlock()
	assert.Equal(t, 2*time.Second, afterSecond, "ladder must climb on consecutive 429s, not restart at its base")

	rl.HandleSuccess()

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	assert.Zero(t, rl.backoffDuration)
}

// Concurrent traffic through every locking method at once. The interleaved
// 429s keep zeroing the bucket, so WaitForTokens is expected to give up on its
// context rather than succeed — what is under test is that all three methods
// keep making progress against the shared mutex. Throughput semantics are
// covered by TestWaitForTokensDoesNotOverAdmitUnderConcurrency.
func TestRateLimiterConcurrentAccessDoesNotDeadlock(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()

	headers := http.Header{}
	headers.Set("x-ratelimit-limit-tokens", "150000")

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for range 200 {
			rl.UpdateFromHeaders(headers)
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			rl.Handle429Error(http.Header{})
		}
	}()
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			_ = rl.WaitForTokens(ctx, 10)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent rate limiter access deadlocked")
	}

	// Should finish as soon as the 500ms context expires; anything close to the
	// 10s ceiling means callers are queueing behind a lock held across a sleep.
	assert.Less(t, time.Since(start), 5*time.Second)
}

// Dropping the mutex around the sleeps must not drop the serialisation that is
// doing the actual rate limiting. Concurrent callers all observe the same empty
// bucket, so admitting on the strength of that stale snapshot lets every one of
// them through at the same instant — N times the configured budget.
func TestWaitForTokensDoesNotOverAdmitUnderConcurrency(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	const tokensPerSecond = 1000
	const callers = 3
	const tokensEach = 200 // 200ms of budget each

	now := time.Now()
	rl.mu.Lock()
	rl.tokensPerMinute = tokensPerSecond * 60
	rl.maxTokens = tokensPerSecond * 60
	rl.currentTokens = 0 // start empty so every caller has to wait its turn
	rl.requestsPerMinute = 60000
	rl.maxRequests = 60000
	rl.currentRequests = 1000 // request limiting must not gate this test
	rl.lastRefill = now
	rl.lastRequestRefill = now
	rl.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	type admission struct {
		at  time.Duration
		err error
	}

	start := time.Now()
	results := make(chan admission, callers)

	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := rl.WaitForTokens(ctx, tokensEach)
			results <- admission{at: time.Since(start), err: err}
		}()
	}
	wg.Wait()
	close(results)

	var times []time.Duration
	for r := range results {
		require.NoError(t, r.err)
		times = append(times, r.at)
	}
	require.Len(t, times, callers)
	slices.Sort(times)

	// Each caller must wait for the bucket to refill its own share — roughly
	// 200ms apart. Admitting all three at once is the regression.
	for i := 1; i < callers; i++ {
		assert.GreaterOrEqualf(t, times[i]-times[i-1], 100*time.Millisecond,
			"callers %d and %d were admitted %s apart; expected them to be serialised (all admissions: %v)",
			i-1, i, times[i]-times[i-1], times)
	}

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	assert.LessOrEqual(t, rl.currentTokens, int64(tokensPerSecond), "bucket should have been charged for every admission")
}

// A 429 that lands while another caller is asleep in its backoff must extend
// the window, not be cleared out from under it when that sleep ends. Otherwise
// the exponential ladder in Handle429Error resets to zero during exactly the
// sustained-429 storm it exists for.
func TestHandle429DuringBackoffIsNotDiscarded(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	short := http.Header{}
	short.Set("retry-after", "1")
	rl.Handle429Error(short)

	returned := make(chan error, 1)
	go func() {
		returned <- rl.WaitForTokens(t.Context(), 1)
	}()

	// Second 429 arrives while the waiter is still inside its 1s sleep.
	time.Sleep(300 * time.Millisecond)
	long := http.Header{}
	long.Set("retry-after", "3600")
	rl.Handle429Error(long)

	select {
	case err := <-returned:
		t.Fatalf("WaitForTokens returned (err=%v) instead of honouring the second 429's backoff", err)
	case <-time.After(2 * time.Second):
		// Still waiting, as it should be.
	}

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	assert.Equal(t, time.Hour, rl.backoffDuration, "the second 429's backoff was discarded")
}

// A request bigger than the provider's entire per-minute budget can never be
// covered by the bucket. It must still be admitted once the bucket is full
// rather than looping until the caller's context expires.
func TestWaitForTokensAdmitsRequestLargerThanWholeBudget(t *testing.T) {
	rl := NewUniversalRateLimiter("openai")

	rl.mu.Lock()
	rl.tokensPerMinute = 60000
	rl.maxTokens = 1000
	rl.currentTokens = 1000 // full
	rl.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	require.NoError(t, rl.WaitForTokens(ctx, 5000)) // 5x the entire bucket

	rl.mu.RLock()
	defer rl.mu.RUnlock()
	assert.Equal(t, int64(0), rl.currentTokens, "an oversized request should be charged the whole bucket")
}
