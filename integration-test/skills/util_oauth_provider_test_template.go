package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	api_skill "github.com/helixml/helix/api/pkg/agent/skill/api_skills"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadScopesFromYAML loads OAuth scopes from a skill's YAML file
func loadScopesFromYAML(skillName string) ([]string, error) {
	manager := api_skill.NewManager()
	if err := manager.LoadSkills(context.TODO()); err != nil {
		return nil, fmt.Errorf("failed to load skills: %w", err)
	}

	skill, err := manager.GetSkill(skillName)
	if err != nil {
		return nil, fmt.Errorf("failed to get skill '%s': %w", skillName, err)
	}

	if len(skill.OAuthScopes) == 0 {
		return nil, fmt.Errorf("no OAuth scopes found for skill '%s'", skillName)
	}

	return skill.OAuthScopes, nil
}

// OAuthProviderConfig defines the configuration for an OAuth provider test
type OAuthProviderConfig struct {
	// Provider identification
	ProviderName string                  // Human-readable name (e.g., "GitHub Skills Test")
	ProviderType types.OAuthProviderType // Provider type enum
	SkillName    string                  // Skill name for skills manager (e.g., "github")

	// OAuth endpoints
	AuthURL     string   // Authorization URL
	TokenURL    string   // Token exchange URL
	UserInfoURL string   // User info endpoint
	Scopes      []string // OAuth scopes

	// Credentials
	ClientID     string
	ClientSecret string
	Username     string
	Password     string

	// Browser automation callback
	GetAuthorizationCodeFunc func(authURL, state string) (string, error)

	// Test data setup/cleanup callbacks (optional)
	SetupTestDataFunc   func() error
	CleanupTestDataFunc func() error

	// Agent test queries (optional - will use defaults if not provided)
	AgentTestQueries []AgentTestQuery

	// Atlassian Cloud ID for Jira/Confluence testing
	AtlassianCloudID string
}

// AgentTestQuery defines a query to test agent integration
type AgentTestQuery struct {
	Query                 string
	SessionName           string
	ExpectedResponseCheck func(response string) bool // Function to validate response
}

// OAuthProviderTestTemplate provides templated test functions for OAuth provider testing
type OAuthProviderTestTemplate struct {
	config       OAuthProviderConfig
	baseSuite    *BaseOAuthTestSuite
	skillManager *api_skill.Manager

	// Test objects created during test
	oauthProvider *types.OAuthProvider
	testApp       *types.App
	oauthConn     *types.OAuthConnection
	skillConfig   *types.SkillDefinition
}

// NewOAuthProviderTestTemplate creates a new OAuth provider test template
func NewOAuthProviderTestTemplate(config OAuthProviderConfig, baseSuite *BaseOAuthTestSuite) (*OAuthProviderTestTemplate, error) {
	template := &OAuthProviderTestTemplate{
		config:    config,
		baseSuite: baseSuite,
	}

	// Initialize skills manager
	template.skillManager = api_skill.NewManager()
	err := template.skillManager.LoadSkills(baseSuite.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load skills: %w", err)
	}

	// Load skill configuration
	skillConfig, err := template.skillManager.GetSkill(config.SkillName)
	if err != nil {
		return nil, fmt.Errorf("failed to get skill %s: %w", config.SkillName, err)
	}
	template.skillConfig = skillConfig

	return template, nil
}

