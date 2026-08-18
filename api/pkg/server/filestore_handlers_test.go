package server

import (
	"context"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type FilestoreSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	store *store.MockStore

	authCtx context.Context
	userID  string

	orgID string

	filestoreMock *filestore.MockFileStore

	server *HelixAPIServer
}

func TestFilestoreSuite(t *testing.T) {
	suite.Run(t, new(FilestoreSuite))
}

func (suite *FilestoreSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.store = store.NewMockStore(ctrl)

	cfg := &config.ServerConfig{}
	cfg.Controller.FilePrefixGlobal = "/dev"

	suite.orgID = "org_id_test"
	suite.userID = "user_id_test"

	suite.authCtx = setRequestUser(context.Background(), types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	suite.filestoreMock = filestore.NewMockFileStore(ctrl)
	extractorMock := extract.NewMockExtractor(ctrl)
	providerManager := manager.NewMockProviderManager(ctrl)
	providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(openai.NewMockClient(ctrl), nil).AnyTimes()

	c, err := controller.NewController(context.Background(), controller.Options{
		Config:          cfg,
		Store:           suite.store,
		Janitor:         janitor.NewJanitor(config.Janitor{}),
		Filestore:       suite.filestoreMock,
		Extractor:       extractorMock,
		ProviderManager: providerManager,
	})
	suite.NoError(err)

	suite.server = &HelixAPIServer{
		Cfg:        cfg,
		Store:      suite.store,
		Controller: c,
	}
}

// TestIsFilestoreRouteAuthorized_AppPath_Authorized tests that a user with proper access
// can access app files through the filestore
func (suite *FilestoreSuite) TestIsFilestoreRouteAuthorized_AppPath_Authorized() {
	// Create a request with an app path
	req := httptest.NewRequest("GET", "/apps/app_123/file.pdf", nil)
	req = req.WithContext(suite.authCtx)

	// Mock getting the app
	app := &types.App{
		ID:             "app_123",
		OrganizationID: suite.orgID,
	}
	suite.store.EXPECT().GetApp(gomock.Any(), "app_123").Return(app, nil)

	// Mock organization membership check
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// Mock team membership and access grants
	suite.store.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return([]*types.Team{
		{
			ID: "team_id_test",
		},
	}, nil)

	// Mock access grants showing the user has read access through their team
	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
		ResourceID:     app.ID,
		TeamIDs:        []string{"team_id_test"},
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{
				{
					Config: types.Config{
						Rules: []types.Rule{
							{
								Resources: []types.Resource{types.ResourceApplication},
								Actions:   []types.Action{types.ActionGet},
							},
						},
					},
				},
			},
		},
	}, nil)

	authorized, err := suite.server.isFilestoreRouteAuthorized(req)
	suite.NoError(err)
	suite.True(authorized)
}

// TestIsFilestoreRouteAuthorized_AppPath_Unauthorized tests that a user without proper access
// cannot access app files through the filestore
func (suite *FilestoreSuite) TestIsFilestoreRouteAuthorized_AppPath_Unauthorized() {
	// Create a request with an app path
	req := httptest.NewRequest("GET", "/apps/app_123/file.pdf", nil)
	req = req.WithContext(suite.authCtx)

	// Mock getting the app
	app := &types.App{
		ID:             "app_123",
		OrganizationID: suite.orgID,
	}
	suite.store.EXPECT().GetApp(gomock.Any(), "app_123").Return(app, nil)

	// Mock organization membership check
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// Mock team membership
	suite.store.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return([]*types.Team{}, nil) // No teams

	// Mock access grants showing no direct access
	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
		ResourceID:     app.ID,
	}).Return([]*types.AccessGrant{}, nil) // No access grants

	// No projects reference this app (project-based fallback)
	suite.store.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: app.OrganizationID,
	}).Return([]*types.Project{}, nil)

	authorized, err := suite.server.isFilestoreRouteAuthorized(req)
	suite.NoError(err)
	suite.False(authorized)
}

