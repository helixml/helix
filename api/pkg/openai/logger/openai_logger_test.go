package logger

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tiktoken-go/tokenizer"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type captureLogStore struct {
	calls chan *types.LLMCall
}

func (s *captureLogStore) CreateLLMCall(_ context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	s.calls <- call
	return call, nil
}

func TestAppendChunkUsesLatestUsageSnapshot(t *testing.T) {
	resp := &openai.ChatCompletionResponse{}

	appendChunk(resp, &openai.ChatCompletionStreamResponse{
		Usage: &openai.Usage{
			PromptTokens:     100,
			CompletionTokens: 10,
			TotalTokens:      110,
			PromptTokensDetails: &openai.PromptTokensDetails{
				CachedTokens: 80,
			},
		},
	})
	appendChunk(resp, &openai.ChatCompletionStreamResponse{
		Usage: &openai.Usage{
			PromptTokens:     100,
			CompletionTokens: 20,
			TotalTokens:      120,
			PromptTokensDetails: &openai.PromptTokensDetails{
				CachedTokens: 80,
			},
		},
	})

	require.NotNil(t, resp.Usage.PromptTokensDetails)
	assert.Equal(t, 100, resp.Usage.PromptTokens)
	assert.Equal(t, 20, resp.Usage.CompletionTokens)
	assert.Equal(t, 120, resp.Usage.TotalTokens)
	assert.Equal(t, 80, resp.Usage.PromptTokensDetails.CachedTokens)
}

func TestAppendChunkAcceptsFinalOnlyUsage(t *testing.T) {
	resp := &openai.ChatCompletionResponse{}
	appendChunk(resp, &openai.ChatCompletionStreamResponse{
		Choices: []openai.ChatCompletionStreamChoice{{
			Delta: openai.ChatCompletionStreamChoiceDelta{Content: "hello"},
		}},
	})
	appendChunk(resp, &openai.ChatCompletionStreamResponse{
		Usage: &openai.Usage{
			PromptTokens:     40,
			CompletionTokens: 2,
			TotalTokens:      42,
		},
	})

	assert.Equal(t, "hello", resp.Choices[0].Message.Content)
	assert.Equal(t, 40, resp.Usage.PromptTokens)
	assert.Equal(t, 2, resp.Usage.CompletionTokens)
	assert.Equal(t, 42, resp.Usage.TotalTokens)
}

func TestCreateChatCompletionStreamLogsLatestUsageSnapshot(t *testing.T) {
	chunks := []openai.ChatCompletionStreamResponse{
		{
			ID: "chatcmpl_test",
			Choices: []openai.ChatCompletionStreamChoice{{
				Delta: openai.ChatCompletionStreamChoiceDelta{Content: "hello "},
			}},
			Usage: &openai.Usage{
				PromptTokens:     100,
				CompletionTokens: 1,
				TotalTokens:      101,
				PromptTokensDetails: &openai.PromptTokensDetails{
					CachedTokens: 80,
				},
			},
		},
		{
			ID: "chatcmpl_test",
			Choices: []openai.ChatCompletionStreamChoice{{
				Delta: openai.ChatCompletionStreamChoiceDelta{Content: "world"},
			}},
			Usage: &openai.Usage{
				PromptTokens:     100,
				CompletionTokens: 2,
				TotalTokens:      102,
				PromptTokensDetails: &openai.PromptTokensDetails{
					CachedTokens: 80,
				},
			},
		},
	}

	var streamBody strings.Builder
	for _, chunk := range chunks {
		payload, err := json.Marshal(chunk)
		require.NoError(t, err)
		streamBody.WriteString("data: ")
		streamBody.Write(payload)
		streamBody.WriteString("\n\n")
	}
	streamBody.WriteString("data: [DONE]\n\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(streamBody.String()))
	}))
	defer server.Close()

	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)
	captured := &captureLogStore{calls: make(chan *types.LLMCall, 2)}
	middleware := Wrap(
		nil,
		types.ProviderOpenAI,
		oai.New("test", server.URL, true),
		modelInfoProvider,
		nil,
		captured,
	)

	stream, err := middleware.CreateChatCompletionStream(context.Background(), openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
	})
	require.NoError(t, err)
	for {
		_, err = stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}
	middleware.wg.Wait()

	require.Equal(t, 1, len(captured.calls))
	call := <-captured.calls
	assert.Equal(t, int64(100), call.PromptTokens)
	assert.Equal(t, int64(2), call.CompletionTokens)
	assert.Equal(t, int64(102), call.TotalTokens)
	assert.Equal(t, int64(80), call.CacheReadTokens)
}

