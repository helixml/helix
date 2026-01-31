package logger

import (
	"context"
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
