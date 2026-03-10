package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestListOrgAPIKeys_OwnerSeesAllKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_owner"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleOwner,
	}, nil)

	mockStore.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{
		OrganizationID: orgID,
		Type:           types.APIkeytypeAPI,
	}).Return([]*types.ApiKey{
		{Key: "key_1", Name: "Key 1", Owner: userID, OrganizationID: orgID},
		{Key: "key_2", Name: "Key 2", Owner: "user_other", OrganizationID: orgID},
	}, nil)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).Return(&types.User{
		ID: userID, Email: "owner@example.com",
	}, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "user_other"}).Return(&types.User{
		ID: "user_other", Email: "other@example.com",
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/api_keys", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.listOrgAPIKeys(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var keys []orgAPIKeyResponse
	err := json.Unmarshal(rr.Body.Bytes(), &keys)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Equal(t, "owner@example.com", keys[0].OwnerEmail)
	require.Equal(t, "other@example.com", keys[1].OwnerEmail)
}

func TestListOrgAPIKeys_MemberSeesOnlyOwnKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_member"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	mockStore.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{
		OrganizationID: orgID,
		Type:           types.APIkeytypeAPI,
		Owner:          userID,
		OwnerType:      types.OwnerTypeUser,
	}).Return([]*types.ApiKey{
		{Key: "key_1", Name: "My Key", Owner: userID, OrganizationID: orgID},
	}, nil)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).Return(&types.User{
		ID: userID, Email: "member@example.com",
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/api_keys", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID:   userID,
		Type: types.OwnerTypeUser,
	}))

	rr := httptest.NewRecorder()
	server.listOrgAPIKeys(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var keys []orgAPIKeyResponse
	err := json.Unmarshal(rr.Body.Bytes(), &keys)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, userID, keys[0].Owner)
	require.Equal(t, "member@example.com", keys[0].OwnerEmail)
}

func TestListOrgAPIKeys_NonMemberForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_outsider"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(nil, store.ErrNotFound)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/api_keys", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.listOrgAPIKeys(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
}

func TestListOrgAPIKeys_FiltersOutSpecTaskKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_owner"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleOwner,
	}, nil)

	mockStore.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{
		OrganizationID: orgID,
		Type:           types.APIkeytypeAPI,
	}).Return([]*types.ApiKey{
		{Key: "key_1", Name: "Regular Key", Owner: userID, OrganizationID: orgID},
		{Key: "key_2", Name: "Spec Task Key", Owner: userID, OrganizationID: orgID, SpecTaskID: "spt_01khrn2nzk7pcwt8zczmwpjtwy"},
		{Key: "key_3", Name: "Another Regular", Owner: userID, OrganizationID: orgID},
	}, nil)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).Return(&types.User{
		ID: userID, Email: "owner@example.com",
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/api_keys", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.listOrgAPIKeys(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var keys []orgAPIKeyResponse
	err := json.Unmarshal(rr.Body.Bytes(), &keys)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Equal(t, "Regular Key", keys[0].Name)
	require.Equal(t, "Another Regular", keys[1].Name)
}

func TestCreateOrgAPIKey_MemberCanCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_member"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	mockStore.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, key *types.ApiKey) (*types.ApiKey, error) {
			require.Equal(t, "My New Key", key.Name)
			require.Equal(t, orgID, key.OrganizationID)
			require.Equal(t, userID, key.Owner)
			require.Equal(t, types.APIkeytypeAPI, key.Type)
			require.NotEmpty(t, key.Key)
			return key, nil
		},
	)

	body, _ := json.Marshal(map[string]string{"name": "My New Key"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/api_keys", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID:   userID,
		Type: types.OwnerTypeUser,
	}))

	rr := httptest.NewRecorder()
	server.createOrgAPIKey(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var created types.ApiKey
	err := json.Unmarshal(rr.Body.Bytes(), &created)
	require.NoError(t, err)
	require.Equal(t, "My New Key", created.Name)
}

func TestCreateOrgAPIKey_EmptyNameRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	userID := "user_member"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	body, _ := json.Marshal(map[string]string{"name": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations/"+orgID+"/api_keys", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

	rr := httptest.NewRecorder()
	server.createOrgAPIKey(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestDeleteOrgAPIKey_OwnerCanDeleteAnyKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	ownerID := "user_owner"
	keyStr := "hl-key-abc123"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         ownerID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         ownerID,
		Role:           types.OrganizationRoleOwner,
	}, nil)

	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: keyStr}).Return(&types.ApiKey{
		Key:            keyStr,
		Owner:          "user_other",
		OrganizationID: orgID,
	}, nil)

	mockStore.EXPECT().DeleteAPIKey(gomock.Any(), keyStr).Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID+"/api_keys/"+keyStr, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID, "key": keyStr})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.deleteOrgAPIKey(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestDeleteOrgAPIKey_MemberCanOnlyDeleteOwnKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	memberID := "user_member"
	keyStr := "hl-key-abc123"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         memberID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         memberID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: keyStr}).Return(&types.ApiKey{
		Key:            keyStr,
		Owner:          "user_other", // belongs to someone else
		OrganizationID: orgID,
	}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID+"/api_keys/"+keyStr, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID, "key": keyStr})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: memberID}))

	rr := httptest.NewRecorder()
	server.deleteOrgAPIKey(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
}

func TestDeleteOrgAPIKey_KeyFromDifferentOrgRejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	ownerID := "user_owner"
	keyStr := "hl-key-abc123"

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         ownerID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         ownerID,
		Role:           types.OrganizationRoleOwner,
	}, nil)

	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: keyStr}).Return(&types.ApiKey{
		Key:            keyStr,
		Owner:          "user_other",
		OrganizationID: "org_different", // different org
	}, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID+"/api_keys/"+keyStr, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID, "key": keyStr})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: ownerID}))

	rr := httptest.NewRecorder()
	server.deleteOrgAPIKey(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestListOrgAPIKeys_AdminSeesAllKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	orgID := "org_123"
	adminID := "user_admin"

	// Admin user - authorizeOrgMember tries to get membership, falls back to synthetic owner
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         adminID,
	}).Return(nil, store.ErrNotFound)

	mockStore.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{
		OrganizationID: orgID,
		Type:           types.APIkeytypeAPI,
	}).Return([]*types.ApiKey{
		{Key: "key_1", Name: "Key 1", Owner: "user_1", OrganizationID: orgID},
		{Key: "key_2", Name: "Key 2", Owner: "user_2", OrganizationID: orgID},
	}, nil)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "user_1"}).Return(&types.User{
		ID: "user_1", Email: "user1@example.com",
	}, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "user_2"}).Return(&types.User{
		ID: "user_2", Email: "user2@example.com",
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgID+"/api_keys", nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: adminID, Admin: true}))

	rr := httptest.NewRecorder()
	server.listOrgAPIKeys(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var keys []orgAPIKeyResponse
	err := json.Unmarshal(rr.Body.Bytes(), &keys)
	require.NoError(t, err)
	require.Len(t, keys, 2)
}
