package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Planted in upstream error messages; none of it may reach the caller.
// Built at runtime so secret scanners do not flag the fixture itself.
var (
	plantedKey  = "sk-" + "proj-" + strings.Repeat("PLANTEDk3y0", 3)
	plantedURL  = "http://10.20.30.40:8000/v1/chat/completions?api_key=" + strings.Repeat("hunter2", 2)
	plantedHost = "bunker-qwen.svc.cluster.local"
)

var plantedLeaks = []string{plantedKey, "PLANTED", "10.20.30.40", "hunter2", plantedHost, "bunker-qwen", "127.0.0.1", "[::1]"}

func plantedMessage(base string) string {
	return fmt.Sprintf("%s (upstream %s via %s:8000, key=%s, Authorization: Bearer %s)", base, plantedURL, plantedHost, plantedKey, plantedKey)
}

func writeUpstreamError(w http.ResponseWriter, status int, typ, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
		"message": message, "type": typ, "code": code, "param": "messages",
	}})
}

// newChatServerWithUpstream wires the real handler, controller, provider
// client and logging middleware to a fake provider at baseURL.
// providerDeadlineClient bounds each provider call with its own deadline, as
// a provider-side timeout does, while the caller's request stays connected.
type providerDeadlineClient struct {
	oai.Client
	timeout time.Duration
}

func (c providerDeadlineClient) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.Client.CreateChatCompletion(ctx, req)
}

func newChatServerWithUpstream(t *testing.T, baseURL string) *HelixAPIServer {
	return newChatServerWithProvider(t, oai.New("provider-key", baseURL, false))
}

func newChatServerWithProvider(t *testing.T, provider oai.Client) *HelixAPIServer {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	mgr := manager.NewMockProviderManager(ctrl)

	modelInfo, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)
	cfg := &config.ServerConfig{}
	cfg.WebServer.ModelsCacheTTL = time.Minute
	client := logger.Wrap(cfg, types.ProviderOpenAI, provider, modelInfo, nil)

	mgr.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(client, nil).AnyTimes()
	mgr.EXPECT().ListProviders(gomock.Any(), gomock.Any()).Return([]types.Provider{}, nil).AnyTimes()
	st.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{}, nil).AnyTimes()
	st.EXPECT().GetUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{}, nil).AnyTimes()

	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{NumCounters: 1e4, MaxCost: 1 << 20, BufferItems: 64})
	require.NoError(t, err)

	ctl, err := controller.NewController(context.Background(), controller.Options{
		Config:          cfg,
		Store:           st,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: mgr,
		Filestore:       filestore.NewMockFileStore(ctrl),
		Extractor:       extract.NewMockExtractor(ctrl),
	})
	require.NoError(t, err)

	return &HelixAPIServer{Cfg: cfg, Store: st, providerManager: mgr, cache: cache, Controller: ctl}
}

func closedPortURL(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "http://" + addr + "/v1"
}

