//go:build integration
// +build integration

package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestOAuthAPIToolIntegration(t *testing.T) {
	// Skip if in short mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 1. Set up a mock GitHub API server that will validate the Authorization header
	var capturedAuthHeader string
	mockGithubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the Authorization header
		capturedAuthHeader = r.Header.Get("Authorization")

		// Check path and respond accordingly
		if r.URL.Path == "/user" {
			// Respond with user info
			resp := map[string]interface{}{
				"login": "testuser",
				"id":    12345,
				"name":  "Test User",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/repos/testuser/testrepo/issues" {
			// Respond with issues data
			if capturedAuthHeader == "" {
				// If no auth header, return 401
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"message":"Unauthorized"}`))
				return
			}

			resp := []map[string]interface{}{
				{
					"id":    1,
					"title": "Test Issue 1",
					"state": "open",
				},
				{
					"id":    2,
					"title": "Test Issue 2",
					"state": "closed",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockGithubServer.Close()

	// 2. Connect to the database to create test data
	cfg, err := config.LoadServerConfig()
	require.NoError(t, err, "failed to load server config")

	// When running outside the container, we need to modify the database connection parameters
	// to connect to the local PostgreSQL instance
	cfg.Store.Host = "localhost"
	cfg.Store.Port = 5432
	cfg.Store.Username = "postgres"
	cfg.Store.Password = "postgres"
	cfg.Store.Database = "postgres"

	// Update Keycloak settings to use localhost
	cfg.Keycloak.KeycloakURL = "http://localhost:8080/auth"
	cfg.Keycloak.ServerURL = "http://localhost:8080"
	cfg.Keycloak.KeycloakFrontEndURL = "http://localhost:8080/auth"
	// Set Keycloak admin credentials to match docker-compose defaults
	cfg.Keycloak.Username = "admin"
	cfg.Keycloak.Password = "oh-hallo-insecure-password" // Default in docker-compose.dev.yaml

	// Make sure the WebServer port is set correctly to match docker-compose
	cfg.WebServer.Port = 8080

	// Create a store connection
	db, err := store.NewPostgresStore(cfg.Store)
	require.NoError(t, err, "failed to create store")
	defer db.Close()

	// 3. Set up a test Keycloak authenticator and create a test user
	email := fmt.Sprintf("oauth-test-%d@example.com", time.Now().Unix())
	var testUser *types.User
	keycloakEnabled := true

	kc, err := auth.NewKeycloakAuthenticator(&cfg.Keycloak, db)
	if err != nil {
		t.Logf("Warning: Failed to connect to Keycloak: %v", err)
		t.Logf("Continuing with a manual test user instead of using Keycloak")
		keycloakEnabled = false

		// Create a test user without Keycloak
		testUserID := fmt.Sprintf("user_%d", time.Now().UnixNano())
		testUser = &types.User{
			ID:       testUserID,
			Email:    email,
			Username: email,
			FullName: "OAuth Test User",
		}

		// Create the user directly in the database
		testUser, err = db.CreateUser(context.Background(), testUser)
		require.NoError(t, err, "failed to create user in database")
	} else {
		// Create the user in Keycloak
		testUser = &types.User{
			Email:    email,
			Username: email,
			FullName: "OAuth Test User",
		}

		createdUser, err := kc.CreateKeycloakUser(context.Background(), testUser)
		require.NoError(t, err, "failed to create user in Keycloak")

		// Set the ID from Keycloak
		testUser.ID = createdUser.ID

		// Create the user in our database
		testUser, err = db.CreateUser(context.Background(), testUser)
		require.NoError(t, err, "failed to create user in database")
	}

	// 5. Create an API key for the user
	apiKey, err := createAPIKey(t, db, testUser.ID)
	require.NoError(t, err, "failed to create API key")

	// 6. Create an OAuth provider for GitHub using our mock server
	githubProvider := &types.OAuthProvider{
		Name:         "GitHub",
		Type:         types.OAuthProviderTypeGitHub,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      mockGithubServer.URL + "/login/oauth/authorize",
		TokenURL:     mockGithubServer.URL + "/login/oauth/access_token",
		UserInfoURL:  mockGithubServer.URL + "/user",
		Enabled:      true,
	}

	createdProvider, err := db.CreateOAuthProvider(context.Background(), githubProvider)
	require.NoError(t, err, "failed to create OAuth provider")

	// 7. Create an OAuth connection for the user with the GitHub provider
	githubConnection := &types.OAuthConnection{
		UserID:       testUser.ID,
		ProviderID:   createdProvider.ID,
		AccessToken:  "github-test-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(24 * time.Hour), // Set expiry in the future
		Scopes:       []string{"repo", "read:user"},  // Standard GitHub scopes
	}

	_, err = db.CreateOAuthConnection(context.Background(), githubConnection)
	require.NoError(t, err, "failed to create OAuth connection")

	// 8. Create a client to communicate with our API
	apiClient, err := client.NewClient(fmt.Sprintf("http://localhost:%d", cfg.WebServer.Port), apiKey)
	if err != nil {
		t.Fatalf("Failed to create API client: %v. Make sure the API server is running on port %d", err, cfg.WebServer.Port)
	}

	// Try a simple request to make sure we can connect to the API server
	configURL := fmt.Sprintf("http://localhost:%d/api/v1/config", cfg.WebServer.Port)
	req, _ := http.NewRequest("GET", configURL, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 400 {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		t.Fatalf("Cannot connect to API server at %s: %v (status: %d). Make sure the API server is running.",
			configURL, err, statusCode)
	}

	// 9. Create an app
	appConfig := types.AppConfig{
		Helix: types.AppHelixConfig{
			Name:        "OAuth Test App",
			Description: "Test app for OAuth integration testing",
		},
	}

	app := &types.App{
		Owner:     testUser.ID,
		OwnerType: types.OwnerTypeUser,
		AppSource: types.AppSourceHelix,
		Config:    appConfig,
	}

	createdApp, err := db.CreateApp(context.Background(), app)
	require.NoError(t, err, "failed to create app")

	// 10. Create a GitHub API tool with our mock server URL
	githubToolSchema := `
	openapi: 3.0.0
	info:
	  title: GitHub API
	  description: Access GitHub repositories, issues, and pull requests
	  version: "1.0"
	paths:
	  /user:
	    get:
	      summary: Get authenticated user
	      description: Get the currently authenticated user
	      operationId: getAuthenticatedUser
	      responses:
	        '200':
	          description: Successful operation
	  /repos/{owner}/{repo}/issues:
	    get:
	      summary: List repository issues
	      description: List issues in a repository
	      operationId: listRepositoryIssues
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
	      responses:
	        '200':
	          description: Successful operation
	`

	githubTool := &types.Tool{
		Name:        "GitHub API",
		Description: "GitHub API access",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:           mockGithubServer.URL,
				OAuthProvider: "GitHub",
				OAuthScopes:   []string{"repo"},
				Schema:        githubToolSchema,
				Actions: []*types.ToolAPIAction{
					{
						Name:        "getAuthenticatedUser",
						Description: "Get the currently authenticated user",
						Path:        "/user",
						Method:      "GET",
					},
					{
						Name:        "listRepositoryIssues",
						Description: "List issues in a repository",
						Path:        "/repos/{owner}/{repo}/issues",
						Method:      "GET",
					},
				},
			},
		},
	}

	// Store doesn't have a CreateTool method, so we'll add tool directly to app config
	toolID := fmt.Sprintf("tool_%d", time.Now().UnixNano())
	githubTool.ID = toolID

	// 11. Update the app config to include the tool
	appConfig.Helix.Assistants = []types.AssistantConfig{
		{
			Name:        "Default Assistant",
			Description: "Default assistant with GitHub API tool",
			Provider:    string(types.ProviderTogetherAI),
			Model:       "meta-llama/Llama-3.1-8B-Instruct",
			Tools:       []*types.Tool{githubTool}, // Using the tool directly rather than createdTool
		},
	}

	createdApp.Config = appConfig
	_, err = db.UpdateApp(context.Background(), createdApp)
	require.NoError(t, err, "failed to update app with tool")

	// 12. Create a session to test with
	session := types.Session{ // Change from pointer to value type
		ParentApp:    createdApp.ID,
		Owner:        testUser.ID,
		OwnerType:    types.OwnerTypeUser,
		Mode:         types.SessionModeInference,
		Type:         types.SessionTypeText,
		ModelName:    "meta-llama/Llama-3.1-8B-Instruct",
		Interactions: types.Interactions{},
		Metadata: types.SessionMetadata{
			OriginalMode:   types.SessionModeInference,
			AppQueryParams: map[string]string{},
		},
	}

	createdSession, err := db.CreateSession(context.Background(), session)
	require.NoError(t, err, "failed to create session")

	// 13. Test using the API tool with the OAuth token
	t.Run("API tool should include OAuth token in requests", func(t *testing.T) {
		// First, ensure a clean state
		capturedAuthHeader = ""

		// Create a request to run the API action directly with the parameters
		params := map[string]string{
			"owner": "testuser",
			"repo":  "testrepo",
		}

		// Create mock OAuth tokens map (would normally be retrieved from the OAuth connection)
		oauthTokens := map[string]string{
			"GitHub": "github-test-token",
		}

		// Prepare to run the API action with RunAPIActionWithParameters
		runAPIReq := &types.RunAPIActionRequest{
			Action:      "listRepositoryIssues",
			Parameters:  params,
			Tool:        githubTool,
			OAuthTokens: oauthTokens,
		}

		// Manually run the API action directly through the API server
		resp, err := apiClient.RunAPIAction(context.Background(), createdApp.ID, runAPIReq.Action, runAPIReq.Parameters)
		require.NoError(t, err, "API request should succeed")
		require.NotNil(t, resp, "Response should not be nil")

		// Verify the Authorization header was set and contains our token
		require.Contains(t, capturedAuthHeader, "Bearer github-test-token",
			"Expected Authorization header to include OAuth token")
	})

	// 14. Clean up test data
	err = db.DeleteApp(context.Background(), createdApp.ID)
	require.NoError(t, err, "failed to delete app")

	// No need to delete tool - it's part of the app config

	createdSession, err = db.DeleteSession(context.Background(), createdSession.ID) // Fix to capture both return values
	require.NoError(t, err, "failed to delete session")

	err = db.DeleteOAuthConnection(context.Background(), githubConnection.ID)
	require.NoError(t, err, "failed to delete OAuth connection")

	err = db.DeleteOAuthProvider(context.Background(), createdProvider.ID)
	require.NoError(t, err, "failed to delete OAuth provider")

	err = db.DeleteUser(context.Background(), testUser.ID)
	require.NoError(t, err, "failed to delete user")

	// Skip Keycloak cleanup if we didn't use it
	if keycloakEnabled {
		t.Log("Would clean up Keycloak user here if needed")
		// If there's a specific Keycloak cleanup needed
	}
}

// Helper function to create an API key for a user
func createAPIKey(t *testing.T, db *store.PostgresStore, userID string) (string, error) {
	t.Helper()

	// Generate a random API key
	const keyLength = 32
	const keyChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	apiKey := ""
	for i := 0; i < keyLength; i++ {
		apiKey += string(keyChars[time.Now().UnixNano()%int64(len(keyChars))])
		time.Sleep(1 * time.Nanosecond) // Ensure uniqueness
	}

	// Create the API key in the database
	_, err := db.CreateAPIKey(context.Background(), &types.ApiKey{
		Name:      "integration-test-key",
		Key:       apiKey,
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	})

	return apiKey, err
}
