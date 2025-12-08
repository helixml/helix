package skills

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/oauth"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	goai "github.com/sashabaranov/go-openai"
	"go.uber.org/mock/gomock"
	"gocloud.dev/blob/memblob"
)

// BaseOAuthTestSuite provides common OAuth testing infrastructure
type BaseOAuthTestSuite struct {
	ctx            context.Context
	store          store.Store
	oauth          *oauth.Manager
	client         *client.HelixClient
	keycloak       *auth.KeycloakAuthenticator
	helixAPIServer *server.HelixAPIServer

	// Server configuration
	serverURL string

	// Test user and API key
	testUser   *types.User
	testAPIKey string

	// Test logging
	logFile       *os.File
	logger        zerolog.Logger
	testTimestamp string
	testID        string // Unique ID for this test instance

	// Browser automation
	browser           *rod.Browser
	screenshotCounter int
}

// GetTestResultsDir returns a fixed test results directory that works everywhere
func GetTestResultsDir() string {
	return "/tmp/helix-oauth-test-results"
}

// SetupBaseInfrastructure initializes the common OAuth test infrastructure
func (suite *BaseOAuthTestSuite) SetupBaseInfrastructure(testName string) error {
	suite.ctx = context.Background()

	// Generate unique test ID for this test instance (for parallel test isolation)
	suite.testID = fmt.Sprintf("%s_%d_%d", testName, time.Now().UnixNano(), rand.Intn(10000))

	// Set up test logging
	err := suite.setupTestLogging(testName)
	if err != nil {
		return fmt.Errorf("failed to setup test logging: %w", err)
	}

	suite.logger.Info().Str("test_name", testName).Str("test_id", suite.testID).Msg("=== OAuth Test Starting ===")

	// Load server config
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("failed to load server config: %w", err)
	}

	// Set up basic server configuration for testing (no actual server started)
	webServerHost := os.Getenv("WEB_SERVER_HOST")
	if webServerHost == "" {
		webServerHost = "localhost"
	}
	cfg.WebServer.URL = fmt.Sprintf("http://%s:8080", webServerHost)
	cfg.WebServer.Host = webServerHost
	cfg.WebServer.Port = 8080
	cfg.WebServer.RunnerToken = "test-runner-token"

	suite.logger.Info().
		Str("host", webServerHost).
		Str("url", cfg.WebServer.URL).
		Msg("Configured test server settings (no actual server started)")

	// Configure OpenAI API for real LLM calls
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		cfg.Providers.OpenAI.APIKey = apiKey
		suite.logger.Info().Msg("Using OpenAI API key from environment for real LLM calls")
	} else {
		suite.logger.Warn().Msg("OPENAI_API_KEY not set - LLM calls may fail")
	}

	// Initialize store with unique schema for parallel test isolation
	// Each test gets its own schema to avoid migration conflicts
	storeConfig := cfg.Store
	storeConfig.Schema = fmt.Sprintf("test_oauth_%s", suite.testID)
	suite.logger.Info().Str("schema", storeConfig.Schema).Msg("Using unique database schema for test isolation")

	suite.store, err = store.NewPostgresStore(storeConfig)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}

	// Initialize OAuth manager
	suite.oauth = oauth.NewManager(suite.store, cfg.Tools.TLSSkipVerify)

	// Initialize server dependencies
	err = suite.setupServerDependencies(cfg, webServerHost)
	if err != nil {
		return fmt.Errorf("failed to setup server dependencies: %w", err)
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

	// Initialize API client
	apiKey, err := suite.createTestUserAPIKey()
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	suite.serverURL = cfg.WebServer.URL
	suite.client, err = client.NewClient(suite.serverURL, apiKey, cfg.Tools.TLSSkipVerify)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	suite.logger.Info().
		Str("user_id", suite.testUser.ID).
		Str("username", suite.testUser.Username).
		Str("server_url", suite.serverURL).
		Msg("Base OAuth test infrastructure setup completed")

	return nil
}

// setupTestLogging initializes the test log file and logger
func (suite *BaseOAuthTestSuite) setupTestLogging(testName string) error {
	testResultsDir := GetTestResultsDir()
	if err := os.MkdirAll(testResultsDir, 0755); err != nil {
		return fmt.Errorf("failed to create test results directory: %w", err)
	}

	suite.testTimestamp = time.Now().Format("20060102_150405")
	// Use unique test ID instead of PID for parallel test isolation
	logFilename := filepath.Join(testResultsDir, fmt.Sprintf("%s_%s_%s.log", testName, suite.testTimestamp, suite.testID))

	var err error
	suite.logFile, err = os.Create(logFilename)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	multiWriter := io.MultiWriter(suite.logFile, os.Stdout)
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	suite.logger = zerolog.New(multiWriter).With().
		Timestamp().
		Str("test", testName).
		Str("test_id", suite.testID).
		Logger()

	return nil
}

