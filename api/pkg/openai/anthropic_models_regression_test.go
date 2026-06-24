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
	var gotAuth, gotAPIKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-opus-4-1","display_name":"Claude","type":"model"}]}`))
	}))
	defer ts.Close()

	t.Run("without SetIsAnthropic sends Bearer (the bug)", func(t *testing.T) {
		client := NewWithOptions("sk-ant-test", ts.URL+"/v1", false, ClientOptions{})
		_, _ = client.ListModels(context.Background())
		require.Empty(t, gotAPIKey, "bug: x-api-key not sent")
		require.NotEmpty(t, gotAuth, "bug: falls through to OpenAI Bearer path")
	})

	gotAuth, gotAPIKey = "", ""

	t.Run("with SetIsAnthropic sends x-api-key (the fix)", func(t *testing.T) {
		client := NewWithOptions("sk-ant-test", ts.URL+"/v1", false, ClientOptions{})
		client.SetIsAnthropic(true)
		_, err := client.ListModels(context.Background())
		require.NoError(t, err)
		require.Equal(t, "sk-ant-test", gotAPIKey, "must send x-api-key")
		require.Empty(t, gotAuth, "must not send Authorization Bearer")
	})
}
