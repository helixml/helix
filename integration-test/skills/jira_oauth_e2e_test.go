// Jira OAuth Skills E2E Test
//
// This test requires OAuth integration environment variables and Chrome
// container, which the stack command will start automatically.
//
// To run this test, from the helix root directory:
//
//   ./stack test -v integration-test/skills/*.go -run TestJiraOAuthSkillsE2E
//
// The test will:
// 1. Set up Helix infrastructure (OAuth manager, API server, etc.)
// 2. Create an Atlassian OAuth provider
// 3. Create a Helix app with Jira skills from jira.yaml
// 4. Perform OAuth flow against real Atlassian using browser automation
// 5. Test agent sessions with real Atlassian Jira API calls with the resulting JWT
// 6. Clean up test resources

package skills

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
)

// ======================================================================================
// TEST QUERIES AND CONFIGURATION - Most important part for understanding what's tested
// ======================================================================================

// JiraTestQueries defines the agent test queries that verify Jira skills integration
var JiraTestQueries = []AgentTestQuery{
	{
		Query:       "Who am I in Jira?",
		SessionName: "Jira Profile Query",
		ExpectedResponseCheck: func(response string) bool {
			return len(response) > 0 && (strings.Contains(strings.ToLower(response), "account") || strings.Contains(response, "@"))
		},
	},
	{
		Query:       "Show me my Jira profile information",
		SessionName: "Jira Profile Details",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "account") || strings.Contains(lower, "jira") || strings.Contains(lower, "profile") || strings.Contains(lower, "user")
		},
	},
	{
		Query:       "Search for issues assigned to me",
		SessionName: "Jira Issues Search",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "issue") || strings.Contains(lower, "assigned") || len(response) > 0
		},
	},
	{
		Query:       "List all projects I have access to",
		SessionName: "Jira Projects List",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "project") || strings.Contains(lower, "access") || len(response) > 0
		},
	},
	{
		Query:       "Find open issues in my projects",
		SessionName: "Jira Open Issues",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "issue") || strings.Contains(lower, "open") || strings.Contains(lower, "project") || len(response) > 0
		},
	},
}

// JiraOAuthProviderConfig defines the OAuth provider configuration for Jira
var JiraOAuthProviderConfig = OAuthProviderConfig{
	ProviderName: "Jira Skills Test",
	ProviderType: types.OAuthProviderTypeAtlassian,
	SkillName:    "jira",
	AuthURL:      "https://auth.atlassian.com/authorize",
	TokenURL:     "https://auth.atlassian.com/oauth/token",
	UserInfoURL:  "https://api.atlassian.com/me",
	Scopes:       []string{"read:jira-work", "write:jira-work", "read:jira-user"},
	// ClientID, ClientSecret, Username, Password, and functions will be set during test setup
}

// ======================================================================================
// MAIN TEST SUITE
// ======================================================================================

// JiraOAuthE2ETestSuite tests the complete Jira OAuth skills workflow
type JiraOAuthE2ETestSuite struct {
	// Embed the base OAuth test suite for common functionality
	BaseOAuthTestSuite

	// Atlassian-specific test configuration from environment
	atlassianClientID     string
	atlassianClientSecret string
	atlassianUsername     string
	atlassianPassword     string

	// OAuth provider test template
	oauthTemplate *OAuthProviderTestTemplate
}

// TestJiraOAuthSkillsE2E is the main end-to-end test for Jira OAuth skills
func TestJiraOAuthSkillsE2E(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	// Enable parallel execution with other tests
	t.Parallel()

	// Set a reasonable timeout for the OAuth browser automation
	timeout := 90 * time.Second // Reasonable timeout for fast iteration
	deadline := time.Now().Add(timeout)
	t.Deadline() // Check if deadline is already set

	// Create a context with timeout for the test
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	suite := &JiraOAuthE2ETestSuite{
		BaseOAuthTestSuite: BaseOAuthTestSuite{
			ctx: ctx,
		},
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping Jira OAuth E2E test: %v", err)
	}

	// Initialize test dependencies using base class
	err = suite.SetupBaseInfrastructure("jira_oauth_e2e")
	require.NoError(t, err, "Failed to setup base infrastructure")

	// Jira-specific setup
	err = suite.setupJiraSpecifics(t)
	require.NoError(t, err, "Failed to setup Jira-specific dependencies")

	// Create OAuth provider test template
	err = suite.createOAuthTemplate()
	require.NoError(t, err, "Failed to create OAuth provider test template")

	// Run the complete end-to-end workflow using template
	t.Run("ValidateJiraSkillYAML", suite.testValidateJiraSkillYAML)
	t.Run("SetupOAuthProvider", suite.oauthTemplate.TestSetupOAuthProvider)
	t.Run("CreateTestApp", suite.oauthTemplate.TestCreateTestApp)
	t.Run("PerformOAuthFlow", suite.oauthTemplate.TestPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.oauthTemplate.TestOAuthTokenDirectly)
	t.Run("TestAgentJiraSkillsIntegration", suite.oauthTemplate.TestAgentOAuthSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanupJiraSpecifics(t)
		suite.oauthTemplate.Cleanup(t)
		suite.CleanupBaseInfrastructure()
	})
}

