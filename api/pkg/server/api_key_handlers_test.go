package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
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

// fakeAPIKey builds a key-shaped test fixture at runtime, so no
// secret-looking literal is committed for the secret scanner to flag.
func fakeAPIKey(fill string) string {
	return types.APIKeyPrefix + strings.Repeat(fill, 44)
}

func orgScopedKeyUser() types.User {
	return types.User{
		ID:             "user_1",
		Type:           types.OwnerTypeUser,
		Token:          fakeAPIKey("g"),
		TokenType:      types.TokenTypeAPIKey,
		APIKeyType:     types.APIkeytypeAPI,
		OrganizationID: "org_1",
	}
}

// A scoped key must not be able to read its owner's unscoped personal key.
func TestGetAPIKeys_ScopedKeyCannotReadPersonalKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Controller: &controller.Controller{Options: controller.Options{Store: mockStore}},
	}

	for name, user := range map[string]types.User{
		"organization": orgScopedKeyUser(),
		"session": {
			ID: "user_1", Type: types.OwnerTypeUser, Token: fakeAPIKey("s"),
			TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, SessionID: "ses_1",
		},
		"project": {
			ID: "user_1", Type: types.OwnerTypeUser, Token: fakeAPIKey("j"),
			TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, ProjectID: "prj_1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/api_keys", nil)
			req = req.WithContext(setRequestUser(context.Background(), user))
			keys, err := server.getAPIKeys(nil, req)
			require.Nil(t, keys)
			var httpErr *system.HTTPError
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, http.StatusForbidden, httpErr.StatusCode)
		})
	}
}

// A scoped key listing with a filter sees app keys (confined to chat) and
// itself in full, but only the prefix of any other api-type key.
func TestGetAPIKeys_ScopedKeyListIsRedacted(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Controller: &controller.Controller{Options: controller.Options{Store: mockStore}},
	}
	user := orgScopedKeyUser()
	personal := &types.ApiKey{Owner: user.ID, OwnerType: user.Type, Key: fakeAPIKey("p"), Type: types.APIkeytypeAPI}
	self := &types.ApiKey{Owner: user.ID, OwnerType: user.Type, Key: user.Token, Type: types.APIkeytypeAPI, OrganizationID: "org_1"}
	app := &types.ApiKey{Owner: user.ID, OwnerType: user.Type, Key: fakeAPIKey("a"), Type: types.APIkeytypeApp,
		AppID: &sql.NullString{String: "app_1", Valid: true}}
	all := []*types.ApiKey{personal, self, app}
	mockStore.EXPECT().ListAPIKeys(gomock.Any(), gomock.Any()).Return(all, nil).Times(2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api_keys?types=all", nil)
	req = req.WithContext(setRequestUser(context.Background(), user))
	keys, err := server.getAPIKeys(nil, req)
	require.NoError(t, err)
	require.Len(t, keys, 3)
	require.Equal(t, "hl-pppp", keys[0].Key)
	require.Equal(t, user.Token, keys[1].Key)
	require.Equal(t, app.Key, keys[2].Key)
	// The store's copy must not be mutated.
	require.Equal(t, fakeAPIKey("p"), personal.Key)
}

// A scoped key must not be able to mint an unscoped personal key.
func TestCreateAPIKey_ScopedKeyCannotMintPersonalKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	// No store expectations: any write fails the test.
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Controller: &controller.Controller{Options: controller.Options{Store: mockStore}},
	}

	for name, body := range map[string]string{
		"query": "",
		"body":  `{"name":"x","type":"api","app_id":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			target := "/api/v1/api_keys"
			if body == "" {
				target += "?name=x"
			}
			req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
			req = req.WithContext(setRequestUser(context.Background(), orgScopedKeyUser()))
			key, err := server.createAPIKey(nil, req)
			require.Empty(t, key)
			var httpErr *system.HTTPError
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, http.StatusForbidden, httpErr.StatusCode)
		})
	}
}

// A scoped key may still mint an app key (the CLI does this for bots), and a
// personal key or browser session may mint a personal key.
func TestCreateAPIKey_AllowedCallers(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Controller: &controller.Controller{Options: controller.Options{Store: mockStore}},
	}
	mockStore.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key *types.ApiKey) (*types.ApiKey, error) { return key, nil },
	).Times(3)

	scoped := orgScopedKeyUser()
	personal := types.User{ID: "user_1", Type: types.OwnerTypeUser, Token: "hl-personal", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI}
	session := types.User{ID: "user_1", Type: types.OwnerTypeUser, TokenType: types.TokenTypeSession}

	cases := []struct {
		user   types.User
		target string
		body   string
	}{
		{scoped, "/api/v1/api_keys", `{"name":"bot","type":"app","app_id":"app_1"}`},
		{personal, "/api/v1/api_keys?name=x", ""},
		{session, "/api/v1/api_keys?name=x", ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, c.target, strings.NewReader(c.body))
		req = req.WithContext(setRequestUser(context.Background(), c.user))
		key, err := server.createAPIKey(nil, req)
		require.NoError(t, err)
		require.NotEmpty(t, key)
	}
}
