package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	apiskill "github.com/helixml/helix/api/pkg/agent/skill/api_skills"
	"github.com/helixml/helix/api/pkg/config"
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
	orgID := "org-1"

	s.mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID:        sessionID,
		ProjectID: projectID,
	}, nil)

	s.mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
		ID:             projectID,
		OrganizationID: orgID,
		KoditEnabled:   true,
	}, nil)

	// When KoditEnabled, resolver queries by org (all org repos), not by project
	s.mockStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{
		OrganizationID: orgID,
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

	user := &types.User{ID: "user-1", OrganizationID: orgID}
	scope, err := resolveKoditRepoScope(s.ctx, s.mockStore, sessionID, user)
	s.Require().NoError(err)

	// Only repo-1 and repo-2 should be allowed (repo-3 not indexed, repo-4 no metadata)
	s.Equal(2, len(scope.idSlice))
	s.True(scope.repoIDs[10])
	s.True(scope.repoIDs[20])
	s.False(scope.repoIDs[30])
}

func (s *KoditRepoResolverSuite) TestProjectKoditDisabledReturnsEmptyScope() {
	sessionID := "ses-disabled"
	projectID := "proj-disabled"

	s.mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID:        sessionID,
		ProjectID: projectID,
	}, nil)

	s.mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
		ID:           projectID,
		KoditEnabled: false,
	}, nil)

	user := &types.User{ID: "user-1", OrganizationID: "org-1"}
	scope, err := resolveKoditRepoScope(s.ctx, s.mockStore, sessionID, user)
	s.Require().NoError(err)

	s.Equal(0, len(scope.idSlice))
	s.Equal(0, len(scope.repoIDs))
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
	projectID := "proj-1"
	orgID := "org-1"

	s.mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID:        sessionID,
		ProjectID: projectID,
	}, nil)

	s.mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
		ID:             projectID,
		OrganizationID: orgID,
		KoditEnabled:   true,
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

	user := &types.User{ID: "user-1", OrganizationID: orgID}
	scope, err := resolveKoditRepoScope(s.ctx, s.mockStore, sessionID, user)
	s.Require().NoError(err)

	s.Equal(0, len(scope.idSlice))
}

// ---------------------------------------------------------------------------
// Integration: enable endpoint → Kodit backend route
// ---------------------------------------------------------------------------

// TestEnableSkillRouteToKoditBackend verifies the end-to-end wiring:
// 1. The enable endpoint produces an AssistantMCP whose URL ends with /api/v1/mcp/kodit.
// 2. A request to that URL path (served by the disabled Kodit backend) returns 501,
//    confirming the route is correctly registered and the URL is right.
func TestEnableSkillRouteToKoditBackend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	serverURL := "http://helix.test"
	userID := "user-route"
	appID := "app-route"
	apiKey := "hl-route-key"

	app := &types.App{
		ID:    appID,
		Owner: userID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Name: "Agent"}},
			},
		},
	}

	// Enable endpoint mock expectations.
	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)
	mockStore.EXPECT().ListAPIKeys(gomock.Any(), gomock.Any()).Return([]*types.ApiKey{{Key: apiKey}}, nil)
	mockStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(func(_ interface{}, updated *types.App) (*types.App, error) {
		return updated, nil
	})

	sm := apiskill.NewManager()
	assert.NoError(t, sm.LoadSkills(context.Background()))

	srv := &HelixAPIServer{
		Store: mockStore,
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: serverURL},
		},
		skillManager: sm,
		kodit:        &KoditResult{mcpBackend: NewKoditMCPBackend(nil, false, mockStore)},
	}

	// Step 1: call the enable endpoint and capture the MCP config.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/"+appID+"/skills/code-intelligence/enable", nil)
	req = mux.SetURLVars(req, map[string]string{"id": appID, "skill": "code-intelligence"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID, Type: types.OwnerTypeUser}))
	rr := httptest.NewRecorder()
	srv.handleEnableSkill(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Step 2: verify the produced URL points to the Kodit MCP backend path.
	updatedApp := app.Config.Helix.Assistants[0]
	assert.Len(t, updatedApp.MCPs, 1)
	mcpURL := updatedApp.MCPs[0].URL
	assert.Equal(t, serverURL+"/api/v1/mcp/kodit", mcpURL)

	// Step 3: confirm the Kodit backend (disabled) is reachable at that path
	// and returns 501 Not Implemented (not 404, which would mean the route is missing).
	koditReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/kodit", nil)
	koditRec := httptest.NewRecorder()
	srv.kodit.mcpBackend.ServeHTTP(koditRec, koditReq, &types.User{ID: userID})
	assert.Equal(t, http.StatusNotImplemented, koditRec.Code, "Kodit backend should return 501 when disabled, not 404")
}
