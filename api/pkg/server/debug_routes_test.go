package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

const pprofIndexMarker = "Types of profiles available"

var debugProbePaths = []string{
	"/debug/pprof/",
	"/debug/pprof",
	"/debug/pprof/goroutine?debug=1",
	"/debug/pprof/goroutine?debug=2",
	"/debug/pprof/heap",
	"/debug/pprof/allocs",
	"/debug/pprof/cmdline",
	"/debug/pprof/symbol",
	"/debug",
	"/debug/vars",
}

// newDebugTestRouter mirrors registerRoutes: debug routes on the root router,
// followed by the catch-all frontend handler that must not receive /debug.
func newDebugTestRouter(extract func(http.Handler) http.Handler) *mux.Router {
	router := mux.NewRouter()
	registerDebugRoutes(router, extract)
	router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("frontend"))
	})
	return router
}

func asCaller(user *types.User) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if user != nil {
				r = r.WithContext(setRequestUser(r.Context(), *user))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func getDebug(router http.Handler, path string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertDebugHidden(t *testing.T, rec *httptest.ResponseRecorder, path string) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET %s: status %d, want 404", path, rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, pprofIndexMarker) || strings.Contains(body, "goroutine profile:") || body == "frontend" {
		t.Errorf("GET %s leaked content: %q", path, body)
	}
}

// TST-01-050 PoC: `curl -s https://app.helix.ml/debug/pprof/` (unauthenticated)
// returned the pprof index. Run it through the real auth extraction middleware.
func TestDebugRoutes_UnauthenticatedGets404(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := newAuthMiddleware(nil, nil, store.NewMockStore(ctrl), authMiddlewareConfig{runnerToken: "runner-token"}, nil, nil)
	router := newDebugTestRouter(auth.extractMiddleware)

	for _, path := range debugProbePaths {
		assertDebugHidden(t, getDebug(router, path, nil), path)
	}
}

// The runner token authenticates sandbox hosts; it is not an admin credential.
func TestDebugRoutes_RunnerTokenGets404(t *testing.T) {
	ctrl := gomock.NewController(t)
	auth := newAuthMiddleware(nil, nil, store.NewMockStore(ctrl), authMiddlewareConfig{runnerToken: "runner-token"}, nil, nil)
	router := newDebugTestRouter(auth.extractMiddleware)

	header := http.Header{"Authorization": []string{"Bearer runner-token"}}
	for _, path := range debugProbePaths {
		assertDebugHidden(t, getDebug(router, path, header), path)
	}
}

func TestDebugRoutes_NonAdminUserGets404(t *testing.T) {
	router := newDebugTestRouter(asCaller(&types.User{ID: "user-1", Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey}))
	for _, path := range debugProbePaths {
		assertDebugHidden(t, getDebug(router, path, nil), path)
	}
}

func TestDebugRoutes_AdminCanProfile(t *testing.T) {
	router := newDebugTestRouter(asCaller(&types.User{ID: "admin-1", Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey, Admin: true}))

	rec := getDebug(router, "/debug/pprof/", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), pprofIndexMarker) {
		t.Fatalf("admin GET /debug/pprof/: status %d, body %q", rec.Code, rec.Body.String())
	}
	rec = getDebug(router, "/debug/pprof/goroutine?debug=1", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "goroutine profile:") {
		t.Fatalf("admin GET goroutine: status %d", rec.Code)
	}
	// Non-pprof /debug paths stay closed even for admins.
	if rec := getDebug(router, "/debug/vars", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("admin GET /debug/vars: status %d, want 404", rec.Code)
	}
}
