package services

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	gh "github.com/google/go-github/v57/github"
)

func TestPRListCache_HitsThenExpires(t *testing.T) {
	c := newPRListCache(50 * time.Millisecond)

	if _, _, ok := c.get("repo-1"); ok {
		t.Fatal("expected miss on empty cache")
	}

	prs := []*types.PullRequest{{ID: "1"}}
	c.set("repo-1", prs)

	got, cachedErr, ok := c.get("repo-1")
	if !ok || cachedErr != nil || len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("expected cached prs, got hit=%v err=%v prs=%v", ok, cachedErr, got)
	}

	time.Sleep(60 * time.Millisecond)

	if _, _, ok := c.get("repo-1"); ok {
		t.Fatal("expected expiry after TTL")
	}
}

func TestPRListCache_ErrorCachedUntilExpiresAt(t *testing.T) {
	c := newPRListCache(time.Hour) // long success TTL so it doesn't interfere

	rlErr := errors.New("rate limited")
	expiry := time.Now().Add(40 * time.Millisecond)
	c.setError("repo-rl", rlErr, expiry)

	_, gotErr, ok := c.get("repo-rl")
	if !ok || gotErr == nil {
		t.Fatalf("expected cached error, got hit=%v err=%v", ok, gotErr)
	}

	time.Sleep(60 * time.Millisecond)

	if _, _, ok := c.get("repo-rl"); ok {
		t.Fatal("expected error entry to expire after its expiresAt")
	}
}

func TestPRListCache_InvalidateClears(t *testing.T) {
	c := newPRListCache(time.Hour)
	c.set("repo-x", []*types.PullRequest{{ID: "1"}})

	c.invalidate("repo-x")

	if _, _, ok := c.get("repo-x"); ok {
		t.Fatal("expected invalidate to clear the entry")
	}
}

func TestRateLimitBackoffUntil_RateLimitError(t *testing.T) {
	resetAt := time.Now().Add(2 * time.Minute)
	rlErr := &gh.RateLimitError{
		Rate: gh.Rate{Reset: gh.Timestamp{Time: resetAt}},
		Response: &http.Response{
			StatusCode: 403,
			Request: &http.Request{
				Method: "GET",
				URL:    mustURL("https://api.github.com/repos/x/y/pulls"),
			},
		},
		Message: "API rate limit exceeded",
	}
	wrapped := errors.New("wrap: " + rlErr.Error())
	_ = wrapped // we need wrapped-with-%w to test errors.As; build inline below

	until, ok := rateLimitBackoffUntil(rlErr)
	if !ok {
		t.Fatal("expected rate-limit error to be recognized")
	}
	if !until.Equal(resetAt) {
		t.Fatalf("expected backoff until %v, got %v", resetAt, until)
	}
}

func TestRateLimitBackoffUntil_RateLimitErrorWrapped(t *testing.T) {
	resetAt := time.Now().Add(3 * time.Minute)
	inner := &gh.RateLimitError{
		Rate: gh.Rate{Reset: gh.Timestamp{Time: resetAt}},
		Response: &http.Response{
			StatusCode: 403,
			Request: &http.Request{
				Method: "GET",
				URL:    mustURL("https://api.github.com/repos/x/y/pulls"),
			},
		},
	}
	wrapped := wrapErr(wrapErr(inner))

	until, ok := rateLimitBackoffUntil(wrapped)
	if !ok {
		t.Fatal("expected wrapped rate-limit error to be recognized via errors.As")
	}
	if !until.Equal(resetAt) {
		t.Fatalf("expected backoff until %v, got %v", resetAt, until)
	}
}

func TestRateLimitBackoffUntil_AbuseRateLimitError(t *testing.T) {
	retry := 90 * time.Second
	abuse := &gh.AbuseRateLimitError{
		RetryAfter: &retry,
		Response: &http.Response{
			StatusCode: 403,
			Request: &http.Request{
				Method: "GET",
				URL:    mustURL("https://api.github.com/repos/x/y/pulls"),
			},
		},
	}

	before := time.Now()
	until, ok := rateLimitBackoffUntil(abuse)
	if !ok {
		t.Fatal("expected abuse rate-limit error to be recognized")
	}
	if until.Before(before.Add(80*time.Second)) || until.After(before.Add(100*time.Second)) {
		t.Fatalf("expected ~90s backoff, got %v (delta %v)", until, until.Sub(before))
	}
}

func TestRateLimitBackoffUntil_RateLimitErrorPastReset(t *testing.T) {
	rlErr := &gh.RateLimitError{
		Rate: gh.Rate{Reset: gh.Timestamp{Time: time.Now().Add(-1 * time.Hour)}},
		Response: &http.Response{
			StatusCode: 403,
			Request: &http.Request{
				Method: "GET",
				URL:    mustURL("https://api.github.com/repos/x/y/pulls"),
			},
		},
	}

	before := time.Now()
	until, ok := rateLimitBackoffUntil(rlErr)
	if !ok {
		t.Fatal("expected rate-limit error to be recognized")
	}
	if until.Before(before.Add(rateLimitFallbackBackoff - 5*time.Second)) {
		t.Fatalf("expected fallback backoff ~5m, got %v", until.Sub(before))
	}
}

func TestRateLimitBackoffUntil_NotRateLimit(t *testing.T) {
	if _, ok := rateLimitBackoffUntil(errors.New("some other failure")); ok {
		t.Fatal("expected non-rate-limit error to be unrecognized")
	}
	if _, ok := rateLimitBackoffUntil(nil); ok {
		t.Fatal("expected nil error to be unrecognized")
	}
}

// wrapErr mimics the fmt.Errorf("...: %w", err) wrapping that
// listGitHubPullRequests / Client.ListPullRequests apply on the way back up.
func wrapErr(err error) error {
	return &wrapErrT{inner: err}
}

type wrapErrT struct{ inner error }

func (e *wrapErrT) Error() string { return "wrapped: " + e.inner.Error() }
func (e *wrapErrT) Unwrap() error { return e.inner }

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