// TestSetupOAuthProvider creates and configures the OAuth provider
func (template *OAuthProviderTestTemplate) TestSetupOAuthProvider(t *testing.T) {
	log.Info().Str("provider_name", template.config.ProviderName).Msg("Setting up OAuth provider")

	// Create OAuth provider with all required fields
	// Note: Scopes are not stored on the provider - they're specified per-flow by the consumer
	callbackURL := template.baseSuite.serverURL + "/api/v1/oauth/flow/callback"
	provider := &types.OAuthProvider{
		Name:         template.config.ProviderName,
		Type:         template.config.ProviderType,
		Enabled:      true,
		ClientID:     template.config.ClientID,
		ClientSecret: template.config.ClientSecret,
		AuthURL:      template.config.AuthURL,
		TokenURL:     template.config.TokenURL,
		UserInfoURL:  template.config.UserInfoURL,
		CallbackURL:  callbackURL,
		CreatorID:    template.baseSuite.testUser.ID,
		CreatorType:  types.OwnerTypeUser,
	}

	log.Info().
		Str("callback_url", callbackURL).
		Str("server_url", template.baseSuite.serverURL).
		Msg("Configuring OAuth provider with callback URL")

	createdProvider, err := template.baseSuite.store.CreateOAuthProvider(template.baseSuite.ctx, provider)
	if err != nil {
		t.Fatalf("Failed to create OAuth provider: %v", err)
	}

	template.oauthProvider = createdProvider

	log.Info().
		Str("provider_id", createdProvider.ID).
		Str("provider_name", createdProvider.Name).
		Msg("OAuth provider created successfully")
}

// TestCreateTestApp creates a Helix app/agent with OAuth skills using full skill configuration
func (template *OAuthProviderTestTemplate) TestCreateTestApp(t *testing.T) {
	template.baseSuite.logger.Info().Msg("Creating test app with OAuth skills from skill definition")

	appName := fmt.Sprintf("%s Skills Test App %d", template.skillConfig.DisplayName, time.Now().Unix())

	app := &types.App{
		Owner:     template.baseSuite.testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        appName,
				Description: fmt.Sprintf("Test app for %s OAuth skills integration", template.skillConfig.DisplayName),
				Assistants: []types.AssistantConfig{
					{
						Name:         template.skillConfig.DisplayName + " Assistant",
						Description:  fmt.Sprintf("Assistant configured with %s OAuth skills", template.skillConfig.DisplayName),
						AgentType:    types.AgentTypeHelixAgent,
						SystemPrompt: template.skillConfig.SystemPrompt,
						// Configure LLM model (use anthropic if available, fallback to helix)
						Provider: "anthropic",
						Model:    "claude-3-5-haiku-20241022",

						// Configure reasoning and generation models for consistent agent behavior
						ReasoningModelProvider:  "anthropic",
						ReasoningModel:          "claude-3-5-haiku-20241022",
						GenerationModelProvider: "anthropic",
						GenerationModel:         "claude-3-5-haiku-20241022",

						// Configure small models for efficient agent operations
						SmallReasoningModelProvider:  "anthropic",
						SmallReasoningModel:          "claude-3-5-haiku-20241022",
						SmallGenerationModelProvider: "anthropic",
						SmallGenerationModel:         "claude-3-5-haiku-20241022",

						// Configure with OAuth skill using full configuration
						APIs: []types.AssistantAPI{
							{
								Name:            template.skillConfig.Name,
								Description:     template.skillConfig.Description,
								URL:             template.skillConfig.BaseURL,
								Schema:          template.skillConfig.Schema,
								Headers:         template.skillConfig.Headers,
								Query:           template.buildRequiredParametersQuery(),
								PathParams:      template.buildRequiredParametersPath(),
								SystemPrompt:    template.skillConfig.SystemPrompt,
								OAuthProvider:   template.oauthProvider.Name,
								SkipUnknownKeys: template.skillConfig.SkipUnknownKeys,
								TransformOutput: template.skillConfig.TransformOutput,
							},
						},
					},
				},
			},
		},
	}

	createdApp, err := template.baseSuite.store.CreateApp(template.baseSuite.ctx, app)
	require.NoError(t, err, "Failed to create test app")
	require.NotNil(t, createdApp, "Created app should not be nil")

	template.testApp = createdApp

	// Verify app was created correctly
	assert.Equal(t, appName, createdApp.Config.Helix.Name)
	assert.Equal(t, template.baseSuite.testUser.ID, createdApp.Owner)
	assert.Len(t, createdApp.Config.Helix.Assistants, 1)
	assert.Len(t, createdApp.Config.Helix.Assistants[0].APIs, 1)
	assert.Equal(t, template.oauthProvider.Name, createdApp.Config.Helix.Assistants[0].APIs[0].OAuthProvider)

	// Verify required parameters were properly configured
	if template.config.AtlassianCloudID != "" {
		// Check that cloudId was added to path parameters via requiredParameters system
		assert.Equal(t, template.config.AtlassianCloudID, createdApp.Config.Helix.Assistants[0].APIs[0].PathParams["cloudId"])
	}

	// Safely truncate description for logging
	description := template.skillConfig.Description
	if len(description) > 50 {
		description = description[:50] + "..."
	}

	template.baseSuite.logger.Info().
		Str("app_id", createdApp.ID).
		Str("app_name", createdApp.Config.Helix.Name).
		Str("skill_name", template.skillConfig.DisplayName).
		Str("skill_description", description).
		Msg("Test app created successfully with OAuth skill configuration")
}

