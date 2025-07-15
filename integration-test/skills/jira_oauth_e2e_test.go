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
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/helixml/helix/api/pkg/types"
)

// getJiraScopes loads OAuth scopes from jira.yaml and adds additional OAuth flow scopes
func getJiraScopes() []string {
	jiraScopes, err := loadScopesFromYAML("jira")
	if err != nil {
		panic(fmt.Sprintf("failed to load scopes from jira.yaml: %v", err))
	}

	// Add additional scopes needed for the OAuth flow itself
	allScopes := append(jiraScopes, "read:me", "read:account")
	return allScopes
}

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
	Scopes:       getJiraScopes(),
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
	atlassianCloudID      string

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
	timeout := 5 * time.Minute // Very long timeout for Atlassian browser automation
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
	t.Run("TestOAuthDebugging", suite.oauthTemplate.TestOAuthDebugging)
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
	require.Contains(t, skill.OAuthScopes, "read:me", "Should have read:me scope")
	require.Contains(t, skill.OAuthScopes, "read:account", "Should have read:account scope")

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

	suite.atlassianCloudID = os.Getenv("ATLASSIAN_SKILL_TEST_JIRA_CLOUD_ID")
	if suite.atlassianCloudID == "" {
		return fmt.Errorf("ATLASSIAN_SKILL_TEST_JIRA_CLOUD_ID environment variable not set")
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
func (suite *JiraOAuthE2ETestSuite) getAtlassianAuthorizationCode(authURL, state string) (string, error) {
	// Start enhanced screenshot capture for the OAuth flow
	suite.logger.Info().Msg("Starting enhanced screenshot capture for Jira OAuth flow")
	// err := suite.StartEnhancedScreenshotCapture("jira_oauth_flow")
	// if err != nil {
	// 	suite.logger.Error().Err(err).Msg("Failed to start enhanced screenshot capture, continuing without recording")
	// }

	// Ensure enhanced screenshot capture is stopped regardless of outcome
	// defer func() {
	// 	suite.logger.Info().Msg("Stopping enhanced screenshot capture for Jira OAuth flow")
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
	atlassianStrategy := NewAtlassianProviderStrategy(suite.logger, "helixml.atlassian.net")
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

// AtlassianOAuthHandler handles Atlassian-specific OAuth challenges including MFA
type AtlassianOAuthHandler struct {
	gmailService           *gmail.Service
	logger                 zerolog.Logger
	gmailCredentialsBase64 string
}

// NewAtlassianOAuthHandler creates a new Atlassian OAuth handler with Gmail integration
func NewAtlassianOAuthHandler(gmailCredentialsBase64 string, logger zerolog.Logger) (*AtlassianOAuthHandler, error) {
	logger.Info().Msg("Creating new Atlassian OAuth handler")

	handler := &AtlassianOAuthHandler{
		gmailCredentialsBase64: gmailCredentialsBase64,
		logger:                 logger,
	}

	// Only set up Gmail service if credentials are provided
	if gmailCredentialsBase64 != "" {
		logger.Info().Msg("Gmail credentials provided, setting up Gmail service")
		err := handler.setupGmailService()
		if err != nil {
			logger.Error().Err(err).Msg("Failed to setup Gmail service")
			return nil, fmt.Errorf("failed to setup Gmail service: %w", err)
		}
		logger.Info().Msg("Gmail service setup completed successfully")
	} else {
		logger.Warn().Msg("No Gmail credentials provided, Gmail integration will be unavailable")
	}

	logger.Info().Msg("Atlassian OAuth handler created successfully")
	return handler, nil
}

// IsRequiredForURL checks if device verification is required for the given URL
func (h *AtlassianOAuthHandler) IsRequiredForURL(url string) bool {
	// Atlassian MFA pages contain these patterns
	return strings.Contains(url, "/login/mfa") || strings.Contains(url, "mfa") || strings.Contains(url, "2fa") || strings.Contains(url, "verify") || strings.Contains(url, "device")
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
	h.logger.Info().Str("url", currentURL).Msg("Handling Atlassian MFA")

	// Strategy 1: Try to find and click skip/alternative options
	h.logger.Info().Msg("Strategy 1: Attempting to skip Atlassian MFA")
	err := h.trySkipMFA(page)
	if err == nil {
		h.logger.Info().Msg("Successfully skipped Atlassian MFA")
		return nil
	}

	h.logger.Info().Err(err).Msg("Could not skip MFA, trying Gmail integration")

	// Strategy 2: Use Gmail integration if available
	if h.gmailService != nil {
		h.logger.Info().Msg("Strategy 2: Using Gmail integration for MFA")
		err = h.handleMFAWithGmail(page)
		if err == nil {
			h.logger.Info().Msg("Successfully handled Atlassian MFA using Gmail")
			return nil
		}
		h.logger.Warn().Err(err).Msg("Gmail MFA handling failed")
	} else {
		h.logger.Warn().Msg("Gmail service not available, skipping Gmail integration strategy")
	}

	// Strategy 3: Return informative error
	h.logger.Error().Str("url", currentURL).Msg("All MFA strategies failed")
	return fmt.Errorf("Atlassian MFA detected but could not be handled automatically. URL: %s. Please disable MFA for the test account or implement manual MFA handling", currentURL)
}

// HandleDeviceVerification handles device verification if required
func (h *AtlassianOAuthHandler) HandleDeviceVerification(url string) error {
	h.logger.Info().Str("url", url).Msg("Handling Atlassian device verification")

	// Use the same logic as Handle but without a page object
	if h.gmailService != nil {
		h.logger.Info().Msg("Gmail service available for device verification")
		return fmt.Errorf("device verification with Gmail requires a page object - use Handle method instead")
	}

	return fmt.Errorf("Atlassian device verification detected but Gmail integration not available. URL: %s", url)
}

// setupGmailService initializes the Gmail API service for MFA code reading
func (h *AtlassianOAuthHandler) setupGmailService() error {
	h.logger.Info().Msg("Setting up Gmail service for Atlassian MFA")

	// Decode base64 credentials
	h.logger.Debug().Msg("Decoding Gmail credentials from base64")
	credentials, err := base64.StdEncoding.DecodeString(h.gmailCredentialsBase64)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to decode Gmail credentials")
		return fmt.Errorf("failed to decode Gmail credentials: %w", err)
	}

	h.logger.Debug().Int("credentials_length", len(credentials)).Msg("Gmail credentials decoded successfully")

	// Create Gmail service with service account credentials and domain-wide delegation
	h.logger.Debug().Msg("Creating background context for Gmail service")
	ctx := context.Background()

	// Parse the service account credentials
	h.logger.Debug().Msg("Parsing service account credentials from JSON")
	config, err := google.JWTConfigFromJSON(credentials, gmail.GmailReadonlyScope)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to parse Gmail service account credentials")
		return fmt.Errorf("failed to parse Gmail credentials: %w", err)
	}

	h.logger.Debug().Str("client_email", config.Email).Msg("Service account credentials parsed successfully")

	// Set the subject to impersonate the test@helix.ml user
	h.logger.Debug().Msg("Setting subject for domain-wide delegation")
	config.Subject = "test@helix.ml"

	h.logger.Debug().Str("subject", config.Subject).Msg("Domain-wide delegation configured")

	// Create HTTP client with the JWT config
	h.logger.Debug().Msg("Creating HTTP client with JWT configuration")
	client := config.Client(ctx)

	// Create Gmail service
	h.logger.Debug().Msg("Creating Gmail service with HTTP client")
	service, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to create Gmail service")
		return fmt.Errorf("failed to create Gmail service: %w", err)
	}

	h.logger.Debug().Msg("Gmail service created successfully")
	h.gmailService = service
	h.logger.Info().Msg("Gmail service setup completed successfully")
	return nil
}

