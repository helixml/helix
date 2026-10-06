package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/toolcall"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type captureAnthropicLogStore struct {
	calls chan *types.LLMCall
}

func (s *captureAnthropicLogStore) CreateLLMCall(_ context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	s.calls <- call
	return call, nil
}

func TestProxySuite(t *testing.T) {
	suite.Run(t, new(ProxySuite))
}

type ProxySuite struct {
	suite.Suite

	ctx               context.Context
	cfg               *config.ServerConfig
	store             *store.MockStore
	modelInfoProvider model.ModelInfoProvider
	billing           *logger.BillingLogger

	proxy *Proxy
}

func (suite *ProxySuite) SetupSuite() {
	ctrl := gomock.NewController(suite.T())

	suite.cfg = &config.ServerConfig{}
	suite.cfg.Stripe.BillingEnabled = true

	suite.ctx = context.Background()
	suite.store = store.NewMockStore(ctrl)

	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	suite.NoError(err, "failed to init model info provider")

	suite.modelInfoProvider = modelInfoProvider

	billingLogger, err := logger.NewBillingLogger(suite.store, true)
	suite.NoError(err)

	suite.billing = billingLogger

	suite.proxy = New(suite.cfg, suite.store, suite.modelInfoProvider)
}

func (suite *ProxySuite) TestProxyBilling_OK() {
	userID := "user-123"
	llmResponse := anthropic.Message{
		Model: anthropic.Model("claude-sonnet-4-20250514"),
		Content: []anthropic.ContentBlockUnion{
			{
				Text: "hello to you too",
			},
		},
		Usage: anthropic.Usage{
			InputTokens:  100,
			OutputTokens: 2000,
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmResponse)
	}))
	defer ts.Close()

	llmRequest := `{
    "model": "claude-sonnet-4-20250514",
    "stream": true, "max_tokens": 1024,
    "messages": [
        {"role": "user", "content": "Hello, world"}
    ]
	}`

	ctx := oai.SetContextValues(suite.ctx, &oai.ContextValues{
		InteractionID:   "interaction_123",
		OriginalRequest: []byte(llmRequest),
		OwnerID:         userID,
	})

	suite.store.EXPECT().GetWalletByUser(gomock.Any(), userID).Return(&types.Wallet{
		ID:      "wallet_123",
		Balance: 100,
	}, nil)

	suite.store.EXPECT().UpdateWalletBalance(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, walletID string, amount float64, metadata types.TransactionMetadata) (*types.Wallet, error) {
			// Validate amount
			assert.Equal(suite.T(), -0.030300000000000004, amount)

			// Wallet
			assert.Equal(suite.T(), "wallet_123", walletID)

			// Validate metadata
			assert.Equal(suite.T(), "interaction_123", metadata.InteractionID)
			assert.Equal(suite.T(), types.TransactionTypeUsage, metadata.TransactionType)

			return &types.Wallet{
				ID:      walletID,
				Balance: 100 - amount,
			}, nil
		},
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost:8080/v1/messages", bytes.NewBufferString(llmRequest))
	suite.NoError(err)

	// Preparing the context
	req = SetRequestProviderEndpoint(req, &types.ProviderEndpoint{
		Name:    "anthropic",
		BaseURL: ts.URL,
	})

	rec := httptest.NewRecorder()

	suite.proxy.ServeHTTP(rec, req)

	suite.Equal(200, rec.Result().StatusCode)

	respBody, err := io.ReadAll(rec.Result().Body)
	suite.NoError(err, "failed to read response body")

	suite.Contains(string(respBody), "hello to you too")

	suite.proxy.wg.Wait()
}

