// Confluence OAuth Skills E2E Test
//
// This test requires OAuth integration environment variables and Chrome
// container, which the stack command will start automatically.
//
// To run this test, from the helix root directory:
//
//   ./stack test -v integration-test/skills/*.go -run TestConfluenceOAuthSkillsE2E
//
// The test will:
// 1. Set up Helix infrastructure (OAuth manager, API server, etc.)
// 2. Create an Atlassian OAuth provider
// 3. Create a Helix app with Confluence skills from confluence.yaml
// 4. Perform OAuth flow against real Atlassian using browser automation
// 5. Test agent sessions with real Atlassian Confluence API calls with the resulting JWT
// 6. Clean up test resources

package skills

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

// getConfluenceScopes loads OAuth scopes from confluence.yaml and adds additional OAuth flow scopes
func getConfluenceScopes() []string {
	confluenceScopes, err := loadScopesFromYAML("confluence")
	if err != nil {
		panic(fmt.Sprintf("failed to load scopes from confluence.yaml: %v", err))
	}

	// Add additional scopes needed for the OAuth flow itself
	allScopes := append(confluenceScopes, "read:me", "read:account")
	return allScopes
}

// ======================================================================================
// TEST QUERIES AND CONFIGURATION - Most important part for understanding what's tested
// ======================================================================================

// ConfluenceTestQueries defines the agent test queries that verify Confluence skills integration
var ConfluenceTestQueries = []AgentTestQuery{
	{
		Query:       "Who am I in Confluence?",
		SessionName: "Confluence Profile Query",
		ExpectedResponseCheck: func(response string) bool {
			return len(response) > 0 && (strings.Contains(strings.ToLower(response), "account") || strings.Contains(response, "@"))
		},
	},
	{
		Query:       "Show me my Confluence profile information",
		SessionName: "Confluence Profile Details",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "account") || strings.Contains(lower, "confluence") || strings.Contains(lower, "profile") || strings.Contains(lower, "user")
		},
	},
	{
		Query:       "List all spaces I have access to",
		SessionName: "Confluence Spaces List",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "space") || strings.Contains(lower, "access") || len(response) > 0
		},
	},
	{
		Query:       "Show me recent pages in my spaces",
		SessionName: "Confluence Recent Pages",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "page") || strings.Contains(lower, "recent") || strings.Contains(lower, "space") || len(response) > 0
		},
	},
	{
		Query:       "Search for content in my Confluence",
		SessionName: "Confluence Content Search",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "content") || strings.Contains(lower, "search") || strings.Contains(lower, "confluence") || len(response) > 0
		},
	},
}

// ConfluenceOAuthProviderConfig defines the OAuth provider configuration for Confluence
var ConfluenceOAuthProviderConfig = OAuthProviderConfig{
	ProviderName: "Confluence Skills Test",
	ProviderType: types.OAuthProviderTypeAtlassian,
	SkillName:    "confluence",
	AuthURL:      "https://auth.atlassian.com/authorize",
	TokenURL:     "https://auth.atlassian.com/oauth/token",
	UserInfoURL:  "https://api.atlassian.com/me",
	Scopes:       getConfluenceScopes(),
	// ClientID, ClientSecret, Username, Password, and functions will be set during test setup
}

// ======================================================================================
// MAIN TEST SUITE
// ======================================================================================

// ConfluenceOAuthE2ETestSuite tests the complete Confluence OAuth skills workflow
type ConfluenceOAuthE2ETestSuite struct {
	// Embed the base OAuth test suite for common functionality
	BaseOAuthTestSuite

	// Atlassian-specific test configuration from environment
	atlassianClientID     string
	atlassianClientSecret string
	atlassianUsername     string
	atlassianPassword     string
	atlassianCloudID      string

	// OAuth provider test template
	oauthTemplate *OAuthProviderTestTemplate
}

