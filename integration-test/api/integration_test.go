package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
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

// generateIntegrationRunnerToken returns a fresh random token for the test
// server. The old hard-coded value was a public credential and is now
// rejected by the API (security finding H4/H8).
func generateIntegrationRunnerToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// serverLogTailBytes bounds how much of the server's (debug-level) output we
// keep. Enough to cover the requests around a failure without holding minutes
// of trace output in memory.
const serverLogTailBytes = 4 << 20 // 4 MiB

// serverLogBuffer keeps the tail of the API server's output. exec writes to it
// from its stdout and stderr copier goroutines while the test goroutine reads
// it, so every access is mutex guarded.
type serverLogBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *serverLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if excess := len(b.buf) - serverLogTailBytes; excess > 0 {
		b.buf = b.buf[excess:]
	}
	return len(p), nil
}

func (b *serverLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// dump writes the captured server output to stderr. Without this the logs that
// explain a failure (upstream 429s, exhausted provider balance, panics) are
// collected and then silently discarded, leaving only an opaque test timeout.
func (b *serverLogBuffer) dump(reason string) {
	if b == nil {
		return
	}
	contents := b.String()
	if contents == "" {
		fmt.Fprintf(os.Stderr, "\n===== API server logs (%s): EMPTY =====\n", reason)
		return
	}
	fmt.Fprintf(os.Stderr, "\n===== API server logs (%s), last %d bytes =====\n%s\n===== end API server logs =====\n",
		reason, len(contents), contents)
}

// dumpServerLogsBeforeTestTimeout arms a watchdog that prints the server logs
// shortly before `go test -timeout` panics the process. The panic kills the
// test binary outright, so anything deferred until after m.Run() never runs —
// which is exactly how an 8 minute hang produced a stack trace with no server
// side context at all.
func dumpServerLogsBeforeTestTimeout(logs *serverLogBuffer) {
	f := flag.Lookup("test.timeout")
	if f == nil {
		return
	}
	timeout, err := time.ParseDuration(f.Value.String())
	if err != nil || timeout <= 0 {
		return
	}
	lead := 30 * time.Second
	if timeout <= lead {
		lead = timeout / 4
	}
	time.AfterFunc(timeout-lead, func() {
		logs.dump(fmt.Sprintf("watchdog: %s test timeout approaching", timeout))
	})
}

func TestMain(m *testing.M) {
	// Load file
	_ = godotenv.Load(".test.env")

	startServer := os.Getenv("START_HELIX_TEST_SERVER") == "true"
	// Accumulate server logs
	var buf *serverLogBuffer

	if startServer {
		// Start server
		buf = startAPIServer()

		// Wait for server to be ready
		if err := waitForAPIServer(buf); err != nil {
			log.Printf("Failed to start API server: %v", err)
			buf.dump("server failed to start")
			os.Exit(1)
		}

		dumpServerLogsBeforeTestTimeout(buf)
	}

	runTests := m.Run()

	if startServer {
		// The server's logs are the only place the cause of a failure is
		// recorded — the test binary sees an HTTP response, not why the agent
		// produced it. Dumping them only on startup failure meant a red CI run
		// showed the assertion and nothing else, which is a long way to go to
		// find out a provider was returning 429.
		if runTests != 0 {
			buf.dump("tests failed")
		}

		// Clean up the server process
		if serverCmd != nil && serverCmd.Process != nil {
			if err := serverCmd.Process.Kill(); err != nil {
				log.Printf("Failed to kill server process: %v", err)
			}
		}
	}

	os.Exit(runTests)
}

func startAPIServer() *serverLogBuffer {
	buf := &serverLogBuffer{}

	serverCmd = exec.Command("helix", "serve")
	serverCmd.Stdout = buf
	serverCmd.Stderr = buf

	// Get the main env variables for keycloak, database, etc.
	serverCmd.Env = os.Environ()

	// Define the rest env variables, similarly to what we set in docker-compose.dev.yaml
	serverCmd.Env = append(serverCmd.Env,
		"SERVER_PORT="+integrationServerPort(),
		"LOG_LEVEL=debug",
		"APP_URL="+integrationServerURL(),
		"RUNNER_TOKEN="+generateIntegrationRunnerToken(),
		"SERVER_URL="+integrationServerURL(),
		"ASSET_SSH_PROXY_LISTEN=127.0.0.1:"+integrationAssetSSHProxyPort(),
		"ASSET_SSH_PROXY_ADDRESS=127.0.0.1:"+integrationAssetSSHProxyPort(),
		"FILESTORE_LOCALFS_PATH=/tmp",
		"FRONTEND_URL=/tmp", // No frontend here but doesn't matter for API integration tests
		"FILESTORE_AVATARS_PATH=/tmp/avatars",
	)

	fmt.Printf("Starting API server on port %s\n", integrationServerPort())

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
func waitForAPIServer(serverLogs *serverLogBuffer) error {
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
			resp, err := httpClient.Get(integrationServerURL() + "/api/v1/config")
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
	apiClient, err := client.NewClient(integrationServerURL(), userAPIKey, false)
	if err != nil {
		return nil, err
	}
	return apiClient, nil
}

func integrationServerPort() string {
	if value := os.Getenv("HELIX_INTEGRATION_SERVER_PORT"); value != "" {
		return value
	}
	return "8080"
}

func integrationServerURL() string {
	return "http://localhost:" + integrationServerPort()
}

func integrationAssetSSHProxyPort() string {
	if value := os.Getenv("HELIX_INTEGRATION_ASSET_SSH_PROXY_PORT"); value != "" {
		return value
	}
	return "2224"
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

	t.Logf("generated API key for user %s", createdUser.ID)

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
