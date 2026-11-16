package server

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// securityHeadersMiddleware sets the response headers every page and API
// response on the application host carries. Defaults are written before the
// handler runs, so a handler that sets its own value (a stricter
// Referrer-Policy, a cacheable Cache-Control) replaces them.
//
//   - X-Content-Type-Options: nosniff and an explicit Referrer-Policy on every
//     response.
//   - Strict-Transport-Security when SERVER_URL is https and the request is
//     for that host. It names the application host only (no includeSubDomains):
//     other names under the same domain may legitimately serve plain HTTP.
//   - Cache-Control: no-store on /api/ responses unless the handler chooses
//     otherwise, so credential-bearing JSON is never stored by a browser or an
//     intermediary regardless of how the edge is configured.
//
// The HTML shell's own policy (CSP, Permissions-Policy) is set by the SPA
// handler, which knows which responses are the shell.
func securityHeadersMiddleware(serverURL string, hstsMaxAge time.Duration, next http.Handler) http.Handler {
	hstsHost, hstsValue := hstsPolicy(serverURL, hstsMaxAge)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if hstsValue != "" && strings.EqualFold(stripPort(r.Host), hstsHost) {
			header.Set("Strict-Transport-Security", hstsValue)
		}
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			header.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// hstsPolicy returns the host HSTS applies to and the header value, or empty
// strings when HSTS is off: disabled by configuration or SERVER_URL is not
// https.
func hstsPolicy(serverURL string, maxAge time.Duration) (string, string) {
	if maxAge <= 0 {
		return "", ""
	}
	u, err := url.Parse(serverURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", ""
	}
	return strings.ToLower(u.Hostname()), "max-age=" + strconv.FormatInt(int64(maxAge/time.Second), 10)
}
