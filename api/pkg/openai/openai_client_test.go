package openai

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// TestTLSSkipVerify_SelfSignedCert tests that TLSSkipVerify actually works
// with a self-signed certificate. This directly tests the code path that
// was broken in 2.5.25 for enterprise customers.
func TestTLSSkipVerify_SelfSignedCert(t *testing.T) {
	// httptest.NewTLSServer creates a server with a self-signed certificate
	// that will fail normal TLS verification
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("TLS server received request: %s %s", r.Method, r.URL.Path)

		// Return a valid models response
		response := struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "test-model"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Logf("failed encoding response: %v", err)
		}
	})

	ts := httptest.NewTLSServer(handler)
	defer ts.Close()

	t.Logf("TLS test server running at: %s", ts.URL)

	t.Run("WithoutTLSSkipVerify_ShouldFail", func(t *testing.T) {
		// Create client WITHOUT TLSSkipVerify - should fail with x509 error
		client := New("test-api-key", ts.URL, false)

		models, err := client.ListModels(context.Background())

		// This MUST fail with a certificate error
		require.Error(t, err, "Expected TLS error with self-signed cert when TLSSkipVerify=false")
		assert.Contains(t, err.Error(), "x509", "Error should be x509 certificate error")
		assert.Nil(t, models)

		t.Logf("Got expected error without TLSSkipVerify: %v", err)
	})

	t.Run("WithTLSSkipVerify_ShouldSucceed", func(t *testing.T) {
		// Create client WITH TLSSkipVerify - should succeed
		client := NewWithOptions("test-api-key", ts.URL, false, ClientOptions{
			TLSSkipVerify: true,
		})

		models, err := client.ListModels(context.Background())

		// This MUST succeed
		require.NoError(t, err, "Should succeed with TLSSkipVerify=true")
		require.NotNil(t, models)
		require.Len(t, models, 1)
		assert.Equal(t, "test-model", models[0].ID)

		t.Logf("Success with TLSSkipVerify: got %d models", len(models))
	})
}

// TestOld2525Behavior_WithActualOpenAIClient tests the ACTUAL OpenAI client
// code path to see if there's a bug in how TLSSkipVerify is applied.
// This simulates the exact 2.5.25 code to find the real issue.
func TestOld2525Behavior_WithActualOpenAIClient(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("TLS server received request: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "test-model"}},
		})
	})

	ts := httptest.NewTLSServer(handler)
	defer ts.Close()

	t.Run("ActualOpenAIClient_ListModels_WithTLSSkipVerify", func(t *testing.T) {
		// Use our actual OpenAI client with TLSSkipVerify
		client := NewWithOptions("test-api-key", ts.URL, false, ClientOptions{
			TLSSkipVerify: true,
		})

		models, err := client.ListModels(context.Background())

		require.NoError(t, err, "Actual OpenAI client ListModels should work with TLSSkipVerify")
		require.NotNil(t, models)
		t.Logf("ActualOpenAIClient ListModels worked: %d models", len(models))
	})

	t.Run("ActualOpenAIClient_ChatCompletion_WithTLSSkipVerify", func(t *testing.T) {
		// Handler for chat completions
		chatHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Logf("Chat TLS server received request: %s %s", r.Method, r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(&openai.ChatCompletionResponse{
				Model: "test-model",
				Choices: []openai.ChatCompletionChoice{
					{Message: openai.ChatCompletionMessage{Content: "Hello!"}},
				},
			})
		})

		chatTS := httptest.NewTLSServer(chatHandler)
		defer chatTS.Close()

		client := NewWithOptions("test-api-key", chatTS.URL, false, ClientOptions{
			TLSSkipVerify: true,
		})

		resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
			Model:    "test-model",
			Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "Hi"}},
		})

		require.NoError(t, err, "Actual OpenAI client ChatCompletion should work with TLSSkipVerify")
		assert.Equal(t, "test-model", resp.Model)
		t.Logf("ActualOpenAIClient ChatCompletion worked: model=%s", resp.Model)
	})
}

// TestOld2525CodeSimulation_ExactCode simulates the EXACT 2.5.25 code path
// to identify what was actually broken
func TestOld2525CodeSimulation_ExactCode(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("TLS server received request: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(&openai.ChatCompletionResponse{
			Model: "test-model",
			Choices: []openai.ChatCompletionChoice{
				{Message: openai.ChatCompletionMessage{Content: "Hello!"}},
			},
		})
	})

	ts := httptest.NewTLSServer(handler)
	defer ts.Close()

	t.Run("Simulate_2525_Exact_Code", func(t *testing.T) {
		// This is the EXACT 2.5.25 code from NewWithOptions
		// I'm copying it here to simulate exactly what 2.5.25 did
		tlsSkipVerify := true

		httpClient := &http.Client{
			Timeout: 5 * time.Minute,
		}

		if tlsSkipVerify {
			// 2.5.25 code: created minimal transport
			httpClient.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			}
		}

		// Now create the interceptor like 2.5.25 did
		rateLimiter := NewUniversalRateLimiter(ts.URL)

		interceptor := &openAIClientInterceptor{
			Client:      *httpClient, // COPY by value - this is what 2.5.25 did
			rateLimiter: rateLimiter,
			baseURL:     ts.URL,
		}

		// Check what Transport the interceptor actually has
		t.Logf("httpClient.Transport type: %T", httpClient.Transport)
		t.Logf("interceptor.Client.Transport type: %T", interceptor.Client.Transport)

		if interceptor.Client.Transport != nil {
			if tr, ok := interceptor.Client.Transport.(*http.Transport); ok {
				t.Logf("interceptor TLSClientConfig: %+v", tr.TLSClientConfig)
				if tr.TLSClientConfig != nil {
					t.Logf("interceptor InsecureSkipVerify: %v", tr.TLSClientConfig.InsecureSkipVerify)
				}
			}
		}

		// Make a request using the interceptor
		req, _ := http.NewRequestWithContext(context.Background(), "POST", ts.URL+"/chat/completions", nil)
		resp, err := interceptor.Do(req)

		if err != nil {
			t.Logf("Error: %v", err)
			require.NoError(t, err, "Interceptor should work with TLSSkipVerify")
		} else {
			defer resp.Body.Close()
			t.Logf("Interceptor request succeeded with status: %d", resp.StatusCode)
		}
	})
}

