package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/agent"
	mcpclient "github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// ExternalMCPBackend implements MCPBackend for user-configured external MCP servers.
// It acts as an MCP server to Zed and forwards requests as an MCP client to external servers.
//
// Route: /api/v1/mcp/external/{mcp_name}/{path...}
//
// Authentication: Uses session-scoped ephemeral API keys. The key's SessionID
// is used to look up the session, which contains the ParentApp (agent) ID.
// The agent's configured MCPs are checked to find the matching MCP by name.
//
// This uses logical proxying:
// 1. Creates an MCP server that Zed connects to
// 2. When Zed calls a tool, forwards it via MCP client to the external server
// 3. Returns the response back to Zed
//
// Benefits of logical proxying over byte-level:
// - Handles SSE endpoint URL rewriting automatically
// - Can adapt between different MCP transports (SSE vs Streamable HTTP)
// - Provides better error handling and logging
type ExternalMCPBackend struct {
	store        store.Store
	clientGetter mcpclient.ClientGetter

	// Cache of HTTP servers per session+mcp combination
	servers   map[string]*externalMCPServer
	serversMu sync.RWMutex

	// Cleanup goroutine control
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
}

// externalMCPServer holds the MCP server for a specific external MCP
type externalMCPServer struct {
	httpServer *server.StreamableHTTPServer
	mcpName    string
	sessionID  string
	createdAt  time.Time
	lastUsed   time.Time          // Updated on each access to keep connection alive
	cancelFunc context.CancelFunc // Cancel the background context when server is removed from cache
	mu         sync.Mutex         // Protects lastUsed
}

// touch updates the lastUsed timestamp
func (s *externalMCPServer) touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

// isExpired checks if the server has been idle for longer than the TTL
func (s *externalMCPServer) isExpired(ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastUsed) > ttl
}

// NewExternalMCPBackend creates a new external MCP backend
func NewExternalMCPBackend(store store.Store) *ExternalMCPBackend {
	ctx, cancel := context.WithCancel(context.Background())
	b := &ExternalMCPBackend{
		store:         store,
		clientGetter:  &mcpclient.DefaultClientGetter{},
		servers:       make(map[string]*externalMCPServer),
		cleanupCtx:    ctx,
		cleanupCancel: cancel,
	}
	// Start background cleanup goroutine
	go b.cleanupLoop()
	return b
}

// Stop stops the background cleanup goroutine and cleans up all servers
func (b *ExternalMCPBackend) Stop() {
	b.cleanupCancel()

	// Cancel all server contexts
	b.serversMu.Lock()
	for key, srv := range b.servers {
		if srv.cancelFunc != nil {
			srv.cancelFunc()
		}
		delete(b.servers, key)
	}
	b.serversMu.Unlock()

	log.Info().Msg("external MCP backend stopped")
}

// cleanupLoop periodically removes expired servers from the cache
func (b *ExternalMCPBackend) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	cacheTTL := 5 * time.Minute

	for {
		select {
		case <-b.cleanupCtx.Done():
			return
		case <-ticker.C:
			b.cleanupExpiredServers(cacheTTL)
		}
	}
}

// cleanupExpiredServers removes servers that have been idle longer than the TTL
func (b *ExternalMCPBackend) cleanupExpiredServers(ttl time.Duration) {
	var expiredKeys []string
	var expiredServers []*externalMCPServer

	// Find expired servers
	b.serversMu.RLock()
	for key, srv := range b.servers {
		if srv.isExpired(ttl) {
			expiredKeys = append(expiredKeys, key)
			expiredServers = append(expiredServers, srv)
		}
	}
	b.serversMu.RUnlock()

	if len(expiredKeys) == 0 {
		return
	}

	// Remove expired servers
	b.serversMu.Lock()
	for _, key := range expiredKeys {
		delete(b.servers, key)
	}
	b.serversMu.Unlock()

	// Cancel contexts outside the lock to avoid holding it during cleanup
	for _, srv := range expiredServers {
		if srv.cancelFunc != nil {
			srv.cancelFunc()
		}
		log.Debug().
			Str("session_id", srv.sessionID).
			Str("mcp_name", srv.mcpName).
			Msg("cleaned up expired external MCP server")
	}

	log.Info().
		Int("count", len(expiredKeys)).
		Msg("cleaned up expired external MCP servers")
}

