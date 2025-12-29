package server

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// MCPBackend interface for MCP server backends
type MCPBackend interface {
	// ServeHTTP handles MCP requests for this backend
	ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User)
}

// MCPGateway routes authenticated MCP requests to registered backends
type MCPGateway struct {
	backends map[string]MCPBackend
}

// NewMCPGateway creates a new MCP gateway
func NewMCPGateway() *MCPGateway {
	return &MCPGateway{
		backends: make(map[string]MCPBackend),
	}
}

// RegisterBackend registers an MCP backend by name
func (g *MCPGateway) RegisterBackend(name string, backend MCPBackend) {
	g.backends[name] = backend
	log.Info().Str("backend", name).Msg("Registered MCP backend")
}

// ServeHTTP handles MCP gateway requests
// Route: /api/v1/mcp/{server}/{path...}
func (g *MCPGateway) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	vars := mux.Vars(r)
	serverName := vars["server"]

	if serverName == "" {
		// List available backends
		g.listBackends(w)
		return
	}

	backend, ok := g.backends[serverName]
	if !ok {
		log.Warn().
			Str("server", serverName).
			Str("user_id", user.ID).
			Msg("MCP backend not found")
		http.Error(w, "MCP server not found: "+serverName, http.StatusNotFound)
		return
	}

	log.Debug().
		Str("server", serverName).
		Str("user_id", user.ID).
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Msg("Routing MCP request to backend")

	backend.ServeHTTP(w, r, user)
}

// listBackends returns list of available MCP backends
func (g *MCPGateway) listBackends(w http.ResponseWriter) {
	var names []string
	for name := range g.backends {
		names = append(names, name)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"backends":["` + strings.Join(names, `","`) + `"]}`))
}

// mcpGatewayHandler wraps the gateway for use with auth middleware
func (s *HelixAPIServer) mcpGatewayHandler(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !hasUser(user) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	s.mcpGateway.ServeHTTP(w, r, user)
}