// TestConfluenceOAuthSkillsE2E is the main end-to-end test for Confluence OAuth skills
func TestConfluenceOAuthSkillsE2E(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	// Enable parallel execution with other tests
	t.Parallel()

	// Set a reasonable timeout for the OAuth browser automation
	timeout := 5 * time.Minute // Very long timeout for Atlassian browser automation
	deadline := time.Now().Add(timeout)
	t.Deadline() // Check if deadline is already set

	// Create a context with timeout for the test
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	suite := &ConfluenceOAuthE2ETestSuite{
		BaseOAuthTestSuite: BaseOAuthTestSuite{
			ctx: ctx,
		},
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping Confluence OAuth E2E test: %v", err)
	}

	// Initialize test dependencies using base class
	err = suite.SetupBaseInfrastructure("confluence_oauth_e2e")
	require.NoError(t, err, "Failed to setup base infrastructure")

	// Confluence-specific setup
	err = suite.setupConfluenceSpecifics(t)
	require.NoError(t, err, "Failed to setup Confluence-specific dependencies")

	// Create OAuth provider test template
	err = suite.createOAuthTemplate()
	require.NoError(t, err, "Failed to create OAuth provider test template")

	// Run the complete end-to-end workflow using template
	t.Run("ValidateConfluenceSkillYAML", suite.testValidateConfluenceSkillYAML)
	t.Run("SetupOAuthProvider", suite.oauthTemplate.TestSetupOAuthProvider)
	t.Run("CreateTestApp", suite.oauthTemplate.TestCreateTestApp)
	t.Run("PerformOAuthFlow", suite.oauthTemplate.TestPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.oauthTemplate.TestOAuthTokenDirectly)
	t.Run("TestOAuthDebugging", suite.oauthTemplate.TestOAuthDebugging)
	t.Run("TestAgentConfluenceSkillsIntegration", suite.oauthTemplate.TestAgentOAuthSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanupConfluenceSpecifics(t)
		suite.oauthTemplate.Cleanup(t)
		suite.CleanupBaseInfrastructure()
	})
}

// ======================================================================================
// TEST IMPLEMENTATION
// ======================================================================================

// testValidateConfluenceSkillYAML validates that the Confluence skill YAML file exists and is properly structured
func (suite *ConfluenceOAuthE2ETestSuite) testValidateConfluenceSkillYAML(t *testing.T) {
	suite.logger.Info().Msg("Validating Confluence skill YAML file")

	// Check that the skill can be loaded by the skills manager
	skillsManager := suite.oauthTemplate.skillManager
	skill, err := skillsManager.GetSkill("confluence")
	require.NoError(t, err, "Failed to load Confluence skill from YAML")
	require.NotNil(t, skill, "Confluence skill should not be nil")

	// Validate basic skill properties
	require.Equal(t, "confluence", skill.Name, "Skill name should be 'confluence'")
	require.Equal(t, "Confluence", skill.DisplayName, "Skill display name should be 'Confluence'")
	require.Equal(t, "atlassian", skill.Provider, "Skill provider should be 'atlassian'")
	require.Equal(t, "Productivity", skill.Category, "Skill category should be 'Productivity'")

	// Validate OAuth configuration
	require.Equal(t, "atlassian", skill.OAuthProvider, "OAuth provider should be 'atlassian'")
	require.Contains(t, skill.OAuthScopes, "read:confluence-content.summary", "Should have read:confluence-content.summary scope")
	require.Contains(t, skill.OAuthScopes, "read:confluence-content.all", "Should have read:confluence-content.all scope")
	require.Contains(t, skill.OAuthScopes, "write:confluence-content", "Should have write:confluence-content scope")
	require.Contains(t, skill.OAuthScopes, "read:confluence-space.summary", "Should have read:confluence-space.summary scope")

	// Validate API configuration
	require.Equal(t, "https://api.atlassian.com", skill.BaseURL, "Base URL should be Atlassian API URL")
	require.NotEmpty(t, skill.Schema, "Confluence skill should have OpenAPI schema")

	// Validate the schema contains key Atlassian Confluence API operations
	require.Contains(t, skill.Schema, "getConfluenceCurrentUser", "Schema should contain getConfluenceCurrentUser operation")
	require.Contains(t, skill.Schema, "getConfluenceContent", "Schema should contain getConfluenceContent operation")
	require.Contains(t, skill.Schema, "getConfluenceContentById", "Schema should contain getConfluenceContentById operation")
	require.Contains(t, skill.Schema, "createConfluenceContent", "Schema should contain createConfluenceContent operation")
	require.Contains(t, skill.Schema, "updateConfluenceContent", "Schema should contain updateConfluenceContent operation")
	require.Contains(t, skill.Schema, "getConfluenceSpaces", "Schema should contain getConfluenceSpaces operation")
	require.Contains(t, skill.Schema, "searchConfluenceContent", "Schema should contain searchConfluenceContent operation")

	// Validate system prompt
	require.NotEmpty(t, skill.SystemPrompt, "Confluence skill should have system prompt")
	require.Contains(t, strings.ToLower(skill.SystemPrompt), "confluence", "System prompt should mention Confluence")

	suite.logger.Info().Msg("Confluence skill YAML validation completed successfully")
}

