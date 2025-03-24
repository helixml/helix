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

		// OAuth environment variables with matching GitHub token
		githubToken := "github-token-123"
		oauthEnvVars := []string{
			"OAUTH_TOKEN_GITHUB=" + githubToken,
			"OAUTH_TOKEN_SLACK=slack-token-456", // Should be ignored
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthEnvVars)

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

		// OAuth environment variables with no matching GitHub token
		oauthEnvVars := []string{
			"OAUTH_TOKEN_SLACK=slack-token-456", // Should be ignored for GitHub
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthEnvVars)

		// Verify no Authorization header was set
		_, exists := tool.Config.API.Headers["Authorization"]
		assert.False(t, exists, "Authorization header should not exist")
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

		// OAuth environment variables with matching GitHub token
		githubToken := "github-token-123"
		oauthEnvVars := []string{
			"OAUTH_TOKEN_GITHUB=" + githubToken, // Should be ignored because existing header
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthEnvVars)

		// Verify Authorization header was not changed
		authHeader, exists := tool.Config.API.Headers["Authorization"]
		assert.True(t, exists, "Authorization header should exist")
		assert.Equal(t, existingAuthValue, authHeader, "Authorization header should not be changed")
	})

	// Test with multiple OAuth tokens
	t.Run("Multiple OAuth tokens", func(t *testing.T) {
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

		// OAuth environment variables with multiple tokens including GitHub
		githubToken := "github-token-123"
		oauthEnvVars := []string{
			"OAUTH_TOKEN_SLACK=slack-token-456",   // Should be ignored
			"OAUTH_TOKEN_GITHUB=" + githubToken,   // Should be used
			"OAUTH_TOKEN_GOOGLE=google-token-789", // Should be ignored
		}

		// Process the OAuth tokens
		processOAuthTokens(tool, oauthEnvVars)

		// Verify the Authorization header was set correctly with GitHub token
		authHeader, exists := tool.Config.API.Headers["Authorization"]
		assert.True(t, exists, "Authorization header should exist")
		expectedAuthHeader := "Bearer " + githubToken
		assert.Equal(t, expectedAuthHeader, authHeader)
	})
}

// Helper function to process OAuth tokens - extracted from RunAPIActionWithParameters
func processOAuthTokens(tool *types.Tool, oauthEnvVars []string) {
	if len(oauthEnvVars) == 0 || tool.Config.API == nil || tool.Config.API.OAuthProvider == "" {
		return
	}

	toolProviderType := string(tool.Config.API.OAuthProvider)

	for _, envVar := range oauthEnvVars {
		parts := splitOAuthVar(envVar)
		if len(parts) != 2 {
			continue
		}

		envKey, envValue := parts[0], parts[1]
		// Only process OAUTH_TOKEN_ variables
		if !isOAuthTokenVar(envKey) {
			continue
		}

		// Extract provider type from env var name (e.g., OAUTH_TOKEN_GITHUB -> github)
		envProviderType := extractProviderType(envKey)

		// Only use token if provider types match
		if isProviderMatch(envProviderType, toolProviderType) {
			// Add the token to headers if not already in headers
			authHeaderKey := "Authorization"
			if tool.Config.API.Headers == nil {
				tool.Config.API.Headers = make(map[string]string)
			}

			if _, exists := tool.Config.API.Headers[authHeaderKey]; !exists {
				// Add OAuth token as Bearer token if the tool doesn't already have an auth header
				tool.Config.API.Headers[authHeaderKey] = "Bearer " + envValue
			}
			// Break after finding the matching token
			break
		}
	}
}

// Helper functions to make the code more testable and readable
func splitOAuthVar(envVar string) []string {
	return splitString(envVar, "=")
}

func splitString(s, sep string) []string {
	// Using strings.SplitN but simplified for testing
	if s == "" {
		return []string{}
	}

	for i := 0; i < len(s); i++ {
		if s[i:i+1] == sep {
			return []string{s[:i], s[i+1:]}
		}
	}

	return []string{s}
}

func isOAuthTokenVar(key string) bool {
	return len(key) > 12 && key[:12] == "OAUTH_TOKEN_"
}

func extractProviderType(key string) string {
	if !isOAuthTokenVar(key) {
		return ""
	}
	return key[12:]
}

func isProviderMatch(envProviderType, toolProviderType string) bool {
	return toolProviderType != "" &&
		envProviderType != "" &&
		normalizeProviderType(envProviderType) == normalizeProviderType(toolProviderType)
}

func normalizeProviderType(providerType string) string {
	// Convert to lowercase for case-insensitive comparison
	return strings.ToLower(providerType)
}
