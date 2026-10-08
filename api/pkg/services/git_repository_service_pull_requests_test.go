package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type prTestStore struct {
	store.Store
	oauthConnections []*types.OAuthConnection
	oauthConnection  *types.OAuthConnection
	gitConnections   []*types.GitProviderConnection
	repository       *types.GitRepository
}

func (s *prTestStore) ListGitProviderConnections(_ context.Context, userID string) ([]*types.GitProviderConnection, error) {
	var result []*types.GitProviderConnection
	for _, conn := range s.gitConnections {
		if conn.UserID == userID {
			result = append(result, conn)
		}
	}
	return result, nil
}

func encryptedTestCredential(t *testing.T, value string) string {
	t.Helper()
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := crypto.EncryptAES256GCM([]byte(value), key)
	if err != nil {
		t.Fatal(err)
	}
	return encrypted
}

func (s *prTestStore) GetGitRepository(_ context.Context, _ string) (*types.GitRepository, error) {
	return s.repository, nil
}

func (s *prTestStore) UpdateGitRepository(_ context.Context, repo *types.GitRepository) error {
	s.repository = repo
	return nil
}

func (s *prTestStore) ListOAuthConnections(_ context.Context, q *store.ListOAuthConnectionsQuery) ([]*types.OAuthConnection, error) {
	var result []*types.OAuthConnection
	for _, conn := range s.oauthConnections {
		if q.UserID != "" && conn.UserID != q.UserID {
			continue
		}
		result = append(result, conn)
	}
	return result, nil
}

func (s *prTestStore) GetOAuthConnection(_ context.Context, id string) (*types.OAuthConnection, error) {
	if s.oauthConnection != nil && s.oauthConnection.ID == id {
		return s.oauthConnection, nil
	}
	for _, conn := range s.oauthConnections {
		if conn.ID == id {
			return conn, nil
		}
	}
	return nil, store.ErrNotFound
}

func newPRTestService(s *prTestStore) *GitRepositoryService {
	return &GitRepositoryService{store: s}
}

func TestGetGitHubClient_UserOAuthTakesPrecedence(t *testing.T) {
	fs := &prTestStore{
		oauthConnections: []*types.OAuthConnection{
			{UserID: "user-a", AccessToken: "token-a", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitHub}},
			{UserID: "user-b", AccessToken: "token-b", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitHub}},
		},
	}
	svc := newPRTestService(fs)
	repo := &types.GitRepository{
		ExternalURL:       "https://github.com/org/repo",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "conn-user-a",
		Password:          "repo-pat",
	}

	// User B should get a client (using user B's token, not repo-level)
	client, err := svc.getGitHubClient(context.Background(), repo, "user-b")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected client, got nil")
	}
}

func TestGetGitHubClient_AgentFallsBackToRepoCreds(t *testing.T) {
	svc := newPRTestService(&prTestStore{})
	repo := &types.GitRepository{
		ExternalURL:  "https://github.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitHub,
		Password:     "repo-pat",
	}

	client, err := svc.getGitHubClient(context.Background(), repo, "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if client == nil {
		t.Fatal("expected client, got nil")
	}
}

func TestGetGitHubClient_UserUsesSharedRepositoryPAT(t *testing.T) {
	svc := newPRTestService(&prTestStore{})
	repo := &types.GitRepository{
		ExternalURL:  "https://github.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitHub,
		Password:     "repo-pat",
	}

	client, err := svc.getGitHubClient(context.Background(), repo, "user-x")
	if err != nil || client == nil {
		t.Fatalf("expected shared repository PAT client, got %v and error %v", client, err)
	}
}

