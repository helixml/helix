package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRoundTripper records each request's body and replies with a sequence
// of pre-canned responses. The N-th request gets responses[N-1].
type fakeRoundTripper struct {
	responses     []*http.Response
	receivedBody  [][]byte
	receivedCalls atomic.Int32
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	idx := int(f.receivedCalls.Add(1)) - 1
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}
	f.receivedBody = append(f.receivedBody, body)
	if idx >= len(f.responses) {
		return nil, io.EOF
	}
	return f.responses[idx], nil
}

func newJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Body:          io.NopCloser(strings.NewReader(body)),
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		ContentLength: int64(len(body)),
	}
}

func newRequestWithThinking(t *testing.T, thinking map[string]interface{}) *http.Request {
	t.Helper()
	body := map[string]interface{}{
		"model":      "claude-opus-4-7",
		"max_tokens": 1024,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
		"thinking": thinking,
	}
	bs, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", bytes.NewReader(bs))
	require.NoError(t, err)
	req.ContentLength = int64(len(bs))
	return req
}

func TestThinkingRetryTransport_AdaptiveRejectedSwapsToEnabled(t *testing.T) {
	rejection := `{"type":"error","error":{"type":"invalid_request_error","message":"thinking: Input tag 'adaptive' found using 'type' does not match any of the expected tags: 'disabled', 'enabled'"}}`
	success := `{"id":"msg_1","type":"message","content":[]}`

	rt := &fakeRoundTripper{
		responses: []*http.Response{
			newJSONResponse(http.StatusBadRequest, rejection),
			newJSONResponse(http.StatusOK, success),
		},
	}
	transport := &thinkingRetryTransport{base: rt}

	req := newRequestWithThinking(t, map[string]interface{}{
		"type":    "adaptive",
		"display": "summarized",
	})

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	require.Equal(t, int32(2), rt.receivedCalls.Load(), "expected exactly one retry")

	// Second request body should have type=enabled with budget injected, no display.
	var retryBody map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rt.receivedBody[1], &retryBody))
	var thinking map[string]interface{}
	require.NoError(t, json.Unmarshal(retryBody["thinking"], &thinking))
	assert.Equal(t, "enabled", thinking["type"])
	assert.EqualValues(t, 4096, thinking["budget_tokens"])
	_, hasDisplay := thinking["display"]
	assert.False(t, hasDisplay, "display field should be stripped")
}

func TestThinkingRetryTransport_EnabledRejectedSwapsToAdaptive(t *testing.T) {
	rejection := `{"type":"error","error":{"type":"invalid_request_error","message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\" to control thinking behavior."}}`
	success := `{"id":"msg_2","type":"message","content":[]}`

	rt := &fakeRoundTripper{
		responses: []*http.Response{
			newJSONResponse(http.StatusBadRequest, rejection),
			newJSONResponse(http.StatusOK, success),
		},
	}
	transport := &thinkingRetryTransport{base: rt}

	req := newRequestWithThinking(t, map[string]interface{}{
		"type":          "enabled",
		"budget_tokens": 4096,
	})

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(2), rt.receivedCalls.Load())

	var retryBody map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rt.receivedBody[1], &retryBody))
	var thinking map[string]interface{}
	require.NoError(t, json.Unmarshal(retryBody["thinking"], &thinking))
	assert.Equal(t, "adaptive", thinking["type"])
	_, hasBudget := thinking["budget_tokens"]
	assert.False(t, hasBudget, "budget_tokens should be stripped when switching to adaptive")

	var outputConfig map[string]interface{}
	require.NoError(t, json.Unmarshal(retryBody["output_config"], &outputConfig))
	assert.Equal(t, "medium", outputConfig["effort"], "should default output_config.effort to medium when swapping to adaptive (otherwise the model produces an empty thinking summary)")
}

func TestSwapThinkingType_AdaptivePreservesExistingOutputConfigEffort(t *testing.T) {
	// If the caller already specified an effort, swapThinkingType must not
	// clobber it — they may have a deliberate reason for high or low.
	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":4096},"output_config":{"effort":"high","other_field":"keep_me"}}`)
	out, ok := swapThinkingType(body, "adaptive")
	require.True(t, ok)

	var bodyMap map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &bodyMap))
	outputConfig, _ := bodyMap["output_config"].(map[string]interface{})
	assert.Equal(t, "high", outputConfig["effort"], "pre-set effort must not be overwritten")
	assert.Equal(t, "keep_me", outputConfig["other_field"], "sibling fields under output_config must be preserved")
}

func TestThinkingRetryTransport_NonThinking400PassesThrough(t *testing.T) {
	rejection := `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens too high"}}`

	rt := &fakeRoundTripper{
		responses: []*http.Response{
			newJSONResponse(http.StatusBadRequest, rejection),
		},
	}
	transport := &thinkingRetryTransport{base: rt}

	req := newRequestWithThinking(t, map[string]interface{}{
		"type": "enabled",
	})

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, int32(1), rt.receivedCalls.Load(), "should not retry on unrelated 400")

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, rejection, string(body), "original response body should be returned unchanged")
}

