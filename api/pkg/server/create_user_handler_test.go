package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateUser_OIDCMode_Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockAuth := auth.NewMockAuthenticator(ctrl)

	// In OIDC mode, neither Store.GetUser nor authenticator.CreateUser must be called.
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Times(0)
	mockAuth.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Times(0)

	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderOIDC

	server := &HelixAPIServer{
		Cfg:           cfg,
		Store:         mockStore,
		authenticator: mockAuth,
	}

	body, err := json.Marshal(types.AdminCreateUserRequest{
		Email:    "new-user@example.com",
		Password: "irrelevant",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := setTestRequestUser(req.Context(), &types.User{
		ID:    "admin-123",
		Email: "admin@example.com",
		Admin: true,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	result, httpErr := server.createUser(rr, req)

	require.Nil(t, result)
	require.NotNil(t, httpErr)
	assert.Contains(t, httpErr.Error(), "OIDC provider")

	var sysErr *system.HTTPError
	require.True(t, errors.As(httpErr, &sysErr), "expected *system.HTTPError")
	assert.Equal(t, http.StatusBadRequest, sysErr.StatusCode)
}

func TestCreateUser_NonAdmin_Forbidden(t *testing.T) {
	// Sanity check: the existing non-admin guard still fires before the OIDC guard.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockAuth := auth.NewMockAuthenticator(ctrl)

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Times(0)
	mockAuth.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Times(0)

	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderOIDC

	server := &HelixAPIServer{
		Cfg:           cfg,
		Store:         mockStore,
		authenticator: mockAuth,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	ctx := setTestRequestUser(req.Context(), &types.User{
		ID:    "user-123",
		Email: "user@example.com",
		Admin: false,
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	_, httpErr := server.createUser(rr, req)

	require.NotNil(t, httpErr)
	var sysErr *system.HTTPError
	require.True(t, errors.As(httpErr, &sysErr), "expected *system.HTTPError")
	assert.Equal(t, http.StatusForbidden, sysErr.StatusCode)
}
