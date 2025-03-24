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
					OAuthProvider: types.OAuthProviderTypeGitHub,
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
			string(types.OAuthProviderTypeGitHub): githubToken,
			string(types.OAuthProviderTypeSlack):  "slack-token-456", // Should be ignored
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthTokens)

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
					OAuthProvider: types.OAuthProviderTypeGitHub,
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
			string(types.OAuthProviderTypeSlack): "slack-token-456", // Should be ignored for GitHub
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthTokens)

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
					OAuthProvider: types.OAuthProviderTypeSlack,
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
			string(types.OAuthProviderTypeGitHub): "github-token-123", // Should be ignored for Slack
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthTokens)

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
					OAuthProvider: types.OAuthProviderTypeGitHub,
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
			string(types.OAuthProviderTypeGitHub): githubToken,
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthTokens)

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
					OAuthProvider: types.OAuthProviderTypeGitHub,
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
			string(types.OAuthProviderTypeGitHub): githubToken, // Should be ignored because existing header
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthTokens)

		// Verify Authorization header was not changed
		authHeader, exists := tool.Config.API.Headers["Authorization"]
		assert.True(t, exists, "Authorization header should exist")
		assert.Equal(t, existingAuthValue, authHeader, "Authorization header should not be changed")
	})
}

// Helper function to process OAuth tokens - extracted from RunAPIActionWithParameters
func processOAuthTokens(tool *types.Tool, oauthTokens map[string]string) {
	if len(oauthTokens) == 0 || tool.Config.API == nil || tool.Config.API.OAuthProvider == "" {
		return
	}

	toolProviderType := string(tool.Config.API.OAuthProvider)

	// Check if Authorization header already exists
	authHeaderKey := "Authorization"
	if tool.Config.API.Headers != nil {
		if _, exists := tool.Config.API.Headers[authHeaderKey]; exists {
			// Don't override existing Authorization header
			return
		}
	}

	// Look for a direct match in the map
	if token, exists := oauthTokens[toolProviderType]; exists {
		// Add the token to headers
		if tool.Config.API.Headers == nil {
			tool.Config.API.Headers = make(map[string]string)
		}
		tool.Config.API.Headers[authHeaderKey] = "Bearer " + token
		return
	}

	// Try normalized provider matching for backward compatibility
	normalizedToolType := normalizeProviderType(toolProviderType)
	for providerType, token := range oauthTokens {
		if normalizeProviderType(providerType) == normalizedToolType {
			// Add the token to headers
			if tool.Config.API.Headers == nil {
				tool.Config.API.Headers = make(map[string]string)
			}
			tool.Config.API.Headers[authHeaderKey] = "Bearer " + token
			return
		}
	}
}

// normalizeProviderType converts provider types to lowercase and standardizes them
func normalizeProviderType(providerType string) string {
	return strings.ToLower(providerType)
}