// TestTLSSkipVerify_ConfigNotPassed tests what happens if TLSSkipVerify
// is NOT passed correctly - simulating a potential config propagation bug
func TestTLSSkipVerify_ConfigNotPassed(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("TLS server received request: %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "test-model"}},
		})
	})

	ts := httptest.NewTLSServer(handler)
	defer ts.Close()

	// This simulates what happens if the config is NOT correctly propagated
	// to the OpenAI client - maybe due to a config loading issue
	t.Run("ConfigNotPassed_ShouldFail", func(t *testing.T) {
		// Create client with TLSSkipVerify=false (simulating config not being applied)
		client := NewWithOptions("test-api-key", ts.URL, false, ClientOptions{
			TLSSkipVerify: false, // Config NOT applied!
		})

		models, err := client.ListModels(context.Background())

		// This SHOULD fail with x509 error because TLSSkipVerify is false
		require.Error(t, err, "Should fail when TLSSkipVerify is false")
		assert.Contains(t, err.Error(), "x509", "Should be a certificate error")
		assert.Nil(t, models)
		t.Logf("Correctly failed with: %v", err)
	})

	t.Run("ConfigPassed_ShouldSucceed", func(t *testing.T) {
		// Create client with TLSSkipVerify=true (config correctly applied)
		client := NewWithOptions("test-api-key", ts.URL, false, ClientOptions{
			TLSSkipVerify: true, // Config applied!
		})

		models, err := client.ListModels(context.Background())

		// This SHOULD succeed
		require.NoError(t, err, "Should succeed when TLSSkipVerify is true")
		require.NotNil(t, models)
		t.Logf("Correctly succeeded with %d models", len(models))
	})
}

// TestTrailingSlashStripped tests that trailing slashes are stripped from baseURL
// to prevent double slashes when concatenating paths like "/models"
func TestTrailingSlashStripped(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Server received request: %s %s", r.Method, r.URL.Path)

		// Verify we don't get double slashes in the path
		assert.NotContains(t, r.URL.Path, "//", "Path should not contain double slashes")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "test-model"}},
		})
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	t.Run("WithTrailingSlash", func(t *testing.T) {
		// Create client with trailing slash in URL
		client := New("test-api-key", ts.URL+"/", false)

		// Verify the baseURL was stripped of trailing slash
		assert.Equal(t, ts.URL, client.BaseURL(), "BaseURL should have trailing slash stripped")

		// Make a request that concatenates paths
		models, err := client.ListModels(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, models)
	})

	t.Run("WithoutTrailingSlash", func(t *testing.T) {
		// Create client without trailing slash
		client := New("test-api-key", ts.URL, false)

		// Verify the baseURL stayed the same
		assert.Equal(t, ts.URL, client.BaseURL())

		// Make a request that concatenates paths
		models, err := client.ListModels(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, models)
	})

	t.Run("WithV1TrailingSlash", func(t *testing.T) {
		// Simulate a URL like https://api.example.com/v1/
		// Server handler should still work
		client := New("test-api-key", ts.URL+"/v1/", false)

		// Verify the trailing slash was stripped
		assert.Equal(t, ts.URL+"/v1", client.BaseURL(), "BaseURL should have trailing slash stripped")
	})
}

// TestTLSSkipVerify_ChatCompletion tests TLS skip verify for chat completions
func TestTLSSkipVerify_ChatCompletion(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("TLS server received request: %s %s", r.Method, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(&openai.ChatCompletionResponse{
			Model: "test-model",
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "Hello!",
					},
				},
			},
		}); err != nil {
			t.Logf("failed encoding response: %v", err)
		}
	})

	ts := httptest.NewTLSServer(handler)
	defer ts.Close()

	t.Run("WithoutTLSSkipVerify_ShouldFail", func(t *testing.T) {
		client := New("test-api-key", ts.URL, false)

		_, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
			Model: "test-model",
			Messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
		})

		require.Error(t, err, "Expected TLS error with self-signed cert")
		assert.Contains(t, err.Error(), "x509", "Error should be x509 certificate error")
	})

	t.Run("WithTLSSkipVerify_ShouldSucceed", func(t *testing.T) {
		client := NewWithOptions("test-api-key", ts.URL, false, ClientOptions{
			TLSSkipVerify: true,
		})

		resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
			Model: "test-model",
			Messages: []openai.ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
		})

		require.NoError(t, err, "Should succeed with TLSSkipVerify=true")
		assert.Equal(t, "test-model", resp.Model)
	})
}
