package yellowdog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the YellowDog public platform endpoint, sourced from
// the official Python SDK's ServicesSchema default
// (yellowdog_client/__init__.py).
const DefaultBaseURL = "https://portal.yellowdog.co/api"

// Client is a thin wrapper around http.Client that knows how to talk to
// the YellowDog REST API.
//
// Construct one per (credentials, endpoint) tuple. Safe for concurrent
// use by multiple goroutines; the embedded *http.Client owns its own
// connection pool.
type Client struct {
	baseURL string
	creds   Credentials
	httpc   *http.Client
}

// Option mutates a Client at construction time. Use NewClient with these.
type Option func(*Client)

// WithBaseURL overrides the platform endpoint. Useful for hitting a
// staging environment, or a mock server in tests. The trailing "/api"
// segment is part of the platform URL by convention; leave it on.
func WithBaseURL(u string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(u, "/")
	}
}

// WithHTTPClient injects a custom http.Client. Helix typically threads
// an instrumented client (metrics, tracing) here.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		c.httpc = h
	}
}

// NewClient returns a Client ready to use. It does NOT verify the
// credentials at construction time - the first call that hits an
// authenticated endpoint will surface auth errors as APIError with
// Status=401.
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
	return c, nil
}

// do is the shared request path for every endpoint method. It
// marshals body to JSON (when non-nil), sets auth + content-type
// headers, fires the request, and decodes a JSON response into out
// (when non-nil). Non-2xx responses are decoded into APIError and
// returned as the error.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("yellowdog: marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	fullURL := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("yellowdog: new request %s %s: %w", method, fullURL, err)
	}
	req.Header.Set("Authorization", c.creds.authHeader())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("yellowdog: %s %s: %w", method, fullURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		apiErr := &APIError{Status: resp.StatusCode}
		// Best-effort decode - some 4xx may not be JSON.
		_ = json.NewDecoder(resp.Body).Decode(apiErr)
		if apiErr.Status == 0 {
			apiErr.Status = resp.StatusCode
		}
		return apiErr
	}

	if out == nil {
		// Caller doesn't want the response body. Drain it so the
		// connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("yellowdog: decode response %s %s: %w", method, fullURL, err)
	}
	return nil
}

// getJSON is a tiny convenience for GET endpoints that decode into out.
// Optional query params are appended as URL-encoded form values.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	if len(query) > 0 {
		path = path + "?" + query.Encode()
	}
	return c.do(ctx, http.MethodGet, path, nil, out)
}
