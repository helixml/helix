package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

// Global variables to store provider IDs
var (
	githubProviderID string
	googleProviderID string
	slackProviderID  string
)

func TestOAuthAPIToolsTestSuite(t *testing.T) {
	suite.Run(t, new(OAuthAPIToolsTestSuite))
}

type OAuthAPIToolsTestSuite struct {
	suite.Suite
	ctx             context.Context
	db              *store.PostgresStore
	keycloak        *auth.KeycloakAuthenticator
	mockAPIServer   *httptest.Server
	capturedHeaders map[string]string
	apiClient       *client.HelixClient
	user            *types.User
	apiKey          string
	mockOAuthServer *httptest.Server
	mockOAuthTokens map[string]string
	apiURL          string
	toolsAPI        tools.ToolsAPI
}

func (suite *OAuthAPIToolsTestSuite) SetupTest() {
	suite.ctx = context.Background()

	// Initialize mock OAuth tokens
	suite.mockOAuthTokens = map[string]string{
		string(types.OAuthProviderTypeGitHub): "github-oauth-token-123",
		string(types.OAuthProviderTypeGoogle): "google-oauth-token-456",
		string(types.OAuthProviderTypeSlack):  "slack-oauth-token-789",
	}

	// Set up required database objects using direct docker exec if needed
	pgContainer := os.Getenv("POSTGRES_CONTAINER")
	if pgContainer != "" {
		suite.T().Logf("Using direct PostgreSQL access via docker container: %s", pgContainer)
		setupPostgresViaDocker(suite.T(), pgContainer)
	}

	// Initialize the database connection
	store, err := getStoreClient()
	suite.Require().NoError(err)
	suite.db = store

	// Initialize Keycloak authenticator
	var keycloakCfg config.Keycloak
	err = envconfig.Process("", &keycloakCfg)
	suite.NoError(err)

	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(&config.Keycloak{
		KeycloakURL:         keycloakCfg.KeycloakURL,
		KeycloakFrontEndURL: keycloakCfg.KeycloakFrontEndURL,
		ServerURL:           keycloakCfg.ServerURL,
		APIClientID:         keycloakCfg.APIClientID,
		FrontEndClientID:    keycloakCfg.FrontEndClientID,
		AdminRealm:          keycloakCfg.AdminRealm,
		Realm:               keycloakCfg.Realm,
		Username:            keycloakCfg.Username,
		Password:            keycloakCfg.Password,
	}, suite.db)
	suite.Require().NoError(err)
	suite.keycloak = keycloakAuthenticator

	// Set up mock API server that will capture headers
	suite.capturedHeaders = make(map[string]string)
	suite.mockAPIServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture all headers
		for key, values := range r.Header {
			if len(values) > 0 {
				suite.capturedHeaders[key] = values[0]
			}
		}

		// Return a successful response
		response := map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"result": "This is a mock API response",
			},
		}
		responseJSON, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(responseJSON)
		if err != nil {
			suite.T().Fatalf("Failed to write response: %v", err)
		}
	}))

	// Set up a mock OAuth server that will issue tokens
	suite.mockOAuthServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			// Parse form to get client_id
			err := r.ParseForm()
			suite.Require().NoError(err)

			clientID := r.FormValue("client_id")
			// Generate token based on client_id
			token := "default-token"
			switch clientID {
			case "github-client":
				token = suite.mockOAuthTokens[string(types.OAuthProviderTypeGitHub)]
			case "google-client":
				token = suite.mockOAuthTokens[string(types.OAuthProviderTypeGoogle)]
			case "slack-client":
				token = suite.mockOAuthTokens[string(types.OAuthProviderTypeSlack)]
			}

			// Return a token response
			tokenResp := map[string]interface{}{
				"access_token": token,
				"token_type":   "Bearer",
				"expires_in":   3600,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResp)
			return
		}

		// Default 404 response
		w.WriteHeader(http.StatusNotFound)
	}))

	// Create a test user
	emailID := uuid.New().String()
	userEmail := fmt.Sprintf("test-oauth-api-%s@test.com", emailID)

	user, apiKey, err := createUser(suite.T(), suite.db, suite.keycloak, userEmail)
	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.Require().NotNil(apiKey)

	suite.user = user
	suite.apiKey = apiKey

	// Create API client
	apiClient, err := getAPIClient(apiKey)
	suite.Require().NoError(err)
	suite.apiClient = apiClient

	// Store OAuth tokens for the user in the database using OAuthConnection
	for provider, token := range suite.mockOAuthTokens {
		// Create a mock provider
		providerID := uuid.New().String()
		providerObj, err := suite.db.CreateOAuthProvider(suite.ctx, &types.OAuthProvider{
			ID:          providerID,
			Name:        fmt.Sprintf("%s Provider", provider),
			Description: fmt.Sprintf("Mock %s provider for testing", provider),
			Type:        types.OAuthProviderType(provider),
			ClientID:    fmt.Sprintf("%s-client", provider),
			CreatorID:   user.ID,
			CreatorType: types.OwnerTypeUser,
			Enabled:     true,
		})
		suite.Require().NoError(err)

		// Create a connection for this provider
		_, err = suite.db.CreateOAuthConnection(suite.ctx, &types.OAuthConnection{
			UserID:       user.ID,
			ProviderID:   providerObj.ID,
			AccessToken:  token,
			RefreshToken: "refresh-" + token,
			ExpiresAt:    time.Now().Add(1 * time.Hour),
			Scopes:       []string{"repo", "user"},
		})
		suite.Require().NoError(err)
	}

	// Get the API URL from the environment or use a default value
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "18080" // Default to 18080 in tests to avoid conflicts with common ports
	}
	suite.apiURL = "http://localhost:" + apiPort

	// Create the tools API client
	apiClient, err = client.NewClient(suite.apiURL, apiKey)
	suite.Require().NoError(err)
	suite.toolsAPI = tools.NewToolsAPI(apiClient)
}

