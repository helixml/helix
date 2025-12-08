package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// OAuthTestSuite tests OAuth functionality
type OAuthTestSuite struct {
	suite.Suite
	ctrl    *gomock.Controller
	store   *store.MockStore
	manager *Manager
	ctx     context.Context
}

func TestOAuthSuite(t *testing.T) {
	suite.Run(t, new(OAuthTestSuite))
}

func (suite *OAuthTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.ctrl = gomock.NewController(suite.T())
	suite.store = store.NewMockStore(suite.ctrl)

	// Create a test GitHub provider with all required fields
	githubProvider := &types.OAuthProvider{
		ID:           "github-provider-id",
		Name:         "GitHub",
		Type:         types.OAuthProviderTypeGitHub,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user", // Add this to prevent token refresh errors
		Enabled:      true,
	}

	// Setup provider expectations
	suite.store.EXPECT().
		GetOAuthProvider(gomock.Any(), "github-provider-id").
		Return(githubProvider, nil).
		AnyTimes()

	suite.store.EXPECT().
		GenerateRandomState(gomock.Any()).
		Return("test-state", nil).
		AnyTimes()

	// Mock UpdateOAuthConnection to avoid unexpected call errors
	suite.store.EXPECT().
		UpdateOAuthConnection(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, conn *types.OAuthConnection) (*types.OAuthConnection, error) {
			return conn, nil
		}).
		AnyTimes()

	// Now that we've setup store expectations, create the manager
	suite.manager = NewManager(suite.store, false)
}

// TestGetTokenForTool tests getting a token for a tool with OAuth provider
func (suite *OAuthTestSuite) TestGetTokenForTool() {
	// Create a test user
	userID := "test-user-id"

	// Create a GitHub connection for the user
	githubConnection := &types.OAuthConnection{
		ID:           "github-conn-id",
		UserID:       userID,
		ProviderID:   "github-provider-id",
		AccessToken:  "github-access-token",
		RefreshToken: "github-refresh-token",
		Scopes:       []string{"repo", "read:user"},
		ExpiresAt:    time.Now().Add(time.Hour * 24), // Set expiry in the future to avoid refresh
		Provider: types.OAuthProvider{
			ID:          "github-provider-id",
			Name:        "GitHub",
			Type:        types.OAuthProviderTypeGitHub,
			UserInfoURL: "https://api.github.com/user",
		},
	}

	// Create a Slack connection for the user
	slackConnection := &types.OAuthConnection{
		ID:           "slack-conn-id",
		UserID:       userID,
		ProviderID:   "slack-provider-id",
		AccessToken:  "slack-access-token",
		RefreshToken: "slack-refresh-token",
		Scopes:       []string{"channels:read", "chat:write"},
		ExpiresAt:    time.Now().Add(time.Hour * 24),
		Provider: types.OAuthProvider{
			ID:   "slack-provider-id",
			Name: "Slack",
			Type: types.OAuthProviderTypeSlack,
		},
	}

	// Setup expectations
	suite.store.EXPECT().
		ListOAuthProviders(gomock.Any(), &store.ListOAuthProvidersQuery{
			Enabled: true,
		}).
		Return([]*types.OAuthProvider{&githubConnection.Provider, &slackConnection.Provider}, nil).
		AnyTimes()

	suite.store.EXPECT().
		ListOAuthConnections(gomock.Any(), &store.ListOAuthConnectionsQuery{
			UserID:     userID,
			ProviderID: githubConnection.ProviderID,
		}).
		Return([]*types.OAuthConnection{githubConnection}, nil).
		AnyTimes()

	// Test getting GitHub token
	token, err := suite.manager.GetTokenForTool(suite.ctx, userID, "GitHub", []string{"repo"})
	suite.NoError(err)
	suite.Equal(githubConnection.AccessToken, token)

	// Test getting token for non-existent provider
	_, err = suite.manager.GetTokenForTool(suite.ctx, userID, "nonexistent", []string{})
	suite.Error(err)

	// Test getting token with insufficient scopes
	_, err = suite.manager.GetTokenForTool(suite.ctx, userID, "GitHub", []string{"repo", "admin:org"})
	suite.Error(err)

	// Check that we got a "no active connection" error (not a ScopeError)
	suite.Contains(err.Error(), "no active connection found for provider GitHub")

	// The test doesn't need to check for ScopeError since in current implementation
	// connections with missing scopes are skipped, causing a "no active connection" error
	// If implementation changes to return ScopeError directly, this test needs adjustment
}