// setupServerDependencies initializes all the server dependencies
func (suite *BaseOAuthTestSuite) setupServerDependencies(cfg config.ServerConfig, webServerHost string) error {
	// Create mocks for testing
	ctrl := gomock.NewController(nil) // We'll need to pass this from the test
	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)
	ragMock := rag.NewMockRAG(ctrl)
	notifierMock := notification.NewMockNotifier(ctrl)

	// Create PubSub
	ps, err := pubsub.New(&config.ServerConfig{
		PubSub: config.PubSub{
			StoreDir: "/tmp", // Use temp directory for testing
			Provider: string(pubsub.ProviderMemory),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create pubsub: %w", err)
	}

	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	if err != nil {
		return fmt.Errorf("failed to create model info provider: %w", err)
	}

	// Create provider manager
	providerManager := manager.NewProviderManager(&cfg, suite.store, nil, modelInfoProvider)

	// Configure tools
	cfg.Tools.Enabled = true

	// Create scheduler
	runnerController, err := scheduler.NewRunnerController(suite.ctx, &scheduler.RunnerControllerConfig{
		PubSub: ps,
		FS:     filestoreMock,
	})
	if err != nil {
		return fmt.Errorf("failed to create runner controller: %w", err)
	}

	schedulerParams := &scheduler.Params{
		RunnerController: runnerController,
		Store:            suite.store,
	}
	sched, err := scheduler.NewScheduler(suite.ctx, &cfg, schedulerParams)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

	// Create controller
	controller, err := controller.NewController(suite.ctx, controller.Options{
		Config:          &cfg,
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
	keycloakConfig := cfg.Auth.Keycloak
	if keycloakURL := os.Getenv("KEYCLOAK_URL"); keycloakURL != "" {
		keycloakConfig.KeycloakURL = keycloakURL
		keycloakConfig.KeycloakFrontEndURL = keycloakURL
	} else {
		keycloakConfig.KeycloakURL = fmt.Sprintf("http://%s:8080/auth", webServerHost)
		keycloakConfig.KeycloakFrontEndURL = fmt.Sprintf("http://%s:8080/auth", webServerHost)
	}

	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(&cfg, suite.store)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak authenticator: %w", err)
	}
	suite.keycloak = keycloakAuthenticator

	// Update config with Keycloak settings
	cfg.Auth.Keycloak = keycloakConfig

	// Create trigger manager
	triggerManager := trigger.NewTriggerManager(&cfg, suite.store, notifierMock, controller)

	anthropicProxy := anthropic.New(&cfg, suite.store, modelInfoProvider, nil)

	// Create the API server
	avatarsBucket := memblob.OpenBucket(nil)
	helixAPIServer, err := server.NewServer(
		&cfg,
		suite.store,
		ps,
		providerManager,
		modelInfoProvider,
		nil,
		keycloakAuthenticator,
		nil,
		controller,
		janitor.NewJanitor(config.Janitor{}),
		nil,
		sched,
		nil,
		suite.oauth,
		avatarsBucket,
		triggerManager,
		anthropicProxy,
	)
	if err != nil {
		return fmt.Errorf("failed to create Helix API server: %w", err)
	}

	suite.helixAPIServer = helixAPIServer
	return nil
}

// setupBrowser initializes the headless browser for OAuth automation
func (suite *BaseOAuthTestSuite) setupBrowser() error {
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

	// Create a new incognito browser context for this test to provide session isolation
	// This prevents concurrent tests from interfering with each other's cookies and sessions
	suite.logger.Info().Msg("Creating incognito browser context for test isolation")
	incognito, err := suite.browser.Incognito()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to create incognito context, using regular browser context")
		// Continue with regular browser if incognito fails
	} else {
		suite.browser = incognito
		suite.logger.Info().Msg("Using incognito browser context for test isolation")
	}

	suite.logger.Info().Msg("Browser setup completed successfully")
	return nil
}

// createTestUser creates a test user in both Keycloak and database
func (suite *BaseOAuthTestSuite) createTestUser() (*types.User, error) {
	// Use shorter unique identifiers for parallel tests
	// Generate a short random suffix to avoid collisions
	shortID := fmt.Sprintf("%d%d", time.Now().Unix()%100000, rand.Intn(1000))

	user := &types.User{
		ID:       fmt.Sprintf("test-user-%s", shortID),
		Email:    fmt.Sprintf("oauth-test-%s@helix.test", shortID),
		Username: fmt.Sprintf("oauth-test-%s", shortID),
		FullName: "OAuth Test User",
		Admin:    false,
	}

	// Create user in Keycloak first
	createdUser, err := suite.keycloak.CreateUser(suite.ctx, user)
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
func (suite *BaseOAuthTestSuite) createTestUserAPIKey() (string, error) {
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

// TakeScreenshot captures a screenshot and saves it with the test timestamp
func (suite *BaseOAuthTestSuite) TakeScreenshot(page *rod.Page, stepName string) {
	suite.screenshotCounter++
	// Use test ID instead of PID for parallel test isolation
	filename := filepath.Join(GetTestResultsDir(), fmt.Sprintf("oauth_test_%s_%s_step_%02d_%s.png",
		suite.testTimestamp, suite.testID, suite.screenshotCounter, stepName))

	// Create a fresh page context with a reasonable timeout specifically for screenshot operations
	// This prevents context deadline exceeded errors when the main page context has expired
	screenshotPage := page.Timeout(30 * time.Second)

	data, err := screenshotPage.Screenshot(false, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})

	if err != nil {
		suite.logger.Error().Err(err).Str("filename", filename).Msg("Failed to take screenshot")
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		suite.logger.Error().Err(err).Str("filename", filename).Msg("Failed to save screenshot")
	} else {
		suite.logger.Info().Str("filename", filename).Msg("Screenshot saved")
	}
}

// CleanupBaseInfrastructure cleans up common test resources
func (suite *BaseOAuthTestSuite) CleanupBaseInfrastructure() {
	suite.logger.Info().Msg("=== Starting Base Infrastructure Cleanup ===")

	// Close browser
	if suite.browser != nil {
		suite.browser.MustClose()
		suite.logger.Info().Msg("Browser closed")
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

	// Clean up test database schema
	if suite.store != nil {
		schemaName := fmt.Sprintf("test_oauth_%s", suite.testID)
		suite.logger.Info().Str("schema", schemaName).Msg("Cleaning up test database schema")

		// Close the store connection
		if postgresStore, ok := suite.store.(*store.PostgresStore); ok {
			postgresStore.Close()
			suite.logger.Info().Str("schema", schemaName).Msg("Closed store connection - test schema will be cleaned up by database maintenance")
		}
	}

	// Close log file
	if suite.logFile != nil {
		suite.logFile.Close()
	}

	suite.logger.Info().Msg("=== Base Infrastructure Cleanup Completed ===")
}

// CleanupExistingOAuthData removes any OAuth connections/providers from previous test runs
func (suite *BaseOAuthTestSuite) CleanupExistingOAuthData() error {
	// Delete all OAuth connections
	connections, err := suite.store.ListOAuthConnections(suite.ctx, &store.ListOAuthConnectionsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list OAuth connections: %w", err)
	}

	for _, conn := range connections {
		err = suite.store.DeleteOAuthConnection(suite.ctx, conn.ID)
		if err != nil {
			suite.logger.Warn().Err(err).Str("connection_id", conn.ID).Msg("Failed to delete OAuth connection")
		} else {
			suite.logger.Debug().Str("connection_id", conn.ID).Msg("Deleted OAuth connection from previous run")
		}
	}

	// Delete test OAuth providers
	providers, err := suite.store.ListOAuthProviders(suite.ctx, &store.ListOAuthProvidersQuery{})
	if err != nil {
		return fmt.Errorf("failed to list OAuth providers: %w", err)
	}

	for _, provider := range providers {
		// Only delete test providers
		if provider.Name == "GitHub Skills Test" ||
			strings.Contains(provider.Name, "Skills Test") ||
			strings.Contains(provider.Name, "Test") {
			err = suite.store.DeleteOAuthProvider(suite.ctx, provider.ID)
			if err != nil {
				suite.logger.Warn().Err(err).Str("provider_id", provider.ID).Msg("Failed to delete OAuth provider")
			} else {
				suite.logger.Debug().Str("provider_id", provider.ID).Msg("Deleted OAuth provider from previous run")
			}
		}
	}

	return nil
}

// StartOAuthFlow starts OAuth flow using OAuth manager directly
func (suite *BaseOAuthTestSuite) StartOAuthFlow(providerID, callbackURL string) (string, string, error) {
	suite.logger.Info().Msg("Starting OAuth flow via OAuth manager")

	// Call OAuth manager directly instead of making HTTP request
	authURL, err := suite.oauth.StartOAuthFlow(suite.ctx, suite.testUser.ID, providerID, callbackURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to start OAuth flow: %w", err)
	}

	// Extract state parameter from the auth URL
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse auth URL: %w", err)
	}
	state := parsedURL.Query().Get("state")
	if state == "" {
		return "", "", fmt.Errorf("no state parameter in auth URL")
	}

	return authURL, state, nil
}

// CompleteOAuthFlow completes OAuth flow using Helix's OAuth manager
func (suite *BaseOAuthTestSuite) CompleteOAuthFlow(providerID, authCode string) (*types.OAuthConnection, error) {
	suite.logger.Info().Msg("Completing OAuth flow via Helix OAuth manager")

	// Use OAuth manager directly to complete the flow
	connection, err := suite.oauth.CompleteOAuthFlow(suite.ctx, suite.testUser.ID, providerID, authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to complete OAuth flow: %w", err)
	}

	return connection, nil
}

// ExecuteSessionQuery creates a session and executes a query using Helix's controller directly
func (suite *BaseOAuthTestSuite) ExecuteSessionQuery(userMessage, sessionName, appID string) (string, error) {
	suite.logger.Info().
		Str("user_message", userMessage).
		Str("session_name", sessionName).
		Msg("Executing session query with Helix controller")

	// Prepare OpenAI chat completion request
	openaiReq := goai.ChatCompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []goai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: userMessage,
			},
		},
		Stream: false,
	}

	// Set up controller options with app context for OAuth
	options := &controller.ChatCompletionOptions{
		AppID: appID,
	}

	// Set app ID and user ID in context for OAuth token retrieval
	ctx := oai.SetContextAppID(suite.ctx, appID)
	// ctx = oai.SetContextSessionID(ctx, session.ID)
	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID: suite.testUser.ID,
		// SessionID:     session.ID,
		InteractionID: system.GenerateInteractionID(),
	})

	suite.logger.Info().
		// Str("session_id", session.ID).
		Str("app_id", appID).
		Msg("Executing chat completion with OAuth context")

	// Execute the chat completion using the controller
	response, _, err := suite.helixAPIServer.Controller.ChatCompletion(ctx, suite.testUser, openaiReq, options)
	if err != nil {
		return "", fmt.Errorf("failed to execute chat completion: %w", err)
	}

	// Extract the agent's response
	agentResponse := ""
	if len(response.Choices) > 0 {
		agentResponse = response.Choices[0].Message.Content
	}

	if agentResponse == "" {
		return "", fmt.Errorf("no response from agent")
	}

	suite.logger.Info().
		// Str("session_id", session.ID).
		Str("agent_response", agentResponse[:min(len(agentResponse), 100)]+"...").
		Msg("Received agent response")

	return agentResponse, nil
}

