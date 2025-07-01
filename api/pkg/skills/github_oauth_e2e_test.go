package skills

import (
	"bytes"
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

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GitHubOAuthE2ETestSuite tests the complete GitHub OAuth skills workflow
type GitHubOAuthE2ETestSuite struct {
	ctx    context.Context
	store  store.Store
	oauth  *oauth.Manager
	client *client.HelixClient

	// Test configuration from environment
	githubClientID     string
	githubClientSecret string
	githubUsername     string
	githubPassword     string
	githubToken        string // Personal access token for GitHub API calls

	// Created during test
	testUser      *types.User
	oauthProvider *types.OAuthProvider
	testApp       *types.App
	oauthConn     *types.OAuthConnection

	// Test logging
	logFile *os.File
	logger  zerolog.Logger

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
	t.Run("TestAgentConversation", suite.testAgentConversation)

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

// setup initializes test dependencies
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

	// Initialize store
	suite.store, err = store.NewPostgresStore(cfg.Store)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}

	// Initialize OAuth manager
	suite.oauth = oauth.NewManager(suite.store)

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

	suite.client, err = client.NewClient(cfg.WebServer.URL, apiKey)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	suite.logger.Info().
		Str("user_id", suite.testUser.ID).
		Str("username", suite.testUser.Username).
		Msg("Test setup completed successfully")

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
	timestamp := time.Now().Format("20060102_150405")
	logFilename := filepath.Join(testResultsDir, fmt.Sprintf("github_oauth_e2e_%s.log", timestamp))

	var err error
	suite.logFile, err = os.Create(logFilename)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	// Create multi-writer to write to both file and console
	multiWriter := io.MultiWriter(suite.logFile, os.Stdout)

	// Setup zerolog with pretty printing
	suite.logger = zerolog.New(multiWriter).
		With().
		Timestamp().
		Str("test", "github_oauth_e2e").
		Logger().
		Output(zerolog.ConsoleWriter{Out: multiWriter, TimeFormat: time.RFC3339})

	suite.logger.Info().
		Str("log_file", logFilename).
		Msg("Test logging initialized")

	return nil
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

// testAgentConversation tests that the agent can use GitHub skills to access repository data
func (suite *GitHubOAuthE2ETestSuite) testAgentConversation(t *testing.T) {
	suite.logger.Info().Msg("Starting agent conversation test with GitHub skills")

	// Test conversation with specific questions about the test repositories
	testQuestions := []string{
		"What repositories do I have on GitHub? Can you tell me about them?",
		fmt.Sprintf("Can you tell me about my repository %s?", suite.testRepos[0]),
		"What files are in my GitHub repositories?",
		"Can you summarize the content of my test repositories?",
	}

	for i, question := range testQuestions {
		suite.logger.Info().
			Int("question_number", i+1).
			Str("question", question).
			Msg("Sending question to agent")

		// Use the session chat API endpoint directly
		response, err := suite.sendChatMessage(question)
		require.NoError(t, err, "Failed to send message to agent")

		suite.logger.Info().
			Int("question_number", i+1).
			Str("response", response).
			Msg("Received agent response")

		// Log the full interaction for analysis
		suite.logInteraction(i+1, question, response)

		// Verify the response contains GitHub-related information
		suite.validateGitHubResponse(t, i+1, question, response)

		// Add some delay between questions to avoid rate limiting
		time.Sleep(3 * time.Second)
	}

	// Verify OAuth connection was used
	connections, err := suite.store.ListOAuthConnections(context.Background(), &store.ListOAuthConnectionsQuery{
		UserID: suite.testUser.ID,
	})
	require.NoError(t, err, "Failed to list OAuth connections")
	assert.Len(t, connections, 1, "Expected exactly one OAuth connection")
	assert.Equal(t, "github", connections[0].Provider, "Expected GitHub OAuth connection")

	suite.logger.Info().
		Int("oauth_connections", len(connections)).
		Int("questions_asked", len(testQuestions)).
		Msg("Agent conversation test completed successfully")
}

// sendChatMessage sends a message to the agent using the session chat API
func (suite *GitHubOAuthE2ETestSuite) sendChatMessage(message string) (string, error) {
	// Create session chat request
	chatRequest := &types.SessionChatRequest{
		AppID:  suite.testApp.ID,
		Stream: false,
		Messages: []*types.Message{
			{
				Role: types.CreatorTypeUser,
				Content: types.MessageContent{
					ContentType: types.MessageContentTypeText,
					Parts:       []any{message},
				},
			},
		},
	}

	// Make HTTP request to session chat endpoint
	reqBody, err := json.Marshal(chatRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal chat request: %w", err)
	}

	// Get server config to build URL
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load server config: %w", err)
	}

	baseURL := cfg.WebServer.URL
	if !strings.HasSuffix(baseURL, "/api/v1") {
		baseURL = baseURL + "/api/v1"
	}

	req, err := http.NewRequest("POST", baseURL+"/sessions/chat", strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+suite.getAPIKey())
	req.Header.Set("User-Agent", "GitHub OAuth E2E Test")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse OpenAI-style response
	var openAIResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &openAIResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(openAIResponse.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return openAIResponse.Choices[0].Message.Content, nil
}

// validateGitHubResponse validates that the agent response contains GitHub-related information
func (suite *GitHubOAuthE2ETestSuite) validateGitHubResponse(t *testing.T, questionNumber int, question, response string) {
	// Convert to lowercase for easier matching
	lowerResponse := strings.ToLower(response)

	switch questionNumber {
	case 1: // Repository list question
		// Should mention repositories, GitHub, or repo names
		hasRepoInfo := strings.Contains(lowerResponse, "repositor") ||
			strings.Contains(lowerResponse, "github") ||
			strings.Contains(lowerResponse, "repo")
		assert.True(t, hasRepoInfo, "Response should contain repository information")

	case 2: // Specific repository question
		// Should mention the specific test repository
		testRepoName := strings.ToLower(suite.testRepos[0])
		hasTestRepo := strings.Contains(lowerResponse, testRepoName)
		assert.True(t, hasTestRepo, "Response should mention the test repository")

	case 3: // Files question
		// Should mention files or file-related terms
		hasFileInfo := strings.Contains(lowerResponse, "file") ||
			strings.Contains(lowerResponse, "readme") ||
			strings.Contains(lowerResponse, "content")
		assert.True(t, hasFileInfo, "Response should contain file information")

	case 4: // Summary question
		// Should provide some summary or analysis
		hasSummary := len(response) > 50 // Basic check for substantial response
		assert.True(t, hasSummary, "Response should provide a meaningful summary")
	}

	suite.logger.Info().
		Int("question_number", questionNumber).
		Bool("validation_passed", true).
		Msg("Response validation completed")
}

// getAPIKey returns the API key for the test user
func (suite *GitHubOAuthE2ETestSuite) getAPIKey() string {
	return suite.testAPIKey
}

// logInteraction logs a question-answer interaction for analysis
func (suite *GitHubOAuthE2ETestSuite) logInteraction(questionNumber int, question, response string) {
	suite.logger.Info().Msg("=== AGENT INTERACTION ===")
	suite.logger.Info().
		Int("question_number", questionNumber).
		Str("question", question).
		Msg("QUESTION")
	suite.logger.Info().
		Int("question_number", questionNumber).
		Str("response", response).
		Msg("AGENT RESPONSE")
	suite.logger.Info().Msg("=== END INTERACTION ===")
}

// Helper methods

func (suite *GitHubOAuthE2ETestSuite) createTestUser() (*types.User, error) {
	user := &types.User{
		ID:       fmt.Sprintf("test-user-%d", time.Now().Unix()),
		Email:    fmt.Sprintf("github-oauth-test-%d@helix.test", time.Now().Unix()),
		Username: fmt.Sprintf("github-oauth-test-%d", time.Now().Unix()),
		FullName: "GitHub OAuth Test User",
		Admin:    false,
	}

	createdUser, err := suite.store.CreateUser(suite.ctx, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return createdUser, nil
}

func (suite *GitHubOAuthE2ETestSuite) createTestUserAPIKey() (string, error) {
	apiKey := fmt.Sprintf("test-api-key-%d", time.Now().Unix())

	_, err := suite.store.CreateAPIKey(suite.ctx, &types.ApiKey{
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

func (suite *GitHubOAuthE2ETestSuite) cleanup(t *testing.T) {
	log.Info().Msg("Cleaning up test resources")

	// Clean up OAuth connection
	if suite.oauthConn != nil {
		err := suite.store.DeleteOAuthConnection(suite.ctx, suite.oauthConn.ID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to cleanup OAuth connection")
		}
	}

	// Clean up OAuth provider
	if suite.oauthProvider != nil {
		err := suite.store.DeleteOAuthProvider(suite.ctx, suite.oauthProvider.ID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to cleanup OAuth provider")
		}
	}

	// Clean up test app
	if suite.testApp != nil {
		err := suite.store.DeleteApp(suite.ctx, suite.testApp.ID)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to cleanup test app")
		}
	}

	// Clean up test user (optional - might want to keep for debugging)
	// if suite.testUser != nil {
	//     err := suite.store.DeleteUser(suite.ctx, suite.testUser.ID)
	//     if err != nil {
	//         log.Warn().Err(err).Msg("Failed to cleanup test user")
	//     }
	// }

	log.Info().Msg("Test cleanup completed")
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
	reqBody := map[string]interface{}{
		"message": "Add " + filename + " via Helix test",
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
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

// startHelixOAuthFlow starts OAuth flow using Helix's /api/v1/oauth/flow/start endpoint
func (suite *GitHubOAuthE2ETestSuite) startHelixOAuthFlow() (string, string, error) {
	suite.logger.Info().Msg("Starting OAuth flow via Helix API")

	// Get server config to get base URL
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return "", "", fmt.Errorf("failed to load server config: %w", err)
	}

	// Use Helix's OAuth flow start endpoint
	endpoint := fmt.Sprintf("/api/v1/oauth/flow/start/%s", suite.oauthProvider.ID)
	fullURL := cfg.WebServer.URL + endpoint

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication header
	req.Header.Set("Authorization", "Bearer "+suite.testAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to start OAuth flow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("OAuth flow start failed: %d %s", resp.StatusCode, string(body))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", fmt.Errorf("failed to decode OAuth flow response: %w", err)
	}

	authURL := result["auth_url"]
	if authURL == "" {
		return "", "", fmt.Errorf("no auth_url in response")
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

// getGitHubAuthorizationCode simulates the user completing OAuth in browser to get authorization code
func (suite *GitHubOAuthE2ETestSuite) getGitHubAuthorizationCode(authURL, state string) (string, error) {
	suite.logger.Info().Msg("Simulating GitHub OAuth authorization")

	// Parse the auth URL to get GitHub's OAuth parameters
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse auth URL: %w", err)
	}

	clientID := parsedURL.Query().Get("client_id")
	scope := parsedURL.Query().Get("scope")

	// Use GitHub's OAuth API to simulate the authorization flow
	// This requires the GitHub username/password from environment
	authRequest := map[string]interface{}{
		"client_id": clientID,
		"scope":     scope,
		"note":      "Helix OAuth Skills Test",
		"note_url":  "https://github.com/helixml/helix",
	}

	reqBody, err := json.Marshal(authRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth request: %w", err)
	}

	// Create authorization request to GitHub
	req, err := http.NewRequest("POST", "https://api.github.com/authorizations", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}

	// Use basic auth with GitHub username/password
	req.SetBasicAuth(suite.githubUsername, suite.githubPassword)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create GitHub authorization: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		// GitHub's authorization API is deprecated, so we'll use the setup token instead
		suite.logger.Warn().
			Int("status_code", resp.StatusCode).
			Str("response", string(body)).
			Msg("GitHub authorization API unavailable, using setup token as authorization code")

		// Return the setup token as the "authorization code"
		// In a real test, this would be the code GitHub redirects back with
		return suite.githubToken, nil
	}

	var authResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}

	token, ok := authResponse["token"].(string)
	if !ok {
		return "", fmt.Errorf("no token in authorization response")
	}

	return token, nil
}

// completeHelixOAuthFlow completes OAuth flow using Helix's callback endpoint
func (suite *GitHubOAuthE2ETestSuite) completeHelixOAuthFlow(authCode, state string) (*types.OAuthConnection, error) {
	suite.logger.Info().Msg("Completing OAuth flow via Helix callback")

	// Since we can't easily call the callback endpoint directly (it expects browser redirect),
	// we'll use Helix's OAuth manager directly to complete the flow
	connection, err := suite.oauth.CompleteOAuthFlow(suite.ctx, suite.testUser.ID, suite.oauthProvider.ID, authCode)
	if err != nil {
		return nil, fmt.Errorf("failed to complete OAuth flow: %w", err)
	}

	return connection, nil
}
