package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
)

func newProxyTestServer(prefix, upstream string) *HelixAPIServer {
	return &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{
				ProxyPathPrefix: prefix,
				ProxyUpstream:   upstream,
			},
		},
	}
}

func TestMountConfiguredProxy_DisabledWhenUnset(t *testing.T) {
	s := newProxyTestServer("", "")
	r := mux.NewRouter()
	if err := s.mountConfiguredProxy(r); err != nil {
		t.Fatalf("expected nil error when unset, got %v", err)
	}
	// No route should be registered → catch-all 404.
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot) // sentinel: fell through to catch-all
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "http://app.helix.ml/auth/realms/helix", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected fall-through to catch-all (418), got %d", rec.Code)
	}
}

func TestMountConfiguredProxy_ErrorWhenHalfSet(t *testing.T) {
	for _, tc := range []struct{ prefix, upstream string }{
		{"/auth/", ""},
		{"", "http://keycloak:8080"},
		{"auth/", "http://keycloak:8080"}, // missing leading slash
		{"/auth/", "://bad"},
	} {
		s := newProxyTestServer(tc.prefix, tc.upstream)
		if err := s.mountConfiguredProxy(mux.NewRouter()); err == nil {
			t.Fatalf("expected error for prefix=%q upstream=%q, got nil", tc.prefix, tc.upstream)
		}
	}
}

func TestMountConfiguredProxy_ForwardsPreservingHost(t *testing.T) {
	var gotPath, gotHost, gotXFH, gotXFP string
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		gotPath = req.URL.Path
		gotHost = req.Host
		gotXFH = req.Header.Get("X-Forwarded-Host")
		gotXFP = req.Header.Get("X-Forwarded-Proto")
	}))
	defer upstream.Close()

	s := newProxyTestServer("/auth/", upstream.URL)
	r := mux.NewRouter()
	if err := s.mountConfiguredProxy(r); err != nil {
		t.Fatalf("mountConfiguredProxy: %v", err)
	}

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "http://app.helix.ml/auth/realms/helix/protocol", nil))

	if gotPath != "/auth/realms/helix/protocol" {
		t.Errorf("upstream path = %q, want /auth/realms/helix/protocol", gotPath)
	}
	if gotHost != "app.helix.ml" {
		t.Errorf("upstream Host = %q, want app.helix.ml (inbound host preserved)", gotHost)
	}
	if gotXFH != "app.helix.ml" {
		t.Errorf("X-Forwarded-Host = %q, want app.helix.ml", gotXFH)
	}
	if gotXFP != "http" { // httptest.NewRequest has no TLS → http
		t.Errorf("X-Forwarded-Proto = %q, want http", gotXFP)
	}
}

func TestMountConfiguredProxy_NonPrefixPathUntouched(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway) // should never be hit for /api
	}))
	defer upstream.Close()

	s := newProxyTestServer("/auth/", upstream.URL)
	r := mux.NewRouter()
	if err := s.mountConfiguredProxy(r); err != nil {
		t.Fatalf("mountConfiguredProxy: %v", err)
	}
	r.PathPrefix("/api/").HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "http://app.helix.ml/api/v1/whatever", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("non-prefix path should not be proxied, got %d", rec.Code)
	}
}
