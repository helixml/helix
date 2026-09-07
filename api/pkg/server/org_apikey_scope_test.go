package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func orgScopedKeyUser(userID, orgID string) types.User {
	return types.User{
		ID:             userID,
		Type:           types.OwnerTypeUser,
		Token:          "oh-key",
		TokenType:      types.TokenTypeAPIKey,
		APIKeyType:     types.APIkeytypeAPI,
		OrganizationID: orgID,
	}
}

func TestIsOrgScopedKey(t *testing.T) {
	tests := []struct {
		name string
		user types.User
		want bool
	}{
		{
			name: "org api key",
			user: orgScopedKeyUser("user-1", "org_a"),
			want: true,
		},
		{
			name: "session key is not org scoped",
			user: func() types.User {
				u := orgScopedKeyUser("user-1", "org_a")
				u.SessionID = "ses_1"
				return u
			}(),
			want: false,
		},
		{
			name: "spec task key is not org scoped",
			user: func() types.User {
				u := orgScopedKeyUser("user-1", "org_a")
				u.SpecTaskID = "task_1"
				return u
			}(),
			want: false,
		},
		{
			name: "key with no org is not org scoped",
			user: types.User{
				ID:         "user-1",
				TokenType:  types.TokenTypeAPIKey,
				APIKeyType: types.APIkeytypeAPI,
			},
			want: false,
		},
		{
			name: "keycloak session is not org scoped",
			user: types.User{
				ID:             "user-1",
				TokenType:      types.TokenTypeOIDC,
				OrganizationID: "org_a",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := tt.user
			assert.Equal(t, tt.want, isOrgScopedKey(&u))
		})
	}

	assert.False(t, isOrgScopedKey(nil))
}

func TestEnforceKeyOrgScope(t *testing.T) {
	user := orgScopedKeyUser("user-1", "org_a")

	err := enforceKeyOrgScope(&user, "org_b")
	require.Error(t, err)
	assert.ErrorIs(t, err, errOrgScopedKey)

	assert.NoError(t, enforceKeyOrgScope(&user, "org_a"))

	plain := types.User{ID: "user-1"}
	assert.NoError(t, enforceKeyOrgScope(&plain, "org_b"))
	assert.NoError(t, enforceKeyOrgScope(nil, "org_b"))
}

// A creator who belongs to several organizations must not be able to use a
// key minted for one of them to authorize against another.
func TestAuthorizeOrgMember_OrgScopedKeyRejectedForOtherOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	user := orgScopedKeyUser("user_1", "org_a")

	_, err := server.authorizeOrgMember(context.Background(), &user, "org_b")
	require.Error(t, err)
	assert.ErrorIs(t, err, errOrgScopedKey)
}

func TestAuthorizeOrgOwner_OrgScopedKeyRejectedForOtherOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	user := orgScopedKeyUser("user_1", "org_a")

	_, err := server.authorizeOrgOwner(context.Background(), &user, "org_b")
	require.Error(t, err)
	assert.ErrorIs(t, err, errOrgScopedKey)
}

// Same-org use of an org key still resolves membership normally.
func TestAuthorizeOrgMember_OrgScopedKeyAllowedForOwnOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_a"
	user := orgScopedKeyUser("user_1", orgID)

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         "user_1",
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         "user_1",
		Role:           types.OrganizationRoleMember,
	}, nil)

	membership, err := server.authorizeOrgMember(context.Background(), &user, orgID)
	require.NoError(t, err)
	assert.Equal(t, types.OrganizationRoleMember, membership.Role)
}

// Even for a global-admin creator, resolveOrgID refuses to point an org key
// at another organization.
func TestResolveOrgID_OrgScopedKeyRejectedForOtherOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	otherOrgID := "org_b"
	expectResolveOrganizationByID(mockStore, otherOrgID)

	user := orgScopedKeyUser("admin_user", "org_a")
	user.Admin = true

	ctx := context.WithValue(context.Background(), userKey, user)

	_, err := server.resolveOrgID(ctx, otherOrgID)
	require.Error(t, err)
	assert.ErrorIs(t, err, errOrgScopedKey)
}

func TestResolveOrgID_OrgScopedKeyResolvesOwnOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_a"
	expectResolveOrganizationByID(mockStore, orgID)

	user := orgScopedKeyUser("user_1", orgID)
	ctx := context.WithValue(context.Background(), userKey, user)

	resolved, err := server.resolveOrgID(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, orgID, resolved)
}

// No user in context (runner/socket paths) is unaffected.
func TestResolveOrgID_NoUserInContextUnaffected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_b"
	expectResolveOrganizationByID(mockStore, orgID)

	resolved, err := server.resolveOrgID(context.Background(), orgID)
	require.NoError(t, err)
	assert.Equal(t, orgID, resolved)
}

