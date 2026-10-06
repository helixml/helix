package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	openailogger "github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/toolcall"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type captureResponsesLogStore struct {
	call *types.LLMCall
}

func (s *captureResponsesLogStore) CreateLLMCall(_ context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	s.call = call
	return call, nil
}

func TestOpenAIResponsesProxyUsesTaskProviderAndReplacesHelixKey(t *testing.T) {
	var gotAuthorization, gotPath, gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"resp_test"}`))
	}))
	defer upstream.Close()

	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Providers.OpenAI.APIKey = "upstream-openai-key"
	cfg.Providers.OpenAI.BaseURL = upstream.URL + "/v1"
	server := &HelixAPIServer{Cfg: cfg, Store: mockStore}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID:             "ses_test",
		OrganizationID: "org_test",
		Metadata: types.SessionMetadata{
			SpecTaskID: "spt_test",
		},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").Return(&types.SpecTask{
		ID:                "spt_test",
		PlanningSessionID: "ses_test",
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeCodexCLI,
			CredentialType: types.CodeAgentCredentialTypeAPIKey,
			ProviderRef:    "openai",
			Model:          "gpt-5.6-sol",
		},
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol"}`))
	req.Header.Set("Authorization", "Bearer session-scoped-helix-key")
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID:             "user_test",
		SessionID:      "ses_test",
		SpecTaskID:     "spt_test",
		OrganizationID: "org_test",
	}))
	recorder := httptest.NewRecorder()

	server.openAIResponsesProxyHandler(recorder, req)

	require.Equal(t, http.StatusCreated, recorder.Code)
	require.JSONEq(t, `{"id":"resp_test"}`, recorder.Body.String())
	require.Equal(t, "Bearer upstream-openai-key", gotAuthorization)
	require.Equal(t, "/v1/responses", gotPath)
	require.JSONEq(t, `{"model":"gpt-5.6-sol"}`, gotBody)
}

func TestOpenAIResponsesProxyRejectsOtherHarnesses(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
		Metadata: types.SessionMetadata{
			CodeAgentConfig: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeOpenCode,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
				ProviderRef:    "openai",
				Model:          "gpt-5.6-sol",
			},
		},
	}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test", SessionID: "ses_test"}))
	recorder := httptest.NewRecorder()

	server.openAIResponsesProxyHandler(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "reserved for the Codex runtime")
}

func TestOpenAIResponsesProxyRejectsModelDifferentFromTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Providers.OpenAI.APIKey = "upstream-openai-key"
	cfg.Providers.OpenAI.BaseURL = "https://api.openai.test/v1"
	server := &HelixAPIServer{Cfg: cfg, Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
		Metadata: types.SessionMetadata{
			CodeAgentConfig: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeCodexCLI,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
				ProviderRef:    "openai",
				Model:          "gpt-5.6-sol",
			},
		},
	}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.6-terra"}`))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test", SessionID: "ses_test"}))
	recorder := httptest.NewRecorder()

	server.openAIResponsesProxyHandler(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `does not match task model "gpt-5.6-sol"`)
}

func TestOpenAIResponsesProxyStreamsAndLogsFinalUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"model\":\"gpt-5.6-sol\",\"usage\":{\"input_tokens\":120,\"output_tokens\":30,\"total_tokens\":150,\"input_tokens_details\":{\"cached_tokens\":80}}}}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer upstream.Close()

	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Providers.OpenAI.APIKey = "upstream-openai-key"
	cfg.Providers.OpenAI.BaseURL = upstream.URL + "/v1"
	cfg.Providers.BillingEnabled = true
	capture := &captureResponsesLogStore{}
	billingCapture := &captureResponsesLogStore{}
	modelInfoProvider := model.NewMockModelInfoProvider(ctrl)
	modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(&types.ModelInfo{
		Pricing: types.Pricing{
			Prompt:         "1",
			Completion:     "2",
			InputCacheRead: "0.5",
		},
	}, nil).Times(2)
	server := &HelixAPIServer{
		Cfg: cfg, Store: mockStore,
		modelInfoProvider:            modelInfoProvider,
		openAIResponsesLogStores:     []openailogger.LogStore{capture},
		openAIResponsesBillingLogger: billingCapture,
	}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
		Metadata: types.SessionMetadata{
			CodeAgentConfig: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeCodexCLI,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
				ProviderRef:    "openai",
				Model:          "gpt-5.6-sol",
			},
		},
	}, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol","stream":true}`))
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID: "user_test", SessionID: "ses_test", ProjectID: "prj_test", SpecTaskID: "spt_test",
	}))
	recorder := httptest.NewRecorder()

	server.openAIResponsesProxyHandler(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "response.completed")
	require.NotNil(t, capture.call)
	require.Equal(t, int64(120), capture.call.PromptTokens)
	require.Equal(t, int64(30), capture.call.CompletionTokens)
	require.Equal(t, int64(80), capture.call.CacheReadTokens)
	require.Equal(t, int64(150), capture.call.TotalTokens)
	require.Equal(t, "gpt-5.6-sol", capture.call.Model)
	require.Equal(t, types.CodeAgentRuntimeCodexCLI, capture.call.CodeAgentRuntime)
	require.Equal(t, "prj_test", capture.call.ProjectID)
	require.Equal(t, "spt_test", capture.call.SpecTaskID)
	require.Equal(t, float64(140), capture.call.TotalCost)
	require.True(t, capture.call.Stream)
	require.Empty(t, capture.call.Error)
	require.Same(t, capture.call, billingCapture.call)
}

