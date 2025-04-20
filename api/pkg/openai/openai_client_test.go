package openai

import (
	"context"
	"encoding/json"
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

	client := New("test", ts.URL)

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.NoError(t, err)

	require.Equal(t, 3, called)

	require.Equal(t, "test-model", resp.Model)
}

func TestValidateModel_Denied(t *testing.T) {

	client := New("test", "https://api.openai.com/v1", "gpt-4.1")

	_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: "gpt-4.1-mini",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "model gpt-4.1-mini is not in the list of allowed models")
}

func TestValidateMode_Stream_Denied(t *testing.T) {

	client := New("test", "https://api.openai.com/v1", "gpt-4.1")

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

	client := New("test", ts.URL, "test-model")

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model: "test-model",
	})
	require.NoError(t, err)

	require.Equal(t, "test-model", resp.Model)
}

func TestDoNotRetryOnAuthFailures(t *testing.T) {
	client := New("test", "https://api.together.xyz/v1")

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

	client := New("test", ts.URL)

	_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)

	require.Equal(t, 1, called)
}