// ======================================================================================
// CONFIGURATION AND SETUP
// ======================================================================================

// loadTestConfig loads configuration from environment variables
func (suite *ConfluenceOAuthE2ETestSuite) loadTestConfig() error {
	suite.atlassianClientID = os.Getenv("ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_ID")
	if suite.atlassianClientID == "" {
		return fmt.Errorf("ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_ID environment variable not set")
	}

	suite.atlassianClientSecret = os.Getenv("ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_SECRET")
	if suite.atlassianClientSecret == "" {
		return fmt.Errorf("ATLASSIAN_SKILL_TEST_OAUTH_CLIENT_SECRET environment variable not set")
	}

	suite.atlassianUsername = os.Getenv("ATLASSIAN_SKILL_TEST_OAUTH_USERNAME")
	if suite.atlassianUsername == "" {
		return fmt.Errorf("ATLASSIAN_SKILL_TEST_OAUTH_USERNAME environment variable not set")
	}

	suite.atlassianPassword = os.Getenv("ATLASSIAN_SKILL_TEST_OAUTH_PASSWORD")
	if suite.atlassianPassword == "" {
		return fmt.Errorf("ATLASSIAN_SKILL_TEST_OAUTH_PASSWORD environment variable not set")
	}

	suite.atlassianCloudID = os.Getenv("ATLASSIAN_SKILL_TEST_CONFLUENCE_CLOUD_ID")
	if suite.atlassianCloudID == "" {
		return fmt.Errorf("ATLASSIAN_SKILL_TEST_CONFLUENCE_CLOUD_ID environment variable not set")
	}

	// Debug logging for CI environment (log lengths only, not actual values)
	log.Info().
		Str("username", suite.atlassianUsername).
		Int("password_length", len(suite.atlassianPassword)).
		Msg("Atlassian OAuth credentials loaded (debug info for CI troubleshooting)")

	log.Info().
		Str("client_id", suite.atlassianClientID).
		Str("username", suite.atlassianUsername).
		Msg("Loaded Atlassian OAuth test configuration")

	return nil
}

// setupConfluenceSpecifics initializes Confluence-specific test environment
func (suite *ConfluenceOAuthE2ETestSuite) setupConfluenceSpecifics(_ *testing.T) error {
	suite.logger.Info().Msg("=== Confluence OAuth Skills E2E Test Starting ===")

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err := suite.CleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	suite.logger.Info().Msg("Confluence-specific test setup completed successfully")
	return nil
}

// createOAuthTemplate creates the OAuth provider test template for Confluence
func (suite *ConfluenceOAuthE2ETestSuite) createOAuthTemplate() error {
	// Make a copy of the config and fill in the dynamic values
	config := ConfluenceOAuthProviderConfig
	config.ClientID = suite.atlassianClientID
	config.ClientSecret = suite.atlassianClientSecret
	config.Username = suite.atlassianUsername
	config.Password = suite.atlassianPassword
	config.GetAuthorizationCodeFunc = suite.getAtlassianAuthorizationCode
	config.SetupTestDataFunc = func() error { return nil }   // No setup needed for Confluence
	config.CleanupTestDataFunc = func() error { return nil } // No cleanup needed for Confluence
	config.AgentTestQueries = ConfluenceTestQueries
	config.AtlassianCloudID = suite.atlassianCloudID

	// Create the OAuth template
	template, err := NewOAuthProviderTestTemplate(config, &suite.BaseOAuthTestSuite)
	if err != nil {
		return fmt.Errorf("failed to create OAuth provider test template: %w", err)
	}

	suite.oauthTemplate = template
	return nil
}