// Every upstream failure is answered with an OpenAI-shaped JSON error whose
// status says whose fault it is, and with no provider URL, host or key.
func TestCreateChatCompletion_UpstreamFailures(t *testing.T) {
	cases := []struct {
		name   string
		stream bool
		// providerTimeout bounds the provider call; the caller stays connected.
		providerTimeout time.Duration
		upstream        http.HandlerFunc // nil: nothing listens at the provider URL
		// expectations
		status     int
		errType    string
		code       any
		retryAfter string
		rateReset  string
		message    string // substring the message must keep
	}{
		{
			name: "400 context length exceeded",
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				writeUpstreamError(w, http.StatusBadRequest, "invalid_request_error", "context_length_exceeded", plantedMessage("This model's maximum context length is 8192 tokens."))
			},
			status: http.StatusBadRequest, errType: "invalid_request_error", code: "context_length_exceeded",
			message: "maximum context length is 8192 tokens",
		},
		{
			name: "404 model not found",
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				writeUpstreamError(w, http.StatusNotFound, "invalid_request_error", "model_not_found", plantedMessage("The model `test-model` does not exist."))
			},
			status: http.StatusNotFound, errType: "invalid_request_error", code: "model_not_found",
			message: "does not exist",
		},
		{
			name: "404 non-JSON body",
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, plantedMessage("404 page not found"), http.StatusNotFound)
			},
			status: http.StatusNotFound, errType: "invalid_request_error",
		},
		{
			name: "429 with Retry-After",
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "1")
				w.Header().Set("X-Ratelimit-Reset-Requests", "1s")
				writeUpstreamError(w, http.StatusTooManyRequests, "requests", "rate_limit_exceeded", plantedMessage("Rate limit reached for requests."))
			},
			status: http.StatusTooManyRequests, errType: "requests", code: "rate_limit_exceeded",
			retryAfter: "1", rateReset: "1s", message: "Rate limit reached",
		},
		{
			name:   "429 with Retry-After, streaming",
			stream: true,
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "1")
				writeUpstreamError(w, http.StatusTooManyRequests, "requests", "rate_limit_exceeded", plantedMessage("Rate limit reached for requests."))
			},
			status: http.StatusTooManyRequests, errType: "requests", code: "rate_limit_exceeded", retryAfter: "1",
		},
		{
			name: "401 from upstream is our credential failure",
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				writeUpstreamError(w, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", plantedMessage("Incorrect API key provided."))
			},
			status: http.StatusBadGateway, errType: "upstream_error",
		},
		{
			name: "403 from upstream is our credential failure",
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, plantedMessage("forbidden"), http.StatusForbidden)
			},
			status: http.StatusBadGateway, errType: "upstream_error",
		},
		{
			name: "500 from upstream",
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				writeUpstreamError(w, http.StatusInternalServerError, "server_error", "internal_error", plantedMessage("The server had an error."))
			},
			status: http.StatusBadGateway, errType: "upstream_error",
		},
		{
			name:   "500 from upstream, streaming",
			stream: true,
			upstream: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, plantedMessage("boom"), http.StatusInternalServerError)
			},
			status: http.StatusBadGateway, errType: "upstream_error",
		},
		{
			name:   "connection refused",
			status: http.StatusBadGateway, errType: "upstream_error",
		},
		{
			name:            "provider timeout with the caller connected",
			providerTimeout: 300 * time.Millisecond,
			upstream: func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
				case <-time.After(2 * time.Second):
				}
			},
			status: http.StatusGatewayTimeout, errType: "upstream_error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			baseURL := closedPortURL(t)
			if tc.upstream != nil {
				upstream := httptest.NewServer(tc.upstream)
				t.Cleanup(upstream.Close)
				baseURL = upstream.URL + "/v1"
			}
			var provider oai.Client = oai.New("provider-key", baseURL, false)
			if tc.providerTimeout > 0 {
				provider = providerDeadlineClient{Client: provider, timeout: tc.providerTimeout}
			}
			srv := newChatServerWithProvider(t, provider)

			body := fmt.Sprintf(`{"model":"openai/test-model","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, tc.stream)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test", OrganizationID: "org_test"}))
			rw := httptest.NewRecorder()

			srv.createChatCompletion(rw, req)

			require.Equal(t, tc.status, rw.Code, rw.Body.String())
			require.Equal(t, "application/json", rw.Header().Get("Content-Type"))
			errObj := decodeOpenAIError(t, rw.Body.Bytes())
			require.Equal(t, tc.errType, errObj["type"])
			require.Equal(t, tc.code, errObj["code"])
			if tc.message != "" {
				require.Contains(t, errObj["message"], tc.message)
			}
			require.Equal(t, tc.retryAfter, rw.Header().Get("Retry-After"))
			require.Equal(t, tc.rateReset, rw.Header().Get("X-Ratelimit-Reset-Requests"))
			for _, leak := range plantedLeaks {
				require.NotContains(t, rw.Body.String(), leak)
			}
		})
	}
}

// A failure after the stream has started cannot change the 200; it ends the
// stream with an OpenAI-style error event instead of a silent clean finish.
func TestCreateChatCompletion_MidStreamFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`+"\n\n")
		errEvent, _ := json.Marshal(map[string]any{"error": map[string]any{"message": plantedMessage("The server had an error while processing your request."), "type": "server_error", "code": "internal_error"}})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", errEvent)
	}))
	t.Cleanup(upstream.Close)
	srv := newChatServerWithUpstream(t, upstream.URL+"/v1")

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/test-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test", OrganizationID: "org_test"}))
	rw := httptest.NewRecorder()

	srv.createChatCompletion(rw, req)

	require.Equal(t, http.StatusOK, rw.Code)
	require.Equal(t, "text/event-stream", rw.Header().Get("Content-Type"))
	var events []string
	scanner := bufio.NewScanner(strings.NewReader(rw.Body.String()))
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			events = append(events, data)
		}
	}
	require.Len(t, events, 2, rw.Body.String())
	require.Contains(t, events[0], `"Hel"`)
	errObj := decodeOpenAIError(t, []byte(events[1]))
	require.Equal(t, "server_error", errObj["type"])
	require.Equal(t, "internal_error", errObj["code"])
	require.Contains(t, errObj["message"], "The server had an error")
	for _, leak := range plantedLeaks {
		require.NotContains(t, rw.Body.String(), leak)
	}
}

