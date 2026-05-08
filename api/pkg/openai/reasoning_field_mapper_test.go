package openai

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenameReasoningField(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantJSON  string // expected JSON (structurally compared); empty means unchanged
		wantExact string // if set, assert exact string equality instead
	}{
		{
			name:  "SSE delta with reasoning gets renamed",
			input: `data: {"id":"abc","choices":[{"index":0,"delta":{"role":"assistant","reasoning":"thinking hard"}}]}`,
			wantJSON: `{
				"id":"abc",
				"choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"thinking hard"}}]
			}`,
		},
		{
			name:  "SSE message with reasoning gets renamed",
			input: `data: {"id":"abc","choices":[{"index":0,"message":{"role":"assistant","reasoning":"thinking","content":"hello"}}]}`,
			wantJSON: `{
				"id":"abc",
				"choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"thinking","content":"hello"}}]
			}`,
		},
		{
			name:  "non-streaming JSON with reasoning in message",
			input: `{"id":"abc","choices":[{"index":0,"message":{"role":"assistant","reasoning":"deep thought","content":"42"}}]}`,
			wantJSON: `{
				"id":"abc",
				"choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"deep thought","content":"42"}}]
			}`,
		},
		{
			name:      "no reasoning field passes through unchanged",
			input:     `data: {"id":"abc","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`,
			wantExact: `data: {"id":"abc","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`,
		},
		{
			name:      "reasoning_content already present passes through unchanged",
			input:     `data: {"id":"abc","choices":[{"index":0,"delta":{"reasoning_content":"already correct"}}]}`,
			wantExact: `data: {"id":"abc","choices":[{"index":0,"delta":{"reasoning_content":"already correct"}}]}`,
		},
		{
			name:      "empty line passes through",
			input:     "",
			wantExact: "",
		},
		{
			name:      "data: [DONE] passes through",
			input:     "data: [DONE]",
			wantExact: "data: [DONE]",
		},
		{
			name:      "non-JSON text passes through",
			input:     ": keepalive",
			wantExact: ": keepalive",
		},
		{
			name:      "reasoning at top level is NOT renamed",
			input:     `{"reasoning":"top level","choices":[{"index":0,"delta":{"content":"hi"}}]}`,
			wantExact: `{"reasoning":"top level","choices":[{"index":0,"delta":{"content":"hi"}}]}`,
		},
		{
			name:  "reasoning nested inside tool_calls is NOT renamed",
			input: `data: {"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{\"reasoning\": \"inside tool\"}"}}]}}]}`,
			wantExact: `data: {"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{\"reasoning\": \"inside tool\"}"}}]}}]}`,
		},
		{
			name:  "multiple choices all get renamed",
			input: `{"choices":[{"index":0,"message":{"reasoning":"first"}},{"index":1,"message":{"reasoning":"second"}}]}`,
			wantJSON: `{
				"choices":[
					{"index":0,"message":{"reasoning_content":"first"}},
					{"index":1,"message":{"reasoning_content":"second"}}
				]
			}`,
		},
		{
			name:  "null reasoning value gets renamed",
			input: `{"choices":[{"index":0,"delta":{"reasoning":null,"content":"hi"}}]}`,
			wantJSON: `{"choices":[{"index":0,"delta":{"reasoning_content":null,"content":"hi"}}]}`,
		},
		{
			name:  "empty string reasoning gets renamed",
			input: `{"choices":[{"index":0,"delta":{"reasoning":"","content":"hi"}}]}`,
			wantJSON: `{"choices":[{"index":0,"delta":{"reasoning_content":"","content":"hi"}}]}`,
		},
		{
			name:  "tool_calls and content survive round-trip",
			input: `data: {"id":"x","object":"chat.completion.chunk","created":1776749289,"model":"Qwen/Qwen3.5","choices":[{"index":0,"delta":{"role":"assistant","content":"hello","reasoning":"thinking","tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"/tmp/test\"}"}}]},"finish_reason":null}],"usage":{"prompt_tokens":15,"completion_tokens":10,"total_tokens":25}}`,
			wantJSON: `{
				"id":"x",
				"object":"chat.completion.chunk",
				"created":1776749289,
				"model":"Qwen/Qwen3.5",
				"choices":[{
					"index":0,
					"delta":{
						"role":"assistant",
						"content":"hello",
						"reasoning_content":"thinking",
						"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"/tmp/test\"}"}}]
					},
					"finish_reason":null
				}],
				"usage":{"prompt_tokens":15,"completion_tokens":10,"total_tokens":25}
			}`,
		},
		{
			name: "content_filter_results survive round-trip",
			input: `data: {"choices":[{"index":0,"delta":{"reasoning":"think"},"content_filter_results":{"hate":{"filtered":false},"self_harm":{"filtered":false}}}]}`,
			wantJSON: `{
				"choices":[{
					"index":0,
					"delta":{"reasoning_content":"think"},
					"content_filter_results":{"hate":{"filtered":false},"self_harm":{"filtered":false}}
				}]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renameReasoningField(tt.input)

			if tt.wantExact != "" || tt.wantJSON == "" {
				assert.Equal(t, tt.wantExact, got, "expected exact string match")
				return
			}

			// Structural JSON comparison: parse both and compare
			gotJSON := got
			if strings.HasPrefix(gotJSON, "data: ") {
				gotJSON = gotJSON[6:]
			}

			var gotObj, wantObj any
			require.NoError(t, json.Unmarshal([]byte(gotJSON), &gotObj), "output is not valid JSON: %s", got)
			require.NoError(t, json.Unmarshal([]byte(tt.wantJSON), &wantObj), "test wantJSON is not valid JSON")
			assert.Equal(t, wantObj, gotObj)

			// Verify the prefix is preserved
			if strings.HasPrefix(tt.input, "data: ") {
				assert.True(t, strings.HasPrefix(got, "data: "), "SSE prefix lost")
			}
		})
	}
}

func TestRenameReasoningField_NumberPreservation(t *testing.T) {
	input := `{"id":"oai_abc","created":1776749289,"choices":[{"index":0,"message":{"reasoning":"think","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":10,"total_tokens":25}}`
	got := renameReasoningField(input)

	// Parse the output and verify numbers survived correctly
	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &obj))

	// Verify the created timestamp is correct
	created, ok := obj["created"].(float64)
	require.True(t, ok, "created should be a number")
	assert.Equal(t, float64(1776749289), created)

	// Verify usage numbers
	usage, ok := obj["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(15), usage["prompt_tokens"])
	assert.Equal(t, float64(10), usage["completion_tokens"])
	assert.Equal(t, float64(25), usage["total_tokens"])

	// Verify the rename happened
	choices := obj["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	assert.Equal(t, "think", msg["reasoning_content"])
	_, hasOld := msg["reasoning"]
	assert.False(t, hasOld, "old 'reasoning' key should be deleted")
}

func TestRenameReasoningInChoices_DepthPrecision(t *testing.T) {
	// Build a deeply nested structure to verify only the right level gets renamed
	obj := map[string]any{
		"reasoning": "top-level should NOT be renamed",
		"choices": []any{
			map[string]any{
				"reasoning": "choice-level should NOT be renamed",
				"delta": map[string]any{
					"reasoning": "delta-level SHOULD be renamed",
					"content":   "hello",
					"nested": map[string]any{
						"reasoning": "deeper should NOT be renamed",
					},
				},
				"message": map[string]any{
					"reasoning": "message-level SHOULD be renamed",
				},
			},
		},
	}

	changed := renameReasoningInChoices(obj)
	assert.True(t, changed)

	// Top level: untouched
	assert.Equal(t, "top-level should NOT be renamed", obj["reasoning"])

	choices := obj["choices"].([]any)
	choice := choices[0].(map[string]any)

	// Choice level: untouched
	assert.Equal(t, "choice-level should NOT be renamed", choice["reasoning"])

	// Delta level: renamed
	delta := choice["delta"].(map[string]any)
	assert.Equal(t, "delta-level SHOULD be renamed", delta["reasoning_content"])
	_, hasDeltaOld := delta["reasoning"]
	assert.False(t, hasDeltaOld)

	// Deeper nested inside delta: untouched
	nested := delta["nested"].(map[string]any)
	assert.Equal(t, "deeper should NOT be renamed", nested["reasoning"])
	_, hasNestedNew := nested["reasoning_content"]
	assert.False(t, hasNestedNew)

	// Message level: renamed
	msg := choice["message"].(map[string]any)
	assert.Equal(t, "message-level SHOULD be renamed", msg["reasoning_content"])
	_, hasMsgOld := msg["reasoning"]
	assert.False(t, hasMsgOld)
}

func TestReasoningFieldMapper_Integration(t *testing.T) {
	// Simulate a full SSE stream with multiple chunks
	sseStream := strings.Join([]string{
		`data: {"id":"abc","choices":[{"index":0,"delta":{"role":"assistant"}}]}`,
		``,
		`data: {"id":"abc","choices":[{"index":0,"delta":{"reasoning":"step 1"}}]}`,
		``,
		`data: {"id":"abc","choices":[{"index":0,"delta":{"reasoning":"step 2"}}]}`,
		``,
		`data: {"id":"abc","choices":[{"index":0,"delta":{"content":"hello"}}]}`,
		``,
		`data: {"id":"abc","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	mapper := &reasoningFieldMapper{
		ReadCloser: io.NopCloser(strings.NewReader(sseStream)),
	}

	output, err := io.ReadAll(mapper)
	require.NoError(t, err)

	lines := strings.Split(string(output), "\n")

	// Filter out empty lines and [DONE] for easier assertion
	var dataLines []string
	for _, l := range lines {
		if strings.HasPrefix(l, "data: ") && l != "data: [DONE]" {
			dataLines = append(dataLines, l)
		}
	}

	require.Len(t, dataLines, 5)

	// First chunk: no reasoning, should pass through structurally intact
	assertJSONContains(t, dataLines[0], "delta", map[string]any{"role": "assistant"})

	// Second and third chunks: reasoning renamed
	assertJSONContains(t, dataLines[1], "delta", map[string]any{"reasoning_content": "step 1"})
	assertJSONContains(t, dataLines[2], "delta", map[string]any{"reasoning_content": "step 2"})

	// Fourth chunk: content, no reasoning
	assertJSONContains(t, dataLines[3], "delta", map[string]any{"content": "hello"})

	// Fifth chunk: finish_reason
	var lastChunk map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(dataLines[4], "data: ")), &lastChunk))
	choices := lastChunk["choices"].([]any)
	choice := choices[0].(map[string]any)
	assert.Equal(t, "stop", choice["finish_reason"])

	// Verify [DONE] is preserved
	assert.Contains(t, string(output), "data: [DONE]")
}

