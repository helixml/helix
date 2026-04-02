package services

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type GitHubClientSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	store *store.MockStore
	svc   *GitRepositoryService
}

func TestGitHubClientSuite(t *testing.T) {
	suite.Run(t, new(GitHubClientSuite))
}

func (s *GitHubClientSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.svc = NewGitRepositoryService(s.store, s.T().TempDir(), "http://localhost:8080", "test", "test@example.com")
}

func (s *GitHubClientSuite) TearDownTest() {
	s.ctrl.Finish()
}

// repo is initialized with User A's OAuthConnectionID.
// When User B (who has their own GitHub OAuth connection) opens a PR,
// getGitHubClient should use User B's connection, not User A's.
func (s *GitHubClientSuite) TestGetGitHubClient_ActingUserTokenUsed() {
	ctx := context.Background()

	userAConnectionID := "conn-user-a"
	userBID := "user-b"
	userBConnectionID := "conn-user-b"
	userBToken := "token-user-b"

	repo := &types.GitRepository{
		ExternalType:    types.ExternalRepositoryTypeGitHub,
		ExternalURL:     "https://github.com/owner/repo",
		OAuthConnectionID: userAConnectionID, // repo initialized by User A
	}

	providerID := "provider-github-uuid"
	userBConnections := []*types.OAuthConnection{
		{
			ID:          userBConnectionID,
			UserID:      userBID,
			ProviderID:  providerID,
			AccessToken: userBToken,
			Provider: types.OAuthProvider{
				ID:   providerID,
				Type: types.OAuthProviderTypeGitHub,
			},
		},
	}

	// Expect ListOAuthConnections to be called with User B's ID
	s.store.EXPECT().
		ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{UserID: userBID}).
		Return(userBConnections, nil)

	// GetOAuthConnection for repo.OAuthConnectionID should NOT be called
	// (no fallback to repo credentials when userID is set)

	client, err := s.svc.getGitHubClient(ctx, repo, userBID)
	s.Require().NoError(err)
	s.Require().NotNil(client)
}

// When the acting user has no GitHub OAuth connection, getGitHubClient must return an error.
func (s *GitHubClientSuite) TestGetGitHubClient_NoUserConnection_ReturnsError() {
	ctx := context.Background()

	userBID := "user-b"

	repo := &types.GitRepository{
		ExternalType:    types.ExternalRepositoryTypeGitHub,
		ExternalURL:     "https://github.com/owner/repo",
		OAuthConnectionID: "conn-user-a",
	}

	// User B has no connections at all
	s.store.EXPECT().
		ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{UserID: userBID}).
		Return([]*types.OAuthConnection{}, nil)

	client, err := s.svc.getGitHubClient(ctx, repo, userBID)
	s.Require().Error(err)
	s.Require().Nil(client)
	s.Contains(err.Error(), "no GitHub OAuth connection found for user")
}

// When userID is empty (background/automated call), fall back to repo-level credentials.
func (s *GitHubClientSuite) TestGetGitHubClient_NoUserID_FallsBackToRepoOAuth() {
	ctx := context.Background()

	repoConnectionID := "conn-repo"
	repoToken := "token-repo-oauth"

	repo := &types.GitRepository{
		ExternalType:    types.ExternalRepositoryTypeGitHub,
		ExternalURL:     "https://github.com/owner/repo",
		OAuthConnectionID: repoConnectionID,
	}

	s.store.EXPECT().
		GetOAuthConnection(ctx, repoConnectionID).
		Return(&types.OAuthConnection{
			ID:          repoConnectionID,
			AccessToken: repoToken,
		}, nil)

	client, err := s.svc.getGitHubClient(ctx, repo, "") // empty userID
	s.Require().NoError(err)
	s.Require().NotNil(client)
}