func TestUpdateGitHubPullRequest_UsesActingUserOAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("expected acting user's OAuth token, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	repo := &types.GitRepository{
		ExternalURL:       server.URL + "/org/repo",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "owner-oauth",
		Password:          "incidental-repo-token",
		GitHub:            &types.GitHub{BaseURL: server.URL + "/"},
	}
	svc := newPRTestService(&prTestStore{
		repository: repo,
		oauthConnections: []*types.OAuthConnection{
			{UserID: "user-b", AccessToken: "user-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitHub, AuthURL: server.URL}},
		},
	})

	if err := svc.UpdatePullRequest(context.Background(), "repo-id", 7, "title", "body", "user-b"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestUpdateGitHubPullRequest_RequiresActingUser(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:       "https://github.com/org/repo",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "owner-oauth",
	}
	svc := newPRTestService(&prTestStore{repository: repo})

	err := svc.UpdatePullRequest(context.Background(), "repo-id", 7, "title", "body", "")
	oauthErr, ok := err.(*OAuthRequiredError)
	if !ok || oauthErr.ProviderType != "github" {
		t.Fatalf("expected GitHub OAuthRequiredError, got %T: %v", err, err)
	}
}

func TestUpdateGitHubPullRequest_SharedPATTakesPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer repo-pat" {
			t.Errorf("expected shared repository PAT, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	repo := &types.GitRepository{ExternalURL: server.URL + "/org/repo", ExternalType: types.ExternalRepositoryTypeGitHub,
		GitHub: &types.GitHub{BaseURL: server.URL + "/", PersonalAccessToken: "repo-pat"}}
	fs := &prTestStore{repository: repo, gitConnections: []*types.GitProviderConnection{{
		UserID: "user-x", ProviderType: types.ExternalRepositoryTypeGitHub, BaseURL: server.URL,
		Token: encryptedTestCredential(t, "user-pat"),
	}}}
	for _, userID := range []string{"user-x", ""} {
		if err := newPRTestService(fs).UpdatePullRequest(context.Background(), "repo-id", 7, "title", "body", userID); err != nil {
			t.Fatalf("expected GitHub update to succeed with actor %q: %v", userID, err)
		}
	}
}

func TestValidateUserOAuth_AcceptsSharedGitLabPAT(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:  "https://gitlab.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitLab,
		GitLab:       &types.GitLab{PersonalAccessToken: "repo-token"},
	}
	if err := newPRTestService(&prTestStore{}).ValidateUserOAuth(context.Background(), repo, "user-b"); err != nil {
		t.Fatalf("expected shared GitLab PAT to pass preflight: %v", err)
	}
}

func TestUpdatePullRequest_RejectsProvidersWithoutUserCredentials(t *testing.T) {
	for _, provider := range []types.ExternalRepositoryType{
		types.ExternalRepositoryTypeADO,
		types.ExternalRepositoryTypeBitbucket,
	} {
		t.Run(string(provider), func(t *testing.T) {
			repo := &types.GitRepository{
				ExternalURL:  "https://example.com/org/repo",
				ExternalType: provider,
			}
			svc := newPRTestService(&prTestStore{repository: repo})

			if err := svc.UpdatePullRequest(context.Background(), "repo-id", 7, "title", "body", "user-b"); err == nil {
				t.Fatal("expected unsupported acting-user credential error")
			}
		})
	}
}

func TestCreatePullRequest_RequiresActingUserForEveryProvider(t *testing.T) {
	for _, provider := range []types.ExternalRepositoryType{
		types.ExternalRepositoryTypeGitHub,
		types.ExternalRepositoryTypeGitLab,
		types.ExternalRepositoryTypeADO,
		types.ExternalRepositoryTypeBitbucket,
	} {
		t.Run(string(provider), func(t *testing.T) {
			repoPath := t.TempDir()
			if err := giteagit.InitRepository(context.Background(), repoPath, true, "sha1"); err != nil {
				t.Fatalf("failed to initialize test repository: %v", err)
			}
			repo := &types.GitRepository{
				ID:           "repo-id",
				LocalPath:    repoPath,
				IsExternal:   true,
				ExternalURL:  "https://example.com/org/repo",
				ExternalType: provider,
			}
			svc := newPRTestService(&prTestStore{repository: repo})
			_, err := svc.CreatePullRequest(context.Background(), "repo-id", "title", "body", "feature", "main", "")
			oauthErr, ok := err.(*OAuthRequiredError)
			if !ok || oauthErr.ProviderType != string(provider) {
				t.Fatalf("expected %s acting-user credential error, got %T: %v", provider, err, err)
			}
		})
	}
}

