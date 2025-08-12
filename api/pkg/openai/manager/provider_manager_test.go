package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	openai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	gomock "go.uber.org/mock/gomock"
)

type MultiClientManagerTestSuite struct {
	suite.Suite
	store             *store.MockStore
	cfg               *config.ServerConfig
	modelInfoProvider model.ModelInfoProvider
}

func TestMultiClientManagerTestSuite(t *testing.T) {
	suite.Run(t, new(MultiClientManagerTestSuite))
}

func (suite *MultiClientManagerTestSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.store = store.NewMockStore(ctrl)

	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	suite.NoError(err)
	suite.modelInfoProvider = modelInfoProvider

	suite.cfg = &config.ServerConfig{
		Providers: config.Providers{
			VLLM: config.VLLM{
				BaseURL: "http://vllm:8000",
				APIKey:  "vllm-key",
			},
			OpenAI: config.OpenAI{
				APIKeyFromFile: "/app/api/secrets/openai-api-key",
			},
		},
	}
}

func (suite *MultiClientManagerTestSuite) Test_VLLM() {
	manager := NewProviderManager(suite.cfg, suite.store, nil, suite.modelInfoProvider)
	client, err := manager.GetClient(context.Background(), &GetClientRequest{Provider: string(types.ProviderVLLM)})
	suite.NoError(err)
	suite.NotNil(client)
}

func (suite *MultiClientManagerTestSuite) Test_WatchAndUpdateClient() {
	// Create a temporary file for testing
	tmpDir := suite.T().TempDir()
	keyFile := filepath.Join(tmpDir, "api-key")
	err := os.WriteFile(keyFile, []byte("initial-key"), 0644)
	suite.NoError(err)

	// Set up mock expectations for store calls
	suite.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).
		Return([]*types.ProviderEndpoint{}, nil).
		AnyTimes() // Allow multiple calls since we check for client availability multiple times

	// Create manager with initial key
	manager := NewProviderManager(suite.cfg, suite.store, nil, suite.modelInfoProvider)

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start watching the file with a short interval
	interval := 50 * time.Millisecond // Increased interval to avoid tight loops

	err = manager.watchAndUpdateClient(ctx, types.ProviderOpenAI, interval, "https://api.openai.com/v1", keyFile)
	suite.NoError(err)

	// Helper function to wait for API key update with timeout
	waitForKeyUpdate := func(expectedKey string, maxRetries int) bool {
		for i := 0; i < maxRetries; i++ {
			client, err := manager.GetClient(ctx, &GetClientRequest{Provider: string(types.ProviderOpenAI)})
			if err != nil {
				suite.T().Logf("Attempt %d: Failed to get client: %v", i+1, err)
				time.Sleep(interval)
				continue
			}

			openaiClient, ok := client.(*logger.LoggingMiddleware)
			if !ok {
				suite.T().Logf("Attempt %d: Client is not LoggingMiddleware", i+1)
				time.Sleep(interval)
				continue
			}

			actualKey := openaiClient.APIKey()
			suite.T().Logf("Attempt %d: Expected key '%s', got key '%s'", i+1, expectedKey, actualKey)

			if actualKey == expectedKey {
				return true
			}

			time.Sleep(interval)
		}
		return false
	}

	// Test sequential key updates
	expectedKeys := []string{"key1", "key2", "key3"}
	for i, key := range expectedKeys {
		suite.T().Logf("Testing key update %d: %s", i+1, key)

		// Write the new key to the file
		err := os.WriteFile(keyFile, []byte(key), 0644)
		suite.NoError(err)

		// Wait for the file system change to be detected (give the file watcher a chance)
		time.Sleep(interval * 2)

		// Wait for the key to be updated with reasonable retries
		maxRetries := 20 // Allow up to 20 attempts
		success := waitForKeyUpdate(key, maxRetries)
		suite.True(success, "Expected key '%s' (iteration %d) was not updated within %d retries", key, i, maxRetries)

		// Verify the client has the correct API key
		client, err := manager.GetClient(ctx, &GetClientRequest{Provider: string(types.ProviderOpenAI)})
		suite.NoError(err)

		openaiClient, ok := client.(*logger.LoggingMiddleware)
		suite.Require().True(ok)
		suite.Equal(key, openaiClient.APIKey(), "API key mismatch for key '%s' (iteration %d)", key, i)
	}

	// Cancel the context and wait for cleanup
	cancel()

	// Wait for the background goroutine to finish with a timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.wg.Wait()
	}()

	select {
	case <-done:
		// Successfully cleaned up
	case <-time.After(5 * time.Second):
		suite.T().Fatal("Timeout waiting for background goroutines to finish")
	}
}

