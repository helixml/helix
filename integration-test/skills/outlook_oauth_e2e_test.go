// Outlook OAuth Skills E2E Test
//
// This test requires OAuth integration environment variables and Chrome
// container, which the stack command will start automatically.
//
// To run this test, from the helix root directory:
//
//   ./stack test -v integration-test/skills/*.go -run TestOutlookOAuthSkillsE2E
//
// The test will:
// 1. Set up Helix infrastructure (OAuth manager, API server, etc.)
// 2. Create a Microsoft OAuth provider
// 3. Create a Helix app with Outlook skills from outlook.yaml
// 4. Perform OAuth flow against real Microsoft using browser automation
// 5. Test agent sessions with real Microsoft Graph API calls with the resulting JWT
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

// ======================================================================================
// TEST QUERIES AND CONFIGURATION - Most important part for understanding what's tested
// ======================================================================================

// OutlookTestQueries defines the agent test queries that verify Outlook skills integration
var OutlookTestQueries = []AgentTestQuery{
	{
		Query:       "What is my Outlook email address?",
		SessionName: "Outlook Profile Query",
		ExpectedResponseCheck: func(response string) bool {
			return len(response) > 0 && (strings.Contains(strings.ToLower(response), "email") || strings.Contains(response, "@"))
		},
	},
	{
		Query:       "Show me my Outlook profile information",
		SessionName: "Outlook Profile Details",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "email") || strings.Contains(lower, "outlook") || strings.Contains(lower, "profile")
		},
	},
	{
		Query:       "List my recent Outlook messages",
		SessionName: "Outlook Messages List",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "message") || strings.Contains(lower, "email") || len(response) > 0
		},
	},
	{
		Query:       "Search for unread emails in my Outlook",
		SessionName: "Outlook Unread Messages",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "unread") || strings.Contains(lower, "message") || strings.Contains(lower, "email") || len(response) > 0
		},
	},
	{
		Query:       "Show me my Outlook mail folders",
		SessionName: "Outlook Folders List",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "folder") || strings.Contains(lower, "inbox") || strings.Contains(lower, "sent") || len(response) > 0
		},
	},
}

// OutlookOAuthProviderConfig defines the OAuth provider configuration for Outlook
var OutlookOAuthProviderConfig = OAuthProviderConfig{
	ProviderName: "Outlook Skills Test",
	ProviderType: types.OAuthProviderTypeMicrosoft,
	SkillName:    "outlook",
	AuthURL:      "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	UserInfoURL:  "https://graph.microsoft.com/v1.0/me",
	Scopes:       []string{"https://graph.microsoft.com/User.Read", "https://graph.microsoft.com/Mail.Read", "https://graph.microsoft.com/Mail.Send"},
	// ClientID, ClientSecret, Username, Password, and functions will be set during test setup
}

// ======================================================================================
// MAIN TEST SUITE
// ======================================================================================

// OutlookOAuthE2ETestSuite tests the complete Outlook OAuth skills workflow
type OutlookOAuthE2ETestSuite struct {
	// Embed the base OAuth test suite for common functionality
	BaseOAuthTestSuite

	// Microsoft-specific test configuration from environment
	microsoftClientID     string
	microsoftClientSecret string
	microsoftUsername     string
	microsoftPassword     string

	// OAuth provider test template
	oauthTemplate *OAuthProviderTestTemplate
}

// TestOutlookOAuthSkillsE2E is the main end-to-end test for Outlook OAuth skills
func TestOutlookOAuthSkillsE2E(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	// Enable parallel execution with other tests
	t.Parallel()

	// Set a reasonable timeout for the OAuth browser automation
	timeout := 5 * time.Minute // Increased timeout to match other OAuth tests for reliability
	deadline := time.Now().Add(timeout)
	t.Deadline() // Check if deadline is already set

	// Create a context with timeout for the test
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	suite := &OutlookOAuthE2ETestSuite{
		BaseOAuthTestSuite: BaseOAuthTestSuite{
			ctx: ctx,
		},
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping Outlook OAuth E2E test: %v", err)
	}

	// Initialize test dependencies using base class
	err = suite.SetupBaseInfrastructure("outlook_oauth_e2e")
	require.NoError(t, err, "Failed to setup base infrastructure")

	// Outlook-specific setup
	err = suite.setupOutlookSpecifics(t)
	require.NoError(t, err, "Failed to setup Outlook-specific dependencies")

	// Create OAuth provider test template
	err = suite.createOAuthTemplate()
	require.NoError(t, err, "Failed to create OAuth provider test template")

	// Run the complete end-to-end workflow using template
	t.Run("ValidateOutlookSkillYAML", suite.testValidateOutlookSkillYAML)
	t.Run("SetupOAuthProvider", suite.oauthTemplate.TestSetupOAuthProvider)
	t.Run("CreateTestApp", suite.oauthTemplate.TestCreateTestApp)
	t.Run("PerformOAuthFlow", suite.oauthTemplate.TestPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.oauthTemplate.TestOAuthTokenDirectly)
	t.Run("TestAgentOutlookSkillsIntegration", suite.oauthTemplate.TestAgentOAuthSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanupOutlookSpecifics(t)
		suite.oauthTemplate.Cleanup(t)
		suite.CleanupBaseInfrastructure()
	})
}

