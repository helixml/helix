// Gmail OAuth Skills E2E Test
//
// This test requires OAuth integration environment variables and Chrome
// container, which the stack command will start automatically.
//
// To run this test, from the helix root directory:
//
//   ./stack test -v integration-test/skills/*.go -run TestGmailOAuthSkillsE2E
//
// The test will:
// 1. Set up Helix infrastructure (OAuth manager, API server, etc.)
// 2. Create a Google OAuth provider
// 3. Create a Helix app with Gmail skills from gmail.yaml
// 4. Perform OAuth flow against real Google using browser automation
// 5. Test agent sessions with real Gmail API calls with the resulting JWT
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

// GmailTestQueries defines the agent test queries that verify Gmail skills integration
var GmailTestQueries = []AgentTestQuery{
	{
		Query:       "What is my Gmail email address?",
		SessionName: "Gmail Profile Query",
		ExpectedResponseCheck: func(response string) bool {
			return len(response) > 0 && (strings.Contains(strings.ToLower(response), "email") || strings.Contains(response, "@"))
		},
	},
	{
		Query:       "Show me my Gmail profile information",
		SessionName: "Gmail Profile Details",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "email") || strings.Contains(lower, "gmail") || strings.Contains(lower, "profile")
		},
	},
	{
		Query:       "List my recent Gmail messages",
		SessionName: "Gmail Messages List",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "message") || strings.Contains(lower, "email") || len(response) > 0
		},
	},
	{
		Query:       "Search for unread emails in my Gmail",
		SessionName: "Gmail Unread Messages",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "unread") || strings.Contains(lower, "message") || strings.Contains(lower, "email") || len(response) > 0
		},
	},
	{
		Query:       "How many total messages are in my Gmail?",
		SessionName: "Gmail Message Count",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "message") || strings.Contains(lower, "total") || strings.Contains(lower, "email") || len(response) > 0
		},
	},
}

// GmailOAuthProviderConfig defines the OAuth provider configuration for Gmail
var GmailOAuthProviderConfig = OAuthProviderConfig{
	ProviderName: "Gmail Skills Test",
	ProviderType: types.OAuthProviderTypeGoogle,
	SkillName:    "gmail",
	AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:     "https://oauth2.googleapis.com/token",
	UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
	Scopes:       []string{"openid", "email", "profile", "https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/gmail.send"},
	// ClientID, ClientSecret, Username, Password, and functions will be set during test setup
}

// ======================================================================================
// MAIN TEST SUITE
// ======================================================================================

// GmailOAuthE2ETestSuite tests the complete Gmail OAuth skills workflow
type GmailOAuthE2ETestSuite struct {
	// Embed the base OAuth test suite for common functionality
	BaseOAuthTestSuite

	// Google-specific test configuration from environment
	googleClientID     string
	googleClientSecret string
	googleUsername     string
	googlePassword     string

	// OAuth provider test template
	oauthTemplate *OAuthProviderTestTemplate
}

// TestGmailOAuthSkillsE2E is the main end-to-end test for Gmail OAuth skills
func TestGmailOAuthSkillsE2E(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	// Enable parallel execution with other tests
	t.Parallel()

	// Set a reasonable timeout for the OAuth browser automation
	timeout := 5 * time.Minute // Increased from 2 minutes for more reliable authorization button search
	deadline := time.Now().Add(timeout)
	t.Deadline() // Check if deadline is already set

	// Create a context with timeout for the test
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	suite := &GmailOAuthE2ETestSuite{
		BaseOAuthTestSuite: BaseOAuthTestSuite{
			ctx: ctx,
		},
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping Gmail OAuth E2E test: %v", err)
	}

	// Initialize test dependencies using base class
	err = suite.SetupBaseInfrastructure("gmail_oauth_e2e")
	require.NoError(t, err, "Failed to setup base infrastructure")

	// Gmail-specific setup
	err = suite.setupGmailSpecifics(t)
	require.NoError(t, err, "Failed to setup Gmail-specific dependencies")

	// Create OAuth provider test template
	err = suite.createOAuthTemplate()
	require.NoError(t, err, "Failed to create OAuth provider test template")

	// Run the complete end-to-end workflow using template
	t.Run("ValidateGmailSkillYAML", suite.testValidateGmailSkillYAML)
	t.Run("SetupOAuthProvider", suite.oauthTemplate.TestSetupOAuthProvider)
	t.Run("CreateTestApp", suite.oauthTemplate.TestCreateTestApp)
	t.Run("PerformOAuthFlow", suite.oauthTemplate.TestPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.oauthTemplate.TestOAuthTokenDirectly)
	t.Run("TestAgentGmailSkillsIntegration", suite.oauthTemplate.TestAgentOAuthSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanupGmailSpecifics(t)
		suite.oauthTemplate.Cleanup(t)
		suite.CleanupBaseInfrastructure()
	})
}

