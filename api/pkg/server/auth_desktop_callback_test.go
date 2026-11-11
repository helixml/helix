package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

const testDesktopSecret = "desktop-secret"

func newDesktopTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore, *auth.MockAuthenticator) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	authn := auth.NewMockAuthenticator(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Auth.DesktopAutoLoginSecret = testDesktopSecret
	return &HelixAPIServer{
		Cfg:            cfg,
		Store:          st,
		authenticator:  authn,
		sessionManager: auth.NewSessionManager(st, nil, cfg),
	}, st, authn
}

func desktopCallback(s *HelixAPIServer, email string) *httptest.ResponseRecorder {
	q := url.Values{"token": {testDesktopSecret}, "name": {"Mac User"}}
	if email != "" {
		q.Set("email", email)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop-callback?"+q.Encode(), nil)
	rr := httptest.NewRecorder()
	s.desktopCallback(rr, req)
	return rr
}

func expectSessionFor(st *store.MockStore, userID string) {
	st.EXPECT().CreateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, sess *types.UserSession) (*types.UserSession, error) {
		if sess.UserID != userID {
			panic("desktop session created for " + sess.UserID + ", want " + userID)
		}
		return sess, nil
	})
}

// The shared secret must not let a caller sign in as an arbitrary existing
// account by naming its email, nor promote it to admin. The mock store
// fails the test on any UpdateUser or CreateUserSession call.
func TestDesktopCallback_RefusesOtherAccounts(t *testing.T) {
	for name, victim := range map[string]*types.User{
		"password user":  {ID: "usr_victim", Email: "victim@example.com", Type: types.OwnerTypeUser, AuthProvider: types.AuthProviderRegular, PasswordHash: []byte("hash")},
		"OIDC user":      {ID: "usr_victim", Email: "victim@example.com", Type: types.OwnerTypeUser, AuthProvider: types.AuthProviderOIDC},
		"non-admin":      {ID: "usr_victim", Email: "victim@example.com", Type: types.OwnerTypeUser, AuthProvider: types.AuthProviderRegular},
		"password admin": {ID: "usr_victim", Email: "victim@example.com", Type: types.OwnerTypeUser, AuthProvider: types.AuthProviderRegular, Admin: true, PasswordHash: []byte("hash")},
	} {
		t.Run(name, func(t *testing.T) {
			s, st, _ := newDesktopTestServer(t)
			st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{DesktopIdentity: true}).Return(nil, store.ErrNotFound)
			st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "victim@example.com"}).Return(victim, nil)

			rr := desktopCallback(s, "victim@example.com")
			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
			require.Empty(t, rr.Result().Cookies())
		})
	}
}

// Once the desktop account exists, the callback signs in as it whatever
// email is passed; an email owned by another account is left alone.
func TestDesktopCallback_SignsInOnlyTheDesktopIdentity(t *testing.T) {
	s, st, _ := newDesktopTestServer(t)
	desktop := &types.User{ID: "usr_desktop", Email: defaultDesktopEmail, FullName: "Mac User", Admin: true, DesktopIdentity: true}
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{DesktopIdentity: true}).Return(desktop, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "victim@example.com"}).
		Return(&types.User{ID: "usr_victim", Email: "victim@example.com"}, nil)
	expectSessionFor(st, "usr_desktop")

	rr := desktopCallback(s, "victim@example.com")
	require.Equal(t, http.StatusFound, rr.Code, rr.Body.String())
	require.Equal(t, defaultDesktopEmail, desktop.Email)
}

// A license key adds the licensee's email to the existing desktop account.
func TestDesktopCallback_UpdatesDesktopIdentityEmail(t *testing.T) {
	s, st, _ := newDesktopTestServer(t)
	desktop := &types.User{ID: "usr_desktop", Email: defaultDesktopEmail, FullName: "Mac User", Admin: true, DesktopIdentity: true}
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{DesktopIdentity: true}).Return(desktop, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "licensee@example.com"}).Return(nil, store.ErrNotFound)
	st.EXPECT().UpdateUser(gomock.Any(), desktop).Return(desktop, nil)
	expectSessionFor(st, "usr_desktop")

	rr := desktopCallback(s, "licensee@example.com")
	require.Equal(t, http.StatusFound, rr.Code, rr.Body.String())
	require.Equal(t, "licensee@example.com", desktop.Email)
}

func TestDesktopCallback_CreatesDesktopIdentity(t *testing.T) {
	s, st, authn := newDesktopTestServer(t)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{DesktopIdentity: true}).Return(nil, store.ErrNotFound)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: defaultDesktopEmail}).Return(nil, store.ErrNotFound)
	authn.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, u *types.User) (*types.User, error) {
		require.True(t, u.Admin)
		require.True(t, u.DesktopIdentity)
		require.Equal(t, defaultDesktopEmail, u.Email)
		u.ID = "usr_new_desktop"
		return u, nil
	})
	expectSessionFor(st, "usr_new_desktop")

	rr := desktopCallback(s, "")
	require.Equal(t, http.StatusFound, rr.Code, rr.Body.String())
}

// Existing installs: the account the desktop flow created before accounts
// were marked (passwordless regular admin) is adopted.
func TestDesktopCallback_AdoptsLegacyDesktopAccount(t *testing.T) {
	s, st, _ := newDesktopTestServer(t)
	legacy := &types.User{ID: "usr_legacy", Email: "licensee@example.com", FullName: "Mac User", Admin: true, Type: types.OwnerTypeUser, AuthProvider: types.AuthProviderRegular}
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{DesktopIdentity: true}).Return(nil, store.ErrNotFound)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "licensee@example.com"}).Return(legacy, nil)
	st.EXPECT().UpdateUser(gomock.Any(), legacy).DoAndReturn(func(_ any, u *types.User) (*types.User, error) {
		require.True(t, u.DesktopIdentity)
		return u, nil
	})
	expectSessionFor(st, "usr_legacy")

	rr := desktopCallback(s, "licensee@example.com")
	require.Equal(t, http.StatusFound, rr.Code, rr.Body.String())
}
