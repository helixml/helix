package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func newAcceptTestServer(t *testing.T, user *types.User) (*HelixAPIServer, *store.MockStore) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderRegular
	st.EXPECT().GetUserSession(gomock.Any(), "sess_1").Return(&types.UserSession{
		ID: "sess_1", UserID: user.ID, AuthProvider: types.AuthProviderRegular,
		ExpiresAt: time.Now().Add(time.Hour), LastUsedAt: time.Now(),
	}, nil).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil).AnyTimes()
	return &HelixAPIServer{Cfg: cfg, Store: st, sessionManager: auth.NewSessionManager(st, nil, cfg)}, st
}

func acceptRequest(body string, session, csrf bool) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/invitations/oin_1/accept", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "oin_1"})
	if session {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "sess_1"})
	}
	if csrf {
		req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "csrf"})
		req.Header.Set(auth.CSRFHeaderName, "csrf")
	}
	return req
}

// A signed-in invitee (including one waitlisted after signing up without
// the link) accepts with the token from the link.
func TestAcceptInvitation_WaitlistedAccountAcceptsWithToken(t *testing.T) {
	user := &types.User{ID: "usr_1", Email: "invitee@example.com", Waitlisted: true}
	s, st := newAcceptTestServer(t, user)
	st.EXPECT().ConsumeInvitation(gomock.Any(), user, "oin_1", "link-token").
		Return([]*types.OrganizationMembership{{OrganizationID: "org_1", UserID: user.ID}}, nil)
	st.EXPECT().UpdateUser(gomock.Any(), user).DoAndReturn(func(_ any, u *types.User) (*types.User, error) {
		require.False(t, u.Waitlisted)
		require.True(t, u.OnboardingCompleted)
		return u, nil
	})

	rr := httptest.NewRecorder()
	s.acceptInvitationHandler(rr, acceptRequest(`{"token":"link-token"}`, true, true))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

func TestAcceptInvitation_Refusals(t *testing.T) {
	user := &types.User{ID: "usr_1", Email: "invitee@example.com"}

	s, _ := newAcceptTestServer(t, user)
	rr := httptest.NewRecorder()
	s.acceptInvitationHandler(rr, acceptRequest(`{"token":"link-token"}`, false, true))
	require.Equal(t, http.StatusUnauthorized, rr.Code, "no session")

	rr = httptest.NewRecorder()
	s.acceptInvitationHandler(rr, acceptRequest(`{"token":"link-token"}`, true, false))
	require.Equal(t, http.StatusForbidden, rr.Code, "no CSRF token")

	rr = httptest.NewRecorder()
	s.acceptInvitationHandler(rr, acceptRequest(`{}`, true, true))
	require.Equal(t, http.StatusBadRequest, rr.Code, "no invitation token")

	// Wrong token or another address: the store matches nothing.
	s, st := newAcceptTestServer(t, user)
	st.EXPECT().ConsumeInvitation(gomock.Any(), user, "oin_1", "guess").Return(nil, nil)
	rr = httptest.NewRecorder()
	s.acceptInvitationHandler(rr, acceptRequest(`{"token":"guess"}`, true, true))
	require.Equal(t, http.StatusNotFound, rr.Code)

	deactivated := &types.User{ID: "usr_2", Email: "invitee@example.com", Deactivated: true}
	s, _ = newAcceptTestServer(t, deactivated)
	rr = httptest.NewRecorder()
	s.acceptInvitationHandler(rr, acceptRequest(`{"token":"link-token"}`, true, true))
	require.Equal(t, http.StatusForbidden, rr.Code, "deactivated account")
}
