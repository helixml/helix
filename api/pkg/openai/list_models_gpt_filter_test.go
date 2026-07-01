package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// compatibleProviderModelsResponse is a representative /models envelope from an
// OpenAI-compatible aggregator: a broad multi-vendor catalogue that happens to
// include one gpt-prefixed id (gpt-oss-120b, OpenAI's open-weights model served
// by a non-OpenAI backend). Pre-fix, that single gpt- id tripped the OpenAI-only
// dall-e heuristic and collapsed the whole catalogue down to just that model.
const compatibleProviderModelsResponse = `{
  "object": "list",
  "data": [
    {"id":"llama-3.3-70b-instruct","object":"model","owned_by":"vendor-a"},
    {"id":"vision-12b","object":"model","owned_by":"vendor-b"},
    {"id":"small-27b-it","object":"model","owned_by":"vendor-c"},
    {"id":"bge-multilingual","object":"model","owned_by":"vendor-d"},
    {"id":"chat-235b-a22b-instruct","object":"model","owned_by":"vendor-e"},
    {"id":"small-24b-instruct","object":"model","owned_by":"vendor-b"},
    {"id":"coder-30b-a3b-instruct","object":"model","owned_by":"vendor-e"},
    {"id":"gpt-oss-120b","object":"model","owned_by":"openai"},
    {"id":"embedding-8b","object":"model","owned_by":"vendor-e"},
    {"id":"agentic-30b-a3b","object":"model","owned_by":"vendor-f"},
    {"id":"devcode-123b-instruct","object":"model","owned_by":"vendor-b"},
    {"id":"chat-397b-a17b","object":"model","owned_by":"vendor-e"},
    {"id":"small-26b-a4b-it","object":"model","owned_by":"vendor-c"},
    {"id":"chat-35b-a3b","object":"model","owned_by":"vendor-e"},
    {"id":"medium-128b","object":"model","owned_by":"vendor-b"},
    {"id":"reasoning-5.2","object":"model","owned_by":"vendor-g"}
  ]
}`

func newModelsTestServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

// TestListModels_NonOpenAIProviderKeepsAllModels is the regression test for a
// report where an OpenAI-compatible provider serving gpt-oss-120b had all but
// that one model silently dropped from the Helix picker, because the gpt-oss-120b
// id tripped the OpenAI-only dall-e filter heuristic.
func TestListModels_NonOpenAIProviderKeepsAllModels(t *testing.T) {
	srv := newModelsTestServer(t, compatibleProviderModelsResponse)
	defer srv.Close()

	client := New("test-key", srv.URL, false)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)

	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}

	// The gpt filter must NOT fire for a non-OpenAI base URL. Every model the
	// provider advertised must survive (none match the audio/tts/realtime
	// substrings filterUnsupportedModels drops).
	require.Contains(t, ids, "reasoning-5.2", "a non-gpt model the user wants must survive")
	require.Contains(t, ids, "llama-3.3-70b-instruct")
	require.Contains(t, ids, "coder-30b-a3b-instruct")
	require.Contains(t, ids, "gpt-oss-120b", "the gpt- model itself must still be present")
	require.Len(t, ids, 16,
		"a non-OpenAI provider must return its full catalogue; got %d models: %v", len(ids), ids)
}

// TestApplyOpenAIDallEFilter_RealOpenAIStillFilters keeps the original heuristic
// alive for actual api.openai.com endpoints: dall-e style rows we can't send
// chat completions to must still be dropped, while gpt- / o-series / embeddings
// survive.
func TestApplyOpenAIDallEFilter_RealOpenAIStillFilters(t *testing.T) {
	models := []types.OpenAIModel{
		{ID: "gpt-4.1"},
		{ID: "o3-mini"},
		{ID: "dall-e-3"},
		{ID: "text-embedding-3-small"},
	}

	got := applyOpenAIDallEFilter(models, "https://api.openai.com/v1")
	ids := make([]string, 0, len(got))
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	require.ElementsMatch(t, []string{"gpt-4.1", "o3-mini", "text-embedding-3-small"}, ids,
		"real OpenAI endpoint must still drop dall-e-3 (non-chat) while keeping gpt/o-series/embeddings")
}

// TestApplyOpenAIDallEFilter_NonOpenAIUntouched pins the core of the fix at the
// unit level: a non-OpenAI base URL is a no-op even when a gpt- model is present.
func TestApplyOpenAIDallEFilter_NonOpenAIUntouched(t *testing.T) {
	models := []types.OpenAIModel{
		{ID: "gpt-oss-120b"},
		{ID: "reasoning-5.2"},
		{ID: "llama-3.3-70b-instruct"},
	}

	got := applyOpenAIDallEFilter(models, "https://api.example-aggregator.ai/v1")
	require.Len(t, got, 3, "non-OpenAI provider list must pass through untouched")
}
