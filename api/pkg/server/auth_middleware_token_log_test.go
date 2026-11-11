package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A bearer token that fails validation may still be a live credential (an
// expired-but-replayable token, a token for another deployment, a key pasted
// into the wrong header). It must not be written to the logs.
func TestGetUserFromToken_DoesNotLogRejectedToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockAuth := auth.NewMockAuthenticator(ctrl)
	mockAuth.EXPECT().ValidateUserToken(gomock.Any(), gomock.Any()).Return(nil, errors.New("token is expired"))

	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = previous })

	middleware := &authMiddleware{
		authenticator: mockAuth,
		store:         store.NewMockStore(ctrl),
	}

	const token = "eyJhbGciOiJIUzI1NiJ9.secret-bearer-payload.signature"
	_, err := middleware.getUserFromToken(context.Background(), token)
	require.Error(t, err)

	require.NotEmpty(t, buf.String(), "the rejection is still logged")
	require.NotContains(t, buf.String(), "secret-bearer-payload")
}

// A failed external-agent runner handshake must not log the configured
// runner token (which grants runner and admin-or-runner access) or the token
// the caller sent.
func TestExternalAgentRunnerRejectedTokenIsNotLogged(t *testing.T) {
	var buf bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&buf)
	t.Cleanup(func() { log.Logger = previous })

	cfg := &config.ServerConfig{}
	cfg.WebServer.RunnerToken = "configured-runner-token-value"
	server := &HelixAPIServer{Cfg: cfg}
	router := mux.NewRouter()
	server.startExternalAgentRunnerWebSocketServer(router, "/ws/external-agent-runner")

	req := httptest.NewRequest(http.MethodGet, "/ws/external-agent-runner?runnerid=r1&access_token=guessed-token-value", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnauthorized, rr.Code)
	require.NotContains(t, buf.String(), "configured-runner-token-value")
	require.NotContains(t, buf.String(), "guessed-token-value")
}