// ServeHTTP implements MCPBackend
func (b *ExternalMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	ctx := r.Context()
	vars := mux.Vars(r)

	// The gateway route is /api/v1/mcp/{server}/{path...}
	// For external, path contains: {mcp_name}[/{remaining_path}]
	// e.g., /api/v1/mcp/external/my-mcp-server/sse
	fullPath := vars["path"]
	mcpName, _ := parseMCPPath(fullPath)

	if mcpName == "" {
		http.Error(w, "mcp_name is required in path: /api/v1/mcp/external/{mcp_name}", http.StatusBadRequest)
		return
	}

	// Get session ID from the authenticated user (set from API key)
	sessionID := user.SessionID
	if sessionID == "" {
		log.Warn().
			Str("user_id", user.ID).
			Str("mcp_name", mcpName).
			Msg("no session ID in API key for external MCP")
		http.Error(w, "session-scoped API key required for external MCP access", http.StatusForbidden)
		return
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("mcp_name", mcpName).
		Str("user_id", user.ID).
		Str("method", r.Method).
		Msg("external MCP proxy request")

	// Get or create the MCP server for this session+mcp combination
	httpServer, err := b.getOrCreateServer(ctx, user, sessionID, mcpName)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", sessionID).
			Str("mcp_name", mcpName).
			Msg("failed to create external MCP server")
		http.Error(w, "failed to initialize external MCP proxy: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// The Streamable HTTP server handles MCP requests via POST
	httpServer.ServeHTTP(w, r)
}

// parseMCPPath extracts mcp_name and remaining path from the path suffix
// Format: {mcp_name}[/{remaining_path}]
func parseMCPPath(path string) (mcpName, remaining string) {
	if path == "" {
		return "", ""
	}

	// Remove leading slash if present
	if path[0] == '/' {
		path = path[1:]
	}

	// Find first slash to split mcp_name from remaining path
	for i, c := range path {
		if c == '/' {
			return path[:i], path[i+1:]
		}
	}

	return path, ""
}

// getOrCreateServer gets or creates a Streamable HTTP server for the given session and MCP
func (b *ExternalMCPBackend) getOrCreateServer(ctx context.Context, user *types.User, sessionID, mcpName string) (*server.StreamableHTTPServer, error) {
	// Check cache first (with TTL check)
	cacheKey := fmt.Sprintf("%s:%s", sessionID, mcpName)
	cacheTTL := 5 * time.Minute

	b.serversMu.RLock()
	if srv, ok := b.servers[cacheKey]; ok {
		if !srv.isExpired(cacheTTL) {
			// Refresh the lastUsed timestamp to keep the connection alive
			srv.touch()
			b.serversMu.RUnlock()
			return srv.httpServer, nil
		}
		// TTL expired (idle too long), cancel old context
		if srv.cancelFunc != nil {
			srv.cancelFunc()
		}
	}
	b.serversMu.RUnlock()

	// Get the session to find the parent app
	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Verify the user owns this session
	if session.Owner != user.ID {
		return nil, fmt.Errorf("forbidden: user does not own session")
	}

	// Get the app (agent) to find configured MCPs
	if session.ParentApp == "" {
		return nil, fmt.Errorf("session has no associated agent")
	}

	app, err := b.store.GetApp(ctx, session.ParentApp)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// Find the MCP configuration by name
	mcpConfig := b.findMCPConfig(app, mcpName)
	if mcpConfig == nil {
		return nil, fmt.Errorf("MCP server '%s' not configured for this agent", mcpName)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("mcp_name", mcpName).
		Str("target_url", mcpConfig.URL).
		Str("transport", mcpConfig.Transport).
		Msg("creating external MCP proxy server")

	// Create a background context for the MCP client that outlives the HTTP request.
	// This is critical for SSE transport which maintains a persistent connection.
	// The context is canceled when the server is removed from cache (idle TTL expiry).
	// We don't use a timeout here - the connection stays alive as long as it's being used.
	// The TTL is based on lastUsed timestamp, not creation time.
	clientCtx, clientCancel := context.WithCancel(context.Background())

	// Create MCP client to the external server
	externalClient, err := b.clientGetter.NewClient(clientCtx, agent.Meta{UserID: user.ID}, nil, mcpConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to external MCP server: %w", err)
	}

	// List tools from the external server
	toolsResp, err := externalClient.ListTools(clientCtx, mcp.ListToolsRequest{})
	if err != nil {
		clientCancel()
		return nil, fmt.Errorf("failed to list tools from external MCP server: %w", err)
	}

	// Create our MCP server that proxies to the external server
	mcpServer := server.NewMCPServer(
		fmt.Sprintf("helix-external-proxy-%s", mcpName),
		"1.0.0",
		server.WithLogging(),
	)

	// Register tools from the external server
	for _, tool := range toolsResp.Tools {
		// Create a handler that forwards to the external server
		handler := b.createToolHandler(externalClient, tool.Name)

		// Build tool options
		var opts []mcp.ToolOption
		opts = append(opts, mcp.WithDescription(tool.Description))

		// Add parameters from the tool's input schema
		// mcp.ToolInputSchema has Properties map[string]interface{} and Required []string
		if tool.InputSchema.Properties != nil {
			required := make(map[string]bool)
			for _, r := range tool.InputSchema.Required {
				required[r] = true
			}

			for propName, propDef := range tool.InputSchema.Properties {
				propMap, ok := propDef.(map[string]interface{})
				if !ok {
					continue
				}

				desc := ""
				if d, ok := propMap["description"].(string); ok {
					desc = d
				}

				if required[propName] {
					opts = append(opts, mcp.WithString(propName, mcp.Required(), mcp.Description(desc)))
				} else {
					opts = append(opts, mcp.WithString(propName, mcp.Description(desc)))
				}
			}
		}

		mcpTool := mcp.NewTool(tool.Name, opts...)
		mcpServer.AddTool(mcpTool, handler)

		log.Debug().
			Str("mcp_name", mcpName).
			Str("tool", tool.Name).
			Msg("registered external MCP tool")
	}

	// Create Streamable HTTP server (modern MCP protocol)
	// Use stateless mode so each request is independent
	httpServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithStateLess(true),
	)

	// Cache the server
	now := time.Now()
	b.serversMu.Lock()
	b.servers[cacheKey] = &externalMCPServer{
		httpServer: httpServer,
		mcpName:    mcpName,
		sessionID:  sessionID,
		createdAt:  now,
		lastUsed:   now,
		cancelFunc: clientCancel,
	}
	b.serversMu.Unlock()

	return httpServer, nil
}

// findMCPConfig searches for an MCP configuration by name in the app's assistants
func (b *ExternalMCPBackend) findMCPConfig(app *types.App, mcpName string) *types.AssistantMCP {
	if app.Config.Helix.Assistants == nil {
		return nil
	}

	for _, assistant := range app.Config.Helix.Assistants {
		for i := range assistant.MCPs {
			if assistant.MCPs[i].Name == mcpName {
				return &assistant.MCPs[i]
			}
		}
	}

	return nil
}

// createToolHandler creates a handler that forwards tool calls to the external MCP server
func (b *ExternalMCPBackend) createToolHandler(client mcpclient.Client, toolName string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Debug().
			Str("tool", toolName).
			Interface("arguments", request.Params.Arguments).
			Msg("forwarding tool call to external MCP server")

		// Forward the request to the external server
		result, err := client.CallTool(ctx, mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      toolName,
				Arguments: request.Params.Arguments,
			},
		})
		if err != nil {
			log.Error().Err(err).Str("tool", toolName).Msg("external MCP tool call failed")
			return nil, fmt.Errorf("external MCP tool call failed: %w", err)
		}

		log.Debug().
			Str("tool", toolName).
			Msg("external MCP tool call succeeded")

		return result, nil
	}
}