// TestPerformOAuthFlow performs OAuth flow using Helix's OAuth endpoints
func (template *OAuthProviderTestTemplate) TestPerformOAuthFlow(t *testing.T) {
	template.baseSuite.logger.Info().Str("provider", template.skillConfig.DisplayName).Msg("Testing Helix OAuth flow")

	// Step 1: Start OAuth flow using Helix's endpoint
	// Use scopes from the skill YAML - these are the scopes the skill needs to function
	callbackURL := template.baseSuite.serverURL + "/api/v1/oauth/flow/callback"
	scopes := template.skillConfig.OAuthScopes
	template.baseSuite.logger.Info().Strs("scopes", scopes).Msg("Using scopes from skill YAML")
	authURL, state, err := template.baseSuite.StartOAuthFlow(template.oauthProvider.ID, callbackURL, scopes)
	require.NoError(t, err, "Failed to start Helix OAuth flow")

	template.baseSuite.logger.Info().
		Str("auth_url", authURL).
		Str("state", state).
		Msg("Successfully started Helix OAuth flow")

	// Step 2: Get authorization code from OAuth provider (simulate user completing OAuth in browser)
	authCode, err := template.config.GetAuthorizationCodeFunc(authURL, state)
	require.NoError(t, err, "Failed to get authorization code")

	template.baseSuite.logger.Info().
		Str("auth_code", authCode[:10]+"...").
		Msg("Successfully obtained authorization code")

	// Step 3: Complete OAuth flow using Helix's callback endpoint
	connection, err := template.baseSuite.CompleteOAuthFlow(template.oauthProvider.ID, authCode)
	require.NoError(t, err, "Failed to complete Helix OAuth flow")
	require.NotNil(t, connection, "OAuth connection should not be nil")

	template.oauthConn = connection

	// Verify connection was created correctly through Helix's OAuth system
	assert.Equal(t, template.baseSuite.testUser.ID, connection.UserID)
	assert.Equal(t, template.oauthProvider.ID, connection.ProviderID)
	assert.NotEmpty(t, connection.AccessToken)
	assert.NotNil(t, connection.Profile)

	template.baseSuite.logger.Info().
		Str("connection_id", connection.ID).
		Str("user_id", connection.UserID).
		Str("provider_id", connection.ProviderID).
		Str("username", connection.Profile.DisplayName).
		Msg("OAuth connection created successfully through Helix OAuth system")
}

// TestOAuthTokenDirectly tests OAuth token functionality (simplified)
func (template *OAuthProviderTestTemplate) TestOAuthTokenDirectly(t *testing.T) {
	require.NotNil(t, template.oauthConn, "OAuth connection should exist")
	require.NotEmpty(t, template.oauthConn.AccessToken, "OAuth access token should not be empty")

	template.baseSuite.logger.Info().Msg("OAuth token test passed - connection exists with access token")
}

// TestOAuthDebugging performs comprehensive OAuth 2.0 3LO debugging tests
func (template *OAuthProviderTestTemplate) TestOAuthDebugging(t *testing.T) {
	require.NotNil(t, template.oauthConn, "OAuth connection should exist")
	require.NotEmpty(t, template.oauthConn.AccessToken, "OAuth access token should not be empty")

	template.baseSuite.logger.Info().Str("provider", template.skillConfig.DisplayName).Msg("=== Starting OAuth 2.0 3LO Debugging ===")

	// Test 1: Validate OAuth token with accessible-resources endpoint
	template.testAccessibleResources(t)

	// Test 2: If this is an Atlassian provider, test the cloud ID and API endpoints
	if template.isAtlassianProvider() {
		template.testAtlassianCloudEndpoints(t)
	}

	// Test 3: Test user info endpoint
	template.testUserInfoEndpoint(t)

	template.baseSuite.logger.Info().Msg("=== OAuth 2.0 3LO Debugging Complete ===")
}