// TestGetTokenForApp tests getting a token for an app with OAuth provider
func (suite *OAuthTestSuite) TestGetTokenForApp() {
	// Create a test user
	userID := "test-user-id"

	// Create a GitHub connection for the user
	githubConnection := &types.OAuthConnection{
		ID:           "github-conn-id",
		UserID:       userID,
		ProviderID:   "github-provider-id",
		AccessToken:  "github-access-token",
		RefreshToken: "github-refresh-token",
		Scopes:       []string{"repo", "read:user"},
		ExpiresAt:    time.Now().Add(time.Hour * 24), // Set expiry in the future to avoid refresh
		Provider: types.OAuthProvider{
			ID:          "github-provider-id",
			Name:        "GitHub",
			Type:        types.OAuthProviderTypeGitHub,
			UserInfoURL: "https://api.github.com/user",
		},
	}

	// Setup expectations
	suite.store.EXPECT().
		ListOAuthProviders(gomock.Any(), &store.ListOAuthProvidersQuery{
			Enabled: true,
		}).
		Return([]*types.OAuthProvider{&githubConnection.Provider}, nil).
		AnyTimes()

	suite.store.EXPECT().
		ListOAuthConnections(gomock.Any(), &store.ListOAuthConnectionsQuery{
			UserID:     userID,
			ProviderID: githubConnection.ProviderID,
		}).
		Return([]*types.OAuthConnection{githubConnection}, nil).
		AnyTimes()

	// Test getting GitHub token
	token, err := suite.manager.GetTokenForApp(suite.ctx, userID, "GitHub")
	suite.NoError(err)
	suite.Equal(githubConnection.AccessToken, token)

	// Test getting token for non-existent provider
	_, err = suite.manager.GetTokenForApp(suite.ctx, userID, "nonexistent")
	suite.Error(err)
}

// TestIntegrationWithTools tests the integration between OAuth tokens and API tools
func (suite *OAuthTestSuite) TestIntegrationWithTools() {
	userID := "test-user-id"

	// Create a GitHub connection for the user
	githubConnection := &types.OAuthConnection{
		ID:           "github-conn-id",
		UserID:       userID,
		ProviderID:   "github-provider-id",
		AccessToken:  "github-access-token",
		RefreshToken: "github-refresh-token",
		Scopes:       []string{"repo", "read:user"},
		ExpiresAt:    time.Now().Add(time.Hour * 24), // Set expiry in the future to avoid refresh
		Provider: types.OAuthProvider{
			ID:          "github-provider-id",
			Name:        "GitHub",
			Type:        types.OAuthProviderTypeGitHub,
			UserInfoURL: "https://api.github.com/user",
		},
	}

	// Setup expectations
	suite.store.EXPECT().
		ListOAuthProviders(gomock.Any(), &store.ListOAuthProvidersQuery{
			Enabled: true,
		}).
		Return([]*types.OAuthProvider{&githubConnection.Provider}, nil).
		AnyTimes()

	suite.store.EXPECT().
		ListOAuthConnections(gomock.Any(), &store.ListOAuthConnectionsQuery{
			UserID:     userID,
			ProviderID: githubConnection.ProviderID,
		}).
		Return([]*types.OAuthConnection{githubConnection}, nil).
		AnyTimes()

	// Get token for the GitHub provider
	token, err := suite.manager.GetTokenForApp(suite.ctx, userID, "GitHub")
	suite.NoError(err)
	suite.Equal(githubConnection.AccessToken, token)

	// Setup a test server to verify the authorization header
	var receivedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"success": true}`))
		if err != nil {
			suite.T().Fatalf("Failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	// Create a simple HTTP client for the request
	client := &http.Client{}

	// Prepare the HTTP request
	httpReq, err := http.NewRequest("GET", ts.URL+"/user/repos", nil)
	suite.NoError(err)

	// Set the Authorization header with the token
	expectedAuthHeader := "Bearer " + token
	httpReq.Header.Set("Authorization", expectedAuthHeader)

	// Make the request
	resp, err := client.Do(httpReq)
	suite.NoError(err)
	defer resp.Body.Close()

	// Verify the response status
	suite.Equal(http.StatusOK, resp.StatusCode)

	// The test server should have received the authorization header
	suite.Equal(expectedAuthHeader, receivedAuthHeader)
}
