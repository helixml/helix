package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/go-jose/go-jose.v2/jwt"

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
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{OIDCSubject: "idp-sub-1"}).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "person@example.com"}).Return(existing, nil)
	// The link is persisted by a column update, never a whole-row save.
	mockStore.EXPECT().UpdateUserEmailVerification(gomock.Any(), existing.ID, true, "idp-sub-1").Return(nil)
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
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{OIDCSubject: "idp-sub-1"}).Return(nil, store.ErrNotFound)
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

		mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).Times(3)
		mockStore.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
			require.Equal(t, verified, u.EmailVerified)
			require.Equal(t, "idp-sub-1", u.OIDCSubject)
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

// Once linked, later sign-ins resolve the account by the stored subject
// before any email lookup, so a changed or unverified email cannot re-link.
func TestValidateUserToken_LinkedAccountResolvesBySubject(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	client := newLinkingTestClient(t, mockStore, false)

	linked := &types.User{ID: "usr_password", Email: "person@example.com", EmailVerified: true, OIDCSubject: "idp-sub-1"}
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "idp-sub-1"}).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{OIDCSubject: "idp-sub-1"}).Return(linked, nil)
	// No lookup by email, no update, no CreateUser.

	user, err := client.ValidateUserToken(context.Background(), "tok")
	require.NoError(t, err)
	require.Equal(t, linked.ID, user.ID)
}

// A session token is bound to the credentials it was issued under: once the
// password changes it is refused, while a token issued before the binding
// existed is accepted until it expires.
func TestValidateUserToken_RefusesTokenAfterPasswordChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	authn, err := NewHelixAuthenticator(&config.ServerConfig{}, mockStore, "a-private-test-secret-value", nil)
	require.NoError(t, err)
	authn.cfg.Auth.Regular.TokenValidity = time.Hour

	user := &types.User{ID: "usr_1", Email: "user@example.com", PasswordHash: []byte("hash-v1")}
	token, err := authn.GenerateUserToken(context.Background(), user)
	require.NoError(t, err)

	t.Run("same credentials", func(t *testing.T) {
		mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil)
		_, err := authn.ValidateUserToken(context.Background(), token)
		require.NoError(t, err)
	})
	t.Run("password changed", func(t *testing.T) {
		changed := &types.User{ID: "usr_1", Email: "user@example.com", PasswordHash: []byte("hash-v2")}
		mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(changed, nil)
		_, err := authn.ValidateUserToken(context.Background(), token)
		require.ErrorContains(t, err, "credentials have changed")
	})
	t.Run("token without binding", func(t *testing.T) {
		legacy, err := jwt.Signed(authn.signer).Claims(jwt.Claims{
			Subject: user.ID, Issuer: "helix", Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now()),
		}).Claims(map[string]any{"user_id": user.ID}).CompactSerialize()
		require.NoError(t, err)
		changed := &types.User{ID: "usr_1", PasswordHash: []byte("hash-v2")}
		mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(changed, nil)
		_, err = authn.ValidateUserToken(context.Background(), legacy)
		require.NoError(t, err)
	})
}

// ADMIN_USER_IDS entries keyed by the IdP subject keep working for an
// account that was linked by email and so has a different ID.
func TestValidateUserToken_LinkedAccountAdminBySubject(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	oidcServer := NewMockOIDCServer()
	t.Cleanup(oidcServer.Close)
	oidcServer.userInfo = map[string]interface{}{"sub": "idp-sub-1", "email": "person@example.com", "email_verified": true}
	client, err := NewOIDCClient(context.Background(), OIDCConfig{
		ProviderURL: oidcServer.URL(), ClientID: "api", ClientSecret: "secret",
		RedirectURL: "http://localhost:8080/callback", Audience: "test-aud", Store: mockStore,
		AdminUserIDs: []string{"idp-sub-1"},
	})
	require.NoError(t, err)

	linked := &types.User{ID: "usr_password", Email: "person@example.com", EmailVerified: true, OIDCSubject: "idp-sub-1"}
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "idp-sub-1"}).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{OIDCSubject: "idp-sub-1"}).Return(linked, nil)
	mockStore.EXPECT().GetOrganizationByDomain(gomock.Any(), "example.com").Return(nil, store.ErrNotFound)

	user, err := client.ValidateUserToken(context.Background(), "tok")
	require.NoError(t, err)
	require.True(t, user.Admin)
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
