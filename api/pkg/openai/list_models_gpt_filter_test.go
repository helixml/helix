package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// robertModelsResponse is the exact /models envelope a customer's
// OpenAI-compatible provider (Scaleway) returned on 2026-07-01. It lists 18
// models, one of which is gpt-oss-120b (OpenAI's open-weights model, served by
// a NON-OpenAI provider). Pre-fix, the presence of a "gpt-"-prefixed id tripped
// the OpenAI-only dall-e heuristic in ListModels and collapsed the whole
// catalogue down to just gpt-oss-120b.
const robertModelsResponse = `{
  "object": "list",
  "data": [
    {"id":"llama-3.3-70b-instruct","object":"model","created":1736258559,"owned_by":"meta"},
    {"id":"pixtral-12b-2409","object":"model","created":1730385501,"owned_by":"mistral"},
    {"id":"gemma-3-27b-it","object":"model","created":1730385501,"owned_by":"google"},
    {"id":"bge-multilingual-gemma2","object":"model","created":1730385501,"owned_by":"baai"},
    {"id":"qwen3-235b-a22b-instruct-2507","object":"model","created":1754049528,"owned_by":"qwen"},
    {"id":"mistral-small-3.2-24b-instruct-2506","object":"model","created":1755006551,"owned_by":"mistral"},
    {"id":"qwen3-coder-30b-a3b-instruct","object":"model","created":1755006551,"owned_by":"qwen"},
    {"id":"gpt-oss-120b","object":"model","created":1755093042,"owned_by":"openai"},
    {"id":"voxtral-small-24b-2507","object":"model","created":1757330049,"owned_by":"mistral"},
    {"id":"whisper-large-v3","object":"model","created":1722263169,"owned_by":"openai"},
    {"id":"qwen3-embedding-8b","object":"model","created":1755006551,"owned_by":"qwen"},
    {"id":"holo2-30b-a3b","object":"model","created":1755006551,"owned_by":"hcompany"},
    {"id":"devstral-2-123b-instruct-2512","object":"model","created":1766496679,"owned_by":"mistral"},
    {"id":"qwen3.5-397b-a17b","object":"model","created":1773394870,"owned_by":"qwen"},
    {"id":"gemma-4-26b-a4b-it","object":"model","created":1777469299,"owned_by":"google"},
    {"id":"qwen3.6-35b-a3b","object":"model","created":1778158991,"owned_by":"qwen"},
    {"id":"mistral-medium-3.5-128b","object":"model","created":1778158991,"owned_by":"mistral"},
    {"id":"glm-5.2","object":"model","created":1782376549,"owned_by":"zai"}
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

// TestListModels_NonOpenAIProviderKeepsAllModels is the regression test for the
// customer report on 2026-07-01: a Scaleway endpoint serving gpt-oss-120b had
// 17 of its 18 models silently dropped from the Helix picker because the
// gpt-oss-120b id tripped the OpenAI-only dall-e filter heuristic.
func TestListModels_NonOpenAIProviderKeepsAllModels(t *testing.T) {
	srv := newModelsTestServer(t, robertModelsResponse)
	defer srv.Close()

	client := New("test-key", srv.URL, false)
	models, err := client.ListModels(context.Background())
	require.NoError(t, err)

	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}

	// The gpt filter must NOT fire for a non-OpenAI base URL. Every model the
	// provider advertised must survive (none of the 18 match the audio/tts/
	// realtime substrings filterUnsupportedModels drops).
	require.Contains(t, ids, "glm-5.2", "the model the customer actually wanted must survive")
	require.Contains(t, ids, "llama-3.3-70b-instruct")
	require.Contains(t, ids, "qwen3-coder-30b-a3b-instruct")
	require.Contains(t, ids, "gpt-oss-120b", "the gpt- model itself must still be present")
	require.Len(t, ids, 18,
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
		{ID: "glm-5.2"},
		{ID: "llama-3.3-70b-instruct"},
	}

	got := applyOpenAIDallEFilter(models, "https://api.scaleway.ai/v1")
	require.Len(t, got, 3, "non-OpenAI provider list must pass through untouched")
}
