package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func expectMembership(st *store.MockStore, orgID, userID string, role types.OrganizationRole) {
	st.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{OrganizationID: orgID, UserID: userID}).
		Return(&types.OrganizationMembership{OrganizationID: orgID, UserID: userID, Role: role}, nil)
}

// Invitation IDs identify invitations in links, revocation and lookups;
// plain members see who is invited, but not the IDs.
func TestInvitationIDsAreHiddenFromMembers(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	s := newTestServerNoNotifier(st)
	orgID, userID := "org_secret", "user_member"
	pending := []*types.OrganizationInvitation{{ID: "oin_secretid", OrganizationID: orgID, Email: "cto@corp.example", Role: types.OrganizationRoleOwner}}

	expectResolveOrganizationByID(st, orgID)
	expectMembership(st, orgID, userID, types.OrganizationRoleMember)
	st.EXPECT().ListOrganizationInvitations(gomock.Any(), gomock.Any()).Return(pending, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/invitations", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))
	rr := httptest.NewRecorder()
	s.listOrganizationInvitations(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "cto@corp.example")
	require.NotContains(t, rr.Body.String(), "oin_secretid", "invitation listing")

	expectResolveOrganizationByID(st, orgID)
	expectMembership(st, orgID, userID, types.OrganizationRoleMember)
	st.EXPECT().ListOrganizationMemberships(gomock.Any(), gomock.Any()).Return(nil, nil)
	st.EXPECT().ListOrganizationInvitations(gomock.Any(), gomock.Any()).Return(pending, nil)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/members", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))
	rr = httptest.NewRecorder()
	s.listOrganizationMembers(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotContains(t, rr.Body.String(), "oin_secretid", "member listing placeholder")
}

// The invitation ID alone (visible to org members) must not reveal the
// invitation: the info lookup needs the link's secret token.
func TestPublicInvitationInfo_RequiresLinkToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	s := newTestServerNoNotifier(st)
	st.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationInvitation{ID: "oin_x", OrganizationID: "org_x", Email: "cto@corp.example"}, nil).AnyTimes()
	st.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_x"}, nil).AnyTimes()

	for _, q := range []string{"", "?token=guess"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/oin_x/info"+q, nil)
		req = mux.SetURLVars(req, map[string]string{"id": "oin_x"})
		rr := httptest.NewRecorder()
		s.publicInvitationInfo(rr, req)
		require.Equal(t, http.StatusNotFound, rr.Code, q)
		require.False(t, strings.Contains(rr.Body.String(), "cto@corp.example"), q)
	}
}
