package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// Regression tests for the cross-tenant repository listing leak: the list
// endpoints returned every tenant's repositories when called without an org
// or project filter, or with another user's owner_id.
type GitRepositoryListAuthzSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
	user   *types.User
}

func TestGitRepositoryListAuthzSuite(t *testing.T) {
	suite.Run(t, new(GitRepositoryListAuthzSuite))
}

func (s *GitRepositoryListAuthzSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Store: s.store}
	s.user = &types.User{ID: "usr_bob"}
}

func (s *GitRepositoryListAuthzSuite) do(handler http.HandlerFunc, user *types.User, url string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(setTestRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func (s *GitRepositoryListAuthzSuite) repoIDs(rr *httptest.ResponseRecorder) []string {
	var repos []*types.GitRepository
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &repos))
	ids := make([]string, 0, len(repos))
	for _, r := range repos {
		ids = append(ids, r.ID)
	}
	return ids
}

func (s *GitRepositoryListAuthzSuite) TestListUnfilteredIsConfinedToCaller() {
	s.store.EXPECT().
		ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{OwnerID: s.user.ID}).
		Return([]*types.GitRepository{{ID: "repo_own", OwnerID: s.user.ID}}, nil)

	rr := s.do(s.server.listGitRepositories, s.user, "/api/v1/git/repositories")

	s.Equal(http.StatusOK, rr.Code)
	s.Equal([]string{"repo_own"}, s.repoIDs(rr))
}

func (s *GitRepositoryListAuthzSuite) TestListOtherOwnerIsForbidden() {
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Times(0)

	rr := s.do(s.server.listGitRepositories, s.user, "/api/v1/git/repositories?owner_id=usr_alice")

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *GitRepositoryListAuthzSuite) TestListForeignOrgIsForbidden() {
	s.store.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_a"}).
		Return(&types.Organization{ID: "org_a"}, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Times(0)

	rr := s.do(s.server.listGitRepositories, s.user, "/api/v1/git/repositories?organization_id=org_a")

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *GitRepositoryListAuthzSuite) TestListAdminIsUnscoped() {
	admin := &types.User{ID: "usr_admin", Admin: true}
	s.store.EXPECT().
		ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{}).
		Return([]*types.GitRepository{{ID: "repo_a", OwnerID: "usr_alice", OrganizationID: "org_a"}}, nil)

	rr := s.do(s.server.listGitRepositories, admin, "/api/v1/git/repositories")

	s.Equal(http.StatusOK, rr.Code)
	s.Equal([]string{"repo_a"}, s.repoIDs(rr))
}

func (s *GitRepositoryListAuthzSuite) TestWithoutProjectsUnfilteredIsConfinedToCaller() {
	s.store.EXPECT().ListReposWithoutProjects(gomock.Any(), "", s.user.ID).
		Return([]*types.GitRepository{{ID: "repo_own", OwnerID: s.user.ID}}, nil)

	rr := s.do(s.server.listReposWithoutProjects, s.user, "/api/v1/repositories/without-projects")

	s.Equal(http.StatusOK, rr.Code)
	s.Equal([]string{"repo_own"}, s.repoIDs(rr))
}

func (s *GitRepositoryListAuthzSuite) TestWithoutProjectsForeignOrgIsForbidden() {
	s.store.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_a"}).
		Return(&types.Organization{ID: "org_a"}, nil)
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.store.EXPECT().ListReposWithoutProjects(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	rr := s.do(s.server.listReposWithoutProjects, s.user, "/api/v1/repositories/without-projects?organization_id=org_a")

	s.Equal(http.StatusForbidden, rr.Code)
}