// testAccessibleResources tests the accessible-resources endpoint to verify OAuth token
func (template *OAuthProviderTestTemplate) testAccessibleResources(t *testing.T) {
	template.baseSuite.logger.Info().Msg("Testing accessible-resources endpoint")

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", "https://api.atlassian.com/oauth/token/accessible-resources", nil)
	require.NoError(t, err, "Failed to create accessible-resources request")

	req.Header.Set("Authorization", "Bearer "+template.oauthConn.AccessToken)
	req.Header.Set("Accept", "application/json")

	template.baseSuite.logger.Info().
		Str("url", req.URL.String()).
		Str("method", req.Method).
		Str("auth_header", "Bearer "+template.oauthConn.AccessToken[:20]+"...").
		Msg("Making accessible-resources request")

	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to make accessible-resources request")
	defer resp.Body.Close()

	template.baseSuite.logger.Info().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Msg("Accessible-resources response received")

	// Read and log response body
	var responseBody interface{}
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	if err != nil {
		template.baseSuite.logger.Error().Err(err).Msg("Failed to decode accessible-resources response")
		return
	}

	responseJSON, _ := json.MarshalIndent(responseBody, "", "  ")
	template.baseSuite.logger.Info().
		Str("response_body", string(responseJSON)).
		Msg("Accessible-resources response body")

	if resp.StatusCode == 200 {
		// If successful, extract cloud IDs and site information
		if resourcesArray, ok := responseBody.([]interface{}); ok {
			template.baseSuite.logger.Info().
				Int("sites_count", len(resourcesArray)).
				Msg("Found accessible sites")

			for i, resource := range resourcesArray {
				if resourceMap, ok := resource.(map[string]interface{}); ok {
					template.baseSuite.logger.Info().
						Int("site_index", i).
						Str("site_id", fmt.Sprintf("%v", resourceMap["id"])).
						Str("site_name", fmt.Sprintf("%v", resourceMap["name"])).
						Str("site_url", fmt.Sprintf("%v", resourceMap["url"])).
						Msg("Found accessible site")
				}
			}
		}
	} else {
		template.baseSuite.logger.Error().
			Int("status_code", resp.StatusCode).
			Str("error_response", string(responseJSON)).
			Msg("Accessible-resources request failed")
	}
}

// testAtlassianCloudEndpoints tests Atlassian-specific cloud endpoints
func (template *OAuthProviderTestTemplate) testAtlassianCloudEndpoints(t *testing.T) {
	if template.config.AtlassianCloudID == "" {
		template.baseSuite.logger.Info().Msg("No Atlassian Cloud ID configured, skipping cloud endpoint tests")
		return
	}

	template.baseSuite.logger.Info().
		Str("cloud_id", template.config.AtlassianCloudID).
		Msg("Testing Atlassian Cloud endpoints")

	client := &http.Client{Timeout: 30 * time.Second}

	// Test endpoints that should work with OAuth 2.0 3LO
	testEndpoints := []struct {
		name string
		url  string
	}{
		{
			name: "serverInfo",
			url:  fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/serverInfo", template.config.AtlassianCloudID),
		},
		{
			name: "myself",
			url:  fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/myself", template.config.AtlassianCloudID),
		},
		{
			name: "project search",
			url:  fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/project/search", template.config.AtlassianCloudID),
		},
		{
			name: "issue search",
			url:  fmt.Sprintf("https://api.atlassian.com/ex/jira/%s/rest/api/3/search?jql=project is not empty", template.config.AtlassianCloudID),
		},
	}

	for _, endpoint := range testEndpoints {
		template.testAtlassianEndpoint(t, client, endpoint.name, endpoint.url)
	}
}

