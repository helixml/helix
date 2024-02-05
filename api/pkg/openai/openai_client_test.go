package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/lukemarsden/go-openai2"
	"github.com/stretchr/testify/require"
)

func TestRetry(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++

		if called > 2 {
			// On the third call return a valid response
			json.NewEncoder(w).Encode(&openai.ChatCompletionResponse{
				Model: "test-model",
			})
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

func TestDoNotRetryOnAuthFailures(t *testing.T) {
	client := New("test", "https://api.together.xyz/v1")

	_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)
}

func TestDoNotRetryOnAuthFailures_TestServer(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++

		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := New("test", ts.URL)

	_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)

	require.Equal(t, 1, called)
}
