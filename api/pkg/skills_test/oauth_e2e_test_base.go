//go:build oauth_integration
// +build oauth_integration

package skills_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"go.uber.org/mock/gomock"
	"gocloud.dev/blob/memblob"
)

// BaseOAuthE2ETestSuite provides common infrastructure for OAuth E2E tests
type BaseOAuthE2ETestSuite struct {
	// Core context and configuration
	ctx       context.Context
	testUser  *types.User
	serverURL string
	logger    zerolog.Logger

	// Helix infrastructure
	store          store.Store
	oauth          *oauth.Manager
	client         *client.HelixClient
	keycloak       *auth.KeycloakAuthenticator
	helixAPIServer *server.HelixAPIServer
	testAPIKey     string

	// Browser automation
	browser           *rod.Browser
	screenshotCounter int
	testTimestamp     string

	// Test logging
	logFile *os.File

	// Test cleanup
	cleanupFuncs []func() error
}

// SetupBaseInfrastructure initializes the common Helix infrastructure
func (suite *BaseOAuthE2ETestSuite) SetupBaseInfrastructure(t TestingT) error {
	// Set up test logging
	err := suite.setupTestLogging(t)
	if err != nil {
		return fmt.Errorf("failed to setup test logging: %w", err)
	}

	suite.logger.Info().Msg("=== Setting up base OAuth E2E test infrastructure ===")

	// Load server config
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("failed to load server config: %w", err)
	}

	// Configure server for testing
	err = suite.configureServer(&cfg)
	if err != nil {
		return fmt.Errorf("failed to configure server: %w", err)
	}

	// Initialize store
	suite.store, err = store.NewPostgresStore(cfg.Store)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}

	// Initialize OAuth manager
	suite.oauth = oauth.NewManager(suite.store)

	// Set up Helix API server with all dependencies
	err = suite.setupHelixAPIServer(t, &cfg)
	if err != nil {
		return fmt.Errorf("failed to setup Helix API server: %w", err)
	}

	// Initialize browser for OAuth flow
	err = suite.setupBrowser()
	if err != nil {
		return fmt.Errorf("failed to setup browser: %w", err)
	}

	// Create test user and API key
	suite.testUser, err = suite.createTestUser()
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	apiKey, err := suite.createTestUserAPIKey()
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	// Initialize API client
	suite.client, err = client.NewClient(suite.serverURL, apiKey)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	suite.logger.Info().
		Str("user_id", suite.testUser.ID).
		Str("username", suite.testUser.Username).
		Str("server_url", suite.serverURL).
		Msg("Base OAuth E2E test infrastructure setup completed")

	return nil
}

// configureServer sets up server configuration for testing
func (suite *BaseOAuthE2ETestSuite) configureServer(cfg *config.ServerConfig) error {
	webServerHost := os.Getenv("WEB_SERVER_HOST")
	if webServerHost == "" {
		webServerHost = "localhost"
	}

	cfg.WebServer.URL = fmt.Sprintf("http://%s:8080", webServerHost)
	cfg.WebServer.Host = webServerHost
	cfg.WebServer.Port = 8080
	cfg.WebServer.RunnerToken = "test-runner-token"
	cfg.Tools.Enabled = true

	// Configure Anthropic API for real LLM calls
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		cfg.Providers.Anthropic.APIKey = apiKey
		suite.logger.Info().Msg("Using Anthropic API key from environment for real LLM calls")
	} else {
		suite.logger.Warn().Msg("ANTHROPIC_API_KEY not set - LLM calls may fail")
	}

	suite.serverURL = cfg.WebServer.URL
	return nil
}

