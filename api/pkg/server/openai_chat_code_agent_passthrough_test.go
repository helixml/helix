package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// harnessSystemPrompt is the shape of prompt a coding harness actually sends
// (this one is modelled on a captured production request). The bug this file
// guards against replaced exactly this with "You are a helpful assistant." on
// every /v1/chat/completions call a harness made.
const harnessSystemPrompt = "You are an AI coding agent running inside a Helix sandbox.\n\nYour working directory is /workspace. Never guess at file contents; read them."

// fakeOpenAI is a stand-in for the upstream provider. It records the verbatim
// request body it was handed, which is the only trustworthy answer to "did
// Helix forward what the harness sent?".
type fakeOpenAI struct {
	server *httptest.Server

	mu     sync.Mutex
	bodies []string
	paths  []string
}

func newFakeOpenAI(t *testing.T) *fakeOpenAI {
	t.Helper()
	f := &fakeOpenAI{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		f.mu.Lock()
		f.bodies = append(f.bodies, string(body))
		f.paths = append(f.paths, r.URL.Path)
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "chatcmpl-fake",
			"object": "chat.completion",
			"model": "qwen3.8-27b",
			"choices": [{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage": {"prompt_tokens": 11, "completion_tokens": 2, "total_tokens": 13}
		}`))
	}))
	t.Cleanup(f.server.Close)
	return f
}

// received returns the single body the upstream saw, failing if the count is not
// exactly one — a silent retry or a tool-loop second call would otherwise hide.
func (f *fakeOpenAI) received(t *testing.T) map[string]any {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.bodies, 1, "expected exactly one upstream call, paths=%v", f.paths)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.bodies[0]), &got))
	return got
}

// captureLLMCallStore records the LLMCall the logging middleware writes, so a
// test can assert on the same original_request/request pair that exposed this
// bug in production.
type captureLLMCallStore struct {
	mu   sync.Mutex
	call *types.LLMCall
}

func (s *captureLLMCallStore) CreateLLMCall(_ context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.call = call
	return call, nil
}

func (s *captureLLMCallStore) get() *types.LLMCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.call
}

// codeAgentTestServer wires a HelixAPIServer whose only provider is the fake
// upstream, registered as the global "openai" provider. A harness addresses it
// the way every harness does in production: a provider-prefixed model id
// ("openai/qwen3.8-27b"), which the handler resolves and strips before
// forwarding.
func codeAgentTestServer(t *testing.T, upstream *fakeOpenAI) (*HelixAPIServer, *store.MockStore, *captureLLMCallStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	cfg := &config.ServerConfig{}
	cfg.Providers.OpenAI.APIKey = "upstream-openai-key"
	cfg.Providers.OpenAI.BaseURL = upstream.server.URL + "/v1"

	// Pricing is irrelevant here; return an error so the logger takes its
	// "no pricing" warning branch instead of nil-panicking.
	modelInfoProvider := model.NewMockModelInfoProvider(ctrl)
	modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("no pricing in test")).AnyTimes()

	// Model routing scans user-defined endpoints before falling back to prefix
	// parsing. There are none here; "openai" resolves as a global provider.
	mockStore.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	capture := &captureLLMCallStore{}
	providerManager := manager.NewProviderManager(cfg, mockStore, nil, modelInfoProvider, capture)

	helixController := controller.NewInferenceTestController(controller.Options{
		Config:          cfg,
		Store:           mockStore,
		ProviderManager: providerManager,
	})

	return &HelixAPIServer{
		Cfg:             cfg,
		Store:           mockStore,
		Controller:      helixController,
		providerManager: providerManager,
	}, mockStore, capture
}

// harnessRequestBody is a realistic harness payload: its own system prompt, its
// own sampling parameters and its own tool definitions.
const harnessRequestBody = `{
	"model": "openai/qwen3.8-27b",
	"messages": [
		{"role": "system", "content": "` + `HARNESS_PROMPT` + `"},
		{"role": "user", "content": "add a test for the reaper"}
	],
	"temperature": 0.15,
	"top_p": 0.85,
	"max_tokens": 4096,
	"frequency_penalty": 0.2,
	"presence_penalty": 0.3,
	"tools": [
		{"type": "function", "function": {"name": "read_file", "description": "Read a file", "parameters": {"type": "object", "properties": {"path": {"type": "string"}}}}}
	]
}`

func harnessRequest() string {
	prompt, err := json.Marshal(harnessSystemPrompt)
	if err != nil {
		panic(err)
	}
	// Splice the JSON-escaped prompt in, so the literal above stays readable.
	return strings.Replace(harnessRequestBody, `"HARNESS_PROMPT"`, string(prompt), 1)
}

// specTaskWithCodeAgentConfig is the shape produced by the code-harness-config
// refactor: the task owns its runtime/provider/model and has no HelixAppID.
func specTaskWithCodeAgentConfig(runtime types.CodeAgentRuntime) *types.SpecTask {
	return &types.SpecTask{
		ID:                "spt_test",
		ProjectID:         "prj_test",
		PlanningSessionID: "ses_test",
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        runtime,
			CredentialType: types.CodeAgentCredentialTypeAPIKey,
			ProviderRef:    "openai",
			Model:          "qwen3.8-27b",
		},
	}
}

func codeAgentChatRequest(t *testing.T, user types.User) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(harnessRequest()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer session-scoped-helix-key")
	return req.WithContext(setRequestUser(req.Context(), user))
}

// TestCodeAgentChatCompletionForwardsHarnessRequestUnmodified is the regression
// test for the opencode / zed-agent / codex prompt clobber: with no App in the
// picture, loadAssistant used to synthesise an assistant seeded with
// DefaultChatSystemPrompt, and setSystemPrompt then overwrote the harness's own
// system message.
//
// GetUserMeta is stubbed here (rather than left unexpected) so that without the
// fix the request still reaches the fake provider and the assertions below show
// the actual corruption. The companion test asserts the store is never read at
// all.
func TestCodeAgentChatCompletionForwardsHarnessRequestUnmodified(t *testing.T) {
	upstream := newFakeOpenAI(t)
	server, mockStore, capture := codeAgentTestServer(t, upstream)

	// Deliberately distinctive: if any of this leaks into harness traffic the
	// assertions below name exactly what leaked.
	chatTemperature := float32(0.9)
	mockStore.EXPECT().GetUserMeta(gomock.Any(), "user_test").Return(&types.UserMeta{
		ID: "user_test",
		ChatSettings: types.UserChatSettings{
			SystemPrompt: "Answer in haiku.",
			Temperature:  &chatTemperature,
		},
	}, nil).AnyTimes()

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID:       "ses_test",
		Metadata: types.SessionMetadata{SpecTaskID: "spt_test"},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").
		Return(specTaskWithCodeAgentConfig(types.CodeAgentRuntimeOpenCode), nil)

	recorder := httptest.NewRecorder()
	server.createChatCompletion(recorder, codeAgentChatRequest(t, types.User{
		ID:         "user_test",
		SessionID:  "ses_test",
		SpecTaskID: "spt_test",
		ProjectID:  "prj_test",
	}))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	got := upstream.received(t)

	// The harness's system prompt survives verbatim. This is the assertion that
	// fails without the fix.
	messages, ok := got["messages"].([]any)
	require.True(t, ok, "messages missing from upstream body: %v", got)
	require.Len(t, messages, 2, "no message was injected or dropped")
	first, ok := messages[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "system", first["role"])
	require.Equal(t, harnessSystemPrompt, first["content"])
	require.NotEqual(t, types.DefaultChatSystemPrompt, first["content"])
	require.NotEqual(t, "Answer in haiku.", first["content"],
		"the user's personal chat system prompt must not reach a coding harness")

	second, ok := messages[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "user", second["role"])
	require.Equal(t, "add a test for the reaper", second["content"])

	// Every sampling parameter the harness chose is forwarded as-is.
	require.InDelta(t, 0.15, got["temperature"], 1e-9)
	require.InDelta(t, 0.85, got["top_p"], 1e-9)
	require.InDelta(t, 0.2, got["frequency_penalty"], 1e-9)
	require.InDelta(t, 0.3, got["presence_penalty"], 1e-9)
	require.EqualValues(t, 4096, got["max_tokens"])

	// The harness's tools are neither replaced nor augmented, and Helix does not
	// take over the tool call loop.
	tools, ok := got["tools"].([]any)
	require.True(t, ok, "tools missing from upstream body: %v", got)
	require.Len(t, tools, 1)

	// The provider prefix is stripped because upstream does not know Helix's
	// naming scheme; the bare model id is what the harness asked for.
	require.Equal(t, "qwen3.8-27b", got["model"])
	require.Equal(t, "/v1/chat/completions", upstream.paths[0])

	// The recorded call attributes to the task with no App, and its forwarded
	// request matches what the harness sent.
	require.Eventually(t, func() bool { return capture.get() != nil }, 5*time.Second, 20*time.Millisecond)
	call := capture.get()
	require.Empty(t, call.AppID, "a code-agent call must not be attributed to an App")
	require.Equal(t, types.CodeAgentRuntimeOpenCode, call.CodeAgentRuntime)
	require.Equal(t, "spt_test", call.SpecTaskID)

	var original, forwarded map[string]any
	require.NoError(t, json.Unmarshal(call.OriginalRequest, &original))
	require.NoError(t, json.Unmarshal(call.Request, &forwarded))
	require.Equal(t, original["messages"], forwarded["messages"],
		"original_request and request must agree — a mismatch here is the production symptom")
}

// TestCodeAgentChatCompletionReadsNoAppAssistantOrUserDefaults is the "loads
// nothing" half of the guarantee. The mock store has NO GetApp, GetAppWithTools
// or GetUserMeta expectation, and gomock fails the test on an unexpected call —
// so this asserts that serving a coding-agent request touches neither an App,
// nor an assistant, nor the user's stored chat defaults.
func TestCodeAgentChatCompletionReadsNoAppAssistantOrUserDefaults(t *testing.T) {
	upstream := newFakeOpenAI(t)
	server, mockStore, _ := codeAgentTestServer(t, upstream)

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID: "ses_test",
		Metadata: types.SessionMetadata{
			CodeAgentConfig: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeOpenCode,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
				ProviderRef:    "openai",
				Model:          "qwen3.8-27b",
			},
		},
	}, nil)

	recorder := httptest.NewRecorder()
	server.createChatCompletion(recorder, codeAgentChatRequest(t, types.User{
		ID:        "user_test",
		SessionID: "ses_test",
	}))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	got := upstream.received(t)
	messages := got["messages"].([]any)
	require.Equal(t, harnessSystemPrompt, messages[0].(map[string]any)["content"])
	require.InDelta(t, 0.15, got["temperature"], 1e-9)
}

// TestCodeAgentChatCompletionIgnoresAppIDOnKeyAndQuery pins the second fix: a
// task that owns its execution config has no App, and neither an app_id stamped
// on the API key nor one passed in the query string may reintroduce one. No
// GetApp expectation is registered, so any attempt to load one fails the test.
func TestCodeAgentChatCompletionIgnoresAppIDOnKeyAndQuery(t *testing.T) {
	upstream := newFakeOpenAI(t)
	server, mockStore, capture := codeAgentTestServer(t, upstream)

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_test").Return(&types.Session{
		ID:       "ses_test",
		Metadata: types.SessionMetadata{SpecTaskID: "spt_test"},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_test").
		Return(specTaskWithCodeAgentConfig(types.CodeAgentRuntimeZedAgent), nil)

	req := codeAgentChatRequest(t, types.User{
		ID:         "user_test",
		SessionID:  "ses_test",
		SpecTaskID: "spt_test",
		AppID:      "app_stale_on_key",
	})
	req.URL.RawQuery = "app_id=app_from_query"

	recorder := httptest.NewRecorder()
	server.createChatCompletion(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	got := upstream.received(t)
	require.Equal(t, harnessSystemPrompt, got["messages"].([]any)[0].(map[string]any)["content"])

	require.Eventually(t, func() bool { return capture.get() != nil }, 5*time.Second, 20*time.Millisecond)
	require.Empty(t, capture.get().AppID)
}

// TestPlainChatCompletionStillAppliesUserChatDefaults is the scoping guard: the
// fix must only affect coding-agent proxy traffic. A normal user chatting
// directly with a model still gets DefaultChatSystemPrompt and their stored
// chat settings, which is what the no-App branch exists for.
func TestPlainChatCompletionStillAppliesUserChatDefaults(t *testing.T) {
	upstream := newFakeOpenAI(t)
	server, mockStore, _ := codeAgentTestServer(t, upstream)

	temperature := float32(0.9)
	mockStore.EXPECT().GetUserMeta(gomock.Any(), "user_test").Return(&types.UserMeta{
		ID: "user_test",
		ChatSettings: types.UserChatSettings{
			SystemPrompt: "Answer in haiku.",
			Temperature:  &temperature,
		},
	}, nil)

	// No SessionID: this is not a coding-agent proxy request, so no attribution
	// lookup happens and the user's chat defaults apply.
	recorder := httptest.NewRecorder()
	server.createChatCompletion(recorder, codeAgentChatRequest(t, types.User{ID: "user_test"}))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	got := upstream.received(t)
	require.Equal(t, "Answer in haiku.", got["messages"].([]any)[0].(map[string]any)["content"])
	require.InDelta(t, 0.9, got["temperature"], 1e-6)
}