// TestIsFilestoreRouteAuthorized_AppPath_AdminAccess tests that an admin user
// gets access to app files regardless of other permissions
func (suite *FilestoreSuite) TestIsFilestoreRouteAuthorized_AppPath_AdminAccess() {
	// Create a request with an app path
	req := httptest.NewRequest("GET", "/apps/app_123/file.pdf", nil)

	// Set up admin user context
	adminCtx := setRequestUser(context.Background(), types.User{
		ID:       "admin_user_id",
		Email:    "admin@email.com",
		FullName: "Admin User",
		Admin:    true,
	})
	req = req.WithContext(adminCtx)

	authorized, err := suite.server.isFilestoreRouteAuthorized(req)
	suite.NoError(err)
	suite.True(authorized, "Admin user should have access to all files")
}

// TestIsFilestoreRouteAuthorized_UserPath_Authorized tests that a user can access
// their own filestore path
func (suite *FilestoreSuite) TestIsFilestoreRouteAuthorized_UserPath_Authorized() {
	// Create a request with a user path
	req := httptest.NewRequest("GET", "/dev/users/user_id_test/file.pdf", nil)
	req = req.WithContext(suite.authCtx)

	authorized, err := suite.server.isFilestoreRouteAuthorized(req)
	suite.NoError(err)
	suite.True(authorized)
}

// TestFilestoreRouteAuthorized_UserPath_Unauthorized tests that a user cannot access
// another user's filestore path
func (suite *FilestoreSuite) TestFilestoreRouteAuthorized_UserPath_Unauthorized() {
	// Create a request with a different user's path
	req := httptest.NewRequest("GET", "/dev/users/different_user_id/file.pdf", nil)
	req = req.WithContext(suite.authCtx)

	authorized, err := suite.server.isFilestoreRouteAuthorized(req)
	suite.NoError(err)
	suite.False(authorized)
}

// TestFilestoreList_PathTraversalRejected is the H6/H9 regression test:
// ?path= values containing ".." must be rejected instead of resolving into
// other tenants' filestore prefixes. No Filestore mock expectation is set —
// reaching the mock would fail the test, proving the path never got resolved.
func (suite *FilestoreSuite) TestFilestoreList_PathTraversalRejected() {
	traversalPaths := []string{
		"..",
		"../..",
		"../usr_01kc1bacajbhaknzj4xy21x9gs/sessions",
		"documents/../../usr_456/data/file.bin",
		"specs/a/../../../users",
	}
	for _, p := range traversalPaths {
		req := httptest.NewRequest("GET", "/api/v1/filestore/list?path="+url.QueryEscape(p), nil)
		req = req.WithContext(suite.authCtx)

		items, err := suite.server.filestoreList(httptest.NewRecorder(), req)
		suite.Error(err, "path=%q must be rejected", p)
		suite.Nil(items)
		suite.Contains(err.Error(), `not allowed`, "path=%q", p)
	}
}

// TestFilestoreList_LegitPathStillWorks guards against the traversal fix
// over-rejecting: a plain relative path resolves under the caller's prefix.
func (suite *FilestoreSuite) TestFilestoreList_LegitPathStillWorks() {
	suite.filestoreMock.EXPECT().
		CreateFolder(gomock.Any(), gomock.Any()).
		Return(filestore.Item{}, nil).
		AnyTimes()
	suite.filestoreMock.EXPECT().
		List(gomock.Any(), "/dev/users/user_id_test").
		Return([]filestore.Item{{Name: "data"}}, nil)

	req := httptest.NewRequest("GET", "/api/v1/filestore/list?path=", nil)
	req = req.WithContext(suite.authCtx)

	items, err := suite.server.filestoreList(httptest.NewRecorder(), req)
	suite.NoError(err)
	suite.Len(items, 1)
	suite.Equal("data", items[0].Name)
}

// TestFilestoreGet_PathTraversalRejected covers the get/delete-style
// resolution path (same join point, different entry point).
func (suite *FilestoreSuite) TestFilestoreGet_PathTraversalRejected() {
	req := httptest.NewRequest("GET", "/api/v1/filestore/get?path=../usr_other/data/secret.pdf", nil)
	req = req.WithContext(suite.authCtx)

	_, err := suite.server.filestoreGet(httptest.NewRecorder(), req)
	suite.Error(err)
	suite.Contains(err.Error(), `not allowed`)
}