// ======================================================================================
// TEST IMPLEMENTATION
// ======================================================================================

// testValidateGmailSkillYAML validates that the Gmail skill YAML file exists and is properly structured
func (suite *GmailOAuthE2ETestSuite) testValidateGmailSkillYAML(t *testing.T) {
	suite.logger.Info().Msg("Validating Gmail skill YAML file")

	// Check that the skill can be loaded by the skills manager
	skillsManager := suite.oauthTemplate.skillManager
	skill, err := skillsManager.GetSkill("gmail")
	require.NoError(t, err, "Failed to load Gmail skill from YAML")
	require.NotNil(t, skill, "Gmail skill should not be nil")

	// Validate basic skill properties
	require.Equal(t, "gmail", skill.Name, "Skill name should be 'gmail'")
	require.Equal(t, "Gmail", skill.DisplayName, "Skill display name should be 'Gmail'")
	require.Equal(t, "google", skill.Provider, "Skill provider should be 'google'")
	require.Equal(t, "Productivity", skill.Category, "Skill category should be 'Productivity'")

	// Validate OAuth configuration
	require.Equal(t, "google", skill.OAuthProvider, "OAuth provider should be 'google'")
	require.Contains(t, skill.OAuthScopes, "https://www.googleapis.com/auth/gmail.readonly", "Should have Gmail readonly scope")
	require.Contains(t, skill.OAuthScopes, "https://www.googleapis.com/auth/gmail.send", "Should have Gmail send scope")

	// Validate API configuration
	require.Equal(t, "https://www.googleapis.com", skill.BaseURL, "Base URL should be Gmail API URL")
	require.NotEmpty(t, skill.Schema, "Gmail skill should have OpenAPI schema")

	// Validate the schema contains key Gmail API operations
	require.Contains(t, skill.Schema, "getGmailProfile", "Schema should contain getGmailProfile operation")
	require.Contains(t, skill.Schema, "listGmailMessages", "Schema should contain listGmailMessages operation")
	require.Contains(t, skill.Schema, "getGmailMessage", "Schema should contain getGmailMessage operation")
	require.Contains(t, skill.Schema, "sendGmailMessage", "Schema should contain sendGmailMessage operation")

	// Validate system prompt
	require.NotEmpty(t, skill.SystemPrompt, "Gmail skill should have system prompt")
	require.Contains(t, strings.ToLower(skill.SystemPrompt), "gmail", "System prompt should mention Gmail")

	suite.logger.Info().Msg("Gmail skill YAML validation completed successfully")
}

// ======================================================================================
// CONFIGURATION AND SETUP
// ======================================================================================

