package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"log"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
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
			resp, err := client.Get("http://localhost:80/api/v1/healthz")
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

func getApiClient() (*client.HelixClient, error) {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	return apiClient, nil
}

func getStoreClient() (*store.PostgresStore, error) {
	cfg := config.Store{
		Host:     "localhost",
		Port:     5432,
		Database: "helix",
		Username: "helix",
		Password: "helix",
	}
	store, err := store.NewPostgresStore(cfg)
	if err != nil {
		return nil, err
	}
	return store, nil
}
