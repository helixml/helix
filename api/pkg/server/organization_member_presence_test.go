package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The members list is the org's presence source: a member who authenticated
// within PresenceOnlineWindow is online, everyone else (including members who
// never signed in) is offline.
func TestListOrganizationMembers_ReportsPresence(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_presence"
	callerID := "user_caller"
	recent := time.Now().Add(-types.PresenceOnlineWindow / 2)
	stale := time.Now().Add(-2 * types.PresenceOnlineWindow)

	expectResolveOrganizationByID(mockStore, orgID)
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         callerID,
	}).Return(&types.OrganizationMembership{OrganizationID: orgID, UserID: callerID, Role: types.OrganizationRoleMember}, nil)
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), gomock.Any()).Return([]*types.OrganizationMembership{
		{OrganizationID: orgID, UserID: "user_active", User: types.User{ID: "user_active", LastSeenAt: &recent}},
		{OrganizationID: orgID, UserID: "user_away", User: types.User{ID: "user_away", LastSeenAt: &stale}},
		{OrganizationID: orgID, UserID: "user_never", User: types.User{ID: "user_never"}},
	}, nil)
	mockStore.EXPECT().ListOrganizationInvitations(gomock.Any(), gomock.Any()).Return([]*types.OrganizationInvitation{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/members", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: callerID}))

	rr := httptest.NewRecorder()
	server.listOrganizationMembers(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var members []struct {
		UserID string `json:"user_id"`
		Online bool   `json:"online"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &members))
	online := map[string]bool{}
	for _, m := range members {
		online[m.UserID] = m.Online
	}
	require.Equal(t, map[string]bool{
		"user_active": true,
		"user_away":   false,
		"user_never":  false,
	}, online)
}
