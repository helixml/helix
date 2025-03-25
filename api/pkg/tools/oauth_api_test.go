package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestChainStrategy is a simplified version of ChainStrategy for testing
type TestChainStrategy struct {
	ChainStrategy
}

// TestOAuthTokenInAPIAction tests that OAuth tokens are correctly added to API requests
func TestOAuthTokenInAPIAction(t *testing.T) {
	// Create a test server to capture the Authorization header
	var capturedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthHeader = r.Header.Get("Authorization")
		response := map[string]interface{}{
			"success": true,
		}
		responseJSON, _ := json.Marshal(response)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(responseJSON)
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	// Test with a GitHub API tool and matching OAuth token
	t.Run("Matching OAuth token", func(t *testing.T) {
		capturedAuthHeader = "" // Reset captured header

		// Create a GitHub tool with a valid action
		githubTool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           ts.URL,
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
				},
			},
		}

		// Create the API request with GitHub OAuth token
		githubToken := "github-token-123"
		oauthTokens := map[string]string{
			"GitHub": githubToken,
			"Slack":  "slack-token-456", // Should be ignored
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthTokens)

		// Create and send a request manually to test if the header was properly set
		req, err := http.NewRequest("GET", ts.URL, nil)
		assert.NoError(t, err)

		// Add the Authorization header if it exists
		if githubTool.Config.API.Headers != nil {
			if authHeader, exists := githubTool.Config.API.Headers["Authorization"]; exists {
				req.Header.Set("Authorization", authHeader)
			}
		}

		// Make the request
		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		// Verify the Authorization header was set correctly
		expectedAuthHeader := "Bearer " + githubToken
		assert.Equal(t, expectedAuthHeader, capturedAuthHeader)
	})

	// Test with non-matching OAuth token
	t.Run("Non-matching OAuth token", func(t *testing.T) {
		capturedAuthHeader = "" // Reset captured header

		// Create a GitHub tool
		githubTool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           ts.URL,
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
				},
			},
		}

		// Create the API request with only a Slack OAuth token
		oauthTokens := map[string]string{
			"Slack": "slack-token-456", // Should be ignored for GitHub tool
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthTokens)

		// Create and send a request manually to test if the header was properly set
		req, err := http.NewRequest("GET", ts.URL, nil)
		assert.NoError(t, err)

		// Add the Authorization header if it exists
		if githubTool.Config.API.Headers != nil {
			if authHeader, exists := githubTool.Config.API.Headers["Authorization"]; exists {
				req.Header.Set("Authorization", authHeader)
			}
		}

		// Make the request
		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		// Verify no Authorization header was set
		assert.Empty(t, capturedAuthHeader)
	})

	// Test with existing Authorization header
	t.Run("Existing Authorization header", func(t *testing.T) {
		// Set up a test server
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the Authorization header
			authHeader := r.Header.Get("Authorization")

			// Check that it's the Basic auth value, not the OAuth token
			assert.Equal(t, "Basic dXNlcjpwYXNz", authHeader)

			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"message": "success"}`))
			if err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
		}))
		defer ts.Close()

		// Create a tool with GitHub OAuth provider and existing Authorization header
		existingAuthValue := "Basic dXNlcjpwYXNz" // Base64 encoded "user:pass"
		githubTool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           ts.URL,
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
					Headers: map[string]string{
						"Authorization": existingAuthValue,
					},
				},
			},
		}

		// Create the API request with a GitHub OAuth token
		githubToken := "github-token-123"
		oauthTokens := map[string]string{
			"GitHub": githubToken, // Should be ignored because existing header present
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthTokens)

		// Create and send a request manually to test if the header was properly set
		req, err := http.NewRequest("GET", ts.URL, nil)
		assert.NoError(t, err)

		// Add headers from the tool
		if githubTool.Config.API.Headers != nil {
			for key, value := range githubTool.Config.API.Headers {
				req.Header.Set(key, value)
			}
		}

		// Send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		// Check the response status
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify that the Authorization header is still the original Basic auth
		assert.Equal(t, existingAuthValue, githubTool.Config.API.Headers["Authorization"])
	})

	// Test with multiple OAuth tokens
	t.Run("Multiple OAuth tokens", func(t *testing.T) {
		capturedAuthHeader = "" // Reset captured header

		// Create a GitHub tool
		githubTool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           ts.URL,
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
				},
			},
		}

		// Create the API request with multiple OAuth tokens
		githubToken := "github-token-123"
		oauthTokens := map[string]string{
			"GitHub": githubToken,
			"Slack":  "slack-token-456",
			"Google": "google-token-789",
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthTokens)

		// Create and send a request manually to test if the header was properly set
		req, err := http.NewRequest("GET", ts.URL, nil)
		assert.NoError(t, err)

		// Add the Authorization header if it exists
		if githubTool.Config.API.Headers != nil {
			if authHeader, exists := githubTool.Config.API.Headers["Authorization"]; exists {
				req.Header.Set("Authorization", authHeader)
			}
		}

		// Make the request
		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		defer resp.Body.Close()

		// Verify the correct token was used
		expectedAuthHeader := "Bearer " + githubToken
		assert.Equal(t, expectedAuthHeader, capturedAuthHeader)
	})
}