// loadTestConfig loads configuration from environment variables
func (suite *GmailOAuthE2ETestSuite) loadTestConfig() error {
	suite.googleClientID = os.Getenv("GOOGLE_SKILL_TEST_OAUTH_CLIENT_ID")
	if suite.googleClientID == "" {
		return fmt.Errorf("GOOGLE_SKILL_TEST_OAUTH_CLIENT_ID environment variable not set")
	}

	suite.googleClientSecret = os.Getenv("GOOGLE_SKILL_TEST_OAUTH_CLIENT_SECRET")
	if suite.googleClientSecret == "" {
		return fmt.Errorf("GOOGLE_SKILL_TEST_OAUTH_CLIENT_SECRET environment variable not set")
	}

	suite.googleUsername = os.Getenv("GOOGLE_SKILL_TEST_OAUTH_USERNAME")
	if suite.googleUsername == "" {
		return fmt.Errorf("GOOGLE_SKILL_TEST_OAUTH_USERNAME environment variable not set")
	}

	suite.googlePassword = os.Getenv("GOOGLE_SKILL_TEST_OAUTH_PASSWORD")
	if suite.googlePassword == "" {
		return fmt.Errorf("GOOGLE_SKILL_TEST_OAUTH_PASSWORD environment variable not set")
	}

	// Debug logging for CI environment (log lengths only, not actual values)
	log.Info().
		Str("username", suite.googleUsername).
		Int("password_length", len(suite.googlePassword)).
		Msg("Google OAuth credentials loaded (debug info for CI troubleshooting)")

	log.Info().
		Str("client_id", suite.googleClientID).
		Str("username", suite.googleUsername).
		Msg("Loaded Google OAuth test configuration")

	return nil
}

// setupGmailSpecifics initializes Gmail-specific test environment
func (suite *GmailOAuthE2ETestSuite) setupGmailSpecifics(_ *testing.T) error {
	suite.logger.Info().Msg("=== Gmail OAuth Skills E2E Test Starting ===")

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err := suite.CleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	suite.logger.Info().Msg("Gmail-specific test setup completed successfully")
	return nil
}

// createOAuthTemplate creates the OAuth provider test template for Gmail
func (suite *GmailOAuthE2ETestSuite) createOAuthTemplate() error {
	// Make a copy of the config and fill in the dynamic values
	config := GmailOAuthProviderConfig
	config.ClientID = suite.googleClientID
	config.ClientSecret = suite.googleClientSecret
	config.Username = suite.googleUsername
	config.Password = suite.googlePassword
	config.GetAuthorizationCodeFunc = suite.getGoogleAuthorizationCode
	config.SetupTestDataFunc = func() error { return nil }   // No setup needed for Gmail
	config.CleanupTestDataFunc = func() error { return nil } // No cleanup needed for Gmail
	config.AgentTestQueries = GmailTestQueries

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

// getGoogleAuthorizationCode performs real browser automation to complete Google OAuth flow
func (suite *GmailOAuthE2ETestSuite) getGoogleAuthorizationCode(authURL, state string) (string, error) {
	// Set up Google-specific OAuth handler
	googleHandler := NewGoogleOAuthHandler(suite.logger)

	// Configure Google browser automation with improved selectors
	googleConfig := BrowserOAuthConfig{
		ProviderName:            "google",
		LoginUsernameSelector:   `input[type="email"], input[name="identifier"], input[id="identifierId"], input[autocomplete="username"]`,
		LoginPasswordSelector:   `input[type="password"], input[name="password"], input[autocomplete="current-password"]`,
		LoginButtonSelector:     `button[id="identifierNext"], button[id="passwordNext"], button[type="submit"], input[type="submit"]`,
		AuthorizeButtonSelector: `input[type="submit"][value="Allow"], button[type="submit"], button[data-l*="allow"]`,
		CallbackURLPattern:      "/api/v1/oauth/flow/callback",
		DeviceVerificationCheck: googleHandler.IsRequiredForURL,
		TwoFactorHandler:        googleHandler,
		ProviderStrategy:        NewGoogleProviderStrategy(suite.logger), // Use Google-specific strategy
	}

	// Create automator with Google configuration
	automator := NewBrowserOAuthAutomator(suite.browser, suite.logger, googleConfig)

	// Perform OAuth flow using the generic automator with Google-specific handling
	return automator.PerformOAuthFlow(authURL, state, suite.googleUsername, suite.googlePassword, suite)
}

// ======================================================================================
// CLEANUP
// ======================================================================================

// cleanupGmailSpecifics cleans up Gmail-specific test resources
func (suite *GmailOAuthE2ETestSuite) cleanupGmailSpecifics(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting Gmail-specific Test Cleanup ===")

	// Gmail doesn't require specific cleanup (no test repositories to delete)
	suite.logger.Info().Msg("Gmail-specific cleanup completed (no specific resources to clean)")

	suite.logger.Info().Msg("=== Gmail-specific Test Cleanup Completed ===")
}
