package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// expectOrgOwner sets the mock expectation for the org-owner authorization
// check used by all invite-related handlers. The membership lookup runs
// once per call to authorizeOrgOwner, plus an extra membership lookup for
// the AddOrganizationMember handler when checking IsMember below — callers
// pass the right Times() count.
func expectOrgOwner(mockStore *store.MockStore, orgID, userID string) *gomock.Call {
	return mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleOwner,
	}, nil)
}

// newTestServerNoNotifier returns a minimal HelixAPIServer wired with the
// supplied store. The notifier (Controller.Options.Notifier) is nil — the
// invite handlers tolerate that and skip the email send, which is exactly
// what we want for handler-level tests.
func newTestServerNoNotifier(mockStore *store.MockStore) *HelixAPIServer {
	return &HelixAPIServer{
		Store: mockStore,
		Cfg: &config.ServerConfig{
			Notifications: config.Notifications{AppURL: "http://test"},
		},
	}
}

func TestAddOrganizationMember_ExistingUser_CreatesMembership(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_existing"
	ownerID := "user_owner"
	newUserID := "user_existing"

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "existing@example.com"}).
		Return(&types.User{ID: newUserID, Email: "existing@example.com"}, nil)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, m *types.OrganizationMembership) (*types.OrganizationMembership, error) {
			require.Equal(t, orgID, m.OrganizationID)
			require.Equal(t, newUserID, m.UserID)
			require.Equal(t, types.OrganizationRoleMember, m.Role)
			return m, nil
		})

	body, _ := json.Marshal(types.AddOrganizationMemberRequest{UserReference: "existing@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp types.AddOrganizationMemberResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotNil(t, resp.Membership, "membership must be populated")
	require.Nil(t, resp.Invitation, "no invitation when user exists")
	require.False(t, resp.Invited)
}

func TestAddOrganizationMember_UnknownEmail_CreatesInvitation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_invite"
	ownerID := "user_owner"

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "stranger@example.com"}).
		Return(nil, store.ErrNotFound)

	// No duplicate exists yet — return ErrNotFound from the existence check
	// inside CreateOrganizationInvitation. Then expect Create.
	mockStore.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound).AnyTimes()
	mockStore.EXPECT().CreateOrganizationInvitation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, inv *types.OrganizationInvitation) (*types.OrganizationInvitation, error) {
			require.Equal(t, orgID, inv.OrganizationID)
			require.Equal(t, "stranger@example.com", inv.Email)
			require.Equal(t, types.OrganizationRoleMember, inv.Role)
			require.Equal(t, ownerID, inv.InvitedBy)
			inv.ID = "oin_test"
			return inv, nil
		})
	// sendInvitationEmail short-circuits because Controller is nil, so no
	// further store calls.

	body, _ := json.Marshal(types.AddOrganizationMemberRequest{UserReference: "stranger@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp types.AddOrganizationMemberResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Nil(t, resp.Membership)
	require.NotNil(t, resp.Invitation)
	require.True(t, resp.Invited)
}

func TestAddOrganizationMember_UnknownUserID_Returns404(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_idfallback"
	ownerID := "user_owner"

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)

	// UserReference is NOT an email — treated as a user id. If not found,
	// the handler must return 404 rather than creating an invitation.
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "user_does_not_exist"}).
		Return(nil, store.ErrNotFound)

	body, _ := json.Marshal(types.AddOrganizationMemberRequest{UserReference: "user_does_not_exist"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestAddOrganizationMember_UnknownEmail_PassesAppContextToInvitation(t *testing.T) {
	// When the access-management dialog invites someone, it sends
	// app_id + grant_roles in the request. The handler must forward
	// those onto the invitation row so ConsumePendingInvitations later
	// has the context it needs to create the access grant.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_invite_scoped"
	ownerID := "user_owner"

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "scoped@example.com"}).
		Return(nil, store.ErrNotFound)
	mockStore.EXPECT().CreateOrganizationInvitation(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ any, inv *types.OrganizationInvitation) (*types.OrganizationInvitation, error) {
			require.Equal(t, "app_xyz", inv.AppID, "app_id must be persisted on the invitation")
			require.Equal(t, []string{"app_user"}, []string(inv.GrantRoles), "grant_roles must be persisted")
			inv.ID = "oin_scoped"
			return inv, nil
		})

	body, _ := json.Marshal(types.AddOrganizationMemberRequest{
		UserReference: "scoped@example.com",
		AppID:         "app_xyz",
		GrantRoles:    []string{"app_user"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp types.AddOrganizationMemberResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Invited)
}

func TestListOrganizationMembers_ExcludesAppScopedInvitations(t *testing.T) {
	// The org-wide members table must not surface project-scoped
	// invitations as placeholders — those belong to the project access
	// list, fetched separately by AccessManagement.tsx. This test
	// pins that contract.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_filter"
	userID := "user_caller"

	expectResolveOrganizationByID(mockStore, orgID)
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), gomock.Any()).
		Return([]*types.OrganizationMembership{}, nil)
	mockStore.EXPECT().ListOrganizationInvitations(gomock.Any(), gomock.Any()).
		Return([]*types.OrganizationInvitation{
			{ID: "oin_orgwide", OrganizationID: orgID, Email: "orgwide@example.com"},
			{ID: "oin_app", OrganizationID: orgID, Email: "scoped@example.com", AppID: "app_x"},
		}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/members", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.listOrganizationMembers(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var members []*types.OrganizationMembership
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &members))
	require.Len(t, members, 1, "only the org-wide invitation should be surfaced as a placeholder")
	require.Equal(t, "oin_orgwide", members[0].UserID)
}

