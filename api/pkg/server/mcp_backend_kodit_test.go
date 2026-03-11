package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// ---------------------------------------------------------------------------
// Backend-level tests
// ---------------------------------------------------------------------------

func TestKoditMCPBackend_Disabled(t *testing.T) {
	backend := NewKoditMCPBackend(nil, false, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/mcp/kodit/", nil)
	backend.ServeHTTP(rec, req, &types.User{ID: "u"})
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestKoditMCPBackend_RequiresSessionID(t *testing.T) {
	// An enabled backend with nil kodit client should still be disabled
	backend := NewKoditMCPBackend(nil, true, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/mcp/kodit/", nil)
	backend.ServeHTTP(rec, req, &types.User{ID: "u"})
	// nil koditClient makes it disabled
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

// ---------------------------------------------------------------------------
// Repo resolver tests
// ---------------------------------------------------------------------------

type KoditRepoResolverSuite struct {
	suite.Suite
	ctx       context.Context
	ctrl      *gomock.Controller
	mockStore *store.MockStore
}

func TestKoditRepoResolverSuite(t *testing.T) {
	suite.Run(t, new(KoditRepoResolverSuite))
}

func (s *KoditRepoResolverSuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
}

func (s *KoditRepoResolverSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *KoditRepoResolverSuite) TestProjectScopedRepos() {
	sessionID := "ses-123"
	projectID := "proj-abc"

	s.mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID:        sessionID,
		ProjectID: projectID,
	}, nil)

	s.mockStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{
		ProjectID: projectID,
	}).Return([]*types.GitRepository{
		{
			ID:            "repo-1",
			KoditIndexing: true,
			Metadata:      map[string]interface{}{"kodit_repo_id": int64(10)},
		},
		{
			ID:            "repo-2",
			KoditIndexing: true,
			Metadata:      map[string]interface{}{"kodit_repo_id": int64(20)},
		},
		{
			ID:            "repo-3",
			KoditIndexing: false, // not indexed
			Metadata:      map[string]interface{}{"kodit_repo_id": int64(30)},
		},
		{
			ID:            "repo-4",
			KoditIndexing: true,
			Metadata:      nil, // no metadata
		},
	}, nil)

	user := &types.User{ID: "user-1", OrganizationID: "org-1"}
	scope, err := resolveKoditRepoScope(s.ctx, s.mockStore, sessionID, user)
	s.Require().NoError(err)

	// Only repo-1 and repo-2 should be allowed (repo-3 not indexed, repo-4 no metadata)
	s.Equal(2, len(scope.idSlice))
	s.True(scope.repoIDs[10])
	s.True(scope.repoIDs[20])
	s.False(scope.repoIDs[30])
}

func (s *KoditRepoResolverSuite) TestOrgFallbackWhenNoProject() {
	sessionID := "ses-456"
	orgID := "org-1"

	s.mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID: sessionID,
		// ProjectID is empty
	}, nil)

	s.mockStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{
		OrganizationID: orgID,
	}).Return([]*types.GitRepository{
		{
			ID:            "repo-1",
			KoditIndexing: true,
			Metadata:      map[string]interface{}{"kodit_repo_id": int64(100)},
		},
	}, nil)

	user := &types.User{ID: "user-1", OrganizationID: orgID}
	scope, err := resolveKoditRepoScope(s.ctx, s.mockStore, sessionID, user)
	s.Require().NoError(err)

	s.Equal(1, len(scope.idSlice))
	s.True(scope.repoIDs[100])
}

func (s *KoditRepoResolverSuite) TestOwnerFallbackWhenNoOrg() {
	sessionID := "ses-789"
	userID := "user-solo"

	s.mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID: sessionID,
	}, nil)

	s.mockStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{
		OwnerID: userID,
	}).Return([]*types.GitRepository{
		{
			ID:            "repo-x",
			KoditIndexing: true,
			Metadata:      map[string]interface{}{"kodit_repo_id": float64(42)}, // JSON number format
		},
	}, nil)

	user := &types.User{ID: userID}
	scope, err := resolveKoditRepoScope(s.ctx, s.mockStore, sessionID, user)
	s.Require().NoError(err)

	s.Equal(1, len(scope.idSlice))
	s.True(scope.repoIDs[42])
}

func (s *KoditRepoResolverSuite) TestSkipsReposWithoutKoditRepoID() {
	sessionID := "ses-noid"

	s.mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID:        sessionID,
		ProjectID: "proj-1",
	}, nil)

	s.mockStore.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{
		{
			ID:            "repo-no-id",
			KoditIndexing: true,
			Metadata:      map[string]interface{}{"some_key": "value"}, // no kodit_repo_id
		},
		{
			ID:            "repo-zero-id",
			KoditIndexing: true,
			Metadata:      map[string]interface{}{"kodit_repo_id": int64(0)},
		},
	}, nil)

	user := &types.User{ID: "user-1", OrganizationID: "org-1"}
	scope, err := resolveKoditRepoScope(s.ctx, s.mockStore, sessionID, user)
	s.Require().NoError(err)

	s.Equal(0, len(scope.idSlice))
}
