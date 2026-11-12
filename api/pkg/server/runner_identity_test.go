package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	testSharedRunnerToken = "shared-runner-token-value"
	// testHost1Token is a per-runner token bound to "host1".
	testHost1Token = types.RunnerTokenPrefix + "host1-secret-0123456789abcdefghijklmnopqrstuv"
	// testRevokedToken is a per-runner token that has been revoked.
	testRevokedToken = types.RunnerTokenPrefix + "revoked-secret-0123456789abcdefghijklmnopqrst"
)

// runnerAuthFixture serves the runner-facing routes behind the real auth
// middleware, the way registerRoutes wires them.
type runnerAuthFixture struct {
	store     *store.MockStore
	apiServer *HelixAPIServer
	connman   *connman.ConnectionManager
	srv       *httptest.Server
}

func newRunnerAuthFixture(t *testing.T, requirePerHost bool) *runnerAuthFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetActiveRunnerTokenByHash(gomock.Any(), types.HashRunnerToken(testHost1Token)).
		Return(&types.RunnerToken{ID: "rtok_1", RunnerID: "host1"}, nil).AnyTimes()
	st.EXPECT().GetActiveRunnerTokenByHash(gomock.Any(), types.HashRunnerToken(testRevokedToken)).
		Return(nil, store.ErrNotFound).AnyTimes()
	// RevDial host registrations auto-register the sandbox row.
	st.EXPECT().ListSandboxInstances(gomock.Any()).Return(nil, nil).AnyTimes()
	st.EXPECT().RegisterSandboxInstance(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	cfg := &config.ServerConfig{}
	cfg.WebServer.RunnerToken = testSharedRunnerToken
	cfg.WebServer.RunnerRequirePerHostTokens = requirePerHost

	cm := connman.New()
	t.Cleanup(cm.Stop)
	apiServer := &HelixAPIServer{Cfg: cfg, Store: st, connman: cm}
	apiServer.authMiddleware = newAuthMiddleware(nil, nil, st, authMiddlewareConfig{runnerToken: testSharedRunnerToken}, cfg, nil)

	router := mux.NewRouter()
	sub := router.PathPrefix(APIPrefix).Subrouter()
	sub.Use(apiServer.authMiddleware.extractMiddleware)
	authRouter := sub.MatcherFunc(matchAllRoutes).Subrouter()
	authRouter.Use(requireUser)
	runnerRouter := sub.MatcherFunc(matchAllRoutes).Subrouter()
	runnerRouter.Use(requireRunner)
	adminOrRunnerRouter := sub.MatcherFunc(matchAllRoutes).Subrouter()
	adminOrRunnerRouter.Use(requireAdminOrRunner)

	authRouter.Handle("/revdial", apiServer.handleRevDial()).Methods(http.MethodGet)
	apiServer.registerSandboxRegistryRoutes(authRouter, runnerRouter, adminOrRunnerRouter)
	runnerRouter.HandleFunc("/runner/{runner_id}/assignment", apiServer.getRunnerAssignment).Methods(http.MethodGet)
	runnerRouter.HandleFunc("/runner/profiles/{id}", apiServer.getRunnerProfile).Methods(http.MethodGet)
	apiServer.startRunnerWebSocketServer(context.Background(), router.PathPrefix(APIPrefix).Subrouter(), "/ws/runner")

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return &runnerAuthFixture{store: st, apiServer: apiServer, connman: cm, srv: srv}
}

// dialRevDial opens a RevDial control connection with a bearer token and
// returns the HTTP status of the upgrade.
func (f *runnerAuthFixture) dialRevDial(t *testing.T, token, runnerID string) int {
	t.Helper()
	url := "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/api/v1/revdial?runnerid=" + runnerID
	header := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		if resp == nil {
			t.Fatalf("dial %s: %v", runnerID, err)
		}
		return resp.StatusCode
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	return resp.StatusCode
}

func (f *runnerAuthFixture) do(t *testing.T, method, path, token string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		bts, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(bts)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, f.srv.URL+path, reader)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	return resp
}

func (f *runnerAuthFixture) registered(id string) bool {
	for _, key := range f.connman.List() {
		if key == id {
			return true
		}
	}
	return false
}

// captureLog redirects the global logger for the rest of the test.
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	previous := log.Logger
	log.Logger = zerolog.New(buf)
	t.Cleanup(func() { log.Logger = previous })
	return buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRunnerIDsMatch(t *testing.T) {
	for _, tc := range []struct {
		bound, presented string
		want             bool
	}{
		{"host1", "host1", true},
		{"host1", "hydra-host1", true},
		{"host1", "sandbox-host1", true},
		{"hydra-host1", "host1", true},
		{"host1", "host2", false},
		{"host1", "hydra-host2", false},
		{"host1", "hydra-sandbox-host1", false},
		{"host1", "desktop-host1", false},
		{"host1", "", false},
		{"", "host1", false},
	} {
		if got := types.RunnerIDsMatch(tc.bound, tc.presented); got != tc.want {
			t.Errorf("RunnerIDsMatch(%q, %q) = %v, want %v", tc.bound, tc.presented, got, tc.want)
		}
	}
}

func TestRunnerUserFromToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetActiveRunnerTokenByHash(gomock.Any(), types.HashRunnerToken(testHost1Token)).
		Return(&types.RunnerToken{ID: "rtok_1", RunnerID: "host1"}, nil)
	st.EXPECT().GetActiveRunnerTokenByHash(gomock.Any(), types.HashRunnerToken(testRevokedToken)).
		Return(nil, store.ErrNotFound)
	ctx := context.Background()

	shared, ok, err := runnerUserFromToken(ctx, st, testSharedRunnerToken, testSharedRunnerToken)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, isRunner(shared))
	require.Empty(t, shared.RunnerID, "the shared token names no runner")

	bound, ok, err := runnerUserFromToken(ctx, st, testSharedRunnerToken, testHost1Token)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, isRunner(bound))
	require.Equal(t, "host1", bound.RunnerID)

	_, ok, err = runnerUserFromToken(ctx, st, testSharedRunnerToken, testRevokedToken)
	require.True(t, ok)
	require.ErrorIs(t, err, errRunnerTokenInvalid)

	// Anything else is not a runner credential (the store is not consulted).
	_, ok, err = runnerUserFromToken(ctx, st, testSharedRunnerToken, types.APIKeyPrefix+"user-key")
	require.NoError(t, err)
	require.False(t, ok)

	// An unset shared token never matches an empty or any other token.
	_, ok, _ = runnerUserFromToken(ctx, st, "", "")
	require.False(t, ok)
}

func TestRunnerToken_RevDialRegistrationBinding(t *testing.T) {
	f := newRunnerAuthFixture(t, false)

	require.Equal(t, http.StatusSwitchingProtocols, f.dialRevDial(t, testHost1Token, "hydra-host1"))
	require.Equal(t, http.StatusSwitchingProtocols, f.dialRevDial(t, testHost1Token, "sandbox-host1"))
	waitFor(t, func() bool { return f.registered("hydra-host1") && f.registered("sandbox-host1") })

	require.Equal(t, http.StatusForbidden, f.dialRevDial(t, testHost1Token, "hydra-host2"))
	require.Equal(t, http.StatusForbidden, f.dialRevDial(t, testHost1Token, "sandbox-host2"))
	require.Equal(t, http.StatusForbidden, f.dialRevDial(t, testHost1Token, "desktop-ses_host1"))
	require.False(t, f.registered("hydra-host2"))
	require.False(t, f.registered("sandbox-host2"))

	require.Equal(t, http.StatusUnauthorized, f.dialRevDial(t, testRevokedToken, "hydra-host1"))
}

// With the switch off, the shared token still registers any host tunnel, and
// each runner id is warned about once.
func TestRunnerToken_SharedTokenAllowedWithDeprecationWarning(t *testing.T) {
	buf := captureLog(t)
	f := newRunnerAuthFixture(t, false)

	require.Equal(t, http.StatusSwitchingProtocols, f.dialRevDial(t, testSharedRunnerToken, "hydra-host1"))
	require.Equal(t, http.StatusSwitchingProtocols, f.dialRevDial(t, testSharedRunnerToken, "hydra-host1"))
	require.Equal(t, http.StatusSwitchingProtocols, f.dialRevDial(t, testSharedRunnerToken, "hydra-host2"))
	waitFor(t, func() bool { return f.registered("hydra-host1") && f.registered("hydra-host2") })

	resp := f.do(t, http.MethodPost, "/api/v1/sandboxes/register", testSharedRunnerToken, types.SandboxInstance{ID: "host1"})
	require.Equal(t, http.StatusOK, resp.StatusCode)

	warnings := strings.Count(buf.String(), "is using the shared RUNNER_TOKEN")
	require.Equal(t, 2, warnings, "one deprecation warning per runner (host1, host2):\n%s", buf.String())
	require.NotContains(t, buf.String(), testSharedRunnerToken)
}

func TestRunnerToken_SharedTokenRefusedWhenPerHostRequired(t *testing.T) {
	f := newRunnerAuthFixture(t, true)

	require.Equal(t, http.StatusForbidden, f.dialRevDial(t, testSharedRunnerToken, "hydra-host1"))
	require.False(t, f.registered("hydra-host1"))
	require.Equal(t, http.StatusForbidden,
		f.do(t, http.MethodPost, "/api/v1/sandboxes/register", testSharedRunnerToken, types.SandboxInstance{ID: "host1"}).StatusCode)
	require.Equal(t, http.StatusForbidden,
		f.do(t, http.MethodGet, "/api/v1/runner/host1/assignment", testSharedRunnerToken, nil).StatusCode)
	require.Equal(t, http.StatusForbidden,
		f.do(t, http.MethodGet, "/api/v1/runner/profiles/p1", testSharedRunnerToken, nil).StatusCode)

	// The per-runner token keeps working.
	require.Equal(t, http.StatusSwitchingProtocols, f.dialRevDial(t, testHost1Token, "hydra-host1"))

	// The shared token is still a runner credential for routes that are not
	// about one runner.
	f.store.EXPECT().ListSandboxInstances(gomock.Any()).Return(nil, nil).AnyTimes()
	require.Equal(t, http.StatusOK, f.do(t, http.MethodGet, "/api/v1/sandboxes", testSharedRunnerToken, nil).StatusCode)
}

