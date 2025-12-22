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
