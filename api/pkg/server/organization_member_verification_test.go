package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func newOrgTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	return &HelixAPIServer{Store: st, Cfg: &config.ServerConfig{}}, st
}

func orgRequest(method, path string, vars map[string]string, requesterID string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req = mux.SetURLVars(req, vars)
	return req.WithContext(setRequestUser(req.Context(), types.User{ID: requesterID, Email: requesterID + "@example.com"}))
}

// Adding by email resolves only an account shown to own the address. A
// password account that merely registered it gets no membership: the address
// is invited, and whoever reads the mail accepts with its token.
func TestAddOrganizationMember_UnverifiedEmailAccountIsInvitedNotAdded(t *testing.T) {
	s, st := newOrgTestServer(t)
	orgID, ownerID := "org_1", "usr_owner"
	expectResolveOrganizationByID(st, orgID)
	expectOrgOwner(st, orgID, ownerID)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "victim@example.com"}).
		Return(&types.User{ID: "usr_squatter", Email: "victim@example.com", EmailVerified: false}, nil)
	st.EXPECT().CreateOrganizationInvitation(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, inv *types.OrganizationInvitation) (*types.OrganizationInvitation, error) {
		require.Equal(t, "victim@example.com", inv.Email)
		inv.ID = "oin_1"
		return inv, nil
	})
	// No CreateOrganizationMembership: the mock fails the test on one.

	rr := httptest.NewRecorder()
	s.addOrganizationMember(rr, orgRequest(http.MethodPost, "/api/v1/organizations/org_1/members", map[string]string{"id": orgID}, ownerID,
		types.AddOrganizationMemberRequest{UserReference: "victim@example.com"}))

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var resp types.AddOrganizationMemberResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.True(t, resp.Invited)
	require.Nil(t, resp.Membership)
}

// A verified account is added directly, as before.
func TestAddOrganizationMember_VerifiedEmailAccountIsAdded(t *testing.T) {
	s, st := newOrgTestServer(t)
	orgID, ownerID := "org_1", "usr_owner"
	expectResolveOrganizationByID(st, orgID)
	expectOrgOwner(st, orgID, ownerID)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "member@example.com"}).
		Return(&types.User{ID: "usr_member", Email: "member@example.com", EmailVerified: true}, nil)
	st.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
		require.Equal(t, "usr_member", m.UserID)
		return m, nil
	})

	rr := httptest.NewRecorder()
	s.addOrganizationMember(rr, orgRequest(http.MethodPost, "/api/v1/organizations/org_1/members", map[string]string{"id": orgID}, ownerID,
		types.AddOrganizationMemberRequest{UserReference: "member@example.com"}))

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

// Adding by user ID is unaffected: the ID names the account itself.
func TestAddOrganizationMember_ByIDIgnoresEmailVerification(t *testing.T) {
	s, st := newOrgTestServer(t)
	orgID, ownerID := "org_1", "usr_owner"
	expectResolveOrganizationByID(st, orgID)
	expectOrgOwner(st, orgID, ownerID)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_member"}).
		Return(&types.User{ID: "usr_member", Email: "member@example.com"}, nil)
	st.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
		return m, nil
	})

	rr := httptest.NewRecorder()
	s.addOrganizationMember(rr, orgRequest(http.MethodPost, "/api/v1/organizations/org_1/members", map[string]string{"id": orgID}, ownerID,
		types.AddOrganizationMemberRequest{UserReference: "usr_member"}))

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

// A team seat by email likewise needs a verified address; the store mock
// fails the test on any membership lookup or write for the squatter.
func TestAddTeamMember_UnverifiedEmailAccountIsRefused(t *testing.T) {
	s, st := newOrgTestServer(t)
	orgID, ownerID, teamID := "org_1", "usr_owner", "team_1"
	expectResolveOrganizationByID(st, orgID)
	expectOrgOwner(st, orgID, ownerID)
	st.EXPECT().GetTeam(gomock.Any(), &store.GetTeamQuery{ID: teamID, OrganizationID: orgID}).Return(&types.Team{ID: teamID, OrganizationID: orgID}, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "victim@example.com"}).
		Return(&types.User{ID: "usr_squatter", Email: "victim@example.com"}, nil)

	rr := httptest.NewRecorder()
	s.addTeamMember(rr, orgRequest(http.MethodPost, "/api/v1/organizations/org_1/teams/team_1/members", map[string]string{"id": orgID, "team_id": teamID}, ownerID,
		types.AddOrganizationMemberRequest{UserReference: "victim@example.com"}))

	require.Equal(t, http.StatusPreconditionFailed, rr.Code, rr.Body.String())
}

