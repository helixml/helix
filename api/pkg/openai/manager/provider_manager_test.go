package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	gomock "go.uber.org/mock/gomock"
)

type MultiClientManagerTestSuite struct {
	suite.Suite
	store *store.MockStore
	cfg   *config.ServerConfig
}

func TestMultiClientManagerTestSuite(t *testing.T) {
	suite.Run(t, new(MultiClientManagerTestSuite))
}

func (suite *MultiClientManagerTestSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.store = store.NewMockStore(ctrl)

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
	manager := NewProviderManager(suite.cfg, suite.store, nil)
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

	// Create manager with initial key
	manager := NewProviderManager(suite.cfg, suite.store, nil)

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start watching the file with a short interval
	interval := 20 * time.Millisecond

	err = manager.watchAndUpdateClient(ctx, types.ProviderOpenAI, interval, "https://api.openai.com/v1", keyFile)
	suite.NoError(err)

	defer manager.wg.Wait()

	// Write new keys to the file
	expectedKeys := []string{"key1", "key2", "key3"}
	for _, key := range expectedKeys {
		time.Sleep(interval * 2) // Wait for two intervals to ensure the file is read
		err := os.WriteFile(keyFile, []byte(key), 0644)
		suite.NoError(err)

		// Wait for the key to be updated
		time.Sleep(interval * 2)

		// Get the client and verify the API key
		client, err := manager.GetClient(ctx, &GetClientRequest{Provider: string(types.ProviderOpenAI)})
		suite.NoError(err)

		// Type assert to access the underlying client
		// Note: This assumes the client implements a way to get the API key
		// You might need to modify your openai.Client interface to expose this
		openaiClient, ok := client.(*logger.LoggingMiddleware)
		suite.Require().True(ok)

		suite.Equal(key, openaiClient.APIKey())
	}

	cancel()
}

func (suite *MultiClientManagerTestSuite) Test_WatchAndUpdateClient_MissingFile() {
	// Create a temporary file for testing
	tmpDir := suite.T().TempDir()
	keyFile := filepath.Join(tmpDir, "api-key")

	go func() {
		time.Sleep(100 * time.Millisecond)
		err := os.WriteFile(keyFile, []byte("initial-key"), 0644)
		suite.NoError(err)
	}()

	// Create manager with initial key
	manager := NewProviderManager(suite.cfg, suite.store, nil)

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Start watching the file with a short interval
	interval := 20 * time.Millisecond

	err := manager.watchAndUpdateClient(ctx, types.ProviderOpenAI, interval, "https://api.openai.com/v1", keyFile)
	suite.NoError(err)

	defer manager.wg.Wait()

	// Wait for the file to be updated
	time.Sleep(time.Second)

	// Get the client and verify the API key
	client, err := manager.GetClient(ctx, &GetClientRequest{Provider: string(types.ProviderOpenAI)})
	suite.NoError(err)

	// Type assert to access the underlying client
	// Note: This assumes the client implements a way to get the API key
	// You might need to modify your openai.Client interface to expose this
	openaiClient, ok := client.(*logger.LoggingMiddleware)
	suite.Require().True(ok)

	suite.Equal("initial-key", openaiClient.APIKey())

	cancel()
}
