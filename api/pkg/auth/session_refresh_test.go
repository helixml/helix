package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

// fakeTokenEndpoint is an OIDC provider whose token endpoint answers every
// refresh with the configured response and counts refresh attempts.
type fakeTokenEndpoint struct {
	server *httptest.Server
	// calls counts refresh attempts. golang.org/x/oauth2 discovers the client
	// authentication style by sending a failed request again with the
	// credentials in the body, so one attempt can be two HTTP requests; only
	// the first, which carries HTTP Basic credentials, is counted.
	calls atomic.Int32
	delay time.Duration
	// beforeAnswer, when set, runs while the refresh call is in flight.
	beforeAnswer func()

	mu     sync.Mutex
	status int
	body   map[string]any
}

func newFakeTokenEndpoint(t *testing.T) *fakeTokenEndpoint {
	f := &fakeTokenEndpoint{}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 f.server.URL,
			"authorization_endpoint": f.server.URL + "/auth",
			"token_endpoint":         f.server.URL + "/token",
			"jwks_uri":               f.server.URL + "/certs",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if _, _, basic := r.BasicAuth(); basic {
			f.calls.Add(1)
		}
		assert.Equal(t, "refresh_token", r.FormValue("grant_type"))
		time.Sleep(f.delay)
		if f.beforeAnswer != nil {
			f.beforeAnswer()
		}
		f.mu.Lock()
		status, body := f.status, f.body
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeTokenEndpoint) respond(status int, body map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.body = status, body
}

func (f *fakeTokenEndpoint) client() *OIDCClient {
	return &OIDCClient{cfg: OIDCConfig{
		ProviderURL:  f.server.URL,
		ClientID:     "helix",
		ClientSecret: "secret",
		RedirectURL:  "https://example.com/callback",
	}}
}

// sessionTable backs the mock store's session calls with a map.
type sessionTable struct {
	mu       sync.Mutex
	sessions map[string]types.UserSession
	deleted  []string
}

func newSessionTable(st *store.MockStore, sessions ...types.UserSession) *sessionTable {
	tbl := &sessionTable{sessions: map[string]types.UserSession{}}
	for _, s := range sessions {
		tbl.sessions[s.ID] = s
	}
	st.EXPECT().GetUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*types.UserSession, error) {
		tbl.mu.Lock()
		defer tbl.mu.Unlock()
		s, ok := tbl.sessions[id]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		return &s, nil
	}).AnyTimes()
	st.EXPECT().UpdateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, s *types.UserSession) (*types.UserSession, error) {
		tbl.mu.Lock()
		defer tbl.mu.Unlock()
		tbl.sessions[s.ID] = *s
		return s, nil
	}).AnyTimes()
	st.EXPECT().DeleteUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) error {
		tbl.mu.Lock()
		defer tbl.mu.Unlock()
		delete(tbl.sessions, id)
		tbl.deleted = append(tbl.deleted, id)
		return nil
	}).AnyTimes()
	return tbl
}

func (tbl *sessionTable) get(id string) (types.UserSession, bool) {
	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	s, ok := tbl.sessions[id]
	return s, ok
}

func expiredOIDCSession(id string) types.UserSession {
	return types.UserSession{
		ID:               id,
		UserID:           "usr_test",
		AuthProvider:     types.AuthProviderOIDC,
		ExpiresAt:        time.Now().Add(time.Hour),
		LastUsedAt:       time.Now(),
		OIDCAccessToken:  "expired_access_token",
		OIDCRefreshToken: "refresh_token",
		OIDCTokenExpiry:  time.Now().Add(-time.Minute),
	}
}

func newRefreshTestManager(t *testing.T, provider *fakeTokenEndpoint, sessions ...types.UserSession) (*SessionManager, *sessionTable) {
	st := store.NewMockStore(gomock.NewController(t))
	tbl := newSessionTable(st, sessions...)
	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderOIDC
	cfg.WebServer.URL = "https://example.com"
	return NewSessionManager(st, provider.client(), cfg), tbl
}

func requestWithSession(id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: id})
	return r
}