// Accepting an invitation with its token shows control of the invited
// address, so the account becomes verified.
func TestAcceptInvitation_MarksEmailVerified(t *testing.T) {
	s, st := newOrgTestServer(t)
	user := &types.User{ID: "usr_1", Email: "invited@example.com"}
	st.EXPECT().ConsumeInvitation(gomock.Any(), user, "oin_1", "secret").
		Return([]*types.OrganizationMembership{{OrganizationID: "org_1", UserID: user.ID}}, nil)
	st.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
		require.True(t, u.EmailVerified)
		return u, nil
	})

	accepted, err := s.acceptInvitation(context.Background(), user, "oin_1", "secret")
	require.NoError(t, err)
	require.True(t, accepted)
}

// Only owner and member are roles. Anything else is refused before the
// target is even looked up; the store mock fails the test on a write.
func TestOrganizationMemberRoles_OnlyOwnerAndMember(t *testing.T) {
	orgID, ownerID := "org_1", "usr_owner"
	for _, role := range []types.OrganizationRole{"admin", "superuser", "Owner"} {
		t.Run("add "+string(role), func(t *testing.T) {
			s, st := newOrgTestServer(t)
			expectResolveOrganizationByID(st, orgID)
			expectOrgOwner(st, orgID, ownerID)
			rr := httptest.NewRecorder()
			s.addOrganizationMember(rr, orgRequest(http.MethodPost, "/api/v1/organizations/org_1/members", map[string]string{"id": orgID}, ownerID,
				types.AddOrganizationMemberRequest{UserReference: "usr_member", Role: role}))
			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		})
		t.Run("invite "+string(role), func(t *testing.T) {
			s, st := newOrgTestServer(t)
			expectResolveOrganizationByID(st, orgID)
			expectOrgOwner(st, orgID, ownerID)
			rr := httptest.NewRecorder()
			s.createOrganizationInvitationHandler(rr, orgRequest(http.MethodPost, "/api/v1/organizations/org_1/invitations", map[string]string{"id": orgID}, ownerID,
				types.AddOrganizationMemberRequest{UserReference: "new@example.com", Role: role}))
			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		})
	}
	for _, role := range []types.OrganizationRole{"", "admin"} {
		t.Run("update to "+string(role), func(t *testing.T) {
			s, st := newOrgTestServer(t)
			expectResolveOrganizationByID(st, orgID)
			expectOrgOwner(st, orgID, ownerID)
			rr := httptest.NewRecorder()
			s.updateOrganizationMember(rr, orgRequest(http.MethodPut, "/api/v1/organizations/org_1/members/usr_member", map[string]string{"id": orgID, "user_id": "usr_member"}, ownerID,
				types.UpdateOrganizationMemberRequest{Role: role}))
			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		})
	}
}

// The last owner can neither be demoted nor removed, so an organization
// always keeps someone who can manage it.
func TestOrganizationLastOwner_CannotBeDemotedOrRemoved(t *testing.T) {
	orgID, ownerID := "org_1", "usr_owner"
	soleOwner := []*types.OrganizationMembership{{OrganizationID: orgID, UserID: ownerID, Role: types.OrganizationRoleOwner}}

	t.Run("demote", func(t *testing.T) {
		s, st := newOrgTestServer(t)
		expectResolveOrganizationByID(st, orgID)
		expectOrgOwner(st, orgID, ownerID).Times(2) // requester, then target
		st.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{OrganizationID: orgID}).Return(soleOwner, nil)
		rr := httptest.NewRecorder()
		s.updateOrganizationMember(rr, orgRequest(http.MethodPut, "/api/v1/organizations/org_1/members/usr_owner", map[string]string{"id": orgID, "user_id": ownerID}, ownerID,
			types.UpdateOrganizationMemberRequest{Role: types.OrganizationRoleMember}))
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	})
	t.Run("remove", func(t *testing.T) {
		s, st := newOrgTestServer(t)
		expectResolveOrganizationByID(st, orgID)
		expectOrgOwner(st, orgID, ownerID).Times(2)
		st.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{OrganizationID: orgID}).Return(soleOwner, nil)
		rr := httptest.NewRecorder()
		s.removeOrganizationMember(rr, orgRequest(http.MethodDelete, "/api/v1/organizations/org_1/members/usr_owner", map[string]string{"id": orgID, "user_id": ownerID}, ownerID, nil))
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	})
}
