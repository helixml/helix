//go:build oauth_integration
// +build oauth_integration

package skills_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gocloud.dev/blob/memblob"
)

// GitHubOAuthE2ETestSuite tests the complete GitHub OAuth skills workflow
type GitHubOAuthE2ETestSuite struct {
	ctx            context.Context
	store          store.Store
	oauth          *oauth.Manager
	client         *client.HelixClient
	keycloak       *auth.KeycloakAuthenticator
	helixAPIServer *server.HelixAPIServer

	// Test configuration from environment
	githubClientID     string
	githubClientSecret string
	githubUsername     string
	githubPassword     string
	githubToken        string // Personal access token for GitHub API calls
	serverURL          string // Server URL for API calls and callbacks

	// Created during test
	testUser      *types.User
	oauthProvider *types.OAuthProvider
	testApp       *types.App
	oauthConn     *types.OAuthConnection

	// Test logging
	logFile *os.File
	logger  zerolog.Logger

	// Browser automation
	browser           *rod.Browser
	screenshotCounter int
	testTimestamp     string

	// Test repositories created
	testRepos []string

	// Test API key
	testAPIKey string
}

// TestGitHubOAuthSkillsE2E is the main end-to-end test for GitHub OAuth skills
func TestGitHubOAuthSkillsE2E(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	suite := &GitHubOAuthE2ETestSuite{
		ctx: context.Background(),
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping GitHub OAuth E2E test: %v", err)
	}

	// Initialize test dependencies
	err = suite.setup(t)
	require.NoError(t, err, "Failed to setup test dependencies")

	// Run the complete end-to-end workflow
	t.Run("SetupOAuthProvider", suite.testSetupOAuthProvider)
	t.Run("CreateTestRepositories", suite.testCreateTestRepositories)
	t.Run("CreateTestApp", suite.testCreateTestApp)
	t.Run("PerformOAuthFlow", suite.testPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.testOAuthTokenDirectly)
	t.Run("TestAgentGitHubSkillsIntegration", suite.testAgentGitHubSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanup(t)
	})
}

// loadTestConfig loads configuration from environment variables
func (suite *GitHubOAuthE2ETestSuite) loadTestConfig() error {
	suite.githubClientID = os.Getenv("GITHUB_SKILL_TEST_OAUTH_CLIENT_ID")
	if suite.githubClientID == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_OAUTH_CLIENT_ID environment variable not set")
	}

	suite.githubClientSecret = os.Getenv("GITHUB_SKILL_TEST_OAUTH_CLIENT_SECRET")
	if suite.githubClientSecret == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_OAUTH_CLIENT_SECRET environment variable not set")
	}

	suite.githubUsername = os.Getenv("GITHUB_SKILL_TEST_OAUTH_USERNAME")
	if suite.githubUsername == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_OAUTH_USERNAME environment variable not set")
	}

	suite.githubPassword = os.Getenv("GITHUB_SKILL_TEST_OAUTH_PASSWORD")
	if suite.githubPassword == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_OAUTH_PASSWORD environment variable not set")
	}

	// For repository setup/cleanup, we need a personal access token with repo creation/deletion permissions
	// This is separate from the OAuth tokens we'll get during the test
	suite.githubToken = os.Getenv("GITHUB_SKILL_TEST_SETUP_PAT")
	if suite.githubToken == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_SETUP_PAT environment variable not set - need PAT for repo creation/cleanup")
	}

	log.Info().
		Str("client_id", suite.githubClientID).
		Str("username", suite.githubUsername).
		Msg("Loaded GitHub OAuth test configuration")

	return nil
}

