package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKoditService_APIKeyAuth(t *testing.T) {
	expectedAPIKey := "test-api-key-12345" // gitleaks:allow
	var receivedAPIKey string
	var receivedRequest bool

	// Create a mock Kodit server that captures the API key header
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = true
		receivedAPIKey = r.Header.Get("X-API-Key")

		// Return a valid repository response for the create endpoint
		if r.URL.Path == "/api/v1/repositories" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			response := map[string]any{
				"data": map[string]any{
					"id":   "123",
					"type": "repository",
					"attributes": map[string]any{
						"remote_uri": "https://github.com/example/repo.git",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create the Kodit service with API key
	service := NewKoditService(mockServer.URL, expectedAPIKey)

	if !service.IsEnabled() {
		t.Fatal("Expected service to be enabled")
	}

	// Make a request to trigger the API key auth
	_, err := service.RegisterRepository(t.Context(), "https://github.com/example/repo.git")
	if err != nil {
		t.Fatalf("RegisterRepository failed: %v", err)
	}

	// Verify the request was made
	if !receivedRequest {
		t.Fatal("Expected mock server to receive a request")
	}

	// Verify the API key header was sent correctly
	if receivedAPIKey != expectedAPIKey {
		t.Errorf("Expected X-API-Key header to be %q, got %q", expectedAPIKey, receivedAPIKey)
	}
}

func TestKoditService_NoAPIKey(t *testing.T) {
	var receivedAPIKey string
	var receivedRequest bool

	// Create a mock Kodit server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = true
		receivedAPIKey = r.Header.Get("X-API-Key")

		if r.URL.Path == "/api/v1/repositories" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			response := map[string]any{
				"data": map[string]any{
					"id":   "123",
					"type": "repository",
					"attributes": map[string]any{
						"remote_uri": "https://github.com/example/repo.git",
					},
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	// Create the Kodit service WITHOUT API key
	service := NewKoditService(mockServer.URL, "")

	if !service.IsEnabled() {
		t.Fatal("Expected service to be enabled even without API key")
	}

	_, err := service.RegisterRepository(t.Context(), "https://github.com/example/repo.git")
	if err != nil {
		t.Fatalf("RegisterRepository failed: %v", err)
	}

	if !receivedRequest {
		t.Fatal("Expected mock server to receive a request")
	}

	// When no API key is configured, the header should be empty
	if receivedAPIKey != "" {
		t.Errorf("Expected no X-API-Key header when API key not configured, got %q", receivedAPIKey)
	}
}

func TestKoditService_DisabledWithoutBaseURL(t *testing.T) {
	service := NewKoditService("", "some-api-key")

	if service.IsEnabled() {
		t.Error("Expected service to be disabled when base URL is empty")
	}
}