// cancelOnFirstChunk cancels the request (the caller disconnecting) as soon
// as the first SSE chunk has been written to it.
type cancelOnFirstChunk struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (w *cancelOnFirstChunk) Write(b []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(b)
	w.cancel()
	return n, err
}

// A caller that goes away is not answered and not reported as a provider
// failure: nothing is written after the disconnect, before or mid-stream.
func TestCreateChatCompletion_ClientDisconnect(t *testing.T) {
	blockUntilGone := func(w http.ResponseWriter, r *http.Request, firstChunk bool) {
		if firstChunk {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, `data: {"id":"c1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hel"}}]}`+"\n\n")
			w.(http.Flusher).Flush()
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}

	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("before the first chunk, stream=%t", stream), func(t *testing.T) {
			t.Parallel()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { blockUntilGone(w, r, false) }))
			t.Cleanup(upstream.Close)
			srv := newChatServerWithUpstream(t, upstream.URL+"/v1")

			ctx, cancel := context.WithCancel(context.Background())
			time.AfterFunc(200*time.Millisecond, cancel)
			body := fmt.Sprintf(`{"model":"openai/test-model","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)).WithContext(ctx)
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test", OrganizationID: "org_test"}))
			rw := httptest.NewRecorder()

			start := time.Now()
			srv.createChatCompletion(rw, req)
			require.Less(t, time.Since(start), 2*time.Second, "the provider call must end with the caller's request")

			require.Empty(t, rw.Body.String())
			require.Empty(t, rw.Header().Get("Content-Type"))
		})
	}

	t.Run("mid-stream", func(t *testing.T) {
		t.Parallel()
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { blockUntilGone(w, r, true) }))
		t.Cleanup(upstream.Close)
		srv := newChatServerWithUpstream(t, upstream.URL+"/v1")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/test-model","stream":true,"messages":[{"role":"user","content":"hi"}]}`)).WithContext(ctx)
		req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test", OrganizationID: "org_test"}))
		rw := &cancelOnFirstChunk{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}

		srv.createChatCompletion(rw, req)

		require.Equal(t, 1, strings.Count(rw.Body.String(), "data: "), rw.Body.String())
		require.Contains(t, rw.Body.String(), `"Hel"`)
		require.NotContains(t, rw.Body.String(), `"error"`)
	})
}

// decodeOpenAIError asserts body is {"error":{"message","type","code","param"}}
// and returns the error object.
func decodeOpenAIError(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var envelope map[string]map[string]any
	require.NoError(t, json.Unmarshal(body, &envelope), string(body))
	errObj, ok := envelope["error"]
	require.True(t, ok, string(body))
	for _, key := range []string{"message", "type", "code", "param"} {
		require.Contains(t, errObj, key, string(body))
	}
	require.NotEmpty(t, errObj["message"])
	return errObj
}

func TestSanitizeProviderMessage(t *testing.T) {
	got := sanitizeProviderMessage(plantedMessage("Invalid value for messages.0.content: expected a string."))
	require.Contains(t, got, "Invalid value for messages.0.content: expected a string.")
	for _, leak := range plantedLeaks {
		require.NotContains(t, got, leak)
	}
	require.LessOrEqual(t, len(sanitizeProviderMessage(strings.Repeat("a ", 2000))), maxProviderMessageBytes+len("…"))
	require.Equal(t, "upstream [host] failed for Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8-0731",
		sanitizeProviderMessage("upstream node06 failed for Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8-0731", "", "Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8-0731"))
	require.Equal(t, "This model's maximum context length is 131072 tokens; you requested 140000 tokens (qwen3.8-27b, gpt-4.1).",
		sanitizeProviderMessage("This model's maximum context length is 131072 tokens; you requested 140000 tokens (qwen3.8-27b, gpt-4.1)."))
}