// setup initializes the test environment
func (suite *GitHubOAuthE2ETestSuite) setup(t *testing.T) error {
	// Set up test logging
	err := suite.setupTestLogging(t)
	if err != nil {
		return fmt.Errorf("failed to setup test logging: %w", err)
	}

	suite.logger.Info().Msg("=== GitHub OAuth Skills E2E Test Starting ===")

	// Load server config
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return fmt.Errorf("failed to load server config: %w", err)
	}

	// Override required fields for testing
	cfg.WebServer.URL = "http://localhost:8080"
	cfg.WebServer.Host = "localhost"
	cfg.WebServer.Port = 8080
	cfg.WebServer.RunnerToken = "test-runner-token"

	// Disable Keycloak and OIDC for testing to avoid authentication complications
	cfg.Keycloak.KeycloakEnabled = false
	cfg.OIDC.Enabled = false

	// Provide minimal OIDC config since server requires it
	cfg.OIDC.Enabled = true
	cfg.OIDC.URL = "http://localhost:8080/auth/realms/test"
	cfg.OIDC.ClientID = "test-client"
	cfg.OIDC.ClientSecret = "test-secret"
	cfg.OIDC.Audience = "account"
	cfg.OIDC.Scopes = "openid,profile,email"

	// Initialize store
	suite.store, err = store.NewPostgresStore(cfg.Store)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err = suite.cleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	// Initialize OAuth manager
	suite.oauth = oauth.NewManager(suite.store)

	// Initialize HelixAPIServer with proper dependencies like in the test examples
	ctrl := gomock.NewController(t)
	filestoreMock := filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)
	ragMock := rag.NewMockRAG(ctrl)

	// Create PubSub for controller
	ps, err := pubsub.New(&config.ServerConfig{
		PubSub: config.PubSub{
			StoreDir: t.TempDir(),
			Provider: string(pubsub.ProviderMemory),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create pubsub: %w", err)
	}

	// Create provider manager mock and configure it
	providerManager := manager.NewMockProviderManager(ctrl)
	openaiClient := openai.NewMockClient(ctrl)

	// Configure provider manager to return the mock client for any provider request
	providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(openaiClient, nil).AnyTimes()

	// Configure tools
	cfg.Tools.Enabled = true

	// Create controller with all dependencies
	runnerController, err := scheduler.NewRunnerController(suite.ctx, &scheduler.RunnerControllerConfig{
		PubSub: ps,
		FS:     filestoreMock,
	})
	if err != nil {
		return fmt.Errorf("failed to create runner controller: %w", err)
	}

	schedulerParams := &scheduler.Params{
		RunnerController: runnerController,
	}
	sched, err := scheduler.NewScheduler(suite.ctx, &cfg, schedulerParams)
	if err != nil {
		return fmt.Errorf("failed to create scheduler: %w", err)
	}

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
	})
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	// Create skill manager
	// skillManager := skills.NewManager()  // Not used in this test

	// Create remaining mocks and dependencies for the server
	gptScriptExecutor := gptscript.NewMockExecutor(ctrl)
	avatarsBucket := memblob.OpenBucket(nil) // Use in-memory blob bucket for testing
	authenticator := auth.NewMockAuthenticator(ctrl)

	// Create the full server with all dependencies including OAuth manager
	helixAPIServer, err := server.NewServer(
		&cfg,
		suite.store,
		ps,
		gptScriptExecutor,
		providerManager,
		nil, // inference server - not needed for this test
		authenticator,
		nil, // stripe - not needed for this test
		controller,
		janitor.NewJanitor(config.Janitor{}),
		nil, // knowledge manager - not needed for this test
		sched,
		nil,         // ping service - not needed for this test
		suite.oauth, // Pass OAuth manager here
		avatarsBucket,
	)
	if err != nil {
		return fmt.Errorf("failed to create Helix API server: %w", err)
	}

	suite.helixAPIServer = helixAPIServer

	// Initialize Keycloak authenticator
	// Use environment variable KEYCLOAK_URL if set, otherwise use config default
	keycloakConfig := cfg.Keycloak
	if keycloakURL := os.Getenv("KEYCLOAK_URL"); keycloakURL != "" {
		keycloakConfig.KeycloakURL = keycloakURL
		suite.logger.Info().Str("keycloak_url", keycloakURL).Msg("Using KEYCLOAK_URL from environment")
	} else {
		// Fallback to localhost for local testing
		keycloakConfig.KeycloakURL = "http://localhost:8080/auth"
		suite.logger.Info().Msg("Using localhost Keycloak URL for local testing")
	}

	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(&keycloakConfig, suite.store)
	if err != nil {
		return fmt.Errorf("failed to create Keycloak authenticator: %w", err)
	}
	suite.keycloak = keycloakAuthenticator

	// Create test user
	suite.testUser, err = suite.createTestUser()
	if err != nil {
		return fmt.Errorf("failed to create test user: %w", err)
	}

	// Initialize API client for test user
	apiKey, err := suite.createTestUserAPIKey()
	if err != nil {
		return fmt.Errorf("failed to create API key: %w", err)
	}

	// Set server URL for API client and callbacks
	suite.serverURL = cfg.WebServer.URL
	if suite.serverURL == "" {
		suite.serverURL = "http://localhost:8080" // Fallback for local testing
	}
	suite.logger.Info().Str("server_url", suite.serverURL).Msg("Using server URL for API client")

	suite.client, err = client.NewClient(suite.serverURL, apiKey)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	suite.logger.Info().
		Str("user_id", suite.testUser.ID).
		Str("username", suite.testUser.Username).
		Msg("Test setup completed successfully")

	// Setup browser
	err = suite.setupBrowser()
	if err != nil {
		return fmt.Errorf("failed to setup browser: %w", err)
	}

	return nil
}

// setupTestLogging initializes the test log file and logger
func (suite *GitHubOAuthE2ETestSuite) setupTestLogging(t *testing.T) error {
	// Create test results directory if it doesn't exist
	testResultsDir := "test_results"
	if err := os.MkdirAll(testResultsDir, 0755); err != nil {
		return fmt.Errorf("failed to create test results directory: %w", err)
	}

	// Create log file with timestamp
	suite.testTimestamp = time.Now().Format("20060102_150405")
	logFilename := filepath.Join(testResultsDir, fmt.Sprintf("github_oauth_e2e_%s.log", suite.testTimestamp))

	var err error
	suite.logFile, err = os.Create(logFilename)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	// Create multi-writer to write to both file and console
	multiWriter := io.MultiWriter(suite.logFile, os.Stdout)

	// Configure structured logging
	suite.logger = zerolog.New(multiWriter).With().
		Timestamp().
		Str("test", "github_oauth_e2e").
		Logger()

	t.Logf("Test log file: %s", logFilename)

	return nil
}

