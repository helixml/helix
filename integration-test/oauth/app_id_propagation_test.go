//go:build integration
// +build integration

package oauth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	goai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// This test demonstrates the issue with app ID not being properly propagated
// through the context chain in streaming sessions. It creates a full end-to-end
// test that follows the production code path for streaming sessions and OAuth token usage.
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
		t.Logf("Mock GitHub received request %d to: %s with auth: %s",
			requestCounter, capturedPath, capturedAuthHeader)

		// For proper testing, handle auth vs no auth differently
		if capturedAuthHeader == "" || !strings.Contains(capturedAuthHeader, "Bearer github-test-token") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"message":"Bad credentials","documentation_url":"https://docs.github.com/rest"}`))
			return
		}

		// Return appropriate response for a successful call
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"title":"Test Issue 1"},{"id":2,"title":"Test Issue 2"}]`))
	}))
	defer mockGithubServer.Close()
	t.Logf("Mock GitHub server started at %s", mockGithubServer.URL)

	// 2. Load server config and set up database connection
	cfg, err := config.LoadServerConfig()
	require.NoError(t, err, "Failed to load server config")

	// Use local PostgreSQL for tests
	cfg.Store.Host = "localhost"
	cfg.Store.Port = 5432
	cfg.Store.Username = "postgres"
	cfg.Store.Password = "postgres"
	cfg.Store.Database = "postgres"

	db, err := store.NewPostgresStore(cfg.Store)
	require.NoError(t, err, "Failed to create store connection")
	defer db.Close()

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

	githubTool := &types.Tool{
		ID:          fmt.Sprintf("tool_%d", time.Now().UnixNano()),
		Name:        "GitHub API",
		Description: "Access GitHub issues and repositories",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:           mockGithubServer.URL,
				OAuthProvider: providerName,
				OAuthScopes:   []string{"repo"},
				Schema:        githubToolSchema,
				Actions: []*types.ToolAPIAction{
					{
						Name:        "listUserIssues",
						Description: "List issues assigned to the authenticated user",
						Path:        "/issues",
						Method:      "GET",
					},
				},
				Headers: map[string]string{}, // Initialize headers map
			},
		},
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
					Provider: string(types.ProviderTogetherAI),
					Model:    "meta-llama/Llama-3.3-70B-Instruct-Turbo",
					Tools:    []*types.Tool{githubTool},
				},
			},
		},
	}

	app := &types.App{
		ID:        fmt.Sprintf("app_%d", time.Now().UnixNano()),
		Owner:     testUser.ID,
		OwnerType: types.OwnerTypeUser,
		AppSource: types.AppSourceHelix,
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
	require.Equal(t, 1, len(retrievedApp.Config.Helix.Assistants[0].Tools), "Assistant should have 1 tool")
	require.Equal(t, githubTool.ID, retrievedApp.Config.Helix.Assistants[0].Tools[0].ID, "Tool ID should match")
	t.Logf("Assistant tools verified. Tool ID: %s, Provider: %s",
		githubTool.ID, githubTool.Config.API.OAuthProvider)

	// 9. Set up the controller components for streaming
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	// Create a mock OpenAI client that returns a tool execution request
	mockClient := openai.NewMockClient(mockCtrl)

	// Set up a real provider manager with the mock OpenAI client for Helix
	providerManager := manager.NewProviderManager(&cfg, db, mockClient)

	// Create a mock RunnerControllerStatus to avoid nil pointer dereference
	mockRunnerController := manager.NewMockRunnerControllerStatus(mockCtrl)
	mockRunnerController.EXPECT().RunnerIDs().Return([]string{"mock-runner-id"}).AnyTimes()

	// Set the mock runner controller on the provider manager
	providerManager.SetRunnerController(mockRunnerController)

	// Set API key expectations for client initialization
	mockClient.EXPECT().APIKey().Return("test-key").AnyTimes()

	// Set up expectations for all CreateChatCompletion calls - this should return a tool call delta
	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Any()).
		Return(goai.ChatCompletionResponse{
			Choices: []goai.ChatCompletionChoice{
				{
					Message: goai.ChatCompletionMessage{
						Role: "assistant",
						ToolCalls: []goai.ChatCompletionToolCall{
							{
								ID:   "call_123",
								Type: "function",
								Function: goai.FunctionCall{
									Name:      "listUserIssues",
									Arguments: `{"filter":"assigned","state":"open","sort":"created"}`,
								},
							},
						},
					},
				},
			},
		}, nil).
		AnyTimes()

	// Create OAuth manager
	oauthManager := oauth.NewManager(db)
	err = oauthManager.LoadProviders(context.Background())
	require.NoError(t, err, "Failed to load OAuth providers")

	// Set up the rest of the controller
	toolsPlanner, err := tools.NewChainStrategy(&cfg, db, nil, mockClient)
	require.NoError(t, err, "Failed to create chain strategy")
	tools.InitChainStrategyOAuth(toolsPlanner, oauthManager, db, db)

	// Create a controller
	ctrlCtx := context.Background()

	// Create mock dependencies for controller
	mockFilestore := filestore.NewMockFileStore(mockCtrl)
	mockExtractor := extract.NewMockExtractor(mockCtrl)
	testJanitor := janitor.NewJanitor(config.Janitor{})

	// Set up some expected calls that might be needed
	mockFilestore.EXPECT().SignedURL(gomock.Any(), gomock.Any()).Return("http://localhost:8080/files/test.txt", nil).AnyTimes()
	mockFilestore.EXPECT().List(gomock.Any(), gomock.Any()).Return([]filestore.Item{}, nil).AnyTimes()
	mockFilestore.EXPECT().Get(gomock.Any(), gomock.Any()).Return(filestore.Item{}, nil).AnyTimes()
	mockExtractor.EXPECT().Extract(gomock.Any(), gomock.Any()).Return("test content", nil).AnyTimes()

	ctrl, err := controller.NewController(ctrlCtx, controller.Options{
		Config:          &cfg,
		Store:           db,
		OAuthManager:    oauthManager,
		ProviderManager: providerManager,
		Filestore:       mockFilestore,
		Extractor:       mockExtractor,
		Janitor:         testJanitor,
	})
	require.NoError(t, err, "Failed to create controller")
	ctrl.ToolsPlanner = toolsPlanner

	// 10. Create a session to run the test with
	session := types.Session{
		ID:        fmt.Sprintf("ses_%d", time.Now().UnixNano()),
		ParentApp: createdApp.ID,
		Owner:     testUser.ID,
		OwnerType: types.OwnerTypeUser,
		Mode:      types.SessionModeInference,
		Type:      types.SessionTypeText,
		ModelName: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
	}

	createdSession, err := db.CreateSession(context.Background(), session)
	require.NoError(t, err, "Failed to create session")
	defer func() {
		_, _ = db.DeleteSession(context.Background(), createdSession.ID)
	}()
	t.Logf("Created session with ID: %s, ParentApp: %s", createdSession.ID, createdSession.ParentApp)

	// 11. Create the streaming request
	req := &goai.ChatCompletionRequest{
		Model: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		Messages: []goai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "Show me my GitHub issues",
			},
		},
	}

	// 12. Set up the streaming context
	streamingCtx := context.Background()
	streamingCtx = openai.SetContextValues(streamingCtx, &openai.ContextValues{
		SessionID:     createdSession.ID,
		InteractionID: system.GenerateUUID(),
		OwnerID:       testUser.ID,
	})
	// Add the app ID to the context (this is what's missing in production)
	streamingCtx = openai.SetContextAppID(streamingCtx, createdApp.ID)

	// 13. Execute the real streaming session code path
	// Set up chat completion options
	opts := &controller.ChatCompletionOptions{
		AppID:       createdApp.ID,
		AssistantID: "test-assistant",
	}

	// This is the actual method we're testing - this follows the real production code path
	// IMPORTANT: This will fail in production because we're missing the app ID in context
	t.Log("Starting streaming session - this might fail due to missing appID in context")

	// Setup done, start the actual test part
	// We need to put this in its own subtest so we can properly observe whether it fails
	t.Run("ChatCompletionStream should use OAuth token", func(t *testing.T) {
		// Save current auth header state before running test
		originalRequestCount := requestCounter

		// Execute the actual streaming method directly
		stream, _, err := ctrl.ChatCompletionStream(streamingCtx, testUser, *req, opts)
		require.NoError(t, err, "ChatCompletionStream should not return an error")
		defer stream.Close()

		// We need to wait a bit to allow the asynchronous tool calls to complete
		// Read a few responses from the stream to ensure tool calls are processed
		streamDone := false
		t.Log("Reading from the stream and checking for tool calls...")

		for i := 0; i < 15 && !streamDone; i++ {
			resp, err := stream.Recv()
			if err == io.EOF {
				streamDone = true
				t.Log("Stream reached EOF")
				break
			}
			require.NoError(t, err, "Stream receive should not return an error")

			// Log what we're getting from the stream in more detail
			if len(resp.Choices) > 0 {
				choice := resp.Choices[0]
				if choice.Delta.ToolCalls != nil && len(choice.Delta.ToolCalls) > 0 {
					toolCall := choice.Delta.ToolCalls[0]
					t.Logf("Tool call detected: %s with args: %s",
						toolCall.Function.Name, toolCall.Function.Arguments)

					// This is important - we need to let the system execute the tool
					// The tool execution happens asynchronously after the tool call is received
					t.Log("Tool call received, waiting for tool execution...")
				}
			}

			// Sleep longer to allow API calls to process
			time.Sleep(300 * time.Millisecond)
		}

		// Wait a bit longer for any asynchronous tool execution to complete
		t.Log("Waiting for any tool executions to complete...")
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
