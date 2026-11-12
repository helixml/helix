package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func newLinkingTestClient(t *testing.T, mockStore *store.MockStore, verified bool) *OIDCClient {
	t.Helper()
	oidcServer := NewMockOIDCServer()
	t.Cleanup(oidcServer.Close)
	oidcServer.userInfo = map[string]interface{}{
		"sub": "idp-sub-1", "name": "Some Person", "email": "person@example.com", "email_verified": verified,
	}
	client, err := NewOIDCClient(context.Background(), OIDCConfig{
		ProviderURL: oidcServer.URL(), ClientID: "api", ClientSecret: "secret",
		RedirectURL: "http://localhost:8080/callback", Audience: "test-aud", Store: mockStore,
	})
	require.NoError(t, err)
	return client
}

// A first SSO sign-in whose verified email already belongs to an account
// (created with a password before the move to SSO) signs in as that account
// instead of creating a second one with the same address.
func TestValidateUserToken_LinksVerifiedEmailToExistingAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	client := newLinkingTestClient(t, mockStore, true)

	existing := &types.User{ID: "usr_password", Username: "person@example.com", Email: "person@example.com", AuthProvider: types.AuthProviderRegular}
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "idp-sub-1"}).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "person@example.com"}).Return(existing, nil)
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
		require.Equal(t, existing.ID, u.ID)
		require.True(t, u.EmailVerified, "the IdP's assertion verifies the address")
		return u, nil
	})
	mockStore.EXPECT().GetOrganizationByDomain(gomock.Any(), "example.com").Return(nil, store.ErrNotFound)
	// No CreateUser, no ConsumePendingInvitations: this is not a new account.

	user, err := client.ValidateUserToken(context.Background(), "tok")
	require.NoError(t, err)
	require.Equal(t, existing.ID, user.ID)
	require.True(t, user.EmailVerified)
}

// Without the IdP's word that the address is verified, a sign-in that
// collides with an existing account is refused: neither linked nor duplicated.
func TestValidateUserToken_RefusesUnverifiedEmailOfExistingAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	client := newLinkingTestClient(t, mockStore, false)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "idp-sub-1"}).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "person@example.com"}).
		Return(&types.User{ID: "usr_password", Email: "person@example.com"}, nil)

	_, err := client.ValidateUserToken(context.Background(), "tok")
	require.ErrorIs(t, err, ErrOIDCEmailCollision)
}

// A genuinely new account records whether the IdP verified its address.
func TestValidateUserToken_NewAccountRecordsEmailVerified(t *testing.T) {
	for _, verified := range []bool{true, false} {
		ctrl := gomock.NewController(t)
		mockStore := store.NewMockStore(ctrl)
		client := newLinkingTestClient(t, mockStore, verified)

		mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).Times(2)
		mockStore.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
			require.Equal(t, verified, u.EmailVerified)
			return u, nil
		})
		if verified {
			mockStore.EXPECT().ConsumePendingInvitations(gomock.Any(), gomock.Any()).Return(nil, nil)
			mockStore.EXPECT().GetOrganizationByDomain(gomock.Any(), "example.com").Return(nil, store.ErrNotFound)
		}

		user, err := client.ValidateUserToken(context.Background(), "tok")
		require.NoError(t, err)
		require.Equal(t, verified, user.EmailVerified)
	}
}

// Completing a password reset shows control of the mailbox the link went to.
func TestPasswordResetComplete_MarksEmailVerified(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	authn, err := NewHelixAuthenticator(cfg, mockStore, "a-private-test-secret-value", nil)
	require.NoError(t, err)

	user := &types.User{ID: "usr_1", Email: "user@example.com", PasswordHash: []byte("old")}
	token, err := authn.generatePasswordResetToken(user, time.Now())
	require.NoError(t, err)

	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil)
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
		require.True(t, u.EmailVerified)
		return u, nil
	})

	_, err = authn.PasswordResetComplete(context.Background(), token, "a-new-password")
	require.NoError(t, err)
}

// The decoy costs a real bcrypt comparison and matches nothing.
func TestPasswordTimingDecoy(t *testing.T) {
	decoy := PasswordTimingDecoy()
	cost, err := bcrypt.Cost(decoy.PasswordHash)
	require.NoError(t, err)
	require.Equal(t, bcrypt.DefaultCost, cost, "same work factor as real accounts")
	require.Error(t, bcrypt.CompareHashAndPassword(decoy.PasswordHash, []byte("")))
	require.Error(t, bcrypt.CompareHashAndPassword(decoy.PasswordHash, []byte("password")))
	require.Empty(t, decoy.ID)
}
