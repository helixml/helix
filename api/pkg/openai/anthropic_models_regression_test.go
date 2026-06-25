package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAnthropicModels_HeaderRegression reproduces the prod bug where a UI/DB-added
// Anthropic provider (no SetIsAnthropic) hits /v1/models with Authorization: Bearer
// instead of x-api-key, causing Anthropic to return 401 "Invalid bearer token".
func TestAnthropicModels_HeaderRegression(t *testing.T) {
	// newServer returns a server plus a func that yields the captured headers
	// from the last request — scoped per-subtest so parallelism/ordering can't
	// leak captures between cases.
	newServer := func() (*httptest.Server, func() (auth, apiKey string)) {
		var gotAuth, gotAPIKey string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotAPIKey = r.Header.Get("x-api-key")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-opus-4-1","display_name":"Claude","type":"model"}]}`))
		}))
		return ts, func() (string, string) { return gotAuth, gotAPIKey }
	}

	t.Run("without SetIsAnthropic sends Bearer (the bug)", func(t *testing.T) {
		ts, captured := newServer()
		defer ts.Close()
		client := NewWithOptions("sk-ant-test", ts.URL+"/v1", false, ClientOptions{})
		_, _ = client.ListModels(context.Background())
		auth, apiKey := captured()
		require.Empty(t, apiKey, "bug: x-api-key not sent")
		require.NotEmpty(t, auth, "bug: falls through to OpenAI Bearer path")
	})

	t.Run("with SetIsAnthropic sends x-api-key (the fix)", func(t *testing.T) {
		ts, captured := newServer()
		defer ts.Close()
		client := NewWithOptions("sk-ant-test", ts.URL+"/v1", false, ClientOptions{})
		client.SetIsAnthropic(true)
		_, err := client.ListModels(context.Background())
		require.NoError(t, err)
		auth, apiKey := captured()
		require.Equal(t, "sk-ant-test", apiKey, "must send x-api-key")
		require.Empty(t, auth, "must not send Authorization Bearer")
	})
}
