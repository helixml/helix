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

	// Cache of SSE servers per session+mcp combination
	servers   map[string]*externalMCPServer
	serversMu sync.RWMutex
}

// externalMCPServer holds the MCP server for a specific external MCP
type externalMCPServer struct {
	sseServer *server.SSEServer
	mcpName   string
	sessionID string
	createdAt time.Time
}

// NewExternalMCPBackend creates a new external MCP backend
func NewExternalMCPBackend(store store.Store) *ExternalMCPBackend {
	return &ExternalMCPBackend{
		store:        store,
		clientGetter: &mcpclient.DefaultClientGetter{},
		servers:      make(map[string]*externalMCPServer),
	}
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
	sseServer, err := b.getOrCreateServer(ctx, user, sessionID, mcpName)
	if err != nil {
		log.Error().Err(err).
			Str("session_id", sessionID).
			Str("mcp_name", mcpName).
			Msg("failed to create external MCP server")
		http.Error(w, "failed to initialize external MCP proxy: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// The SSE server handles routing based on path:
	// - /sse endpoint for SSE connections
	// - /message endpoint for POST requests
	sseServer.ServeHTTP(w, r)
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

// getOrCreateServer gets or creates an SSE server for the given session and MCP
func (b *ExternalMCPBackend) getOrCreateServer(ctx context.Context, user *types.User, sessionID, mcpName string) (*server.SSEServer, error) {
	// Check cache first (with TTL check)
	cacheKey := fmt.Sprintf("%s:%s", sessionID, mcpName)
	cacheTTL := 5 * time.Minute

	b.serversMu.RLock()
	if srv, ok := b.servers[cacheKey]; ok {
		if time.Since(srv.createdAt) < cacheTTL {
			b.serversMu.RUnlock()
			return srv.sseServer, nil
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

	// Create MCP client to the external server
	externalClient, err := b.clientGetter.NewClient(ctx, agent.Meta{UserID: user.ID}, nil, mcpConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to external MCP server: %w", err)
	}

	// List tools from the external server
	toolsResp, err := externalClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
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

	// Create SSE server
	basePath := fmt.Sprintf("/api/v1/mcp/external/%s", mcpName)
	sseServer := server.NewSSEServer(mcpServer,
		server.WithBasePath(basePath),
	)

	// Cache the server
	b.serversMu.Lock()
	b.servers[cacheKey] = &externalMCPServer{
		sseServer: sseServer,
		mcpName:   mcpName,
		sessionID: sessionID,
		createdAt: time.Now(),
	}
	b.serversMu.Unlock()

	return sseServer, nil
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
