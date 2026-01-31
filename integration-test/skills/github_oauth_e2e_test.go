// GitHub OAuth Skills E2E Test
//
// This test requires OAuth integration environment variables and Chrome
// container, which the stack command will start automatically.
//
// To run this test, from the helix root directory:
//
//   ./stack test -v integration-test/skills/*.go -run TestGitHubOAuthSkillsE2E
//
// The test will:
// 1. Set up Helix infrastructure (OAuth manager, API server, etc.)
// 2. Create a GitHub OAuth provider
// 3. Create test repositories on GitHub using the test user's PAT
// 4. Create a Helix app with GitHub skills from github.yaml
// 5. Perform OAuth flow against real GitHub using browser automation
// 6. Test agent sessions with real GitHub API calls with the resulting JWT
// 7. Clean up test resources

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

// GitHubTestQueries defines the agent test queries that verify GitHub skills integration
var GitHubTestQueries = []AgentTestQuery{
	{
		Query:       "What is my GitHub username?",
		SessionName: "GitHub Username Query",
		ExpectedResponseCheck: func(response string) bool {
			return len(response) > 0 && strings.Contains(strings.ToLower(response), "username")
		},
	},
	{
		Query:       "List my GitHub repositories",
		SessionName: "GitHub Repository Listing",
		ExpectedResponseCheck: func(response string) bool {
			lower := strings.ToLower(response)
			return strings.Contains(lower, "repositor") || (strings.Contains(lower, "stop") && strings.Contains(lower, "task"))
		},
	},
	{
		Query:       "What issues are open in my repository {REPO_NAME}?", // {REPO_NAME} will be replaced
		SessionName: "GitHub Issues Query",
		ExpectedResponseCheck: func(response string) bool {
			return len(response) > 0 // Just check that we get some response
		},
	},
}

// GitHubOAuthProviderConfig defines the OAuth provider configuration for GitHub
var GitHubOAuthProviderConfig = OAuthProviderConfig{
	ProviderName: "GitHub Skills Test",
	ProviderType: types.OAuthProviderTypeGitHub,
	SkillName:    "github",
	AuthURL:      "https://github.com/login/oauth/authorize",
	TokenURL:     "https://github.com/login/oauth/access_token",
	UserInfoURL:  "https://api.github.com/user",
	Scopes:       []string{"repo", "user:read"},
	// ClientID, ClientSecret, Username, Password, and functions will be set during test setup
}

// ======================================================================================
// MAIN TEST SUITE
// ======================================================================================

// GitHubOAuthE2ETestSuite tests the complete GitHub OAuth skills workflow
type GitHubOAuthE2ETestSuite struct {
	// Embed the base OAuth test suite for common functionality
	BaseOAuthTestSuite

	// GitHub-specific test configuration from environment
	githubClientID     string
	githubClientSecret string
	githubUsername     string
	githubPassword     string
	githubToken        string // Personal access token for GitHub API calls

	// Gmail configuration for device verification
	gmailCredentialsBase64 string // Base64 encoded Gmail API credentials JSON

	// Test repositories created and GitHub utilities
	testRepos   []string
	githubUtils *GitHubTestUtils

	// OAuth provider test template
	oauthTemplate *OAuthProviderTestTemplate
}

// TestGitHubOAuthSkillsE2E is the main end-to-end test for GitHub OAuth skills
func TestGitHubOAuthSkillsE2E(t *testing.T) {
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

	suite := &GitHubOAuthE2ETestSuite{
		BaseOAuthTestSuite: BaseOAuthTestSuite{
			ctx: ctx,
		},
	}

	// Load test configuration from environment
	err := suite.loadTestConfig()
	if err != nil {
		t.Skipf("Skipping GitHub OAuth E2E test: %v", err)
	}

	// Initialize test dependencies using base class
	err = suite.SetupBaseInfrastructure("github_oauth_e2e")
	require.NoError(t, err, "Failed to setup base infrastructure")

	// GitHub-specific setup
	err = suite.setupGitHubSpecifics(t)
	require.NoError(t, err, "Failed to setup GitHub-specific dependencies")

	// Create OAuth provider test template
	err = suite.createOAuthTemplate()
	require.NoError(t, err, "Failed to create OAuth provider test template")

	// Run the complete end-to-end workflow using template
	t.Run("SetupOAuthProvider", suite.oauthTemplate.TestSetupOAuthProvider)
	t.Run("CreateTestRepositories", suite.testCreateTestRepositories)
	t.Run("CreateTestApp", suite.oauthTemplate.TestCreateTestApp)
	t.Run("PerformOAuthFlow", suite.oauthTemplate.TestPerformOAuthFlow)
	t.Run("TestOAuthTokenDirectly", suite.oauthTemplate.TestOAuthTokenDirectly)
	t.Run("TestAgentGitHubSkillsIntegration", suite.oauthTemplate.TestAgentOAuthSkillsIntegration)

	// Cleanup
	t.Cleanup(func() {
		suite.cleanupGitHubSpecifics(t)
		suite.oauthTemplate.Cleanup(t)
		suite.CleanupBaseInfrastructure()
	})
}