// setupHelixAPIServer creates the full Helix API server with all dependencies
func (suite *BaseOAuthE2ETestSuite) setupHelixAPIServer(t TestingT, cfg *config.ServerConfig) error {
	ctrl := gomock.NewController(t)

	// Create mocks for non-essential components
	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)
	ragMock := rag.NewMockRAG(ctrl)
	gptScriptExecutor := gptscript.NewMockExecutor(ctrl)
	avatarsBucket := memblob.OpenBucket(nil)

	// Create PubSub
	ps, err := pubsub.New(&config.ServerConfig{
		PubSub: config.PubSub{
			StoreDir: t.TempDir(),
			Provider: string(pubsub.ProviderMemory),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create pubsub: %w", err)
	}

	// Create provider manager for LLM calls
	providerManager := manager.NewProviderManager(cfg, suite.store, nil)

	// Create scheduler
	runnerController, err := scheduler.NewRunnerController(suite.ctx, &scheduler.RunnerControllerConfig{
		PubSub: ps,
		FS:     filestoreMock,
	})
	if err != nil {
		return fmt.Errorf("failed to create runner controller: %w", err)
	}

	sched, err := scheduler.NewScheduler(suite.ctx, cfg, &scheduler.Params{
		RunnerController: runnerController,
	})
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Create controller
	controller, err := controller.NewController(suite.ctx, controller.Options{
		Config:          cfg,
		Store:           suite.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		ProviderManager: providerManager,
		Filestore:       filestoreMock,
		Extractor:       extractorMock,
		RAG:             ragMock,
		Scheduler:       sched,
		PubSub:          ps,
		OAuthManager:    suite.oauth,
	})
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Create Keycloak authenticator
	keycloakConfig := suite.configureKeycloak(cfg)
	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(keycloakConfig, suite.store)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak authenticator: %w", err)
	}
	suite.keycloak = keycloakAuthenticator

	// Create the full server
	helixAPIServer, err := server.NewServer(
		cfg,
		suite.store,
		ps,
		gptScriptExecutor,
		providerManager,
		nil, // inference server
		keycloakAuthenticator,
		nil, // stripe
		controller,
		janitor.NewJanitor(config.Janitor{}),
		nil, // knowledge manager
		sched,
		nil,         // ping service
		suite.oauth, // OAuth manager
		avatarsBucket,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create Helix API server: %w", err)
	}

	suite.helixAPIServer = helixAPIServer
	return nil
}

// configureKeycloak sets up Keycloak configuration for testing
func (suite *BaseOAuthE2ETestSuite) configureKeycloak(cfg *config.ServerConfig) *config.Keycloak {
	keycloakConfig := cfg.Keycloak

	if keycloakURL := os.Getenv("KEYCLOAK_URL"); keycloakURL != "" {
		keycloakConfig.KeycloakURL = keycloakURL
		keycloakConfig.KeycloakFrontEndURL = keycloakURL
		suite.logger.Info().Str("keycloak_url", keycloakURL).Msg("Using KEYCLOAK_URL from environment")
	} else {
		webServerHost := os.Getenv("WEB_SERVER_HOST")
		if webServerHost == "" {
			webServerHost = "localhost"
		}
		keycloakConfig.KeycloakURL = fmt.Sprintf("http://%s:8080/auth", webServerHost)
		keycloakConfig.KeycloakFrontEndURL = fmt.Sprintf("http://%s:8080/auth", webServerHost)
		suite.logger.Info().Msg("Using localhost Keycloak URL for local testing")
	}

	return &keycloakConfig
}

// setupTestLogging initializes test logging
func (suite *BaseOAuthE2ETestSuite) setupTestLogging(t TestingT) error {
	testResultsDir := getTestResultsDir()
	if err := os.MkdirAll(testResultsDir, 0755); err != nil {
		return fmt.Errorf("failed to create test results directory: %w", err)
	}

	suite.testTimestamp = time.Now().Format("20060102_150405")
	logFilename := filepath.Join(testResultsDir, fmt.Sprintf("oauth_e2e_%s.log", suite.testTimestamp))

	var err error
	suite.logFile, err = os.Create(logFilename)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	multiWriter := io.MultiWriter(suite.logFile, os.Stdout)
	suite.logger = zerolog.New(multiWriter).With().
		Timestamp().
		Str("test", "oauth_e2e").
		Logger()

	t.Logf("Test log file: %s", logFilename)
	return nil
}

