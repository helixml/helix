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

	db, err := store.NewPostgresStore(cfg.Store)
	require.NoError(t, err, "failed to create store")
	defer db.Close()

	// 3. Set up a test Keycloak authenticator
	kc, err := auth.NewKeycloakAuthenticator(cfg.Auth.Keycloak)
	require.NoError(t, err, "failed to create Keycloak authenticator")

	// 4. Create a test user
	email := fmt.Sprintf("oauth-test-%d@example.com", time.Now().Unix())
	testUser := &types.User{
		Email:    email,
		Username: email,
		FullName: "OAuth Test User",
	}

	// Create the user in Keycloak
	createdUser, err := kc.CreateKeycloakUser(context.Background(), testUser)
	require.NoError(t, err, "failed to create user in Keycloak")

	// Set the ID from Keycloak
	testUser.ID = createdUser.ID

	// Create the user in our database
	testUser, err = db.CreateUser(context.Background(), testUser)
	require.NoError(t, err, "failed to create user in database")

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
	apiClient, err := client.NewClient(fmt.Sprintf("http://localhost:%d", cfg.Server.Port), apiKey)
	require.NoError(t, err, "failed to create API client")

	// 9. Create an app with a GitHub API tool
	app := &types.AppConfig{
		Name:        "OAuth Test App",
		Description: "Test app for OAuth integration testing",
		Model:       "meta-llama/Llama-3.1-8B-Instruct",
	}

	createdApp, err := apiClient.CreateApp(context.Background(), app)
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
				OAuthProvider: "GitHub",         // Must match the provider name exactly
				OAuthScopes:   []string{"repo"}, // Required scopes
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

	createdTool, err := apiClient.CreateTool(context.Background(), githubTool)
	require.NoError(t, err, "failed to create GitHub API tool")

	// 11. Add the tool to the app
	_, err = apiClient.AddToolToApp(context.Background(), createdApp.ID, createdTool.ID)
	require.NoError(t, err, "failed to add tool to app")

	// 12. Create a session to test with
	session, err := apiClient.CreateSession(context.Background(), &types.SessionConfig{
		AppID: createdApp.ID,
	})
	require.NoError(t, err, "failed to create session")

	// 13. Test using the API tool with the OAuth token
	t.Run("API tool should include OAuth token in requests", func(t *testing.T) {
		// First, ensure a clean state
		capturedAuthHeader = ""

		// Send a message that will trigger the GitHub API tool to list issues
		response, err := apiClient.SendMessage(context.Background(), session.ID, &types.SendMessageRequest{
			Message: "What issues are in the testuser/testrepo repository?",
		})
		require.NoError(t, err, "failed to send message")

		// Wait for processing to complete - in a real test we would use proper waiting mechanisms
		time.Sleep(5 * time.Second)

		// Verify the response status
		require.NotNil(t, response, "response should not be nil")

		// Verify the Authorization header was set and contains our token
		require.Contains(t, capturedAuthHeader, "Bearer github-test-token",
			"Expected Authorization header to include OAuth token")
	})

	// 14. Clean up test data
	err = db.DeleteApp(context.Background(), createdApp.ID)
	require.NoError(t, err, "failed to delete app")

	err = db.DeleteTool(context.Background(), createdTool.ID)
	require.NoError(t, err, "failed to delete tool")

	err = db.DeleteOAuthConnection(context.Background(), githubConnection.ID)
	require.NoError(t, err, "failed to delete OAuth connection")

	err = db.DeleteOAuthProvider(context.Background(), createdProvider.ID)
	require.NoError(t, err, "failed to delete OAuth provider")

	err = db.DeleteUser(context.Background(), testUser.ID)
	require.NoError(t, err, "failed to delete user")
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
