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

// defaultBaseURL is the production YellowDog REST API base. All
// service endpoints (account, compute, scheduler, images) live under
// this single prefix.
const defaultBaseURL = "https://portal.yellowdog.co/api"

// maxResponseBodyBytes caps how much of a response we read. Even
// well-behaved JSON list endpoints return a few hundred KB; an 8 MB
// limit protects against runaway responses without rejecting normal
// pagination.
const maxResponseBodyBytes = 8 << 20

// credentials is the YellowDog API key pair. json:"-" tags + the
// redaction methods below prevent accidental disclosure via the fmt
// package or accidental JSON marshalling.
type credentials struct {
	keyID  string `json:"-"`
	secret string `json:"-"`
}

func (c credentials) valid() bool { return c.keyID != "" && c.secret != "" }

// authHeader mirrors the Python SDK's header format
// (yellowdog_client/common/credentials/api_key_authentication_headers_provider.py):
//
//	Authorization: yd-key <keyID>:<secret>
//
// Do not log the return value.
func (c credentials) authHeader() string {
	return fmt.Sprintf("yd-key %s:%s", c.keyID, c.secret)
}

func (c credentials) String() string {
	return fmt.Sprintf("yellowdog.credentials{keyID: %q, secret: <redacted>}", maskKey(c.keyID))
}

func (c credentials) GoString() string { return c.String() }

func maskKey(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 6 {
		return "<redacted>"
	}
	return s[:6] + "..."
}

// apiError is the RFC 7807 problem+json shape the platform returns on
// 4xx / 5xx. Unexported because callers outside this package interact
// with the provider via compute.Provider, not with raw YD errors.
type apiError struct {
	Type     string `json:"type,omitempty"`
	Title    string `json:"title,omitempty"`
	Status   int    `json:"status,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func (e *apiError) Error() string {
	switch {
	case e.Detail != "":
		return fmt.Sprintf("yellowdog: %d %s: %s", e.Status, e.Title, e.Detail)
	case e.Title != "":
		return fmt.Sprintf("yellowdog: %d %s", e.Status, e.Title)
	default:
		return fmt.Sprintf("yellowdog: HTTP %d", e.Status)
	}
}

func isNotFound(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status == http.StatusNotFound
	}
	return false
}

// retryConfig tunes the simple exponential-backoff-with-jitter behaviour
// applied to idempotent requests. POST is never retried.
type retryConfig struct {
	maxAttempts    int
	initialBackoff time.Duration
}

var idempotentMethods = map[string]bool{
	http.MethodGet:    true,
	http.MethodHead:   true,
	http.MethodPut:    true,
	http.MethodDelete: true,
}

// doJSON sends method+path to the YD API with optional JSON body, decodes
// the response into out (if non-nil), and returns an *apiError for
// non-2xx responses. Retries idempotent methods on transient failure.
func doJSON(ctx context.Context, httpc *http.Client, creds credentials, baseURL, method, path string, query url.Values, body, out any, retry retryConfig) error {
	var bodyBytes []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("yellowdog: marshal request body: %w", err)
		}
		bodyBytes = raw
	}

	fullURL := baseURL + path
	if len(query) > 0 {
		sep := "?"
		if strings.Contains(fullURL, "?") {
			sep = "&"
		}
		fullURL += sep + query.Encode()
	}

	attempts := 1
	if retry.maxAttempts > 1 && idempotentMethods[method] {
		attempts = retry.maxAttempts
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if err := backoffSleep(ctx, retry.initialBackoff, attempt); err != nil {
				return err
			}
		}
		err := singleAttempt(ctx, httpc, creds, method, fullURL, bodyBytes, out)
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

func singleAttempt(ctx context.Context, httpc *http.Client, creds credentials, method, fullURL string, bodyBytes []byte, out any) error {
	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("yellowdog: new request %s %s: %w", method, fullURL, err)
	}
	req.Header.Set("Authorization", creds.authHeader())
	req.Header.Set("Accept", "application/json")
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpc.Do(req)
	if err != nil {
		return fmt.Errorf("yellowdog: %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBodyBytes)

	if resp.StatusCode >= 400 {
		ae := &apiError{Status: resp.StatusCode}
		_ = json.NewDecoder(limited).Decode(ae)
		if ae.Status == 0 {
			ae.Status = resp.StatusCode
		}
		_, _ = io.Copy(io.Discard, limited)
		return ae
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

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status >= 500 || ae.Status == http.StatusTooManyRequests
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func backoffSleep(ctx context.Context, initial time.Duration, attempt int) error {
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	d := initial << uint(attempt-1)
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	jitter := time.Duration(rand.Int63n(int64(d / 2)))
	d = d/2 + jitter
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
