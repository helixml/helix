package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// KoditMCPBackend implements MCPBackend for Kodit code intelligence
type KoditMCPBackend struct {
	cfg *config.Kodit
}

// NewKoditMCPBackend creates a new Kodit MCP backend
func NewKoditMCPBackend(cfg *config.Kodit) *KoditMCPBackend {
	return &KoditMCPBackend{cfg: cfg}
}

// ServeHTTP implements MCPBackend
func (b *KoditMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	// Check if Kodit is enabled
	if !b.cfg.Enabled {
		http.Error(w, "Kodit is not enabled", http.StatusNotImplemented)
		return
	}

	// Parse Kodit base URL
	koditURL, err := url.Parse(b.cfg.BaseURL)
	if err != nil {
		log.Error().Err(err).Str("url", b.cfg.BaseURL).Msg("failed to parse Kodit base URL")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Get the path suffix after /api/v1/mcp/kodit
	// The gateway route is /api/v1/mcp/{server}/{path...}
	vars := mux.Vars(r)
	pathSuffix := vars["path"]

	// Build the target path: /mcp or /mcp/{path}
	// Preserve trailing slash from original request to avoid redirect loops
	// (Kodit/uvicorn redirects /mcp to /mcp/)
	targetPath := "/mcp"
	if pathSuffix != "" {
		targetPath = "/mcp/" + pathSuffix
	} else if strings.HasSuffix(r.URL.Path, "/") {
		// Original request had trailing slash, preserve it
		targetPath = "/mcp/"
	}

	log.Debug().
		Str("user_id", user.ID).
		Str("method", r.Method).
		Str("target_path", targetPath).
		Str("kodit_url", koditURL.String()).
		Msg("proxying MCP request to Kodit")

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(koditURL)

	// Configure the director to modify the request
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Set the target path
		req.URL.Path = targetPath
		req.URL.RawPath = targetPath

		// Add Kodit API key for authentication
		if b.cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+b.cfg.APIKey)
		}

		// Remove the original Authorization header (user's Helix API key)
		// to avoid confusion with Kodit's auth
		// The user is already authenticated by Helix, now we use Kodit's internal key
		req.Header.Del("X-Api-Key") // In case it was passed as header

		// Preserve important headers for MCP protocol
		req.Host = koditURL.Host

		log.Debug().
			Str("final_url", req.URL.String()).
			Str("host", req.Host).
			Msg("forwarding MCP request to Kodit")
	}

	// Handle SSE responses properly - don't buffer
	proxy.FlushInterval = -1 // Flush immediately for streaming

	// Capture original request host for rewriting redirect Location headers
	originalScheme := "http"
	if r.TLS != nil {
		originalScheme = "https"
	}
	// Check X-Forwarded-Proto header (set by reverse proxies like nginx)
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		originalScheme = proto
	}
	originalHost := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		originalHost = fwdHost
	}

	// Rewrite redirect Location headers to use the original client-facing URL
	// instead of the internal Kodit URL
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			if location := resp.Header.Get("Location"); location != "" {
				// Parse the Location URL
				locURL, err := url.Parse(location)
				if err != nil {
					return nil // Leave malformed URLs unchanged
				}

				// Check if this is pointing to the internal Kodit host
				if locURL.Host == koditURL.Host || locURL.Host == "" {
					// Rewrite to external URL
					// /mcp/... -> /api/v1/mcp/kodit/...
					newPath := strings.TrimPrefix(locURL.Path, "/mcp")
					if newPath == "" {
						newPath = "/"
					}
					locURL.Scheme = originalScheme
					locURL.Host = originalHost
					locURL.Path = "/api/v1/mcp/kodit" + newPath

					resp.Header.Set("Location", locURL.String())
					log.Debug().
						Str("original_location", location).
						Str("rewritten_location", locURL.String()).
						Msg("rewrote redirect Location header")
				}
			}
		}
		return nil
	}

	// Error handler
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error().
			Err(err).
			Str("url", r.URL.String()).
			Msg("Kodit MCP proxy error")

		if strings.Contains(err.Error(), "connection refused") {
			http.Error(w, "Kodit service unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "proxy error: "+err.Error(), http.StatusBadGateway)
	}

	// Serve the request
	proxy.ServeHTTP(w, r)
}
