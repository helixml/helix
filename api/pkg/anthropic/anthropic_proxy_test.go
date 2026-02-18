package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

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

func Test_parseAnthropicRequestModel(t *testing.T) {
	tests := []struct {
		name  string
		body  []byte
		want  string
		found bool
	}{
		{
			name:  "valid model",
			body:  []byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`),
			want:  "claude-sonnet-4-20250514",
			found: true,
		},
		{
			name:  "missing model",
			body:  []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
			want:  "",
			found: false,
		},
		{
			name:  "invalid json",
			body:  []byte(`{"model":"claude-sonnet-4"`),
			want:  "",
			found: false,
		},
		{
			name:  "empty body",
			body:  []byte{},
			want:  "",
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseAnthropicRequestModel(tt.body)
			assert.Equal(t, tt.found, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
