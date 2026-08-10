package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

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
	servers      map[string]*externalMCPServer
	serversMu    sync.RWMutex
	serverFlight singleflight.Group

	// Cleanup goroutine control
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
}

// externalMCPServer holds the MCP server for a specific external MCP
type externalMCPServer struct {
	httpServer  *server.StreamableHTTPServer
	mcpName     string
	sessionID   string
	createdAt   time.Time
	lastUsed    time.Time          // Updated on each access to keep connection alive
	cancelFunc  context.CancelFunc // Cancel the background context when server is removed from cache
	fingerprint [sha256.Size]byte  // Hash of the MCP config; never stores header secrets directly
	mu          sync.Mutex         // Protects lastUsed
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
	expired := make(map[string]*externalMCPServer)

	// Find expired servers
	b.serversMu.RLock()
	for key, srv := range b.servers {
		if srv.isExpired(ttl) {
			expired[key] = srv
		}
	}
	b.serversMu.RUnlock()

	if len(expired) == 0 {
		return
	}

	removed := b.deleteExpiredServers(expired)

	// Cancel contexts outside the lock to avoid holding it during cleanup
	for _, srv := range removed {
		if srv.cancelFunc != nil {
			srv.cancelFunc()
		}
		log.Debug().
			Str("session_id", srv.sessionID).
			Str("mcp_name", srv.mcpName).
			Msg("cleaned up expired external MCP server")
	}

	if len(removed) > 0 {
		log.Info().
			Int("count", len(removed)).
			Msg("cleaned up expired external MCP servers")
	}
}

func (b *ExternalMCPBackend) deleteExpiredServers(expired map[string]*externalMCPServer) []*externalMCPServer {
	removed := make([]*externalMCPServer, 0, len(expired))
	b.serversMu.Lock()
	for key, snapshot := range expired {
		if current := b.servers[key]; current != snapshot {
			continue
		}
		delete(b.servers, key)
		removed = append(removed, snapshot)
	}
	b.serversMu.Unlock()
	return removed
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
	cacheKey := fmt.Sprintf("%s:%s", sessionID, mcpName)
	result, err, _ := b.serverFlight.Do(cacheKey, func() (any, error) {
		return b.getOrCreateServerOnce(ctx, user, sessionID, mcpName, cacheKey)
	})
	if err != nil {
		return nil, err
	}
	return result.(*server.StreamableHTTPServer), nil
}

func (b *ExternalMCPBackend) getOrCreateServerOnce(ctx context.Context, user *types.User, sessionID, mcpName, cacheKey string) (*server.StreamableHTTPServer, error) {
	cacheTTL := 5 * time.Minute

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

	var projectSkills *types.AssistantSkills
	if session.ProjectID != "" {
		project, err := b.store.GetProject(ctx, session.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("project not found: %w", err)
		}
		projectSkills = project.Skills
	}

	// Find the MCP configuration by name
	mcpConfig := b.findMCPConfig(app, projectSkills, mcpName)
	if mcpConfig == nil {
		return nil, fmt.Errorf("MCP server '%s' not configured for this agent", mcpName)
	}
	configJSON, err := json.Marshal(mcpConfig)
	if err != nil {
		return nil, fmt.Errorf("fingerprint MCP config: %w", err)
	}
	fingerprint := sha256.Sum256(configJSON)

	var stale *externalMCPServer
	b.serversMu.Lock()
	if srv, ok := b.servers[cacheKey]; ok {
		if srv.fingerprint == fingerprint && !srv.isExpired(cacheTTL) {
			srv.touch()
			b.serversMu.Unlock()
			return srv.httpServer, nil
		}
		delete(b.servers, cacheKey)
		stale = srv
	}
	b.serversMu.Unlock()
	if stale != nil && stale.cancelFunc != nil {
		stale.cancelFunc()
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

		mcpTool := buildProxyTool(tool)
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
		httpServer:  httpServer,
		mcpName:     mcpName,
		sessionID:   sessionID,
		createdAt:   now,
		lastUsed:    now,
		cancelFunc:  clientCancel,
		fingerprint: fingerprint,
	}
	b.serversMu.Unlock()

	return httpServer, nil
}

// findMCPConfig searches project MCPs before app MCPs to match Zed config precedence.
func (b *ExternalMCPBackend) findMCPConfig(app *types.App, projectSkills *types.AssistantSkills, mcpName string) *types.AssistantMCP {
	if projectSkills != nil {
		for i := range projectSkills.MCPs {
			if sanitizeMCPName(projectSkills.MCPs[i].Name) == mcpName {
				return &projectSkills.MCPs[i]
			}
		}
	}

	if app.Config.Helix.Assistants == nil {
		return nil
	}

	for _, assistant := range app.Config.Helix.Assistants {
		for i := range assistant.MCPs {
			if sanitizeMCPName(assistant.MCPs[i].Name) == mcpName {
				return &assistant.MCPs[i]
			}
		}
	}

	return nil
}

func sanitizeMCPName(name string) string {
	name = strings.ToLower(name)
	return strings.Trim(strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name), "-")
}

// buildProxyTool reconstructs the schema the proxy advertises to Zed for a
// single upstream tool. It forwards the upstream input schema verbatim so
// array/object/enum/nested params survive — a proxy transports schemas, it
// must not reinterpret them. The previous per-property rebuild flattened every
// param to string, which made array params like create_bot.tools/topics and
// subscribe.topicIds uncallable ("cannot unmarshal string into []string").
// See helix-specs task 002204_the-one-blocker.
func buildProxyTool(tool mcp.Tool) mcp.Tool {
	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		// Degrade to a description-only tool rather than a wrong (all-string)
		// schema; log so the drop is visible.
		log.Error().Err(err).Str("tool", tool.Name).Msg("marshal upstream MCP schema; serving description-only")
		return mcp.NewTool(tool.Name, mcp.WithDescription(tool.Description))
	}
	return mcp.NewToolWithRawSchema(tool.Name, tool.Description, raw)
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
