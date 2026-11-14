package openai

import (
	"context"
	"errors"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

// ProviderError marks a chat-completion failure that happened at the upstream
// provider (an HTTP refusal, or a transport failure reaching it) rather than
// inside Helix. Header holds the provider's back-off headers (Retry-After,
// X-Ratelimit-Reset-*) from the response that failed the request.
type ProviderError struct {
	Err    error
	Header http.Header
}

func (e *ProviderError) Error() string { return e.Err.Error() }

func (e *ProviderError) Unwrap() error { return e.Err }

// ProviderStatusCode extracts the HTTP status from a provider SDK error. The
// SDK error types carry the code as a field, so read it rather than looking
// for the digits in the rendered message: a provider that echoes the request
// back in its body can easily put "400" inside a 502's message.
//
// Returns 0 when the error carries no HTTP status (a transport failure, or a
// mid-stream error event).
func ProviderStatusCode(err error) int {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && apiErr.HTTPStatusCode > 0 {
		return apiErr.HTTPStatusCode
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) && reqErr.HTTPStatusCode > 0 {
		return reqErr.HTTPStatusCode
	}
	var genaiErr genai.APIError
	if errors.As(err, &genaiErr) && genaiErr.Code > 0 {
		return genaiErr.Code
	}
	return 0
}

// isBackoffHeader reports whether a provider response header tells the caller
// when it may retry.
func isBackoffHeader(name string) bool {
	canonical := http.CanonicalHeaderKey(name)
	return canonical == "Retry-After" || strings.HasPrefix(canonical, "X-Ratelimit-Reset-")
}

type providerHeaderCaptureKey struct{}

// providerHeaderCapture records the back-off headers of the most recent failed
// provider response for one chat-completion call. Attempts of a call run
// sequentially, so it needs no locking.
type providerHeaderCapture struct {
	header http.Header
}

func withProviderHeaderCapture(ctx context.Context) (context.Context, *providerHeaderCapture) {
	capture := &providerHeaderCapture{}
	return context.WithValue(ctx, providerHeaderCaptureKey{}, capture), capture
}

// recordProviderResponse is called by the HTTP interceptor for every provider
// response; a failed one replaces whatever an earlier attempt recorded.
func recordProviderResponse(ctx context.Context, resp *http.Response) {
	capture, ok := ctx.Value(providerHeaderCaptureKey{}).(*providerHeaderCapture)
	if !ok || resp == nil || resp.StatusCode < 400 {
		return
	}
	header := http.Header{}
	for name, values := range resp.Header {
		if isBackoffHeader(name) {
			header[http.CanonicalHeaderKey(name)] = values
		}
	}
	capture.header = header
}

// reset forgets the previous attempt's headers, so a retry that then fails at
// the transport does not inherit an earlier 429's Retry-After.
func (c *providerHeaderCapture) reset() { c.header = nil }

func (c *providerHeaderCapture) wrap(err error) error {
	return asProviderError(err, c.header)
}

func asProviderError(err error, header http.Header) error {
	if err == nil {
		return nil
	}
	return &ProviderError{Err: err, Header: header}
}
