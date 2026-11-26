package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func openBrowserTicket(s *HelixAPIServer, ticket string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/flow/browser/"+ticket, nil)
	req = mux.SetURLVars(req, map[string]string{"ticket": ticket})
	rr := httptest.NewRecorder()
	s.oauthBrowserFlowHandler().ServeHTTP(rr, req)
	return rr
}

// A ticket only verifies untouched, unexpired, and under this server's key.
func TestOAuthBrowserTicket_SignedAndExpiring(t *testing.T) {
	s, _ := newOAuthCallbackTestServer(t)
	now := time.Now()
	ticket, err := s.signOAuthBrowserTicket(oauthBrowserTicket{AuthURL: "https://github.com/login/oauth/authorize?state=st", State: "st", Expires: now.Add(time.Minute).Unix()})
	require.NoError(t, err)

	got, err := s.verifyOAuthBrowserTicket(ticket, now)
	require.NoError(t, err)
	require.Equal(t, "st", got.State)

	_, err = s.verifyOAuthBrowserTicket(ticket, now.Add(2*time.Minute))
	require.Error(t, err, "expired ticket accepted")

	payload, sig, _ := strings.Cut(ticket, ".")
	forged, _ := (&HelixAPIServer{}).signOAuthBrowserTicket(oauthBrowserTicket{AuthURL: "https://evil.test/", State: "st", Expires: now.Add(time.Minute).Unix()})
	forgedPayload, _, _ := strings.Cut(forged, ".")
	_, err = s.verifyOAuthBrowserTicket(forgedPayload+"."+sig, now)
	require.Error(t, err, "payload swapped under another signature")
	_, err = s.verifyOAuthBrowserTicket(forged, now)
	require.Error(t, err, "ticket signed by another key")
	_, err = s.verifyOAuthBrowserTicket(payload, now)
	require.Error(t, err, "unsigned ticket")
}

// Opening a valid ticket for a pending flow binds this browser to the flow and
// goes to the provider; anything else shows the standalone failure page.
func TestOAuthBrowserTicket_OpenBindsBrowser(t *testing.T) {
	s, st := newOAuthCallbackTestServer(t)
	authURL := "https://github.com/login/oauth/authorize?client_id=c&state=flow-state"
	ticket, err := s.signOAuthBrowserTicket(oauthBrowserTicket{AuthURL: authURL, State: "flow-state", Expires: time.Now().Add(time.Minute).Unix()})
	require.NoError(t, err)

	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "flow-state").Return([]*types.OAuthRequestToken{
		{ID: "req-1", UserID: "usr_starter", ProviderID: "prov-1", State: "flow-state", ExpiresAt: time.Now().Add(5 * time.Minute)},
	}, nil)
	rr := openBrowserTicket(s, ticket)
	require.Equal(t, http.StatusFound, rr.Code, rr.Body.String())
	require.Equal(t, authURL, rr.Header().Get("Location"))
	var cookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == oauthBrowserFlowCookie {
			cookie = c
		}
	}
	require.NotNil(t, cookie)
	require.Equal(t, "flow-state", cookie.Value)
	require.True(t, cookie.HttpOnly)
	require.Equal(t, "/api/v1/oauth/flow/callback", cookie.Path)

	// A finished or timed-out flow cannot be resumed.
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "flow-state").Return(nil, nil)
	rr = openBrowserTicket(s, ticket)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Empty(t, rr.Result().Cookies())

	rr = openBrowserTicket(s, "not-a-ticket")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), "oauth-failure")
	require.Empty(t, rr.Result().Cookies())
}

// The callback completes for a browser bound to the flow by its ticket, with
// no Helix session; a cookie for another flow does not pass.
func TestOAuthCallback_CompletesForBrowserBoundByTicket(t *testing.T) {
	s, st := newOAuthCallbackTestServer(t)
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "flow-state").Return([]*types.OAuthRequestToken{
		{ID: "req-1", UserID: "usr_starter", ProviderID: "prov-1", State: "flow-state", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_starter"}).Return(&types.User{ID: "usr_starter"}, nil)
	// Past the identity gate the manager resolves the provider; failing that
	// lookup proves the flow proceeded.
	st.EXPECT().GetOAuthProvider(gomock.Any(), "prov-1").Return(nil, errors.New("not found"))

	req := oauthCallbackRequest("flow-state", "")
	req.AddCookie(&http.Cookie{Name: oauthBrowserFlowCookie, Value: "flow-state"})
	rr := httptest.NewRecorder()
	s.oauthCallbackHandler().ServeHTTP(rr, req)
	require.NotEqual(t, http.StatusForbidden, rr.Code, rr.Body.String())

	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "victim-state").Return([]*types.OAuthRequestToken{
		{ID: "req-2", UserID: "usr_victim", ProviderID: "prov-1", State: "victim-state", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)
	req = oauthCallbackRequest("victim-state", "")
	req.AddCookie(&http.Cookie{Name: oauthBrowserFlowCookie, Value: "flow-state"})
	rr = httptest.NewRecorder()
	s.oauthCallbackHandler().ServeHTTP(rr, req)
	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

// An agent's project- or session-scoped key cannot connect accounts for the
// person it runs as.
func TestOAuthBrowserTicket_RefusesAgentKeys(t *testing.T) {
	s, _ := newOAuthCallbackTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/flow/browser-ticket/prov-1", nil)
	req = mux.SetURLVars(req, map[string]string{"provider_id": "prov-1"})
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_1", SessionID: "ses_1"}))
	_, err := s.handleCreateOAuthBrowserTicket(nil, req)
	require.Error(t, err)
}
