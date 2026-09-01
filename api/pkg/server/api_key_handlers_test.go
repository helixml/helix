package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetAPIKeysReturnsStableLatestPersonalKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	user := &types.User{ID: "user_1", Type: types.OwnerTypeUser}
	older := &types.ApiKey{
		Created:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Owner:     user.ID,
		OwnerType: user.Type,
		Key:       "hl-older",
		Name:      "API Key",
		Type:      types.APIkeytypeAPI,
	}
	newer := &types.ApiKey{
		Created:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Owner:     user.ID,
		OwnerType: user.Type,
		Key:       "hl-newer",
		Name:      "API Key",
		Type:      types.APIkeytypeAPI,
	}
	organizationKey := &types.ApiKey{
		Created:        time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
		Owner:          user.ID,
		OwnerType:      user.Type,
		Key:            "hl-organization",
		Name:           "Organization Key",
		Type:           types.APIkeytypeAPI,
		OrganizationID: "org_1",
	}
	allKeys := []*types.ApiKey{older, organizationKey, newer}

	mockStore.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{
		Owner: user.ID, OwnerType: user.Type, Type: types.APIkeytypeAPI,
	}).Return(allKeys, nil).Times(2)
	mockStore.EXPECT().ListAPIKeys(gomock.Any(), &store.ListAPIKeysQuery{
		Owner: user.ID, OwnerType: user.Type,
	}).Return(allKeys, nil).Times(2)

	server := &HelixAPIServer{
		Controller: &controller.Controller{Options: controller.Options{Store: mockStore}},
	}

	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/api_keys", nil)
		req = req.WithContext(setRequestUser(context.Background(), *user))
		keys, err := server.getAPIKeys(nil, req)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		require.Equal(t, newer.Key, keys[0].Key)
	}
}

func TestPersonalAPIKeyRejectsScopedKeys(t *testing.T) {
	base := types.ApiKey{Type: types.APIkeytypeAPI}
	require.True(t, isPersonalAPIKey(&base))

	for name, mutate := range map[string]func(*types.ApiKey){
		"organization": func(key *types.ApiKey) { key.OrganizationID = "org_1" },
		"project":      func(key *types.ApiKey) { key.ProjectID = "prj_1" },
		"spec task":    func(key *types.ApiKey) { key.SpecTaskID = "spt_1" },
		"session":      func(key *types.ApiKey) { key.SessionID = "ses_1" },
	} {
		t.Run(name, func(t *testing.T) {
			key := base
			mutate(&key)
			require.False(t, isPersonalAPIKey(&key))
		})
	}
}
