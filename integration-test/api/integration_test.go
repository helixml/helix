package api

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"log"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

var serverCmd *exec.Cmd
var serverLogBuffer *bytes.Buffer

func TestMain(m *testing.M) {
	// Check for required directories
	checkDirectories()

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

// checkDirectories ensures all required directories exist and have proper permissions
func checkDirectories() {
	// Check filestore path
	filestorePath := getEnvOrDefault("FILESTORE_LOCALFS_PATH", "/tmp")
	if _, err := os.Stat(filestorePath); os.IsNotExist(err) {
		log.Printf("FILESTORE_LOCALFS_PATH %s does not exist, creating it", filestorePath)
		if err := os.MkdirAll(filestorePath, 0777); err != nil {
			log.Fatalf("Failed to create FILESTORE_LOCALFS_PATH %s: %v", filestorePath, err)
		}
	}

	// Check NATS store directory
	natsStorePath := getEnvOrDefault("NATS_STORE_DIR", "/tmp/nats")
	if _, err := os.Stat(natsStorePath); os.IsNotExist(err) {
		log.Printf("NATS_STORE_DIR %s does not exist, creating it", natsStorePath)
		if err := os.MkdirAll(natsStorePath, 0777); err != nil {
			log.Fatalf("Failed to create NATS_STORE_DIR %s: %v", natsStorePath, err)
		}
	}

	// Ensure the directories are writable
	testFile := filestorePath + "/.test_write"
	if err := os.WriteFile(testFile, []byte("test"), 0666); err != nil {
		log.Fatalf("FILESTORE_LOCALFS_PATH %s is not writable: %v", filestorePath, err)
	}
	os.Remove(testFile)

	testFile = natsStorePath + "/.test_write"
	if err := os.WriteFile(testFile, []byte("test"), 0666); err != nil {
		log.Fatalf("NATS_STORE_DIR %s is not writable: %v", natsStorePath, err)
	}
	os.Remove(testFile)

	log.Printf("All required directories exist and are writable")
}

func startAPIServer() *bytes.Buffer {
	// Create a buffer to capture server output
	serverLogBuffer = bytes.NewBuffer(nil)

	// Use configurable API port for flexibility
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "18080" // Default to 18080 if not specified (high port to avoid conflicts)
	}

	serverCmd = exec.Command("helix", "serve")
	go func() {
		cmd := exec.Command("helix", "serve")

		// Set up a multiwriter to capture logs and also write to stderr for immediate visibility
		mw := io.MultiWriter(serverLogBuffer, os.Stderr)
		cmd.Stdout = mw
		cmd.Stderr = mw

		// Get PostgreSQL connection details from environment
		pgHost := os.Getenv("POSTGRES_HOST")
		pgPort := os.Getenv("POSTGRES_PORT")
		pgUser := os.Getenv("POSTGRES_USER")
		pgPass := os.Getenv("POSTGRES_PASSWORD")
		pgDB := os.Getenv("POSTGRES_DATABASE")
		dbURL := os.Getenv("DATABASE_URL")

		// Log the database connection details for debugging
		log.Printf("Database connection details:")
		if dbURL != "" {
			log.Printf("  DATABASE_URL=%s", strings.Replace(dbURL, "postgres://postgres:"+pgPass+"@", "postgres://postgres:****@", 1))
		}
		log.Printf("  POSTGRES_HOST=%s", pgHost)
		log.Printf("  POSTGRES_PORT=%s", pgPort)
		log.Printf("  POSTGRES_USER=%s", pgUser)
		log.Printf("  POSTGRES_DATABASE=%s", pgDB)
		log.Printf("  POSTGRES_PASSWORD=%s", strings.Repeat("*", len(pgPass))) // Mask password in logs

		// Get the main env variables from the current environment
		cmd.Env = os.Environ()

		// Get file storage paths
		filestorePath := getEnvOrDefault("FILESTORE_LOCALFS_PATH", "/tmp")
		natsStorePath := getEnvOrDefault("NATS_STORE_DIR", "/tmp/nats")

		// Log the paths for debugging
		log.Printf("File storage paths:")
		log.Printf("  FILESTORE_LOCALFS_PATH=%s", filestorePath)
		log.Printf("  NATS_STORE_DIR=%s", natsStorePath)

		// Make sure the directories exist
		os.MkdirAll(filestorePath, 0755)
		os.MkdirAll(natsStorePath, 0755)

		// Define the rest env variables, similarly to what we set in docker-compose.dev.yaml
		cmd.Env = append(cmd.Env,
			fmt.Sprintf("SERVER_PORT=%s", apiPort),
			"LOG_LEVEL=debug",
			fmt.Sprintf("APP_URL=http://localhost:%s", apiPort),
			"RUNNER_TOKEN=oh-hallo-insecure-token",
			fmt.Sprintf("SERVER_URL=http://localhost:%s", apiPort),
			// Use environment variables for paths if they exist, otherwise fallback to defaults
			fmt.Sprintf("FILESTORE_LOCALFS_PATH=%s", filestorePath),
			fmt.Sprintf("NATS_STORE_DIR=%s", natsStorePath),
			"FRONTEND_URL=/tmp",          // No frontend here but doesn't matter for API integration tests
			"DATABASE_AUTO_MIGRATE=true", // Ensure migrations run
		)

		log.Printf("Starting API server on port %s", apiPort)
		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start API server: %v", err)
			return
		}

		// Store the command for later cleanup
		serverCmd = cmd

		// Get the error from Wait() to check if the process terminated abnormally
		if err := cmd.Wait(); err != nil {
			log.Printf("API server process terminated with error: %v", err)
			// Only print logs if we haven't already sent them to stderr
			if serverLogBuffer != nil {
				log.Printf("Last server logs before termination:\n%s", serverLogBuffer.String())
			}
		}
	}()

	return serverLogBuffer
}

