package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateSecretRejectsUnknownProject(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_missing").Return(nil, store.ErrNotFound)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewBufferString(
		`{"name":"TOKEN","value":"secret","project_id":"prj_missing"}`,
	))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test"}))
	_, httpErr := server.createSecret(httptest.NewRecorder(), req)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}

func TestUpdateSecretRejectsOrphanedProjectScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	secret := &types.Secret{ID: "sec_test", Name: "TOKEN", Owner: "user_test", ProjectID: "prj_missing"}
	mockStore.EXPECT().GetSecret(gomock.Any(), secret.ID).Return(secret, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), secret.ProjectID).Return(nil, store.ErrNotFound)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/sec_test", bytes.NewBufferString(
		`{"name":"TOKEN","value":"rotated"}`,
	))
	req = mux.SetURLVars(req, map[string]string{"id": secret.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user_test"}))
	_, httpErr := server.updateSecret(httptest.NewRecorder(), req)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusNotFound, httpErr.StatusCode)
}
