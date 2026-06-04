package yellowdog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the YellowDog public platform endpoint, sourced from
// the official Python SDK's ServicesSchema default
// (yellowdog_client/__init__.py).
const DefaultBaseURL = "https://portal.yellowdog.co/api"

// maxResponseBodyBytes caps the response body we will read into memory
// for JSON decoding. YellowDog payloads we use (namespaces, work
// requirements, etc.) are well under 1 MB. The 8 MB ceiling protects
// us from a misbehaving or compromised upstream returning an unbounded
// body and OOM-ing the Helix API process.
const maxResponseBodyBytes = 8 << 20

// Client is a thin wrapper around http.Client that knows how to talk to
// the YellowDog REST API.
//
// Construct one per (credentials, endpoint) tuple. Safe for concurrent
// use by multiple goroutines; the embedded *http.Client owns its own
// connection pool.
type Client struct {
	baseURL       string
	creds         Credentials
	httpc         *http.Client
	allowInsecure bool
	retry         retryConfig
}

// String returns a human-readable representation that explicitly
// hides the credential secret. Used by fmt verbs that respect
// Stringer.
func (c *Client) String() string {
	return fmt.Sprintf("yellowdog.Client{baseURL: %q, creds: %s}", c.baseURL, c.creds)
}

// retryConfig is the per-Client retry policy applied to idempotent
// requests. Zero value disables retry (one attempt, no backoff).
type retryConfig struct {
	maxAttempts    int
	initialBackoff time.Duration
}

// Option mutates a Client at construction time. Use NewClient with these.
type Option func(*Client)

// WithBaseURL overrides the platform endpoint. The trailing "/api"
// segment is part of the platform URL by convention; leave it on.
// Non-HTTPS URLs are rejected at NewClient time unless
// WithInsecureBaseURL is also supplied (mostly for httptest fakes).
func WithBaseURL(u string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(u, "/")
	}
}

// WithInsecureBaseURL permits a non-HTTPS base URL. Intended for
// tests against httptest servers; do not use in production. The
// option is named loudly so a reviewer will catch any accidental
// production use.
func WithInsecureBaseURL() Option {
	return func(c *Client) {
		c.allowInsecure = true
	}
}

// WithHTTPClient injects a custom http.Client. Helix typically threads
// an instrumented client (metrics, tracing) here.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		c.httpc = h
	}
}

// WithRetry enables retry-with-exponential-backoff for transient
// failures (network errors, 5xx, 429). Retries are ONLY applied to
// idempotent methods (GET, PUT, DELETE, HEAD, OPTIONS) - POST is
// always single-attempt because the server may have side-effected on
// the first try.
//
// maxAttempts is the total number of tries including the first.
// Sensible values are 2-4. initialBackoff is the wait before the
// first retry; subsequent retries use exponential backoff (2x, 4x,
// 8x ...) with jitter, capped at 10 seconds per sleep.
//
// Pass maxAttempts < 2 to effectively disable retry.
func WithRetry(maxAttempts int, initialBackoff time.Duration) Option {
	return func(c *Client) {
		c.retry = retryConfig{maxAttempts: maxAttempts, initialBackoff: initialBackoff}
	}
}

// NewClient returns a Client ready to use. It does NOT verify the
// credentials at construction time - the first call that hits an
// authenticated endpoint will surface auth errors as APIError with
// Status=401.
//
// Returns an error if the credentials are empty or the base URL is
// not HTTPS (without WithInsecureBaseURL).
func NewClient(creds Credentials, opts ...Option) (*Client, error) {
	if !creds.Valid() {
		return nil, fmt.Errorf("yellowdog: invalid credentials (KeyID or Secret empty)")
	}
	c := &Client{
		baseURL: DefaultBaseURL,
		creds:   creds,
		httpc:   &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	if !c.allowInsecure && !strings.HasPrefix(c.baseURL, "https://") {
		return nil, fmt.Errorf("yellowdog: base URL %q must use https (pass WithInsecureBaseURL to override)", c.baseURL)
	}
	return c, nil
}

// idempotentMethods are the HTTP methods we'll retry on transient
// failure. POST is excluded because it may have side-effected on the
// first attempt and a blind retry could double-submit.
var idempotentMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
}

// do is the shared request path for every endpoint method. It
// marshals body to JSON (when non-nil), sets auth + content-type
// headers, fires the request (with retry where applicable), and
// decodes a JSON response into out (when non-nil). Non-2xx responses
// are decoded into APIError and returned as the error.
//
// The body parameter must be a Go value that json.Marshal can encode;
// callers must not pass []byte or io.Reader (those would be marshalled
// as their containing types, not as raw payload).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyBytes []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("yellowdog: marshal request body: %w", err)
		}
		bodyBytes = raw
	}

	fullURL := c.baseURL + path
	attempts := 1
	if c.retry.maxAttempts > 1 && idempotentMethods[method] {
		attempts = c.retry.maxAttempts
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := backoffSleep(ctx, c.retry.initialBackoff, attempt); err != nil {
				return err
			}
		}

		err := c.singleAttempt(ctx, method, fullURL, bodyBytes, out)
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// singleAttempt makes one HTTP request to the YellowDog API. Returns
// nil on success (out populated if non-nil), an *APIError on a non-2xx
// response (the APIError carries the status code), or a wrapped error
// on transport / marshal / decode failure.
func (c *Client) singleAttempt(ctx context.Context, method, fullURL string, bodyBytes []byte, out any) error {
	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("yellowdog: new request %s %s: %w", method, fullURL, err)
	}
	req.Header.Set("Authorization", c.creds.authHeader())
	req.Header.Set("Accept", "application/json")
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("yellowdog: %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBodyBytes)

	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode}
		_ = json.NewDecoder(limited).Decode(apiErr)
		if apiErr.Status == 0 {
			apiErr.Status = resp.StatusCode
		}
		// Drain so keep-alive can reuse the connection.
		_, _ = io.Copy(io.Discard, limited)
		return apiErr
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, limited)
		return nil
	}

	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("yellowdog: decode response %s %s: %w", method, fullURL, err)
	}
	_, _ = io.Copy(io.Discard, limited)
	return nil
}

// isRetryable returns true if the error indicates a transient failure
// worth retrying.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		// 5xx server errors and 429 rate-limited are worth retrying.
		// 4xx client errors are not (request is malformed or
		// unauthorised; retrying won't help).
		return apiErr.Status >= 500 || apiErr.Status == http.StatusTooManyRequests
	}
	// Transport-level errors (DNS, connection refused, TLS handshake
	// failure, read timeout mid-stream) are retryable. Context
	// cancellation is not (the caller asked us to stop).
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

// backoffSleep waits for an exponentially-increasing duration with
// jitter, or returns the context error if the context is cancelled
// during the wait. attempt is 1-indexed (first retry is attempt=1).
func backoffSleep(ctx context.Context, initial time.Duration, attempt int) error {
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	// exponential: initial * 2^(attempt-1)
	d := initial << uint(attempt-1)
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	// jitter: ±25%
	jitter := time.Duration(rand.Int63n(int64(d / 2)))
	d = d/2 + jitter
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// getJSON is a tiny convenience for GET endpoints that decode into out.
// Optional query params are appended as URL-encoded form values.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	if len(query) > 0 {
		path = path + "?" + query.Encode()
	}
	return c.do(ctx, http.MethodGet, path, nil, out)
}
