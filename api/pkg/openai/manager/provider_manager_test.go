package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

type MultiClientManagerTestSuite struct {
	suite.Suite
	cfg *config.ServerConfig
}

func TestMultiClientManagerTestSuite(t *testing.T) {
	suite.Run(t, new(MultiClientManagerTestSuite))
}

func (suite *MultiClientManagerTestSuite) SetupTest() {
	suite.cfg = &config.ServerConfig{
		Providers: config.Providers{
			OpenAI: config.OpenAI{
				APIKeyFromFile: "/app/api/secrets/openai-api-key",
			},
		},
	}
}

func (suite *MultiClientManagerTestSuite) Test_WatchAndUpdateClient() {
	// Create a temporary file for testing
	tmpDir := suite.T().TempDir()
	keyFile := filepath.Join(tmpDir, "api-key")
	err := os.WriteFile(keyFile, []byte("initial-key"), 0644)
	suite.NoError(err)

	// Create manager with initial key
	manager := NewProviderManager(suite.cfg, nil)

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching the file with a short interval
	interval := 20 * time.Millisecond
	go func() {
		err := manager.watchAndUpdateClient(ctx, types.ProviderOpenAI, interval, "https://api.openai.com/v1", keyFile)
		suite.NoError(err)
	}()

	// Write new keys to the file
	expectedKeys := []string{"key1", "key2", "key3"}
	for _, key := range expectedKeys {
		time.Sleep(interval * 2) // Wait for two intervals to ensure the file is read
		err := os.WriteFile(keyFile, []byte(key), 0644)
		suite.NoError(err)

		// Wait for the key to be updated
		time.Sleep(interval * 2)

		// Get the client and verify the API key
		client, err := manager.GetClient(ctx, &GetClientRequest{Provider: types.ProviderOpenAI})
		suite.NoError(err)

		// Type assert to access the underlying client
		// Note: This assumes the client implements a way to get the API key
		// You might need to modify your openai.Client interface to expose this
		openaiClient, ok := client.(*logger.LoggingMiddleware)
		suite.Require().True(ok)

		suite.Equal(key, openaiClient.APIKey())
	}
}
