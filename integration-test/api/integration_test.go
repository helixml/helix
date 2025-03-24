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
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/joho/godotenv"
)

var serverCmd *exec.Cmd

func TestMain(m *testing.M) {
	// Load file
	_ = godotenv.Load(".test.env")
	// Start server
	buf := startAPIServer()

	// Wait for server to be ready
	if err := waitForAPIServer(); err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}

	runTests := m.Run()

	// Clean up the server process
	if serverCmd != nil && serverCmd.Process != nil {
		if err := serverCmd.Process.Kill(); err != nil {
			log.Printf("Failed to kill server process: %v", err)
		}
	}
	// Print the server logs
	log.Printf("Server logs: %s", buf.String())

	os.Exit(runTests)
}

func startAPIServer() *bytes.Buffer {
	buf := bytes.NewBuffer(nil)
	serverCmd = exec.Command("helix", "serve")
	go func() {
		cmd := exec.Command("helix", "serve")

		cmd.Stdout = buf
		cmd.Stderr = buf

		// Get the main env variables for keycloak, database, etc.
		cmd.Env = os.Environ()

		// Define the rest env variables, similarly to what we set in docker-compose.dev.yaml
		cmd.Env = append(cmd.Env,
			"SERVER_PORT=8080",
			"LOG_LEVEL=debug",
			"APP_URL=http://localhost:8080",
			"RUNNER_TOKEN=oh-hallo-insecure-token",
			"SERVER_URL=http://localhost:8080",
			"FILESTORE_LOCALFS_PATH=/tmp",
			"FRONTEND_URL=/tmp", // No frontend here but doesn't matter for API integration tests
		)

		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start API server: %v", err)
			return
		}
	}()

	return buf
}

// Wait for API to be ready
func waitForAPIServer() error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	timeout := time.After(30 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for API server to start")
		case <-tick.C:
			resp, err := client.Get("http://localhost:8080/api/v1/healthz")
			if err != nil {
				log.Printf("API not ready yet: %v", err)
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				return nil
			}
			log.Printf("API returned status code %d", resp.StatusCode)
		}
	}
}

func getAPIClient(userAPIKey string) (*client.HelixClient, error) {
	apiClient, err := client.NewClient("http://localhost:8080", userAPIKey)
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
	store, err := store.NewPostgresStore(cfg.Store)
	if err != nil {
		return nil, err
	}
	return store, nil
}

// createUser - creates user in the database and returns the user and api key
func createUser(t *testing.T, db *store.PostgresStore, kc *auth.KeycloakAuthenticator, email string) (user *types.User, apiKey string, err error) {
	t.Helper()
	// Create user in Keycloak
	user = &types.User{
		Email:    email,
		Username: email,
		FullName: "test user " + time.Now().Format("20060102150405"),
	}
	createdUser, err := kc.CreateKeycloakUser(context.Background(), user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create user in Keycloak: %w", err)
	}

	t.Logf("created user in Keycloak: %+v", createdUser)

	user.ID = createdUser.ID

	user, err = db.CreateUser(context.Background(), user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create user in database: %w", err)
	}

	t.Logf("created user in database: %+v", user)

	apiKey, err = system.GenerateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}

	t.Logf("generated API key for user %s: %s", user.ID, apiKey)

	_, err = db.CreateAPIKey(context.Background(), &types.ApiKey{
		Name:      "first-test-key",
		Key:       apiKey,
		Owner:     user.ID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	})
	if err != nil {
		return nil, "", err
	}

	return user, apiKey, nil
}