// trySkipMFA attempts to skip MFA by finding skip/alternative options
func (h *AtlassianOAuthHandler) trySkipMFA(page *rod.Page) error {
	h.logger.Info().Msg("Trying to skip Atlassian MFA")

	// Common skip button selectors for Atlassian
	skipSelectors := []string{
		`button[value="Skip"]`,
		`input[type="submit"][value="Skip"]`,
		`button:contains("Skip")`,
		`button:contains("Not now")`,
		`button:contains("Maybe later")`,
		`a[href*="skip"]`,
		`button[id*="skip"]`,
		`button[class*="skip"]`,
		`button:contains("Try another way")`,
		`button:contains("Use a different method")`,
	}

	for i, selector := range skipSelectors {
		h.logger.Debug().Str("selector", selector).Int("attempt", i+1).Msg("Trying skip button selector")
		element, err := page.Timeout(10 * time.Second).Element(selector)
		if err != nil {
			h.logger.Debug().Err(err).Str("selector", selector).Msg("Skip button selector failed")
			continue
		}

		// Check if element is visible and clickable
		visible, visErr := element.Visible()
		if visErr != nil || !visible {
			h.logger.Debug().Str("selector", selector).Bool("visible", visible).Msg("Skip button not visible")
			continue
		}

		h.logger.Info().Str("selector", selector).Msg("Found visible skip button, attempting to click")

		// Try to click the skip button
		err = element.ScrollIntoView()
		if err != nil {
			h.logger.Warn().Err(err).Msg("Failed to scroll skip button into view")
		}

		err = element.Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			h.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click skip button")
			continue
		}

		h.logger.Info().Str("selector", selector).Msg("Successfully clicked skip button")
		time.Sleep(3 * time.Second)
		return nil
	}

	h.logger.Info().Msg("No skip options found for Atlassian MFA")
	return fmt.Errorf("no skip options found for Atlassian MFA")
}

