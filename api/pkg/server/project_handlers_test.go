package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ProjectRepositoryHandlersSuite struct {
	suite.Suite

	ctrl    *gomock.Controller
	store   *store.MockStore
	authCtx context.Context
	userID  string
	server  *HelixAPIServer
}

func TestProjectRepositoryHandlersSuite(t *testing.T) {
	suite.Run(t, new(ProjectRepositoryHandlersSuite))
}

func (s *ProjectRepositoryHandlersSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.userID = "user-test"
	s.authCtx = setRequestUser(context.Background(), types.User{
		ID:    s.userID,
		Email: "test@example.com",
	})
	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.store,
	}
}

// makeProject returns a personal project owned by the test user.
func (s *ProjectRepositoryHandlersSuite) makeProject(id, defaultRepoID string) *types.Project {
	return &types.Project{
		ID:            id,
		UserID:        s.userID,
		OrganizationID: "",
		DefaultRepoID: defaultRepoID,
	}
}

// makeRepo returns a repo owned by the test user.
func (s *ProjectRepositoryHandlersSuite) makeRepo(id string) *types.GitRepository {
	return &types.GitRepository{
		ID:             id,
		OwnerID:        s.userID,
		OrganizationID: "",
	}
}

func (s *ProjectRepositoryHandlersSuite) attachRequest(projectID, repoID string) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/repositories/"+repoID+"/attach", http.NoBody)
	req = req.WithContext(s.authCtx)
	return mux.SetURLVars(req, map[string]string{"id": projectID, "repo_id": repoID})
}

func (s *ProjectRepositoryHandlersSuite) detachRequest(projectID, repoID string) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/"+projectID+"/repositories/"+repoID+"/detach", http.NoBody)
	req = req.WithContext(s.authCtx)
	return mux.SetURLVars(req, map[string]string{"id": projectID, "repo_id": repoID})
}

// ---------------------------------------------------------------------------
// Attach tests
// ---------------------------------------------------------------------------

// TestAttachRepo_SetsDefaultWhenEmpty: project has no default repo → attaching should set it.
func (s *ProjectRepositoryHandlersSuite) TestAttachRepo_SetsDefaultWhenEmpty() {
	project := s.makeProject("proj-1", "") // DefaultRepoID is empty
	repo := s.makeRepo("repo-1")

	s.store.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	s.store.EXPECT().GetGitRepository(gomock.Any(), repo.ID).Return(repo, nil)
	s.store.EXPECT().AttachRepositoryToProject(gomock.Any(), project.ID, repo.ID).Return(nil)
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{repo}, nil)
	s.store.EXPECT().SetProjectPrimaryRepository(gomock.Any(), project.ID, repo.ID).Return(nil)

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.attachRepositoryToProject(rec, s.attachRequest(project.ID, repo.ID))
	s.Nil(httpErr)
	s.NotNil(resp)
}

// TestAttachRepo_SetsDefaultWhenStale: project default_repo_id points to a repo not in the
// attached set → attaching the new repo should update the default.
func (s *ProjectRepositoryHandlersSuite) TestAttachRepo_SetsDefaultWhenStale() {
	project := s.makeProject("proj-2", "old-repo-no-longer-attached")
	repo := s.makeRepo("repo-2")

	s.store.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	s.store.EXPECT().GetGitRepository(gomock.Any(), repo.ID).Return(repo, nil)
	s.store.EXPECT().AttachRepositoryToProject(gomock.Any(), project.ID, repo.ID).Return(nil)
	// After attach, only repo-2 is in the project — old repo is gone.
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{repo}, nil)
	s.store.EXPECT().SetProjectPrimaryRepository(gomock.Any(), project.ID, repo.ID).Return(nil)

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.attachRepositoryToProject(rec, s.attachRequest(project.ID, repo.ID))
	s.Nil(httpErr)
	s.NotNil(resp)
}

