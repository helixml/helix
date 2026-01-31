package server

import (
	"bytes"
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

type AppAccessGrantSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	store *store.MockStore

	authCtx context.Context
	userID  string

	orgID string

	server *HelixAPIServer
}

func TestAppAccessGrantSuite(t *testing.T) {
	suite.Run(t, new(AppAccessGrantSuite))
}

func (suite *AppAccessGrantSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.store = store.NewMockStore(ctrl)

	cfg := &config.ServerConfig{}

	suite.orgID = "org_id_test"
	suite.userID = "user_id_test"

	suite.authCtx = setRequestUser(context.Background(), types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	suite.server = &HelixAPIServer{
		Cfg:   cfg,
		Store: suite.store,
	}
}

func (suite *AppAccessGrantSuite) TestListAppAccessGrants_NoOrg() {
	app := &types.App{
		ID: "app_id_test",
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/app_id_test/access-grants", http.NoBody)
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	// suite.server.listAppAccessGrants(rec, req)
	suite.server.listAppAccessGrants(rec, req)

	suite.Equal(http.StatusBadRequest, rec.Code)
	// check the response body
	suite.Contains(rec.Body.String(), `agent is not associated with an organization`)
}

func (suite *AppAccessGrantSuite) TestListAppAccessGrants_OrgOwner() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleOwner,
	}

	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
	}).Return([]*types.AccessGrant{
		{
			ID: "access_grant_id_test",
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/app_id_test/access-grants", http.NoBody)
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.listAppAccessGrants(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

func (suite *AppAccessGrantSuite) TestListAppAccessGrants_OrgMember_AppOwner() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          suite.userID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}

	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
	}).Return([]*types.AccessGrant{
		{
			ID: "access_grant_id_test",
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/app_id_test/access-grants", http.NoBody)
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.listAppAccessGrants(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

func (suite *AppAccessGrantSuite) TestListAppAccessGrants_OrgMember_NoAccess() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}

	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	suite.store.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return([]*types.Team{}, nil) // No teams

	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
		ResourceID:     app.ID,
	}).Return([]*types.AccessGrant{}, nil) // No direct access grants

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/app_id_test/access-grants", http.NoBody)
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.listAppAccessGrants(rec, req)

	suite.Equal(http.StatusForbidden, rec.Code)
}

func (suite *AppAccessGrantSuite) TestListAppAccessGrants_OrgMember_HasDirectAccess() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}

	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	suite.store.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return([]*types.Team{}, nil) // No teams

	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
		ResourceID:     app.ID,
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{
				{
					Config: types.Config{
						Rules: []types.Rule{
							{
								Resources: []types.Resource{types.ResourceAccessGrants},
								Actions:   []types.Action{types.ActionGet},
							},
						},
					},
				},
			},
		},
	}, nil) // Read access to access grants

	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
	}).Return([]*types.AccessGrant{
		{
			ID: "access_grant_id_test",
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/app_id_test/access-grants", http.NoBody)
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.listAppAccessGrants(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

func (suite *AppAccessGrantSuite) TestListAppAccessGrants_OrgMember_HasTeamAccess() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}

	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	suite.store.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return([]*types.Team{
		{
			ID: "team_id_test",
		},
	}, nil) // One team

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
								Resources: []types.Resource{types.ResourceAccessGrants},
								Actions:   []types.Action{types.ActionGet},
							},
						},
					},
				},
			},
		},
	}, nil) // Read access to access grants

	suite.store.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
	}).Return([]*types.AccessGrant{
		{
			ID: "access_grant_id_test",
		},
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/apps/app_id_test/access-grants", http.NoBody)
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.listAppAccessGrants(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

// TestGrantAccess_OrgOwner org owner can grant access to a user
func (suite *AppAccessGrantSuite) TestGrantAccess_OrgOwner_ToUser() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	// 1. Checking whether caller is org owner
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleOwner,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// 2. Checking grantee user exists
	suite.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{
		ID: "user_id_test",
	}).Return(&types.User{
		ID: "user_id_test",
	}, nil)

	// 3. Since caller is org owner, we can skip authorization check

	// 4. Ensure roles
	suite.store.EXPECT().ListRoles(gomock.Any(), suite.orgID).Return([]*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}, nil)

	// 5. Create access grant
	suite.store.EXPECT().CreateAccessGrant(gomock.Any(), &types.AccessGrant{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
		UserID:         "user_id_test",
	}, []*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}).Return(&types.AccessGrant{
		ID: "access_grant_id_test",
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/apps/app_id_test/access-grants", bytes.NewBufferString(`{"user_reference": "user_id_test", "roles": ["admin"]}`))
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.createAppAccessGrant(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

// TestGrantAccess_OrgOwner org owner can grant access to a team
func (suite *AppAccessGrantSuite) TestGrantAccess_OrgOwner_ToTeam() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	// 1. Checking whether caller is org owner
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleOwner,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// 2. Checking grantee user exists
	suite.store.EXPECT().GetTeam(gomock.Any(), &store.GetTeamQuery{
		OrganizationID: app.OrganizationID,
		ID:             "team_id_test",
	}).Return(&types.Team{
		ID: "team_id_test",
	}, nil)

	// 3. Since caller is org owner, we can skip authorization check

	// 4. Ensure roles
	suite.store.EXPECT().ListRoles(gomock.Any(), suite.orgID).Return([]*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}, nil)

	// 5. Create access grant
	suite.store.EXPECT().CreateAccessGrant(gomock.Any(), &types.AccessGrant{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
		// ResourceType:   types.ResourceApplication,
		TeamID: "team_id_test",
	}, []*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}).Return(&types.AccessGrant{
		ID: "access_grant_id_test",
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/apps/app_id_test/access-grants", bytes.NewBufferString(`{"team_id": "team_id_test", "roles": ["admin"]}`))
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.createAppAccessGrant(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

func (suite *AppAccessGrantSuite) TestGrantAccess_AppOwner_ToUser() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          suite.userID,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	// 1. Checking whether caller is org member
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// 2. Checking grantee user exists
	suite.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{
		ID: "user_id_test",
	}).Return(&types.User{
		ID: "user_id_test",
	}, nil)

	// 3. Since caller is org owner, we can skip authorization check

	// 4. Ensure roles
	suite.store.EXPECT().ListRoles(gomock.Any(), suite.orgID).Return([]*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}, nil)

	// 5. Create access grant
	suite.store.EXPECT().CreateAccessGrant(gomock.Any(), &types.AccessGrant{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
		UserID:         "user_id_test",
	}, []*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}).Return(&types.AccessGrant{
		ID: "access_grant_id_test",
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/apps/app_id_test/access-grants", bytes.NewBufferString(`{"user_reference": "user_id_test", "roles": ["admin"]}`))
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.createAppAccessGrant(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

// TestGrantAccess_HasAccess_ToUser checks whether non-org owner and non-app owner can grant access to a user
// if they have the permissions to do so (they are in the team that has access admin access to the app)
func (suite *AppAccessGrantSuite) TestGrantAccess_HasAccess_ToUser() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          "someone-else", // Not the caller
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	// 1. Checking whether caller is org member
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// 2. Checking grantee user exists
	suite.store.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{
		ID: "user_id_test",
	}).Return(&types.User{
		ID: "user_id_test",
	}, nil)

	// 3. Checking whether caller has access to the app
	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceAccessGrants}, []types.Action{types.ActionUpdate})

	// 4. Ensure roles
	suite.store.EXPECT().ListRoles(gomock.Any(), suite.orgID).Return([]*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}, nil)

	// 5. Create access grant
	suite.store.EXPECT().CreateAccessGrant(gomock.Any(), &types.AccessGrant{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
		UserID:         "user_id_test",
	}, []*types.Role{
		{
			Name: "admin",
			ID:   "admin_role_id_test",
		},
	}).Return(&types.AccessGrant{
		ID: "access_grant_id_test",
	}, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/apps/app_id_test/access-grants", bytes.NewBufferString(`{"user_reference": "user_id_test", "roles": ["admin"]}`))
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.createAppAccessGrant(rec, req)

	suite.Equal(http.StatusOK, rec.Code)
	suite.Contains(rec.Body.String(), `access_grant_id_test`)
}

func (suite *AppAccessGrantSuite) TestGrantAccess_Denied_ToUser() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          "someone-else", // Not the caller
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	// 1. Checking whether caller is org member
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// 2. Caller does not have access to the app's access grant updates (can only list them)
	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceAccessGrants}, []types.Action{types.ActionGet})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/apps/app_id_test/access-grants", bytes.NewBufferString(`{"user_reference": "user_id_test", "roles": ["admin"]}`))
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"id": "app_id_test",
	}
	req = mux.SetURLVars(req, vars)

	suite.server.createAppAccessGrant(rec, req)

	suite.Equal(http.StatusForbidden, rec.Code)
}

func setupAuthorizationMocks(mockStore *store.MockStore, app *types.App, callerUserID string, resources []types.Resource, actions []types.Action) {
	mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         callerUserID,
	}).Return([]*types.Team{
		{
			ID: "team_id_test",
		},
	}, nil) // One team

	mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		UserID:         callerUserID,
		ResourceID:     app.ID,
		TeamIDs:        []string{"team_id_test"},
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{
				{
					Config: types.Config{
						Rules: []types.Rule{
							{
								Resources: resources,
								Actions:   actions,
							},
						},
					},
				},
			},
		},
	}, nil)
}
