package auth

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gopkg.in/go-jose/go-jose.v2"
	"gopkg.in/go-jose/go-jose.v2/jwt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
)

// signWithKey builds a token the way HelixAuthenticator does, but with an
// arbitrary key, to model a token forged with a known secret.
func signWithKey(t *testing.T, key string, claims helixTokenClaims) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte(key)}, nil)
	require.NoError(t, err)
	token, err := jwt.Signed(signer).Claims(claims).CompactSerialize()
	require.NoError(t, err)
	return token
}

func TestDefaultConfigHasNoJWTSecret(t *testing.T) {
	t.Setenv("REGULAR_AUTH_JWT_SECRET", "")
	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)
	require.Empty(t, cfg.Auth.Regular.JWTSecret)
}

func TestTokenSignedWithPublicDefaultIsRejected(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	// The store keeps whatever candidate the first caller offers.
	mockStore.EXPECT().EnsureJWTSigningSecret(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, candidate string) (string, error) { return candidate, nil })

	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)

	secret, err := ResolveJWTSecret(ctx, mockStore, cfg.Auth.Regular.JWTSecret)
	require.NoError(t, err)
	authenticator, err := NewHelixAuthenticator(&cfg, mockStore, secret, nil)
	require.NoError(t, err)

	now := time.Now()
	claims := helixTokenClaims{
		Claims: jwt.Claims{
			Subject:  "forged-admin",
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		},
		UserID: "forged-admin",
		Admin:  true,
	}
	for _, key := range legacyPublicJWTSecrets {
		forged := signWithKey(t, key, claims)

		_, err := authenticator.ValidateUserToken(ctx, forged)
		require.ErrorContains(t, err, "failed to verify token", "key %q", key)

		err = authenticator.PasswordResetComplete(ctx, forged, "new-password-123")
		require.ErrorContains(t, err, "failed to verify token", "key %q", key)
	}
}

func TestResolveJWTSecret_ConfiguredValueWins(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl) // no calls expected

	secret, err := ResolveJWTSecret(context.Background(), mockStore, "operator-provided-secret")
	require.NoError(t, err)
	require.Equal(t, "operator-provided-secret", secret)
}

func TestResolveJWTSecret_LegacyValueIsTreatedAsUnset(t *testing.T) {
	for _, legacy := range legacyPublicJWTSecrets {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		mockStore.EXPECT().EnsureJWTSigningSecret(gomock.Any(), gomock.Any()).Return("stored-secret", nil)

		secret, err := ResolveJWTSecret(context.Background(), mockStore, legacy)
		require.NoError(t, err)
		require.Equal(t, "stored-secret", secret)
	}
}

func TestResolveJWTSecret_GeneratesRandomCandidate(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	var candidates []string
	mockStore.EXPECT().EnsureJWTSigningSecret(gomock.Any(), gomock.Any()).Times(2).
		DoAndReturn(func(_ context.Context, candidate string) (string, error) {
			candidates = append(candidates, candidate)
			return candidate, nil
		})

	for range 2 {
		_, err := ResolveJWTSecret(context.Background(), mockStore, "")
		require.NoError(t, err)
	}
	for _, c := range candidates {
		raw, err := hex.DecodeString(c)
		require.NoError(t, err)
		require.Len(t, raw, jwtSecretBytes)
	}
	require.NotEqual(t, candidates[0], candidates[1])
}