// The sandbox registry and runner assignment routes act on the caller's own
// runner id only. A refused request must not reach the store.
func TestRunnerToken_RunnerScopedRoutesBinding(t *testing.T) {
	f := newRunnerAuthFixture(t, false)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"register another sandbox", http.MethodPost, "/api/v1/sandboxes/register", types.SandboxInstance{ID: "host2"}},
		{"heartbeat for another sandbox", http.MethodPost, "/api/v1/sandboxes/host2/heartbeat", types.SandboxHeartbeatRequest{}},
		{"deregister another sandbox", http.MethodDelete, "/api/v1/sandboxes/host2", nil},
		{"another sandbox's disk history", http.MethodGet, "/api/v1/sandboxes/host2/disk-history", nil},
		{"another runner's assignment", http.MethodGet, "/api/v1/runner/host2/assignment", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := f.do(t, tc.method, tc.path, testHost1Token, tc.body)
			require.Equal(t, http.StatusForbidden, resp.StatusCode)
		})
	}

	// Its own runner id works.
	resp := f.do(t, http.MethodPost, "/api/v1/sandboxes/register", testHost1Token, types.SandboxInstance{ID: "host1"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	f.store.EXPECT().GetRunnerAssignment(gomock.Any(), "host1").Return(&types.RunnerAssignment{RunnerID: "host1", ProfileID: "p1"}, nil).AnyTimes()
	require.Equal(t, http.StatusOK, f.do(t, http.MethodGet, "/api/v1/runner/host1/assignment", testHost1Token, nil).StatusCode)

	// A profile other than the one assigned to host1 is refused.
	require.Equal(t, http.StatusForbidden, f.do(t, http.MethodGet, "/api/v1/runner/profiles/p2", testHost1Token, nil).StatusCode)

	// A revoked token is not a credential at all.
	require.Equal(t, http.StatusUnauthorized,
		f.do(t, http.MethodPost, "/api/v1/sandboxes/register", testRevokedToken, types.SandboxInstance{ID: "host1"}).StatusCode)
}

func TestRunnerToken_RunnerWebSocketBinding(t *testing.T) {
	f := newRunnerAuthFixture(t, false)
	dial := func(token, runnerID string) int {
		url := "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/api/v1/ws/runner?runnerid=" + runnerID
		conn, resp, err := websocket.DefaultDialer.Dial(url, http.Header{"Authorization": []string{"Bearer " + token}})
		if err != nil {
			require.NotNil(t, resp, err)
			return resp.StatusCode
		}
		conn.Close()
		return resp.StatusCode
	}
	require.Equal(t, http.StatusForbidden, dial(testHost1Token, "host2"))
	require.Equal(t, http.StatusUnauthorized, dial(testRevokedToken, "host1"))
	require.Equal(t, http.StatusSwitchingProtocols, dial(testHost1Token, "host1"))
}

func TestRunnerToken_ExternalAgentRunnerWebSocketBinding(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetActiveRunnerTokenByHash(gomock.Any(), types.HashRunnerToken(testHost1Token)).
		Return(&types.RunnerToken{ID: "rtok_1", RunnerID: "host1"}, nil).AnyTimes()
	st.EXPECT().GetActiveRunnerTokenByHash(gomock.Any(), types.HashRunnerToken(testRevokedToken)).
		Return(nil, store.ErrNotFound).AnyTimes()
	cfg := &config.ServerConfig{}
	cfg.WebServer.RunnerToken = testSharedRunnerToken
	server := &HelixAPIServer{Cfg: cfg, Store: st}
	router := mux.NewRouter()
	server.startExternalAgentRunnerWebSocketServer(router, "/ws/external-agent-runner")

	status := func(runnerID, token string) int {
		req := httptest.NewRequest(http.MethodGet, "/ws/external-agent-runner?runnerid="+runnerID+"&access_token="+token, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr.Code
	}
	require.Equal(t, http.StatusForbidden, status("host2", testHost1Token))
	require.Equal(t, http.StatusUnauthorized, status("host1", testRevokedToken))

	cfg.WebServer.RunnerRequirePerHostTokens = true
	require.Equal(t, http.StatusForbidden, status("host1", testSharedRunnerToken))
}

func TestRunnerNATSProxy_NotRegisteredWhenPerHostTokensRequired(t *testing.T) {
	apiServer := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	apiServer.Cfg.WebServer.RunnerNATSProxyEnabled = true
	apiServer.Cfg.WebServer.RunnerRequirePerHostTokens = true
	router := mux.NewRouter()
	apiServer.registerRunnerNATSProxy(router)
	var match mux.RouteMatch
	require.False(t, router.Match(httptest.NewRequest(http.MethodGet, "/runner/any-runner/ws", nil), &match),
		"the NATS proxy only accepts the shared token, so it stays off once hosts must use per-runner tokens")
}
