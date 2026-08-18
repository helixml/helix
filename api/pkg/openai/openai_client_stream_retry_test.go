package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

// writeStreamOK emits a minimal SSE chat-completion stream so the client's
// stream constructor succeeds.
func writeStreamOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[]}\n\n")
	fmt.Fprint(w, "data: [DONE]\n\n")
}

// A 502 while opening a stream is the exact shape of the 2026-08-18 incident:
// the reverse proxy in front of a model server returned "upstream unavailable"
// for ~40s while the backend restarted. The streaming path used to bypass
// retry entirely, so the coding agent saw a hard failure on attempt one and
// aborted the whole turn.
func TestCreateChatCompletionStream_RetriesUpstream502(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		if called > 2 {
			writeStreamOK(w)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, "upstream unavailable")
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	stream, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{})
	require.NoError(t, err)
	defer stream.Close()

	require.Equal(t, 3, called, "a 502 must be retried up to the attempt limit, not surfaced immediately")
}

// A 503 is the same class of failure as a 502 and must behave identically —
// pinned separately because the old code enumerated only 429 and 529, so the
// rest of the 5xx range depended on a fall-through that was easy to lose.
func TestCreateChatCompletionStream_RetriesUpstream503(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		if called > 1 {
			writeStreamOK(w)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	stream, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{})
	require.NoError(t, err)
	defer stream.Close()

	require.Equal(t, 2, called)
}

// A 401 will fail identically however many times it is sent, so retrying it
// only delays a certain failure. The streaming path must stop on the first
// attempt exactly as the non-streaming path does.
func TestCreateChatCompletionStream_DoesNotRetryAuthFailure(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	_, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)
	require.Equal(t, 1, called, "an auth failure must not be retried")
}

// A 400 means the request itself is wrong (an unsupported reasoning effort,
// for instance). Retrying multiplies the latency of a guaranteed failure.
func TestCreateChatCompletionStream_DoesNotRetryBadRequest(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	_, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)
	require.Equal(t, 1, called)
}

// The non-streaming path must keep retrying 5xx now that the classification
// moved into a shared helper.
func TestCreateChatCompletion_RetriesUpstream502(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		if called > 2 {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"model":"test-model"}`)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{})
	require.NoError(t, err)
	require.Equal(t, 3, called)
	require.Equal(t, "test-model", resp.Model)
}

// The status code is read from the typed go-openai error, not looked for in
// the rendered message. A provider that echoes the request back in a 502 body
// can easily put "400" in the text; substring matching would then classify a
// transient upstream failure as permanently unrecoverable and abort the turn.
func TestCreateChatCompletionStream_502WithMisleadingBodyStillRetries(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		if called > 1 {
			writeStreamOK(w)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		// Digits that a substring classifier would latch onto as a 4xx.
		fmt.Fprint(w, `{"error":{"message":"upstream unavailable while serving max_tokens 400 model gpt-404"}}`)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	stream, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{})
	require.NoError(t, err)
	defer stream.Close()

	require.Equal(t, 2, called, "the 502 status must win over 4xx-looking digits in the body")
}

// The mirror of the above: a real 400 whose message happens to contain 5xx
// digits must still fail immediately.
func TestCreateChatCompletionStream_400WithMisleadingBodyDoesNotRetry(t *testing.T) {
	called := 0

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"unsupported reasoning effort; context limit is 500000 tokens"}}`)
	}))
	defer ts.Close()

	client := New("test", ts.URL, true)

	_, err := client.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{})
	require.Error(t, err)
	require.Equal(t, 1, called, "a 400 must not be retried because its message contains 5xx-looking digits")
}
