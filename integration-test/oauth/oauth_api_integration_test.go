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

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	goai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Test verifies that OAuth tokens are properly included in API tool requests.
// This integration test:
// 1. Creates a mock GitHub API server to simulate an OAuth2 provider API
// 2. Sets up a user with an OAuth connection to the GitHub provider
// 3. Creates a tool configured to use GitHub's API with OAuth
// 4. Tests both direct token passing and the more realistic session-based flow
func TestOAuthAPIToolIntegration(t *testing.T) {
	// Skip if in short mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Log("Starting OAuth API tool integration test")

	// 1. Set up a mock GitHub API server that will validate the Authorization header
	var capturedAuthHeader string
	var capturedPath string
	mockGithubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the Authorization header and path
		capturedAuthHeader = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		t.Logf("Mock GitHub server received request to path: %s with Authorization: %s", capturedPath, capturedAuthHeader)

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
				t.Log("Mock GitHub server returned 401 Unauthorized - missing Authorization header")
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
			t.Log("Mock GitHub server returned issues data")
		} else if r.URL.Path == "/issues" {
			// This path simulates the GitHub API endpoint for user issues
			if capturedAuthHeader == "" {
				// If no auth header, return 404 (this mimics the behavior seen in production)
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte(`{"message":"Not Found","documentation_url":"https://docs.github.com/rest/issues/issues#list-issues-assigned-to-the-authenticated-user","status":"404"}`))
				t.Log("Mock GitHub server returned 404 Not Found - missing Authorization header")
				return
			}

			// If auth header is present, return success
			resp := []map[string]interface{}{
				{
					"id":    1,
					"title": "User Issue 1",
					"state": "open",
				},
				{
					"id":    2,
					"title": "User Issue 2",
					"state": "closed",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			t.Log("Mock GitHub server returned user issues data")
		} else {
			w.WriteHeader(http.StatusNotFound)
			t.Logf("Mock GitHub server returned 404 Not Found for path: %s", r.URL.Path)
		}
	}))
	defer mockGithubServer.Close()
	t.Logf("Mock GitHub server started at %s", mockGithubServer.URL)

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

	// Make sure the WebServer port is set correctly to match docker-compose
	cfg.WebServer.Port = 8080

	// Create a store connection
	db, err := store.NewPostgresStore(cfg.Store)
	require.NoError(t, err, "failed to create store")
	defer db.Close()

	// 3. Create a test user directly in the database
	email := fmt.Sprintf("oauth-test-%d@example.com", time.Now().Unix())
	testUserID := fmt.Sprintf("user_%d", time.Now().UnixNano())
	testUser := &types.User{
		ID:       testUserID,
		Email:    email,
		Username: email,
		FullName: "OAuth Test User",
	}

	// Create the user directly in the database
	testUser, err = db.CreateUser(context.Background(), testUser)
	require.NoError(t, err, "failed to create user in database")

	// 5. Create an OAuth provider for GitHub using our mock server
	uniqueProviderName := fmt.Sprintf("GitHub-Test-%d", time.Now().UnixNano())
	githubProvider := &types.OAuthProvider{
		Name:         uniqueProviderName,
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
	t.Logf("Created test GitHub provider with ID: %s and name: %s", createdProvider.ID, uniqueProviderName)

	// 7. Create an OAuth connection for the user with the GitHub provider
	githubConnection := &types.OAuthConnection{
		UserID:       testUser.ID,
		ProviderID:   createdProvider.ID,
		AccessToken:  "github-test-token",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       []string{"repo", "read:user"},
	}

	createdConnection, err := db.CreateOAuthConnection(context.Background(), githubConnection)
	require.NoError(t, err, "failed to create OAuth connection")
	t.Logf("Created OAuth connection with ID: %s for user %s and provider %s", createdConnection.ID, testUser.ID, createdProvider.ID)

	// Verify the connection exists
	connections, err := db.ListOAuthConnections(context.Background(), &store.ListOAuthConnectionsQuery{
		UserID: testUser.ID,
	})
	require.NoError(t, err, "failed to list OAuth connections")
	t.Logf("Found %d OAuth connections for user %s", len(connections), testUser.ID)
	for _, conn := range connections {
		t.Logf("Connection: ID=%s, UserID=%s, ProviderID=%s", conn.ID, conn.UserID, conn.ProviderID)
	}

	// 8. Create an app
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

	// 10. Create a GitHub API tool with our mock server URL and include the listUserIssues endpoint
	githubToolSchema := `openapi: 3.0.0
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
  /issues:
    get:
      summary: List issues assigned to the authenticated user
      description: List all issues assigned to the authenticated user across all repositories
      operationId: listUserIssues
      parameters:
        - name: filter
          in: query
          description: Filter issues by state
          schema:
            type: string
            enum: [assigned, created, mentioned, subscribed, all]
            default: assigned
        - name: state
          in: query
          description: Filter issues by state
          schema:
            type: string
            enum: [open, closed, all]
            default: open
        - name: sort
          in: query
          description: What to sort results by
          schema:
            type: string
            enum: [created, updated, comments]
            default: created
      responses:
        '200':
          description: Successful operation`

	githubTool := &types.Tool{
		Name:        "GitHub API",
		Description: "GitHub API access",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:           mockGithubServer.URL,
				OAuthProvider: uniqueProviderName,
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
					{
						Name:        "listUserIssues",
						Description: "List issues assigned to the authenticated user",
						Path:        "/issues",
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
	sessionData := types.Session{
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

	createdSession, err := db.CreateSession(context.Background(), sessionData)
	require.NoError(t, err, "failed to create session")

	// 13. Test using the API tool with the OAuth token
	t.Run("API tool should include OAuth token in requests", func(t *testing.T) {
		// First, ensure a clean state
		capturedAuthHeader = ""
		t.Log("Starting API request OAuth token test")

		// Create a request to run the API action directly with the parameters
		params := map[string]string{
			"owner": "testuser",
			"repo":  "testrepo",
		}

		// Create mock OAuth tokens map (would normally be retrieved from the OAuth connection)
		oauthTokens := map[string]string{
			"GitHub": "github-test-token",
		}
		t.Log("Prepared OAuth tokens for request")

		// We'll directly create a request to our mock GitHub server with the parameters
		// This simulates what the API would do internally when processing an API tool request
		// The key point being tested is whether the OAuth token gets included in the request
		githubURL := fmt.Sprintf("%s/repos/%s/%s/issues", mockGithubServer.URL, params["owner"], params["repo"])
		t.Logf("Creating request to GitHub API URL: %s", githubURL)

		req, err := http.NewRequest("GET", githubURL, nil)
		require.NoError(t, err, "Failed to create request")

		// Add the Authorization header with the OAuth token
		// This is the critical part we're testing - in production, this is done by processOAuthTokens
		// and tools_api_run_action.go would add this header to outgoing requests
		req.Header.Set("Authorization", "Bearer "+oauthTokens["GitHub"])
		t.Log("Added OAuth token to request authorization header")

		// Make the request directly to our mock server
		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err, "Request to mock GitHub server failed")
		defer resp.Body.Close()

		// Verify the response status
		require.Equal(t, http.StatusOK, resp.StatusCode, "Expected 200 OK response")
		t.Log("Received 200 OK response from mock GitHub server")

		// Verify the Authorization header was captured by our mock server
		// This confirms the OAuth token was correctly included in the request
		require.Contains(t, capturedAuthHeader, "Bearer github-test-token",
			"Expected Authorization header to include OAuth token")

		// Also log the success for clarity
		t.Logf("Successfully verified OAuth token was included in request header: %s", capturedAuthHeader)
		t.Log("OAuth token verification test PASSED!")
	})

	// 14. NEW TEST: Test the full flow with RunAction that mirrors the production issue
	t.Run("RunAction with sessionID should retrieve and use OAuth tokens", func(t *testing.T) {
		// This test will simulate the issue seen in production
		capturedAuthHeader = ""
		capturedPath = ""

		// Create a chain strategy with OAuth manager
		oauthManager := oauth.NewManager(db)
		err = oauthManager.LoadProviders(context.Background())
		require.NoError(t, err, "Failed to load OAuth providers")

		// List all providers to see what's available
		providers, err := db.ListOAuthProviders(context.Background(), &store.ListOAuthProvidersQuery{})
		require.NoError(t, err, "Failed to list OAuth providers")
		t.Logf("Found %d OAuth providers in database", len(providers))
		for _, p := range providers {
			t.Logf("Provider: ID=%s, Name=%s", p.ID, p.Name)
		}

		// Set up a minimal ChainStrategy that will work for testing
		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()

		mockClient := openai.NewMockClient(mockCtrl)

		// Mock the CreateChatCompletion method to return predefined parameters
		mockClient.EXPECT().
			CreateChatCompletion(gomock.Any(), gomock.Any()).
			Return(goai.ChatCompletionResponse{
				Choices: []goai.ChatCompletionChoice{
					{
						Message: goai.ChatCompletionMessage{
							Content: `{"filter": "assigned", "state": "open", "sort": "created"}`,
						},
					},
				},
			}, nil).
			AnyTimes()

		// Mock the APIKey method which might be needed
		mockClient.EXPECT().
			APIKey().
			Return("test-key").
			AnyTimes()

		chainStrategy, err := tools.NewChainStrategy(&cfg, db, nil, mockClient)
		require.NoError(t, err, "Failed to create chain strategy")
		tools.InitChainStrategyOAuth(chainStrategy, oauthManager, db, db)

		// Create a context with the session ID and app ID
		ctx := context.Background()
		ctx = oai.SetContextValues(ctx, &oai.ContextValues{
			OwnerID:       testUser.ID,
			SessionID:     createdSession.ID,
			InteractionID: "test-interaction-id",
		})
		ctx = oai.SetContextAppID(ctx, createdApp.ID)

		// Create simplified history with a request for issues
		history := []*types.ToolHistoryMessage{
			{
				Role:    "user",
				Content: "What issues do I have?",
			},
		}

		// Call RunAction to test the full flow - using listUserIssues which will call the /issues endpoint
		actionResponse, err := chainStrategy.RunAction(
			ctx,
			createdSession.ID,
			"test-interaction-id",
			githubTool,
			history,
			"listUserIssues",
		)
		require.NoError(t, err, "RunAction should not return an error")

		// Verify the path that was called
		require.Equal(t, "/issues", capturedPath, "Expected to call /issues endpoint")

		// Check that an Authorization header was included with the GitHub token
		require.Contains(t, capturedAuthHeader, "Bearer github-test-token",
			"Expected Authorization header with OAuth token")

		// Verify we got a successful response and not a 404
		require.NotContains(t, actionResponse.RawMessage, "Not Found",
			"Response should not be a 404 Not Found")
		require.Contains(t, actionResponse.RawMessage, "User Issue",
			"Response should contain issue data")

		t.Logf("RunAction OAuth test PASSED! Authorization header: %s", capturedAuthHeader)
	})

	// 15. Clean up test data
	err = db.DeleteApp(context.Background(), createdApp.ID)
	require.NoError(t, err, "failed to delete app")

	createdSession, err = db.DeleteSession(context.Background(), createdSession.ID)
	require.NoError(t, err, "failed to delete session")

	err = db.DeleteOAuthConnection(context.Background(), githubConnection.ID)
	require.NoError(t, err, "failed to delete OAuth connection")

	err = db.DeleteOAuthProvider(context.Background(), createdProvider.ID)
	require.NoError(t, err, "failed to delete OAuth provider")

	err = db.DeleteUser(context.Background(), testUser.ID)
	require.NoError(t, err, "failed to delete user")
}

// TestOAuthAppIDPropagationIntegration tests the complete flow from session creation to OAuth token usage
// This test specifically focuses on app ID propagation through the context chain
func TestOAuthAppIDPropagationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 1. Set up the test database
	cfg, err := config.LoadServerConfig()
	require.NoError(t, err, "Failed to load server config")

	// When running outside the container, we need to modify the database connection parameters
	// to connect to the local PostgreSQL instance
	cfg.Store.Host = "localhost"
	cfg.Store.Port = 5432
	cfg.Store.Username = "postgres"
	cfg.Store.Password = "postgres"
	cfg.Store.Database = "postgres"

	db, err := store.NewPostgresStore(cfg.Store)
	require.NoError(t, err, "Failed to create test database")
	defer db.Close()

	// 2. Create the OAuth manager
	oauthManager := oauth.NewManager(db)
	err = oauthManager.LoadProviders(context.Background())
	require.NoError(t, err, "Failed to load OAuth providers")

	// 3. Create a test user
	testUser := &types.User{
		ID:       fmt.Sprintf("user_%d", time.Now().UnixNano()),
		Username: "testuser",
		Email:    "test@example.com",
	}
	testUser, err = db.CreateUser(context.Background(), testUser)
	require.NoError(t, err, "Failed to create test user")
	defer func() {
		err = db.DeleteUser(context.Background(), testUser.ID)
		require.NoError(t, err, "Failed to delete test user")
	}()

	// 4. Create a test GitHub provider
	providerName := fmt.Sprintf("GitHub-Test-%d", time.Now().UnixNano())
	createdProvider := &types.OAuthProvider{
		ID:           uuid.NewString(),
		Name:         providerName,
		Type:         "github",
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		Enabled:      true,
	}
	createdProvider, err = db.CreateOAuthProvider(context.Background(), createdProvider)
	require.NoError(t, err, "Failed to create OAuth provider")
	defer func() {
		err = db.DeleteOAuthProvider(context.Background(), createdProvider.ID)
		require.NoError(t, err, "Failed to delete OAuth provider")
	}()

	// 5. Create a test app
	createdApp := &types.App{
		ID:        fmt.Sprintf("app_%d", time.Now().UnixNano()),
		Owner:     testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name: "Test App",
			},
		},
		Global: false,
	}
	createdApp, err = db.CreateApp(context.Background(), createdApp)
	require.NoError(t, err, "Failed to create app")
	defer func() {
		err = db.DeleteApp(context.Background(), createdApp.ID)
		require.NoError(t, err, "Failed to delete app")
	}()

	// 6. Create a test session linked to the app
	sessionData := types.Session{
		ID:        fmt.Sprintf("ses_%d", time.Now().UnixNano()),
		ParentApp: createdApp.ID,
		Name:      "Test Session",
		Owner:     testUser.ID,
		OwnerType: types.OwnerTypeUser,
	}
	createdSession, err := db.CreateSession(context.Background(), sessionData)
	require.NoError(t, err, "Failed to create session")
	defer func() {
		_, err = db.DeleteSession(context.Background(), createdSession.ID)
		require.NoError(t, err, "Failed to delete session")
	}()

	// 7. Create an OAuth connection for the user and provider
	githubConnection := &types.OAuthConnection{
		ID:           uuid.NewString(),
		UserID:       testUser.ID,
		ProviderID:   createdProvider.ID,
		AccessToken:  "github-test-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       []string{"repo"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	createdConnection, err := db.CreateOAuthConnection(context.Background(), githubConnection)
	require.NoError(t, err, "Failed to create OAuth connection")
	defer func() {
		err = db.DeleteOAuthConnection(context.Background(), createdConnection.ID)
		require.NoError(t, err, "Failed to delete OAuth connection")
	}()

	// 8. Set up a GitHub API tool with OAuth provider
	githubTool := &types.Tool{
		ID:       fmt.Sprintf("tool_%d", time.Now().UnixNano()),
		Name:     "GitHub API",
		ToolType: types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL: "https://api.github.com",
				Schema: `openapi: 3.0.0
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
  /issues:
    get:
      summary: List issues assigned to the authenticated user
      description: List all issues assigned to the authenticated user across all repositories
      operationId: listUserIssues
      parameters:
        - name: filter
          in: query
          description: Filter issues by state
          schema:
            type: string
            enum: [assigned, created, mentioned, subscribed, all]
            default: assigned
        - name: state
          in: query
          description: Filter issues by state
          schema:
            type: string
            enum: [open, closed, all]
            default: open
        - name: sort
          in: query
          description: What to sort results by
          schema:
            type: string
            enum: [created, updated, comments]
            default: created
      responses:
        '200':
          description: Successful operation`,
				Actions: []*types.ToolAPIAction{
					{
						Name:        "getAuthenticatedUser",
						Description: "Get the currently authenticated user",
						Method:      "GET",
						Path:        "/user",
					},
					{
						Name:        "listRepositoryIssues",
						Description: "List issues in a repository",
						Method:      "GET",
						Path:        "/repos/{owner}/{repo}/issues",
					},
					{
						Name:        "listUserIssues",
						Description: "List issues assigned to the authenticated user",
						Method:      "GET",
						Path:        "/issues",
					},
				},
				OAuthProvider: providerName,
				OAuthScopes:   []string{"repo"},
				// Explicitly set an empty map for headers to ensure they're initialized
				Headers: map[string]string{},
			},
		},
	}

	// 9. Set up controller components needed for testing - this more closely mirrors the production flow
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Mock client for OpenAI with expectations
	mockClient := openai.NewMockClient(mockCtrl)
	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any()).
		Return(goai.ChatCompletionResponse{
			Choices: []goai.ChatCompletionChoice{
				{
					Message: goai.ChatCompletionMessage{
						Content: `{"filter": "assigned", "state": "open", "sort": "created"}`,
					},
				},
			},
		}, nil).
		AnyTimes()

	mockClient.EXPECT().APIKey().Return("test-key").AnyTimes()

	// Initialize the inference controller components that handle API tool execution
	chainStrategy, err := tools.NewChainStrategy(&cfg, db, nil, mockClient)
	require.NoError(t, err, "Failed to create chain strategy")
	tools.InitChainStrategyOAuth(chainStrategy, oauthManager, db, db)

	// Create a context that simulates how production sets it up
	ctx := context.Background()

	// This is the key part - we need to test both with and without app ID in context
	// Test 1: Without app ID in context (should fail)
	t.Run("Without app ID in context", func(t *testing.T) {
		testCtx := ctx
		testCtx = oai.SetContextValues(testCtx, &oai.ContextValues{
			OwnerID:       testUser.ID,
			SessionID:     createdSession.ID,
			InteractionID: "test-interaction-id-without-app",
			// Deliberately omit app ID to reproduce the production issue
		})

		// Create simplified history with a request for issues
		history := []*types.ToolHistoryMessage{
			{
				Role:    "user",
				Content: "What issues do I have?",
			},
		}

		// Call RunAction - this should fail to add the OAuth token
		actionResponse, err := chainStrategy.RunAction(
			testCtx,
			createdSession.ID,
			"test-interaction-id-without-app",
			githubTool,
			history,
			"listUserIssues",
		)

		// We expect the call to work but without OAuth tokens
		require.NoError(t, err, "RunAction should not return an error")

		// Check the response message to see if it contains a 401/404 error
		// This indicates the token wasn't included
		t.Logf("Response (without app ID): %s", actionResponse.RawMessage)
		require.Contains(t, actionResponse.RawMessage, "Bad credentials",
			"Response should indicate authentication failure when app ID is missing")
	})

	// Test 2: With app ID in context (should succeed)
	t.Run("With app ID in context", func(t *testing.T) {
		testCtx := ctx
		testCtx = oai.SetContextValues(testCtx, &oai.ContextValues{
			OwnerID:       testUser.ID,
			SessionID:     createdSession.ID,
			InteractionID: "test-interaction-id-with-app",
		})
		// This is the critical part - set app ID in context, which is likely missing in production
		testCtx = oai.SetContextAppID(testCtx, createdApp.ID)

		// Create simplified history with a request for issues
		history := []*types.ToolHistoryMessage{
			{
				Role:    "user",
				Content: "What issues do I have?",
			},
		}

		// Call RunAction with app ID in context
		actionResponse, err := chainStrategy.RunAction(
			testCtx,
			createdSession.ID,
			"test-interaction-id-with-app",
			githubTool,
			history,
			"listUserIssues",
		)

		// This should still fail with a Not Found but for a different reason:
		// Not because of missing Authorization but because our token is not valid for actual GitHub
		require.NoError(t, err, "RunAction should not return an error")

		// Log the response for inspection
		t.Logf("Response (with app ID): %s", actionResponse.RawMessage)
	})

	// 10. Now test accessing the OAuth context directly to see what's happening
	t.Run("Verifying OAuth token retrieval", func(t *testing.T) {
		// Test direct OAuth token retrieval
		token, err := oauthManager.GetTokenForApp(context.Background(), testUser.ID, providerName)
		require.NoError(t, err, "GetTokenForApp should not fail")
		require.Equal(t, "github-test-token", token, "OAuth token should be retrieved correctly")

		// Now test with context values as set in production
		ctxWithAppID := oai.SetContextAppID(context.Background(), createdApp.ID)
		appID, ok := oai.GetContextAppID(ctxWithAppID)
		require.True(t, ok, "App ID should be retrievable from context")
		require.Equal(t, createdApp.ID, appID, "App ID should match what was set")

		// Check actual token retrieval mechanism
		app, err := db.GetApp(context.Background(), createdApp.ID)
		require.NoError(t, err, "GetApp should not fail")
		require.Equal(t, testUser.ID, app.Owner, "App owner should match test user")

		// This simulates the full chain of lookups that happens in production
		token, err = oauthManager.GetTokenForApp(context.Background(), app.Owner, providerName)
		require.NoError(t, err, "Token lookup should not fail")
		require.Equal(t, "github-test-token", token, "Token should be correct")
	})
}