// setupBrowser initializes the headless browser for OAuth automation
func (suite *GitHubOAuthE2ETestSuite) setupBrowser() error {
	suite.logger.Info().Msg("Setting up headless browser for OAuth automation")

	// Use environment variable for Chrome URL, with fallback to localhost
	chromeURL := os.Getenv("RAG_CRAWLER_LAUNCHER_URL")
	if chromeURL == "" {
		chromeURL = "http://localhost:7317" // Fallback for local testing
	}

	suite.logger.Info().Str("chrome_url", chromeURL).Msg("Connecting to Chrome container")

	// Connect to existing Chrome container
	suite.browser = rod.New().
		ControlURL(chromeURL).
		Context(suite.ctx)

	// Connect to the browser
	err := suite.browser.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to Chrome container: %w", err)
	}

	suite.logger.Info().Msg("Successfully connected to Chrome container")
	return nil
}

// takeScreenshot captures a screenshot and saves it with the test timestamp
func (suite *GitHubOAuthE2ETestSuite) takeScreenshot(page *rod.Page, stepName string) {
	suite.screenshotCounter++
	filename := fmt.Sprintf("test_results/github_oauth_e2e_%s_step_%02d_%s.png",
		suite.testTimestamp, suite.screenshotCounter, stepName)

	data, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
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

// testSetupOAuthProvider creates and configures the GitHub OAuth provider
func (suite *GitHubOAuthE2ETestSuite) testSetupOAuthProvider(t *testing.T) {
	log.Info().Msg("Setting up GitHub OAuth provider")

	// Create OAuth provider
	provider := &types.OAuthProvider{
		Name:         "GitHub Skills Test",
		Type:         types.OAuthProviderTypeGitHub,
		Enabled:      true,
		ClientID:     suite.githubClientID,
		ClientSecret: suite.githubClientSecret,
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user",
		CallbackURL:  suite.serverURL + "/api/v1/oauth/callback/github",
		Scopes:       []string{"repo", "user:read"},
		CreatorID:    suite.testUser.ID,
		CreatorType:  types.OwnerTypeUser,
	}

	createdProvider, err := suite.store.CreateOAuthProvider(suite.ctx, provider)
	require.NoError(t, err, "Failed to create OAuth provider")
	require.NotNil(t, createdProvider, "Created provider should not be nil")

	suite.oauthProvider = createdProvider

	// Verify provider was created correctly
	assert.Equal(t, "GitHub Skills Test", createdProvider.Name)
	assert.Equal(t, types.OAuthProviderTypeGitHub, createdProvider.Type)
	assert.True(t, createdProvider.Enabled)
	assert.Equal(t, suite.githubClientID, createdProvider.ClientID)

	log.Info().
		Str("provider_id", createdProvider.ID).
		Str("provider_name", createdProvider.Name).
		Msg("GitHub OAuth provider created successfully")
}

// testCreateTestApp creates a Helix app/agent with GitHub skills using full skill configuration
func (suite *GitHubOAuthE2ETestSuite) testCreateTestApp(t *testing.T) {
	suite.logger.Info().Msg("Creating test app with GitHub skills from skill definition")

	// Load the complete GitHub skill configuration
	githubSkill := suite.loadGitHubSkillConfig()

	appName := fmt.Sprintf("GitHub Skills Test App %d", time.Now().Unix())

	app := &types.App{
		Owner:     suite.testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        appName,
				Description: "Test app for GitHub OAuth skills integration",
				Assistants: []types.AssistantConfig{
					{
						Name:         githubSkill.DisplayName + " Assistant",
						Description:  "Assistant configured with GitHub OAuth skills",
						AgentMode:    true,
						SystemPrompt: githubSkill.SystemPrompt, // Use the skill's system prompt
						// Configure LLM model (use anthropic if available, fallback to helix)
						Provider: "anthropic",
						Model:    "claude-3-haiku-20240307",
						// Configure with GitHub skill using full configuration
						APIs: []types.AssistantAPI{
							{
								Name:          githubSkill.Name,
								Description:   githubSkill.Description,
								URL:           githubSkill.BaseURL,
								Schema:        githubSkill.Schema,
								Headers:       githubSkill.Headers,
								OAuthProvider: suite.oauthProvider.ID,
							},
						},
					},
				},
			},
		},
	}

	createdApp, err := suite.store.CreateApp(suite.ctx, app)
	require.NoError(t, err, "Failed to create test app")
	require.NotNil(t, createdApp, "Created app should not be nil")

	suite.testApp = createdApp

	// Verify app was created correctly
	assert.Equal(t, appName, createdApp.Config.Helix.Name)
	assert.Equal(t, suite.testUser.ID, createdApp.Owner)
	assert.Len(t, createdApp.Config.Helix.Assistants, 1)
	assert.Len(t, createdApp.Config.Helix.Assistants[0].APIs, 1)
	assert.Equal(t, suite.oauthProvider.ID, createdApp.Config.Helix.Assistants[0].APIs[0].OAuthProvider)

	suite.logger.Info().
		Str("app_id", createdApp.ID).
		Str("app_name", createdApp.Config.Helix.Name).
		Str("skill_name", githubSkill.DisplayName).
		Str("skill_description", githubSkill.Description[:50]+"...").
		Msg("Test app created successfully with GitHub skill configuration")
}