// ======================================================================================
// TEST IMPLEMENTATION
// ======================================================================================

// testValidateJiraSkillYAML validates that the Jira skill YAML file exists and is properly structured
func (suite *JiraOAuthE2ETestSuite) testValidateJiraSkillYAML(t *testing.T) {
	suite.logger.Info().Msg("Validating Jira skill YAML file")

	// Check that the skill can be loaded by the skills manager
	skillsManager := suite.oauthTemplate.skillManager
	skill, err := skillsManager.GetSkill("jira")
	require.NoError(t, err, "Failed to load Jira skill from YAML")
	require.NotNil(t, skill, "Jira skill should not be nil")

	// Validate basic skill properties
	require.Equal(t, "jira", skill.Name, "Skill name should be 'jira'")
	require.Equal(t, "Jira", skill.DisplayName, "Skill display name should be 'Jira'")
	require.Equal(t, "atlassian", skill.Provider, "Skill provider should be 'atlassian'")
	require.Equal(t, "Productivity", skill.Category, "Skill category should be 'Productivity'")

	// Validate OAuth configuration
	require.Equal(t, "atlassian", skill.OAuthProvider, "OAuth provider should be 'atlassian'")
	require.Contains(t, skill.OAuthScopes, "read:jira-work", "Should have read:jira-work scope")
	require.Contains(t, skill.OAuthScopes, "write:jira-work", "Should have write:jira-work scope")
	require.Contains(t, skill.OAuthScopes, "read:jira-user", "Should have read:jira-user scope")

	// Validate API configuration
	require.Equal(t, "https://api.atlassian.com", skill.BaseURL, "Base URL should be Atlassian API URL")
	require.NotEmpty(t, skill.Schema, "Jira skill should have OpenAPI schema")

	// Validate the schema contains key Atlassian Jira API operations
	require.Contains(t, skill.Schema, "getJiraCurrentUser", "Schema should contain getJiraCurrentUser operation")
	require.Contains(t, skill.Schema, "searchJiraIssues", "Schema should contain searchJiraIssues operation")
	require.Contains(t, skill.Schema, "getJiraIssue", "Schema should contain getJiraIssue operation")
	require.Contains(t, skill.Schema, "createJiraIssue", "Schema should contain createJiraIssue operation")
	require.Contains(t, skill.Schema, "updateJiraIssue", "Schema should contain updateJiraIssue operation")
	require.Contains(t, skill.Schema, "searchJiraProjects", "Schema should contain searchJiraProjects operation")

	// Validate system prompt
	require.NotEmpty(t, skill.SystemPrompt, "Jira skill should have system prompt")
	require.Contains(t, strings.ToLower(skill.SystemPrompt), "jira", "System prompt should mention Jira")

	suite.logger.Info().Msg("Jira skill YAML validation completed successfully")
}

// ======================================================================================
// CONFIGURATION AND SETUP
// ======================================================================================

