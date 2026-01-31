package server

import (
	"context"
	"net/http/httptest"
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

// TestIsFilestoreRouteAuthorized_UserPath_Unauthorized tests that a user cannot access
// another user's filestore path
func (suite *FilestoreSuite) TestIsFilestoreRouteAuthorized_UserPath_Unauthorized() {
	// Create a request with a different user's path
	req := httptest.NewRequest("GET", "/dev/users/different_user_id/file.pdf", nil)
	req = req.WithContext(suite.authCtx)

	authorized, err := suite.server.isFilestoreRouteAuthorized(req)
	suite.NoError(err)
	suite.False(authorized)
}
