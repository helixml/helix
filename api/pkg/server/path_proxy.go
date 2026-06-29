package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// mountConfiguredProxy mounts a single generic reverse proxy when both
// HELIX_PROXY_PATH_PREFIX and HELIX_PROXY_UPSTREAM are set: requests whose
// path starts with the prefix are forwarded to the upstream, with the
// original Host preserved and X-Forwarded-* set. It is deliberately not
// tied to any specific service — the first use is fronting an external
// OIDC IdP (e.g. Keycloak at /auth/) when Helix terminates TLS itself, so
// that path keeps reaching the IdP without a separate reverse proxy.
//
// It must be registered before the SPA catch-all ("/") so the prefix wins.
// A no-op when either var is empty.
func (apiServer *HelixAPIServer) mountConfiguredProxy(router *mux.Router) error {
	prefix := strings.TrimSpace(apiServer.Cfg.WebServer.ProxyPathPrefix)
	upstream := strings.TrimSpace(apiServer.Cfg.WebServer.ProxyUpstream)
	if prefix == "" && upstream == "" {
		return nil
	}
	if prefix == "" || upstream == "" {
		return fmt.Errorf("HELIX_PROXY_PATH_PREFIX and HELIX_PROXY_UPSTREAM must both be set (got prefix=%q upstream=%q)", prefix, upstream)
	}
	if !strings.HasPrefix(prefix, "/") {
		return fmt.Errorf("HELIX_PROXY_PATH_PREFIX must start with '/' (got %q)", prefix)
	}

	target, err := url.Parse(upstream)
	if err != nil {
		return fmt.Errorf("invalid HELIX_PROXY_UPSTREAM %q: %w", upstream, err)
	}
	if target.Scheme == "" || target.Host == "" {
		return fmt.Errorf("HELIX_PROXY_UPSTREAM %q must be an absolute URL (scheme://host)", upstream)
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Preserve the inbound Host so the upstream (e.g. an IdP that
			// derives issuer/redirect URLs from it) sees the public name.
			if req.Header.Get("X-Forwarded-Host") == "" {
				req.Header.Set("X-Forwarded-Host", req.Host)
			}
			if req.Header.Get("X-Forwarded-Proto") == "" {
				// Helix terminates TLS for the canonical host, so inbound is https.
				proto := "https"
				if req.TLS == nil {
					proto = "http"
				}
				req.Header.Set("X-Forwarded-Proto", proto)
			}
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			// Keep req.Host as the inbound public host (do not overwrite with
			// the upstream host) so the IdP keeps generating public URLs.
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			log.Error().Err(err).Str("upstream", upstream).Msg("configured path proxy: upstream error")
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	router.PathPrefix(prefix).Handler(proxy)
	log.Info().Str("prefix", prefix).Str("upstream", upstream).Msg("mounted configured reverse proxy")
	return nil
}
