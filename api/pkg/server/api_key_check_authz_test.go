package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func checkAPIKeyAs(t *testing.T, user types.User) (*types.ApiKey, error) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: "hl-alice"}).Return(
		&types.ApiKey{Key: "hl-alice", Owner: "usr_alice", OwnerType: types.OwnerTypeUser}, nil)
	server := &HelixAPIServer{
		Cfg:        &config.ServerConfig{},
		Store:      mockStore,
		Controller: &controller.Controller{Options: controller.Options{Store: mockStore}},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/api_keys/check?key=hl-alice", nil)
	req = req.WithContext(setRequestUser(context.Background(), user))
	return server.checkAPIKey(httptest.NewRecorder(), req)
}

func TestCheckAPIKey_OnlyReportsCallersOwnKeys(t *testing.T) {
	_, err := checkAPIKeyAs(t, types.User{ID: "usr_bob"})
	require.Error(t, err)
	var httpErr *system.HTTPError
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusNotFound, httpErr.StatusCode)

	key, err := checkAPIKeyAs(t, types.User{ID: "usr_alice"})
	require.NoError(t, err)
	require.Equal(t, "usr_alice", key.Owner)
}
