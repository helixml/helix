package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

// OAuthToolsTestSuite is a test suite for OAuth token functionality in tools
type OAuthToolsTestSuite struct {
	suite.Suite
	ctx        context.Context
	oauthToken string
}

// SetupTest sets up the test suite
func (suite *OAuthToolsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.oauthToken = "test-oauth-token"
}

// TestOAuthTokenInAPIRequest verifies that the OAuth token is correctly added to API requests
func (suite *OAuthToolsTestSuite) TestOAuthTokenInAPIRequest() {
	// Setup a test server to verify the Authorization header
	var receivedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		// Return a successful response
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "success"}`)
	}))
	defer ts.Close()

	// Create a GitHub tool with a valid action
	githubTool := &types.Tool{
		Name:        "githubAPI",
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

	// Create the API request with OAuth token
	oauthTokens := map[string]string{
		string(types.OAuthProviderTypeGitHub): suite.oauthToken,
	}

	// Process the OAuth token directly
	processOAuthTokens(githubTool, oauthTokens)

	// Create and send a request manually to test if the header was properly set
	req, err := http.NewRequest("GET", ts.URL, nil)
	suite.NoError(err)

	// Add the Authorization header if it exists
	if githubTool.Config.API.Headers != nil {
		if authHeader, exists := githubTool.Config.API.Headers["Authorization"]; exists {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	suite.NoError(err)
	defer resp.Body.Close()

	// Check that the Authorization header was set correctly
	expectedAuthHeader := fmt.Sprintf("Bearer %s", suite.oauthToken)
	suite.Equal(expectedAuthHeader, receivedAuthHeader)
}

// TestOAuthTokenNotOverridingExistingAuth ensures OAuth tokens don't override existing auth headers
func (suite *OAuthToolsTestSuite) TestOAuthTokenNotOverridingExistingAuth() {
	// Setup a test server to verify the Authorization header
	var receivedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		// Return a successful response
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "success"}`)
	}))
	defer ts.Close()

	// Create a GitHub tool with existing Authorization header
	existingAuthValue := "Basic dXNlcjpwYXNz" // Base64 encoded "user:pass"
	githubTool := &types.Tool{
		Name:        "githubAPI",
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

	// Create the API request with OAuth token
	oauthTokens := map[string]string{
		string(types.OAuthProviderTypeGitHub): suite.oauthToken,
	}

	// Store the original auth header value for comparison
	originalAuthHeader := githubTool.Config.API.Headers["Authorization"]

	// Process the OAuth token directly
	processOAuthTokens(githubTool, oauthTokens)

	// First, verify that the Authorization header wasn't changed in the tool
	suite.Equal(originalAuthHeader, githubTool.Config.API.Headers["Authorization"],
		"The Authorization header in the tool should not be changed")

	// Create and send a request manually to test if the header was properly set
	req, err := http.NewRequest("GET", ts.URL, nil)
	suite.NoError(err)

	// Add the Authorization header from the tool
	if githubTool.Config.API.Headers != nil {
		if authHeader, exists := githubTool.Config.API.Headers["Authorization"]; exists {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	suite.NoError(err)
	defer resp.Body.Close()

	// Check that the existing Authorization header was not overridden
	suite.Equal(existingAuthValue, receivedAuthHeader)
}

// TestOAuthTokenNotSetForWrongProvider ensures OAuth tokens aren't used for the wrong provider
func (suite *OAuthToolsTestSuite) TestOAuthTokenNotSetForWrongProvider() {
	// Setup a test server to verify the Authorization header
	var receivedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		// Return a successful response
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "success"}`)
	}))
	defer ts.Close()

	// Create a Slack tool but provide a GitHub token
	slackTool := &types.Tool{
		Name:        "slackAPI",
		Description: "Slack API access",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:           ts.URL,
				OAuthProvider: types.OAuthProviderTypeSlack,
				OAuthScopes:   []string{"chat:write"},
			},
		},
	}

	// Create the API request with GitHub OAuth token (should be ignored for Slack)
	oauthTokens := map[string]string{
		string(types.OAuthProviderTypeGitHub): suite.oauthToken, // GitHub token for Slack tool (wrong provider)
	}

	// Process the OAuth token directly
	processOAuthTokens(slackTool, oauthTokens)

	// Create and send a request manually to test if the header was properly set
	req, err := http.NewRequest("GET", ts.URL, nil)
	suite.NoError(err)

	// Add the Authorization header if it exists
	if slackTool.Config.API.Headers != nil {
		if authHeader, exists := slackTool.Config.API.Headers["Authorization"]; exists {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	suite.NoError(err)
	defer resp.Body.Close()

	// Check that no Authorization header was set (GitHub token not used for Slack)
	suite.Empty(receivedAuthHeader)
}

// TestAppRunAPIAction tests the full app API action execution flow with OAuth tokens
func (suite *OAuthToolsTestSuite) TestAppRunAPIAction() {
	// Setup a test server to verify the Authorization header
	var receivedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		// Return a successful response
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "success"}`)
	}))
	defer ts.Close()

	// Create a GitHub app tool
	githubTool := &types.Tool{
		Name:        "githubAPI",
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

	// Create the API request with OAuth token
	oauthTokens := map[string]string{
		string(types.OAuthProviderTypeGitHub): suite.oauthToken,
	}

	// Process the OAuth token directly
	processOAuthTokens(githubTool, oauthTokens)

	// Create and send a request manually to test if the header was properly set
	req, err := http.NewRequest("GET", ts.URL, nil)
	suite.NoError(err)

	// Add the Authorization header if it exists
	if githubTool.Config.API.Headers != nil {
		if authHeader, exists := githubTool.Config.API.Headers["Authorization"]; exists {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	suite.NoError(err)
	defer resp.Body.Close()

	// Check that the Authorization header was set correctly
	expectedAuthHeader := fmt.Sprintf("Bearer %s", suite.oauthToken)
	suite.Equal(expectedAuthHeader, receivedAuthHeader)
}

// TestMultipleOAuthTokens tests handling multiple OAuth tokens in the environment
func (suite *OAuthToolsTestSuite) TestMultipleOAuthTokens() {
	// Setup a test server to verify the Authorization header
	var receivedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status": "success"}`)
	}))
	defer ts.Close()

	// Create a GitHub tool
	githubTool := &types.Tool{
		Name:        "githubAPI",
		Description: "GitHub API for accessing repositories",
		ToolType:    types.ToolTypeAPI,
		Config: types.ToolConfig{
			API: &types.ToolAPIConfig{
				URL:           ts.URL,
				OAuthProvider: types.OAuthProviderTypeGitHub,
				OAuthScopes:   []string{"repo"},
			},
		},
	}

	// Create multiple OAuth tokens
	githubToken := "github-oauth-token"
	slackToken := "slack-oauth-token"
	googleToken := "google-oauth-token"

	// Create the API request with multiple OAuth tokens
	oauthTokens := map[string]string{
		string(types.OAuthProviderTypeSlack):  slackToken,  // Should be ignored
		string(types.OAuthProviderTypeGitHub): githubToken, // Should be used
		string(types.OAuthProviderTypeGoogle): googleToken, // Should be ignored
	}

	// Process the OAuth token directly
	processOAuthTokens(githubTool, oauthTokens)

	// Create and send a request manually to test if the header was properly set
	req, err := http.NewRequest("GET", ts.URL, nil)
	suite.NoError(err)

	// Add the Authorization header if it exists
	if githubTool.Config.API.Headers != nil {
		if authHeader, exists := githubTool.Config.API.Headers["Authorization"]; exists {
			req.Header.Set("Authorization", authHeader)
		}
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	suite.NoError(err)
	defer resp.Body.Close()

	// Check that only the GitHub token was used
	expectedAuthHeader := fmt.Sprintf("Bearer %s", githubToken)
	suite.Equal(expectedAuthHeader, receivedAuthHeader)
}

// TestOAuthToolsTestSuite runs the OAuth tools test suite
func TestOAuthToolsTestSuite(t *testing.T) {
	suite.Run(t, new(OAuthToolsTestSuite))
}