// TestAttachRepo_KeepsDefaultWhenValid: project already has a valid default repo and it is still
// attached → attaching a second repo must NOT call SetProjectPrimaryRepository.
func (s *ProjectRepositoryHandlersSuite) TestAttachRepo_KeepsDefaultWhenValid() {
	existingRepo := s.makeRepo("repo-existing")
	project := s.makeProject("proj-3", existingRepo.ID) // valid existing default
	newRepo := s.makeRepo("repo-new")

	s.store.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	s.store.EXPECT().GetGitRepository(gomock.Any(), newRepo.ID).Return(newRepo, nil)
	s.store.EXPECT().AttachRepositoryToProject(gomock.Any(), project.ID, newRepo.ID).Return(nil)
	// Both repos present after attach — existing default is still valid.
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{existingRepo, newRepo}, nil)
	// SetProjectPrimaryRepository must NOT be called.

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.attachRepositoryToProject(rec, s.attachRequest(project.ID, newRepo.ID))
	s.Nil(httpErr)
	s.NotNil(resp)
}

// ---------------------------------------------------------------------------
// Detach tests
// ---------------------------------------------------------------------------

// TestDetachRepo_UpdatesDefaultToRemainingRepo: detach the default repo when another repo is
// still attached → default should update to the remaining repo.
func (s *ProjectRepositoryHandlersSuite) TestDetachRepo_UpdatesDefaultToRemainingRepo() {
	remainingRepo := s.makeRepo("repo-remaining")
	defaultRepo := s.makeRepo("repo-default")
	project := s.makeProject("proj-4", defaultRepo.ID)

	s.store.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	// First ListGitRepositories call: verify that defaultRepo is attached.
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{defaultRepo, remainingRepo}, nil)
	s.store.EXPECT().DetachRepositoryFromProject(gomock.Any(), project.ID, defaultRepo.ID).Return(nil)
	// Second ListGitRepositories call: get remaining repos after detach.
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{remainingRepo}, nil)
	s.store.EXPECT().SetProjectPrimaryRepository(gomock.Any(), project.ID, remainingRepo.ID).Return(nil)

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.detachRepositoryFromProject(rec, s.detachRequest(project.ID, defaultRepo.ID))
	s.Nil(httpErr)
	s.NotNil(resp)
}

// TestDetachRepo_ClearsDefaultWhenLastRepo: detach the default repo when it is the only one →
// default_repo_id should be cleared to "".
func (s *ProjectRepositoryHandlersSuite) TestDetachRepo_ClearsDefaultWhenLastRepo() {
	defaultRepo := s.makeRepo("repo-only")
	project := s.makeProject("proj-5", defaultRepo.ID)

	s.store.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{defaultRepo}, nil)
	s.store.EXPECT().DetachRepositoryFromProject(gomock.Any(), project.ID, defaultRepo.ID).Return(nil)
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{}, nil)
	s.store.EXPECT().SetProjectPrimaryRepository(gomock.Any(), project.ID, "").Return(nil)

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.detachRepositoryFromProject(rec, s.detachRequest(project.ID, defaultRepo.ID))
	s.Nil(httpErr)
	s.NotNil(resp)
}

// TestDetachRepo_KeepsDefaultWhenNotDefault: detach a repo that is NOT the default →
// SetProjectPrimaryRepository must NOT be called.
func (s *ProjectRepositoryHandlersSuite) TestDetachRepo_KeepsDefaultWhenNotDefault() {
	defaultRepo := s.makeRepo("repo-default")
	otherRepo := s.makeRepo("repo-other")
	project := s.makeProject("proj-6", defaultRepo.ID)

	s.store.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	s.store.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).
		Return([]*types.GitRepository{defaultRepo, otherRepo}, nil)
	s.store.EXPECT().DetachRepositoryFromProject(gomock.Any(), project.ID, otherRepo.ID).Return(nil)
	// SetProjectPrimaryRepository must NOT be called.

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.detachRepositoryFromProject(rec, s.detachRequest(project.ID, otherRepo.ID))
	s.Nil(httpErr)
	s.NotNil(resp)
}
