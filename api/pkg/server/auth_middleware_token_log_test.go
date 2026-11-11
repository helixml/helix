package server

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/auth"
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