// testAtlassianEndpoint tests a specific Atlassian endpoint
func (template *OAuthProviderTestTemplate) testAtlassianEndpoint(t *testing.T, client *http.Client, name, url string) {
	template.baseSuite.logger.Info().
		Str("endpoint_name", name).
		Str("url", url).
		Msg("Testing Atlassian endpoint")

	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err, "Failed to create request for "+name)

	req.Header.Set("Authorization", "Bearer "+template.oauthConn.AccessToken)
	req.Header.Set("Accept", "application/json")

	template.baseSuite.logger.Info().
		Str("endpoint_name", name).
		Str("method", req.Method).
		Str("auth_header", "Bearer "+template.oauthConn.AccessToken[:20]+"...").
		Msg("Making endpoint request")

	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to make request to "+name)
	defer resp.Body.Close()

	template.baseSuite.logger.Info().
		Str("endpoint_name", name).
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Msg("Endpoint response received")

	// Log response headers
	for headerName, headerValues := range resp.Header {
		for _, headerValue := range headerValues {
			template.baseSuite.logger.Info().
				Str("endpoint_name", name).
				Str("header_name", headerName).
				Str("header_value", headerValue).
				Msg("Response header")
		}
	}

	// Read and log response body (first 500 chars to avoid spam)
	var responseBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	if err != nil {
		template.baseSuite.logger.Error().
			Str("endpoint_name", name).
			Err(err).
			Msg("Failed to decode endpoint response")
		return
	}

	responseJSON, _ := json.MarshalIndent(responseBody, "", "  ")
	responseStr := string(responseJSON)
	if len(responseStr) > 500 {
		responseStr = responseStr[:500] + "... (truncated)"
	}

	template.baseSuite.logger.Info().
		Str("endpoint_name", name).
		Str("response_body", responseStr).
		Msg("Endpoint response body")

	if resp.StatusCode != 200 {
		template.baseSuite.logger.Error().
			Str("endpoint_name", name).
			Int("status_code", resp.StatusCode).
			Str("error_response", responseStr).
			Msg("Endpoint request failed")
	} else {
		template.baseSuite.logger.Info().
			Str("endpoint_name", name).
			Msg("Endpoint request successful")
	}
}

// testUserInfoEndpoint tests the user info endpoint from the OAuth provider config
func (template *OAuthProviderTestTemplate) testUserInfoEndpoint(t *testing.T) {
	if template.config.UserInfoURL == "" {
		template.baseSuite.logger.Info().Msg("No user info URL configured, skipping user info test")
		return
	}

	template.baseSuite.logger.Info().
		Str("user_info_url", template.config.UserInfoURL).
		Msg("Testing user info endpoint")

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", template.config.UserInfoURL, nil)
	require.NoError(t, err, "Failed to create user info request")

	req.Header.Set("Authorization", "Bearer "+template.oauthConn.AccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err, "Failed to make user info request")
	defer resp.Body.Close()

	template.baseSuite.logger.Info().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Msg("User info response received")

	var responseBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	if err != nil {
		template.baseSuite.logger.Error().Err(err).Msg("Failed to decode user info response")
		return
	}

	responseJSON, _ := json.MarshalIndent(responseBody, "", "  ")
	template.baseSuite.logger.Info().
		Str("response_body", string(responseJSON)).
		Msg("User info response body")
}

// isAtlassianProvider checks if this is an Atlassian provider (Jira/Confluence)
func (template *OAuthProviderTestTemplate) isAtlassianProvider() bool {
	return template.config.ProviderType == types.OAuthProviderTypeAtlassian ||
		strings.Contains(strings.ToLower(template.config.ProviderName), "jira") ||
		strings.Contains(strings.ToLower(template.config.ProviderName), "confluence") ||
		strings.Contains(strings.ToLower(template.config.ProviderName), "atlassian")
}

