package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type AppTriggerSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	store *store.MockStore

	authCtx context.Context
	userID  string

	orgID string

	server *HelixAPIServer
}

func TestAppTriggerSuite(t *testing.T) {
	suite.Run(t, new(AppTriggerSuite))
}

func (suite *AppTriggerSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.store = store.NewMockStore(ctrl)

	cfg := &config.ServerConfig{
		WebServer: config.WebServer{
			URL: "https://test.example.com",
		},
	}

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

func (suite *AppTriggerSuite) TestCreateAppTrigger_Success() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		AgentKind:      types.AgentKindHelix,
	}

	triggerConfig := &types.TriggerConfiguration{
		Name:  "Test Cron Trigger",
		AppID: app.ID,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
				Input:    "Hello from cron",
			},
		},
	}

	expectedTriggerConfig := &types.TriggerConfiguration{
		ID:             "trigger_id_test",
		AppID:          app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          suite.userID,
		OwnerType:      types.OwnerTypeUser,
		Name:           triggerConfig.Name,
		Trigger:        triggerConfig.Trigger,
	}

	// 1. Checking whether caller is org member
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceApplication}, []types.Action{types.ActionGet})

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	suite.store.EXPECT().CreateTriggerConfiguration(gomock.Any(), gomock.Any()).Return(expectedTriggerConfig, nil)

	rec := httptest.NewRecorder()
	reqBody, _ := json.Marshal(triggerConfig)
	req := httptest.NewRequest("POST", "/triggers", bytes.NewBuffer(reqBody))
	req = req.WithContext(suite.authCtx)

	resp, err := suite.server.createAppTrigger(rec, req)
	require.Nil(suite.T(), err)

	suite.Equal(expectedTriggerConfig.Trigger.Cron.Enabled, resp.Trigger.Cron.Enabled)

}