func Test_computeTokenUsage_SingleMessage(t *testing.T) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)
	mw := &LoggingMiddleware{
		defaultCodec: enc,
	}

	// Test case 1: OpenAI model
	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello, world!"},
		},
	}
	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{Content: "Hello, world!"},
			},
		},
	}

	promptTokens, completionTokens, totalTokens := mw.computeTokenUsage(req, resp)
	assert.Equal(t, 4, promptTokens)
	assert.Equal(t, 4, completionTokens)
	assert.Equal(t, 8, totalTokens)
}

func Test_computeTokenUsage_MultipleMessage(t *testing.T) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)
	mw := &LoggingMiddleware{
		defaultCodec: enc,
	}

	// Test case 1: OpenAI model
	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello, world!"},
			{Role: "assistant", Content: "Hello"},
			{Role: "user", Content: "How are you?"},
		},
	}
	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{Content: "OK!"},
			},
		},
	}

	promptTokens, completionTokens, totalTokens := mw.computeTokenUsage(req, resp)
	assert.Equal(t, 9, promptTokens)
	assert.Equal(t, 2, completionTokens)
	assert.Equal(t, 11, totalTokens)
}

func Test_logLLMCall_WithoutBillingLogger(t *testing.T) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)

	mockClient := oai.NewMockClient(ctrl)

	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)

	// Create a LoggingMiddleware without a billing logger
	mw := &LoggingMiddleware{
		defaultCodec: enc,
		// billingLogger is nil by default
		provider:          types.ProviderOpenAI, // Set a default provider
		modelInfoProvider: modelInfoProvider,
	}

	// Create test request and response
	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello, world!"},
		},
	}

	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{Content: "Hello, world!"},
			},
		},
		Usage: openai.Usage{
			PromptTokens:     4,
			CompletionTokens: 4,
			TotalTokens:      8,
		},
	}

	// Create a context with some test values
	ctx := context.Background()

	// Create a mock model info provider

	mw.client = mockClient
	mw.billingLogger = nil // Explicitly set to nil

	mockClient.EXPECT().CreateChatCompletion(ctx, *req).Return(*resp, nil)

	mockClient.EXPECT().BaseURL().Return("https://api.openai.com/v1")

	// Test that CreateChatCompletion works without a billing logger
	// This will call logLLMCall internally
	result, err := mw.CreateChatCompletion(ctx, *req)
	require.NoError(t, err)
	assert.Equal(t, resp.Choices[0].Message.Content, result.Choices[0].Message.Content)

	// Wait for the goroutine to complete
	mw.wg.Wait()
}

