package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type HasReadAccessSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockStore   *store.MockStore
	repoService *GitRepositoryService
	tmpDir      string
}

func TestHasReadAccessSuite(t *testing.T) {
	suite.Run(t, new(HasReadAccessSuite))
}

func (s *HasReadAccessSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)

	// updateRepositoryFromGit calls UpdateGitRepository after reading repo info
	s.mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	tmpDir, err := os.MkdirTemp("", "git-http-server-test-*")
	s.Require().NoError(err)
	s.tmpDir = tmpDir

	s.repoService = &GitRepositoryService{
		store:       s.mockStore,
		gitRepoBase: filepath.Join(tmpDir, "repos"),
	}
}

func (s *HasReadAccessSuite) TearDownTest() {
	os.RemoveAll(s.tmpDir)
}

// createBareRepo creates a bare git repo on disk and returns its path.
func (s *HasReadAccessSuite) createBareRepo(name string) string {
	repoPath := filepath.Join(s.tmpDir, name+".git")
	cmd := exec.Command("git", "init", "--bare", repoPath)
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, "git init --bare failed: %s", string(out))
	return repoPath
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
	repoPath := s.createBareRepo("repo1")

	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return fmt.Errorf("should not be called for owner")
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:        "repo1",
		OwnerID:   "user1",
		LocalPath: repoPath,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo1").Return(repo, nil)

	user := &types.User{ID: "user1"}
	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.True(result)
}

func (s *HasReadAccessSuite) TestNonOwner_AuthorizeFnAllows() {
	repoPath := s.createBareRepo("repo2")

	var capturedAction types.Action
	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		capturedAction = action
		return nil
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:        "repo2",
		OwnerID:   "owner1",
		LocalPath: repoPath,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo2").Return(repo, nil)

	user := &types.User{ID: "user2"}
	result := server.hasReadAccess(context.Background(), user, "repo2")
	s.True(result)
	s.Equal(types.ActionGet, capturedAction, "hasReadAccess should use ActionGet")
}

func (s *HasReadAccessSuite) TestNonOwner_AuthorizeFnDenies() {
	repoPath := s.createBareRepo("repo3")

	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return fmt.Errorf("not authorized")
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:        "repo3",
		OwnerID:   "owner1",
		LocalPath: repoPath,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo3").Return(repo, nil)

	user := &types.User{ID: "user2"}
	result := server.hasReadAccess(context.Background(), user, "repo3")
	s.False(result)
}