func (suite *AppTriggerSuite) TestCreateAppTrigger_AppNotFound() {
	suite.store.EXPECT().GetApp(gomock.Any(), "app_id_test").Return(nil, store.ErrNotFound)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/triggers/triggers", bytes.NewBufferString(`{
		"app_id": "app_id_test"
	}`))

	_, httpError := suite.server.createAppTrigger(rec, req)
	suite.Equal(http.StatusInternalServerError, httpError.StatusCode)
}

func (suite *AppTriggerSuite) TestCreateAppTrigger_Unauthorized() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		AgentKind:      types.AgentKindHelix,
		Owner:          "different_user", // Different owner
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/triggers", bytes.NewBufferString(`{
		"app_id": "app_id_test"
	}`))

	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	// Not to the app
	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceKnowledge}, []types.Action{types.ActionGet})

	// No projects reference this app (project-based fallback)
	suite.store.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: app.OrganizationID,
	}).Return([]*types.Project{}, nil)

	// Set user
	req = req.WithContext(suite.authCtx)

	_, httpError := suite.server.createAppTrigger(rec, req)

	suite.Equal(http.StatusForbidden, httpError.StatusCode)
}

func (suite *AppTriggerSuite) TestUpdateAppTrigger_Success() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		AgentKind:      types.AgentKindHelix,
	}

	existingTrigger := &types.TriggerConfiguration{
		ID:             "trigger_id_test",
		AppID:          app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          suite.userID,
		OwnerType:      types.OwnerTypeUser,
		Name:           "Original Trigger",
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
				Input:    "Original input",
			},
		},
	}

	updatedTriggerConfig := &types.TriggerConfiguration{
		Name:           "Updated Trigger",
		AppID:          app.ID,
		OrganizationID: app.OrganizationID,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 12 * * *",
				Input:    "Updated input",
			},
		},
	}

	expectedUpdatedTrigger := &types.TriggerConfiguration{
		ID:             "trigger_id_test",
		AppID:          app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          suite.userID,
		OwnerType:      types.OwnerTypeUser,
		Name:           "Updated Trigger",
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 12 * * *",
				Input:    "Updated input",
			},
		},
	}

	// Authorization setup
	orgMembership := &types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(orgMembership, nil)

	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceApplication}, []types.Action{types.ActionGet})

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	// Looked up without an owner filter; the caller is authorized as the trigger's
	// own owner, so no further store calls are needed.
	suite.store.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{
		ID: "trigger_id_test",
	}).Return(existingTrigger, nil)
	suite.store.EXPECT().UpdateTriggerConfiguration(gomock.Any(), gomock.Any()).Return(expectedUpdatedTrigger, nil)

	rec := httptest.NewRecorder()
	reqBody, _ := json.Marshal(updatedTriggerConfig)

	req := httptest.NewRequest("PUT", "/triggers/trigger_id_test", bytes.NewBuffer(reqBody))
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"trigger_id": "trigger_id_test",
	}
	req = mux.SetURLVars(req, vars)

	resp, _ := suite.server.updateAppTrigger(rec, req)

	suite.Equal(expectedUpdatedTrigger.Name, resp.Name)
	suite.Equal(expectedUpdatedTrigger.Trigger.Cron.Schedule, resp.Trigger.Cron.Schedule)
	suite.Equal(expectedUpdatedTrigger.Trigger.Cron.Input, resp.Trigger.Cron.Input)
}

func (suite *AppTriggerSuite) TestDeleteAppTrigger_Success() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
	}

	existingTrigger := &types.TriggerConfiguration{
		ID:             "trigger_id_test",
		AppID:          app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          suite.userID,
		OwnerType:      types.OwnerTypeUser,
		Name:           "Test Trigger",
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
				Input:    "Test input",
			},
		},
	}

	suite.store.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{
		ID: "trigger_id_test",
	}).Return(existingTrigger, nil)
	suite.store.EXPECT().DeleteTriggerConfiguration(gomock.Any(), "trigger_id_test").Return(nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/triggers/trigger_id_test", nil)
	req = req.WithContext(suite.authCtx)
	vars := map[string]string{
		"trigger_id": "trigger_id_test",
	}
	req = mux.SetURLVars(req, vars)

	resp, _ := suite.server.deleteAppTrigger(rec, req)

	suite.Equal(existingTrigger.ID, resp.ID)
	suite.Equal(existingTrigger.Name, resp.Name)
	suite.Equal(existingTrigger.AppID, resp.AppID)
}

// The regression behind spec 002867: a cron trigger created by someone else on an
// app you manage used to be invisible to you, while Helix's cron scheduler
// (getCronAppsFromTriggers, which applies no owner filter) went on firing it. Nobody
// could list it, so nobody could prune it.
func (suite *AppTriggerSuite) TestListAppTriggers_ManagerSeesOtherOwnersTriggers() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          suite.userID, // we manage this app
		AgentKind:      types.AgentKindCoding,
	}

	strangersTrigger := &types.TriggerConfiguration{
		ID:      "trigger_id_orphan",
		AppID:   app.ID,
		Owner:   "a_rotated_service_account",
		Name:    "Daily Deal Intelligence",
		Trigger: types.Trigger{Cron: &types.CronTrigger{Enabled: true, Schedule: "0 9 * * 1-5"}},
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	// No Owner in the query: the app's manager sees every trigger on it.
	suite.store.EXPECT().ListTriggerConfigurations(gomock.Any(), &store.ListTriggerConfigurationsQuery{
		AppID:          app.ID,
		OrganizationID: app.OrganizationID,
	}).Return([]*types.TriggerConfiguration{strangersTrigger}, nil)

	req := httptest.NewRequest("GET", "/agents/app_id_test/triggers", nil)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"agent_id": app.ID})

	resp, httpErr := suite.server.listAppTriggers(httptest.NewRecorder(), req)
	require.Nil(suite.T(), httpErr)
	require.Len(suite.T(), resp, 1)
	suite.Equal("trigger_id_orphan", resp[0].ID)
	suite.Equal("a_rotated_service_account", resp[0].Owner)
}

// A caller who can only read the app still sees just their own triggers.
func (suite *AppTriggerSuite) TestListAppTriggers_ReaderSeesOnlyOwnTriggers() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          "somebody_else",
		AgentKind:      types.AgentKindHelix,
	}

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}, nil).AnyTimes() // once for the ActionDelete probe, once for ActionGet

	// Read-only: granted Get on applications, but not Delete. Set up twice — the
	// handler probes ActionDelete first, then falls back to ActionGet.
	setupAuthorizationMocks(suite.store, app, suite.userID,
		[]types.Resource{types.ResourceApplication}, []types.Action{types.ActionGet})
	setupAuthorizationMocks(suite.store, app, suite.userID,
		[]types.Resource{types.ResourceApplication}, []types.Action{types.ActionGet})

	// The failed ActionDelete probe falls through to the project-based grant check.
	suite.store.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: app.OrganizationID,
	}).Return([]*types.Project{}, nil).AnyTimes()

	suite.store.EXPECT().ListTriggerConfigurations(gomock.Any(), &store.ListTriggerConfigurationsQuery{
		AppID:          app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          suite.userID,
	}).Return([]*types.TriggerConfiguration{}, nil)

	req := httptest.NewRequest("GET", "/agents/app_id_test/triggers", nil)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"agent_id": app.ID})

	_, httpErr := suite.server.listAppTriggers(httptest.NewRecorder(), req)
	require.Nil(suite.T(), httpErr)
}

// An app's manager can delete a trigger they do not own — the lever that was missing
// when 22 orphaned triggers had to be cleaned up by minting a key for their owner.
func (suite *AppTriggerSuite) TestDeleteAppTrigger_ManagerDeletesOtherOwnersTrigger() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          suite.userID,
	}

	strangersTrigger := &types.TriggerConfiguration{
		ID:    "trigger_id_orphan",
		AppID: app.ID,
		Owner: "a_rotated_service_account",
		Name:  "Daily Deal Intelligence",
	}

	suite.store.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{
		ID: "trigger_id_orphan",
	}).Return(strangersTrigger, nil)
	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	suite.store.EXPECT().DeleteTriggerConfiguration(gomock.Any(), "trigger_id_orphan").Return(nil)

	req := httptest.NewRequest("DELETE", "/triggers/trigger_id_orphan", nil)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"trigger_id": "trigger_id_orphan"})

	resp, httpErr := suite.server.deleteAppTrigger(httptest.NewRecorder(), req)
	require.Nil(suite.T(), httpErr)
	suite.Equal("trigger_id_orphan", resp.ID)
}

// A stranger with no rights over the app or the trigger gets 404, not a deletion.
func (suite *AppTriggerSuite) TestDeleteAppTrigger_StrangerRefused() {
	app := &types.App{
		ID:             "app_id_test",
		OrganizationID: suite.orgID,
		Owner:          "somebody_else",
	}

	strangersTrigger := &types.TriggerConfiguration{
		ID:    "trigger_id_orphan",
		AppID: app.ID,
		Owner: "a_third_party",
	}

	suite.store.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{
		ID: "trigger_id_orphan",
	}).Return(strangersTrigger, nil)
	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID,
		UserID:         suite.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: app.OrganizationID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	setupAuthorizationMocks(suite.store, app, suite.userID,
		[]types.Resource{types.ResourceKnowledge}, []types.Action{types.ActionGet})
	suite.store.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: app.OrganizationID,
	}).Return([]*types.Project{}, nil)

	req := httptest.NewRequest("DELETE", "/triggers/trigger_id_orphan", nil)
	req = req.WithContext(suite.authCtx)
	req = mux.SetURLVars(req, map[string]string{"trigger_id": "trigger_id_orphan"})

	_, httpErr := suite.server.deleteAppTrigger(httptest.NewRecorder(), req)
	require.NotNil(suite.T(), httpErr)
	suite.Equal(http.StatusNotFound, httpErr.StatusCode)
}

// Cron triggers that fire spec tasks target CODING agents — spec task creation
// requires AgentKindCoding. Pinning this surface to helix_agent banned exactly the
// triggers HelixOS's bot schedules are made of.
func TestRequireTriggerAgentKindAcceptsCodingAgents(t *testing.T) {
	require.NoError(t, requireTriggerAgentKind(&types.App{AgentKind: types.AgentKindCoding}))
	require.NoError(t, requireTriggerAgentKind(&types.App{AgentKind: types.AgentKindHelix}))
	require.Error(t, requireTriggerAgentKind(&types.App{AgentKind: types.AgentKindOrg}))
	require.Error(t, requireTriggerAgentKind(nil))
}