func TestAddOrganizationMember_DuplicateInvitation_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_dup"
	ownerID := "user_owner"
	existing := &types.OrganizationInvitation{
		ID:             "oin_existing",
		OrganizationID: orgID,
		Email:          "dup@example.com",
		Role:           types.OrganizationRoleMember,
	}

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "dup@example.com"}).
		Return(nil, store.ErrNotFound)

	// Store returns the existing invitation + sentinel — handler should
	// treat it as a no-op success.
	mockStore.EXPECT().CreateOrganizationInvitation(gomock.Any(), gomock.Any()).
		Return(existing, store.ErrInvitationAlreadyExists)

	body, _ := json.Marshal(types.AddOrganizationMemberRequest{UserReference: "dup@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp types.AddOrganizationMemberResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.True(t, resp.Invited)
	require.NotNil(t, resp.Invitation)
	require.Equal(t, existing.ID, resp.Invitation.ID)
}

func TestAddOrganizationMember_NonOwnerForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_no_owner"
	userID := "user_member"

	expectResolveOrganizationByID(mockStore, orgID)
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	body, _ := json.Marshal(types.AddOrganizationMemberRequest{UserReference: "x@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/members", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.addOrganizationMember(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
}

func TestListOrganizationMembers_IncludesPendingInvitations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_list"
	userID := "user_caller"

	expectResolveOrganizationByID(mockStore, orgID)
	// authorizeOrgMember just needs a membership row to exist
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{
		OrganizationID: orgID,
	}).Return([]*types.OrganizationMembership{
		{OrganizationID: orgID, UserID: "user_real", Role: types.OrganizationRoleMember},
	}, nil)
	mockStore.EXPECT().ListOrganizationInvitations(gomock.Any(), &store.ListOrganizationInvitationsQuery{
		OrganizationID: orgID,
	}).Return([]*types.OrganizationInvitation{
		{ID: "oin_pending", OrganizationID: orgID, Email: "pending@example.com", Role: types.OrganizationRoleOwner},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/members", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.listOrganizationMembers(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var members []*types.OrganizationMembership
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &members))
	require.Len(t, members, 2)
	// Last row is the synthesised invitation placeholder
	require.Equal(t, "oin_pending", members[1].UserID, "invitation id must be exposed as user_id so the UI can detect it")
	require.Equal(t, "pending@example.com", members[1].User.Email)
	require.Equal(t, types.OrganizationRoleOwner, members[1].Role)
}

func TestDeleteOrganizationInvitation_OK(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_del"
	ownerID := "user_owner"
	invitationID := "oin_revoke"

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)
	mockStore.EXPECT().GetOrganizationInvitation(gomock.Any(), &store.GetOrganizationInvitationQuery{ID: invitationID}).
		Return(&types.OrganizationInvitation{ID: invitationID, OrganizationID: orgID, Email: "x@example.com"}, nil)
	mockStore.EXPECT().DeleteOrganizationInvitation(gomock.Any(), invitationID).Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID+"/invitations/"+invitationID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID, "invitation_id": invitationID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.deleteOrganizationInvitation(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestDeleteOrganizationInvitation_DifferentOrg_NotFound(t *testing.T) {
	// Defence-in-depth: an owner of org A must not be able to revoke an
	// invitation that belongs to org B by guessing the ID. The handler
	// loads the invitation, sees a mismatched org, and returns 404 — we
	// deliberately don't differentiate from "not found" to avoid leaking
	// existence.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	callerOrg := "org_a"
	ownerID := "user_owner"
	invitationID := "oin_in_other_org"

	expectResolveOrganizationByID(mockStore, callerOrg)
	expectOrgOwner(mockStore, callerOrg, ownerID)
	mockStore.EXPECT().GetOrganizationInvitation(gomock.Any(), &store.GetOrganizationInvitationQuery{ID: invitationID}).
		Return(&types.OrganizationInvitation{ID: invitationID, OrganizationID: "org_b"}, nil)
	// No DeleteOrganizationInvitation call expected.

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+callerOrg+"/invitations/"+invitationID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": callerOrg, "invitation_id": invitationID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.deleteOrganizationInvitation(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestLookupOrgUser_StateMatrix(t *testing.T) {
	// Walk through each of the four states the invite UI cares about:
	//  - email doesn't belong to any user, no invitation → not_helix
	//  - user exists, not in org, no invitation → not_in_org
	//  - user exists, in org → in_org
	//  - invitation pending → is_invited=true (takes precedence in UI)
	cases := []struct {
		name           string
		userLookup     func(*store.MockStore)
		membership     func(*store.MockStore, string)
		invitation     func(*store.MockStore)
		wantExists     bool
		wantIsMember   bool
		wantIsInvited  bool
	}{
		{
			name: "not_helix",
			userLookup: func(m *store.MockStore) {
				m.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			},
			invitation: func(m *store.MockStore) {
				m.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			},
		},
		{
			name: "not_in_org",
			userLookup: func(m *store.MockStore) {
				m.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "user_known"}, nil)
			},
			membership: func(m *store.MockStore, orgID string) {
				m.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
					OrganizationID: orgID,
					UserID:         "user_known",
				}).Return(nil, store.ErrNotFound)
			},
			invitation: func(m *store.MockStore) {
				m.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			},
			wantExists: true,
		},
		{
			name: "in_org",
			userLookup: func(m *store.MockStore) {
				m.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "user_known"}, nil)
			},
			membership: func(m *store.MockStore, orgID string) {
				m.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
					OrganizationID: orgID,
					UserID:         "user_known",
				}).Return(&types.OrganizationMembership{
					OrganizationID: orgID,
					UserID:         "user_known",
					Role:           types.OrganizationRoleMember,
				}, nil)
			},
			invitation: func(m *store.MockStore) {
				m.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			},
			wantExists:   true,
			wantIsMember: true,
		},
		{
			name: "already_invited",
			userLookup: func(m *store.MockStore) {
				m.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
			},
			invitation: func(m *store.MockStore) {
				m.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).
					Return(&types.OrganizationInvitation{ID: "oin_pending"}, nil)
			},
			wantIsInvited: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := store.NewMockStore(ctrl)
			server := newTestServerNoNotifier(mockStore)

			orgID := "org_lookup"
			ownerID := "user_owner"

			expectResolveOrganizationByID(mockStore, orgID)
			// Owner check is the first DB hit — it must come BEFORE the
			// later GetOrganizationMembership for the target user. Since
			// they share a method name on the mock, we use distinct query
			// matchers (different UserID) to disambiguate.
			mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
				OrganizationID: orgID,
				UserID:         ownerID,
			}).Return(&types.OrganizationMembership{
				OrganizationID: orgID,
				UserID:         ownerID,
				Role:           types.OrganizationRoleOwner,
			}, nil)

			tc.userLookup(mockStore)
			if tc.membership != nil {
				tc.membership(mockStore, orgID)
			}
			tc.invitation(mockStore)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/users/lookup?email=Test@Example.com", nil)
			req = mux.SetURLVars(req, map[string]string{"id": orgID})
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

			rr := httptest.NewRecorder()
			server.lookupOrgUser(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			var resp types.OrgUserLookupResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			require.Equal(t, "test@example.com", resp.Email, "email must be normalised lowercase")
			require.Equal(t, tc.wantExists, resp.Exists, "exists")
			require.Equal(t, tc.wantIsMember, resp.IsMember, "is_member")
			require.Equal(t, tc.wantIsInvited, resp.IsInvited, "is_invited")
		})
	}
}