func (suite *MultiClientManagerTestSuite) Test_WatchAndUpdateClient_MissingFile() {
	// Create a temporary directory for testing (file doesn't exist initially)
	tmpDir := suite.T().TempDir()
	keyFile := filepath.Join(tmpDir, "api-key")

	// Set up mock expectations for store calls
	suite.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).
		Return([]*types.ProviderEndpoint{}, nil).
		AnyTimes() // Allow multiple calls since we check for client availability multiple times

	// Create manager
	manager := NewProviderManager(suite.cfg, suite.store, nil, suite.modelInfoProvider)

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start watching the file with a short interval
	interval := 50 * time.Millisecond

	err := manager.watchAndUpdateClient(ctx, types.ProviderOpenAI, interval, "https://api.openai.com/v1", keyFile)
	suite.NoError(err)

	// Helper function to wait for client availability with timeout
	waitForClientWithKey := func(expectedKey string, maxRetries int) bool {
		for i := 0; i < maxRetries; i++ {
			client, err := manager.GetClient(ctx, &GetClientRequest{Provider: string(types.ProviderOpenAI)})
			if err == nil {
				if openaiClient, ok := client.(*logger.LoggingMiddleware); ok {
					if openaiClient.APIKey() == expectedKey {
						return true
					}
				}
			}
			time.Sleep(interval)
		}
		return false
	}

	// Initially, the client should not be available (file doesn't exist)
	// Wait a bit to ensure the watcher has had a chance to try reading the file
	time.Sleep(interval * 3)

	_, err = manager.GetClient(ctx, &GetClientRequest{Provider: string(types.ProviderOpenAI)})
	suite.Error(err, "Client should not be available when file doesn't exist")

	// Create the file with the initial key after some delay
	time.Sleep(50 * time.Millisecond) // Small delay to simulate file creation
	err = os.WriteFile(keyFile, []byte("initial-key"), 0644)
	suite.NoError(err)

	// Wait for the client to be created and have the correct API key
	maxRetries := 30 // Allow extra time for file creation detection
	success := waitForClientWithKey("initial-key", maxRetries)
	suite.True(success, "Expected client with key 'initial-key' was not created within %d retries", maxRetries)

	// Verify the client has the correct API key
	client, err := manager.GetClient(ctx, &GetClientRequest{Provider: string(types.ProviderOpenAI)})
	suite.NoError(err)

	openaiClient, ok := client.(*logger.LoggingMiddleware)
	suite.Require().True(ok)
	suite.Equal("initial-key", openaiClient.APIKey())

	// Cancel the context and wait for cleanup
	cancel()

	// Wait for the background goroutine to finish with a timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		manager.wg.Wait()
	}()

	select {
	case <-done:
		// Successfully cleaned up
	case <-time.After(5 * time.Second):
		suite.T().Fatal("Timeout waiting for background goroutines to finish")
	}
}

func (suite *MultiClientManagerTestSuite) Test_initializeClient_WithoutBilling() {
	endpoint := &types.ProviderEndpoint{
		ID:     "openai",
		APIKey: "openai-api-key",
	}

	suite.cfg.Stripe.BillingEnabled = false

	manager := NewProviderManager(suite.cfg, suite.store, nil, suite.modelInfoProvider)
	client, err := manager.initializeClient(endpoint)
	suite.NoError(err)
	suite.NotNil(client)

	wrappedClient, ok := client.(*logger.LoggingMiddleware)
	suite.Require().True(ok)

	// Check that the billing logger is a NoopBillingLogger
	billingLogger := wrappedClient.BillingLogger()

	// Should be no-op
	res, err := billingLogger.CreateLLMCall(context.Background(), &types.LLMCall{
		Model: "gpt-4o",
	})
	suite.NoError(err)
	suite.NotNil(res)
}

func (suite *MultiClientManagerTestSuite) Test_initializeClient_WithBilling() {
	endpoint := &types.ProviderEndpoint{
		ID:     "openai",
		APIKey: "openai-api-key",
	}

	suite.cfg.Stripe.BillingEnabled = true

	manager := NewProviderManager(suite.cfg, suite.store, nil, suite.modelInfoProvider)
	client, err := manager.initializeClient(endpoint)
	suite.NoError(err)
	suite.NotNil(client)

	wrappedClient, ok := client.(*logger.LoggingMiddleware)
	suite.Require().True(ok)

	// Check that the billing logger is a NoopBillingLogger
	billingLogger := wrappedClient.BillingLogger()

	userID := "user_123"

	ctx := openai.SetContextValues(context.Background(), &openai.ContextValues{
		OwnerID: userID,
	})

	// Expect to get user wallet
	suite.store.EXPECT().GetWalletByUser(gomock.Any(), "user_123").Return(&types.Wallet{
		ID: "wallet_123",
	}, nil)

	suite.store.EXPECT().UpdateWalletBalance(gomock.Any(), "wallet_123", float64(-100), types.TransactionMetadata{
		InteractionID:   "int_123",
		LLMCallID:       "llm_1",
		TransactionType: types.TransactionTypeUsage,
	})

	// Should be no-op
	res, err := billingLogger.CreateLLMCall(ctx, &types.LLMCall{
		Model:         "gpt-4o",
		UserID:        userID,
		InteractionID: "int_123",
		ID:            "llm_1",
		TotalCost:     100,
	})
	suite.NoError(err)
	suite.NotNil(res)
}