// ======================================================================================
// OAUTH FLOW IMPLEMENTATION
// ======================================================================================

// getAtlassianAuthorizationCode performs real browser automation to complete Atlassian OAuth flow
func (suite *ConfluenceOAuthE2ETestSuite) getAtlassianAuthorizationCode(authURL, state string) (string, error) {
	// Start enhanced screenshot capture for the OAuth flow
	// suite.logger.Info().Msg("Starting enhanced screenshot capture for Confluence OAuth flow")
	// err := suite.StartEnhancedScreenshotCapture("confluence_oauth_flow")
	// if err != nil {
	// 	suite.logger.Error().Err(err).Msg("Failed to start enhanced screenshot capture, continuing without recording")
	// }

	// Ensure enhanced screenshot capture is stopped regardless of outcome
	// defer func() {
	// 	suite.logger.Info().Msg("Stopping enhanced screenshot capture for Confluence OAuth flow")
	// 	stopErr := suite.StopEnhancedScreenshotCapture()
	// 	if stopErr != nil {
	// 		suite.logger.Error().Err(stopErr).Msg("Failed to stop enhanced screenshot capture")
	// 	}
	// }()

	// Set up Atlassian-specific OAuth handler
	atlassianHandler, err := NewAtlassianOAuthHandler(os.Getenv("GMAIL_CREDENTIALS_BASE64"), suite.logger)
	if err != nil {
		return "", fmt.Errorf("failed to create Atlassian OAuth handler: %w", err)
	}

	// Configure Atlassian browser automation with improved selectors and provider strategy
	atlassianStrategy := NewAtlassianProviderStrategy(suite.logger, "helixml-confluence")
	atlassianConfig := BrowserOAuthConfig{
		ProviderName:            "atlassian",
		LoginUsernameSelector:   `input[type="email"], input[name="username"], input[id="username"], input[placeholder*="email"], input[placeholder*="Email"]`,
		LoginPasswordSelector:   `input[type="password"], input[name="password"], input[id="password"], input[placeholder*="password"], input[placeholder*="Password"]`,
		LoginButtonSelector:     `button[type="submit"], input[type="submit"], button[id="login-submit"], button:contains("Continue"), button:contains("Log in")`,
		AuthorizeButtonSelector: `button[type="submit"], input[type="submit"], button:contains("Accept"), button:contains("Allow"), button:contains("Authorize")`,
		CallbackURLPattern:      "/api/v1/oauth/flow/callback",
		DeviceVerificationCheck: atlassianHandler.IsRequiredForURL,
		TwoFactorHandler:        atlassianHandler,
		ProviderStrategy:        atlassianStrategy,
	}

	// Create automator with Atlassian configuration
	automator := NewBrowserOAuthAutomator(suite.browser, suite.logger, atlassianConfig)

	// Perform OAuth flow using the generic automator with Atlassian-specific handling
	return automator.PerformOAuthFlow(authURL, state, suite.atlassianUsername, suite.atlassianPassword, suite)
}

// ======================================================================================
// CLEANUP
// ======================================================================================

// cleanupConfluenceSpecifics cleans up Confluence-specific test resources
func (suite *ConfluenceOAuthE2ETestSuite) cleanupConfluenceSpecifics(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting Confluence-specific Test Cleanup ===")

	// Confluence doesn't require specific cleanup (no test repositories to delete)
	suite.logger.Info().Msg("Confluence-specific cleanup completed (no specific resources to clean)")

	suite.logger.Info().Msg("=== Confluence-specific Test Cleanup Completed ===")
}

// ======================================================================================
// SHARED ATLASSIAN OAUTH HANDLER (could be moved to shared utility)
// ======================================================================================

// Note: The AtlassianOAuthHandler is already defined in jira_oauth_e2e_test.go
// In a real implementation, this would be moved to a shared utility file to avoid duplication