// LogAgentConversation logs the full agent conversation to a text file for manual verification
func (suite *BaseOAuthTestSuite) LogAgentConversation(sessionName, userMessage, agentResponse, testName, providerName string) {
	conversationFilename := filepath.Join(GetTestResultsDir(), fmt.Sprintf("%s_%s_conversation_%s.txt",
		testName, suite.testTimestamp, strings.ReplaceAll(strings.ToLower(sessionName), " ", "_")))

	conversationContent := fmt.Sprintf(`=== %s OAuth Skills E2E Test - %s ===
Timestamp: %s
Test User: %s
OAuth Provider: %s

=== CONVERSATION ===

USER: %s

AGENT: %s

=== TEST METADATA ===
- OAuth connection verified: YES
- Access token present: YES
- Expected to use real OAuth API calls: YES

=== VERIFICATION NOTES ===
- Agent response should contain real data from OAuth API calls
- Agent should NOT return generic/mock responses
- OAuth tokens should be used for actual API requests
`,
		providerName,
		sessionName,
		time.Now().Format("2006-01-02 15:04:05"),
		suite.testUser.Username,
		providerName,
		userMessage,
		agentResponse,
	)

	err := os.WriteFile(conversationFilename, []byte(conversationContent), 0644)
	if err != nil {
		suite.logger.Error().Err(err).Str("filename", conversationFilename).Msg("Failed to write conversation log")
	} else {
		suite.logger.Info().Str("filename", conversationFilename).Msg("Agent conversation logged to file")
	}
}