// End-to-end through the handler: an org-A key hitting another org's
// api_keys route is rejected before any membership lookup.
func TestListOrgAPIKeys_OrgScopedKeyRejectedForOtherOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	otherOrgID := "org_b"
	expectResolveOrganizationByID(mockStore, otherOrgID)

	user := orgScopedKeyUser("user_1", "org_a")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+otherOrgID+"/api_keys", nil)
	req = mux.SetURLVars(req, map[string]string{"id": otherOrgID})
	req = req.WithContext(setRequestUser(req.Context(), user))

	rr := httptest.NewRecorder()
	server.listOrgAPIKeys(rr, req)

	// Cross-org references resolve as "not found" (same as a nonexistent org).
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestCreateOrgAPIKey_OrgScopedKeyRejectedForOtherOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	otherOrgID := "org_b"
	expectResolveOrganizationByID(mockStore, otherOrgID)

	user := orgScopedKeyUser("user_1", "org_a")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+otherOrgID+"/api_keys", nil)
	req = mux.SetURLVars(req, map[string]string{"id": otherOrgID})
	req = req.WithContext(setRequestUser(req.Context(), user))

	rr := httptest.NewRecorder()
	server.createOrgAPIKey(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// The org list shown to an org-scoped key is confined to its organization,
// even when the creator is a member of several.
func TestListOrganizations_OrgScopedKeySeesOnlyItsOrg(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	userID := "user_1"
	orgA, orgB := "org_a", "org_b"
	user := orgScopedKeyUser(userID, orgA)

	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), &store.ListOrganizationMembershipsQuery{
		UserID: userID,
	}).Return([]*types.OrganizationMembership{
		{OrganizationID: orgA, UserID: userID, Role: types.OrganizationRoleOwner},
		{OrganizationID: orgB, UserID: userID, Role: types.OrganizationRoleMember},
	}, nil)

	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: orgA}).Return(
		&types.Organization{ID: orgA, Name: "Org A"}, nil)
	mockStore.EXPECT().GetProjectsCount(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil)
	req = req.WithContext(setRequestUser(req.Context(), user))

	rr := httptest.NewRecorder()
	server.listOrganizations(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var orgs []types.Organization
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &orgs))
	require.Len(t, orgs, 1)
	assert.Equal(t, orgA, orgs[0].ID)
}

// Authenticating an org API key must not hand the caller the creator's
// global admin flag, whichever way the creator is an admin.
func TestGetUserFromToken_OrgAPIKeyDoesNotInheritAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	auth := newAuthMiddleware(nil, nil, mockStore, authMiddlewareConfig{
		adminUserIDs: []string{"user_admin"},
	}, nil, nil)

	creatorID := "user_admin"
	keyStr := types.APIKeyPrefix + "abc123"

	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: keyStr}).Return(&types.ApiKey{
		Key:            keyStr,
		Owner:          creatorID,
		OwnerType:      types.OwnerTypeUser,
		Type:           types.APIkeytypeAPI,
		OrganizationID: "org_a",
	}, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: creatorID}).Return(
		&types.User{ID: creatorID, Admin: true}, nil)
	mockStore.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: creatorID}, nil)

	user, err := auth.getUserFromToken(context.Background(), keyStr)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "org_a", user.OrganizationID)
	assert.False(t, user.Admin)
}

func TestGetUserFromToken_OrgAPIKeyDevModeAdminDoesNotLeak(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	// Dev mode: everyone is an admin via ADMIN_USER_IDS=all.
	auth := newAuthMiddleware(nil, nil, mockStore, authMiddlewareConfig{
		adminUserIDs: []string{"all"},
	}, nil, nil)

	creatorID := "user_1"
	keyStr := types.APIKeyPrefix + "dev123"

	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: keyStr}).Return(&types.ApiKey{
		Key:            keyStr,
		Owner:          creatorID,
		OwnerType:      types.OwnerTypeUser,
		Type:           types.APIkeytypeAPI,
		OrganizationID: "org_a",
	}, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: creatorID}).Return(
		&types.User{ID: creatorID}, nil)
	mockStore.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: creatorID}, nil)

	user, err := auth.getUserFromToken(context.Background(), keyStr)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.False(t, user.Admin)
}

// Non-org keys keep their existing admin resolution.
func TestGetUserFromToken_NonOrgKeyKeepsAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	auth := newAuthMiddleware(nil, nil, mockStore, authMiddlewareConfig{
		adminUserIDs: []string{"user_admin"},
	}, nil, nil)

	creatorID := "user_admin"
	keyStr := types.APIKeyPrefix + "plain123"

	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: keyStr}).Return(&types.ApiKey{
		Key:       keyStr,
		Owner:     creatorID,
		OwnerType: types.OwnerTypeUser,
		Type:      types.APIkeytypeAPI,
	}, nil)
	// Owner load; the admin check short-circuits on the ADMIN_USER_IDS list.
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: creatorID}).Return(
		&types.User{ID: creatorID}, nil)
	mockStore.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: creatorID}, nil)

	user, err := auth.getUserFromToken(context.Background(), keyStr)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.True(t, user.Admin)
}
