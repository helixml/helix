// Google Calendar OAuth Skills E2E Test
//
// This test requires OAuth integration environment variables and Chrome
// container, which the stack command will start automatically.
//
// To run this test, from the helix root directory:
//
//   ./stack test -v integration-test/skills/*.go -run TestGoogleCalendarOAuthSkillsE2E
//
// The test will:
// 1. Set up Helix infrastructure (OAuth manager, API server, etc.)
// 2. Create a Google OAuth provider
// 3. Create a Helix app with Google Calendar skills from google-calendar.yaml
// 4. Perform OAuth flow against real Google using browser automation
// 5. Test agent sessions with real Google Calendar API calls with the resulting JWT
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

// GoogleCalendarTestQueries defines the agent test queries that verify Google Calendar skills integration
var GoogleCalendarTestQueries = []AgentTestQuery{
	{
		Query:       "What calendars do I have access to?",
		SessionName: "Google Calendar List",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "calendar") || len(response) > 0
		},
	},
	{
		Query:       "Show me my primary calendar information",
		SessionName: "Google Calendar Primary",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "calendar") || strings.Contains(lower, "primary") || len(response) > 0
		},
	},
	{
		Query:       "List my upcoming events",
		SessionName: "Google Calendar Events",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "event") || strings.Contains(lower, "calendar") || strings.Contains(lower, "schedule") || len(response) > 0
		},
	},
	{
		Query:       "What events do I have today?",
		SessionName: "Google Calendar Today",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "today") || strings.Contains(lower, "event") || strings.Contains(lower, "schedule") || len(response) > 0
		},
	},
	{
		Query:       "Show me my schedule for this week",
		SessionName: "Google Calendar Week",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "week") || strings.Contains(lower, "schedule") || strings.Contains(lower, "event") || len(response) > 0
		},
	},
}

// GoogleCalendarOAuthProviderConfig defines the OAuth provider configuration for Google Calendar
var GoogleCalendarOAuthProviderConfig = OAuthProviderConfig{
	ProviderName: "Google Calendar Skills Test",
	ProviderType: types.OAuthProviderTypeGoogle,
	SkillName:    "google-calendar",
	AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:     "https://oauth2.googleapis.com/token",
	UserInfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
	Scopes:       []string{"openid", "email", "profile", "https://www.googleapis.com/auth/calendar", "https://www.googleapis.com/auth/calendar.events"},
	// ClientID, ClientSecret, Username, Password, and functions will be set during test setup
}

// ======================================================================================
// MAIN TEST SUITE
// ======================================================================================

// GoogleCalendarOAuthE2ETestSuite tests the complete Google Calendar OAuth skills workflow
type GoogleCalendarOAuthE2ETestSuite struct {
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

// TestGoogleCalendarOAuthSkillsE2E is the main end-to-end test for Google Calendar OAuth skills
func TestGoogleCalendarOAuthSkillsE2E(t *testing.T) {
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

	suite := &GoogleCalendarOAuthE2ETestSuite{
		BaseOAuthTestSuite: BaseOAuthTestSuite{
			ctx: ctx,
		},
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping Google Calendar OAuth E2E test: %v", err)
	}

	// Initialize test dependencies using base class
	err = suite.SetupBaseInfrastructure("gcal_oauth_e2e")
	require.NoError(t, err, "Failed to setup base infrastructure")

	// Google Calendar-specific setup
	err = suite.setupGoogleCalendarSpecifics(t)
	require.NoError(t, err, "Failed to setup Google Calendar-specific dependencies")

	// Create OAuth provider test template
	err = suite.createOAuthTemplate()
	require.NoError(t, err, "Failed to create OAuth provider test template")

	// Run the complete end-to-end workflow using template
	t.Run("ValidateGoogleCalendarSkillYAML", suite.testValidateGoogleCalendarSkillYAML)
	t.Run("SetupOAuthProvider", suite.oauthTemplate.TestSetupOAuthProvider)
	t.Run("CreateTestApp", suite.oauthTemplate.TestCreateTestApp)
	t.Run("PerformOAuthFlow", suite.oauthTemplate.TestPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.oauthTemplate.TestOAuthTokenDirectly)
	t.Run("TestAgentGoogleCalendarSkillsIntegration", suite.oauthTemplate.TestAgentOAuthSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanupGoogleCalendarSpecifics(t)
		suite.oauthTemplate.Cleanup(t)
		suite.CleanupBaseInfrastructure()
	})
}

// ======================================================================================
// TEST IMPLEMENTATION
// ======================================================================================

