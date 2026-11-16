package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func serveWithSecurityHeaders(serverURL string, hstsMaxAge time.Duration, host, path string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	securityHeadersMiddleware(serverURL, hstsMaxAge, handler).ServeHTTP(rec, req)
	return rec
}

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	for _, path := range []string{"/", "/api/v1/config", "/artifacts/a/document"} {
		rec := serveWithSecurityHeaders("https://app.example.com", 365*24*time.Hour, "app.example.com", path, okHandler)
		require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"), path)
		require.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"), path)
		require.Equal(t, "max-age=31536000", rec.Header().Get("Strict-Transport-Security"), path)
	}
}

func TestSecurityHeadersHSTSOnlyForHTTPSAppHost(t *testing.T) {
	// Another host routed to the app (a custom domain falling through the
	// vhost proxy) is not covered by the policy.
	rec := serveWithSecurityHeaders("https://app.example.com", time.Hour, "other.example.com", "/", okHandler)
	require.Empty(t, rec.Header().Get("Strict-Transport-Security"))

	// The port is not part of the host comparison.
	rec = serveWithSecurityHeaders("https://app.example.com", time.Hour, "app.example.com:443", "/", okHandler)
	require.Equal(t, "max-age=3600", rec.Header().Get("Strict-Transport-Security"))

	// Plain-HTTP deployments never send it.
	rec = serveWithSecurityHeaders("http://localhost:8080", time.Hour, "localhost:8080", "/", okHandler)
	require.Empty(t, rec.Header().Get("Strict-Transport-Security"))

	// Zero disables it.
	rec = serveWithSecurityHeaders("https://app.example.com", 0, "app.example.com", "/", okHandler)
	require.Empty(t, rec.Header().Get("Strict-Transport-Security"))
}

func TestSecurityHeadersAPIResponsesAreNotStored(t *testing.T) {
	rec := serveWithSecurityHeaders("https://app.example.com", time.Hour, "app.example.com", "/api/v1/organizations/org_1/api_keys", okHandler)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	// The SPA handler owns caching for the shell and static assets.
	rec = serveWithSecurityHeaders("https://app.example.com", time.Hour, "app.example.com", "/assets/index.js", okHandler)
	require.Empty(t, rec.Header().Get("Cache-Control"))
}

func TestSecurityHeadersHandlerValuesWin(t *testing.T) {
	rec := serveWithSecurityHeaders("https://app.example.com", time.Hour, "app.example.com", "/api/v1/sessions/ses_1/stream", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "private, no-cache, must-revalidate")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.WriteHeader(http.StatusOK)
	})
	require.Equal(t, []string{"private, no-cache, must-revalidate"}, rec.Header().Values("Cache-Control"))
	require.Equal(t, []string{"no-referrer"}, rec.Header().Values("Referrer-Policy"))
}
