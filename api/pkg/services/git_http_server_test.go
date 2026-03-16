package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type HasReadAccessSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockStore  *store.MockStore
	server     *GitHTTPServer
	repoService *GitRepositoryService
}

func TestHasReadAccessSuite(t *testing.T) {
	suite.Run(t, new(HasReadAccessSuite))
}

func (s *HasReadAccessSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)

	s.repoService = &GitRepositoryService{
		store: s.mockStore,
	}
}

func (s *HasReadAccessSuite) newServer(authFn AuthorizationToRepositoryFunc) *GitHTTPServer {
	return &GitHTTPServer{
		store:          s.mockStore,
		gitRepoService: s.repoService,
		authorizeFn:    authFn,
	}
}

func (s *HasReadAccessSuite) TestNoAuthorizeFn_AllowsAccess() {
	server := s.newServer(nil)
	user := &types.User{ID: "user1"}

	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.True(result)
}

func (s *HasReadAccessSuite) TestRepoNotFound_DeniesAccess() {
	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return nil
	}
	server := s.newServer(authFn)

	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo1").Return(nil, fmt.Errorf("not found"))

	user := &types.User{ID: "user1"}
	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.False(result)
}

func (s *HasReadAccessSuite) TestOwner_AllowsAccess() {
	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return fmt.Errorf("should not be called")
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:         "repo1",
		OwnerID:    "user1",
		IsExternal: true,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo1").Return(repo, nil)

	user := &types.User{ID: "user1"}
	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.True(result)
}

func (s *HasReadAccessSuite) TestNonOwner_AuthorizeFnAllows() {
	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		s.Equal(types.ActionGet, action)
		return nil
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:         "repo1",
		OwnerID:    "owner1",
		IsExternal: true,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo1").Return(repo, nil)

	user := &types.User{ID: "user2"}
	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.True(result)
}

func (s *HasReadAccessSuite) TestNonOwner_AuthorizeFnDenies() {
	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return fmt.Errorf("not authorized")
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:         "repo1",
		OwnerID:    "owner1",
		IsExternal: true,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo1").Return(repo, nil)

	user := &types.User{ID: "user2"}
	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.False(result)
}