// testValidateGoogleCalendarSkillYAML validates that the Google Calendar skill YAML file exists and is properly structured
func (suite *GoogleCalendarOAuthE2ETestSuite) testValidateGoogleCalendarSkillYAML(t *testing.T) {
	suite.logger.Info().Msg("Validating Google Calendar skill YAML file")

	// Check that the skill can be loaded by the skills manager
	skillsManager := suite.oauthTemplate.skillManager
	skill, err := skillsManager.GetSkill("google-calendar")
	require.NoError(t, err, "Failed to load Google Calendar skill from YAML")
	require.NotNil(t, skill, "Google Calendar skill should not be nil")

	// Validate basic skill properties
	require.Equal(t, "google-calendar", skill.Name, "Skill name should be 'google-calendar'")
	require.Equal(t, "Google Calendar", skill.DisplayName, "Skill display name should be 'Google Calendar'")
	require.Equal(t, "google", skill.Provider, "Skill provider should be 'google'")
	require.Equal(t, "Productivity", skill.Category, "Skill category should be 'Productivity'")

	// Validate OAuth configuration
	require.Equal(t, "google", skill.OAuthProvider, "OAuth provider should be 'google'")
	require.Contains(t, skill.OAuthScopes, "https://www.googleapis.com/auth/calendar", "Should have Google Calendar scope")
	require.Contains(t, skill.OAuthScopes, "https://www.googleapis.com/auth/calendar.events", "Should have Google Calendar events scope")

	// Validate API configuration
	require.Equal(t, "https://www.googleapis.com", skill.BaseURL, "Base URL should be Google API URL")
	require.NotEmpty(t, skill.Schema, "Google Calendar skill should have OpenAPI schema")

	// Validate the schema contains key Google Calendar API operations
	require.Contains(t, skill.Schema, "getPrimaryCalendar", "Schema should contain getPrimaryCalendar operation")
	require.Contains(t, skill.Schema, "listCalendars", "Schema should contain listCalendars operation")
	require.Contains(t, skill.Schema, "listCalendarEvents", "Schema should contain listCalendarEvents operation")
	require.Contains(t, skill.Schema, "createCalendarEvent", "Schema should contain createCalendarEvent operation")
	require.Contains(t, skill.Schema, "getCalendarEvent", "Schema should contain getCalendarEvent operation")
	require.Contains(t, skill.Schema, "updateCalendarEvent", "Schema should contain updateCalendarEvent operation")
	require.Contains(t, skill.Schema, "deleteCalendarEvent", "Schema should contain deleteCalendarEvent operation")

	// Validate system prompt
	require.NotEmpty(t, skill.SystemPrompt, "Google Calendar skill should have system prompt")
	require.Contains(t, strings.ToLower(skill.SystemPrompt), "calendar", "System prompt should mention calendar")

	suite.logger.Info().Msg("Google Calendar skill YAML validation completed successfully")
}

// ======================================================================================
// CONFIGURATION AND SETUP
// ======================================================================================

// loadTestConfig loads configuration from environment variables
func (suite *GoogleCalendarOAuthE2ETestSuite) loadTestConfig() error {
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

// setupGoogleCalendarSpecifics initializes Google Calendar-specific test environment
func (suite *GoogleCalendarOAuthE2ETestSuite) setupGoogleCalendarSpecifics(_ *testing.T) error {
	suite.logger.Info().Msg("=== Google Calendar OAuth Skills E2E Test Starting ===")

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err := suite.CleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	suite.logger.Info().Msg("Google Calendar-specific test setup completed successfully")
	return nil
}

// createOAuthTemplate creates the OAuth provider test template for Google Calendar
func (suite *GoogleCalendarOAuthE2ETestSuite) createOAuthTemplate() error {
	// Make a copy of the config and fill in the dynamic values
	config := GoogleCalendarOAuthProviderConfig
	config.ClientID = suite.googleClientID
	config.ClientSecret = suite.googleClientSecret
	config.Username = suite.googleUsername
	config.Password = suite.googlePassword
	config.GetAuthorizationCodeFunc = suite.getGoogleCalendarAuthorizationCode
	config.SetupTestDataFunc = func() error { return nil }   // No setup needed for Google Calendar
	config.CleanupTestDataFunc = func() error { return nil } // No cleanup needed for Google Calendar
	config.AgentTestQueries = GoogleCalendarTestQueries

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

// getGoogleCalendarAuthorizationCode performs real browser automation to complete Google Calendar OAuth flow
func (suite *GoogleCalendarOAuthE2ETestSuite) getGoogleCalendarAuthorizationCode(authURL, state string) (string, error) {
	// Set up Google-specific OAuth handler
	googleHandler := NewGoogleOAuthHandler(suite.logger)

	// Configure Google browser automation with improved selectors
	googleConfig := BrowserOAuthConfig{
		ProviderName:            "google",
		LoginUsernameSelector:   `input[type="email"], input[name="identifier"], input[id="identifierId"], input[autocomplete="username"]`,
		LoginPasswordSelector:   `input[type="password"], input[name="password"], input[autocomplete="current-password"]`,
		LoginButtonSelector:     `button[id="identifierNext"], button[id="passwordNext"], button[type="submit"], input[type="submit"]`,
		AuthorizeButtonSelector: `button.VfPpkd-LgbsSe, button[type="submit"], input[type="submit"][value="Allow"], button[data-l*="allow"]`, // FIXED: Use same working pattern as Gmail
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

// cleanupGoogleCalendarSpecifics cleans up Google Calendar-specific test resources
func (suite *GoogleCalendarOAuthE2ETestSuite) cleanupGoogleCalendarSpecifics(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting Google Calendar-specific Test Cleanup ===")

	// Google Calendar doesn't require specific cleanup (no test repositories to delete)
	suite.logger.Info().Msg("Google Calendar-specific cleanup completed (no specific resources to clean)")

	suite.logger.Info().Msg("=== Google Calendar-specific Test Cleanup Completed ===")
}
