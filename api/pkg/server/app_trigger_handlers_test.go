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

	// Set user
	req = req.WithContext(suite.authCtx)

	_, httpError := suite.server.createAppTrigger(rec, req)

	suite.Equal(http.StatusForbidden, httpError.StatusCode)
}

func (suite *AppTriggerSuite) TestUpdateAppTrigger_Success() {
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

	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceApplication}, []types.Action{types.ActionUpdate})

	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)

	suite.store.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{
		ID:    "trigger_id_test",
		Owner: suite.userID,
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
		ID:    "trigger_id_test",
		Owner: suite.userID,
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