func Test_logLLMCall_WithBillingLogger_User(t *testing.T) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)

	ctx := context.Background()

	ctrl := gomock.NewController(t)
	mockClient := oai.NewMockClient(ctrl)
	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)
	store := store.NewMockStore(ctrl)

	billingLogger, err := NewBillingLogger(store, true)
	require.NoError(t, err)

	vals := &oai.ContextValues{
		OwnerID:       "user_123",
		InteractionID: "interaction_123",
	}

	ctx = oai.SetContextValues(ctx, vals)

	// Create a LoggingMiddleware without a billing logger
	mw := &LoggingMiddleware{
		defaultCodec: enc,
		// billingLogger is nil by default
		provider:          types.ProviderOpenAI, // Set a default provider
		modelInfoProvider: modelInfoProvider,
		billingLogger:     billingLogger,
		client:            mockClient,
	}

	// Create test request and response
	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello, world!"},
		},
	}

	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{Content: "Hello, world!"},
			},
		},
		Usage: openai.Usage{
			PromptTokens:     40000,
			CompletionTokens: 40000,
			TotalTokens:      80000,
		},
	}

	// Create a mock model info provider

	mockClient.EXPECT().CreateChatCompletion(ctx, *req).Return(*resp, nil)

	mockClient.EXPECT().BaseURL().Return("https://api.openai.com/v1")

	// We will expect a wallet get and transaction
	store.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(&types.Wallet{
		ID:      "wallet_123",
		Balance: 100,
	}, nil)

	store.EXPECT().UpdateWalletBalance(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, walletID string, amount float64, metadata types.TransactionMetadata) (*types.Wallet, error) {
			// Validate amount
			assert.Equal(t, -0.08, amount)

			// Wallet
			assert.Equal(t, "wallet_123", walletID)

			// Validate metadata
			assert.Equal(t, "interaction_123", metadata.InteractionID)
			assert.Equal(t, types.TransactionTypeUsage, metadata.TransactionType)

			return &types.Wallet{
				ID:      walletID,
				Balance: 100 - amount,
			}, nil
		},
	)

	// Test that CreateChatCompletion works without a billing logger
	// This will call logLLMCall internally
	result, err := mw.CreateChatCompletion(ctx, *req)
	require.NoError(t, err)
	assert.Equal(t, resp.Choices[0].Message.Content, result.Choices[0].Message.Content)

	// Wait for the goroutine to complete
	mw.wg.Wait()
}

func Test_logLLMCall_WithBillingLogger_Org(t *testing.T) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)

	ctx := context.Background()

	ctrl := gomock.NewController(t)
	mockClient := oai.NewMockClient(ctrl)
	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)
	store := store.NewMockStore(ctrl)

	billingLogger, err := NewBillingLogger(store, true)
	require.NoError(t, err)

	vals := &oai.ContextValues{
		OwnerID:       "user_123",
		InteractionID: "interaction_123",
	}

	ctx = oai.SetContextValues(ctx, vals)

	ctx = oai.SetContextOrganizationID(ctx, "org_123")

	// Create a LoggingMiddleware without a billing logger
	mw := &LoggingMiddleware{
		defaultCodec: enc,
		// billingLogger is nil by default
		provider:          types.ProviderOpenAI, // Set a default provider
		modelInfoProvider: modelInfoProvider,
		billingLogger:     billingLogger,
		client:            mockClient,
	}

	// Create test request and response
	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello, world!"},
		},
	}

	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{Content: "Hello, world!"},
			},
		},
		Usage: openai.Usage{
			PromptTokens:     40000,
			CompletionTokens: 40000,
			TotalTokens:      80000,
		},
	}

	// Create a mock model info provider

	mockClient.EXPECT().CreateChatCompletion(ctx, *req).Return(*resp, nil)

	mockClient.EXPECT().BaseURL().Return("https://api.openai.com/v1")

	// We will expect a wallet get and transaction
	store.EXPECT().GetWalletByOrg(gomock.Any(), "org_123").Return(&types.Wallet{
		ID:      "wallet_123",
		Balance: 100,
	}, nil)

	store.EXPECT().UpdateWalletBalance(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, walletID string, amount float64, metadata types.TransactionMetadata) (*types.Wallet, error) {
			// Validate amount
			assert.Equal(t, -0.08, amount)

			// Wallet
			assert.Equal(t, "wallet_123", walletID)

			// Validate metadata
			assert.Equal(t, "interaction_123", metadata.InteractionID)
			assert.Equal(t, types.TransactionTypeUsage, metadata.TransactionType)

			return &types.Wallet{
				ID:      walletID,
				Balance: 100 - amount,
			}, nil
		},
	)

	// Test that CreateChatCompletion works without a billing logger
	// This will call logLLMCall internally
	result, err := mw.CreateChatCompletion(ctx, *req)
	require.NoError(t, err)
	assert.Equal(t, resp.Choices[0].Message.Content, result.Choices[0].Message.Content)

	// Wait for the goroutine to complete
	mw.wg.Wait()
}
