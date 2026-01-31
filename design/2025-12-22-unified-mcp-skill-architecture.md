# Unified MCP Gateway Architecture

**Date:** 2025-12-22
**Status:** Implementation Ready

## Overview

Extend the Kodit MCP proxy pattern into a **generic MCP gateway** that authenticates users and proxies to any configured MCP server. This replaces the CLI-based MCP approach with authenticated HTTP for all agent MCP access.

## Current Architecture

### Kodit MCP Proxy (`kodit_mcp_proxy.go`)
```
Zed Agent → GET/POST /api/v1/kodit/mcp
         ↓
    [Auth: User API Key]
         ↓
    [Add Kodit API Key]
         ↓
    Forward to Kodit MCP server
```

This pattern works well:
- User authenticated via Helix API key
- Internal service auth added by proxy
- SSE/streaming handled correctly

### Problem: Other MCP Sources
- **APIs/Knowledge/Zapier**: Currently use helix-cli MCP (stdio, not HTTP)
- **Custom MCPs**: Passed directly to Zed (no Helix auth layer)
- **Future MCPs**: No pattern for adding new built-in MCPs

## Proposed Architecture

### Generic MCP Gateway

Single endpoint that routes to multiple MCP backends based on path:

```
/api/v1/mcp/{server}/{path...}

Examples:
- /api/v1/mcp/kodit/...       → Kodit MCP server
- /api/v1/mcp/helix/...       → Helix native tools (APIs, Knowledge)
- /api/v1/mcp/github/...      → GitHub MCP (if configured)
```

### Gateway Implementation

```go
// MCPGateway handles authenticated proxying to multiple MCP servers
type MCPGateway struct {
    backends map[string]MCPBackend
}

type MCPBackend interface {
    // ServeHTTP handles the MCP request for this backend
    ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User)
}

// Built-in backends:
// - KoditBackend: forwards to external Kodit service
// - HelixBackend: in-process MCP server for APIs/Knowledge/Zapier
```

### Helix Native MCP Server

Convert existing helix-cli MCP tools to an HTTP MCP server running in the API:

```go
// HelixMCPServer implements MCP protocol over HTTP
type HelixMCPServer struct {
    store     store.Store
    apiRunner *tools.APIRunner
    knowledge *knowledge.Service
}

func (s *HelixMCPServer) ListTools(ctx context.Context, user *types.User, appID string) []mcp.Tool {
    // Return tools from:
    // - App's configured APIs
    // - App's knowledge sources
    // - App's Zapier integrations
}

func (s *HelixMCPServer) CallTool(ctx context.Context, user *types.User, appID string, tool string, args map[string]any) (any, error) {
    // Execute the tool and return results
}
```

### Zed Config Generation

Update `zed_config.go` to use the MCP gateway:

```go
func GenerateZedMCPConfig(...) (*ZedMCPConfig, error) {
    config := &ZedMCPConfig{
        ContextServers: make(map[string]ContextServerConfig),
    }

    // Helix MCP gateway (replaces helix-cli for native tools)
    if hasNativeTools(*assistant) {
        config.ContextServers["helix"] = ContextServerConfig{
            URL: fmt.Sprintf("%s/api/v1/mcp/helix?app_id=%s&session_id=%s",
                helixAPIURL, app.ID, sessionID),
            Headers: map[string]string{
                "Authorization": "Bearer " + helixToken,
            },
        }
    }

    // Kodit via gateway (if enabled for this app/project)
    if koditEnabled {
        config.ContextServers["kodit"] = ContextServerConfig{
            URL: fmt.Sprintf("%s/api/v1/mcp/kodit", helixAPIURL),
            Headers: map[string]string{
                "Authorization": "Bearer " + helixToken,
            },
        }
    }

    // Custom MCPs still supported via direct passthrough
    for _, mcp := range assistant.MCPs {
        config.ContextServers[sanitizeName(mcp.Name)] = mcpToContextServer(mcp)
    }

    return config, nil
}
```

## Implementation Plan

### Phase 1: MCP Gateway Router (15 min)
1. Create `/api/v1/mcp/{server}/{path...}` route
2. Implement `MCPGateway` that routes by server name
3. Move Kodit proxy to be a backend of this gateway

```go
// routes.go
router.PathPrefix("/api/v1/mcp/{server}").Handler(
    authMiddleware(s.mcpGateway.ServeHTTP),
)
```

### Phase 2: Kodit Backend (5 min)
1. Refactor `kodit_mcp_proxy.go` to implement `MCPBackend`
2. Register as "kodit" backend in gateway

### Phase 3: Helix Native Backend (30 min)
1. Create `helix_mcp_server.go` with in-process MCP server
2. Implement `tools/list` returning app's configured tools
3. Implement `tools/call` executing APIs, knowledge queries, Zapier
4. Register as "helix" backend in gateway

### Phase 4: Update Zed Config (10 min)
1. Change `helix-native` from CLI command to HTTP URL
2. Add Authorization header with user token
3. Remove `koditEnabled` parameter (now part of gateway)

### Phase 5: Auto-Enable for SpecTasks (10 min)
1. Check project repos for `kodit_indexing`
2. Auto-add Kodit to ContextServers if repos have indexing enabled

## API Endpoints

### MCP Gateway
```
# List tools from a backend
GET /api/v1/mcp/{server}
Content-Type: application/json
Response: { "tools": [...] }

# Call a tool
POST /api/v1/mcp/{server}
Content-Type: application/json
{ "method": "tools/call", "params": { "name": "search_code", "arguments": {...} } }

# SSE transport (for streaming)
GET /api/v1/mcp/{server}/sse
```

### Available Backends
- `kodit` - Code intelligence (search, architecture, definitions)
- `helix` - Native Helix tools (APIs, knowledge, Zapier)

## Benefits

1. **Unified Authentication**: All MCP access through Helix auth
2. **No CLI Dependency**: External agents don't need helix-cli installed
3. **Centralized Logging**: All MCP calls logged in Helix API
4. **Easier Extension**: Add new backends by implementing MCPBackend
5. **Repository Scoping**: Kodit can be scoped to project repositories

## Migration

- **Backward Compatible**: Existing CLI-based helix-native continues to work
- **Gradual Migration**: Switch to HTTP backend per-app basis
- **SpecTasks**: Use HTTP backend by default (no CLI in sandbox)

## Testing

1. Verify Kodit search works via `/api/v1/mcp/kodit`
2. Verify API tool calls work via `/api/v1/mcp/helix`
3. Verify knowledge queries work via `/api/v1/mcp/helix`
4. Test from Zed agent with HTTP-based context_servers
5. Test authentication - unauthorized requests rejected
