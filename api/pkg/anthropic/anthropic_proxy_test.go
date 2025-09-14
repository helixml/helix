package anthropic

import (
	"context"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestProxySuite(t *testing.T) {
	suite.Run(t, new(ProxySuite))
}

type ProxySuite struct {
	suite.Suite

	store             *store.MockStore
	modelInfoProvider *model.MockModelInfoProvider
	ctx               context.Context

	billing *logger.BillingLogger
}

func (suite *ProxySuite) SetupSuite() {
	ctrl := gomock.NewController(suite.T())

	suite.ctx = context.Background()
	suite.store = store.NewMockStore(ctrl)
	suite.modelInfoProvider = model.NewMockModelInfoProvider(ctrl)

	billingLogger, err := logger.NewBillingLogger(suite.store, true)
	suite.NoError(err)

	suite.billing = billingLogger
}

func (suite *ProxySuite) TestProxyBilling_OK() {

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

func Test_appendChunk(t *testing.T) {
	tests := []struct {
		name     string
		resp     *anthropic.Message
		chunk    *anthropic.Message
		expected anthropic.Message
	}{
		{
			name: "append chunk to empty response",
			resp: &anthropic.Message{},
			chunk: &anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
				StopReason: "end_turn",
				Usage: anthropic.Usage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			},
			expected: anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
				StopReason: "end_turn",
				Usage: anthropic.Usage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			},
		},
		{
			name: "append chunk to existing response",
			resp: &anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
				Usage: anthropic.Usage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			},
			chunk: &anthropic.Message{
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: " world",
					},
				},
				StopReason: "end_turn",
				Usage: anthropic.Usage{
					InputTokens:  0,
					OutputTokens: 2,
				},
			},
			expected: anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
					{
						Type: "text",
						Text: " world",
					},
				},
				StopReason: "end_turn",
				Usage: anthropic.Usage{
					InputTokens:  10,
					OutputTokens: 7,
				},
			},
		},
		{
			name: "append nil chunk",
			resp: &anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
			},
			chunk: nil,
			expected: anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
			},
		},
		{
			name: "append chunk with empty content",
			resp: &anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
			},
			chunk: &anthropic.Message{
				StopReason: "end_turn",
				Usage: anthropic.Usage{
					InputTokens:  0,
					OutputTokens: 0,
				},
			},
			expected: anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
				StopReason: "end_turn",
				Usage: anthropic.Usage{
					InputTokens:  0,
					OutputTokens: 0,
				},
			},
		},
		{
			name: "append chunk with usage to response without usage",
			resp: &anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
				},
			},
			chunk: &anthropic.Message{
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: " world",
					},
				},
				Usage: anthropic.Usage{
					InputTokens:  5,
					OutputTokens: 2,
				},
			},
			expected: anthropic.Message{
				ID:    "msg_123",
				Model: "claude-3-5-sonnet-20241022",
				Role:  "assistant",
				Content: []anthropic.ContentBlockUnion{
					{
						Type: "text",
						Text: "Hello",
					},
					{
						Type: "text",
						Text: " world",
					},
				},
				Usage: anthropic.Usage{
					InputTokens:  5,
					OutputTokens: 2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the response to avoid modifying the original
			resp := &anthropic.Message{}
			if tt.resp != nil {
				*resp = *tt.resp
			}

			// Call appendChunk
			appendChunk(resp, tt.chunk)

			// Verify the result
			assert.Equal(t, tt.expected.ID, resp.ID)
			assert.Equal(t, tt.expected.Model, resp.Model)
			assert.Equal(t, tt.expected.Role, resp.Role)
			assert.Equal(t, tt.expected.StopReason, resp.StopReason)
			assert.Equal(t, tt.expected.StopSequence, resp.StopSequence)
			assert.Equal(t, len(tt.expected.Content), len(resp.Content))

			// Verify content blocks
			for i, expectedBlock := range tt.expected.Content {
				require.Less(t, i, len(resp.Content), "response content should have at least %d blocks", i+1)
				assert.Equal(t, expectedBlock.Type, resp.Content[i].Type)
				if expectedBlock.Text != "" {
					assert.Equal(t, expectedBlock.Text, resp.Content[i].Text)
				}
			}

			// Verify usage
			if tt.expected.Usage.InputTokens > 0 || tt.expected.Usage.OutputTokens > 0 {
				assert.Equal(t, tt.expected.Usage.InputTokens, resp.Usage.InputTokens)
				assert.Equal(t, tt.expected.Usage.OutputTokens, resp.Usage.OutputTokens)
			}
		})
	}
}
