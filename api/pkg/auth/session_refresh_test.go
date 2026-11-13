package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	// readErr, when set, fails every GetUserSession after the first n reads.
	readErr        error
	readsBeforeErr int
	reads          int
}

func newSessionTable(st *store.MockStore, sessions ...types.UserSession) *sessionTable {
	tbl := &sessionTable{sessions: map[string]types.UserSession{}}
	for _, s := range sessions {
		tbl.sessions[s.ID] = s
	}
	st.EXPECT().GetUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id string) (*types.UserSession, error) {
		tbl.mu.Lock()
		defer tbl.mu.Unlock()
		tbl.reads++
		if tbl.readErr != nil && tbl.reads > tbl.readsBeforeErr {
			return nil, tbl.readErr
		}
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
	st.EXPECT().MarkUserSessionOIDCRefreshDead(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id, refreshToken string) (bool, error) {
		tbl.mu.Lock()
		defer tbl.mu.Unlock()
		s, ok := tbl.sessions[id]
		if !ok || s.OIDCRefreshToken != refreshToken {
			return false, nil
		}
		s.OIDCRefreshDead = true
		tbl.sessions[id] = s
		return true, nil
	}).AnyTimes()
	st.EXPECT().DeleteUserSessionWithRefreshToken(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, id, refreshToken string) (bool, error) {
		tbl.mu.Lock()
		defer tbl.mu.Unlock()
		s, ok := tbl.sessions[id]
		if !ok || s.OIDCRefreshToken != refreshToken {
			return false, nil
		}
		delete(tbl.sessions, id)
		return true, nil
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

func newRefreshTestManager(t *testing.T, provider *fakeTokenEndpoint, endSession bool, sessions ...types.UserSession) (*SessionManager, *sessionTable, *store.MockStore) {
	st := store.NewMockStore(gomock.NewController(t))
	tbl := newSessionTable(st, sessions...)
	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderOIDC
	cfg.Auth.OIDC.EndSessionWhenIdPSessionEnds = endSession
	cfg.WebServer.URL = "https://example.com"
	return NewSessionManager(st, provider.client(), cfg), tbl, st
}

func requestWithSession(id string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: id})
	return r
}

var sessionNotActive = map[string]any{"error": "invalid_grant", "error_description": "Session not active"}

func okTokenResponse(expiresIn int) map[string]any {
	return map[string]any{
		"access_token": "new_access_token", "refresh_token": "new_refresh_token",
		"token_type": "Bearer", "expires_in": expiresIn,
	}
}

// Default: when the identity-provider session ends, the Helix session lives
// out its normal duration but is never refreshed again.
func TestGetSessionFromRequest_InvalidGrantMarksSessionDead(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusBadRequest, sessionNotActive)
	sm, tbl, _ := newRefreshTestManager(t, provider, false, expiredOIDCSession("uss_ended"))

	for range 5 {
		session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_ended"))
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.Equal(t, "uss_ended", session.ID)
	}
	assert.Equal(t, int32(1), provider.calls.Load(), "a dead session must not be refreshed again")

	stored, ok := tbl.get("uss_ended")
	require.True(t, ok, "the session is kept")
	assert.True(t, stored.OIDCRefreshDead)
	assert.False(t, stored.NeedsOIDCRefresh())

	// The background refresher skips it as well.
	sm.refreshRetry.succeeded("uss_ended")
	_, err := sm.refreshSessionOIDC(context.Background(), &stored)
	require.NoError(t, err)
	assert.Equal(t, int32(1), provider.calls.Load())
}

// OIDC_END_SESSION_WHEN_IDP_SESSION_ENDS: the session is deleted, so the
// next request is unauthenticated and does not ask the provider again.
func TestGetSessionFromRequest_InvalidGrantDeletesSessionWhenConfigured(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusBadRequest, sessionNotActive)
	sm, tbl, _ := newRefreshTestManager(t, provider, true, expiredOIDCSession("uss_ended"))

	session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_ended"))
	require.ErrorIs(t, err, ErrSessionRevoked)
	assert.True(t, IsSessionDiscarded(err))
	assert.Nil(t, session)
	_, ok := tbl.get("uss_ended")
	assert.False(t, ok, "the session must be deleted")

	_, err = sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_ended"))
	require.ErrorIs(t, err, ErrSessionNotFound)
	assert.Equal(t, int32(1), provider.calls.Load())
}

// Rejected client credentials are a deployment-wide misconfiguration, not a
// fact about this session: no session is retired, the refresh backs off.
func TestGetSessionFromRequest_RejectedClientIsNotTerminal(t *testing.T) {
	for _, code := range []string{"invalid_client", "unauthorized_client"} {
		for _, endSession := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/end_session=%v", code, endSession), func(t *testing.T) {
				provider := newFakeTokenEndpoint(t)
				provider.respond(http.StatusUnauthorized, map[string]any{"error": code})
				sm, tbl, _ := newRefreshTestManager(t, provider, endSession, expiredOIDCSession("uss_client"))

				for range 3 {
					session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_client"))
					require.NoError(t, err)
					require.NotNil(t, session)
				}
				assert.Equal(t, int32(1), provider.calls.Load(), "the retry gate applies")
				stored, ok := tbl.get("uss_client")
				require.True(t, ok)
				assert.False(t, stored.OIDCRefreshDead)
			})
		}
	}
}

