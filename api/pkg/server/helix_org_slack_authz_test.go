package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Connecting, starting the install of, or removing a Slack workspace changes
// org-wide configuration, so a plain member is refused before anything else
// happens.
func TestSlackWorkspaceMutations_RequireOrgOwner(t *testing.T) {
	for name, call := range map[string]func(*HelixAPIServer, http.ResponseWriter, *http.Request){
		"connect":     (*HelixAPIServer).connectSlackWorkspace,
		"delete":      (*HelixAPIServer).deleteSlackWorkspace,
		"oauth start": (*HelixAPIServer).slackOAuthStart,
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_1"}, nil)
			mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(
				&types.OrganizationMembership{OrganizationID: "org_1", UserID: "usr_member", Role: types.OrganizationRoleMember}, nil).AnyTimes()
			server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/orgs/org_1/slack/workspaces", strings.NewReader(`{"bot_token":"xoxb-1"}`))
			req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_member"}))
			req = mux.SetURLVars(req, map[string]string{"org": "org_1", "id": "conn_1"})
			rr := httptest.NewRecorder()
			call(server, rr, req)
			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}