func TestCreatePullRequest_AllowsBlankActorForSharedPAT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer repo-pat" {
			t.Errorf("expected shared repository PAT, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"number":7}`)
	}))
	defer server.Close()

	repoPath := t.TempDir()
	if err := giteagit.InitRepository(context.Background(), repoPath, true, "sha1"); err != nil {
		t.Fatalf("failed to initialize test repository: %v", err)
	}
	repo := &types.GitRepository{
		ID: "repo-id", LocalPath: repoPath, IsExternal: true,
		ExternalURL: server.URL + "/org/repo", ExternalType: types.ExternalRepositoryTypeGitHub,
		GitHub: &types.GitHub{BaseURL: server.URL + "/", PersonalAccessToken: "repo-pat"},
	}
	prID, err := newPRTestService(&prTestStore{repository: repo}).CreatePullRequest(
		context.Background(), repo.ID, "title", "body", "feature", "main", "",
	)
	if err != nil || prID != "7" {
		t.Fatalf("expected PR 7 using shared PAT, got %q and error %v", prID, err)
	}
}

func TestSharedRepositoryPATClassification(t *testing.T) {
	tests := []struct {
		name     string
		repo     *types.GitRepository
		username string
		password string
		shared   bool
	}{
		{name: "GitHub provider PAT", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub, GitHub: &types.GitHub{PersonalAccessToken: "pat"}}, username: "x-access-token", password: "pat", shared: true},
		{name: "GitHub direct PAT", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub, Password: "pat"}, username: "x-access-token", password: "pat", shared: true},
		{name: "GitLab direct PAT", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitLab, Password: "pat"}, username: "oauth2", password: "pat", shared: true},
		{name: "ADO provider PAT", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeADO, AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme", PersonalAccessToken: "pat"}}, username: "PAT", password: "pat", shared: true},
		{name: "Bitbucket app password", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeBitbucket, Bitbucket: &types.Bitbucket{Username: "repo-user", AppPassword: "pat"}}, username: "repo-user", password: "pat", shared: true},
		{name: "saved PAT overrides OAuth ID", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub, OAuthConnectionID: "oauth", GitProviderConnectionID: "pat-connection", GitHub: &types.GitHub{PersonalAccessToken: "pat"}}, username: "x-access-token", password: "pat", shared: true},
		{name: "OAuth with incidental password", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub, OAuthConnectionID: "oauth", Password: "stale"}},
		{name: "GitHub App", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub, GitHub: &types.GitHub{AppID: 1, InstallationID: 2, PrivateKey: "key"}}},
		{name: "ADO service principal", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeADO, AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme", TenantID: "tenant", ClientID: "client", ClientSecret: "secret"}}},
		{name: "ADO legacy credentials without organization", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeADO, Username: "user", Password: "pat"}, username: "user", password: "pat", shared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasSharedRepositoryPAT(tt.repo); got != tt.shared {
				t.Fatalf("expected shared=%t, got %t", tt.shared, got)
			}
			if !tt.shared {
				return
			}
			username, password, _ := repositoryPATCredentials(tt.repo)
			if username != tt.username || password != tt.password {
				t.Fatalf("expected %q / %q, got %q / %q", tt.username, tt.password, username, password)
			}
		})
	}
}

func TestGetCredentialsForRepo_SavedPATOverridesOAuthID(t *testing.T) {
	repo := &types.GitRepository{
		ExternalType:            types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID:       "oauth",
		GitProviderConnectionID: "pat-connection",
		GitHub:                  &types.GitHub{PersonalAccessToken: "repo-pat"},
	}
	svc := newPRTestService(&prTestStore{oauthConnection: &types.OAuthConnection{ID: "oauth", AccessToken: "oauth-token"}})
	username, password := svc.getCredentialsForRepo(context.Background(), repo)
	if username != "x-access-token" || password != "repo-pat" {
		t.Fatalf("expected saved repository PAT, got %q / %q", username, password)
	}
}

