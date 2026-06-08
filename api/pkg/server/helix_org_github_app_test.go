package server

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// testEncKey is a valid 32-byte AES-256 key for the decrypt path.
var testEncKey = []byte("0123456789abcdef0123456789abcdef")

func testKeyGetter() ([]byte, error) { return testEncKey, nil }

// mustEncrypt mirrors how service_connection_handlers.go stores the PEM:
// AES-256-GCM, base64. The resolver must decrypt it before minting.
func mustEncrypt(t *testing.T, plaintext string) string {
	t.Helper()
	enc, err := crypto.EncryptAES256GCM([]byte(plaintext), testEncKey)
	require.NoError(t, err)
	return enc
}

// newGitHubApp builds a github_app ServiceConnection row with an
// encrypted PEM, as the store would return it.
func newGitHubApp(t *testing.T, appID, installID int64, pem string) *types.ServiceConnection {
	return &types.ServiceConnection{
		Type:                 types.ServiceConnectionTypeGitHubApp,
		OrganizationID:       "org-1",
		GitHubAppID:          appID,
		GitHubInstallationID: installID,
		GitHubPrivateKey:     mustEncrypt(t, pem),
	}
}

func TestOrgGitHubIdentityResolver_NoAppFallsBackToOAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().
		ListServiceConnectionsByType(gomock.Any(), "org-1", types.ServiceConnectionTypeGitHubApp).
		Return([]*types.ServiceConnection{}, nil)

	oauth := func(_ context.Context, _ string) (string, error) { return "oauth-token", nil }
	mint := func(_ context.Context, _, _ int64, _, _ string) (string, error) {
		t.Fatal("mintFn must not be called when no app is installed")
		return "", nil
	}

	resolve := newOrgGitHubIdentityResolver(testKeyGetter, st, oauth, mint)
	id, err := resolve(context.Background(), "org-1")
	require.NoError(t, err)
	require.Equal(t, "oauth", id.Mode)
	require.Equal(t, "oauth-token", id.Token)
}

func TestOrgGitHubIdentityResolver_AppMintsBotToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().
		ListServiceConnectionsByType(gomock.Any(), "org-1", types.ServiceConnectionTypeGitHubApp).
		Return([]*types.ServiceConnection{newGitHubApp(t, 111, 222, "FAKE_PEM")}, nil)

	var gotAppID, gotInstallID int64
	var gotPEM string
	mint := func(_ context.Context, appID, installID int64, pem, _ string) (string, error) {
		gotAppID, gotInstallID, gotPEM = appID, installID, pem
		return "bot-token", nil
	}
	oauth := func(_ context.Context, _ string) (string, error) {
		t.Fatal("oauth fallback must not be called when the app mints successfully")
		return "", nil
	}

	resolve := newOrgGitHubIdentityResolver(testKeyGetter, st, oauth, mint)
	id, err := resolve(context.Background(), "org-1")
	require.NoError(t, err)
	require.Equal(t, "app", id.Mode)
	require.Equal(t, "bot-token", id.Token)
	require.Equal(t, int64(111), id.AppID)
	require.Equal(t, int64(222), id.InstallationID)
	// The PEM handed to the minter must be the DECRYPTED key.
	require.Equal(t, "FAKE_PEM", gotPEM)
	require.Equal(t, int64(111), gotAppID)
	require.Equal(t, int64(222), gotInstallID)
}

func TestOrgGitHubIdentityResolver_NewestAppWins(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	// Store returns created_at DESC, so the newest connection is first.
	st.EXPECT().
		ListServiceConnectionsByType(gomock.Any(), "org-1", types.ServiceConnectionTypeGitHubApp).
		Return([]*types.ServiceConnection{
			newGitHubApp(t, 999, 888, "NEWEST_PEM"),
			newGitHubApp(t, 111, 222, "OLDER_PEM"),
		}, nil)

	var gotAppID int64
	mint := func(_ context.Context, appID, _ int64, _, _ string) (string, error) {
		gotAppID = appID
		return "bot-token", nil
	}
	oauth := func(_ context.Context, _ string) (string, error) { return "oauth-token", nil }

	resolve := newOrgGitHubIdentityResolver(testKeyGetter, st, oauth, mint)
	id, err := resolve(context.Background(), "org-1")
	require.NoError(t, err)
	require.Equal(t, "app", id.Mode)
	require.Equal(t, int64(999), gotAppID)
}

func TestOrgGitHubIdentityResolver_MintErrorFallsBackToOAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().
		ListServiceConnectionsByType(gomock.Any(), "org-1", types.ServiceConnectionTypeGitHubApp).
		Return([]*types.ServiceConnection{newGitHubApp(t, 111, 222, "FAKE_PEM")}, nil)

	mint := func(_ context.Context, _, _ int64, _, _ string) (string, error) {
		return "", errors.New("github says no")
	}
	oauth := func(_ context.Context, _ string) (string, error) { return "oauth-token", nil }

	resolve := newOrgGitHubIdentityResolver(testKeyGetter, st, oauth, mint)
	id, err := resolve(context.Background(), "org-1")
	require.NoError(t, err)
	require.Equal(t, "oauth", id.Mode, "a broken app config must never break an org OAuth could still serve")
	require.Equal(t, "oauth-token", id.Token)
}

func TestOrgGitHubIdentityResolver_DecryptErrorFallsBackToOAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	corrupt := &types.ServiceConnection{
		Type:                 types.ServiceConnectionTypeGitHubApp,
		OrganizationID:       "org-1",
		GitHubAppID:          111,
		GitHubInstallationID: 222,
		GitHubPrivateKey:     "not-valid-ciphertext",
	}
	st.EXPECT().
		ListServiceConnectionsByType(gomock.Any(), "org-1", types.ServiceConnectionTypeGitHubApp).
		Return([]*types.ServiceConnection{corrupt}, nil)

	mint := func(_ context.Context, _, _ int64, _, _ string) (string, error) {
		t.Fatal("mintFn must not be called when the PEM cannot be decrypted")
		return "", nil
	}
	oauth := func(_ context.Context, _ string) (string, error) { return "oauth-token", nil }

	resolve := newOrgGitHubIdentityResolver(testKeyGetter, st, oauth, mint)
	id, err := resolve(context.Background(), "org-1")
	require.NoError(t, err)
	require.Equal(t, "oauth", id.Mode)
	require.Equal(t, "oauth-token", id.Token)
}
