package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"log"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/joho/godotenv"
)

var serverCmd *exec.Cmd
var serverExited = make(chan error, 1) // signals when the server process exits

func TestMain(m *testing.M) {
	// Load file
	_ = godotenv.Load(".test.env")

	startServer := os.Getenv("START_HELIX_TEST_SERVER") == "true"
	// Accumulate server logs
	var buf *bytes.Buffer

	if startServer {
		// Start server
		buf = startAPIServer()

		// Wait for server to be ready
		if err := waitForAPIServer(buf); err != nil {
			log.Printf("Failed to start API server: %v", err)
			log.Printf("Server logs:\n%s", buf.String())
			os.Exit(1)
		}
	}

	runTests := m.Run()

	if startServer {
		// Clean up the server process
		if serverCmd != nil && serverCmd.Process != nil {
			if err := serverCmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill server process: %v", err)
			}
		}
	}

	os.Exit(runTests)
}

func startAPIServer() *bytes.Buffer {
	buf := bytes.NewBuffer(nil)

	serverCmd = exec.Command("helix", "serve")
	serverCmd.Stdout = buf
	serverCmd.Stderr = buf

	// Get the main env variables for keycloak, database, etc.
	serverCmd.Env = os.Environ()

	// Define the rest env variables, similarly to what we set in docker-compose.dev.yaml
	serverCmd.Env = append(serverCmd.Env,
		"SERVER_PORT=8080",
		"LOG_LEVEL=debug",
		"APP_URL=http://localhost:8080",
		"RUNNER_TOKEN=oh-hallo-insecure-token",
		"SERVER_URL=http://localhost:8080",
		"FILESTORE_LOCALFS_PATH=/tmp",
		"FRONTEND_URL=/tmp", // No frontend here but doesn't matter for API integration tests
		"FILESTORE_AVATARS_PATH=/tmp/avatars",
	)

	fmt.Println("Starting API server on port 8080")

	if err := serverCmd.Start(); err != nil {
		log.Printf("Failed to start API server: %v (%s)", err, buf.String())
		os.Exit(1)
	}

	// Monitor server process — detect early crashes
	go func() {
		serverExited <- serverCmd.Wait()
	}()

	return buf
}

// waitForAPIServer polls the server until it responds or the process exits.
func waitForAPIServer(serverLogs *bytes.Buffer) error {
	httpClient := &http.Client{
		Timeout: 5 * time.Second,
	}

	timeout := time.After(30 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case err := <-serverExited:
			// Server process crashed before becoming ready
			return fmt.Errorf("server process exited early: %v", err)
		case <-timeout:
			return fmt.Errorf("timeout waiting for API server to start")
		case <-tick.C:
			resp, err := httpClient.Get("http://localhost:8080/api/v1/config")
			if err != nil {
				log.Printf("API not ready yet: %v", err)
				continue
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return nil
			}
			log.Printf("API returned status code %d", resp.StatusCode)
		}
	}
}

func getAPIClient(userAPIKey string) (*client.HelixClient, error) {
	apiClient, err := client.NewClient("http://localhost:8080", userAPIKey, false)
	if err != nil {
		return nil, err
	}
	return apiClient, nil
}

func getStoreClient() (*store.PostgresStore, error) {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return nil, err
	}

	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-memory pubsub: %w", err)
	}

	store, err := store.NewPostgresStore(cfg.Store, ps)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// createUser - creates user in the database and returns the user and api key
func createUser(t *testing.T, db *store.PostgresStore, authenticator auth.Authenticator, email string) (user *types.User, apiKey string, err error) {
	t.Helper()
	// Create user with generated ID
	user = &types.User{
		ID:       system.GenerateUUID(),
		Email:    email,
		Username: email,
		FullName: "test user " + time.Now().Format("20060102150405"),
	}
	createdUser, err := authenticator.CreateUser(context.Background(), user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create user: %w", err)
	}

	t.Logf("created user: %+v", createdUser)

	apiKey, err = system.GenerateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}

	t.Logf("generated API key for user %s: %s", createdUser.ID, apiKey)

	_, err = db.CreateAPIKey(context.Background(), &types.ApiKey{
		Name:      "first-test-key",
		Key:       apiKey,
		Owner:     createdUser.ID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	})
	if err != nil {
		return nil, "", err
	}

	return createdUser, apiKey, nil
}

func createApp(t *testing.T, apiClient *client.HelixClient, agentConfig *types.App) (*types.App, error) {
	t.Helper()

	app, err := apiClient.CreateApp(context.Background(), agentConfig)
	if err != nil {
		return nil, err
	}

	return app, nil
}

// TestExternalAgentModelParameter tests that external agent sessions
// properly handle the model parameter and don't get rejected with
// "you must provide a model parameter" error
func TestExternalAgentModelParameter(t *testing.T) {
	if os.Getenv("START_HELIX_TEST_SERVER") != "true" {
		t.Skip("Skipping integration test - set START_HELIX_TEST_SERVER=true to enable")
	}

	db, err := getStoreClient()
	if err != nil {
		t.Fatalf("Failed to get store client: %v", err)
	}

	// Initialize authenticator for user creation
	cfg := &config.ServerConfig{}
	authenticator, err := auth.NewHelixAuthenticator(cfg, db, "test-secret", nil)
	if err != nil {
		t.Fatalf("Failed to create authenticator: %v", err)
	}

	// Create test user
	_, apiKey, err := createUser(t, db, authenticator, fmt.Sprintf("test-external-agent-%d@example.com", time.Now().Unix()))
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	apiClient, err := getAPIClient(apiKey)
	if err != nil {
		t.Fatalf("Failed to get API client: %v", err)
	}

	// Test session creation with external agent configuration
	sessionReq := &types.SessionChatRequest{
		Type:      types.SessionTypeText,
		Model:     "external_agent",
		AgentType: "zed_external",
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hello from external agent integration test",
					},
				},
			},
		},
		ExternalAgentConfig: &types.ExternalAgentConfig{
			Resolution: "1080p",
		},
	}

	// This should not fail with "you must provide a model parameter" error
	// Note: It may fail for other reasons (like no external agent available)
	// but we're specifically testing that the model parameter is accepted
	sessionID, err := apiClient.ChatSession(context.Background(), sessionReq)

	// The session creation might fail due to external agent not being available,
	// but it should NOT fail with "you must provide a model parameter"
	if err != nil {
		// Check that it's not the model parameter error
		if fmt.Sprintf("%v", err) == "400 Bad Request: you must provide a model parameter" {
			t.Fatalf("Got the model parameter error that should be fixed: %v", err)
		}
		// Other errors are acceptable for this test (external agent not available, etc.)
		t.Logf("Session creation failed with expected error (external agent not available): %v", err)
		return
	}

	// If session creation succeeded, log the session ID
	if sessionID != "" {
		t.Logf("Successfully created external agent session with ID: %s", sessionID)
	}
}
