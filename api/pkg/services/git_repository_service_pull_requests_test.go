package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type prTestStore struct {
	store.Store
	oauthConnections []*types.OAuthConnection
	oauthConnection  *types.OAuthConnection
	gitConnections   []*types.GitProviderConnection
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

func TestGetGitHubClient_UserWithoutOAuth_ReturnsError(t *testing.T) {
	svc := newPRTestService(&prTestStore{})
	repo := &types.GitRepository{
		ExternalURL:  "https://github.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitHub,
		Password:     "repo-pat",
	}

	// User has no OAuth connection -- should return OAuthRequiredError, NOT fall back
	_, err := svc.getGitHubClient(context.Background(), repo, "user-x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	oauthErr, ok := err.(*OAuthRequiredError)
	if !ok {
		t.Fatalf("expected *OAuthRequiredError, got %T: %v", err, err)
	}
	if oauthErr.ProviderType != "github" {
		t.Fatalf("expected provider_type 'github', got '%s'", oauthErr.ProviderType)
	}
}

func TestUpdateGitHubPullRequest_UsesActingUserOAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer actor-token" {
			t.Errorf("expected acting user token, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	repo := &types.GitRepository{
		ExternalURL:       server.URL + "/org/repo",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "owner-connection",
		Password:          "owner-token",
		GitHub:            &types.GitHub{BaseURL: server.URL + "/"},
	}
	service := newPRTestService(&prTestStore{
		oauthConnections: []*types.OAuthConnection{{
			UserID:      "actor",
			AccessToken: "actor-token",
			Provider: types.OAuthProvider{
				Type:    types.OAuthProviderTypeCustom,
				Name:    "GitHub Enterprise",
				AuthURL: server.URL + "/oauth/authorize",
			},
		}},
	})

	if err := service.updateGitHubPullRequest(context.Background(), repo, 7, "title", "body", "actor"); err != nil {
		t.Fatal(err)
	}
}

func TestPullRequestUpdateCredentials_SharedPATTakesPrecedence(t *testing.T) {
	for _, repo := range []*types.GitRepository{
		{ExternalType: types.ExternalRepositoryTypeGitHub, OAuthConnectionID: "oauth", GitProviderConnectionID: "pat", GitHub: &types.GitHub{PersonalAccessToken: "repo-token"}},
		{ExternalType: types.ExternalRepositoryTypeGitLab, OAuthConnectionID: "oauth", GitProviderConnectionID: "pat", GitLab: &types.GitLab{PersonalAccessToken: "repo-token"}},
		{ExternalType: types.ExternalRepositoryTypeADO, AzureDevOps: &types.AzureDevOps{PersonalAccessToken: "repo-token"}},
		{ExternalType: types.ExternalRepositoryTypeBitbucket, Bitbucket: &types.Bitbucket{Username: "repo", AppPassword: "repo-token"}},
	} {
		t.Run(string(repo.ExternalType), func(t *testing.T) {
			_, token, oauth, err := newPRTestService(&prTestStore{}).pullRequestUpdateCredentials(context.Background(), repo, "actor")
			if err != nil || token != "repo-token" || oauth {
				t.Fatalf("got token %q, oauth %t, error %v", token, oauth, err)
			}
		})
	}
}

func TestPullRequestUpdateCredentials_DoesNotUseRepositoryOAuth(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:       "https://github.com/org/repo",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "owner-connection",
		Password:          "owner-token",
	}
	_, _, _, err := newPRTestService(&prTestStore{}).pullRequestUpdateCredentials(context.Background(), repo, "actor")
	if _, ok := err.(*OAuthRequiredError); !ok {
		t.Fatalf("expected acting-user credential error, got %T: %v", err, err)
	}
}

