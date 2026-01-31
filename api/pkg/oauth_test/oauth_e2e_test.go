package oauth

// This test is in a separate package to avoid an import loop.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	oai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Global counter for test schema uniqueness
var testCounter int64

// This test demonstrates the issue with app ID not being properly propagated
// through the context chain in streaming sessions. It creates a full end-to-end
// test that follows the production code path for streaming sessions and OAuth token usage.
//
// This test uses a unique database schema to avoid migration conflicts with concurrent tests.
func TestOAuthAppIDPropagationProduction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// 1. Set up the test environment with a mock GitHub API server to capture auth headers
	var capturedAuthHeader string
	var capturedPath string
	var requestCounter int

	mockGithubServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCounter++
		capturedAuthHeader = r.Header.Get("Authorization")
		capturedPath = r.URL.Path
		t.Logf("MOCK SERVER: Received request %d to: %s with auth: %s",
			requestCounter, capturedPath, capturedAuthHeader)

		// Log request details for debugging
		t.Logf("MOCK SERVER: Request details: %s %s", r.Method, r.URL.String())
		for key, values := range r.Header {
			t.Logf("MOCK SERVER: Header %s: %v", key, values)
		}

		// For proper testing, handle auth vs no auth differently
		if capturedAuthHeader == "" || !strings.Contains(capturedAuthHeader, "Bearer github-test-token") {
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte(`{"message":"Bad credentials","documentation_url":"https://docs.github.com/rest"}`))
			if err != nil {
				t.Logf("MOCK SERVER: Error writing response: %v", err)
			}
			t.Logf("MOCK SERVER: Returned 401 - Bad credentials (no valid token)")
			return
		}

		// Return appropriate response for a successful call
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`[{"id":1,"title":"Test Issue 1"},{"id":2,"title":"Test Issue 2"}]`))
		if err != nil {
			t.Logf("MOCK SERVER: Error writing response: %v", err)
		}
		t.Logf("MOCK SERVER: Returned 200 OK with issue data")
	}))
	defer mockGithubServer.Close()
	t.Logf("Mock GitHub server started at %s", mockGithubServer.URL)

	// Take a note of the mock server URL to make it more visible in logs
	githubAPIURL := mockGithubServer.URL
	t.Logf("******************************************")
	t.Logf("MOCK GITHUB SERVER URL: %s", githubAPIURL)
	t.Logf("******************************************")

	// 2. Load server config and set up database connection
	cfg, err := config.LoadServerConfig()
	require.NoError(t, err, "Failed to load server config")

	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)

	// Use a unique schema for this test to avoid migration conflicts
	testSchema := fmt.Sprintf("oauth_test_%d_%d", time.Now().UnixNano(), atomic.AddInt64(&testCounter, 1))

	// Modify the store config to use the unique schema
	testStoreCfg := cfg.Store
	testStoreCfg.Schema = testSchema

	// Use database configuration from environment variables (provided by the test harness)
	db, err := store.NewPostgresStore(testStoreCfg, ps)
	require.NoError(t, err, "Failed to create store connection")
	defer db.Close()

	t.Logf("Using test schema: %s", testSchema)

	// 3. Create test user
	testUserID := fmt.Sprintf("user_%d", time.Now().UnixNano())
	testUser := &types.User{
		ID:       testUserID,
		Username: "testuser",
		Email:    fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()),
		FullName: "Test User",
	}
	testUser, err = db.CreateUser(context.Background(), testUser)
	require.NoError(t, err, "Failed to create test user")
	defer func() {
		_ = db.DeleteUser(context.Background(), testUser.ID)
	}()

	// 4. Create a GitHub OAuth provider
	providerName := fmt.Sprintf("GitHub-Test-%d", time.Now().UnixNano())
	githubProvider := &types.OAuthProvider{
		ID:           uuid.NewString(),
		Name:         providerName,
		Type:         types.OAuthProviderTypeGitHub,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      mockGithubServer.URL + "/login/oauth/authorize",
		TokenURL:     mockGithubServer.URL + "/login/oauth/access_token",
		UserInfoURL:  mockGithubServer.URL + "/user",
		Enabled:      true,
	}
	createdProvider, err := db.CreateOAuthProvider(context.Background(), githubProvider)
	require.NoError(t, err, "Failed to create OAuth provider")
	defer func() {
		_ = db.DeleteOAuthProvider(context.Background(), createdProvider.ID)
	}()
	t.Logf("Created OAuth provider: ID=%s, Name=%s", createdProvider.ID, createdProvider.Name)

	// 5. Create an OAuth connection for the user with valid token
	githubConnection := &types.OAuthConnection{
		ID:           uuid.NewString(),
		UserID:       testUser.ID,
		ProviderID:   createdProvider.ID,
		AccessToken:  "github-test-token",
		RefreshToken: "test-refresh-token",
		ExpiresAt:    time.Now().Add(24 * time.Hour), // Valid for 24 hours
		Scopes:       []string{"repo", "read:user"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	createdConnection, err := db.CreateOAuthConnection(context.Background(), githubConnection)
	require.NoError(t, err, "Failed to create OAuth connection")
	defer func() {
		_ = db.DeleteOAuthConnection(context.Background(), createdConnection.ID)
	}()
	t.Logf("Created OAuth connection with ID: %s", createdConnection.ID)

	// 6. Create a GitHub API tool with our mock server URL
	githubToolSchema := `openapi: 3.0.0
info:
  title: GitHub API
  description: Access GitHub API
  version: "1.0"
paths:
  /issues:
    get:
      summary: List issues assigned to the authenticated user
      description: List all issues assigned to the authenticated user
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

	githubTool := types.AssistantAPI{
		Name:          "GitHub API",
		Description:   "Access GitHub issues and repositories",
		Schema:        githubToolSchema,
		URL:           githubAPIURL,
		Headers:       map[string]string{},
		OAuthProvider: providerName,
		OAuthScopes:   []string{"repo"},
	}

	// 7. Create an app with an assistant that includes the GitHub tool
	appConfig := types.AppConfig{
		Helix: types.AppHelixConfig{
			Name:        "OAuth Test App",
			Description: "Test app for OAuth integration testing",
			Assistants: []types.AssistantConfig{
				{
					ID:       "test-assistant",
					Name:     "Test Assistant",
					Provider: "togetherai",
					Model:    "meta-llama/Llama-3.3-70B-Instruct-Turbo",
					APIs:     []types.AssistantAPI{githubTool},
				},
			},
		},
	}

	app := &types.App{
		ID:        fmt.Sprintf("app_%d", time.Now().UnixNano()),
		Owner:     testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Config:    appConfig,
	}

	createdApp, err := db.CreateApp(context.Background(), app)
	require.NoError(t, err, "Failed to create app")
	defer func() {
		_ = db.DeleteApp(context.Background(), createdApp.ID)
	}()
	t.Logf("Created app with ID: %s", createdApp.ID)

	// 8. Verify the app was properly created with tools
	retrievedApp, err := db.GetApp(context.Background(), createdApp.ID)
	require.NoError(t, err, "Failed to retrieve app")
	require.Equal(t, 1, len(retrievedApp.Config.Helix.Assistants), "App should have 1 assistant")
	require.Equal(t, "test-assistant", retrievedApp.Config.Helix.Assistants[0].ID, "Assistant ID should match")
	require.Equal(t, 1, len(retrievedApp.Config.Helix.Assistants[0].APIs), "Assistant should have 1 API")
	require.Equal(t, githubTool.URL, retrievedApp.Config.Helix.Assistants[0].APIs[0].URL, "API URL should match")
	t.Logf("Assistant tools verified. API URL: %s, Provider: %s",
		githubTool.URL, githubTool.OAuthProvider)

	// 9. Set up the controller components for streaming
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Create a mock OpenAI client that returns a tool execution request
	mockClient := openai.NewMockClient(mockCtrl)

	modelInfoProvider, err := model.NewBaseModelInfoProvider()
	require.NoError(t, err)

	// Set up a real provider manager with the mock OpenAI client for Helix
	providerManager := manager.NewProviderManager(&cfg, db, mockClient, modelInfoProvider)

	// Create a mock RunnerControllerStatus to avoid nil pointer dereference
	mockRunnerController := manager.NewMockRunnerControllerStatus(mockCtrl)
	mockRunnerController.EXPECT().RunnerIDs().Return([]string{"mock-runner-id"}).AnyTimes()

	// Set the mock runner controller on the provider manager
	providerManager.SetRunnerController(mockRunnerController)

	// Set API key expectations for client initialization
	mockClient.EXPECT().APIKey().Return("test-key").AnyTimes()

	// Create OAuth manager
	oauthManager := oauth.NewManager(db, cfg.Tools.TLSSkipVerify)
	err = oauthManager.LoadProviders(context.Background())
	require.NoError(t, err, "Failed to load OAuth providers")

	// Set up the rest of the controller
	toolsPlanner, err := tools.NewChainStrategy(&cfg, db, mockClient)
	require.NoError(t, err, "Failed to create chain strategy")
	tools.InitChainStrategyOAuth(toolsPlanner, oauthManager, db, db)

	// Create mock dependencies for controller
	mockFilestore := filestore.NewMockFileStore(mockCtrl)
	mockExtractor := extract.NewMockExtractor(mockCtrl)
	testJanitor := janitor.NewJanitor(config.Janitor{})

	// Set up some expected calls that might be needed
	mockFilestore.EXPECT().SignedURL(gomock.Any(), gomock.Any()).Return("http://localhost:8080/files/test.txt", nil).AnyTimes()
	mockFilestore.EXPECT().List(gomock.Any(), gomock.Any()).Return([]filestore.Item{}, nil).AnyTimes()
	mockFilestore.EXPECT().Get(gomock.Any(), gomock.Any()).Return(filestore.Item{}, nil).AnyTimes()
	mockExtractor.EXPECT().Extract(gomock.Any(), gomock.Any()).Return("test content", nil).AnyTimes()

	// Create an in-memory PubSub for testing
	helixPubSub, err := pubsub.NewInMemoryNats()
	require.NoError(t, err, "Failed to create in-memory PubSub")
	defer helixPubSub.Close()

	// Create a controller with context
	ctrl, err := controller.NewController(context.Background(), controller.Options{
		Config:          &cfg,
		Store:           db,
		OAuthManager:    oauthManager,
		ProviderManager: providerManager,
		Filestore:       mockFilestore,
		Extractor:       mockExtractor,
		Janitor:         testJanitor,
		PubSub:          helixPubSub,
		Scheduler:       nil, // No scheduler needed for direct API calls
	})
	require.NoError(t, err, "Failed to create controller")
	defer func() {
		if err := ctrl.Close(); err != nil {
			t.Logf("Failed to close controller: %v", err)
		}
	}()
	ctrl.ToolsPlanner = toolsPlanner

	t.Logf("****************************************************")
	t.Logf("APP ID: %s", createdApp.ID)
	t.Logf("OWNER ID: %s", testUser.ID)
	t.Logf("TOOL ID: %s", githubTool.Name)
	t.Logf("GITHUB API URL: %s", githubAPIURL)
	t.Logf("****************************************************")

	// We need to put this in its own subtest so we can properly observe whether it fails
	t.Run("Sessions API should use OAuth token", func(t *testing.T) {
		// Save current auth header state before running test
		originalRequestCount := requestCounter

		// Create a simple chat completion request without any OpenAI tool specifications
		chatCompletionRequest := oai.ChatCompletionRequest{
			Model:  "meta-llama/Llama-3.3-70B-Instruct-Turbo",
			Stream: true,
			Messages: []oai.ChatCompletionMessage{
				{
					Role:    oai.ChatMessageRoleSystem,
					Content: "You are a helpful assistant that can use GitHub to look up repository issues.",
				},
				{
					Role:    oai.ChatMessageRoleUser,
					Content: "What issues do I have on GitHub?",
				},
			},
			// No tool specifications - we're using our internal API tools implementation
		}

		// Setup our context with any needed values
		ctx := context.Background()

		// Setup chat completion options with app ID
		opts := &controller.ChatCompletionOptions{
			AppID:       createdApp.ID, // This is critical - setting the app ID explicitly
			Provider:    "togetherai",
			OAuthTokens: map[string]string{}, // Will be populated internally by evalAndAddOAuthTokens
		}

		t.Logf("Making direct chat completion stream call with app ID: %s, user ID: %s", createdApp.ID, testUser.ID)

		// Add explicit logging about the OAuth connection
		connections, err := db.ListOAuthConnections(context.Background(), &store.ListOAuthConnectionsQuery{
			UserID: testUser.ID,
		})
		require.NoError(t, err, "Failed to list OAuth connections")
		t.Logf("Found %d OAuth connections for user %s", len(connections), testUser.ID)
		for i, conn := range connections {
			t.Logf("Connection %d: Provider=%s, ID=%s, Token=%s (prefix), Expires=%v",
				i, conn.ProviderID, conn.ID, conn.AccessToken[:5], conn.ExpiresAt)
		}

		// Log enabled OAuth providers
		provider, err := db.GetOAuthProvider(context.Background(), createdProvider.ID)
		require.NoError(t, err, "Failed to get OAuth provider")
		t.Logf("OAuth Provider %s (%s) enabled: %v", provider.Name, provider.ID, provider.Enabled)

		// Add additional context for debugging the AppID in the request
		t.Logf("APP ID RIGHT BEFORE STREAM: %s", opts.AppID)

		// Make the direct streaming chat completion call through the real controller
		stream, _, err := ctrl.ChatCompletionStream(ctx, testUser, chatCompletionRequest, opts)
		require.NoError(t, err, "Failed to create chat completion stream")
		defer stream.Close()

		// Process the stream just to ensure completion
		// We don't care about the response content - just that the stream completes
		fmt.Println("\n🔍 LLM RESPONSE START 🔍")
		var fullResponse string
		for {
			response, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					fmt.Println("\n🔍 LLM RESPONSE END (EOF) 🔍")
					break
				}
				fmt.Printf("\n🔍 Stream error: %v 🔍\n", err)
				break
			}

			// Print the actual content from the response (if any)
			if len(response.Choices) > 0 && response.Choices[0].Delta.Content != "" {
				content := response.Choices[0].Delta.Content
				fullResponse += content
				fmt.Print(content)
			}
		}

		// Print the full response at the end for clarity
		fmt.Printf("\n\n💬 FULL RESPONSE: %s\n", fullResponse)

		// Wait a bit to ensure any asynchronous calls complete
		t.Log("Waiting for any API calls to complete...")
		time.Sleep(1 * time.Second)

		// Verify that our mock server received a request
		t.Logf("Request counter: %d (original: %d)", requestCounter, originalRequestCount)
		require.Greater(t, requestCounter, originalRequestCount,
			"The mock GitHub server should have received at least one request")

		// Verify that the GitHub API was called with the OAuth token
		// This is the critical assertion that our test is checking
		t.Logf("Captured auth header: %s", capturedAuthHeader)
		require.Contains(t, capturedAuthHeader, "Bearer github-test-token",
			"API request should include the OAuth Bearer token")

		// If we've gotten here, the test has passed!
		t.Log("Success! OAuth token was properly included in the API request")
	})
}
