package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A global agent is shown to and runnable by every user, so only an admin
// may publish one.
func TestCreateAgent_NonAdminCannotCreateGlobal(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().CreateApp(gomock.Any(), gomock.Any()).Times(0)
	mockStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
	for name, body := range map[string]string{
		"legacy":     `{"global":true,"config":{"helix":{"name":"everyone","assistants":[{"name":"a"}]}}}`,
		"structured": `{"global":true,"yaml_config":{"name":"everyone","assistants":[{"name":"a"}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", strings.NewReader(body))
			req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob"}))
			_, httpErr := server.createAgent(httptest.NewRecorder(), req)
			require.NotNil(t, httpErr)
			require.Equal(t, http.StatusForbidden, httpErr.StatusCode, httpErr.Message)
		})
	}
}

func TestUpdateAgent_OwnerCannotMakeAgentGlobal(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").Return(&types.App{ID: "app_1", Owner: "usr_bob"}, nil)
	mockStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).Times(0)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/app_1",
		strings.NewReader(`{"global":true,"config":{"helix":{"name":"mine","assistants":[{"name":"a"}]}}}`))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob"}))
	req = mux.SetURLVars(req, map[string]string{"id": "app_1"})

	_, httpErr := server.updateAgent(httptest.NewRecorder(), req)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusForbidden, httpErr.StatusCode, httpErr.Message)
}

// A non-admin owner of an existing global agent may still save it: leaving
// "global" out of the update keeps it, it is not a request to change it.
func TestUpdateAgent_OmittedGlobalKeepsStoredValue(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").Return(&types.App{ID: "app_1", Owner: "usr_bob", Global: true}, nil)
	var saved *types.App
	mockStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) { saved = app; return app, nil })
	mockStore.EXPECT().ListTriggerConfigurations(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/app_1",
		strings.NewReader(`{"config":{"helix":{"name":"mine","assistants":[{"name":"a"}]}}}`))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob"}))
	req = mux.SetURLVars(req, map[string]string{"id": "app_1"})

	_, httpErr := server.updateAgent(httptest.NewRecorder(), req)
	require.Nil(t, httpErr)
	require.NotNil(t, saved)
	require.True(t, saved.Global)
}
