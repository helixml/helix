package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

// staticTokenSource returns a fixed token for testing.
type staticTokenSource struct {
	token string
}

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: s.token}, nil
}

func TestVertexBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		expected string
	}{
		{
			name:     "global region uses plain domain",
			region:   "global",
			expected: "https://aiplatform.googleapis.com/",
		},
		{
			name:     "us-east5 region uses regional domain",
			region:   "us-east5",
			expected: "https://us-east5-aiplatform.googleapis.com/",
		},
		{
			name:     "us-central1 region",
			region:   "us-central1",
			expected: "https://us-central1-aiplatform.googleapis.com/",
		},
		{
			name:     "europe-west4 region",
			region:   "europe-west4",
			expected: "https://europe-west4-aiplatform.googleapis.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, VertexBaseURL(tt.region))
		})
	}
}

func TestVertexTransformRequest_NonStreaming(t *testing.T) {
	body := map[string]interface{}{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/v1/messages", io.NopCloser(bytes.NewReader(bodyBytes)))
	require.NoError(t, err)
	req.Header.Set("x-api-key", "should-be-removed")
	req.ContentLength = int64(len(bodyBytes))

	ts := &staticTokenSource{token: "test-vertex-token"}

	err = vertexTransformRequest(req, "helixml", "global", ts)
	require.NoError(t, err)

	// Check URL was rewritten to rawPredict (non-streaming), model name uses @ format for Vertex
	assert.Equal(t, "/v1/projects/helixml/locations/global/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict", req.URL.Path)
	assert.Equal(t, "aiplatform.googleapis.com", req.URL.Host)
	assert.Equal(t, "https", req.URL.Scheme)

	// Check auth headers
	assert.Equal(t, "Bearer test-vertex-token", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("x-api-key"))

	// Check body: model should be removed, anthropic_version should be injected
	newBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	var result map[string]json.RawMessage
	err = json.Unmarshal(newBody, &result)
	require.NoError(t, err)

	_, hasModel := result["model"]
	assert.False(t, hasModel, "model should be removed from body")

	var version string
	err = json.Unmarshal(result["anthropic_version"], &version)
	require.NoError(t, err)
	assert.Equal(t, VertexAnthropicVersion, version)

	// max_tokens should still be present
	var maxTokens int
	err = json.Unmarshal(result["max_tokens"], &maxTokens)
	require.NoError(t, err)
	assert.Equal(t, 1024, maxTokens)
}

func TestVertexTransformRequest_Streaming(t *testing.T) {
	body := map[string]interface{}{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"stream":     true,
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/v1/messages", io.NopCloser(bytes.NewReader(bodyBytes)))
	require.NoError(t, err)
	req.ContentLength = int64(len(bodyBytes))

	ts := &staticTokenSource{token: "test-token"}

	err = vertexTransformRequest(req, "helixml", "us-east5", ts)
	require.NoError(t, err)

	// Check URL uses streamRawPredict for streaming, model name uses @ format
	assert.Equal(t, "/v1/projects/helixml/locations/us-east5/publishers/anthropic/models/claude-sonnet-4@20250514:streamRawPredict", req.URL.Path)
	assert.Equal(t, "us-east5-aiplatform.googleapis.com", req.URL.Host)
}

func TestVertexTransformRequest_GlobalRegion(t *testing.T) {
	body := map[string]interface{}{
		"model":      "claude-opus-4-20250514",
		"max_tokens": 2048,
		"messages": []map[string]string{
			{"role": "user", "content": "Test"},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/v1/messages", io.NopCloser(bytes.NewReader(bodyBytes)))
	require.NoError(t, err)
	req.ContentLength = int64(len(bodyBytes))

	ts := &staticTokenSource{token: "tok"}
	err = vertexTransformRequest(req, "my-project", "global", ts)
	require.NoError(t, err)

	assert.Equal(t, "aiplatform.googleapis.com", req.URL.Host)
	assert.Equal(t, "/v1/projects/my-project/locations/global/publishers/anthropic/models/claude-opus-4@20250514:rawPredict", req.URL.Path)
}

func TestVertexTransformRequest_PreservesExistingAnthropicVersion(t *testing.T) {
	body := map[string]interface{}{
		"model":             "claude-sonnet-4-20250514",
		"max_tokens":        1024,
		"anthropic_version": "2023-06-01",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/v1/messages", io.NopCloser(bytes.NewReader(bodyBytes)))
	require.NoError(t, err)
	req.ContentLength = int64(len(bodyBytes))

	ts := &staticTokenSource{token: "tok"}
	err = vertexTransformRequest(req, "proj", "global", ts)
	require.NoError(t, err)

	newBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	var result map[string]json.RawMessage
	err = json.Unmarshal(newBody, &result)
	require.NoError(t, err)

	var version string
	err = json.Unmarshal(result["anthropic_version"], &version)
	require.NoError(t, err)
	assert.Equal(t, "2023-06-01", version, "existing anthropic_version should be preserved, not overwritten")
}

func TestVertexTransformRequest_CountTokens(t *testing.T) {
	body := map[string]interface{}{
		"model": "claude-sonnet-4-20250514",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/v1/messages/count_tokens", io.NopCloser(bytes.NewReader(bodyBytes)))
	require.NoError(t, err)
	req.ContentLength = int64(len(bodyBytes))

	ts := &staticTokenSource{token: "tok"}
	err = vertexTransformRequest(req, "helixml", "global", ts)
	require.NoError(t, err)

	assert.Equal(t, "/v1/projects/helixml/locations/global/publishers/anthropic/models/count-tokens:rawPredict", req.URL.Path)
}

func TestVertexTransformRequest_NilBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://proxy.example.com/v1/models", nil)
	require.NoError(t, err)
	req.Header.Set("x-api-key", "old-key")

	ts := &staticTokenSource{token: "vertex-tok"}
	err = vertexTransformRequest(req, "proj", "global", ts)
	require.NoError(t, err)

	// Auth should still be set even with nil body
	assert.Equal(t, "Bearer vertex-tok", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("x-api-key"))

	// URL host/scheme should be set even with nil body
	assert.Equal(t, "aiplatform.googleapis.com", req.URL.Host)
	assert.Equal(t, "https", req.URL.Scheme)
}

func TestVertexTransformRequest_ContentLengthUpdated(t *testing.T) {
	body := map[string]interface{}{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": "Hello"},
		},
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "https://proxy.example.com/v1/messages", io.NopCloser(bytes.NewReader(bodyBytes)))
	require.NoError(t, err)
	req.ContentLength = int64(len(bodyBytes))

	ts := &staticTokenSource{token: "tok"}
	err = vertexTransformRequest(req, "proj", "global", ts)
	require.NoError(t, err)

	// Body was modified (model removed, anthropic_version added), so content length should be updated
	newBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, int64(len(newBody)), req.ContentLength, "ContentLength should match actual body size")

	// GetBody should also work
	require.NotNil(t, req.GetBody)
	bodyReader, err := req.GetBody()
	require.NoError(t, err)
	getBodyContent, err := io.ReadAll(bodyReader)
	require.NoError(t, err)
	assert.Equal(t, newBody, getBodyContent, "GetBody should return same content")
}

func TestTokenSourceCacheKey(t *testing.T) {
	tests := []struct {
		name            string
		credentialsJSON string
		credentialsFile string
		expectedPrefix  string
	}{
		{
			name:            "JSON string produces json: prefix",
			credentialsJSON: `{"type":"service_account","project_id":"test"}`,
			credentialsFile: "",
			expectedPrefix:  "json:",
		},
		{
			name:            "JSON takes precedence over file",
			credentialsJSON: `{"type":"service_account","project_id":"test"}`,
			credentialsFile: "/some/path.json",
			expectedPrefix:  "json:",
		},
		{
			name:            "file path produces file: prefix",
			credentialsJSON: "",
			credentialsFile: "/run/secrets/vertex-credentials/key",
			expectedPrefix:  "file:",
		},
		{
			name:            "neither produces adc key",
			credentialsJSON: "",
			credentialsFile: "",
			expectedPrefix:  "adc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tokenSourceCacheKey(tt.credentialsJSON, tt.credentialsFile)
			assert.True(t, len(key) > 0, "cache key should not be empty")
			if tt.expectedPrefix == "adc" {
				assert.Equal(t, "adc", key)
			} else {
				assert.Contains(t, key, tt.expectedPrefix)
			}
		})
	}

	// Same JSON content should produce the same key
	key1 := tokenSourceCacheKey(`{"type":"service_account"}`, "")
	key2 := tokenSourceCacheKey(`{"type":"service_account"}`, "")
	assert.Equal(t, key1, key2, "identical JSON should produce identical cache keys")

	// Different JSON content should produce different keys
	key3 := tokenSourceCacheKey(`{"type":"service_account","project_id":"a"}`, "")
	key4 := tokenSourceCacheKey(`{"type":"service_account","project_id":"b"}`, "")
	assert.NotEqual(t, key3, key4, "different JSON should produce different cache keys")
}

func TestConvertModelNameToVertex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-sonnet-4-20250514", "claude-sonnet-4@20250514"},
		{"claude-opus-4-5-20251101", "claude-opus-4-5@20251101"},
		{"claude-haiku-3-5-20241022", "claude-haiku-3-5@20241022"},
		{"claude-sonnet-4@20250514", "claude-sonnet-4@20250514"}, // already @ format
		{"claude-sonnet-4", "claude-sonnet-4"},                   // versionless → bare (Vertex accepts without @latest)
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},               // 4.6 versionless → bare
		{"claude-opus-4-6", "claude-opus-4-6"},                   // 4.6 versionless → bare
		{"claude-sonnet-4-6-latest", "claude-sonnet-4-6"},        // strip -latest (Zed normalization artifact)
		{"claude-3-opus", "claude-3-opus"},                       // old naming → bare
		{"claude-3-opus-latest", "claude-3-opus"},                // strip -latest
		{"gpt-4o", "gpt-4o"},                                    // non-claude unchanged
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, convertModelNameToVertex(tt.input))
		})
	}
}
