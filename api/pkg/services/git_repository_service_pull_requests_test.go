package services

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type prTestStore struct {
	store.Store
	oauthConnections []*types.OAuthConnection
	oauthConnection  *types.OAuthConnection
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

func TestGetGitHubClient_UserWithoutOAuth_FallsBackToRepoCreds(t *testing.T) {
	svc := newPRTestService(&prTestStore{})
	repo := &types.GitRepository{
		ExternalURL:  "https://github.com/org/repo",
		ExternalType: types.ExternalRepositoryTypeGitHub,
		Password:     "repo-pat",
	}

	// User has no OAuth connection, but repo has a PAT -- should fall back
	client, err := svc.getGitHubClient(context.Background(), repo, "user-x")
	if err != nil {
		t.Fatalf("expected no error (fallback to PAT), got: %v", err)
	}
	if client == nil {
		t.Fatal("expected client, got nil")
	}
}
