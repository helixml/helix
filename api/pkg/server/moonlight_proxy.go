package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/rs/zerolog/log"
)

// proxyToMoonlightWeb reverse proxies requests to the moonlight-web service
func (apiServer *HelixAPIServer) proxyToMoonlightWeb(w http.ResponseWriter, r *http.Request) {
	// Moonlight Web service URL (from docker-compose network)
	moonlightWebURL := "http://moonlight-web:8080"

	// Parse the target URL
	target, err := url.Parse(moonlightWebURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse moonlight-web URL")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Modify the request to remove /moonlight prefix
	originalPath := r.URL.Path
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/moonlight")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	log.Debug().
		Str("original_path", originalPath).
		Str("proxy_path", r.URL.Path).
		Str("target", target.String()).
		Msg("Proxying to moonlight-web")

	// Proxy the request
	proxy.ServeHTTP(w, r)
}
