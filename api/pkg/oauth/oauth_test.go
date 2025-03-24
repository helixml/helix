package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/license"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

// MockOAuthStore is a minimal mock implementation of store.Store for testing OAuth functionality
type MockOAuthStore struct {
	connections []*types.OAuthConnection
	providers   []*types.OAuthProvider
}

// Implement the methods needed for OAuth functionality
func (m *MockOAuthStore) CreateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error) {
	m.providers = append(m.providers, provider)
	return provider, nil
}

func (m *MockOAuthStore) GetOAuthProvider(ctx context.Context, id string) (*types.OAuthProvider, error) {
	for _, p := range m.providers {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *MockOAuthStore) ListOAuthProviders(ctx context.Context, _ *store.ListOAuthProvidersQuery) ([]*types.OAuthProvider, error) {
	return m.providers, nil
}

func (m *MockOAuthStore) CreateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error) {
	m.connections = append(m.connections, connection)
	return connection, nil
}

func (m *MockOAuthStore) ListOAuthConnections(ctx context.Context, query *store.ListOAuthConnectionsQuery) ([]*types.OAuthConnection, error) {
	var result []*types.OAuthConnection
	for _, c := range m.connections {
		if c.UserID == query.UserID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *MockOAuthStore) GenerateRandomState(ctx context.Context) (string, error) {
	return "test-state", nil
}

// Stub out the rest of the store.Store interface with minimal implementations
func (m *MockOAuthStore) CountUsers(ctx context.Context) (int64, error) {
	return 0, nil
}

func (m *MockOAuthStore) CreateAPIKey(ctx context.Context, apiKey *types.ApiKey) (*types.ApiKey, error) {
	return nil, nil
}

// Implement all the other Store interface methods with nil returns
func (m *MockOAuthStore) UpdateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteOAuthProvider(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) GetOAuthConnection(ctx context.Context, id string) (*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetOAuthConnectionByUserAndProvider(ctx context.Context, userID, providerID string) (*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteOAuthConnection(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) GetOAuthConnectionsNearExpiry(ctx context.Context, threshold time.Time) ([]*types.OAuthConnection, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateOAuthRequestToken(ctx context.Context, token *types.OAuthRequestToken) (*types.OAuthRequestToken, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetOAuthRequestToken(ctx context.Context, userID, providerID string) ([]*types.OAuthRequestToken, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetOAuthRequestTokenByState(ctx context.Context, state string) ([]*types.OAuthRequestToken, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteOAuthRequestToken(ctx context.Context, id string) error {
	return nil
}

// Implement all other Store interface methods with minimal nil implementations
func (m *MockOAuthStore) CreateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetOrganization(ctx context.Context, q *store.GetOrganizationQuery) (*types.Organization, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteOrganization(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) ListOrganizations(ctx context.Context, query *store.ListOrganizationsQuery) ([]*types.Organization, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetOrganizationMembership(ctx context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteOrganizationMembership(ctx context.Context, organizationID, userID string) error {
	return nil
}

func (m *MockOAuthStore) ListOrganizationMemberships(ctx context.Context, query *store.ListOrganizationMembershipsQuery) ([]*types.OrganizationMembership, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateTeam(ctx context.Context, team *types.Team) (*types.Team, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetTeam(ctx context.Context, q *store.GetTeamQuery) (*types.Team, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateTeam(ctx context.Context, team *types.Team) (*types.Team, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteTeam(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) ListTeams(ctx context.Context, query *store.ListTeamsQuery) ([]*types.Team, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateTeamMembership(ctx context.Context, membership *types.TeamMembership) (*types.TeamMembership, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetTeamMembership(ctx context.Context, q *store.GetTeamMembershipQuery) (*types.TeamMembership, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListTeamMemberships(ctx context.Context, query *store.ListTeamMembershipsQuery) ([]*types.TeamMembership, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteTeamMembership(ctx context.Context, teamID, userID string) error {
	return nil
}

func (m *MockOAuthStore) CreateRole(ctx context.Context, role *types.Role) (*types.Role, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetRole(ctx context.Context, id string) (*types.Role, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateRole(ctx context.Context, role *types.Role) (*types.Role, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteRole(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) ListRoles(ctx context.Context, organizationID string) ([]*types.Role, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateAccessGrant(ctx context.Context, resourceAccess *types.AccessGrant, roles []*types.Role) (*types.AccessGrant, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListAccessGrants(ctx context.Context, q *store.ListAccessGrantsQuery) ([]*types.AccessGrant, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteAccessGrant(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateAccessGrantRoleBinding(ctx context.Context, binding *types.AccessGrantRoleBinding) (*types.AccessGrantRoleBinding, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteAccessGrantRoleBinding(ctx context.Context, accessGrantID, roleID string) error {
	return nil
}

func (m *MockOAuthStore) GetAccessGrantRoleBindings(ctx context.Context, q *store.GetAccessGrantRoleBindingsQuery) ([]*types.AccessGrantRoleBinding, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetSession(ctx context.Context, id string) (*types.Session, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetSessions(ctx context.Context, query store.GetSessionsQuery) ([]*types.Session, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetSessionsCounter(ctx context.Context, query store.GetSessionsQuery) (*types.Counter, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateSessionName(ctx context.Context, sessionID, name string) error {
	return nil
}

func (m *MockOAuthStore) UpdateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateSessionMeta(ctx context.Context, data types.SessionMetaUpdate) (*types.Session, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteSession(ctx context.Context, id string) (*types.Session, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetUser(ctx context.Context, q *store.GetUserQuery) (*types.User, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateUser(ctx context.Context, user *types.User) (*types.User, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateUser(ctx context.Context, user *types.User) (*types.User, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteUser(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) ListUsers(ctx context.Context, query *store.ListUsersQuery) ([]*types.User, error) {
	return nil, nil
}

func (m *MockOAuthStore) SearchUsers(ctx context.Context, query *store.SearchUsersQuery) ([]*types.User, int64, error) {
	return nil, 0, nil
}

func (m *MockOAuthStore) GetUserMeta(ctx context.Context, id string) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockOAuthStore) CreateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockOAuthStore) EnsureUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListAPIKeys(ctx context.Context, query *store.ListAPIKeysQuery) ([]*types.ApiKey, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteAPIKey(ctx context.Context, apiKey string) error {
	return nil
}

func (m *MockOAuthStore) CreateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetProviderEndpoint(ctx context.Context, q *store.GetProviderEndpointsQuery) (*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListProviderEndpoints(ctx context.Context, q *store.ListProviderEndpointsQuery) ([]*types.ProviderEndpoint, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteProviderEndpoint(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetSecret(ctx context.Context, id string) (*types.Secret, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListSecrets(ctx context.Context, q *store.ListSecretsQuery) ([]*types.Secret, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteSecret(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateApp(ctx context.Context, tool *types.App) (*types.App, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateApp(ctx context.Context, tool *types.App) (*types.App, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetApp(ctx context.Context, id string) (*types.App, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetAppWithTools(ctx context.Context, id string) (*types.App, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListApps(ctx context.Context, q *store.ListAppsQuery) ([]*types.App, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteApp(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateDataEntity(ctx context.Context, dataEntity *types.DataEntity) (*types.DataEntity, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateDataEntity(ctx context.Context, dataEntity *types.DataEntity) (*types.DataEntity, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetDataEntity(ctx context.Context, id string) (*types.DataEntity, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListDataEntities(ctx context.Context, q *store.ListDataEntitiesQuery) ([]*types.DataEntity, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteDataEntity(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockOAuthStore) LookupKnowledge(ctx context.Context, q *store.LookupKnowledgeQuery) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error) {
	return nil, nil
}

func (m *MockOAuthStore) UpdateKnowledgeState(ctx context.Context, id string, state types.KnowledgeState, message string) error {
	return nil
}

func (m *MockOAuthStore) ListKnowledge(ctx context.Context, q *store.ListKnowledgeQuery) ([]*types.Knowledge, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteKnowledge(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateKnowledgeVersion(ctx context.Context, version *types.KnowledgeVersion) (*types.KnowledgeVersion, error) {
	return nil, nil
}

func (m *MockOAuthStore) GetKnowledgeVersion(ctx context.Context, id string) (*types.KnowledgeVersion, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListKnowledgeVersions(ctx context.Context, q *store.ListKnowledgeVersionQuery) ([]*types.KnowledgeVersion, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteKnowledgeVersion(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateScriptRun(ctx context.Context, task *types.ScriptRun) (*types.ScriptRun, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListScriptRuns(ctx context.Context, q *types.GptScriptRunsQuery) ([]*types.ScriptRun, error) {
	return nil, nil
}

func (m *MockOAuthStore) DeleteScriptRun(ctx context.Context, id string) error {
	return nil
}

func (m *MockOAuthStore) CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	return nil, nil
}

func (m *MockOAuthStore) ListLLMCalls(ctx context.Context, q *store.ListLLMCallsQuery) ([]*types.LLMCall, int64, error) {
	return nil, 0, nil
}

func (m *MockOAuthStore) GetLicenseKey(ctx context.Context) (*types.LicenseKey, error) {
	return nil, nil
}

func (m *MockOAuthStore) SetLicenseKey(ctx context.Context, licenseKey string) error {
	return nil
}

func (m *MockOAuthStore) GetDecodedLicense(ctx context.Context) (*license.License, error) {
	return nil, nil
}

// OAuthTestSuite tests OAuth functionality
type OAuthTestSuite struct {
	suite.Suite
	store   *MockOAuthStore
	manager *Manager
	ctx     context.Context
}

func TestOAuthSuite(t *testing.T) {
	suite.Run(t, new(OAuthTestSuite))
}

func (suite *OAuthTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.store = &MockOAuthStore{
		connections: make([]*types.OAuthConnection, 0),
		providers:   make([]*types.OAuthProvider, 0),
	}
	suite.manager = NewManager(suite.store)

	// Add a test GitHub provider
	githubProvider := &types.OAuthProvider{
		ID:           "github-provider-id",
		Name:         "GitHub",
		Type:         types.OAuthProviderTypeGitHub,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		Enabled:      true,
	}
	suite.store.providers = append(suite.store.providers, githubProvider)
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
		Provider: types.OAuthProvider{
			Type: types.OAuthProviderTypeGitHub,
		},
	}
	suite.store.connections = append(suite.store.connections, githubConnection)

	// Create a Slack connection for the user
	slackConnection := &types.OAuthConnection{
		ID:           "slack-conn-id",
		UserID:       userID,
		ProviderID:   "slack-provider-id",
		AccessToken:  "slack-access-token",
		RefreshToken: "slack-refresh-token",
		Scopes:       []string{"chat:write"},
		Provider: types.OAuthProvider{
			Type: types.OAuthProviderTypeSlack,
		},
	}
	suite.store.connections = append(suite.store.connections, slackConnection)

	// Test getting GitHub token
	token, err := suite.manager.GetTokenForTool(suite.ctx, userID, types.OAuthProviderTypeGitHub, []string{"repo"})
	suite.NoError(err)
	suite.Equal(githubConnection.AccessToken, token)

	// Test getting token for non-existent provider
	_, err = suite.manager.GetTokenForTool(suite.ctx, userID, "nonexistent", []string{})
	suite.Error(err)

	// Test getting token with insufficient scopes
	_, err = suite.manager.GetTokenForTool(suite.ctx, userID, types.OAuthProviderTypeGitHub, []string{"repo", "admin:org"})
	suite.Error(err)

	// Check that the error is a ScopeError
	var scopeErr *ScopeError
	suite.ErrorAs(err, &scopeErr)
	suite.Contains(scopeErr.Missing, "admin:org")
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
		Provider: types.OAuthProvider{
			Type: types.OAuthProviderTypeGitHub,
		},
	}
	suite.store.connections = append(suite.store.connections, githubConnection)

	// Test getting GitHub token
	token, err := suite.manager.GetTokenForApp(suite.ctx, userID, types.OAuthProviderTypeGitHub)
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
		Provider: types.OAuthProvider{
			Type: types.OAuthProviderTypeGitHub,
		},
	}
	suite.store.connections = append(suite.store.connections, githubConnection)

	// Get token for the GitHub provider
	token, err := suite.manager.GetTokenForApp(suite.ctx, userID, types.OAuthProviderTypeGitHub)
	suite.NoError(err)
	suite.Equal(githubConnection.AccessToken, token)

	// Setup a test server to verify the authorization header
	var receivedAuthHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
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