// Wait for API to be ready
func waitForAPIServer() error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	timeout := time.After(30 * time.Second)
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()

	// Use configurable API port for flexibility
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "18080" // Default to 18080 if not specified (high port to avoid conflicts)
	}

	healthcheckURL := fmt.Sprintf("http://localhost:%s/api/v1/healthz", apiPort)
	log.Printf("Testing API health with URL: %s", healthcheckURL)

	failureCount := 0
	for {
		select {
		case <-timeout:
			// On timeout, let's dump the server log buffer to help debug
			if serverCmd != nil && serverCmd.Process != nil {
				log.Printf("API server startup timed out, dumping all captured server logs:")
				fmt.Printf("=== START SERVER LOG DUMP ===\n%s\n=== END SERVER LOG DUMP ===\n", serverLogBuffer.String())
			}
			return fmt.Errorf("timeout waiting for API server to start")
		case <-tick.C:
			log.Printf("Attempting to connect to: %s", healthcheckURL)
			resp, err := client.Get(healthcheckURL)
			if err != nil {
				failureCount++
				log.Printf("API not ready yet, URL: %s, error: %v (attempt %d)", healthcheckURL, err, failureCount)

				// Periodically log server output during startup attempts
				if failureCount%5 == 0 && serverLogBuffer != nil {
					logContent := serverLogBuffer.String()
					if len(logContent) > 0 {
						log.Printf("Server logs after %d connection attempts:\n%s", failureCount, logContent)
					} else {
						log.Printf("No server logs captured after %d connection attempts", failureCount)
					}
				}
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				log.Printf("API health check successful: %s returned status %d", healthcheckURL, resp.StatusCode)
				return nil
			}
			log.Printf("API returned status code %d for URL: %s", resp.StatusCode, healthcheckURL)
		}
	}
}

func getAPIClient(userAPIKey string) (*client.HelixClient, error) {
	// Use configurable API port for flexibility
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "18080" // Default to 18080 if not specified (high port to avoid conflicts)
	}

	apiURL := fmt.Sprintf("http://localhost:%s", apiPort)
	fmt.Printf("Creating API client with URL: %s\n", apiURL)

	apiClient, err := client.NewClient(apiURL, userAPIKey)
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

	// Check if we should use docker exec for PostgreSQL access
	useDockerExec := os.Getenv("USE_DOCKER_EXEC")
	if useDockerExec == "true" {
		// When using docker exec, we'll use direct SQL commands instead of GORM
		log.Printf("PostgreSQL connections will use docker exec instead of direct TCP")
	}

	// Normal path - use the loaded config
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

// Helper function to get an environment variable or return a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	return defaultValue
}
