// Outlook Calendar OAuth Skills E2E Test
//
// This test requires OAuth integration environment variables and Chrome
// container, which the stack command will start automatically.
//
// To run this test, from the helix root directory:
//
//   ./stack test -v integration-test/skills/*.go -run TestOutlookCalendarOAuthSkillsE2E
//
// The test will:
// 1. Set up Helix infrastructure (OAuth manager, API server, etc.)
// 2. Create a Microsoft OAuth provider
// 3. Create a Helix app with Outlook Calendar skills from outlook_calendar.yaml
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

// OutlookCalendarTestQueries defines the agent test queries that verify Outlook Calendar skills integration
var OutlookCalendarTestQueries = []AgentTestQuery{
	{
		Query:       "Show me my Outlook profile information",
		SessionName: "Calendar Profile Query",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "email") || strings.Contains(lower, "profile") || strings.Contains(response, "@")
		},
	},
	{
		Query:       "List my Outlook calendars",
		SessionName: "Calendar List Query",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "calendar") || strings.Contains(lower, "default") || len(response) > 0
		},
	},
	{
		Query:       "Show me my calendar events for today",
		SessionName: "Today's Events Query",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "event") || strings.Contains(lower, "calendar") || strings.Contains(lower, "today") || len(response) > 0
		},
	},
	{
		Query:       "Get my default calendar information",
		SessionName: "Default Calendar Query",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "default") || strings.Contains(lower, "calendar") || len(response) > 0
		},
	},
	{
		Query:       "Show me my upcoming meetings this week",
		SessionName: "Upcoming Meetings Query",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "meeting") || strings.Contains(lower, "event") || strings.Contains(lower, "week") || len(response) > 0
		},
	},
}

// OutlookCalendarOAuthProviderConfig defines the OAuth provider configuration for Outlook Calendar
var OutlookCalendarOAuthProviderConfig = OAuthProviderConfig{
	ProviderName: "Outlook Calendar Skills Test",
	ProviderType: types.OAuthProviderTypeMicrosoft,
	SkillName:    "outlook_calendar",
	AuthURL:      "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	UserInfoURL:  "https://graph.microsoft.com/v1.0/me",
	Scopes:       []string{"https://graph.microsoft.com/User.Read", "https://graph.microsoft.com/Calendars.Read", "https://graph.microsoft.com/Calendars.ReadWrite"},
	// ClientID, ClientSecret, Username, Password, and functions will be set during test setup
}

// ======================================================================================
// MAIN TEST SUITE
// ======================================================================================

// OutlookCalendarOAuthE2ETestSuite tests the complete Outlook Calendar OAuth skills workflow
type OutlookCalendarOAuthE2ETestSuite struct {
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

// TestOutlookCalendarOAuthSkillsE2E is the main end-to-end test for Outlook Calendar OAuth skills
func TestOutlookCalendarOAuthSkillsE2E(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	// Enable parallel execution with other tests
	t.Parallel()

	// Set a reasonable timeout for the OAuth browser automation
	timeout := 2 * time.Minute // Increased timeout for Microsoft OAuth flow
	deadline := time.Now().Add(timeout)
	t.Deadline() // Check if deadline is already set

	// Create a context with timeout for the test
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	suite := &OutlookCalendarOAuthE2ETestSuite{
		BaseOAuthTestSuite: BaseOAuthTestSuite{
			ctx: ctx,
		},
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping Outlook Calendar OAuth E2E test: %v", err)
	}

	// Initialize test dependencies using base class
	err = suite.SetupBaseInfrastructure("ocal_oauth_e2e")
	require.NoError(t, err, "Failed to setup base infrastructure")

	// Outlook Calendar-specific setup
	err = suite.setupOutlookCalendarSpecifics(t)
	require.NoError(t, err, "Failed to setup Outlook Calendar-specific dependencies")

	// Create OAuth provider test template
	err = suite.createOAuthTemplate()
	require.NoError(t, err, "Failed to create OAuth provider test template")

	// Run the complete end-to-end workflow using template
	t.Run("ValidateOutlookCalendarSkillYAML", suite.testValidateOutlookCalendarSkillYAML)
	t.Run("SetupOAuthProvider", suite.oauthTemplate.TestSetupOAuthProvider)
	t.Run("CreateTestApp", suite.oauthTemplate.TestCreateTestApp)
	t.Run("PerformOAuthFlow", suite.oauthTemplate.TestPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.oauthTemplate.TestOAuthTokenDirectly)
	t.Run("TestAgentOutlookCalendarSkillsIntegration", suite.oauthTemplate.TestAgentOAuthSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanupOutlookCalendarSpecifics(t)
		suite.oauthTemplate.Cleanup(t)
		suite.CleanupBaseInfrastructure()
	})
}

// ======================================================================================
// TEST IMPLEMENTATION
// ======================================================================================