func TestAnthropicProxyUsesTaskOwnedProviderSelection(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Providers.Anthropic.APIKey = "anthropic-key"
	cfg.Providers.Anthropic.BaseURL = "https://api.anthropic.com"
	server := &HelixAPIServer{Cfg: cfg, Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
		Metadata: types.SessionMetadata{
			SpecTaskID: "spt_test",
		},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").Return(&types.SpecTask{
		ID:                "spt_test",
		PlanningSessionID: "ses_test",
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeClaudeCode,
			CredentialType: types.CodeAgentCredentialTypeAPIKey,
			ProviderRef:    "anthropic",
			Model:          "claude-opus-5",
		},
	}, nil)

	endpoint, err := server.getProviderEndpoint(t.Context(), &types.User{
		ID:         "user_test",
		SessionID:  "ses_test",
		SpecTaskID: "spt_test",
	})

	require.NoError(t, err)
	require.Equal(t, "anthropic", endpoint.Name)
	require.Equal(t, "anthropic-key", endpoint.APIKey)
}

func TestAnthropicProxyRejectsNonAnthropicProviderForClaudeCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Providers.OpenAI.APIKey = "openai-key"
	cfg.Providers.OpenAI.BaseURL = "https://api.openai.com/v1"
	server := &HelixAPIServer{Cfg: cfg, Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
		Metadata: types.SessionMetadata{
			CodeAgentConfig: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeClaudeCode,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
				ProviderRef:    "openai",
				Model:          "claude-opus-5",
			},
		},
	}, nil)

	_, err := server.getProviderEndpoint(t.Context(), &types.User{
		ID:        "user_test",
		SessionID: "ses_test",
	})

	require.ErrorContains(t, err, "requires the anthropic provider")
}

func TestAnthropicProxySessionWithoutCodeAgentContextUsesLegacyProviderLookup(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Providers.Anthropic.APIKey = "anthropic-key"
	cfg.Providers.Anthropic.BaseURL = "https://api.anthropic.com"
	server := &HelixAPIServer{Cfg: cfg, Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
	}, nil)
	mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		Owner: "",
		Name:  "anthropic",
	}).Return(nil, store.ErrNotFound)

	endpoint, err := server.getProviderEndpoint(t.Context(), &types.User{
		ID:        "user_test",
		SessionID: "ses_test",
	})

	require.NoError(t, err)
	require.Equal(t, "anthropic", endpoint.Name)
	require.Equal(t, "anthropic-key", endpoint.APIKey)
}

func TestAnthropicProxyConfiguredTaskDoesNotFallBackWithoutProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
		Metadata: types.SessionMetadata{
			CodeAgentConfig: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeClaudeCode,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
		},
	}, nil)

	_, err := server.getProviderEndpoint(t.Context(), &types.User{
		ID:        "user_test",
		SessionID: "ses_test",
	})

	require.ErrorContains(t, err, "code-agent task has no API provider selected")
}

func TestJoinProxyPath(t *testing.T) {
	require.Equal(t, "/v1/responses", joinProxyPath("/v1/", "/responses"))
}

func TestNewOpenAIResponsesTransport(t *testing.T) {
	require.Same(t, http.DefaultTransport, newOpenAIResponsesTransport(false))

	transport, ok := newOpenAIResponsesTransport(true).(*http.Transport)
	require.True(t, ok)
	require.NotSame(t, http.DefaultTransport, transport)
	require.NotNil(t, transport.TLSClientConfig)
	require.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestValidateResponsesToolCalls(t *testing.T) {
	request := []byte(`{
		"model": "gpt-5-codex",
		"tools": [
			{"type": "function", "name": "shell", "parameters": {"type": "object", "properties": {"command": {"type": "array", "items": {"type": "string"}}}, "required": ["command"]}},
			{"type": "web_search"}
		]
	}`)

	t.Run("streaming terminal event", func(t *testing.T) {
		response := []byte(`{"type":"response.completed","response":{"status":"completed","output":[{"type":"function_call","name":"shell","arguments":"{\"command\":[\"ls\"]}"}]}}`)
		verdict := validateResponsesToolCalls(request, response)
		require.Equal(t, 1, verdict.Result.ToolsOffered)
		require.Equal(t, 1, verdict.Result.Calls)
		require.Equal(t, 0, verdict.Result.Errors)
		require.Equal(t, "tool_calls", verdict.FinishReason)
	})

	t.Run("non-streaming envelope with bad arguments", func(t *testing.T) {
		response := []byte(`{"status":"completed","output":[{"type":"function_call","name":"shell","arguments":"{\"command\":\"ls\"}"}]}`)
		verdict := validateResponsesToolCalls(request, response)
		require.Equal(t, 1, verdict.Result.Errors)
		require.Equal(t, toolcall.KindSchemaMismatch, verdict.Result.KindsString())
	})

	t.Run("unparseable arguments", func(t *testing.T) {
		response := []byte(`{"status":"completed","output":[{"type":"function_call","name":"shell","arguments":"{\"command\":"}]}`)
		verdict := validateResponsesToolCalls(request, response)
		require.Equal(t, 1, verdict.Result.Errors)
		require.Equal(t, toolcall.KindInvalidJSON, verdict.Result.KindsString())
	})

	t.Run("message output keeps the response status", func(t *testing.T) {
		response := []byte(`{"status":"completed","output":[{"type":"message"}]}`)
		verdict := validateResponsesToolCalls(request, response)
		require.Equal(t, 0, verdict.Result.Calls)
		require.Equal(t, "completed", verdict.FinishReason)
	})
}