func TestThinkingRetryTransport_SuccessFirstTryNoRetry(t *testing.T) {
	success := `{"id":"msg_3","type":"message"}`

	rt := &fakeRoundTripper{
		responses: []*http.Response{
			newJSONResponse(http.StatusOK, success),
		},
	}
	transport := &thinkingRetryTransport{base: rt}

	req := newRequestWithThinking(t, map[string]interface{}{
		"type": "adaptive",
	})

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), rt.receivedCalls.Load())
}

func TestThinkingRetryTransport_NoBodyOrGetRequestBypasses(t *testing.T) {
	rt := &fakeRoundTripper{
		responses: []*http.Response{
			newJSONResponse(http.StatusOK, `{}`),
		},
	}
	transport := &thinkingRetryTransport{base: rt}

	req, err := http.NewRequest(http.MethodGet, "https://example.com/v1/models", nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), rt.receivedCalls.Load())
}

func TestThinkingRetryTransport_BodyWithoutThinkingFieldDoesNotRetry(t *testing.T) {
	// Even if we hit a thinking-type 400 (defensive), without a thinking field
	// in the body we have nothing to swap — return the original 400.
	rejection := `{"error":{"message":"thinking: Input tag 'adaptive' found"}}`
	rt := &fakeRoundTripper{
		responses: []*http.Response{
			newJSONResponse(http.StatusBadRequest, rejection),
		},
	}
	transport := &thinkingRetryTransport{base: rt}

	body := []byte(`{"model":"claude","messages":[{"role":"user","content":"hi"}]}`)
	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", bytes.NewReader(body))
	require.NoError(t, err)
	req.ContentLength = int64(len(body))

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, int32(1), rt.receivedCalls.Load())
}

func TestSwapThinkingType(t *testing.T) {
	t.Run("adaptive_to_enabled_strips_display_and_adds_budget", func(t *testing.T) {
		body := []byte(`{"model":"x","thinking":{"type":"adaptive","display":"summarized"}}`)
		out, ok := swapThinkingType(body, "enabled")
		require.True(t, ok)
		var parsed map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &parsed))
		var thinking map[string]interface{}
		require.NoError(t, json.Unmarshal(parsed["thinking"], &thinking))
		assert.Equal(t, "enabled", thinking["type"])
		assert.EqualValues(t, 4096, thinking["budget_tokens"])
		_, hasDisplay := thinking["display"]
		assert.False(t, hasDisplay)
	})

	t.Run("adaptive_to_enabled_preserves_existing_budget", func(t *testing.T) {
		body := []byte(`{"thinking":{"type":"adaptive","budget_tokens":8192}}`)
		out, ok := swapThinkingType(body, "enabled")
		require.True(t, ok)
		var parsed map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &parsed))
		var thinking map[string]interface{}
		require.NoError(t, json.Unmarshal(parsed["thinking"], &thinking))
		assert.EqualValues(t, 8192, thinking["budget_tokens"], "existing budget should be preserved")
	})

	t.Run("enabled_to_adaptive_strips_budget", func(t *testing.T) {
		body := []byte(`{"thinking":{"type":"enabled","budget_tokens":4096}}`)
		out, ok := swapThinkingType(body, "adaptive")
		require.True(t, ok)
		var parsed map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out, &parsed))
		var thinking map[string]interface{}
		require.NoError(t, json.Unmarshal(parsed["thinking"], &thinking))
		assert.Equal(t, "adaptive", thinking["type"])
		_, hasBudget := thinking["budget_tokens"]
		assert.False(t, hasBudget)
	})

	t.Run("missing_thinking_field_returns_false", func(t *testing.T) {
		body := []byte(`{"model":"x"}`)
		_, ok := swapThinkingType(body, "enabled")
		assert.False(t, ok)
	})

	t.Run("invalid_json_returns_false", func(t *testing.T) {
		_, ok := swapThinkingType([]byte("not json"), "enabled")
		assert.False(t, ok)
	})
}

func TestDetectThinkingTypeMismatch(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "adaptive_rejected",
			body: `{"error":{"message":"thinking: Input tag 'adaptive' found using 'type' does not match any of the expected tags: 'disabled', 'enabled'"}}`,
			want: "enabled",
		},
		{
			name: "enabled_rejected_for_opus",
			body: `{"error":{"message":"\"thinking.type.enabled\" is not supported for this model. Use \"thinking.type.adaptive\" and \"output_config.effort\" to control thinking behavior."}}`,
			want: "adaptive",
		},
		{
			name: "unrelated_400",
			body: `{"error":{"message":"max_tokens exceeds limit"}}`,
			want: "",
		},
		{
			name: "empty_body",
			body: ``,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, detectThinkingTypeMismatch([]byte(tt.body)))
		})
	}
}
