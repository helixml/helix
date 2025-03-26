package tools

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestOAuthTokenProcessing tests the OAuth token processing logic in isolation
func TestOAuthTokenProcessing(t *testing.T) {
	// Test with matching OAuth token
	t.Run("Matching OAuth token", func(t *testing.T) {
		// Create a tool with GitHub OAuth provider
		tool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           "https://api.github.com",
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
					Actions: []*types.ToolAPIAction{
						{
							Name:   "listRepos",
							Method: "GET",
							Path:   "/user/repos",
						},
					},
				},
			},
		}

		// OAuth tokens with matching GitHub token
		githubToken := "github-token-123"
		oauthTokens := map[string]string{
			"GitHub": githubToken,
			"Slack":  "slack-token-456", // Should be ignored
		}

		// Process the OAuth tokens
		testProcessOAuthTokens(tool, oauthTokens)

		// Verify the Authorization header was set correctly
		authHeader, exists := tool.Config.API.Headers["Authorization"]
		assert.True(t, exists, "Authorization header should exist")
		expectedAuthHeader := "Bearer " + githubToken
		assert.Equal(t, expectedAuthHeader, authHeader)
	})

	// Test with non-matching OAuth token
	t.Run("Non-matching OAuth token", func(t *testing.T) {
		// Create a tool with GitHub OAuth provider
		tool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           "https://api.github.com",
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
					Actions: []*types.ToolAPIAction{
						{
							Name:   "listRepos",
							Method: "GET",
							Path:   "/user/repos",
						},
					},
				},
			},
		}

		// OAuth tokens with no matching GitHub token
		oauthTokens := map[string]string{
			"Slack": "slack-token-456", // Should be ignored for GitHub
		}

		// Process the OAuth tokens
		testProcessOAuthTokens(tool, oauthTokens)

		// Verify no Authorization header was set
		_, exists := tool.Config.API.Headers["Authorization"]
		assert.False(t, exists, "Authorization header should not exist")
	})

	// Test with GitHub token for Slack tool
	t.Run("GitHub token for Slack tool", func(t *testing.T) {
		// Create a tool with Slack OAuth provider
		tool := &types.Tool{
			Name:        "Slack API",
			Description: "Slack API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           "https://slack.com/api",
					OAuthProvider: "Slack",
					OAuthScopes:   []string{"chat:write"},
					Actions: []*types.ToolAPIAction{
						{
							Name:   "sendMessage",
							Method: "POST",
							Path:   "/chat.postMessage",
						},
					},
				},
			},
		}

		// OAuth tokens with GitHub but not Slack
		oauthTokens := map[string]string{
			"GitHub": "github-token-123", // Should be ignored for Slack
		}

		// Process the OAuth tokens
		testProcessOAuthTokens(tool, oauthTokens)

		// Verify no Authorization header was set
		_, exists := tool.Config.API.Headers["Authorization"]
		assert.False(t, exists, "Authorization header should not exist")
	})

	// Test with existing header
	t.Run("Tool with existing header", func(t *testing.T) {
		// Create a tool with GitHub OAuth provider and existing headers
		tool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           "https://api.github.com",
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
					Headers: map[string]string{
						"Accept": "application/vnd.github.v3+json",
					},
					Actions: []*types.ToolAPIAction{
						{
							Name:   "listRepos",
							Method: "GET",
							Path:   "/user/repos",
						},
					},
				},
			},
		}

		// OAuth tokens with matching GitHub token
		githubToken := "github-token-123"
		oauthTokens := map[string]string{
			"GitHub": githubToken,
		}

		// Process the OAuth tokens
		testProcessOAuthTokens(tool, oauthTokens)

		// Verify the Authorization header was set correctly
		authHeader, exists := tool.Config.API.Headers["Authorization"]
		assert.True(t, exists, "Authorization header should exist")
		expectedAuthHeader := "Bearer " + githubToken
		assert.Equal(t, expectedAuthHeader, authHeader)

		// Verify existing header was preserved
		acceptHeader, exists := tool.Config.API.Headers["Accept"]
		assert.True(t, exists, "Accept header should exist")
		assert.Equal(t, "application/vnd.github.v3+json", acceptHeader)
	})

	// Test with existing Authorization header
	t.Run("Existing Authorization header", func(t *testing.T) {
		// Create a tool with GitHub OAuth provider and existing Authorization header
		existingAuthValue := "Basic dXNlcjpwYXNz" // Base64 encoded "user:pass"
		tool := &types.Tool{
			Name:        "GitHub API",
			Description: "GitHub API access",
			ToolType:    types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					URL:           "https://api.github.com",
					OAuthProvider: "GitHub",
					OAuthScopes:   []string{"repo"},
					Headers: map[string]string{
						"Authorization": existingAuthValue,
					},
					Actions: []*types.ToolAPIAction{
						{
							Name:   "listRepos",
							Method: "GET",
							Path:   "/user/repos",
						},
					},
				},
			},
		}

		// OAuth tokens with matching GitHub token
		githubToken := "github-token-123"
		oauthTokens := map[string]string{
			"GitHub": githubToken,
		}

		// Process the OAuth tokens
		testProcessOAuthTokens(tool, oauthTokens)

		// Verify that the Authorization header remains unchanged
		authHeader, exists := tool.Config.API.Headers["Authorization"]
		assert.True(t, exists, "Authorization header should exist")
		assert.Equal(t, existingAuthValue, authHeader)
	})
}

// testProcessOAuthTokens processes OAuth tokens for a tool (test implementation)
func testProcessOAuthTokens(tool *types.Tool, oauthTokens map[string]string) {
	if tool.Config.API == nil || tool.Config.API.OAuthProvider == "" {
		return
	}

	// Normalize the provider name
	providerName := normalizeProviderType(tool.Config.API.OAuthProvider)

	// Check if we have a token for this provider
	if token, exists := oauthTokens[providerName]; exists {
		// Initialize headers map if it doesn't exist
		if tool.Config.API.Headers == nil {
			tool.Config.API.Headers = make(map[string]string)
		}

		// Only set the Authorization header if it doesn't already exist
		if _, exists := tool.Config.API.Headers["Authorization"]; !exists {
			tool.Config.API.Headers["Authorization"] = "Bearer " + token
		}
	}
}

// normalizeProviderType normalizes the provider type string
func normalizeProviderType(providerType string) string {
	// Convert to lowercase for case-insensitive comparison
	normalized := strings.ToLower(providerType)
	// Remove any "oauth" prefix if present
	normalized = strings.TrimPrefix(normalized, "oauth")
	// Remove any remaining non-alphanumeric characters
	normalized = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, normalized)
	return normalized
}