func TestReasoningFieldMapper_SmallReadBuffer(t *testing.T) {
	// Verify the mapper works correctly when the caller uses a tiny read buffer,
	// forcing multiple Read calls per line
	input := `data: {"choices":[{"index":0,"delta":{"reasoning":"hello world"}}]}` + "\n"
	mapper := &reasoningFieldMapper{
		ReadCloser: io.NopCloser(strings.NewReader(input)),
	}

	// Read in 16-byte chunks
	var result []byte
	buf := make([]byte, 16)
	for {
		n, err := mapper.Read(buf)
		result = append(result, buf[:n]...)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}

	// Verify the output contains reasoning_content, not reasoning
	var obj map[string]any
	jsonStr := strings.TrimPrefix(strings.TrimSpace(string(result)), "data: ")
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &obj))
	delta := obj["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	assert.Equal(t, "hello world", delta["reasoning_content"])
	_, hasOld := delta["reasoning"]
	assert.False(t, hasOld)
}

func TestReasoningFieldMapper_NonStreaming(t *testing.T) {
	// Non-streaming response: single JSON object, no SSE framing
	input := `{"id":"abc","choices":[{"index":0,"message":{"role":"assistant","reasoning":"deep thought","content":"result"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}` + "\n"

	mapper := &reasoningFieldMapper{
		ReadCloser: io.NopCloser(strings.NewReader(input)),
	}

	output, err := io.ReadAll(mapper)
	require.NoError(t, err)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(output, &obj))

	msg := obj["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	assert.Equal(t, "deep thought", msg["reasoning_content"])
	assert.Equal(t, "result", msg["content"])
	assert.Equal(t, "assistant", msg["role"])
	_, hasOld := msg["reasoning"]
	assert.False(t, hasOld)
}

// assertJSONContains parses an SSE data line and checks that the first choice's
// delta/message matches the expected fields.
func assertJSONContains(t *testing.T, dataLine, fieldName string, expected map[string]any) {
	t.Helper()
	jsonStr := strings.TrimPrefix(dataLine, "data: ")
	var obj map[string]any
	require.NoError(t, json.Unmarshal([]byte(jsonStr), &obj))

	choices := obj["choices"].([]any)
	choice := choices[0].(map[string]any)
	field := choice[fieldName].(map[string]any)

	for k, v := range expected {
		assert.Equal(t, v, field[k], "field %s.%s mismatch", fieldName, k)
	}
}