// TestAgentOAuthSkillsIntegration tests the complete agent integration with OAuth skills
func (template *OAuthProviderTestTemplate) TestAgentOAuthSkillsIntegration(t *testing.T) {
	template.baseSuite.logger.Info().Str("provider", template.skillConfig.DisplayName).Msg("=== Testing Agent OAuth Skills Integration ===")

	// Verify OAuth connection exists and is accessible
	connections, err := template.baseSuite.store.ListOAuthConnections(template.baseSuite.ctx, &store.ListOAuthConnectionsQuery{
		UserID: template.baseSuite.testUser.ID,
	})
	require.NoError(t, err, "Failed to list OAuth connections")

	template.baseSuite.logger.Info().
		Int("connections_found", len(connections)).
		Str("user_id", template.baseSuite.testUser.ID).
		Msg("OAuth connections found for test user")

	require.NotZero(t, len(connections), "Should have at least one OAuth connection")

	// Find OAuth connection for this provider
	var oauthConn *types.OAuthConnection
	for _, conn := range connections {
		template.baseSuite.logger.Info().
			Str("connection_id", conn.ID).
			Str("provider_id", conn.ProviderID).
			Str("user_id", conn.UserID).
			Msg("Found OAuth connection")

		if conn.ProviderID == template.oauthProvider.ID {
			oauthConn = conn
			break
		}
	}

	require.NotNil(t, oauthConn, "Should have OAuth connection")
	require.NotEmpty(t, oauthConn.AccessToken, "OAuth connection should have access token")

	template.baseSuite.logger.Info().
		Str("provider_id", oauthConn.ProviderID).
		Str("user_id", oauthConn.UserID).
		Str("access_token", oauthConn.AccessToken[:10]+"...").
		Msg("Verified OAuth connection exists with access token")

	// Get test queries (use defaults if not provided)
	testQueries := template.config.AgentTestQueries
	if len(testQueries) == 0 {
		testQueries = template.getDefaultAgentTestQueries()
	}

	// Execute test queries
	for i, query := range testQueries {
		template.baseSuite.logger.Info().
			Int("query_index", i+1).
			Str("query", query.Query).
			Msg("Testing agent query with real execution")

		response, err := template.baseSuite.ExecuteSessionQuery(query.Query, query.SessionName, template.testApp.ID)
		require.NoError(t, err, fmt.Sprintf("Failed to execute query %d", i+1))

		template.baseSuite.logger.Info().
			Str("user_query", query.Query).
			Str("agent_response", response[:min(len(response), 200)]+"...").
			Msg("Agent responded to query")

		// Verify response if check function provided
		if query.ExpectedResponseCheck != nil {
			assert.True(t, query.ExpectedResponseCheck(response),
				"Agent response did not pass validation check for query: %s. Response: %s", query.Query, response)
		} else {
			// Default check - just ensure response is not empty
			assert.NotEmpty(t, response, "Agent should provide a response to query: %s", query.Query)
		}

		// Log conversation for manual verification
		template.baseSuite.LogAgentConversation(query.SessionName, query.Query, response,
			template.config.SkillName+"_oauth_e2e", template.skillConfig.DisplayName)
	}

	template.baseSuite.logger.Info().
		Str("oauth_connection_id", oauthConn.ID).
		Str("provider", template.skillConfig.DisplayName).
		Msg("Successfully executed OAuth skills sessions with real agent responses")
}

// SetupTestData calls the optional setup function for provider-specific test data
func (template *OAuthProviderTestTemplate) SetupTestData(t *testing.T) {
	if template.config.SetupTestDataFunc != nil {
		template.baseSuite.logger.Info().Str("provider", template.skillConfig.DisplayName).Msg("Setting up provider-specific test data")
		err := template.config.SetupTestDataFunc()
		require.NoError(t, err, "Failed to setup provider-specific test data")
	}
}

// CleanupTestData calls the optional cleanup function for provider-specific test data
func (template *OAuthProviderTestTemplate) CleanupTestData(_ *testing.T) {
	if template.config.CleanupTestDataFunc != nil {
		template.baseSuite.logger.Info().Str("provider", template.skillConfig.DisplayName).Msg("Cleaning up provider-specific test data")
		if err := template.config.CleanupTestDataFunc(); err != nil {
			template.baseSuite.logger.Error().Err(err).Msg("Failed to cleanup provider-specific test data")
		}
	}
}