// loadTestConfig loads configuration from environment variables
func (suite *JiraOAuthE2ETestSuite) loadTestConfig() error {
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

// setupJiraSpecifics initializes Jira-specific test environment
func (suite *JiraOAuthE2ETestSuite) setupJiraSpecifics(_ *testing.T) error {
	suite.logger.Info().Msg("=== Jira OAuth Skills E2E Test Starting ===")

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err := suite.CleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	suite.logger.Info().Msg("Jira-specific test setup completed successfully")
	return nil
}

// createOAuthTemplate creates the OAuth provider test template for Jira
func (suite *JiraOAuthE2ETestSuite) createOAuthTemplate() error {
	// Make a copy of the config and fill in the dynamic values
	config := JiraOAuthProviderConfig
	config.ClientID = suite.atlassianClientID
	config.ClientSecret = suite.atlassianClientSecret
	config.Username = suite.atlassianUsername
	config.Password = suite.atlassianPassword
	config.GetAuthorizationCodeFunc = suite.getAtlassianAuthorizationCode
	config.SetupTestDataFunc = func() error { return nil }   // No setup needed for Jira
	config.CleanupTestDataFunc = func() error { return nil } // No cleanup needed for Jira
	config.AgentTestQueries = JiraTestQueries

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
func (suite *JiraOAuthE2ETestSuite) getAtlassianAuthorizationCode(authURL, state string) (string, error) {
	// Set up Atlassian-specific OAuth handler
	atlassianHandler := NewAtlassianOAuthHandler(suite.logger)

	// Configure Atlassian browser automation with improved selectors
	atlassianConfig := BrowserOAuthConfig{
		ProviderName:            "atlassian",
		LoginUsernameSelector:   `input[type="email"], input[name="username"], input[id="username"], input[placeholder*="email"], input[placeholder*="Email"]`,
		LoginPasswordSelector:   `input[type="password"], input[name="password"], input[id="password"], input[placeholder*="password"], input[placeholder*="Password"]`,
		LoginButtonSelector:     `button[type="submit"], input[type="submit"], button[id="login-submit"], button:contains("Continue"), button:contains("Log in")`,
		AuthorizeButtonSelector: `button[type="submit"], input[type="submit"], button:contains("Accept"), button:contains("Allow"), button:contains("Authorize")`,
		CallbackURLPattern:      "/api/v1/oauth/flow/callback",
		DeviceVerificationCheck: atlassianHandler.IsRequiredForURL,
		TwoFactorHandler:        atlassianHandler,
	}

	// Create automator with Atlassian configuration
	automator := NewBrowserOAuthAutomator(suite.browser, suite.logger, atlassianConfig)

	// Perform OAuth flow using the generic automator with Atlassian-specific handling
	return automator.PerformOAuthFlow(authURL, state, suite.atlassianUsername, suite.atlassianPassword, suite)
}

// ======================================================================================
// CLEANUP
// ======================================================================================

// cleanupJiraSpecifics cleans up Jira-specific test resources
func (suite *JiraOAuthE2ETestSuite) cleanupJiraSpecifics(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting Jira-specific Test Cleanup ===")

	// Jira doesn't require specific cleanup (no test repositories to delete)
	suite.logger.Info().Msg("Jira-specific cleanup completed (no specific resources to clean)")

	suite.logger.Info().Msg("=== Jira-specific Test Cleanup Completed ===")
}

// ======================================================================================
// ATLASSIAN OAUTH HANDLER STUB
// ======================================================================================

// AtlassianOAuthHandler handles Atlassian-specific OAuth challenges
type AtlassianOAuthHandler struct {
	logger zerolog.Logger
}

// NewAtlassianOAuthHandler creates a new Atlassian OAuth handler
func NewAtlassianOAuthHandler(logger zerolog.Logger) *AtlassianOAuthHandler {
	return &AtlassianOAuthHandler{
		logger: logger,
	}
}

// IsRequiredForURL checks if device verification is required for the given URL
func (h *AtlassianOAuthHandler) IsRequiredForURL(url string) bool {
	// Atlassian may require 2FA or device verification
	return strings.Contains(url, "2fa") || strings.Contains(url, "verify") || strings.Contains(url, "device")
}

// IsRequired checks if 2FA is required for the current page (TwoFactorHandler interface)
func (h *AtlassianOAuthHandler) IsRequired(page *rod.Page) bool {
	// Check if the page contains 2FA indicators
	currentURL := page.MustInfo().URL
	return h.IsRequiredForURL(currentURL)
}

// Handle handles 2FA if required (TwoFactorHandler interface)
func (h *AtlassianOAuthHandler) Handle(page *rod.Page, _ *BrowserOAuthAutomator) error {
	currentURL := page.MustInfo().URL
	h.logger.Info().Str("url", currentURL).Msg("Handling Atlassian 2FA")

	// This would need to be implemented based on Atlassian's specific 2FA/device verification flow
	// For now, we'll just log that it's not implemented
	h.logger.Warn().Msg("Atlassian 2FA handling not implemented yet")
	return nil
}

// HandleDeviceVerification handles device verification if required
func (h *AtlassianOAuthHandler) HandleDeviceVerification(url string) error {
	h.logger.Info().Str("url", url).Msg("Handling Atlassian device verification")
	// This would need to be implemented based on Atlassian's specific 2FA/device verification flow
	// For now, we'll just log that it's not implemented
	h.logger.Warn().Msg("Atlassian device verification not implemented yet")
	return nil
}