// ======================================================================================
// TEST IMPLEMENTATION
// ======================================================================================

// testValidateOutlookSkillYAML validates that the Outlook skill YAML file exists and is properly structured
func (suite *OutlookOAuthE2ETestSuite) testValidateOutlookSkillYAML(t *testing.T) {
	suite.logger.Info().Msg("Validating Outlook skill YAML file")

	// Check that the skill can be loaded by the skills manager
	skillsManager := suite.oauthTemplate.skillManager
	skill, err := skillsManager.GetSkill("outlook")
	require.NoError(t, err, "Failed to load Outlook skill from YAML")
	require.NotNil(t, skill, "Outlook skill should not be nil")

	// Validate basic skill properties
	require.Equal(t, "outlook", skill.Name, "Skill name should be 'outlook'")
	require.Equal(t, "Outlook", skill.DisplayName, "Skill display name should be 'Outlook'")
	require.Equal(t, "microsoft", skill.Provider, "Skill provider should be 'microsoft'")
	require.Equal(t, "Productivity", skill.Category, "Skill category should be 'Productivity'")

	// Validate OAuth configuration
	require.Equal(t, "microsoft", skill.OAuthProvider, "OAuth provider should be 'microsoft'")
	require.Contains(t, skill.OAuthScopes, "https://graph.microsoft.com/User.Read", "Should have User.Read scope")
	require.Contains(t, skill.OAuthScopes, "https://graph.microsoft.com/Mail.Read", "Should have Mail.Read scope")
	require.Contains(t, skill.OAuthScopes, "https://graph.microsoft.com/Mail.Send", "Should have Mail.Send scope")

	// Validate API configuration
	require.Equal(t, "https://graph.microsoft.com", skill.BaseURL, "Base URL should be Microsoft Graph API URL")
	require.NotEmpty(t, skill.Schema, "Outlook skill should have OpenAPI schema")

	// Validate the schema contains key Microsoft Graph API operations
	require.Contains(t, skill.Schema, "getOutlookProfile", "Schema should contain getOutlookProfile operation")
	require.Contains(t, skill.Schema, "listOutlookMessages", "Schema should contain listOutlookMessages operation")
	require.Contains(t, skill.Schema, "getOutlookMessage", "Schema should contain getOutlookMessage operation")
	require.Contains(t, skill.Schema, "sendOutlookMessage", "Schema should contain sendOutlookMessage operation")
	require.Contains(t, skill.Schema, "listOutlookFolders", "Schema should contain listOutlookFolders operation")

	// Validate system prompt
	require.NotEmpty(t, skill.SystemPrompt, "Outlook skill should have system prompt")
	require.Contains(t, strings.ToLower(skill.SystemPrompt), "outlook", "System prompt should mention Outlook")

	suite.logger.Info().Msg("Outlook skill YAML validation completed successfully")
}

// ======================================================================================
// CONFIGURATION AND SETUP
// ======================================================================================