// An identity-provider session that has ended can never be refreshed: the
// BFF session is deleted, so the next request does not ask the provider again.
func TestGetSessionFromRequest_InvalidGrantDeletesSession(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "Session not active"})
	sm, tbl := newRefreshTestManager(t, provider, expiredOIDCSession("uss_ended"))

	session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_ended"))
	require.ErrorIs(t, err, ErrSessionRevoked)
	assert.True(t, IsSessionDiscarded(err))
	assert.Nil(t, session)
	_, ok := tbl.get("uss_ended")
	assert.False(t, ok, "the session must be deleted")

	_, err = sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_ended"))
	require.ErrorIs(t, err, ErrSessionNotFound)
	assert.Equal(t, int32(1), provider.calls.Load(), "a deleted session must not be refreshed again")
}

func TestGetSessionFromRequest_RejectedClientDeletesSession(t *testing.T) {
	for _, code := range []string{"invalid_client", "unauthorized_client"} {
		t.Run(code, func(t *testing.T) {
			provider := newFakeTokenEndpoint(t)
			provider.respond(http.StatusUnauthorized, map[string]any{"error": code})
			sm, tbl := newRefreshTestManager(t, provider, expiredOIDCSession("uss_client"))

			_, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_client"))
			require.ErrorIs(t, err, ErrSessionRevoked)
			_, ok := tbl.get("uss_client")
			assert.False(t, ok)
		})
	}
}

// A provider outage is not the user's fault: the session survives, and the
// failing refresh is not retried on every request.
func TestGetSessionFromRequest_TransientRefreshErrorKeepsSessionAndBacksOff(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"})
	sm, tbl := newRefreshTestManager(t, provider, expiredOIDCSession("uss_outage"))

	for i := 0; i < 5; i++ {
		session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_outage"))
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.Equal(t, "expired_access_token", session.OIDCAccessToken)
	}
	assert.Equal(t, int32(1), provider.calls.Load(), "requests inside the retry interval must not call the provider")
	_, ok := tbl.get("uss_outage")
	assert.True(t, ok, "a transient failure must not delete the session")

	// Once the retry interval has passed, the next request tries again.
	sm.refreshRetry.mu.Lock()
	sm.refreshRetry.until["uss_outage"] = time.Now().Add(-time.Second)
	sm.refreshRetry.mu.Unlock()
	provider.respond(http.StatusOK, map[string]any{
		"access_token": "new_access_token", "refresh_token": "new_refresh_token",
		"token_type": "Bearer", "expires_in": 300,
	})
	session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_outage"))
	require.NoError(t, err)
	assert.Equal(t, "new_access_token", session.OIDCAccessToken)
	assert.Equal(t, int32(2), provider.calls.Load())
}

// A burst of parallel requests from one browser shares one refresh, and
// every request gets the refreshed token.
func TestGetSessionFromRequest_ConcurrentRequestsShareOneRefresh(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.delay = 100 * time.Millisecond
	provider.respond(http.StatusOK, map[string]any{
		"access_token": "new_access_token", "refresh_token": "new_refresh_token",
		"token_type": "Bearer", "expires_in": 3600,
	})
	sm, tbl := newRefreshTestManager(t, provider, expiredOIDCSession("uss_burst"))

	const requests = 20
	start := make(chan struct{})
	tokens := make([]string, requests)
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_burst"))
			if assert.NoError(t, err) {
				tokens[i] = session.OIDCAccessToken
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int32(1), provider.calls.Load())
	for _, token := range tokens {
		assert.Equal(t, "new_access_token", token)
	}
	stored, _ := tbl.get("uss_burst")
	assert.Equal(t, "new_refresh_token", stored.OIDCRefreshToken)
}

// When another API replica rotated the refresh token first, the provider
// rejects the stale one; that must not log the user out.
func TestGetSessionFromRequest_InvalidGrantAfterRotationElsewhereKeepsSession(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "Stale token"})
	sm, tbl := newRefreshTestManager(t, provider, expiredOIDCSession("uss_rotated"))

	// The other replica stores its refreshed tokens while this one's call
	// to the provider is in flight.
	rotated := expiredOIDCSession("uss_rotated")
	rotated.OIDCRefreshToken = "rotated_refresh_token"
	rotated.OIDCAccessToken = "rotated_access_token"
	rotated.OIDCTokenExpiry = time.Now().Add(time.Hour)
	provider.beforeAnswer = func() {
		tbl.mu.Lock()
		tbl.sessions["uss_rotated"] = rotated
		tbl.mu.Unlock()
	}

	session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_rotated"))
	require.NoError(t, err)
	assert.Equal(t, "rotated_access_token", session.OIDCAccessToken)
	_, ok := tbl.get("uss_rotated")
	assert.True(t, ok)
}
