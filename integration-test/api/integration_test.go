package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"log"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func TestMain(m *testing.M) {
	// Start server
	startAPIServer()

	// Wait for server to be ready
	if err := waitForAPIServer(); err != nil {
		log.Fatalf("Failed to start API server: %v", err)
	}

	runTests := m.Run()
	os.Exit(runTests)
}

func startAPIServer() {
	go func() {
		cmd := exec.Command("helix", "serve")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

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

		if err := cmd.Run(); err != nil {
			log.Printf("Failed to start API server: %v", err)
		}
	}()

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

func getApiClient(apiKey string) (*client.HelixClient, error) {
	apiClient, err := client.NewClient("http://localhost:8080", apiKey)
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
func createUser(db *store.PostgresStore, kc *auth.KeycloakAuthenticator, email string) (user *types.User, apiKey string, err error) {
	userID := uuid.New().String()

	user = &types.User{
		ID:    userID,
		Email: email,
	}

	user, err = db.CreateUser(context.Background(), user)
	if err != nil {
		return nil, "", err
	}

	// Create user in Keycloak
	_, err = kc.CreateKeycloakUser(context.Background(), user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create user in Keycloak: %w", err)
	}

	apiKey, err = system.GenerateAPIKey()
	if err != nil {
		return nil, "", err
	}

	_, err = db.CreateAPIKey(context.Background(), &types.ApiKey{
		Name:      "first-test-key",
		Key:       apiKey,
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	})
	if err != nil {
		return nil, "", err
	}

	return user, apiKey, nil
}