func TestValidateUserOAuth_AcceptsMatchingSavedPATs(t *testing.T) {
	tests := []struct {
		name       string
		repo       *types.GitRepository
		connection *types.GitProviderConnection
	}{
		{
			name:       "GitHub Enterprise",
			repo:       &types.GitRepository{ExternalURL: "https://github.example.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitHub},
			connection: &types.GitProviderConnection{BaseURL: "https://github.example.com/"},
		},
		{
			name: "GitLab",
			repo: &types.GitRepository{ExternalURL: "https://gitlab.example.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab,
				GitLab: &types.GitLab{BaseURL: "https://gitlab.example.com/api/v4"}},
			connection: &types.GitProviderConnection{BaseURL: "https://gitlab.example.com/api/v4/"},
		},
		{
			name: "Azure DevOps",
			repo: &types.GitRepository{ExternalURL: "https://dev.azure.com/acme/project/_git/repo", ExternalType: types.ExternalRepositoryTypeADO,
				AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme/"}},
			connection: &types.GitProviderConnection{OrganizationURL: "https://dev.azure.com/acme"},
		},
		{
			name: "Bitbucket",
			repo: &types.GitRepository{ExternalURL: "https://bitbucket.example.com/projects/acme/repos/repo", ExternalType: types.ExternalRepositoryTypeBitbucket,
				Bitbucket: &types.Bitbucket{BaseURL: "https://bitbucket.example.com/"}},
			connection: &types.GitProviderConnection{BaseURL: "https://bitbucket.example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.connection.UserID = "user-x"
			tt.connection.ProviderType = tt.repo.ExternalType
			tt.connection.Token = encryptedTestCredential(t, "user-pat")
			if tt.repo.ExternalType == types.ExternalRepositoryTypeBitbucket {
				tt.connection.AuthUsername = encryptedTestCredential(t, "actor")
			}
			fs := &prTestStore{gitConnections: []*types.GitProviderConnection{tt.connection}}
			if err := newPRTestService(fs).ValidateUserOAuth(context.Background(), tt.repo, "user-x"); err != nil {
				t.Fatalf("expected matching saved PAT to pass preflight: %v", err)
			}
		})
	}
}

func TestValidateUserOAuth_RejectsWrongEndpointAndAmbiguousConnections(t *testing.T) {
	repo := &types.GitRepository{ExternalURL: "https://gitlab.example.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab,
		GitLab: &types.GitLab{BaseURL: "https://gitlab.example.com/api/v4"}}
	wrongEndpoint := &types.GitProviderConnection{UserID: "user-x", ProviderType: types.ExternalRepositoryTypeGitLab,
		BaseURL: "https://other.example.com/api/v4", Token: encryptedTestCredential(t, "pat")}
	if err := newPRTestService(&prTestStore{gitConnections: []*types.GitProviderConnection{wrongEndpoint}}).ValidateUserOAuth(context.Background(), repo, "user-x"); err == nil {
		t.Fatal("expected wrong-endpoint connection to be rejected")
	}
	matching := &types.GitProviderConnection{UserID: "user-x", ProviderType: types.ExternalRepositoryTypeGitLab,
		BaseURL: "https://gitlab.example.com/api/v4", Token: encryptedTestCredential(t, "pat")}
	if err := newPRTestService(&prTestStore{gitConnections: []*types.GitProviderConnection{matching, matching}}).ValidateUserOAuth(context.Background(), repo, "user-x"); err == nil {
		t.Fatal("expected ambiguous connections to be rejected")
	}
}

func TestUpdateBitbucketPullRequest_SharedPATTakesPrecedence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "repo" || password != "repo-pat" {
			t.Errorf("expected shared repository credentials, got %q / %q", username, password)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	repo := &types.GitRepository{ExternalURL: server.URL + "/projects/acme/repos/repo", ExternalType: types.ExternalRepositoryTypeBitbucket,
		Bitbucket: &types.Bitbucket{BaseURL: server.URL, Username: "repo", AppPassword: "repo-pat"}}
	fs := &prTestStore{repository: repo, gitConnections: []*types.GitProviderConnection{{
		UserID: "user-x", ProviderType: types.ExternalRepositoryTypeBitbucket, BaseURL: server.URL,
		Token: encryptedTestCredential(t, "user-pat"), AuthUsername: encryptedTestCredential(t, "actor"),
	}}}
	if err := newPRTestService(fs).UpdatePullRequest(context.Background(), "repo-id", 7, "title", "body", "user-x"); err != nil {
		t.Fatalf("expected Bitbucket update to succeed: %v", err)
	}
}

func TestValidateUserOAuth_AcceptsSharedAzureDevOpsPAT(t *testing.T) {
	repo := &types.GitRepository{ExternalURL: "https://dev.azure.com/acme/project/_git/repo", ExternalType: types.ExternalRepositoryTypeADO,
		AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme", PersonalAccessToken: "repo-pat"}}
	fs := &prTestStore{gitConnections: []*types.GitProviderConnection{{
		UserID: "user-x", ProviderType: types.ExternalRepositoryTypeADO, OrganizationURL: "https://dev.azure.com/acme",
		Token: encryptedTestCredential(t, "user-pat"),
	}}}
	if err := newPRTestService(fs).ValidateUserOAuth(context.Background(), repo, "user-x"); err != nil {
		t.Fatalf("expected shared ADO PAT to pass preflight: %v", err)
	}
}

func TestGetPushCredentialsForRepo_UsesActingUserPATByProvider(t *testing.T) {
	tests := []struct {
		name       string
		repo       *types.GitRepository
		connection *types.GitProviderConnection
		username   string
	}{
		{name: "GitHub", repo: &types.GitRepository{ExternalURL: "https://github.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitHub},
			connection: &types.GitProviderConnection{ProviderType: types.ExternalRepositoryTypeGitHub}, username: "x-access-token"},
		{name: "GitLab", repo: &types.GitRepository{ExternalURL: "https://gitlab.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab},
			connection: &types.GitProviderConnection{ProviderType: types.ExternalRepositoryTypeGitLab}, username: "oauth2"},
		{name: "Azure DevOps", repo: &types.GitRepository{ExternalURL: "https://dev.azure.com/acme/project/_git/repo", ExternalType: types.ExternalRepositoryTypeADO,
			AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme"}},
			connection: &types.GitProviderConnection{ProviderType: types.ExternalRepositoryTypeADO, OrganizationURL: "https://dev.azure.com/acme"}, username: "PAT"},
		{name: "Bitbucket", repo: &types.GitRepository{ExternalURL: "https://bitbucket.org/acme/repo", ExternalType: types.ExternalRepositoryTypeBitbucket},
			connection: &types.GitProviderConnection{ProviderType: types.ExternalRepositoryTypeBitbucket}, username: "actor"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.connection.UserID = "user-x"
			tt.connection.Token = encryptedTestCredential(t, "user-pat")
			if tt.repo.ExternalType == types.ExternalRepositoryTypeBitbucket {
				tt.connection.AuthUsername = encryptedTestCredential(t, "actor")
			}
			username, password, err := newPRTestService(&prTestStore{gitConnections: []*types.GitProviderConnection{tt.connection}}).
				getPushCredentialsForRepo(context.Background(), tt.repo, "user-x")
			if err != nil || username != tt.username || password != "user-pat" {
				t.Fatalf("expected %q / user-pat, got %q / %q and error %v", tt.username, username, password, err)
			}
		})
	}
}

func TestGetPushCredentialsForRepo_SharedPATTakesPrecedenceByProvider(t *testing.T) {
	tests := []struct {
		name     string
		repo     *types.GitRepository
		username string
	}{
		{name: "GitHub", repo: &types.GitRepository{ExternalURL: "https://github.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitHub,
			GitHub: &types.GitHub{PersonalAccessToken: "repo-pat"}}, username: "x-access-token"},
		{name: "GitLab", repo: &types.GitRepository{ExternalURL: "https://gitlab.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab,
			GitLab: &types.GitLab{PersonalAccessToken: "repo-pat"}}, username: "oauth2"},
		{name: "Azure DevOps", repo: &types.GitRepository{ExternalURL: "https://dev.azure.com/acme/project/_git/repo", ExternalType: types.ExternalRepositoryTypeADO,
			AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme", PersonalAccessToken: "repo-pat"}}, username: "PAT"},
		{name: "Bitbucket", repo: &types.GitRepository{ExternalURL: "https://bitbucket.org/acme/repo", ExternalType: types.ExternalRepositoryTypeBitbucket,
			Bitbucket: &types.Bitbucket{Username: "repo-user", AppPassword: "repo-pat"}}, username: "repo-user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oauth := &types.OAuthConnection{UserID: "user-x", AccessToken: "user-oauth", Provider: types.OAuthProvider{Type: OAuthProviderTypeForRepo(tt.repo.ExternalType)}}
			personal := &types.GitProviderConnection{UserID: "user-x", ProviderType: tt.repo.ExternalType, Token: encryptedTestCredential(t, "user-pat")}
			username, password, err := newPRTestService(&prTestStore{oauthConnections: []*types.OAuthConnection{oauth}, gitConnections: []*types.GitProviderConnection{personal}}).
				getPushCredentialsForRepo(context.Background(), tt.repo, "user-x")
			if err != nil || username != tt.username || password != "repo-pat" {
				t.Fatalf("expected %q / repo-pat, got %q / %q and error %v", tt.username, username, password, err)
			}
		})
	}
}

func TestGetPushCredentialsForRepo_OAuthRepoRequiresActingUserConnection(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:       "https://github.com/org/repo",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "owner-oauth",
	}
	svc := newPRTestService(&prTestStore{oauthConnection: &types.OAuthConnection{ID: "owner-oauth", AccessToken: "owner-token"}})
	_, _, err := svc.getPushCredentialsForRepo(context.Background(), repo, "user-x")
	if _, ok := err.(*OAuthRequiredError); !ok {
		t.Fatalf("expected acting-user credential error, got %T: %v", err, err)
	}
}

func TestValidateUserOAuth_DoesNotUseServiceCredentials(t *testing.T) {
	tests := []*types.GitRepository{
		{ExternalURL: "https://github.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitHub,
			GitHub: &types.GitHub{AppID: 1, InstallationID: 2, PrivateKey: "key"}},
		{ExternalURL: "https://dev.azure.com/acme/project/_git/repo", ExternalType: types.ExternalRepositoryTypeADO,
			AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme", TenantID: "tenant", ClientID: "client", ClientSecret: "secret"}},
	}
	for _, repo := range tests {
		t.Run(string(repo.ExternalType), func(t *testing.T) {
			err := newPRTestService(&prTestStore{}).ValidateUserOAuth(context.Background(), repo, "user-x")
			if _, ok := err.(*OAuthRequiredError); !ok {
				t.Fatalf("expected acting-user credential error, got %T: %v", err, err)
			}
		})
	}
}

func TestValidateUserOAuth_ADOLegacyCredentialsRequireOrganizationURL(t *testing.T) {
	repo := &types.GitRepository{ExternalType: types.ExternalRepositoryTypeADO, Username: "user", Password: "pat"}
	err := newPRTestService(&prTestStore{}).ValidateUserOAuth(context.Background(), repo, "user-x")
	if err == nil || err.Error() != "azure devops configuration not found" {
		t.Fatalf("expected missing ADO configuration error, got %v", err)
	}
	username, password := newPRTestService(&prTestStore{}).getCredentialsForRepo(context.Background(), repo)
	if username != "user" || password != "pat" {
		t.Fatalf("expected legacy push credentials, got %q / %q", username, password)
	}
}

func TestSharedRepositoryPATUsedForPushAndPRPreflight(t *testing.T) {
	tests := []struct {
		name string
		repo *types.GitRepository
		want string
	}{
		{name: "provider PAT", repo: &types.GitRepository{ExternalURL: "https://gitlab.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab,
			GitLab: &types.GitLab{PersonalAccessToken: "repo-pat"}}, want: "oauth2"},
		{name: "legacy username and password", repo: &types.GitRepository{ExternalURL: "https://gitlab.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab,
			Username: "repo-user", Password: "repo-pat"}, want: "repo-user"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newPRTestService(&prTestStore{})
			username, password, err := svc.getPushCredentialsForRepo(context.Background(), tt.repo, "user-x")
			if err != nil || username != tt.want || password != "repo-pat" {
				t.Fatalf("expected %q / repo-pat push fallback, got %q / %q and error %v", tt.want, username, password, err)
			}
			if err := svc.ValidateUserOAuth(context.Background(), tt.repo, "user-x"); err != nil {
				t.Fatalf("expected PR credential validation to accept repository PAT: %v", err)
			}
		})
	}
}

func TestSharedRepositoryPATTakesPrecedenceOverBrokenPersonalConnection(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:  "https://gitlab.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitLab,
		GitLab:       &types.GitLab{PersonalAccessToken: "repo-pat"},
	}
	matching := &types.GitProviderConnection{
		UserID: "user-x", ProviderType: types.ExternalRepositoryTypeGitLab,
		Token: encryptedTestCredential(t, "user-pat"),
	}
	t.Run("ambiguous", func(t *testing.T) {
		svc := newPRTestService(&prTestStore{gitConnections: []*types.GitProviderConnection{matching, matching}})
		_, password, err := svc.getPushCredentialsForRepo(context.Background(), repo, "user-x")
		if err != nil || password != "repo-pat" {
			t.Fatalf("expected shared PAT, got %q and error %v", password, err)
		}
	})
	t.Run("invalid token", func(t *testing.T) {
		broken := *matching
		broken.Token = "not-encrypted"
		svc := newPRTestService(&prTestStore{gitConnections: []*types.GitProviderConnection{&broken}})
		_, password, err := svc.getPushCredentialsForRepo(context.Background(), repo, "user-x")
		if err != nil || password != "repo-pat" {
			t.Fatalf("expected shared PAT, got %q and error %v", password, err)
		}
	})
	t.Run("ambiguous OAuth", func(t *testing.T) {
		oauth := &types.OAuthConnection{UserID: "user-x", AccessToken: "oauth-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitLab}}
		svc := newPRTestService(&prTestStore{oauthConnections: []*types.OAuthConnection{oauth, oauth}})
		_, password, err := svc.getPushCredentialsForRepo(context.Background(), repo, "user-x")
		if err != nil || password != "repo-pat" {
			t.Fatalf("expected shared PAT, got %q and error %v", password, err)
		}
	})
}

func TestCreateGitLabMergeRequest_UserOAuthTakesPrecedence(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("expected acting user's bearer token, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `{"id":123}`)
		case r.Method == http.MethodPost:
			fmt.Fprint(w, `{"iid":7}`)
		default:
			http.Error(w, "unexpected request", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	fs := &prTestStore{
		oauthConnections: []*types.OAuthConnection{
			{UserID: "user-b", AccessToken: "user-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeCustom, Name: "Self-hosted GitLab", AuthURL: server.URL}},
		},
		oauthConnection: &types.OAuthConnection{
			ID: "shared-connection", AccessToken: "shared-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitLab},
		},
	}
	repo := &types.GitRepository{
		ExternalURL:       server.URL + "/org/repo",
		ExternalType:      types.ExternalRepositoryTypeGitLab,
		OAuthConnectionID: "shared-connection",
		GitLab: &types.GitLab{
			BaseURL:             server.URL + "/api/v4/",
			PersonalAccessToken: "repo-pat",
		},
	}

	mrID, err := newPRTestService(fs).createGitLabMergeRequest(
		context.Background(), repo, "title", "description", "feature", "main", "user-b",
	)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if mrID != "7" || requestCount != 2 {
		t.Fatalf("expected MR 7 after two API requests, got MR %q and %d requests", mrID, requestCount)
	}
}

func TestCreateGitLabMergeRequest_SharedPATTakesPrecedence(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("Private-Token"); got != "repo-pat" {
			t.Errorf("expected shared repository PAT, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"id":123}`)
		} else {
			fmt.Fprint(w, `{"iid":7}`)
		}
	}))
	defer server.Close()

	repo := &types.GitRepository{
		ExternalURL:  server.URL + "/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitLab,
		GitLab:       &types.GitLab{BaseURL: server.URL + "/api/v4/", PersonalAccessToken: "repo-pat"},
	}
	fs := &prTestStore{gitConnections: []*types.GitProviderConnection{{
		UserID: "user-x", ProviderType: types.ExternalRepositoryTypeGitLab,
		BaseURL: server.URL + "/api/v4", Token: encryptedTestCredential(t, "user-pat"),
	}}}
	if err := newPRTestService(fs).ValidateUserOAuth(context.Background(), repo, "user-x"); err != nil {
		t.Fatalf("preflight rejected acting user PAT: %v", err)
	}
	mrID, err := newPRTestService(fs).createGitLabMergeRequest(context.Background(), repo, "title", "description", "feature", "main", "user-x")
	if err != nil || mrID != "7" || requestCount != 2 {
		t.Fatalf("expected MR 7 after two API requests, got MR %q, %d requests, error %v", mrID, requestCount, err)
	}
}

func TestGetGitLabClient_UserWithoutOAuthOrRepoPATReturnsError(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:  "https://gitlab.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitLab,
	}

	_, err := newPRTestService(&prTestStore{}).getGitLabClient(context.Background(), repo, "user-x")
	oauthErr, ok := err.(*OAuthRequiredError)
	if !ok || oauthErr.ProviderType != "gitlab" {
		t.Fatalf("expected GitLab OAuthRequiredError, got %T: %v", err, err)
	}
}

func TestGetGitLabClient_AgentFallsBackToRepoCredentials(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:  "https://gitlab.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitLab,
		GitLab:       &types.GitLab{PersonalAccessToken: "repo-pat"},
	}

	client, err := newPRTestService(&prTestStore{}).getGitLabClient(context.Background(), repo, "")
	if err != nil || client == nil {
		t.Fatalf("expected repo credential fallback, got client %v and error %v", client, err)
	}
}

func TestValidateUserOAuth_GitLabRequiresMatchingConnection(t *testing.T) {
	fs := &prTestStore{oauthConnections: []*types.OAuthConnection{
		{UserID: "user-x", AccessToken: "github-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitHub}},
	}}
	repo := &types.GitRepository{ExternalURL: "https://gitlab.example.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab}

	err := newPRTestService(fs).ValidateUserOAuth(context.Background(), repo, "user-x")
	oauthErr, ok := err.(*OAuthRequiredError)
	if !ok || oauthErr.ProviderType != "gitlab" {
		t.Fatalf("expected GitLab OAuthRequiredError, got %T: %v", err, err)
	}
}

func TestValidateUserOAuth_AcceptsCustomGitLabProvider(t *testing.T) {
	fs := &prTestStore{oauthConnections: []*types.OAuthConnection{
		{UserID: "user-x", AccessToken: "gitlab-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeCustom, Name: "Self-hosted GitLab", AuthURL: "https://gitlab.example.com/oauth/authorize"}},
	}}
	repo := &types.GitRepository{ExternalURL: "https://gitlab.example.com/org/repo", ExternalType: types.ExternalRepositoryTypeGitLab}

	if err := newPRTestService(fs).ValidateUserOAuth(context.Background(), repo, "user-x"); err != nil {
		t.Fatalf("expected custom GitLab provider to match, got: %v", err)
	}
}