// ======================================================================================
// CONFIGURATION AND SETUP
// ======================================================================================

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

	// Debug logging for CI environment (log lengths only, not actual values)
	log.Info().
		Str("username", suite.githubUsername).
		Int("password_length", len(suite.githubPassword)).
		Msg("GitHub OAuth credentials loaded (debug info for CI troubleshooting)")

	// For repository setup/cleanup, we need a personal access token with repo creation/deletion permissions
	// This is separate from the OAuth tokens we'll get during the test
	suite.githubToken = os.Getenv("GITHUB_SKILL_TEST_SETUP_PAT")
	if suite.githubToken == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_SETUP_PAT environment variable not set - need PAT for repo creation/cleanup")
	}

	// Gmail API credentials for device verification
	suite.gmailCredentialsBase64 = os.Getenv("GMAIL_CREDENTIALS_BASE64")
	if suite.gmailCredentialsBase64 == "" {
		return fmt.Errorf("GMAIL_CREDENTIALS_BASE64 environment variable not set - need Gmail API credentials for device verification")
	}

	log.Info().
		Str("client_id", suite.githubClientID).
		Str("username", suite.githubUsername).
		Msg("Loaded GitHub OAuth test configuration with Gmail credentials")

	return nil
}

// setupGitHubSpecifics initializes GitHub-specific test environment
func (suite *GitHubOAuthE2ETestSuite) setupGitHubSpecifics(_ *testing.T) error {
	suite.logger.Info().Msg("=== GitHub OAuth Skills E2E Test Starting ===")

	// Clean up any existing OAuth connections from previous test runs
	suite.logger.Info().Msg("Cleaning up existing OAuth connections from previous test runs")
	err := suite.CleanupExistingOAuthData()
	if err != nil {
		suite.logger.Warn().Err(err).Msg("Failed to cleanup existing OAuth data, continuing anyway")
	}

	// Initialize GitHub utilities
	suite.githubUtils = NewGitHubTestUtils(suite.githubToken, suite.githubUsername, suite.logger)

	suite.logger.Info().Msg("GitHub-specific test setup completed successfully")
	return nil
}