func TestStreamingProxyLogsCumulativeUsageSnapshot(t *testing.T) {
	cfg := &config.ServerConfig{}
	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)

	logStore := &captureAnthropicLogStore{calls: make(chan *types.LLMCall, 1)}
	proxy := New(cfg, nil, modelInfoProvider, logStore)

	ctx := oai.SetContextValues(context.Background(), &oai.ContextValues{
		InteractionID:   "interaction_123",
		OriginalRequest: []byte(`{"model":"claude-sonnet-4-20250514","stream":true}`),
		OwnerID:         "user-123",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://localhost/v1/messages", nil)
	require.NoError(t, err)
	req = SetRequestProviderEndpoint(req, &types.ProviderEndpoint{
		ID:      "anthropic-endpoint",
		Name:    "anthropic",
		BaseURL: "https://api.anthropic.com",
	})
	req = setStartTime(req, time.Now())

	stream := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":80,"output_tokens":1}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":20}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream)),
		Request:    req,
	}

	require.NoError(t, proxy.anthropicAPIProxyModifyResponse(resp))
	forwarded, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, stream, string(forwarded))

	var call *types.LLMCall
	select {
	case call = <-logStore.calls:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Anthropic usage log")
	}
	assert.EqualValues(t, 200, call.PromptTokens)
	assert.EqualValues(t, 20, call.CompletionTokens)
	assert.EqualValues(t, 220, call.TotalTokens)
	assert.EqualValues(t, 80, call.CacheReadTokens)
	assert.EqualValues(t, 20, call.CacheWriteTokens)
}

func TestLogLLMCallSkipsVertexErrorResponse(t *testing.T) {
	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)

	logStore := &captureAnthropicLogStore{calls: make(chan *types.LLMCall, 1)}
	proxy := New(&config.ServerConfig{}, nil, modelInfoProvider, logStore)

	req, err := http.NewRequest(http.MethodPost, "https://localhost/v1/messages", nil)
	require.NoError(t, err)
	req = SetRequestProviderEndpoint(req, &types.ProviderEndpoint{
		ID:      "vertex-endpoint",
		Name:    "anthropic",
		BaseURL: "https://us-east5-aiplatform.googleapis.com",
	})

	proxy.logLLMCall(req.Context(), time.Now(), []byte(`[{"error":{"code":403,"message":"Vertex AI API is disabled","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"SERVICE_DISABLED","domain":"googleapis.com"}]}}]`), nil, false, 0)

	select {
	case <-logStore.calls:
		t.Fatal("Vertex error response was logged as an LLM call")
	default:
	}
}

func Test_stripDateFromModelName(t *testing.T) {
	tests := []struct {
		name      string // description of this test case
		modelName string
		want      string
	}{
		// Sonnet models with dates
		{
			name:      "claude-sonnet-4 with dash date",
			modelName: "claude-sonnet-4-20250514",
			want:      "claude-sonnet-4",
		},
		{
			name:      "claude-sonnet-4 with @ date",
			modelName: "claude-sonnet-4@20250514",
			want:      "claude-sonnet-4",
		},
		{
			name:      "claude-sonnet-4 with thinking suffix and dash date",
			modelName: "claude-sonnet-4-20250514-thinking",
			want:      "claude-sonnet-4-20250514-thinking", // Should not strip since "thinking" is not a date
		},
		{
			name:      "claude-sonnet-4 with thinking suffix and @ date",
			modelName: "claude-sonnet-4@20250514-thinking",
			want:      "claude-sonnet-4@20250514-thinking", // Should not strip since "thinking" is not a date
		},

		// Opus models with dates
		{
			name:      "claude-opus-4 with dash date",
			modelName: "claude-opus-4-20250514",
			want:      "claude-opus-4",
		},
		{
			name:      "claude-opus-4 with @ date",
			modelName: "claude-opus-4@20250514",
			want:      "claude-opus-4",
		},
		{
			name:      "claude-opus-4-1 with dash date",
			modelName: "claude-opus-4-1-20250805",
			want:      "claude-opus-4-1",
		},
		{
			name:      "claude-opus-4-1 with @ date",
			modelName: "claude-opus-4-1@20250805",
			want:      "claude-opus-4-1",
		},
		{
			name:      "claude-opus-4-1 with thinking suffix and dash date",
			modelName: "claude-opus-4-1-20250805-thinking",
			want:      "claude-opus-4-1-20250805-thinking", // Should not strip since "thinking" is not a date
		},
		{
			name:      "claude-opus-4-1 with thinking suffix and @ date",
			modelName: "claude-opus-4-1@20250805-thinking",
			want:      "claude-opus-4-1@20250805-thinking", // Should not strip since "thinking" is not a date
		},

		// Haiku models with dates
		{
			name:      "claude-3-5-haiku with dash date",
			modelName: "claude-3-5-haiku-20241022",
			want:      "claude-3-5-haiku",
		},
		{
			name:      "claude-3-5-haiku with @ date",
			modelName: "claude-3-5-haiku@20241022",
			want:      "claude-3-5-haiku",
		},
		{
			name:      "claude-3-haiku with dash date",
			modelName: "claude-3-haiku-20240307",
			want:      "claude-3-haiku",
		},
		{
			name:      "claude-3-haiku with @ date",
			modelName: "claude-3-haiku@20240307",
			want:      "claude-3-haiku",
		},

		// Models without dates (should remain unchanged)
		{
			name:      "claude-sonnet-4 without date",
			modelName: "claude-sonnet-4",
			want:      "claude-sonnet-4",
		},
		{
			name:      "claude-opus-4 without date",
			modelName: "claude-opus-4",
			want:      "claude-opus-4",
		},
		{
			name:      "claude-opus-4-1 without date",
			modelName: "claude-opus-4-1",
			want:      "claude-opus-4-1",
		},
		{
			name:      "claude-3-5-haiku without date",
			modelName: "claude-3-5-haiku",
			want:      "claude-3-5-haiku",
		},
		{
			name:      "claude-3-haiku without date",
			modelName: "claude-3-haiku",
			want:      "claude-3-haiku",
		},

		// Edge cases
		{
			name:      "empty string",
			modelName: "",
			want:      "",
		},
		{
			name:      "single word",
			modelName: "claude",
			want:      "claude",
		},
		{
			name:      "model with non-date suffix",
			modelName: "claude-sonnet-4-beta",
			want:      "claude-sonnet-4-beta",
		},
		{
			name:      "model with short numeric suffix",
			modelName: "claude-sonnet-4-123",
			want:      "claude-sonnet-4-123", // Should not strip since it's not 8 digits
		},
		{
			name:      "model with long numeric suffix",
			modelName: "claude-sonnet-4-123456789",
			want:      "claude-sonnet-4-123456789", // Should not strip since it's not 8 digits
		},
		{
			name:      "model with mixed alphanumeric suffix",
			modelName: "claude-sonnet-4-2025a0514",
			want:      "claude-sonnet-4-2025a0514", // Should not strip since it contains non-numeric characters
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDateFromModelName(tt.modelName)
			assert.Equal(t, tt.want, got, "stripDateFromModelName() = %v, want %v", got, tt.want)
		})
	}
}