// setupBrowser initializes browser for OAuth automation
func (suite *BaseOAuthE2ETestSuite) setupBrowser() error {
	suite.logger.Info().Msg("Setting up headless browser for OAuth automation")

	chromeURL := os.Getenv("RAG_CRAWLER_LAUNCHER_URL")
	if chromeURL == "" {
		chromeURL = "http://localhost:7317"
	}

	suite.browser = rod.New().
		ControlURL(chromeURL).
		Context(suite.ctx)

	err := suite.browser.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to Chrome container: %w", err)
	}

	suite.logger.Info().Msg("Browser setup completed successfully")
	return nil
}

// createTestUser creates a test user in both Keycloak and database
func (suite *BaseOAuthE2ETestSuite) createTestUser() (*types.User, error) {
	user := &types.User{
		ID:       fmt.Sprintf("test-user-%d", time.Now().Unix()),
		Email:    fmt.Sprintf("oauth-test-%d@helix.test", time.Now().Unix()),
		Username: fmt.Sprintf("oauth-test-%d", time.Now().Unix()),
		FullName: "OAuth Test User",
		Admin:    false,
	}

	// Create user in Keycloak first
	createdUser, err := suite.keycloak.CreateKeycloakUser(suite.ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user in Keycloak: %w", err)
	}

	user.ID = createdUser.ID

	// Create user in database
	createdUser, err = suite.store.CreateUser(suite.ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user in database: %w", err)
	}

	return createdUser, nil
}

// createTestUserAPIKey creates an API key for the test user
func (suite *BaseOAuthE2ETestSuite) createTestUserAPIKey() (string, error) {
	apiKey, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	_, err = suite.store.CreateAPIKey(suite.ctx, &types.ApiKey{
		Name:      "OAuth Test Key",
		Key:       apiKey,
		Owner:     suite.testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	suite.testAPIKey = apiKey
	return apiKey, nil
}

// AddCleanupFunc adds a cleanup function to be called during test cleanup
func (suite *BaseOAuthE2ETestSuite) AddCleanupFunc(cleanupFunc func() error) {
	suite.cleanupFuncs = append(suite.cleanupFuncs, cleanupFunc)
}

// Cleanup performs cleanup of test resources
func (suite *BaseOAuthE2ETestSuite) Cleanup() {
	suite.logger.Info().Msg("=== Starting base OAuth E2E test cleanup ===")

	// Close browser
	if suite.browser != nil {
		suite.browser.MustClose()
		suite.logger.Info().Msg("Browser closed")
	}

	// Run custom cleanup functions
	for _, cleanupFunc := range suite.cleanupFuncs {
		if err := cleanupFunc(); err != nil {
			suite.logger.Error().Err(err).Msg("Cleanup function failed")
		}
	}

	// Delete test user
	if suite.testUser != nil {
		err := suite.store.DeleteUser(suite.ctx, suite.testUser.ID)
		if err != nil {
			suite.logger.Error().Err(err).Msg("Failed to delete test user")
		} else {
			suite.logger.Info().Msg("Test user deleted")
		}
	}

	// Close log file
	if suite.logFile != nil {
		suite.logFile.Close()
	}

	suite.logger.Info().Msg("=== Base OAuth E2E test cleanup completed ===")
}

// GetTestResultsDir returns the test results directory
func (suite *BaseOAuthE2ETestSuite) GetTestResultsDir() string {
	return getTestResultsDir()
}

// getTestResultsDir returns a fixed test results directory
func getTestResultsDir() string {
	return "/tmp/helix-oauth-test-results"
}

// TestingT is a minimal interface for testing.T compatibility
type TestingT interface {
	Logf(format string, args ...interface{})
	TempDir() string
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
}