// A provider outage is not the user's fault: the session survives, and the
// failing refresh is not retried on every request.
func TestGetSessionFromRequest_TransientRefreshErrorKeepsSessionAndBacksOff(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusServiceUnavailable, map[string]any{"error": "temporarily_unavailable"})
	sm, tbl, _ := newRefreshTestManager(t, provider, true, expiredOIDCSession("uss_outage"))

	for range 5 {
		session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_outage"))
		require.NoError(t, err)
		require.NotNil(t, session)
		assert.Equal(t, "expired_access_token", session.OIDCAccessToken)
	}
	assert.Equal(t, int32(1), provider.calls.Load(), "requests inside the retry interval must not call the provider")
	stored, ok := tbl.get("uss_outage")
	assert.True(t, ok, "a transient failure must not delete the session")
	assert.False(t, stored.OIDCRefreshDead)

	// Once the retry interval has passed, the next request tries again.
	sm.refreshRetry.mu.Lock()
	sm.refreshRetry.until["uss_outage"] = time.Now().Add(-time.Second)
	sm.refreshRetry.mu.Unlock()
	provider.respond(http.StatusOK, okTokenResponse(300))
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
	provider.respond(http.StatusOK, okTokenResponse(3600))
	sm, tbl, _ := newRefreshTestManager(t, provider, false, expiredOIDCSession("uss_burst"))

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
// rejects the stale one; the session must be neither retired nor deleted.
func TestGetSessionFromRequest_InvalidGrantAfterRotationElsewhereKeepsSession(t *testing.T) {
	for _, endSession := range []bool{false, true} {
		t.Run(fmt.Sprintf("end_session=%v", endSession), func(t *testing.T) {
			provider := newFakeTokenEndpoint(t)
			provider.respond(http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": "Stale token"})
			sm, tbl, _ := newRefreshTestManager(t, provider, endSession, expiredOIDCSession("uss_rotated"))

			// The other replica stores its refreshed tokens while this one's
			// call to the provider is in flight.
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
			stored, ok := tbl.get("uss_rotated")
			require.True(t, ok)
			assert.False(t, stored.OIDCRefreshDead)
		})
	}
}

// If the session cannot be re-read after losing the rotation race, the
// failure is treated as transient: nothing is retired and the refresh backs off.
func TestGetSessionFromRequest_RereadFailureAfterRotationIsTransient(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusBadRequest, sessionNotActive)
	sm, tbl, _ := newRefreshTestManager(t, provider, true, expiredOIDCSession("uss_reread"))
	provider.beforeAnswer = func() {
		tbl.mu.Lock()
		defer tbl.mu.Unlock()
		s := tbl.sessions["uss_reread"]
		s.OIDCRefreshToken = "rotated_refresh_token"
		tbl.sessions["uss_reread"] = s
		tbl.readErr = errors.New("connection reset")
		tbl.readsBeforeErr = tbl.reads
	}

	session, err := sm.GetSessionFromRequest(context.Background(), requestWithSession("uss_reread"))
	require.NoError(t, err, "a transient failure leaves the request authenticated")
	require.NotNil(t, session)
	_, ok := tbl.get("uss_reread")
	assert.True(t, ok)
	assert.True(t, sm.refreshRetry.blocked("uss_reread", time.Now()))
}

// A realm-wide event must not retire every session: past the limit per
// minute, further ended sessions are left alone and retried later.
func TestGetSessionFromRequest_EndedSessionLimit(t *testing.T) {
	for _, endSession := range []bool{false, true} {
		t.Run(fmt.Sprintf("end_session=%v", endSession), func(t *testing.T) {
			provider := newFakeTokenEndpoint(t)
			provider.respond(http.StatusBadRequest, sessionNotActive)
			var sessions []types.UserSession
			for i := range endedSessionLimit + 5 {
				sessions = append(sessions, expiredOIDCSession(fmt.Sprintf("uss_%02d", i)))
			}
			sm, tbl, _ := newRefreshTestManager(t, provider, endSession, sessions...)

			for _, s := range sessions {
				_, _ = sm.GetSessionFromRequest(context.Background(), requestWithSession(s.ID))
			}

			retired := 0
			for _, s := range sessions {
				stored, ok := tbl.get(s.ID)
				if !ok || stored.OIDCRefreshDead {
					retired++
				} else {
					assert.True(t, sm.refreshRetry.blocked(s.ID, time.Now()), "a session past the limit backs off")
				}
			}
			assert.Equal(t, endedSessionLimit, retired)
		})
	}
}

// The background refresher retires ended sessions too, and then leaves them alone.
func TestRefreshExpiredSessions_SkipsDeadSessions(t *testing.T) {
	provider := newFakeTokenEndpoint(t)
	provider.respond(http.StatusBadRequest, sessionNotActive)
	live := expiredOIDCSession("uss_live")
	dead := expiredOIDCSession("uss_dead")
	dead.OIDCRefreshDead = true
	sm, tbl, st := newRefreshTestManager(t, provider, false, live, dead)
	st.EXPECT().GetUserSessionsNearOIDCExpiry(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, time.Time) ([]*types.UserSession, error) {
		var out []*types.UserSession
		for _, id := range []string{"uss_live", "uss_dead"} {
			s, _ := tbl.get(id)
			out = append(out, &s)
		}
		return out, nil
	}).Times(2)

	sm.refreshExpiredSessions(context.Background())
	assert.Equal(t, int32(1), provider.calls.Load(), "only the live session is refreshed")
	stored, _ := tbl.get("uss_live")
	assert.True(t, stored.OIDCRefreshDead)

	sm.refreshRetry.succeeded("uss_live")
	sm.refreshExpiredSessions(context.Background())
	assert.Equal(t, int32(1), provider.calls.Load(), "dead sessions are not refreshed")
}