// testValidateOutlookCalendarSkillYAML validates that the Outlook Calendar skill YAML file exists and is properly structured
func (suite *OutlookCalendarOAuthE2ETestSuite) testValidateOutlookCalendarSkillYAML(t *testing.T) {
	suite.logger.Info().Msg("Validating Outlook Calendar skill YAML file")

	// Check that the skill can be loaded by the skills manager
	skillsManager := suite.oauthTemplate.skillManager
	skill, err := skillsManager.GetSkill("outlook_calendar")
	require.NoError(t, err, "Failed to load Outlook Calendar skill from YAML")
	require.NotNil(t, skill, "Outlook Calendar skill should not be nil")

	// Validate basic skill properties
	require.Equal(t, "outlook_calendar", skill.Name, "Skill name should be 'outlook_calendar'")
	require.Equal(t, "Outlook Calendar", skill.DisplayName, "Skill display name should be 'Outlook Calendar'")
	require.Equal(t, "microsoft", skill.Provider, "Skill provider should be 'microsoft'")
	require.Equal(t, "Productivity", skill.Category, "Skill category should be 'Productivity'")

	// Validate OAuth configuration
	require.Equal(t, "microsoft", skill.OAuthProvider, "OAuth provider should be 'microsoft'")
	require.Contains(t, skill.OAuthScopes, "https://graph.microsoft.com/User.Read", "Should have User.Read scope")
	require.Contains(t, skill.OAuthScopes, "https://graph.microsoft.com/Calendars.Read", "Should have Calendars.Read scope")
	require.Contains(t, skill.OAuthScopes, "https://graph.microsoft.com/Calendars.ReadWrite", "Should have Calendars.ReadWrite scope")

	// Validate API configuration
	require.Equal(t, "https://graph.microsoft.com", skill.BaseURL, "Base URL should be Microsoft Graph API URL")
	require.NotEmpty(t, skill.Schema, "Outlook Calendar skill should have OpenAPI schema")

	// Validate the schema contains key Microsoft Graph Calendar API operations
	require.Contains(t, skill.Schema, "getCalendarProfile", "Schema should contain getCalendarProfile operation")
	require.Contains(t, skill.Schema, "listCalendarEvents", "Schema should contain listCalendarEvents operation")
	require.Contains(t, skill.Schema, "getCalendarEvent", "Schema should contain getCalendarEvent operation")
	require.Contains(t, skill.Schema, "createCalendarEvent", "Schema should contain createCalendarEvent operation")
	require.Contains(t, skill.Schema, "updateCalendarEvent", "Schema should contain updateCalendarEvent operation")
	require.Contains(t, skill.Schema, "deleteCalendarEvent", "Schema should contain deleteCalendarEvent operation")
	require.Contains(t, skill.Schema, "listCalendars", "Schema should contain listCalendars operation")
	require.Contains(t, skill.Schema, "getDefaultCalendar", "Schema should contain getDefaultCalendar operation")
	require.Contains(t, skill.Schema, "getCalendarView", "Schema should contain getCalendarView operation")

	// Validate system prompt
	require.NotEmpty(t, skill.SystemPrompt, "Outlook Calendar skill should have system prompt")
	require.Contains(t, strings.ToLower(skill.SystemPrompt), "calendar", "System prompt should mention calendar")

	suite.logger.Info().Msg("Outlook Calendar skill YAML validation completed successfully")
}

// ======================================================================================
// CONFIGURATION AND SETUP
// ======================================================================================

// loadTestConfig loads configuration from environment variables
func (suite *OutlookCalendarOAuthE2ETestSuite) loadTestConfig() error {
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
		Msg("Loaded Microsoft OAuth test configuration for Calendar")

	return nil
}

// setupOutlookCalendarSpecifics initializes Outlook Calendar-specific test environment
func (suite *OutlookCalendarOAuthE2ETestSuite) setupOutlookCalendarSpecifics(_ *testing.T) error {
	suite.logger.Info().Msg("=== Outlook Calendar OAuth Skills E2E Test Starting ===")

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err := suite.CleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	suite.logger.Info().Msg("Outlook Calendar-specific test setup completed successfully")
	return nil
}

// createOAuthTemplate creates the OAuth provider test template for Outlook Calendar
func (suite *OutlookCalendarOAuthE2ETestSuite) createOAuthTemplate() error {
	// Make a copy of the config and fill in the dynamic values
	config := OutlookCalendarOAuthProviderConfig
	config.ClientID = suite.microsoftClientID
	config.ClientSecret = suite.microsoftClientSecret
	config.Username = suite.microsoftUsername
	config.Password = suite.microsoftPassword
	config.GetAuthorizationCodeFunc = suite.getMicrosoftAuthorizationCode
	config.SetupTestDataFunc = func() error { return nil }   // No setup needed for Outlook Calendar
	config.CleanupTestDataFunc = func() error { return nil } // No cleanup needed for Outlook Calendar
	config.AgentTestQueries = OutlookCalendarTestQueries

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
func (suite *OutlookCalendarOAuthE2ETestSuite) getMicrosoftAuthorizationCode(authURL, state string) (string, error) {
	// Set up Microsoft-specific OAuth handler
	microsoftHandler := NewMicrosoftOAuthHandler(suite.logger)

	// Configure Microsoft browser automation with improved selectors
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

	// Create automator with Microsoft configuration
	automator := NewBrowserOAuthAutomator(suite.browser, suite.logger, microsoftConfig)

	// Perform OAuth flow using the generic automator with Microsoft-specific handling
	return automator.PerformOAuthFlow(authURL, state, suite.microsoftUsername, suite.microsoftPassword, suite)
}

// ======================================================================================
// CLEANUP
// ======================================================================================

// cleanupOutlookCalendarSpecifics cleans up Outlook Calendar-specific test resources
func (suite *OutlookCalendarOAuthE2ETestSuite) cleanupOutlookCalendarSpecifics(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting Outlook Calendar-specific Test Cleanup ===")

	// Outlook Calendar doesn't require specific cleanup (no test resources to delete)
	suite.logger.Info().Msg("Outlook Calendar-specific cleanup completed (no specific resources to clean)")

	suite.logger.Info().Msg("=== Outlook Calendar-specific Test Cleanup Completed ===")
}