func TestLookupOrgUser_RequiresEmail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	orgID := "org_lookup"
	ownerID := "user_owner"

	expectResolveOrganizationByID(mockStore, orgID)
	expectOrgOwner(mockStore, orgID, ownerID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/users/lookup?email=", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.lookupOrgUser(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPublicInvitationInfo_OK(t *testing.T) {
	// Unauthenticated endpoint — only the invitation id is required.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	invID := "oin_public"
	orgID := "org_public"

	mockStore.EXPECT().GetOrganizationInvitation(gomock.Any(), &store.GetOrganizationInvitationQuery{ID: invID}).
		Return(&types.OrganizationInvitation{
			ID:             invID,
			OrganizationID: orgID,
			Email:          "invited@example.com",
		}, nil)
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: orgID}).
		Return(&types.Organization{ID: orgID, Name: "acme", DisplayName: "Acme Corp"}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/"+invID+"/info", nil)
	req = mux.SetURLVars(req, map[string]string{"id": invID})

	rr := httptest.NewRecorder()
	server.publicInvitationInfo(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var info types.PublicInvitationInfo
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &info))
	require.Equal(t, invID, info.ID)
	require.Equal(t, "invited@example.com", info.Email)
	require.Equal(t, "Acme Corp", info.OrganizationDisplayName)
	require.Equal(t, "acme", info.OrganizationName)
}

func TestPublicInvitationInfo_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	mockStore.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/oin_missing/info", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "oin_missing"})

	rr := httptest.NewRecorder()
	server.publicInvitationInfo(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPublicInvitationInfo_TolerantOfMissingOrg(t *testing.T) {
	// Edge case: invitation exists but the org has since been deleted.
	// We still return the email so the recipient can register; org fields
	// are simply empty.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := newTestServerNoNotifier(mockStore)

	mockStore.EXPECT().GetOrganizationInvitation(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationInvitation{ID: "oin_orphan", OrganizationID: "org_gone", Email: "orphan@example.com"}, nil)
	mockStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("org gone"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/invitations/oin_orphan/info", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "oin_orphan"})

	rr := httptest.NewRecorder()
	server.publicInvitationInfo(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var info types.PublicInvitationInfo
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &info))
	require.Equal(t, "orphan@example.com", info.Email)
	require.Empty(t, info.OrganizationDisplayName)
}
