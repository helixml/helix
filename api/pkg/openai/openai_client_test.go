package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++

		if called > 2 {
			// On the third call return a valid response
			if err := json.NewEncoder(w).Encode(&openai.ChatCompletionResponse{
				Model: "test-model",
			}); err != nil {
				t.Logf("failed encoding request: %v", err)
			}
			return
		}

		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.NoError(t, err)

	require.Equal(t, 3, called)

	require.Equal(t, "test-model", resp.Model)
}

func TestIncludeUsage_Stream(t *testing.T) {
	var called bool

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		var req openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			require.NoError(t, err)
		}

		require.True(t, req.Stream)
		require.True(t, req.StreamOptions.IncludeUsage)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send a stream event that includes usage per IncludeUsage option
		payload := struct {
			Model string       `json:"model"`
			Usage openai.Usage `json:"usage"`
		}{
			Model: "test-model",
			Usage: openai.Usage{TotalTokens: 100},
		}
		b, err := json.Marshal(payload)
		require.NoError(t, err)

		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n\n"))

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Terminate the stream
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	stream, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{})
	require.NoError(t, err)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.True(t, called)
		require.Equal(t, 100, chunk.Usage.TotalTokens)
	}
}

func TestValidateModel_Denied(t *testing.T) {

	client := New("test", "https://api.openai.com/v1", false, "gpt-4.1")

	_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: "gpt-4.1-mini",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model gpt-4.1-mini is not in the list of allowed models")
}

func TestValidateMode_Stream_Denied(t *testing.T) {

	client := New("test", "https://api.openai.com/v1", false, "gpt-4.1")

	_, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{
		Model: "gpt-4.1-mini",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model gpt-4.1-mini is not in the list of allowed models")
}

func TestValidateModel_Success(t *testing.T) {

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(&openai.ChatCompletionResponse{
			Model: "test-model",
		}); err != nil {
			t.Logf("failed encoding request: %v", err)
		}
	}))
	defer ts.Close()

	client := New("test", ts.URL, true, "test-model")

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: "test-model",
	})
	require.NoError(t, err)

	require.Equal(t, "test-model", resp.Model)
}

func TestDoNotRetryOnAuthFailures(t *testing.T) {
	client := New("test", "https://api.together.xyz/v1", true)

	_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)
}

func TestDoNotRetryOnAuthFailures_TestServer(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++

		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)

	require.Equal(t, 1, called)
}