func TestPullRequestUpdateCredentials_UsesActingUserPAT(t *testing.T) {
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	encrypt := func(value string) string {
		encrypted, err := crypto.EncryptAES256GCM([]byte(value), key)
		if err != nil {
			t.Fatal(err)
		}
		return encrypted
	}
	tests := []struct {
		name       string
		repo       *types.GitRepository
		connection *types.GitProviderConnection
		username   string
	}{
		{
			name: "Azure DevOps",
			repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeADO,
				AzureDevOps: &types.AzureDevOps{OrganizationURL: "https://dev.azure.com/acme"}},
			connection: &types.GitProviderConnection{UserID: "actor", ProviderType: types.ExternalRepositoryTypeADO,
				OrganizationURL: "https://dev.azure.com/acme", Token: encrypt("actor-token")},
		},
		{
			name: "Bitbucket",
			repo: &types.GitRepository{ExternalURL: "https://bitbucket.org/acme/repo", ExternalType: types.ExternalRepositoryTypeBitbucket},
			connection: &types.GitProviderConnection{UserID: "actor", ProviderType: types.ExternalRepositoryTypeBitbucket,
				Token: encrypt("actor-token"), AuthUsername: encrypt("actor-name")},
			username: "actor-name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, token, oauth, err := newPRTestService(&prTestStore{gitConnections: []*types.GitProviderConnection{tt.connection}}).
				pullRequestUpdateCredentials(context.Background(), tt.repo, "actor")
			if err != nil || username != tt.username || token != "actor-token" || oauth {
				t.Fatalf("got %q / %q / oauth=%t / err=%v", username, token, oauth, err)
			}
		})
	}
}

func TestTaskPRUserID(t *testing.T) {
	if got := taskPRUserID(&types.SpecTask{CreatedBy: "creator"}); got != "creator" {
		t.Fatalf("expected creator fallback, got %q", got)
	}
	if got := taskPRUserID(&types.SpecTask{CreatedBy: "creator", ImplementationApprovedBy: "approver"}); got != "approver" {
		t.Fatalf("expected approver, got %q", got)
	}
}

func TestGetCredentialsForRepo_GitLabUserOAuthTakesPrecedence(t *testing.T) {
	fs := &prTestStore{
		oauthConnections: []*types.OAuthConnection{
			{ID: "user-connection", UserID: "user-b", AccessToken: "user-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitLab}},
		},
		oauthConnection: &types.OAuthConnection{
			ID: "shared-connection", AccessToken: "shared-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitLab},
		},
	}
	repo := &types.GitRepository{
		ExternalType:      types.ExternalRepositoryTypeGitLab,
		OAuthConnectionID: "shared-connection",
		GitLab:            &types.GitLab{PersonalAccessToken: "repo-pat"},
	}

	username, password := newPRTestService(fs).getCredentialsForRepo(context.Background(), repo, "user-b")

	if username != "oauth2" || password != "user-token" {
		t.Fatalf("expected acting GitLab OAuth credentials, got %q / %q", username, password)
	}
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
			{UserID: "user-b", AccessToken: "user-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeCustom, Name: "Self-hosted GitLab"}},
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

func TestCreateGitLabMergeRequest_UserWithoutOAuthFallsBackToPAT(t *testing.T) {
	tests := []struct {
		name     string
		password string
		pat      string
	}{
		{name: "provider PAT", pat: "repo-pat"},
		{name: "legacy password", password: "repo-pat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				if got := r.Header.Get("Private-Token"); got != "repo-pat" {
					t.Errorf("expected repository PAT, got %q", got)
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

			repo := &types.GitRepository{
				ExternalURL:  server.URL + "/org/repo",
				ExternalType: types.ExternalRepositoryTypeGitLab,
				Password:     tt.password,
				GitLab: &types.GitLab{
					BaseURL:             server.URL + "/api/v4/",
					PersonalAccessToken: tt.pat,
				},
			}

			mrID, err := newPRTestService(&prTestStore{}).createGitLabMergeRequest(
				context.Background(), repo, "title", "description", "feature", "main", "user-x",
			)
			if err != nil {
				t.Fatalf("expected repository PAT fallback, got: %v", err)
			}
			if mrID != "7" || requestCount != 2 {
				t.Fatalf("expected MR 7 after two API requests, got MR %q and %d requests", mrID, requestCount)
			}
		})
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
	repo := &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitLab}

	err := newPRTestService(fs).ValidateUserOAuth(context.Background(), repo, "user-x")
	oauthErr, ok := err.(*OAuthRequiredError)
	if !ok || oauthErr.ProviderType != "gitlab" {
		t.Fatalf("expected GitLab OAuthRequiredError, got %T: %v", err, err)
	}
}

func TestValidateUserOAuth_AcceptsCustomGitLabProvider(t *testing.T) {
	fs := &prTestStore{oauthConnections: []*types.OAuthConnection{
		{UserID: "user-x", AccessToken: "gitlab-token", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeCustom, Name: "Self-hosted GitLab"}},
	}}
	repo := &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitLab}

	if err := newPRTestService(fs).ValidateUserOAuth(context.Background(), repo, "user-x"); err != nil {
		t.Fatalf("expected custom GitLab provider to match, got: %v", err)
	}
}