func TestValidateToolCallsAnthropic(t *testing.T) {
	request := []byte(`{
		"model": "claude-opus-5",
		"tools": [
			{"name": "edit_file", "input_schema": {"type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"], "additionalProperties": false}},
			{"type": "bash_20250124", "name": "bash"}
		]
	}`)

	message := func(blocks string) *anthropic.Message {
		var msg anthropic.Message
		require.NoError(t, msg.UnmarshalJSON([]byte(`{"content": `+blocks+`}`)))
		return &msg
	}

	t.Run("valid tool_use block", func(t *testing.T) {
		result := validateToolCalls(request, message(`[{"type": "tool_use", "name": "edit_file", "input": {"path": "a.go"}}]`))
		assert.Equal(t, 2, result.ToolsOffered)
		assert.Equal(t, 1, result.Calls)
		assert.Equal(t, 0, result.Errors)
	})

	t.Run("input misses the schema", func(t *testing.T) {
		result := validateToolCalls(request, message(`[{"type": "tool_use", "name": "edit_file", "input": {"file": "a.go"}}]`))
		assert.Equal(t, 1, result.Errors)
		assert.Equal(t, toolcall.KindSchemaMismatch, result.KindsString())
	})

	t.Run("server tools carry no schema and are unconstrained", func(t *testing.T) {
		result := validateToolCalls(request, message(`[{"type": "tool_use", "name": "bash", "input": {"command": "ls"}}]`))
		assert.Equal(t, 0, result.Errors)
	})

	t.Run("text blocks are not tool calls", func(t *testing.T) {
		result := validateToolCalls(request, message(`[{"type": "text", "text": "hello"}]`))
		assert.Equal(t, 0, result.Calls)
		assert.False(t, result.Errored())
	})
}

func TestNormalizeStopReason(t *testing.T) {
	assert.Equal(t, "tool_calls", normalizeStopReason(anthropic.StopReasonToolUse))
	assert.Equal(t, "stop", normalizeStopReason(anthropic.StopReasonEndTurn))
	assert.Equal(t, "length", normalizeStopReason(anthropic.StopReasonMaxTokens))
	assert.Equal(t, "pause_turn", normalizeStopReason(anthropic.StopReason("pause_turn")))
}
