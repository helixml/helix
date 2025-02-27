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
	suite.Contains(rec.Body.String(), `app is not associated with an organization`)
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

	// suite.server.listAppAccessGrants(rec, req)
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
		ResourceType:   types.ResourceAccessGrants,
		// TeamIDs:        []string{},
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
