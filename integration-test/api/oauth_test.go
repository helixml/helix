package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
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
			w.Write([]byte(`{"message":"Bad credentials","documentation_url":"https://docs.github.com/rest"}`))
			t.Logf("MOCK SERVER: Returned 401 - Bad credentials (no valid token)")
			return
		}

		// Return appropriate response for a successful call
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":1,"title":"Test Issue 1"},{"id":2,"title":"Test Issue 2"}]`))
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
				URL:           githubAPIURL,
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
					Provider: "together",
					Model:    "llama3:instruct",
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

	// Create an in-memory PubSub for testing
	helixPubSub, err := pubsub.NewInMemoryNats()
	require.NoError(t, err, "Failed to create in-memory PubSub")

	// Create a RunnerController for the scheduler
	runnerCtrlCfg := &scheduler.RunnerControllerConfig{
		PubSub: helixPubSub,
		FS:     mockFilestore,
	}
	runnerController, err := scheduler.NewRunnerController(ctrlCtx, runnerCtrlCfg)
	require.NoError(t, err, "Failed to create runner controller")

	// Create a scheduler for the controller
	schedulerParams := &scheduler.Params{
		RunnerController: runnerController,
		QueueSize:        50,
	}
	scheduler, err := scheduler.NewScheduler(ctrlCtx, &cfg, schedulerParams)
	require.NoError(t, err, "Failed to create scheduler")

	ctrl, err := controller.NewController(ctrlCtx, controller.Options{
		Config:          &cfg,
		Store:           db,
		OAuthManager:    oauthManager,
		ProviderManager: providerManager,
		Filestore:       mockFilestore,
		Extractor:       mockExtractor,
		Janitor:         testJanitor,
		PubSub:          helixPubSub,
		Scheduler:       scheduler,
	})
	require.NoError(t, err, "Failed to create controller")
	ctrl.ToolsPlanner = toolsPlanner

	t.Logf("****************************************************")
	t.Logf("APP ID: %s", createdApp.ID)
	t.Logf("OWNER ID: %s", testUser.ID)
	t.Logf("TOOL ID: %s", githubTool.ID)
	t.Logf("GITHUB API URL: %s", githubAPIURL)
	t.Logf("****************************************************")

	// Setup done, start the actual test part
	// We need to put this in its own subtest so we can properly observe whether it fails
	t.Run("Sessions API should use OAuth token", func(t *testing.T) {
		// Save current auth header state before running test
		originalRequestCount := requestCounter

		// Create a sessions API request rather than directly calling ChatCompletionStream
		// This better simulates the actual production flow
		sessionReq := &types.SessionChatRequest{
			Model:        "llama3:instruct",
			Stream:       true,
			SystemPrompt: "You are a helpful assistant that can use GitHub to look up repository issues.",
			AppID:        createdApp.ID, // This is critical - setting the app ID explicitly
			Messages: []*types.Message{
				{
					Role: types.CreatorTypeUser,
					Content: types.MessageContent{
						ContentType: types.MessageContentTypeText,
						Parts:       []any{"What issues do I have on GitHub?"},
					},
				},
			},
		}

		// Convert session chat request to internal session request
		message, ok := sessionReq.Message()
		require.True(t, ok, "Failed to get message from session request")

		// Create user interaction
		userInteraction := &types.Interaction{
			ID:        system.GenerateUUID(),
			Created:   time.Now(),
			Updated:   time.Now(),
			Creator:   types.CreatorTypeUser,
			Mode:      types.SessionModeInference,
			Message:   message,
			State:     types.InteractionStateComplete,
			Finished:  true,
			Completed: time.Now(),
		}

		// Create a direct session request to the controller (no HTTP)
		internalReq := types.InternalSessionRequest{
			ID:               system.GenerateSessionID(),
			Stream:           true,
			Mode:             types.SessionModeInference,
			Type:             types.SessionTypeText,
			SystemPrompt:     sessionReq.SystemPrompt,
			ModelName:        "llama3:instruct",
			ParentApp:        createdApp.ID,
			Owner:            testUser.ID,
			OwnerType:        testUser.Type,
			UserInteractions: []*types.Interaction{userInteraction},
		}

		t.Logf("Starting a new session with app ID: %s", createdApp.ID)

		// Call the controller directly to create the session
		session, err := ctrl.StartSession(context.Background(), testUser, internalReq)
		require.NoError(t, err, "Failed to start session")

		t.Logf("****************************************************")
		t.Logf("CREATED SESSION ID: %s", session.ID)
		t.Logf("PARENT APP: %s", session.ParentApp)
		t.Logf("****************************************************")

		// Set up a subscription to the session's topic to see the messages
		sessionTopic := pubsub.GetSessionQueue(testUser.ID, session.ID)
		t.Logf("Subscribing to session topic: %s", sessionTopic)

		// Create a channel to signal when streaming is done
		doneCh := make(chan struct{})
		// Keep track of whether we saw any meaningful session responses
		sawResponse := false

		// Create a subscription to the session updates
		sub, err := ctrl.Options.PubSub.Subscribe(context.Background(), sessionTopic, func(payload []byte) error {
			// Try to parse the event
			var event types.WebsocketEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				t.Logf("Error unmarshaling event: %v", err)
				return nil
			}

			t.Logf("Received event type: %s for session: %s", event.Type, event.SessionID)

			// See if we've got a session with a useful response
			if event.Session != nil && len(event.Session.Interactions) > 0 {
				interaction := event.Session.Interactions[len(event.Session.Interactions)-1]
				if interaction.Creator == types.CreatorTypeAssistant && interaction.Message != "" {
					t.Logf("Received session response: %s", interaction.Message)
					sawResponse = true
				}

				// If the interaction is finished and has a tool_id, we know the tool was used
				if interaction.Finished && interaction.Metadata != nil {
					if toolID, ok := interaction.Metadata["tool_id"]; ok {
						t.Logf("Tool was used! Tool ID: %s", toolID)
						close(doneCh)
						return nil
					}
				}
			}

			// If we got a worker task response, show it
			if event.WorkerTaskResponse != nil && event.WorkerTaskResponse.Message != "" {
				t.Logf("Worker response: %s", event.WorkerTaskResponse.Message)
				sawResponse = true

				// If the worker task is done, we're done streaming
				if event.WorkerTaskResponse.Done {
					close(doneCh)
					return nil
				}
			}

			return nil
		})
		require.NoError(t, err, "Failed to subscribe to session topic")
		defer sub.Unsubscribe()

		// Wait for the streaming to complete or timeout
		select {
		case <-doneCh:
			t.Log("Session streaming completed")
		case <-time.After(10 * time.Second):
			t.Log("Session streaming timed out after 10 seconds")
		}

		if sawResponse {
			t.Log("Successfully received streaming responses from the session")
		} else {
			t.Log("Did not receive any streaming responses from the session")
		}

		// Wait for any asynchronous API calls to complete
		t.Log("Waiting for any API calls to complete...")
		time.Sleep(5 * time.Second)

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
