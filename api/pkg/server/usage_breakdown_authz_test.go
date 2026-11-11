package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func getUsersDailyUsage(t *testing.T, server *HelixAPIServer, user types.User, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider-endpoints/"+id+"/users-daily-usage", nil)
	req = req.WithContext(setRequestUser(context.Background(), user))
	req = mux.SetURLVars(req, map[string]string{"id": id})
	rr := httptest.NewRecorder()
	server.getProviderUsersDailyUsage(rr, req)
	return rr
}

// Every user can see a deployment-wide provider, but its per-user breakdown
// is everybody else's activity.
func TestProviderUsersDailyUsage_GlobalProviderIsAdminOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockManager := manager.NewMockProviderManager(ctrl)
	mockStore.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{ProvidersManagementEnabled: true}, nil).AnyTimes()
	mockManager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{types.ProviderOpenAI}, nil).AnyTimes()
	mockStore.EXPECT().GetUsersAggregatedUsageMetrics(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore, providerManager: mockManager}
	rr := getUsersDailyUsage(t, server, types.User{ID: "usr_bob"}, string(types.ProviderOpenAI))

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

// A stored endpoint of the global type is used by every user too, even when
// its owner is not an admin.
func TestProviderUsersDailyUsage_StoredGlobalEndpointIsAdminOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockManager := manager.NewMockProviderManager(ctrl)
	mockStore.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{ProvidersManagementEnabled: true}, nil).AnyTimes()
	mockManager.EXPECT().ListProviders(gomock.Any(), "").Return(nil, nil).AnyTimes()
	mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), gomock.Any()).Return(&types.ProviderEndpoint{
		ID: "pe_global", Owner: "usr_bob", OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeGlobal,
	}, nil).AnyTimes()
	mockStore.EXPECT().GetUsersAggregatedUsageMetrics(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore, providerManager: mockManager}
	rr := getUsersDailyUsage(t, server, types.User{ID: "usr_bob"}, "pe_global")

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestProviderUsersDailyUsage_RedactsUserRecords(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockManager := manager.NewMockProviderManager(ctrl)
	mockStore.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{ProvidersManagementEnabled: true}, nil).AnyTimes()
	mockManager.EXPECT().ListProviders(gomock.Any(), "").Return(nil, nil).AnyTimes()
	mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), gomock.Any()).
		Return(&types.ProviderEndpoint{ID: "pe_1", Owner: "usr_bob", OwnerType: types.OwnerTypeUser}, nil).AnyTimes()
	mockStore.EXPECT().GetUsersAggregatedUsageMetrics(gomock.Any(), "pe_1", gomock.Any(), gomock.Any()).
		Return([]*types.UsersAggregatedUsageMetric{{User: types.User{
			ID: "usr_carol", Email: "carol@example.com", FullName: "Carol",
			Token: "hl-stored-token", Admin: true,
		}}}, nil)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore, providerManager: mockManager}
	rr := getUsersDailyUsage(t, server, types.User{ID: "usr_bob"}, "pe_1")

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.NotContains(t, rr.Body.String(), "hl-stored-token")
	var metrics []*types.UsersAggregatedUsageMetric
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &metrics))
	require.Len(t, metrics, 1)
	require.Equal(t, "carol@example.com", metrics[0].User.Email)
	require.False(t, metrics[0].User.Admin)
}

// Read access to a global app is granted to every user; the per-user usage
// breakdown is not.
func TestAppUsersDailyUsage_GlobalAppNeedsManageAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_global").
		Return(&types.App{ID: "app_global", Owner: "usr_admin", Global: true}, nil)
	mockStore.EXPECT().GetAppUsersAggregatedUsageMetrics(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	server := &HelixAPIServer{Store: mockStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app_global/users-daily-usage", nil)
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob"}))
	req = mux.SetURLVars(req, map[string]string{"id": "app_global"})

	_, httpErr := server.getAppUsersDailyUsage(httptest.NewRecorder(), req)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusForbidden, httpErr.StatusCode)
}
