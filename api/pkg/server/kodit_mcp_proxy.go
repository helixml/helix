package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// koditMCPProxy proxies MCP requests to Kodit's MCP server.
// This allows external clients (e.g., Zed agent) to access Kodit's code search
// capabilities authenticated with their Helix API key.
//
// The proxy:
// 1. Authenticates the user via Helix API key (standard auth middleware)
// 2. Forwards the request to Kodit's MCP endpoint
// 3. Adds the Kodit API key for internal service authentication
//
// Endpoints:
// - GET/POST /api/v1/kodit/mcp - Main MCP endpoint (streamable HTTP)
// - GET/POST /api/v1/kodit/mcp/sse - SSE endpoint for MCP transport
func (s *HelixAPIServer) koditMCPProxy(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !hasUser(user) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if Kodit is enabled
	if !s.Cfg.Kodit.Enabled {
		http.Error(w, "Kodit is not enabled", http.StatusNotImplemented)
		return
	}

	// Parse Kodit base URL
	koditURL, err := url.Parse(s.Cfg.Kodit.BaseURL)
	if err != nil {
		log.Error().Err(err).Str("url", s.Cfg.Kodit.BaseURL).Msg("failed to parse Kodit base URL")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Get the path suffix after /api/v1/kodit/mcp
	vars := mux.Vars(r)
	pathSuffix := vars["path"]

	// Build the target path: /mcp or /mcp/{path}
	targetPath := "/mcp"
	if pathSuffix != "" {
		targetPath = "/mcp/" + pathSuffix
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
		if s.Cfg.Kodit.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.Cfg.Kodit.APIKey)
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