// Cleanup cleans up OAuth test resources
func (template *OAuthProviderTestTemplate) Cleanup(_ *testing.T) {
	template.baseSuite.logger.Info().Str("provider", template.skillConfig.DisplayName).Msg("=== Starting OAuth Provider Test Cleanup ===")

	// Delete OAuth connection
	if template.oauthConn != nil {
		err := template.baseSuite.store.DeleteOAuthConnection(template.baseSuite.ctx, template.oauthConn.ID)
		if err != nil {
			template.baseSuite.logger.Error().Err(err).Msg("Failed to delete OAuth connection")
		} else {
			template.baseSuite.logger.Info().Msg("OAuth connection deleted")
		}
	}

	// Delete test app
	if template.testApp != nil {
		err := template.baseSuite.store.DeleteApp(template.baseSuite.ctx, template.testApp.ID)
		if err != nil {
			template.baseSuite.logger.Error().Err(err).Msg("Failed to delete test app")
		} else {
			template.baseSuite.logger.Info().Msg("Test app deleted")
		}
	}

	// Delete OAuth provider
	if template.oauthProvider != nil {
		err := template.baseSuite.store.DeleteOAuthProvider(template.baseSuite.ctx, template.oauthProvider.ID)
		if err != nil {
			template.baseSuite.logger.Error().Err(err).Msg("Failed to delete OAuth provider")
		} else {
			template.baseSuite.logger.Info().Msg("OAuth provider deleted")
		}
	}

	template.baseSuite.logger.Info().Str("provider", template.skillConfig.DisplayName).Msg("=== OAuth Provider Test Cleanup Completed ===")
}

// buildRequiredParametersQuery builds query parameters for the API configuration based on skill's requiredParameters
func (template *OAuthProviderTestTemplate) buildRequiredParametersQuery() map[string]string {
	queryParams, _ := template.buildRequiredParameters()
	return queryParams
}

// buildRequiredParametersPath builds path parameters for the API configuration based on skill's requiredParameters
func (template *OAuthProviderTestTemplate) buildRequiredParametersPath() map[string]string {
	_, pathParams := template.buildRequiredParameters()
	return pathParams
}

// buildRequiredParameters builds both query and path parameters for the API configuration
func (template *OAuthProviderTestTemplate) buildRequiredParameters() (map[string]string, map[string]string) {
	queryParams := make(map[string]string)
	pathParams := make(map[string]string)

	// Process the skill's required parameters and add values based on their type
	for _, param := range template.skillConfig.RequiredParameters {
		var value string

		// Get the value for this parameter
		switch param.Name {
		case "cloudId":
			value = template.config.AtlassianCloudID
		// Add more parameters here as needed for other skills
		default:
			continue // Skip parameters we don't have values for
		}

		// Add to appropriate parameter type
		switch param.Type {
		case "query":
			queryParams[param.Name] = value
		case "path":
			pathParams[param.Name] = value
			// headers would be handled separately in buildRequiredParametersHeaders()
		}
	}

	return queryParams, pathParams
}

// getDefaultAgentTestQueries returns default test queries for agent integration testing
func (template *OAuthProviderTestTemplate) getDefaultAgentTestQueries() []AgentTestQuery {
	providerName := template.skillConfig.DisplayName

	return []AgentTestQuery{
		{
			Query:       fmt.Sprintf("What is my %s username?", providerName),
			SessionName: fmt.Sprintf("%s Username Query", providerName),
			ExpectedResponseCheck: func(response string) bool {
				return len(response) > 0 // Just check that we get some response
			},
		},
		{
			Query:       fmt.Sprintf("Test my %s connection", providerName),
			SessionName: fmt.Sprintf("%s Connection Test", providerName),
			ExpectedResponseCheck: func(response string) bool {
				// Check for successful connection indicators
				lower := strings.ToLower(response)
				return strings.Contains(lower, "connect") || strings.Contains(lower, "success") ||
					strings.Contains(lower, "work") || len(response) > 20
			},
		},
	}
}
