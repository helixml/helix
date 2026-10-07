package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

var (
	registryTestUser   = types.User{ID: "user-1", Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey}
	registryTestAdmin  = types.User{ID: "admin-1", Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey, Admin: true}
	registryTestRunner = types.User{ID: "runner-system", Type: types.OwnerTypeUser, Token: "runner-token", TokenType: types.TokenTypeRunner}
)

// newSandboxRegistryTestRouter mirrors the router layout in registerRoutes:
// a /api/v1 subrouter that resolves the caller, with auth, runner and
// admin-or-runner subrouters hanging off it.
func newSandboxRegistryTestRouter(apiServer *HelixAPIServer, caller *types.User) *mux.Router {
	router := mux.NewRouter()
	subRouter := router.PathPrefix(APIPrefix).Subrouter()
	subRouter.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if caller != nil {
				r = r.WithContext(setRequestUser(r.Context(), *caller))
			}
			next.ServeHTTP(w, r)
		})
	})
	authRouter := subRouter.MatcherFunc(matchAllRoutes).Subrouter()
	authRouter.Use(requireUser)
	runnerRouter := subRouter.MatcherFunc(matchAllRoutes).Subrouter()
	runnerRouter.Use(requireRunner)
	adminOrRunnerRouter := subRouter.MatcherFunc(matchAllRoutes).Subrouter()
	adminOrRunnerRouter.Use(requireAdminOrRunner)

	apiServer.registerSandboxRegistryRoutes(authRouter, runnerRouter, adminOrRunnerRouter)
	return router
}

func serveRegistry(router *mux.Router, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, APIPrefix+path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// A non-admin user must be refused on every registry route before any store
// access. The mock store has no expectations, so any read or write fails the test.
func TestSandboxRegistryRoutes_NonAdminDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	// blkio is the one route a user may reach; it looks up the session to
	// authorise, and must refuse a session that is not on that sandbox.
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_other").
		Return(&types.Session{ID: "ses_other", Owner: "someone-else", SandboxID: "sbx_1"}, nil)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_missing").
		Return(nil, errors.New("not found"))

	router := newSandboxRegistryTestRouter(&HelixAPIServer{Store: mockStore}, &registryTestUser)

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/sandboxes", ""},
		{http.MethodPost, "/sandboxes/register", `{"id":"sbx_evil","hostname":"evil"}`},
		{http.MethodPost, "/sandboxes/sbx_1/heartbeat", `{"desktop_versions":{"evil":"1"}}`},
		{http.MethodDelete, "/sandboxes/sbx_1", ""},
		{http.MethodGet, "/sandboxes/sbx_1/disk-history", ""},
		{http.MethodGet, "/sandboxes/sbx_1/containers/ses_other/blkio", ""},
		{http.MethodGet, "/sandboxes/sbx_1/containers/ses_missing/blkio", ""},
	}
	for _, tc := range cases {
		rec := serveRegistry(router, tc.method, tc.path, tc.body)
		if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as non-admin: got %d, want 401/403 (body %q)", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestSandboxRegistryRoutes_UnauthenticatedDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := newSandboxRegistryTestRouter(&HelixAPIServer{Store: store.NewMockStore(ctrl)}, nil)

	for _, path := range []string{"/sandboxes", "/sandbox-desktop-types", "/sandboxes/sbx_1/containers/ses_1/blkio"} {
		if rec := serveRegistry(router, http.MethodGet, path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s unauthenticated: got %d, want 401", path, rec.Code)
		}
	}
}

func TestSandboxRegistryRoutes_AdminAndRunnerCanList(t *testing.T) {
	for name, caller := range map[string]*types.User{"admin": &registryTestAdmin, "runner": &registryTestRunner} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().ListSandboxInstances(gomock.Any()).
				Return([]*types.SandboxInstance{{ID: "sbx_1", Hostname: "host-1"}}, nil)

			router := newSandboxRegistryTestRouter(&HelixAPIServer{Store: mockStore}, caller)
			rec := serveRegistry(router, http.MethodGet, "/sandboxes", "")
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "host-1") {
				t.Fatalf("GET /sandboxes as %s: got %d %q", name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSandboxRegistryRoutes_AdminCannotHeartbeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	router := newSandboxRegistryTestRouter(&HelixAPIServer{Store: store.NewMockStore(ctrl)}, &registryTestAdmin)

	// Host-side calls are for the runner credential only.
	for _, path := range []string{"/sandboxes/register", "/sandboxes/sbx_1/heartbeat"} {
		if rec := serveRegistry(router, http.MethodPost, path, `{"id":"sbx_1"}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("POST %s as admin: got %d, want 401", path, rec.Code)
		}
	}
}

func TestSandboxRegistryRoutes_RunnerCanHeartbeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().UpdateSandboxHeartbeat(gomock.Any(), "sbx_1", gomock.Any()).Return(nil)

	router := newSandboxRegistryTestRouter(&HelixAPIServer{Store: mockStore}, &registryTestRunner)
	if rec := serveRegistry(router, http.MethodPost, "/sandboxes/sbx_1/heartbeat", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("POST heartbeat as runner: got %d %q", rec.Code, rec.Body.String())
	}
}

// Any user may learn which desktop types exist, but nothing else about hosts.
func TestSandboxDesktopTypes_UserSeesTypesOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{
		{ID: "sbx_1", Hostname: "host-1", DesktopVersions: datatypes.JSON(`{"ubuntu":"1.0","sway":"2.0"}`)},
		{ID: "sbx_2", Hostname: "host-2", DesktopVersions: datatypes.JSON(`{"ubuntu":"1.1"}`)},
		{ID: "sbx_3", Hostname: "host-3"},
	}, nil)

	router := newSandboxRegistryTestRouter(&HelixAPIServer{Store: mockStore}, &registryTestUser)
	rec := serveRegistry(router, http.MethodGet, "/sandbox-desktop-types", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /sandbox-desktop-types: got %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `["sway","ubuntu"]` {
		t.Fatalf("desktop types: got %s", got)
	}
}