// handleMFAWithGmail handles MFA using Gmail integration
func (h *AtlassianOAuthHandler) handleMFAWithGmail(page *rod.Page) error {
	h.logger.Info().Msg("Handling Atlassian MFA using Gmail integration")

	// Find MFA input field
	h.logger.Info().Msg("Step 1: Searching for Atlassian MFA input field")
	mfaInputSelectors := []string{
		// Modern Atlassian MFA selectors (2024/2025)
		`input[data-testid*="verification"]`,
		`input[data-testid*="code"]`,
		`input[data-testid*="mfa"]`,
		`input[data-testid*="otp"]`,
		`input[class*="verification"]`,
		`input[class*="code"]`,
		`input[aria-label*="verification"]`,
		`input[aria-label*="code"]`,
		`input[aria-describedby*="verification"]`,
		`input[aria-describedby*="code"]`,
		// Legacy selectors
		`input[name="otp"]`,
		`input[id="otp"]`,
		`input[name="code"]`,
		`input[id="code"]`,
		`input[name="verification_code"]`,
		`input[id="verification_code"]`,
		`input[name="mfa_code"]`,
		`input[id="mfa_code"]`,
		`input[placeholder*="code"]`,
		`input[placeholder*="verification"]`,
		`input[type="text"][maxlength="6"]`,
		`input[type="text"][maxlength="8"]`,
		`input[type="tel"]`,
		// More comprehensive fallbacks
		`input[type="text"]:not([name="username"]):not([name="email"]):not([name="password"])`,
		`input[inputmode="numeric"]`,
		`input[pattern*="[0-9]"]`,
	}

	var mfaInput *rod.Element
	var err error

	for i, selector := range mfaInputSelectors {
		h.logger.Info().Str("selector", selector).Int("attempt", i+1).Msg("Trying MFA input field selector")
		mfaInput, err = page.Timeout(1 * time.Second).Element(selector)
		if err == nil && mfaInput != nil {
			h.logger.Info().Str("selector", selector).Msg("Found Atlassian MFA input field")
			break
		}
		h.logger.Debug().Err(err).Str("selector", selector).Msg("MFA input field selector failed")
	}

	if mfaInput == nil {
		h.logger.Error().Msg("Could not find any Atlassian MFA input field")
		return fmt.Errorf("could not find Atlassian MFA input field")
	}

	// Get MFA code from Gmail
	h.logger.Info().Msg("Step 2: Getting MFA code from Gmail")
	mfaCode, err := h.getAtlassianMFACode()
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get Atlassian MFA code from Gmail")
		return fmt.Errorf("failed to get Atlassian MFA code from Gmail: %w", err)
	}

	h.logger.Info().Str("code", mfaCode).Msg("Successfully retrieved MFA code from Gmail")

	// Enter the MFA code
	h.logger.Info().Msg("Step 3: Entering MFA code into browser")
	h.logger.Info().Str("code", mfaCode).Msg("Entering Atlassian MFA code")

	h.logger.Debug().Msg("Clicking MFA input field")
	err = mfaInput.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to click MFA input field")
		return fmt.Errorf("failed to click MFA input field: %w", err)
	}

	h.logger.Debug().Msg("Selecting all text in MFA input field")
	err = mfaInput.SelectAllText()
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to select all text in MFA input, continuing")
	}

	h.logger.Debug().Str("code", mfaCode).Msg("Inputting MFA code character by character")

	// Clear the field first
	err = mfaInput.Input("")
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to clear MFA input field")
	}

	// Click on the first input field to focus it
	err = mfaInput.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to click MFA input field")
	}

	// Type each character individually with a small delay to allow JavaScript to move between fields
	for i, char := range mfaCode {
		h.logger.Debug().Str("char", string(char)).Int("position", i+1).Msg("Typing character")

		// Type the character using keyboard input
		err = page.Keyboard.Type(input.Key(char))
		if err != nil {
			h.logger.Error().Err(err).Str("char", string(char)).Msg("Failed to type character")
			return fmt.Errorf("failed to type character %s: %w", string(char), err)
		}

		// Small delay to allow JavaScript to process and move to next field
		time.Sleep(300 * time.Millisecond)
	}

	h.logger.Info().Msg("Successfully entered MFA code character by character")

	// Find and click the submit button
	h.logger.Info().Msg("Step 4: Finding and clicking submit button")
	submitSelectors := []string{
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button[id="otp-submit"]`,
		`button:contains("Submit")`,
		`button:contains("Verify")`,
		`button:contains("Continue")`,
		`button:contains("Confirm")`,
	}

	var submitButton *rod.Element
	for i, selector := range submitSelectors {
		h.logger.Info().Str("selector", selector).Int("attempt", i+1).Msg("Trying submit button selector")
		submitButton, err = page.Timeout(10 * time.Second).Element(selector)
		if err == nil && submitButton != nil {
			h.logger.Info().Str("selector", selector).Msg("Found MFA submit button")
			break
		}
		h.logger.Debug().Err(err).Str("selector", selector).Msg("Submit button selector failed")
	}

	if submitButton == nil {
		h.logger.Error().Msg("Could not find any MFA submit button")
		return fmt.Errorf("could not find MFA submit button")
	}

	h.logger.Info().Msg("Clicking submit button")
	err = submitButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to click MFA submit button")
		return fmt.Errorf("failed to click MFA submit button: %w", err)
	}

	h.logger.Info().Msg("Successfully clicked submit button")
	h.logger.Info().Msg("Waiting for MFA submission to process")

	// Wait and check for redirect to authorization page
	for i := 0; i < 12; i++ { // Check every 2 seconds for 24 seconds
		time.Sleep(2 * time.Second)
		currentURL := page.MustInfo().URL
		h.logger.Info().Str("current_url", currentURL).Int("check_iteration", i+1).Msg("Checking URL after MFA submission")

		// Check if we've been redirected away from the MFA page
		if !strings.Contains(currentURL, "/login/mfa") {
			h.logger.Info().Str("redirected_url", currentURL).Msg("Successfully redirected away from MFA page")
			break
		}

		// Check if page has any success indicators
		successSelectors := []string{
			`.success`, `.alert-success`, `[class*="success"]`,
			`[role="status"]`, `.message.success`,
		}

		for _, selector := range successSelectors {
			elements, err := page.Elements(selector)
			if err == nil && len(elements) > 0 {
				for _, element := range elements {
					text := element.MustText()
					if text != "" {
						h.logger.Info().Str("selector", selector).Str("success_text", text).Msg("Found success message on MFA page")
					}
				}
			}
		}

		// Check if page has any error messages
		errorSelectors := []string{
			`.error`, `.alert-error`, `.alert-danger`,
			`[class*="error"]`, `[class*="invalid"]`, `[class*="fail"]`,
			`[role="alert"]`, `.message.error`,
		}

		for _, selector := range errorSelectors {
			elements, err := page.Elements(selector)
			if err == nil && len(elements) > 0 {
				for _, element := range elements {
					text := element.MustText()
					if text != "" {
						h.logger.Warn().Str("selector", selector).Str("error_text", text).Msg("Found error message on MFA page")
					}
				}
			}
		}

		// Check if there are any "Continue" or "Next" buttons that might need clicking
		continueSelectors := []string{
			`button:contains("Continue")`,
			`button:contains("Next")`,
			`button:contains("Proceed")`,
			`button[id*="continue"]`,
			`button[class*="continue"]`,
			`a:contains("Continue")`,
			`a[href*="continue"]`,
		}

		for _, selector := range continueSelectors {
			elements, err := page.Elements(selector)
			if err == nil && len(elements) > 0 {
				h.logger.Info().Str("selector", selector).Int("count", len(elements)).Msg("Found potential continue button on MFA page")
				// Try clicking the first continue button
				if err := elements[0].Click(proto.InputMouseButtonLeft, 1); err == nil {
					h.logger.Info().Str("selector", selector).Msg("Successfully clicked continue button")
					time.Sleep(3 * time.Second)
					break
				}
			}
		}
	}

	// Final URL check
	finalURL := page.MustInfo().URL
	h.logger.Info().Str("final_url", finalURL).Msg("Final URL after MFA completion")

	// If we're still on MFA page, try to manually navigate to the continue URL
	if strings.Contains(finalURL, "/login/mfa") {
		// Extract the continue URL from the current URL
		if u, err := url.Parse(finalURL); err == nil {
			if continueURL := u.Query().Get("continue"); continueURL != "" {
				h.logger.Info().Str("continue_url", continueURL).Msg("Attempting to manually navigate to continue URL")
				if err := page.Navigate(continueURL); err == nil {
					h.logger.Info().Msg("Successfully navigated to continue URL")
					time.Sleep(3 * time.Second)
				} else {
					h.logger.Warn().Err(err).Msg("Failed to navigate to continue URL")
				}
			}
		}
	}

	h.logger.Info().Msg("Atlassian MFA submitted successfully")
	return nil
}

// getAtlassianMFACode reads the latest Atlassian MFA email and extracts the code
func (h *AtlassianOAuthHandler) getAtlassianMFACode() (string, error) {
	h.logger.Info().Msg("Searching for Atlassian MFA email")

	// Search for recent emails from Atlassian with MFA codes
	// Use time filter to only get recent emails (last 10 minutes)
	timeFilter := time.Now().Add(-10 * time.Minute).Format("2006/01/02")
	query := fmt.Sprintf("from:noreply@id.atlassian.com after:%s", timeFilter)

	h.logger.Info().Str("query", query).Msg("Gmail search query")
	h.logger.Info().Msg("Executing Gmail API search")

	listCall := h.gmailService.Users.Messages.List("test@helix.ml").Q(query).MaxResults(10)

	messages, err := listCall.Do()
	if err != nil {
		h.logger.Error().Err(err).Msg("Gmail API search failed")
		return "", fmt.Errorf("failed to search for Atlassian MFA emails: %w", err)
	}

	// If no messages found with primary search, try fallback searches
	if len(messages.Messages) == 0 {
		h.logger.Warn().Str("primary_query", query).Msg("No messages found with primary search, trying fallback searches")

		// Try different sender patterns that Atlassian might use
		fallbackQueries := []string{
			fmt.Sprintf("from:noreply+@id.atlassian.com after:%s", timeFilter), // Pattern with + prefix
			fmt.Sprintf("from:no-reply@atlassian.com after:%s", timeFilter),    // Original pattern
			fmt.Sprintf("from:noreply@atlassian.com after:%s", timeFilter),     // Without id subdomain
			fmt.Sprintf("from:@id.atlassian.com after:%s", timeFilter),         // Any sender from id.atlassian.com
			fmt.Sprintf("from:@atlassian.com after:%s", timeFilter),            // Any sender from atlassian.com
		}

		for i, fallbackQuery := range fallbackQueries {
			h.logger.Info().Int("attempt", i+1).Str("query", fallbackQuery).Msg("Trying fallback Gmail search")
			listCall := h.gmailService.Users.Messages.List("test@helix.ml").Q(fallbackQuery).MaxResults(10)
			messages, err = listCall.Do()
			if err != nil {
				h.logger.Warn().Err(err).Str("query", fallbackQuery).Msg("Fallback search failed")
				continue
			}

			if len(messages.Messages) > 0 {
				h.logger.Info().Int("message_count", len(messages.Messages)).Str("successful_query", fallbackQuery).Msg("Found messages with fallback search")
				break
			}
		}
	}

	if len(messages.Messages) == 0 {
		h.logger.Warn().Str("time_filter", timeFilter).Msg("No recent Atlassian MFA emails found with any search pattern")
		return "", fmt.Errorf("no recent Atlassian MFA emails found")
	}

	h.logger.Info().Int("message_count", len(messages.Messages)).Msg("Found Atlassian MFA emails")

	// Debug: show details of found emails
	for i, msg := range messages.Messages {
		if i >= 3 { // Limit debug output to first 3 emails
			break
		}

		// Get message details
		msgCall := h.gmailService.Users.Messages.Get("test@helix.ml", msg.Id)
		fullMessage, err := msgCall.Do()
		if err != nil {
			h.logger.Warn().Err(err).Str("message_id", msg.Id).Msg("Failed to get message details")
			continue
		}

		var sender, subject string
		for _, header := range fullMessage.Payload.Headers {
			if header.Name == "From" {
				sender = header.Value
			}
			if header.Name == "Subject" {
				subject = header.Value
			}
		}

		h.logger.Info().
			Int("email_index", i).
			Str("message_id", msg.Id).
			Str("sender", sender).
			Str("subject", subject).
			Msg("Found email details")
	}

	h.logger.Info().Int("message_count", len(messages.Messages)).Msg("Found Atlassian emails")

	// Check each message for MFA codes (most recent first)
	for i, msg := range messages.Messages {
		messageID := msg.Id
		h.logger.Info().Str("message_id", messageID).Int("message_index", i).Msg("Processing message")

		message, err := h.gmailService.Users.Messages.Get("test@helix.ml", messageID).Do()
		if err != nil {
			h.logger.Warn().Err(err).Str("message_id", messageID).Msg("Failed to get message")
			continue
		}

		h.logger.Debug().Str("message_id", messageID).Msg("Successfully retrieved message")

		// Extract email body
		h.logger.Debug().Str("message_id", messageID).Msg("Extracting email body")
		emailBody := ""
		if message.Payload.Body.Data != "" {
			h.logger.Debug().Msg("Found email body in main payload")
			decoded, err := base64.URLEncoding.DecodeString(message.Payload.Body.Data)
			if err == nil {
				emailBody = string(decoded)
			} else {
				h.logger.Warn().Err(err).Msg("Failed to decode main email body")
			}
		}

		// Check parts for the body if main body is empty
		if emailBody == "" && len(message.Payload.Parts) > 0 {
			h.logger.Debug().Int("parts_count", len(message.Payload.Parts)).Msg("Checking email parts for body")
			for j, part := range message.Payload.Parts {
				h.logger.Debug().Str("mime_type", part.MimeType).Int("part_index", j).Msg("Checking email part")
				if (part.MimeType == "text/plain" || part.MimeType == "text/html") && part.Body.Data != "" {
					decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
					if err == nil {
						emailBody = string(decoded)
						h.logger.Debug().Str("mime_type", part.MimeType).Int("part_index", j).Msg("Found email body in part")
						break
					}
					h.logger.Warn().Err(err).Str("mime_type", part.MimeType).Int("part_index", j).Msg("Failed to decode email part")
				}
			}
		}

		if emailBody == "" {
			h.logger.Warn().Str("message_id", messageID).Msg("Could not extract email body")
			continue
		}

		// Log a snippet of the email for debugging
		snippet := emailBody
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		h.logger.Info().Str("email_snippet", snippet).Msg("Atlassian email content")

		// Extract MFA code using multiple regex patterns
		h.logger.Debug().Str("message_id", messageID).Msg("Attempting to extract MFA code")
		// Atlassian MFA codes are typically 6-character alphanumeric (e.g., "BRDV7G")
		codePatterns := []string{
			`\b([A-Z0-9]{6})\b`,                         // 6-character alphanumeric code
			`code:\s*([A-Z0-9]{6})`,                     // "code: BRDV7G"
			`following code:\s*([A-Z0-9]{6})`,           // "following code: BRDV7G"
			`enter the following code:\s*([A-Z0-9]{6})`, // "enter the following code: BRDV7G"
			`verification code:\s*([A-Z0-9]{6})`,        // "verification code: BRDV7G"
			`\b([A-Z0-9]{4}-[A-Z0-9]{2})\b`,             // Alternative format like "BRDV-7G"
		}

		for j, pattern := range codePatterns {
			h.logger.Debug().Str("pattern", pattern).Int("pattern_index", j).Msg("Trying regex pattern")
			codeRegex := regexp.MustCompile(pattern)
			matches := codeRegex.FindStringSubmatch(emailBody)
			if len(matches) > 1 {
				mfaCode := matches[1]
				h.logger.Info().Str("mfa_code", mfaCode).Str("pattern", pattern).Msg("Extracted Atlassian MFA code")
				return mfaCode, nil
			}
		}

		h.logger.Warn().Str("message_id", messageID).Msg("No MFA code found in this email")
	}

	h.logger.Error().Msg("Could not find Atlassian MFA code in any recent emails")
	return "", fmt.Errorf("could not find Atlassian MFA code in any recent emails")
}