// loadTestConfig loads configuration from environment variables
func (suite *OutlookOAuthE2ETestSuite) loadTestConfig() error {
	suite.microsoftClientID = os.Getenv("MICROSOFT_SKILL_TEST_OAUTH_CLIENT_ID")
	if suite.microsoftClientID == "" {
		return fmt.Errorf("MICROSOFT_SKILL_TEST_OAUTH_CLIENT_ID environment variable not set")
	}

	suite.microsoftClientSecret = os.Getenv("MICROSOFT_SKILL_TEST_OAUTH_CLIENT_SECRET")
	if suite.microsoftClientSecret == "" {
		return fmt.Errorf("MICROSOFT_SKILL_TEST_OAUTH_CLIENT_SECRET environment variable not set")
	}

	suite.microsoftUsername = os.Getenv("MICROSOFT_SKILL_TEST_OAUTH_USERNAME")
	if suite.microsoftUsername == "" {
		return fmt.Errorf("MICROSOFT_SKILL_TEST_OAUTH_USERNAME environment variable not set")
	}

	suite.microsoftPassword = os.Getenv("MICROSOFT_SKILL_TEST_OAUTH_PASSWORD")
	if suite.microsoftPassword == "" {
		return fmt.Errorf("MICROSOFT_SKILL_TEST_OAUTH_PASSWORD environment variable not set")
	}

	// Debug logging for CI environment (log lengths only, not actual values)
	log.Info().
		Str("username", suite.microsoftUsername).
		Int("password_length", len(suite.microsoftPassword)).
		Msg("Microsoft OAuth credentials loaded (debug info for CI troubleshooting)")

	log.Info().
		Str("client_id", suite.microsoftClientID).
		Str("username", suite.microsoftUsername).
		Msg("Loaded Microsoft OAuth test configuration")

	return nil
}

// setupOutlookSpecifics initializes Outlook-specific test environment
func (suite *OutlookOAuthE2ETestSuite) setupOutlookSpecifics(_ *testing.T) error {
	suite.logger.Info().Msg("=== Outlook OAuth Skills E2E Test Starting ===")

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err := suite.CleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	suite.logger.Info().Msg("Outlook-specific test setup completed successfully")
	return nil
}

// createOAuthTemplate creates the OAuth provider test template for Outlook
func (suite *OutlookOAuthE2ETestSuite) createOAuthTemplate() error {
	// Make a copy of the config and fill in the dynamic values
	config := OutlookOAuthProviderConfig
	config.ClientID = suite.microsoftClientID
	config.ClientSecret = suite.microsoftClientSecret
	config.Username = suite.microsoftUsername
	config.Password = suite.microsoftPassword
	config.GetAuthorizationCodeFunc = suite.getMicrosoftAuthorizationCode
	config.SetupTestDataFunc = func() error { return nil }   // No setup needed for Outlook
	config.CleanupTestDataFunc = func() error { return nil } // No cleanup needed for Outlook
	config.AgentTestQueries = OutlookTestQueries

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

// getMicrosoftAuthorizationCode performs real browser automation to complete Microsoft OAuth flow
func (suite *OutlookOAuthE2ETestSuite) getMicrosoftAuthorizationCode(authURL, state string) (string, error) {
	// Set up Microsoft-specific OAuth handler
	microsoftHandler := NewMicrosoftOAuthHandler(suite.logger)

	// Configure Microsoft browser automation with Microsoft-specific strategy
	microsoftConfig := BrowserOAuthConfig{
		ProviderName:            "microsoft",
		LoginUsernameSelector:   `input[type="email"], input[name="loginfmt"], input[placeholder*="email"], input[placeholder*="Email"]`,
		LoginPasswordSelector:   `input[type="password"], input[name="passwd"], input[placeholder*="password"], input[placeholder*="Password"]`,
		LoginButtonSelector:     `input[type="submit"][value="Next"], input[type="submit"][value="Sign in"], input[id="idSIButton9"], button[type="submit"], input[type="submit"]`,
		AuthorizeButtonSelector: `input[type="submit"][value="Accept"], input[type="submit"][value="Allow"], input[id="idSIButton9"], button[type="submit"], input[type="submit"]`,
		CallbackURLPattern:      "/api/v1/oauth/flow/callback",
		DeviceVerificationCheck: microsoftHandler.IsRequiredForURL,
		TwoFactorHandler:        microsoftHandler,
	}

	// Add Microsoft-specific strategy to the config
	microsoftConfig.ProviderStrategy = NewMicrosoftProviderStrategy(suite.logger)

	// Create automator with Microsoft configuration
	automator := NewBrowserOAuthAutomator(suite.browser, suite.logger, microsoftConfig)

	// Perform OAuth flow using the generic automator with Microsoft-specific handling
	return automator.PerformOAuthFlow(authURL, state, suite.microsoftUsername, suite.microsoftPassword, suite)
}

// ======================================================================================
// CLEANUP
// ======================================================================================

// cleanupOutlookSpecifics cleans up Outlook-specific test resources
func (suite *OutlookOAuthE2ETestSuite) cleanupOutlookSpecifics(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting Outlook-specific Test Cleanup ===")

	// Outlook doesn't require specific cleanup (no test repositories to delete)
	suite.logger.Info().Msg("Outlook-specific cleanup completed (no specific resources to clean)")

	suite.logger.Info().Msg("=== Outlook-specific Test Cleanup Completed ===")
}
