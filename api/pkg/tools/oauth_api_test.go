package tools

import (
	"context"
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
	testServer *httptest.Server
	authHeader string
}

// prepareRequest overrides the ChainStrategy method to return a simple request to the test server
func (t *TestChainStrategy) prepareRequest(ctx context.Context, tool *types.Tool, action string, params map[string]string) (*http.Request, error) {
	// Create a simple GET request to the test server (we're not actually using the action name in the test)
	req, err := http.NewRequest("GET", t.testServer.URL, nil)
	if err != nil {
		return nil, err
	}

	// Add the Authorization header if it exists in the tool config
	if tool.Config.API != nil && tool.Config.API.Headers != nil {
		if authHeader, exists := tool.Config.API.Headers["Authorization"]; exists {
			req.Header.Set("Authorization", authHeader)
			t.authHeader = authHeader
		}
	}

	return req, nil
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
		w.Write(responseJSON)
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
					OAuthProvider: types.OAuthProviderTypeGitHub,
					OAuthScopes:   []string{"repo"},
				},
			},
		}

		// Create the API request with GitHub OAuth token
		githubToken := "github-token-123"
		oauthEnvVars := []string{
			"OAUTH_TOKEN_GITHUB=" + githubToken,
			"OAUTH_TOKEN_SLACK=slack-token-456", // Should be ignored
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthEnvVars)

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
					OAuthProvider: types.OAuthProviderTypeGitHub,
					OAuthScopes:   []string{"repo"},
				},
			},
		}

		// Create the API request with only a Slack OAuth token
		oauthEnvVars := []string{
			"OAUTH_TOKEN_SLACK=slack-token-456", // Should be ignored for GitHub tool
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthEnvVars)

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
		capturedAuthHeader = "" // Reset captured header

		// Create a GitHub tool with pre-defined Authorization header
		existingAuthValue := "Basic dXNlcjpwYXNz" // Base64 encoded "user:pass"
		githubTool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           ts.URL,
					OAuthProvider: types.OAuthProviderTypeGitHub,
					OAuthScopes:   []string{"repo"},
					Headers: map[string]string{
						"Authorization": existingAuthValue,
					},
				},
			},
		}

		// Create the API request with a GitHub OAuth token
		githubToken := "github-token-123"
		oauthEnvVars := []string{
			"OAUTH_TOKEN_GITHUB=" + githubToken, // Should be ignored because existing header present
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthEnvVars)

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

		// Verify the Authorization header was not changed
		assert.Equal(t, existingAuthValue, capturedAuthHeader)
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
					OAuthProvider: types.OAuthProviderTypeGitHub,
					OAuthScopes:   []string{"repo"},
				},
			},
		}

		// Create the API request with multiple OAuth tokens
		githubToken := "github-token-123"
		oauthEnvVars := []string{
			"OAUTH_TOKEN_GITHUB=" + githubToken,
			"OAUTH_TOKEN_SLACK=slack-token-456",
			"OAUTH_TOKEN_GOOGLE=google-token-789",
		}

		// Process the OAuth token directly
		processOAuthTokens(githubTool, oauthEnvVars)

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
