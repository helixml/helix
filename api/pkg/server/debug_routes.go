package server

import (
	"net/http"
	"net/http/pprof"
	"strings"

	"github.com/gorilla/mux"
)

// debugPathPrefix is the root of Go's runtime diagnostic surface.
const debugPathPrefix = "/debug"

// isDebugPath reports whether the request targets /debug or anything below it.
func isDebugPath(path string) bool {
	return path == debugPathPrefix || strings.HasPrefix(path, debugPathPrefix+"/")
}

// newPprofHandler returns the net/http/pprof handlers on a private mux, so the
// API never depends on (or exposes) whatever happens to be on
// http.DefaultServeMux.
func newPprofHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/debug/pprof/", pprof.Index) // also serves named profiles (heap, goroutine, ...)
	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)
	return m
}

// requireAdminOrNotFound admits only global admins. Everyone else, including
// anonymous callers, gets a plain 404 so the surface is not advertised.
func requireAdminOrNotFound(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAdmin(getRequestUser(r)) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// registerDebugRoutes claims all of /debug so nothing under it falls through to
// the frontend proxy, and serves pprof only to global admins (TST-01-050).
func registerDebugRoutes(router *mux.Router, extract func(http.Handler) http.Handler) {
	gated := extract(requireAdminOrNotFound(newPprofHandler()))
	router.MatcherFunc(func(r *http.Request, _ *mux.RouteMatch) bool {
		return isDebugPath(r.URL.Path)
	}).Handler(gated)
}
