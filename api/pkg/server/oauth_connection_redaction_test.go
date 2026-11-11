package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func oauthConnectionWithSecrets() *types.OAuthConnection {
	return &types.OAuthConnection{
		ID:           "conn_1",
		UserID:       "user_1",
		ProviderID:   "prov_1",
		AccessToken:  "user-access-token",
		RefreshToken: "user-refresh-token",
		Provider: types.OAuthProvider{
			ID:           "prov_1",
			Name:         "GitHub",
			ClientID:     "client-id",
			ClientSecret: "operator-client-secret",
		},
	}
}

func requireOAuthConnectionRedacted(t *testing.T, connection *types.OAuthConnection) {
	t.Helper()
	require.Empty(t, connection.AccessToken)
	require.Empty(t, connection.RefreshToken)
	require.Empty(t, connection.Provider.ClientSecret, "the provider's client secret must not reach a non-admin")
	require.Equal(t, "client-id", connection.Provider.ClientID, "non-secret provider fields stay")
}

// The connection list preloads each connection's provider. A non-admin must
// get neither the connection tokens nor the provider's client secret.
func TestListOAuthConnections_RedactsProviderClientSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListOAuthConnections(gomock.Any(), &store.ListOAuthConnectionsQuery{UserID: "user_1"}).
		Return([]*types.OAuthConnection{oauthConnectionWithSecrets()}, nil)

	server := &HelixAPIServer{Store: mockStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/connections", nil)
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "user_1"}))

	connections, err := server.handleListOAuthConnections(httptest.NewRecorder(), req)
	require.NoError(t, err)
	require.Len(t, connections, 1)
	requireOAuthConnectionRedacted(t, connections[0])
}

func TestGetOAuthConnection_RedactsProviderClientSecret(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetOAuthConnection(gomock.Any(), "conn_1").Return(oauthConnectionWithSecrets(), nil)

	server := &HelixAPIServer{Store: mockStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/connections/conn_1", nil)
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "user_1"}))
	req = mux.SetURLVars(req, map[string]string{"id": "conn_1"})

	connection, err := server.handleGetOAuthConnection(httptest.NewRecorder(), req)
	require.NoError(t, err)
	requireOAuthConnectionRedacted(t, connection)
}