// createOAuthTemplate creates the OAuth provider test template for GitHub
func (suite *GitHubOAuthE2ETestSuite) createOAuthTemplate() error {
	// Make a copy of the config and fill in the dynamic values
	config := GitHubOAuthProviderConfig
	config.ClientID = suite.githubClientID
	config.ClientSecret = suite.githubClientSecret
	config.Username = suite.githubUsername
	config.Password = suite.githubPassword
	config.GetAuthorizationCodeFunc = suite.getGitHubAuthorizationCode
	config.SetupTestDataFunc = func() error { return nil } // No setup needed - done in separate test step
	config.CleanupTestDataFunc = suite.cleanupGitHubTestData

	// Customize test queries with actual repo name
	testQueries := make([]AgentTestQuery, len(GitHubTestQueries))
	copy(testQueries, GitHubTestQueries)

	// Replace {REPO_NAME} placeholder in the issues query
	for i, query := range testQueries {
		if strings.Contains(query.Query, "{REPO_NAME}") {
			testQueries[i].Query = strings.Replace(query.Query, "{REPO_NAME}", suite.getTestRepoName(), 1)
		}
	}
	config.AgentTestQueries = testQueries

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

// getGitHubAuthorizationCode performs real browser automation to complete GitHub OAuth flow
func (suite *GitHubOAuthE2ETestSuite) getGitHubAuthorizationCode(authURL, state string) (string, error) {
	// Set up GitHub-specific 2FA handler
	deviceHandler, err := NewGitHubDeviceVerificationHandler(suite.gmailCredentialsBase64, suite.logger)
	if err != nil {
		return "", fmt.Errorf("failed to create GitHub device verification handler: %w", err)
	}

	// Configure GitHub browser automation with GitHub-specific strategy
	githubConfig := BrowserOAuthConfig{
		ProviderName:            "github",
		LoginUsernameSelector:   `input[name="login"]`,
		LoginPasswordSelector:   `input[name="password"]`,
		LoginButtonSelector:     `input[type="submit"][value="Sign in"], button[type="submit"]`,
		AuthorizeButtonSelector: `input[type="submit"][value="Authorize"], button[value="authorize"], button[name="authorize"], input[name="authorize"], form button[type="submit"]:nth-of-type(1)`,
		CallbackURLPattern:      "/api/v1/oauth/flow/callback",
		DeviceVerificationCheck: IsGitHubDeviceVerificationPage,
		TwoFactorHandler:        deviceHandler,
		ProviderStrategy:        NewGitHubProviderStrategy(suite.logger), // Use GitHub-specific strategy
	}

	// Create automator with GitHub configuration
	automator := NewBrowserOAuthAutomator(suite.browser, suite.logger, githubConfig)

	// Perform OAuth flow using the generic automator
	return automator.PerformOAuthFlow(authURL, state, suite.githubUsername, suite.githubPassword, suite)
}

// ======================================================================================
// TEST DATA MANAGEMENT
// ======================================================================================

// testCreateTestRepositories creates test repositories on GitHub for testing
func (suite *GitHubOAuthE2ETestSuite) testCreateTestRepositories(t *testing.T) {
	suite.logger.Info().Msg("Creating test repositories on GitHub")

	// Create a test repository with some content
	repoName := suite.getTestRepoName()

	err := suite.githubUtils.CreateRepo(repoName, "Test repository for Helix OAuth skills testing")
	require.NoError(t, err, "Failed to create test repository")

	suite.testRepos = append(suite.testRepos, repoName)

	// Add some test content to the repository
	readmeContent := fmt.Sprintf(`# Helix Test Repository

This is a test repository created for testing Helix OAuth skills integration.

## Test Data

- Repository: %s
- Created by: Helix OAuth E2E Test
- Purpose: Testing GitHub skills integration
`, repoName)

	err = suite.githubUtils.AddContentToRepo(repoName, "README.md", readmeContent)
	require.NoError(t, err, "Failed to add content to test repository")

	err = suite.githubUtils.AddContentToRepo(repoName, "test-file.txt", "This is a test file for Helix to read and understand.\nIt contains some sample data that the agent should be able to access.")
	require.NoError(t, err, "Failed to add test file to repository")

	suite.logger.Info().
		Str("repo_name", repoName).
		Msg("Successfully created test repository with content")
}

// getTestRepoName returns the name of the test repository (will be created during test)
func (suite *GitHubOAuthE2ETestSuite) getTestRepoName() string {
	if len(suite.testRepos) > 0 {
		return suite.testRepos[0]
	}
	return fmt.Sprintf("helix-test-repo-%d", time.Now().Unix())
}

// cleanupGitHubTestData cleans up GitHub-specific test data
func (suite *GitHubOAuthE2ETestSuite) cleanupGitHubTestData() error {
	// Delete test GitHub repositories
	for _, repoName := range suite.testRepos {
		err := suite.githubUtils.DeleteRepo(repoName)
		if err != nil {
			suite.logger.Error().Err(err).Str("repo", repoName).Msg("Failed to delete GitHub repository")
		} else {
			suite.logger.Info().Str("repo", repoName).Msg("GitHub repository deleted")
		}
	}
	return nil
}

// ======================================================================================
// CLEANUP
// ======================================================================================

// cleanupGitHubSpecifics cleans up GitHub-specific test resources
func (suite *GitHubOAuthE2ETestSuite) cleanupGitHubSpecifics(_ *testing.T) {
	suite.logger.Info().Msg("=== Starting GitHub-specific Test Cleanup ===")

	// Clean up test data through the template
	err := suite.cleanupGitHubTestData()
	if err != nil {
		suite.logger.Error().Err(err).Msg("Failed to cleanup GitHub test data")
	}

	suite.logger.Info().Msg("=== GitHub-specific Test Cleanup Completed ===")
}
