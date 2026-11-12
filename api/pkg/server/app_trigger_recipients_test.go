package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func expectRecipientLookups(st *store.MockStore, orgID string) {
	users := map[string]*types.User{
		"owner@example.com":  {ID: "user_id_test"},
		"member@example.com": {ID: "usr_member"},
		"other@example.com":  {ID: "usr_other"},
	}
	st.EXPECT().GetUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *store.GetUserQuery) (*types.User, error) {
			if u, ok := users[strings.ToLower(q.Email)]; ok {
				return u, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	st.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID, UserID: "usr_member",
	}).Return(&types.OrganizationMembership{OrganizationID: orgID, UserID: "usr_member"}, nil).AnyTimes()
	st.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID, UserID: "usr_other",
	}).Return(nil, store.ErrNotFound).AnyTimes()
}

func TestCheckTriggerRecipients(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	expectRecipientLookups(st, "org_1")
	srv := &HelixAPIServer{Store: st}
	ctx := context.Background()

	ok := &types.Trigger{Cron: &types.CronTrigger{Emails: []string{"owner@example.com", "Member@example.com"}}}
	require.Nil(t, srv.checkTriggerRecipients(ctx, "user_id_test", "org_1", ok))

	bad := &types.Trigger{Cron: &types.CronTrigger{Emails: []string{
		"owner@example.com", "other@example.com", "someone@elsewhere.example",
	}}}
	httpErr := srv.checkTriggerRecipients(ctx, "user_id_test", "org_1", bad)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
	require.Contains(t, httpErr.Message, "other@example.com, someone@elsewhere.example")
	require.NotContains(t, httpErr.Message, "owner@example.com")

	// Personal agents (no organization): only the owner.
	require.NotNil(t, srv.checkTriggerRecipients(ctx, "user_id_test", "", ok))

	// Triggers without cron emails are not checked.
	require.Nil(t, srv.checkTriggerRecipients(ctx, "user_id_test", "org_1", &types.Trigger{}))
}

func (suite *AppTriggerSuite) TestCreateAppTrigger_RejectsOutsideRecipients() {
	app := &types.App{ID: "app_id_test", OrganizationID: suite.orgID, AgentKind: types.AgentKindHelix}

	suite.store.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: app.OrganizationID, UserID: suite.userID,
	}).Return(&types.OrganizationMembership{OrganizationID: app.OrganizationID, Role: types.OrganizationRoleMember}, nil)
	setupAuthorizationMocks(suite.store, app, suite.userID, []types.Resource{types.ResourceApplication}, []types.Action{types.ActionGet})
	suite.store.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	expectRecipientLookups(suite.store, suite.orgID)
	// No CreateTriggerConfiguration expectation: nothing is saved.

	body, _ := json.Marshal(&types.TriggerConfiguration{
		Name:  "cron",
		AppID: app.ID,
		Trigger: types.Trigger{Cron: &types.CronTrigger{
			Enabled: true, Schedule: "0 0 * * *", Input: "hi",
			Emails: []string{"owner@example.com", "someone@elsewhere.example"},
		}},
	})
	req := httptest.NewRequest("POST", "/triggers", bytes.NewBuffer(body)).WithContext(suite.authCtx)

	_, httpErr := suite.server.createAppTrigger(httptest.NewRecorder(), req)
	suite.Require().NotNil(httpErr)
	suite.Equal(http.StatusBadRequest, httpErr.StatusCode)
	suite.Contains(httpErr.Message, "someone@elsewhere.example")
}