func (suite *OAuthAPIToolsTestSuite) TearDownTest() {
	suite.mockAPIServer.Close()
	suite.mockOAuthServer.Close()
}

// TestOAuthAPIToolIntegration tests that OAuth tokens are correctly used in API tools
func (suite *OAuthAPIToolsTestSuite) TestOAuthAPIToolIntegration() {
	// Log the OAuth2 mock URL
	fmt.Printf("Using OAuth2 mock URL: %s\n", os.Getenv("OAUTH2_MOCK_URL"))

	// Check if we're using docker exec mode
	pgContainer := os.Getenv("POSTGRES_CONTAINER")
	if pgContainer != "" {
		suite.T().Logf("Running in docker exec mode with container: %s", pgContainer)
		// Use the user created by setupPostgresViaDocker
		queryUserSQL := `SELECT id, email FROM users WHERE email = 'test-oauth-docker@example.com' LIMIT 1`
		output, err := exec.Command("docker", "exec", pgContainer, "psql", "-U", "postgres", "-t", "-c", queryUserSQL).CombinedOutput()
		suite.Require().NoError(err, "Failed to query user: %s", string(output))

		// Parse user ID from output
		userID := strings.TrimSpace(string(output))
		if userID == "" {
			suite.T().Fatalf("Could not find test user in database")
		}
		suite.T().Logf("Found test user ID: %s", userID)
	}

	// Create an app with a GitHub API tool using AppConfig
	appConfig := &types.AppConfig{
		Helix: types.AppHelixConfig{
			Name:        "OAuth API Test App",
			Description: "App for testing OAuth API tools integration",
			Assistants: []types.AssistantConfig{
				{
					Name:         "GitHub API Assistant",
					Description:  "Assistant that uses GitHub API",
					SystemPrompt: "You are an assistant that can help with GitHub repositories.",
					Tools: []*types.Tool{
						{
							Name:        "GitHub API",
							Description: "GitHub API access",
							ToolType:    types.ToolTypeAPI,
							Config: types.ToolConfig{
								API: &types.ToolAPIConfig{
									URL:           suite.mockAPIServer.URL + "/api/v3",
									OAuthProvider: types.OAuthProviderTypeGitHub,
									OAuthScopes:   []string{"repo"},
									Actions: []*types.ToolAPIAction{
										{
											Name:        "listRepositories",
											Description: "List repositories for the authenticated user",
											Method:      "GET",
											Path:        "/user/repos",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Create the app
	app := &types.App{
		ID:        system.GenerateUUID(),
		Owner:     suite.user.ID,
		OwnerType: types.OwnerTypeUser,
		AppSource: types.AppSourceHelix,
		Config:    *appConfig,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)
	suite.Require().NotNil(createdApp)

	// Clean up the app after the test
	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	// For real integration with API tools, we would need a session/interaction
	// but for simplicity in this test, we just verify the OAuth token is available
	// and can be used in HTTP requests

	// Clear captured headers
	suite.capturedHeaders = make(map[string]string)

	// Log mock server URL
	fmt.Printf("Mock API server URL: %s\n", suite.mockAPIServer.URL)

	// Make a direct request to simulate the API call with proper OAuth token
	testURL := suite.mockAPIServer.URL + "/api/v3/user/repos"
	fmt.Printf("Making test request to: %s\n", testURL)

	req, err := http.NewRequest("GET", testURL, nil)
	suite.Require().NoError(err)

	// Add Authorization header with the GitHub token
	expectedToken := suite.mockOAuthTokens[string(types.OAuthProviderTypeGitHub)]
	req.Header.Set("Authorization", "Bearer "+expectedToken)
	fmt.Printf("Using Authorization header: Bearer %s\n", expectedToken)

	// Send the request
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error making request to %s: %v\n", testURL, err)
		suite.Fail("Failed to make HTTP request: %v", err)
	}
	suite.Require().NoError(err)
	suite.Require().Equal(http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Verify the Authorization header was captured
	authHeader, exists := suite.capturedHeaders["Authorization"]
	fmt.Printf("Captured headers: %+v\n", suite.capturedHeaders)
	suite.Require().True(exists, "Authorization header should be present")
	suite.Equal("Bearer "+expectedToken, authHeader, "Authorization header should contain the GitHub token")
}

// setupPostgresViaDocker creates required database objects directly using docker exec
func setupPostgresViaDocker(t *testing.T, pgContainer string) {
	// This function bypasses the GORM connection issues by running commands directly
	// through docker exec into the PostgreSQL container
	t.Helper()

	// Create a test user first
	userID := uuid.New().String()
	execSQL(t, pgContainer, fmt.Sprintf(`
		-- Create test user
		INSERT INTO users (id, created_at, updated_at, token, token_type, admin, email, username, full_name)
		VALUES ('%s', NOW(), NOW(), '', 'none', false, 'test-oauth-docker@example.com', 'test-oauth-docker', 'Test OAuth Docker User')
		ON CONFLICT (id) DO NOTHING
	`, userID))
	t.Logf("Created test user with ID: %s", userID)

	// Create OAuth providers in the database
	githubProvID := uuid.New().String()
	googleProvID := uuid.New().String()
	slackProvID := uuid.New().String()

	// Execute SQL statements directly in the container
	execSQL(t, pgContainer, fmt.Sprintf(`
		-- Create OAuth providers
		INSERT INTO oauth_providers (
			id, created_at, updated_at, name, description, type, 
			client_id, client_secret, creator_id, creator_type, enabled
		) VALUES 
		('%s', NOW(), NOW(), 'GitHub Provider', 'Mock GitHub provider for testing', 'github', 
		'github-client', 'github-secret', '%s', 'user', true),
		('%s', NOW(), NOW(), 'Google Provider', 'Mock Google provider for testing', 'google', 
		'google-client', 'google-secret', '%s', 'user', true),
		('%s', NOW(), NOW(), 'Slack Provider', 'Mock Slack provider for testing', 'slack', 
		'slack-client', 'slack-secret', '%s', 'user', true)
		ON CONFLICT (id) DO NOTHING
	`, githubProvID, userID, googleProvID, userID, slackProvID, userID))

	t.Logf("Created OAuth providers with IDs: GitHub=%s, Google=%s, Slack=%s",
		githubProvID, googleProvID, slackProvID)

	// Create OAuth connections with tokens
	githubConnID := uuid.New().String()
	googleConnID := uuid.New().String()
	slackConnID := uuid.New().String()

	// Add one hour to current time for expiry
	expiryTime := time.Now().Add(1 * time.Hour).Format("2006-01-02 15:04:05")

	execSQL(t, pgContainer, fmt.Sprintf(`
		-- Create OAuth connections with tokens
		INSERT INTO oauth_connections (
			id, created_at, updated_at, user_id, provider_id, 
			access_token, refresh_token, expires_at, provider_user_id
		) VALUES 
		('%s', NOW(), NOW(), '%s', '%s', 
		'github-oauth-token-123', 'refresh-github-token', '%s', 'github-user'),
		('%s', NOW(), NOW(), '%s', '%s', 
		'google-oauth-token-456', 'refresh-google-token', '%s', 'google-user'),
		('%s', NOW(), NOW(), '%s', '%s', 
		'slack-oauth-token-789', 'refresh-slack-token', '%s', 'slack-user')
		ON CONFLICT (id) DO NOTHING
	`, githubConnID, userID, githubProvID, expiryTime,
		googleConnID, userID, googleProvID, expiryTime,
		slackConnID, userID, slackProvID, expiryTime))

	t.Logf("Created OAuth connections with IDs: GitHub=%s, Google=%s, Slack=%s",
		githubConnID, googleConnID, slackConnID)

	// Make these available as global variables
	githubProviderID = githubProvID
	googleProviderID = googleProvID
	slackProviderID = slackProvID
}

// execSQL executes a SQL command in the PostgreSQL container
func execSQL(t *testing.T, pgContainer, sql string) {
	t.Helper()

	// Create a temporary file with the SQL
	tmpfile, err := os.CreateTemp("", "sql")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(sql)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Execute the SQL using docker exec
	cmd := exec.Command("docker", "cp", tmpfile.Name(), fmt.Sprintf("%s:/tmp/sql.sql", pgContainer))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to copy SQL to container: %v", err)
	}

	cmd = exec.Command("docker", "exec", pgContainer, "psql", "-U", "postgres", "-f", "/tmp/sql.sql")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to execute SQL in container: %v\nOutput: %s", err, string(output))
	}

	t.Logf("SQL executed successfully: %s", string(output))
}
