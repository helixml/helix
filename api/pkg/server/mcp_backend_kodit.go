package server

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit"
	koditapi "github.com/helixml/kodit/infrastructure/api"
	"github.com/rs/zerolog/log"
)

// KoditMCPBackend implements MCPBackend for Kodit code intelligence
// using the in-process kodit library's built-in HTTP handler.
type KoditMCPBackend struct {
	handler http.Handler
	enabled bool
}

// NewKoditMCPBackend creates a new Kodit MCP backend using the kodit library's HTTP handler.
func NewKoditMCPBackend(koditClient *kodit.Client, enabled bool) *KoditMCPBackend {
	if !enabled || koditClient == nil {
		return &KoditMCPBackend{enabled: false}
	}

	apiServer := koditapi.NewAPIServer(koditClient, nil)
	apiServer.MountRoutes()

	return &KoditMCPBackend{
		handler: apiServer.Handler(),
		enabled: true,
	}
}

// ServeHTTP implements MCPBackend
func (b *KoditMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	if !b.enabled {
		http.Error(w, "Kodit is not enabled", http.StatusNotImplemented)
		return
	}

	// Get the path suffix after /api/v1/mcp/kodit
	vars := mux.Vars(r)
	pathSuffix := vars["path"]

	// Build the target path: /mcp or /mcp/{path}
	// Preserve trailing slash from original request to avoid redirect loops
	targetPath := "/mcp"
	if pathSuffix != "" {
		targetPath = "/mcp/" + pathSuffix
	} else if strings.HasSuffix(r.URL.Path, "/") {
		targetPath = "/mcp/"
	}

	log.Debug().
		Str("user_id", user.ID).
		Str("method", r.Method).
		Str("target_path", targetPath).
		Msg("routing MCP request to in-process Kodit handler")

	// Create a modified request with the rewritten path
	r2 := r.Clone(r.Context())
	r2.URL.Path = targetPath
	r2.URL.RawPath = targetPath
	r2.RequestURI = targetPath
	if r.URL.RawQuery != "" {
		r2.RequestURI = targetPath + "?" + r.URL.RawQuery
	}

	b.handler.ServeHTTP(w, r2)
}