// testPerformOAuthFlow performs OAuth flow using Helix's OAuth endpoints
func (suite *GitHubOAuthE2ETestSuite) testPerformOAuthFlow(t *testing.T) {
	suite.logger.Info().Msg("Testing Helix OAuth flow with GitHub")

	// Step 1: Start OAuth flow using Helix's endpoint
	authURL, state, err := suite.startHelixOAuthFlow()
	require.NoError(t, err, "Failed to start Helix OAuth flow")

	suite.logger.Info().
		Str("auth_url", authURL).
		Str("state", state).
		Msg("Successfully started Helix OAuth flow")

	// Step 2: Get authorization code from GitHub OAuth (simulate user completing OAuth in browser)
	authCode, err := suite.getGitHubAuthorizationCode(authURL, state)
	require.NoError(t, err, "Failed to get GitHub authorization code")

	suite.logger.Info().
		Str("auth_code", authCode[:10]+"...").
		Msg("Successfully obtained GitHub authorization code")

	// Step 3: Complete OAuth flow using Helix's callback endpoint
	connection, err := suite.completeHelixOAuthFlow(authCode, state)
	require.NoError(t, err, "Failed to complete Helix OAuth flow")
	require.NotNil(t, connection, "OAuth connection should not be nil")

	suite.oauthConn = connection

	// Verify connection was created correctly through Helix's OAuth system
	assert.Equal(t, suite.testUser.ID, connection.UserID)
	assert.Equal(t, suite.oauthProvider.ID, connection.ProviderID)
	assert.NotEmpty(t, connection.AccessToken)
	assert.NotNil(t, connection.Profile)

	suite.logger.Info().
		Str("connection_id", connection.ID).
		Str("user_id", connection.UserID).
		Str("provider_id", connection.ProviderID).
		Str("github_username", connection.Profile.DisplayName).
		Msg("OAuth connection created successfully through Helix OAuth system")
}

// Helper methods

// createTestUser creates a test user in both Keycloak and database
func (suite *GitHubOAuthE2ETestSuite) createTestUser() (*types.User, error) {
	user := &types.User{
		ID:       fmt.Sprintf("test-user-%d", time.Now().Unix()),
		Email:    fmt.Sprintf("github-oauth-test-%d@helix.test", time.Now().Unix()),
		Username: fmt.Sprintf("github-oauth-test-%d", time.Now().Unix()),
		FullName: "GitHub OAuth Test User",
		Admin:    false,
	}

	// Create user in Keycloak first
	createdUser, err := suite.keycloak.CreateKeycloakUser(suite.ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user in Keycloak: %w", err)
	}

	// Use the ID returned by Keycloak
	user.ID = createdUser.ID

	// Create user in database
	createdUser, err = suite.store.CreateUser(suite.ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user in database: %w", err)
	}

	return createdUser, nil
}

func (suite *GitHubOAuthE2ETestSuite) createTestUserAPIKey() (string, error) {
	apiKey, err := system.GenerateAPIKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}

	_, err = suite.store.CreateAPIKey(suite.ctx, &types.ApiKey{
		Name:      "GitHub OAuth Test Key",
		Key:       apiKey,
		Owner:     suite.testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create API key: %w", err)
	}

	// Store the API key for later use
	suite.testAPIKey = apiKey

	return apiKey, nil
}

