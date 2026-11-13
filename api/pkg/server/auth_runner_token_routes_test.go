package server

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The runner token authenticates sandbox hosts and kodit, not a person. These
// tests drive the production router: user routes refuse it, and every route a
// runner-token holder calls still serves it.

func newRunnerTokenTestServer(t *testing.T) (*HelixAPIServer, *storeCallLog) {
	t.Helper()
	st, calls := newVictimStore(t)
	s := newRouteTestServer(t, st)
	s.authMiddleware.lastSeenCache.Store(routeTestUserID, time.Now().Add(time.Hour))
	return s, calls
}

func serveWithToken(s *HelixAPIServer, calls *storeCallLog, method, path, body, token string) (probeOutcome, string) {
	req := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	id := calls.start()
	out := serveProbe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.router.ServeHTTP(rec, r)
		w.WriteHeader(rec.Code)
	}), req, id)
	out.calls = calls.take()
	return out, rec.Body.String()
}

// refusedByGate reports whether an auth gate, not the handler, turned the
// request away.
func refusedByGate(status int, body string) bool {
	body = strings.TrimSpace(body)
	return (status == http.StatusForbidden && body == errRunnerTokenNotAccepted) ||
		(status == http.StatusUnauthorized && body == "Unauthorized")
}

func TestRunnerToken_RefusedOnUserRoutes(t *testing.T) {
	s, calls := newRunnerTokenTestServer(t)
	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/secrets", ""},
		{http.MethodPost, "/api/v1/secrets", `{"name":"x","value":"y"}`},
		{http.MethodGet, "/api/v1/apps", ""},
		{http.MethodPost, "/api/v1/apps", `{}`},
		{http.MethodGet, "/api/v1/api_keys", ""},
		{http.MethodPost, "/api/v1/api_keys", `{}`},
		{http.MethodDelete, "/api/v1/api_keys?key=hl-x", ""},
		{http.MethodGet, "/api/v1/organizations", ""},
		{http.MethodPost, "/api/v1/organizations", `{"name":"x"}`},
		{http.MethodGet, "/api/v1/sessions", ""},
		{http.MethodPut, "/api/v1/users/me/guidelines", `{}`},
		{http.MethodGet, "/api/v1/filestore/list?path=x", ""},
		{http.MethodGet, "/api/v1/sessions/ses_route_victim/toc", ""},
		{http.MethodDelete, "/api/v1/admin/users/usr_route_victim", ""},
		// Handler-auth routes that resolve the token themselves.
		{http.MethodGet, "/api/v1/filestore/viewer/dev/users/usr_route_victim/secret.txt", ""},
		{http.MethodGet, "/api/v1/filestore/viewer/dev/users/runner-system/x.txt", ""},
		{http.MethodGet, "/api/v1/filestore/viewer/dev/apps/app_route_victim/x.txt", ""},
		{http.MethodGet, "/api/v1/ws/user?session_id=ses_route_victim", ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			out, body := serveWithToken(s, calls, tc.method, tc.path, tc.body, routeTestRunnerToken)
			require.False(t, out.hung, "request hung")
			require.Empty(t, out.panic)
			assert.Equal(t, http.StatusForbidden, out.status)
			assert.Equal(t, errRunnerTokenNotAccepted, strings.TrimSpace(body))
			assert.Empty(t, out.calls, "the runner token reached the store")
		})
	}

	// Control: the same routes still serve a user.
	for _, path := range []string{"/api/v1/secrets", "/api/v1/apps", "/api/v1/api_keys", "/api/v1/organizations"} {
		out, body := serveWithToken(s, calls, http.MethodGet, path, "", routeTestUserKey)
		assert.Equal(t, http.StatusOK, out.status, "%s with a user key: %s", path, body)
	}
	// Control: the viewer still lets a user past auth to their own folder (the
	// file does not exist, so the filestore answers, not the access check).
	userPath, err := s.Controller.GetFilestoreUserPath(types.OwnerContext{Owner: routeTestUserID, OwnerType: types.OwnerTypeUser}, "")
	require.NoError(t, err)
	out, body := serveWithToken(s, calls, http.MethodGet, "/api/v1/filestore/viewer/"+userPath+"/missing.txt", "", routeTestUserKey)
	assert.NotEqual(t, http.StatusForbidden, out.status, body)
}