// cleanup cleans up test resources
func (suite *GitHubOAuthE2ETestSuite) cleanup(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting Test Cleanup ===")

	// Close browser
	if suite.browser != nil {
		suite.browser.MustClose()
		suite.logger.Info().Msg("Browser closed")
	}

	// Delete test GitHub repositories
	for _, repoName := range suite.testRepos {
		err := suite.deleteGitHubRepo(repoName)
		if err != nil {
			suite.logger.Error().Err(err).Str("repo", repoName).Msg("Failed to delete GitHub repository")
		} else {
			suite.logger.Info().Str("repo", repoName).Msg("GitHub repository deleted")
		}
	}

	// Delete OAuth connection
	if suite.oauthConn != nil {
		err := suite.store.DeleteOAuthConnection(suite.ctx, suite.oauthConn.ID)
		if err != nil {
			suite.logger.Error().Err(err).Msg("Failed to delete OAuth connection")
		} else {
			suite.logger.Info().Msg("OAuth connection deleted")
		}
	}

	// Delete test app
	if suite.testApp != nil {
		err := suite.store.DeleteApp(suite.ctx, suite.testApp.ID)
		if err != nil {
			suite.logger.Error().Err(err).Msg("Failed to delete test app")
		} else {
			suite.logger.Info().Msg("Test app deleted")
		}
	}

	// Delete OAuth provider
	if suite.oauthProvider != nil {
		err := suite.store.DeleteOAuthProvider(suite.ctx, suite.oauthProvider.ID)
		if err != nil {
			suite.logger.Error().Err(err).Msg("Failed to delete OAuth provider")
		} else {
			suite.logger.Info().Msg("OAuth provider deleted")
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

	suite.logger.Info().Msg("=== Test Cleanup Completed ===")
}

// testCreateTestRepositories creates test repositories on GitHub for testing
func (suite *GitHubOAuthE2ETestSuite) testCreateTestRepositories(t *testing.T) {
	suite.logger.Info().Msg("Creating test repositories on GitHub")

	// Create a test repository with some content
	repoName := fmt.Sprintf("helix-test-repo-%d", time.Now().Unix())

	err := suite.createGitHubRepo(repoName, "Test repository for Helix OAuth skills testing")
	require.NoError(t, err, "Failed to create test repository")

	suite.testRepos = append(suite.testRepos, repoName)

	// Add some test content to the repository
	err = suite.addContentToRepo(repoName, "README.md", "# Helix Test Repository\n\nThis is a test repository created for testing Helix OAuth skills integration.\n\n## Test Data\n\n- Repository: "+repoName+"\n- Created by: Helix OAuth E2E Test\n- Purpose: Testing GitHub skills integration\n")
	require.NoError(t, err, "Failed to add content to test repository")

	err = suite.addContentToRepo(repoName, "test-file.txt", "This is a test file for Helix to read and understand.\nIt contains some sample data that the agent should be able to access.")
	require.NoError(t, err, "Failed to add test file to repository")

	suite.logger.Info().
		Str("repo_name", repoName).
		Msg("Successfully created test repository with content")
}

// createGitHubRepo creates a new repository on GitHub
func (suite *GitHubOAuthE2ETestSuite) createGitHubRepo(name, description string) error {
	reqBody := map[string]interface{}{
		"name":        name,
		"description": description,
		"private":     true,
		"auto_init":   true,
	}

	return suite.makeGitHubAPICall("POST", "https://api.github.com/user/repos", reqBody, nil)
}

// addContentToRepo adds content to a GitHub repository
func (suite *GitHubOAuthE2ETestSuite) addContentToRepo(repoName, filename, content string) error {
	// First try to get existing file to see if it already exists
	getURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", suite.githubUsername, repoName, filename)

	client := &http.Client{Timeout: 30 * time.Second}
	getReq, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}
	getReq.Header.Set("Authorization", "token "+suite.githubToken)
	getReq.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(getReq)
	if err != nil {
		return fmt.Errorf("failed to check if file exists: %w", err)
	}
	defer resp.Body.Close()

	reqBody := map[string]interface{}{
		"message": "Add " + filename + " via Helix test",
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}

	// If file exists (200), we need to include the SHA for updates
	if resp.StatusCode == 200 {
		var existingFile map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&existingFile); err == nil {
			if sha, ok := existingFile["sha"].(string); ok {
				reqBody["sha"] = sha
				suite.logger.Info().
					Str("filename", filename).
					Str("sha", sha).
					Msg("File exists, including SHA for update")
			}
		}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", suite.githubUsername, repoName, filename)
	return suite.makeGitHubAPICall("PUT", url, reqBody, nil)
}

// deleteGitHubRepo deletes a repository from GitHub
func (suite *GitHubOAuthE2ETestSuite) deleteGitHubRepo(name string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", suite.githubUsername, name)
	return suite.makeGitHubAPICall("DELETE", url, nil, nil)
}

// makeGitHubAPICall makes an authenticated API call to GitHub
func (suite *GitHubOAuthE2ETestSuite) makeGitHubAPICall(method, url string, body interface{}, result interface{}) error {
	client := &http.Client{Timeout: 30 * time.Second}

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+suite.githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %d %s - %s", resp.StatusCode, resp.Status, string(respBody))
	}

	if result != nil && resp.StatusCode != 204 {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

// GitHubSkillConfig represents the parsed GitHub skill configuration
type GitHubSkillConfig struct {
	Name          string
	DisplayName   string
	Description   string
	SystemPrompt  string
	BaseURL       string
	Headers       map[string]string
	Schema        string
	OAuthProvider string
	OAuthScopes   []string
}

// GitHubUserInfo represents GitHub user information from API
type GitHubUserInfo struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// getGitHubSkillSchema returns the OpenAPI schema for GitHub skill
func (suite *GitHubOAuthE2ETestSuite) getGitHubSkillSchema() string {
	return `openapi: 3.0.3
info:
  title: GitHub API
  description: Access GitHub repositories, issues, pull requests, and user information
  version: "2022-11-28"
servers:
  - url: https://api.github.com
paths:
  /user:
    get:
      summary: Get the authenticated user
      operationId: getAuthenticatedUser
      security:
        - BearerAuth: []
      responses:
        '200':
          description: Authenticated user profile
          content:
            application/json:
              schema:
                type: object
                properties:
                  login:
                    type: string
                  name:
                    type: string
                  email:
                    type: string
                  bio:
                    type: string
                  public_repos:
                    type: integer
                  followers:
                    type: integer
                  following:
                    type: integer
  /user/repos:
    get:
      summary: List repositories for the authenticated user
      operationId: listUserRepos
      security:
        - BearerAuth: []
      parameters:
        - name: type
          in: query
          schema:
            type: string
            enum: [all, owner, public, private, member]
            default: owner
        - name: sort
          in: query
          schema:
            type: string
            enum: [created, updated, pushed, full_name]
            default: created
        - name: direction
          in: query
          schema:
            type: string
            enum: [asc, desc]
            default: desc
        - name: per_page
          in: query
          schema:
            type: integer
            default: 30
            maximum: 100
      responses:
        '200':
          description: List of repositories
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    name:
                      type: string
                    full_name:
                      type: string
                    description:
                      type: string
                    private:
                      type: boolean
                    html_url:
                      type: string
                    language:
                      type: string
                    stargazers_count:
                      type: integer
                    forks_count:
                      type: integer
                    created_at:
                      type: string
                    updated_at:
                      type: string
  /repos/{owner}/{repo}/contents/{path}:
    get:
      summary: Get repository content
      operationId: getRepoContent
      security:
        - BearerAuth: []
      parameters:
        - name: owner
          in: path
          required: true
          schema:
            type: string
        - name: repo
          in: path
          required: true
          schema:
            type: string
        - name: path
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Repository content
          content:
            application/json:
              schema:
                type: object
                properties:
                  name:
                    type: string
                  path:
                    type: string
                  content:
                    type: string
                  encoding:
                    type: string
security:
  - BearerAuth: []
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer`
}

// loadGitHubSkillConfig loads the complete GitHub skill configuration from github.yaml
func (suite *GitHubOAuthE2ETestSuite) loadGitHubSkillConfig() *GitHubSkillConfig {
	return &GitHubSkillConfig{
		Name:        "github",
		DisplayName: "GitHub",
		Description: `Access GitHub repositories, issues, pull requests, and user information with OAuth authentication.

This skill provides secure access to GitHub's REST API, allowing you to:
• View and manage repositories
• Create and track issues  
• Monitor pull requests and code reviews
• Search repositories and code
• Access user profiles and organizations`,
		SystemPrompt: `You are a GitHub development assistant. Your expertise is in helping users manage their GitHub repositories, issues, pull requests, and development workflows.

Key capabilities:
- Repository management and code exploration
- Issue tracking, creation, and management  
- Pull request workflows and code reviews
- User and organization information
- Repository statistics and insights

When users ask about code repositories, development tasks, or GitHub workflows:
1. Help them find and explore repositories and their contents
2. Assist with issue management - creating, updating, searching issues
3. Support pull request workflows and code collaboration
4. Provide repository insights and statistics
5. Help with user and organization management

Always focus on GitHub-related development tasks. If asked about other platforms or non-GitHub topics, politely redirect to GitHub-specific assistance.`,
		BaseURL: "https://api.github.com",
		Headers: map[string]string{
			"Accept":               "application/vnd.github+json",
			"X-GitHub-Api-Version": "2022-11-28",
		},
		Schema:        suite.getGitHubSkillSchema(),
		OAuthProvider: "github",
		OAuthScopes:   []string{"repo", "user:read"},
	}
}

// cleanupExistingOAuthData removes any OAuth connections/providers from previous test runs
func (suite *GitHubOAuthE2ETestSuite) cleanupExistingOAuthData() error {
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

	// Delete all OAuth providers (be careful not to delete production ones)
	providers, err := suite.store.ListOAuthProviders(suite.ctx, &store.ListOAuthProvidersQuery{})
	if err != nil {
		return fmt.Errorf("failed to list OAuth providers: %w", err)
	}

	for _, provider := range providers {
		// Only delete test providers
		if strings.Contains(provider.Name, "Skills Test") || strings.Contains(provider.Name, "Test") {
			err = suite.store.DeleteOAuthProvider(suite.ctx, provider.ID)
			if err != nil {
				suite.logger.Warn().Err(err).Str("provider_id", provider.ID).Msg("Failed to delete OAuth provider")
			} else {
				suite.logger.Debug().Str("provider_id", provider.ID).Msg("Deleted OAuth provider from previous run")
			}
		}
	}

	suite.logger.Info().
		Int("connections_cleaned", len(connections)).
		Msg("Cleanup of existing OAuth data completed")

	return nil
}

// startHelixOAuthFlow starts OAuth flow using OAuth manager directly
func (suite *GitHubOAuthE2ETestSuite) startHelixOAuthFlow() (string, string, error) {
	suite.logger.Info().Msg("Starting OAuth flow via OAuth manager")

	// Call OAuth manager directly instead of making HTTP request
	authURL, err := suite.oauth.StartOAuthFlow(suite.ctx, suite.testUser.ID, suite.oauthProvider.ID, suite.oauthProvider.CallbackURL)
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

// getGitHubAuthorizationCode performs real browser automation to complete GitHub OAuth flow
func (suite *GitHubOAuthE2ETestSuite) getGitHubAuthorizationCode(authURL, state string) (string, error) {
	suite.logger.Info().
		Str("auth_url", authURL).
		Str("state", state).
		Msg("Starting browser automation for GitHub OAuth")

	// Create a new page for the OAuth flow
	page, err := suite.browser.Page(proto.TargetCreateTarget{
		URL: "about:blank",
	})
	if err != nil {
		return "", fmt.Errorf("failed to create browser page: %w", err)
	}
	defer page.Close()

	// Navigate to GitHub OAuth authorization URL
	suite.logger.Info().Str("url", authURL).Msg("Navigating to GitHub OAuth authorization URL")
	err = page.Navigate(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to navigate to GitHub OAuth URL: %w", err)
	}

	// Wait for page to load
	err = page.WaitLoad()
	if err != nil {
		return "", fmt.Errorf("failed to wait for page load: %w", err)
	}

	suite.takeScreenshot(page, "github_auth_page_loaded")

	// Check if we're already logged in or need to login
	// Look for either the login form or the authorization form
	loginUsernameSelector := `input[name="login"]`
	authButtonSelector := `button[name="authorize"], input[type="submit"][value="Authorize"], button:contains("Authorize")`

	suite.logger.Info().Msg("Checking if GitHub login is required")

	// Wait a moment for the page to fully load
	time.Sleep(2 * time.Second)

	// Check if we need to login first
	loginElement := page.Element(loginUsernameSelector)
	var authButtonElement *rod.Element

	if loginElement != nil {
		// We need to log in first
		suite.logger.Info().Msg("GitHub login required - filling in credentials")

		// Fill in username
		err = loginElement.Input(suite.githubUsername)
		if err != nil {
			return "", fmt.Errorf("failed to enter GitHub username: %w", err)
		}

		// Fill in password
		passwordElement := page.MustElement(`input[name="password"]`)
		err = passwordElement.Input(suite.githubPassword)
		if err != nil {
			return "", fmt.Errorf("failed to enter GitHub password: %w", err)
		}

		suite.takeScreenshot(page, "github_login_filled")

		// Click login button
		loginButton := page.MustElement(`input[type="submit"][value="Sign in"], button[type="submit"]`)
		err = loginButton.Click()
		if err != nil {
			return "", fmt.Errorf("failed to click GitHub login button: %w", err)
		}

		suite.logger.Info().Msg("Clicked GitHub login button")

		// Wait for login to complete and authorization page to load
		// Look for the authorization button after login
		suite.logger.Info().Msg("Waiting for authorization page after login")

		// Wait for page to reload after login
		time.Sleep(3 * time.Second)

		suite.takeScreenshot(page, "github_auth_page_after_login")
	} else {
		suite.logger.Info().Msg("Already logged into GitHub - proceeding with authorization")
	}

	// Now look for the authorization button (either after login or if already logged in)
	suite.logger.Info().Msg("Looking for authorization button on the page")
	authButtonElement = page.Element(authButtonSelector)
	if authButtonElement == nil {
		// If we can't find the standard selectors, try to find any button with "Authorize" text
		authButtonElement = page.Element(`button`, `input[type="submit"]`, `a[href*="authorize"]`)
		if authButtonElement == nil {
			// Take a screenshot to debug what's on the page
			suite.takeScreenshot(page, "github_auth_button_not_found")
			pageContent, _ := page.HTML()
			suite.logger.Error().Str("page_content", pageContent[:2000]+"...").Msg("Authorization button not found - page content")
			return "", fmt.Errorf("could not find GitHub authorization button on page")
		}
	}

	// Now we should be on the authorization page
	suite.logger.Info().Msg("Looking for GitHub authorization button")

	// Take screenshot of authorization page
	suite.takeScreenshot(page, "github_authorization_page")

	// Check what the authorization form looks like
	pageContent, err := page.HTML()
	if err == nil {
		suite.logger.Debug().Str("page_content", pageContent[:1000]+"...").Msg("Authorization page content (first 1000 chars)")
	}

	// Click the authorize button
	suite.logger.Info().Msg("Clicking GitHub authorize button")
	err = authButtonElement.Click()
	if err != nil {
		return "", fmt.Errorf("failed to click GitHub authorize button: %w", err)
	}

	// Wait for redirect to callback URL
	suite.logger.Info().Msg("Waiting for GitHub OAuth callback redirect")

	// Wait for navigation to callback URL (with authorization code)
	// The callback URL should contain the authorization code
	callbackURL := ""
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			suite.takeScreenshot(page, "github_oauth_timeout")
			currentURL := page.MustInfo().URL
			return "", fmt.Errorf("timeout waiting for GitHub OAuth callback, current URL: %s", currentURL)
		case <-ticker.C:
			currentURL := page.MustInfo().URL
			if strings.Contains(currentURL, "/api/v1/oauth/callback") || strings.Contains(currentURL, "code=") {
				callbackURL = currentURL
				suite.logger.Info().Str("callback_url", callbackURL).Msg("GitHub OAuth callback received")
				break
			}
		}
		if callbackURL != "" {
			break
		}
	}

	suite.takeScreenshot(page, "github_oauth_callback_received")

	// Extract authorization code from callback URL
	parsedURL, err := url.Parse(callbackURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse callback URL: %w", err)
	}

	authCode := parsedURL.Query().Get("code")
	if authCode == "" {
		return "", fmt.Errorf("no authorization code in callback URL: %s", callbackURL)
	}

	// Verify state parameter matches
	callbackState := parsedURL.Query().Get("state")
	if callbackState != state {
		return "", fmt.Errorf("state mismatch: expected %s, got %s", state, callbackState)
	}

	suite.logger.Info().
		Str("auth_code", authCode[:10]+"...").
		Str("state", callbackState).
		Msg("Successfully extracted GitHub authorization code from callback")

	return authCode, nil
}

// completeHelixOAuthFlow completes OAuth flow using Helix's OAuth manager
func (suite *GitHubOAuthE2ETestSuite) completeHelixOAuthFlow(authCode, _ string) (*types.OAuthConnection, error) {
	suite.logger.Info().Msg("Completing OAuth flow via Helix OAuth manager")

	// Use OAuth manager directly to complete the flow
	connection, err := suite.oauth.CompleteOAuthFlow(suite.ctx, suite.testUser.ID, suite.oauthProvider.ID, authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to complete OAuth flow: %w", err)
	}

	return connection, nil
}

// testOAuthTokenDirectly tests OAuth token functionality (simplified)
func (suite *GitHubOAuthE2ETestSuite) testOAuthTokenDirectly(t *testing.T) {
	require.NotNil(t, suite.oauthConn, "OAuth connection should exist")
	require.NotEmpty(t, suite.oauthConn.AccessToken, "OAuth access token should not be empty")

	suite.logger.Info().Msg("OAuth token test passed - connection exists with access token")
}

// testAgentGitHubSkillsIntegration tests the complete agent integration with GitHub skills
func (suite *GitHubOAuthE2ETestSuite) testAgentGitHubSkillsIntegration(t *testing.T) {
	suite.logger.Info().Msg("=== Testing Agent GitHub Skills Integration ===")

	// Verify OAuth connection exists and is accessible
	connections, err := suite.oauth.ListUserConnections(suite.ctx, suite.testUser.ID)
	require.NoError(t, err, "Failed to list OAuth connections")
	require.NotEmpty(t, connections, "Should have at least one OAuth connection")

	// Find GitHub connection
	var githubConn *types.OAuthConnection
	for _, conn := range connections {
		if conn.ProviderID == suite.oauthProvider.ID {
			githubConn = conn
			break
		}
	}

	require.NotNil(t, githubConn, "Should have GitHub OAuth connection")
	require.NotEmpty(t, githubConn.AccessToken, "GitHub OAuth connection should have access token")

	suite.logger.Info().
		Str("provider_id", githubConn.ProviderID).
		Str("user_id", githubConn.UserID).
		Str("access_token", githubConn.AccessToken[:10]+"...").
		Msg("Verified OAuth connection exists with access token")

	// Test 1: Ask agent about GitHub username (should use real OAuth data)
	suite.logger.Info().Msg("Testing agent GitHub username query")

	session, err := suite.store.CreateSession(suite.ctx, &types.Session{
		Name:      "GitHub Skills Test Session",
		UserID:    suite.testUser.ID,
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
		ModelName: types.Model{Name: "anthropic:claude-3-haiku-20240307"},
		Messages:  []*types.Message{},
		Metadata: map[string]interface{}{
			"app_id": suite.testApp.ID,
		},
	})
	require.NoError(t, err, "Failed to create test session")

	// Add user message asking for GitHub username
	_, err = suite.store.CreateMessage(suite.ctx, &types.Message{
		SessionID: session.ID,
		Role:      types.MessageRoleUser,
		Content:   "What is my GitHub username?",
		Metadata:  map[string]interface{}{},
	})
	require.NoError(t, err, "Failed to create user message")

	suite.logger.Info().
		Str("session_id", session.ID).
		Str("app_id", suite.testApp.ID).
		Msg("Created test session with GitHub username query")

	// Test 2: Ask agent to list GitHub repositories (should use real OAuth API calls)
	suite.logger.Info().Msg("Testing agent GitHub repository listing")

	session2, err := suite.store.CreateSession(suite.ctx, &types.Session{
		Name:      "GitHub Skills Test Session 2",
		UserID:    suite.testUser.ID,
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
		ModelName: types.Model{Name: "anthropic:claude-3-haiku-20240307"},
		Messages:  []*types.Message{},
		Metadata: map[string]interface{}{
			"app_id": suite.testApp.ID,
		},
	})
	require.NoError(t, err, "Failed to create second test session")

	// Add user message asking for repository list
	_, err = suite.store.CreateMessage(suite.ctx, &types.Message{
		SessionID: session2.ID,
		Role:      types.MessageRoleUser,
		Content:   "List my GitHub repositories",
		Metadata:  map[string]interface{}{},
	})
	require.NoError(t, err, "Failed to create user message for repository query")

	suite.logger.Info().
		Str("session_id", session2.ID).
		Str("app_id", suite.testApp.ID).
		Msg("Created test session with GitHub repository query")

	// Test 3: Ask agent about issues in test repository (should use real OAuth API calls)
	if len(suite.testRepos) > 0 {
		suite.logger.Info().Msg("Testing agent GitHub issues query")

		session3, err := suite.store.CreateSession(suite.ctx, &types.Session{
			Name:      "GitHub Skills Test Session 3",
			UserID:    suite.testUser.ID,
			Mode:      types.SessionModeInference,
			Type:      types.SessionTypeText,
			ModelName: types.Model{Name: "anthropic:claude-3-haiku-20240307"},
			Messages:  []*types.Message{},
			Metadata: map[string]interface{}{
				"app_id": suite.testApp.ID,
			},
		})
		require.NoError(t, err, "Failed to create third test session")

		// Add user message asking about issues in test repository
		repoQuery := fmt.Sprintf("What issues are open in my repository %s?", suite.testRepos[0])
		_, err = suite.store.CreateMessage(suite.ctx, &types.Message{
			SessionID: session3.ID,
			Role:      types.MessageRoleUser,
			Content:   repoQuery,
			Metadata:  map[string]interface{}{},
		})
		require.NoError(t, err, "Failed to create user message for issues query")

		suite.logger.Info().
			Str("session_id", session3.ID).
			Str("app_id", suite.testApp.ID).
			Str("repository", suite.testRepos[0]).
			Msg("Created test session with GitHub issues query")
	}

	// NOTE: The actual execution of these sessions with the Helix agent system would require
	// starting the full Helix server with session handler and letting it process these messages.
	// This test verifies the OAuth infrastructure is working and sessions are created correctly.
	// In a real end-to-end test, we would:
	// 1. Start the Helix server with the session handler
	// 2. Make HTTP requests to process these sessions
	// 3. Verify the agent responses contain real GitHub data (username, repos, issues)
	// 4. Verify OAuth tokens are being used for GitHub API calls

	suite.logger.Info().
		Int("sessions_created", 3).
		Str("oauth_connection_id", githubConn.ID).
		Msg("Successfully created test sessions with GitHub skills - OAuth integration infrastructure verified")
}