// Every route mounted behind a gate that admits the runner token must still
// serve it. One subtest per route, grouped by gate.
func TestRunnerToken_ServedOnRunnerRoutes(t *testing.T) {
	s, calls := newRunnerTokenTestServer(t)
	byGate := map[routeAuthClass][]string{}
	seen := map[string]bool{}
	for _, r := range walkServedRoutes(t, s.router) {
		if seen[r.Key()] {
			continue
		}
		seen[r.Key()] = true
		switch r.Gate {
		case classRunner, classAdminOrRunner, gateUserOrRunner:
		default:
			continue
		}
		byGate[r.Gate] = append(byGate[r.Gate], r.Key())
		t.Run(string(r.Gate)+"/"+r.Key(), func(t *testing.T) {
			req, err := probeRequest(s.router, r)
			require.NoError(t, err)
			out, body := serveWithToken(s, calls, req.Method, req.URL.RequestURI(), "{}", routeTestRunnerToken)
			if out.hung || out.panic != "" {
				return // the handler ran and hit a subsystem the test server leaves nil
			}
			assert.False(t, refusedByGate(out.status, body), "runner token refused by the gate: %d %s", out.status, body)
		})
	}
	for gate, keys := range byGate {
		sort.Strings(keys)
		t.Logf("%s: %s", gate, strings.Join(keys, ", "))
	}
	for _, gate := range []routeAuthClass{classRunner, classAdminOrRunner, gateUserOrRunner} {
		assert.NotEmpty(t, byGate[gate], "no route is mounted behind the %s gate", gate)
	}

	// The runner-capable user routes are exactly the alsoRunner() policies.
	var want []string
	for key, p := range routeAuthPolicies {
		if p.RunnerAllowed {
			want = append(want, key)
		}
	}
	sort.Strings(want)
	got := append([]string(nil), byGate[gateUserOrRunner]...)
	sort.Strings(got)
	assert.Equal(t, want, got)
}

// Sandbox hosts register their RevDial tunnel with the runner token.
func TestRunnerToken_RevDialRegistrationPassesAuth(t *testing.T) {
	s, calls := newRunnerTokenTestServer(t)
	out, body := serveWithToken(s, calls, http.MethodGet, "/api/v1/revdial?runnerid=hydra-1", "", routeTestRunnerToken)
	// Not a WebSocket upgrade: the handler itself answers 400.
	assert.Equal(t, http.StatusBadRequest, out.status, body)
	assert.Contains(t, body, "WebSocket")
}

// kodit authenticates to the OpenAI-compatible LLM proxy with the runner token
// (KODIT_LLM_API_KEY). Those routes are outside /api/v1 and keep serving it.
func TestRunnerToken_KoditLLMProxyStillServed(t *testing.T) {
	s, calls := newRunnerTokenTestServer(t)

	out, body := serveWithToken(s, calls, http.MethodPost, "/v1/embeddings", `{}`, routeTestRunnerToken)
	require.Empty(t, out.panic)
	assert.Equal(t, http.StatusBadRequest, out.status, body)
	assert.Contains(t, body, "model field is required", "the embeddings handler, not auth, answered")

	out, body = serveWithToken(s, calls, http.MethodPost, "/v1/chat/completions", `not json`, routeTestRunnerToken)
	require.Empty(t, out.panic)
	assert.Equal(t, http.StatusBadRequest, out.status, body)
	assert.NotContains(t, body, "unauthorized")
}

func TestRequireUser_RefusesRunnerIdentity(t *testing.T) {
	runner := types.User{ID: "runner-system", Type: types.OwnerTypeUser, Token: "runner-token", TokenType: types.TokenTypeRunner}
	user := types.User{ID: "usr_1", Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	cases := []struct {
		name       string
		middleware func(http.Handler) http.Handler
		caller     *types.User
		want       int
	}{
		{"requireUser/runner", requireUser, &runner, http.StatusForbidden},
		{"requireUser/user", requireUser, &user, http.StatusNoContent},
		{"requireUser/anonymous", requireUser, nil, http.StatusUnauthorized},
		{"requireUserOrRunner/runner", requireUserOrRunner, &runner, http.StatusNoContent},
		{"requireUserOrRunner/user", requireUserOrRunner, &user, http.StatusNoContent},
		{"requireUserOrRunner/anonymous", requireUserOrRunner, nil, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
			if tc.caller != nil {
				req = req.WithContext(setRequestUser(req.Context(), *tc.caller))
			}
			rec := httptest.NewRecorder()
			tc.middleware(ok).ServeHTTP(rec, req)
			assert.Equal(t, tc.want, rec.Code)
		})
	}
}
