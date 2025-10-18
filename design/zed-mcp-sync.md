# Design: Sync Helix Skills/Tools to Zed MCP Servers

**Status**: Draft
**Created**: 2025-10-08
**Author**: Claude Code

## Table of Contents
1. [Executive Summary](#executive-summary)
2. [Problem Statement](#problem-statement)
3. [Requirements](#requirements)
4. [Architecture Overview](#architecture-overview)
5. [Detailed Design](#detailed-design)
6. [Implementation Plan](#implementation-plan)
7. [Security Considerations](#security-considerations)
8. [Testing Strategy](#testing-strategy)
9. [Rollout Plan](#rollout-plan)

---

## 1. Executive Summary

This design enables Helix-configured skills and tools to be automatically available in Zed as MCP (Model Context Protocol) servers. The system will:

1. **Pass-through MCP servers** - Direct MCP server configs from Helix → Zed
2. **Expose native Helix skills** - RAG, Vision RAG, OpenAPI integrations via Helix CLI MCP passthrough
3. **Handle OAuth transparently** - Inject OAuth tokens for API integrations
4. **Auto-sync on agent start** - Generate Zed configuration dynamically per agent/PDE

### Key Design Principles
- **No reinventing the wheel** - Reuse existing patterns from code review
- **Transparent to Zed** - All tools appear as standard MCP servers
- **Secure by default** - OAuth tokens isolated per user/session
- **Hot-reload capable** - Configuration updates without container recreation

---

## 2. Problem Statement

### Current State
- Helix apps/agents have rich tool configurations (APIs, RAG, MCP servers)
- External agents (Zed in Wolf containers) don't have access to these tools
- Manual Zed MCP configuration required per agent
- OAuth-protected APIs can't be used in Zed
- No way to access Helix RAG/Knowledge from Zed

### Desired State
- Helix tool configurations automatically sync to Zed
- Native Helix skills (RAG, OpenAPI) accessible as MCP tools
- OAuth tokens transparently injected
- Single source of truth (Helix app config)
- Zero manual Zed configuration

### Success Criteria
1. Agent starts with Zed → All Helix tools available in Zed AI assistant
2. User configures OpenAPI integration with OAuth → Works in Zed automatically
3. RAG knowledge base → Queryable from Zed via MCP
4. External MCP server added to Helix app → Available in Zed immediately

---

## 3. Requirements

### Functional Requirements

**FR1: MCP Server Passthrough**
- Direct MCP server configs from Helix → Zed without modification
- Support HTTP/SSE and stdio MCP transports
- Preserve headers, environment variables, authentication

**FR2: Helix Native Skills as MCP**
- Expose RAG/Knowledge search as MCP tool
- Expose Vision RAG as MCP tool
- Expose OpenAPI integrations as MCP tools
- Expose Browser, Calculator, Email tools as MCP

**FR3: OAuth Token Injection**
- Fetch OAuth tokens for user/session
- Inject into API tool headers automatically
- Handle token refresh/expiry
- Isolate tokens per user (no cross-contamination)

**FR4: Dynamic Configuration**
- Generate Zed MCP config on agent start
- Update config when Helix app config changes
- Support hot-reload without container restart

**FR5: Helix CLI MCP Proxy**
- `helix-cli mcp run` acts as MCP server
- Proxies to Helix API for native tools
- Proxies to external MCP servers
- Handles authentication/authorization

### Non-Functional Requirements

**NFR1: Performance**
- MCP tool calls < 500ms overhead
- Config generation < 100ms
- Minimal memory overhead in Zed

**NFR2: Security**
- OAuth tokens encrypted in transit
- No token leakage in logs/errors
- User isolation enforced
- Audit trail for tool usage

**NFR3: Reliability**
- MCP server failures don't crash Zed
- Graceful degradation (tools unavailable but Zed works)
- Retry logic for transient failures

**NFR4: Maintainability**
- Reuse existing Helix MCP client code
- Minimal Zed code changes
- Clear error messages for debugging

---

## 4. Architecture Overview

### 4.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Helix API Server                         │
│  ┌────────────────────────────────────────────────────┐    │
│  │ App Configuration (app.config.helix.assistants[0]) │    │
│  │  - APIs: [{name, url, oauth, schema}, ...]         │    │
│  │  - MCPs: [{name, url, headers}, ...]               │    │
│  │  - RAG: Knowledge bases                            │    │
│  │  - Tools: [calculated, browser, email, ...]        │    │
│  └────────────────────────────────────────────────────┘    │
│                          ↓                                   │
│  ┌────────────────────────────────────────────────────┐    │
│  │       GenerateZedMCPConfig()                        │    │
│  │  - Convert APIs → helix-cli mcp run invocations    │    │
│  │  - Pass-through MCP servers                        │    │
│  │  - Generate context_servers config                 │    │
│  └────────────────────────────────────────────────────┘    │
│                          ↓                                   │
│  ┌────────────────────────────────────────────────────┐    │
│  │   /wolf/zed-config/{instance_id}/settings.json     │    │
│  │   {                                                 │    │
│  │     "context_servers": {                           │    │
│  │       "helix-rag": {                               │    │
│  │         "command": "helix-cli",                    │    │
│  │         "args": ["mcp", "run", ...],               │    │
│  │         "env": {"HELIX_TOKEN": "..."}              │    │
│  │       },                                            │    │
│  │       "external-mcp": {                            │    │
│  │         "command": "node",                         │    │
│  │         "args": ["server.js"],                     │    │
│  │         "env": {...}                               │    │
│  │       }                                             │    │
│  │     }                                               │    │
│  │   }                                                 │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
                           ↓ (bind mount)
┌─────────────────────────────────────────────────────────────┐
│              Wolf Container (Zed Agent)                      │
│  ┌────────────────────────────────────────────────────┐    │
│  │   /home/retro/.config/zed/settings.json            │    │
│  │   (bind-mounted from host)                          │    │
│  └────────────────────────────────────────────────────┘    │
│                          ↓                                   │
│  ┌────────────────────────────────────────────────────┐    │
│  │            Zed Context Server Manager               │    │
│  │  - Loads context_servers from settings.json        │    │
│  │  - Spawns MCP server processes                      │    │
│  │  - Manages stdio/HTTP communication                 │    │
│  └────────────────────────────────────────────────────┘    │
│              ↓                          ↓                    │
│  ┌──────────────────┐      ┌──────────────────────────┐    │
│  │  helix-cli mcp   │      │  External MCP Server     │    │
│  │  (proxy mode)    │      │  (direct connection)     │    │
│  └──────────────────┘      └──────────────────────────┘    │
│          ↓                                                   │
│  ┌────────────────────────────────────────────────────┐    │
│  │         Zed AI Assistant (ACP Thread)               │    │
│  │  - Sees all tools as standard MCP tools             │    │
│  │  - Calls tools via MCP protocol                     │    │
│  │  - Displays results to user                         │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 Component Responsibilities

#### Helix API Server
- **GenerateZedMCPConfig()**: Convert Helix app config → Zed settings.json
- **Write config to filesystem**: `/wolf/zed-config/{instance_id}/settings.json`
- **OAuth token management**: Fetch and encrypt tokens for injection

#### helix-cli mcp run
- **MCP server process**: Implements MCP protocol
- **Tool routing**:
  - Native tools → Helix API calls (RAG, OpenAPI, etc.)
  - External MCP servers → HTTP/SSE proxy
- **Authentication**: Use HELIX_TOKEN env var for API calls
- **Token injection**: Add OAuth tokens to API tool headers

#### Zed (Minimal Changes)
- **Load settings.json**: Already implemented
- **Spawn context servers**: Already implemented
- **Use MCP tools**: Already implemented
- **New**: Read settings from custom path (env var override)

### 4.3 Data Flow

#### 1. Agent Start Flow
```
1. User creates external agent / PDE
   ↓
2. Wolf executor (wolf_executor.go:CreateExternalAgent)
   ↓
3. Call GenerateZedMCPConfig(app, user, session)
   ↓
4. Write /wolf/zed-config/{instance_id}/settings.json
   ↓
5. Add bind mount: {host_path}:/home/retro/.config/zed/settings.json:ro
   ↓
6. Add env var: HELIX_CLI_CONFIG_PATH=/home/retro/.config/zed/helix-cli.json
   ↓
7. Container starts, Zed reads settings.json
   ↓
8. Zed spawns context servers (helix-cli mcp run, external servers)
   ↓
9. MCP tools available in Zed AI assistant
```

#### 2. MCP Tool Execution Flow (Helix Native)
```
1. User asks Zed AI: "Search knowledge base for X"
   ↓
2. Zed AI chooses tool: helix-rag/search_knowledge
   ↓
3. Zed → helix-cli mcp run (stdio/JSON-RPC)
   ↓
4. helix-cli parses tool call, routes to Helix API
   ↓
5. POST /api/v1/knowledge/search
      Headers: Authorization: Bearer {HELIX_TOKEN}
      Body: {query: "X", app_id: "..."}
   ↓
6. Helix API executes RAG search
   ↓
7. Return knowledge chunks as MCP tool result
   ↓
8. helix-cli formats as MCP response
   ↓
9. Zed receives result, displays to user
```

#### 3. MCP Tool Execution Flow (OAuth API)
```
1. User asks Zed AI: "Create GitHub issue"
   ↓
2. Zed AI chooses tool: github-api/create_issue
   ↓
3. Zed → helix-cli mcp run
   ↓
4. helix-cli checks OAuth config for github-api
   ↓
5. Fetch OAuth token:
      GET /api/v1/oauth/token?provider=github&user_id={user_id}
   ↓
6. Inject token into API call headers
   ↓
7. Execute OpenAPI action with authenticated headers
   ↓
8. Return API response as MCP tool result
```

#### 4. MCP Tool Execution Flow (External MCP Server)
```
1. User asks Zed AI: "List files in current directory"
   ↓
2. Zed AI chooses tool: filesystem/list_directory
   ↓
3. Zed → External MCP Server (configured as context server)
   ↓
4. Direct communication (no helix-cli proxy)
   ↓
5. MCP server executes tool
   ↓
6. Return result to Zed
```

---

## 5. Detailed Design

### 5.1 Configuration Generation

**Location**: `/home/luke/pm/helix/api/pkg/external-agent/zed_config.go` (new file)

```go
package externalagent

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

// ZedMCPConfig represents Zed's MCP configuration format
type ZedMCPConfig struct {
    ContextServers map[string]ContextServerConfig `json:"context_servers"`
}

type ContextServerConfig struct {
    Command string            `json:"command"`
    Args    []string          `json:"args"`
    Env     map[string]string `json:"env,omitempty"`
}

// GenerateZedMCPConfig creates Zed MCP configuration from Helix app config
func GenerateZedMCPConfig(
    app *types.App,
    userID string,
    sessionID string,
    helixToken string,
) (*ZedMCPConfig, error) {
    config := &ZedMCPConfig{
        ContextServers: make(map[string]ContextServerConfig),
    }

    // Get primary assistant (first assistant or default)
    if len(app.Config.Helix.Assistants) == 0 {
        return config, nil // No assistants configured
    }
    assistant := app.Config.Helix.Assistants[0]

    // 1. Add Helix native tools as helix-cli MCP proxy
    if hasNativeTools(assistant) {
        config.ContextServers["helix-native"] = ContextServerConfig{
            Command: "helix-cli",
            Args: []string{
                "mcp", "run",
                "--app-id", app.ID,
                "--user-id", userID,
                "--session-id", sessionID,
            },
            Env: map[string]string{
                "HELIX_URL":   os.Getenv("HELIX_API_URL"),
                "HELIX_TOKEN": helixToken,
            },
        }
    }

    // 2. Pass-through external MCP servers
    for _, mcp := range assistant.MCPs {
        config.ContextServers[sanitizeName(mcp.Name)] = mcpToContextServer(mcp)
    }

    return config, nil
}

// hasNativeTools checks if assistant has Helix native tools
func hasNativeTools(assistant types.AssistantConfig) bool {
    return len(assistant.APIs) > 0 ||
           assistant.RAG.Enabled ||
           len(assistant.Knowledge) > 0 ||
           assistant.Browser.Enabled ||
           assistant.Calculator.Enabled ||
           assistant.Email.Enabled
}

// mcpToContextServer converts Helix MCP config to Zed context server config
func mcpToContextServer(mcp types.AssistantMCP) ContextServerConfig {
    // Parse MCP URL to determine connection type
    if strings.HasPrefix(mcp.URL, "http://") || strings.HasPrefix(mcp.URL, "https://") {
        // HTTP/SSE transport - use helix-cli as proxy
        return ContextServerConfig{
            Command: "helix-cli",
            Args: []string{
                "mcp", "proxy",
                "--url", mcp.URL,
                "--name", mcp.Name,
            },
            Env: buildMCPEnv(mcp),
        }
    }

    // Stdio transport - direct command execution
    // Parse command from URL (e.g., "stdio://npx @modelcontextprotocol/server-filesystem /tmp")
    cmd, args := parseStdioURL(mcp.URL)
    return ContextServerConfig{
        Command: cmd,
        Args:    args,
        Env:     buildMCPEnv(mcp),
    }
}

func buildMCPEnv(mcp types.AssistantMCP) map[string]string {
    env := make(map[string]string)
    for k, v := range mcp.Headers {
        env[fmt.Sprintf("MCP_HEADER_%s", strings.ToUpper(k))] = v
    }
    return env
}

// WriteZedMCPConfig writes configuration to filesystem for bind mounting
func WriteZedMCPConfig(instanceID string, config *ZedMCPConfig) (string, error) {
    configDir := filepath.Join("/opt/helix/wolf/zed-config", instanceID)
    if err := os.MkdirAll(configDir, 0755); err != nil {
        return "", fmt.Errorf("failed to create config dir: %w", err)
    }

    settingsPath := filepath.Join(configDir, "settings.json")

    // Read existing settings if present
    var settings map[string]interface{}
    if data, err := os.ReadFile(settingsPath); err == nil {
        json.Unmarshal(data, &settings)
    } else {
        settings = make(map[string]interface{})
    }

    // Merge MCP config into settings
    settings["context_servers"] = config.ContextServers

    data, err := json.MarshalIndent(settings, "", "  ")
    if err != nil {
        return "", fmt.Errorf("failed to marshal settings: %w", err)
    }

    if err := os.WriteFile(settingsPath, data, 0644); err != nil {
        return "", fmt.Errorf("failed to write settings: %w", err)
    }

    return settingsPath, nil
}
```

### 5.2 Helix CLI MCP Proxy Enhancement

**Location**: `/home/luke/pm/helix/api/pkg/cli/mcp/mcp_proxy.go` (existing file)

**Enhancements needed**:

```go
// Add new command: helix-cli mcp run
// This runs an MCP server that exposes Helix tools

type MCPRunOptions struct {
    AppID     string
    UserID    string
    SessionID string
}

func RunMCPServer(opts MCPRunOptions) error {
    // Initialize Helix API client
    client, err := client.NewClient(context.Background(), &client.ClientOptions{
        URL:   os.Getenv("HELIX_URL"),
        Token: os.Getenv("HELIX_TOKEN"),
    })
    if err != nil {
        return fmt.Errorf("failed to create client: %w", err)
    }

    // Fetch app configuration
    app, err := client.GetApp(context.Background(), opts.AppID)
    if err != nil {
        return fmt.Errorf("failed to get app: %w", err)
    }

    // Create MCP server with Helix tools
    server := &HelixMCPServer{
        client:    client,
        app:       app,
        userID:    opts.UserID,
        sessionID: opts.SessionID,
    }

    // Start MCP server on stdio
    return server.Serve()
}

type HelixMCPServer struct {
    client    client.Client
    app       *types.App
    userID    string
    sessionID string
}

func (s *HelixMCPServer) ListTools(ctx context.Context) ([]mcp.Tool, error) {
    var tools []mcp.Tool

    assistant := s.app.Config.Helix.Assistants[0]

    // Convert RAG/Knowledge to MCP tools
    if assistant.RAG.Enabled {
        tools = append(tools, mcp.Tool{
            Name:        "search_knowledge",
            Description: "Search the knowledge base",
            InputSchema: map[string]interface{}{
                "type": "object",
                "properties": map[string]interface{}{
                    "query": map[string]interface{}{
                        "type":        "string",
                        "description": "Search query",
                    },
                },
                "required": []string{"query"},
            },
        })
    }

    // Convert API integrations to MCP tools
    for _, api := range assistant.APIs {
        apiTools, err := s.convertAPIToMCPTools(api)
        if err != nil {
            log.Warn().Err(err).Str("api", api.Name).Msg("failed to convert API to MCP tools")
            continue
        }
        tools = append(tools, apiTools...)
    }

    // Add browser, calculator, email tools...
    if assistant.Browser.Enabled {
        tools = append(tools, s.browserTools()...)
    }

    return tools, nil
}

func (s *HelixMCPServer) CallTool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
    // Route based on tool name prefix
    parts := strings.SplitN(name, "/", 2)
    if len(parts) != 2 {
        return nil, fmt.Errorf("invalid tool name format: %s", name)
    }

    toolType, toolName := parts[0], parts[1]

    switch toolType {
    case "knowledge":
        return s.callKnowledgeTool(ctx, toolName, args)
    case "api":
        return s.callAPITool(ctx, toolName, args)
    case "browser":
        return s.callBrowserTool(ctx, toolName, args)
    default:
        return nil, fmt.Errorf("unknown tool type: %s", toolType)
    }
}

func (s *HelixMCPServer) callKnowledgeTool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
    query, ok := args["query"].(string)
    if !ok {
        return nil, fmt.Errorf("missing query parameter")
    }

    // Call Helix knowledge search API
    results, err := s.client.SearchKnowledge(ctx, &client.KnowledgeSearchRequest{
        AppID: s.app.ID,
        Query: query,
    })
    if err != nil {
        return nil, fmt.Errorf("knowledge search failed: %w", err)
    }

    // Format results as MCP tool result
    var content []mcp.Content
    for _, result := range results {
        content = append(content, mcp.Content{
            Type: "text",
            Text: result.Content,
        })
    }

    return &mcp.CallToolResult{
        Content: content,
    }, nil
}

func (s *HelixMCPServer) callAPITool(ctx context.Context, name string, args map[string]interface{}) (*mcp.CallToolResult, error) {
    // Find API configuration
    var apiConfig *types.AssistantAPI
    for i := range s.app.Config.Helix.Assistants[0].APIs {
        if s.app.Config.Helix.Assistants[0].APIs[i].Name == name {
            apiConfig = &s.app.Config.Helix.Assistants[0].APIs[i]
            break
        }
    }
    if apiConfig == nil {
        return nil, fmt.Errorf("API config not found: %s", name)
    }

    // Fetch OAuth token if needed
    var headers map[string]string
    if apiConfig.OAuthProvider != "" {
        token, err := s.fetchOAuthToken(ctx, apiConfig.OAuthProvider)
        if err != nil {
            return nil, fmt.Errorf("failed to fetch OAuth token: %w", err)
        }
        headers = map[string]string{
            "Authorization": fmt.Sprintf("Bearer %s", token),
        }
    }

    // Execute API action via Helix API
    result, err := s.client.RunAPIAction(ctx, &client.RunAPIActionRequest{
        AppID:   s.app.ID,
        APIName: name,
        Args:    args,
        Headers: headers,
    })
    if err != nil {
        return nil, fmt.Errorf("API action failed: %w", err)
    }

    return &mcp.CallToolResult{
        Content: []mcp.Content{{
            Type: "text",
            Text: result.Response,
        }},
    }, nil
}

func (s *HelixMCPServer) fetchOAuthToken(ctx context.Context, provider string) (string, error) {
    // Call Helix OAuth API to get token for user
    resp, err := s.client.GetOAuthToken(ctx, &client.GetOAuthTokenRequest{
        Provider: provider,
        UserID:   s.userID,
    })
    if err != nil {
        return "", err
    }
    return resp.AccessToken, nil
}
```

### 5.3 Wolf Executor Integration

**Location**: `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor.go`

**Modifications to createSwayWolfApp()**:

```go
func (w *WolfExecutor) createSwayWolfApp(config SwayWolfAppConfig) *wolf.App {
    // ... existing code ...

    // NEW: Generate Zed MCP configuration
    if config.App != nil {
        mcpConfig, err := GenerateZedMCPConfig(
            config.App,
            config.UserID,
            config.SessionID,
            w.helixAPIToken,
        )
        if err != nil {
            log.Error().Err(err).Msg("Failed to generate Zed MCP config")
        } else {
            settingsPath, err := WriteZedMCPConfig(config.InstanceID, mcpConfig)
            if err != nil {
                log.Error().Err(err).Msg("Failed to write Zed MCP config")
            } else {
                // Add bind mount for Zed settings
                mounts = append(mounts,
                    fmt.Sprintf("%s:/home/retro/.config/zed/settings.json:ro", settingsPath),
                )
                log.Info().
                    Str("instance_id", config.InstanceID).
                    Str("settings_path", settingsPath).
                    Msg("Mounted Zed MCP configuration")
            }
        }
    }

    // ... rest of existing code ...
}
```

**Update SwayWolfAppConfig struct**:

```go
type SwayWolfAppConfig struct {
    WolfAppID         string
    Title             string
    ContainerHostname string
    UserID            string
    SessionID         string
    InstanceID        string  // NEW: For config file path
    App               *types.App  // NEW: For MCP config generation
    WorkspaceDir      string
    ExtraEnv          []string
    DisplayWidth      int
    DisplayHeight     int
    DisplayFPS        int
}
```

### 5.4 Tool Name Sanitization

**Pattern**: Ensure tool names are valid MCP identifiers

```go
// sanitizeName converts Helix tool names to valid MCP tool names
func sanitizeName(name string) string {
    // MCP tool names: alphanumeric, hyphens, underscores only
    name = strings.ToLower(name)
    name = regexp.MustCompile(`[^a-z0-9-_]`).ReplaceAllString(name, "-")
    name = strings.Trim(name, "-")
    return name
}

// namespaceToolName adds prefix to avoid collisions
func namespaceToolName(toolType, toolName string) string {
    return fmt.Sprintf("%s/%s", sanitizeName(toolType), sanitizeName(toolName))
}
```

### 5.5 OAuth Token Flow

```go
// GetOAuthToken fetches OAuth token for user and provider
func (c *Client) GetOAuthToken(ctx context.Context, req *GetOAuthTokenRequest) (*OAuthToken, error) {
    // Call Helix OAuth API
    resp, err := c.get(ctx, fmt.Sprintf("/api/v1/oauth/tokens/%s/%s", req.Provider, req.UserID))
    if err != nil {
        return nil, err
    }

    var token OAuthToken
    if err := json.Unmarshal(resp, &token); err != nil {
        return nil, err
    }

    return &token, nil
}

// API handler (existing, may need modification)
func (s *HelixAPIServer) getOAuthToken(res http.ResponseWriter, req *http.Request) {
    user := getRequestUser(req)
    provider := mux.Vars(req)["provider"]

    // Verify user owns this OAuth connection
    conn, err := s.Store.GetOAuthConnectionByUserAndProvider(req.Context(), user.ID, provider)
    if err != nil {
        system.Error(res, req, http.StatusNotFound, "OAuth connection not found")
        return
    }

    // Refresh token if expired
    if conn.ExpiresAt.Before(time.Now()) {
        // ... refresh logic ...
    }

    system.RespondJSON(res, http.StatusOK, map[string]string{
        "access_token": conn.AccessToken,
        "token_type":   "Bearer",
    })
}
```

---

## 6. Implementation Plan

### Phase 1: Core Infrastructure (Week 1)
**Goal**: Basic MCP config generation and mounting

- [ ] Create `zed_config.go` with `GenerateZedMCPConfig()`
- [ ] Implement config writing to `/opt/helix/wolf/zed-config/{instance_id}/`
- [ ] Update `wolf_executor.go` to call config generation
- [ ] Add bind mount for settings.json
- [ ] Test with simple external MCP server (filesystem)

**Deliverable**: External MCP servers passed through to Zed

### Phase 2: Helix CLI MCP Proxy (Week 2)
**Goal**: Helix native tools accessible via MCP

- [ ] Enhance `helix-cli mcp run` command
- [ ] Implement `HelixMCPServer.ListTools()` for RAG/API tools
- [ ] Implement `HelixMCPServer.CallTool()` routing
- [ ] Add RAG/Knowledge search tool
- [ ] Add Browser tool
- [ ] Test end-to-end: Helix → Zed → helix-cli → Helix API

**Deliverable**: RAG and Browser tools work in Zed

### Phase 3: OAuth Integration (Week 3)
**Goal**: API integrations with OAuth work in Zed

- [ ] Implement OAuth token fetch in helix-cli
- [ ] Add token injection to API tool calls
- [ ] Handle token refresh/expiry
- [ ] Test with GitHub/Google OAuth APIs
- [ ] Add audit logging for OAuth usage

**Deliverable**: OAuth-protected APIs callable from Zed

### Phase 4: Advanced Tools (Week 4)
**Goal**: Full tool parity

- [ ] Add Calculator, Email, WebSearch tools to MCP proxy
- [ ] Add Vision RAG tool
- [ ] Add Zapier integration tools
- [ ] Support MCP stdio transport (in addition to HTTP/SSE)
- [ ] Add tool execution monitoring/metrics

**Deliverable**: All Helix tools available in Zed

### Phase 5: Polish & Optimization (Week 5)
**Goal**: Production-ready

- [ ] Hot-reload support (update config without restart)
- [ ] Error handling and graceful degradation
- [ ] Performance optimization (caching, connection pooling)
- [ ] Comprehensive testing (unit, integration, E2E)
- [ ] Documentation and examples

**Deliverable**: Production-ready feature

---

## 7. Security Considerations

### 7.1 OAuth Token Security

**Threat**: Token leakage or misuse

**Mitigations**:
1. **Encrypted storage**: Tokens encrypted at rest in database
2. **TLS in transit**: API calls over HTTPS only
3. **Scoped tokens**: Request minimum scopes needed
4. **Token isolation**: Per-user tokens, never cross-contaminate
5. **Audit logging**: Log all OAuth token usage
6. **Rotation**: Support token refresh, expire old tokens
7. **No logging**: Never log tokens in error messages/debug logs

### 7.2 User Isolation

**Threat**: User A accessing User B's tools/data

**Mitigations**:
1. **Config isolation**: Separate config dirs per instance
2. **Token validation**: Verify HELIX_TOKEN matches user_id in API calls
3. **Bind mount isolation**: Read-only mounts, user-specific paths
4. **API authorization**: Helix API enforces user ownership

### 7.3 MCP Server Security

**Threat**: Malicious MCP server exploiting Zed

**Mitigations**:
1. **Sandboxing**: MCP servers run in isolated processes
2. **Resource limits**: CPU/memory limits on MCP servers
3. **Timeout protection**: Kill hung MCP servers
4. **Input validation**: Sanitize all MCP tool inputs
5. **Admin approval**: Require admin approval for adding MCP servers (future)

### 7.4 Configuration Security

**Threat**: Config injection attacks

**Mitigations**:
1. **JSON schema validation**: Validate all config before writing
2. **Path traversal protection**: Sanitize file paths
3. **Command injection prevention**: Escape all shell commands
4. **Read-only mounts**: Settings mounted read-only in containers

---

## 8. Testing Strategy

### 8.1 Unit Tests

**Config Generation** (`zed_config_test.go`):
```go
func TestGenerateZedMCPConfig(t *testing.T) {
    app := &types.App{
        Config: types.AppConfig{
            Helix: types.AppHelixConfig{
                Assistants: []types.AssistantConfig{{
                    MCPs: []types.AssistantMCP{{
                        Name: "filesystem",
                        URL:  "stdio://npx @modelcontextprotocol/server-filesystem /tmp",
                    }},
                    RAG: types.AssistantRAG{Enabled: true},
                }},
            },
        },
    }

    config, err := GenerateZedMCPConfig(app, "user-123", "session-456", "token-789")
    assert.NoError(t, err)
    assert.Len(t, config.ContextServers, 2) // filesystem + helix-native

    // Verify filesystem MCP server
    fs := config.ContextServers["filesystem"]
    assert.Equal(t, "npx", fs.Command)
    assert.Equal(t, []string{"@modelcontextprotocol/server-filesystem", "/tmp"}, fs.Args)

    // Verify helix-native MCP server
    hn := config.ContextServers["helix-native"]
    assert.Equal(t, "helix-cli", hn.Command)
    assert.Contains(t, hn.Args, "--app-id")
    assert.Equal(t, "token-789", hn.Env["HELIX_TOKEN"])
}
```

**MCP Proxy** (`mcp_proxy_test.go`):
```go
func TestHelixMCPServerListTools(t *testing.T) {
    // Mock Helix client
    client := &MockClient{
        App: &types.App{/* with RAG, APIs, etc. */},
    }

    server := &HelixMCPServer{client: client}
    tools, err := server.ListTools(context.Background())

    assert.NoError(t, err)
    assert.Contains(t, toolNames(tools), "knowledge/search_knowledge")
    assert.Contains(t, toolNames(tools), "api/github-create-issue")
}

func TestHelixMCPServerCallKnowledgeTool(t *testing.T) {
    client := &MockClient{
        SearchResults: []*types.KnowledgeResult{{Content: "Answer"}},
    }

    server := &HelixMCPServer{client: client}
    result, err := server.CallTool(context.Background(), "knowledge/search_knowledge", map[string]interface{}{
        "query": "test query",
    })

    assert.NoError(t, err)
    assert.Len(t, result.Content, 1)
    assert.Equal(t, "Answer", result.Content[0].Text)
}
```

### 8.2 Integration Tests

**E2E Flow** (`integration_test.go`):
```go
func TestZedMCPIntegration(t *testing.T) {
    // 1. Create Helix app with MCP config
    app := createTestApp(t, &types.AssistantConfig{
        RAG: types.AssistantRAG{Enabled: true},
        MCPs: []types.AssistantMCP{{
            Name: "test-mcp",
            URL:  testMCPServerURL,
        }},
    })

    // 2. Start external agent
    agent := startTestAgent(t, app)
    defer agent.Stop()

    // 3. Verify Zed config generated
    settingsPath := fmt.Sprintf("/opt/helix/wolf/zed-config/%s/settings.json", agent.InstanceID)
    assert.FileExists(t, settingsPath)

    var settings map[string]interface{}
    data, _ := os.ReadFile(settingsPath)
    json.Unmarshal(data, &settings)

    servers := settings["context_servers"].(map[string]interface{})
    assert.Contains(t, servers, "helix-native")
    assert.Contains(t, servers, "test-mcp")

    // 4. Call MCP tool via Zed
    result := agent.CallZedTool(t, "knowledge/search_knowledge", map[string]interface{}{
        "query": "test",
    })
    assert.NotEmpty(t, result)
}
```

### 8.3 Manual Testing Checklist

- [ ] External MCP server (filesystem) accessible in Zed
- [ ] Helix RAG search works from Zed
- [ ] GitHub OAuth API works from Zed
- [ ] Multiple MCP servers work simultaneously
- [ ] Config hot-reload works
- [ ] Error handling graceful (MCP server crash doesn't break Zed)
- [ ] Performance acceptable (< 500ms overhead)

---

## 9. Rollout Plan

### 9.1 Feature Flag

```go
// In config
type FeatureFlags struct {
    ZedMCPSync bool `json:"zed_mcp_sync"`
}

// In wolf_executor.go
if w.featureFlags.ZedMCPSync {
    // Generate and mount MCP config
}
```

### 9.2 Gradual Rollout

**Week 1**: Internal testing
- Enable for development environment only
- Manual testing by team

**Week 2**: Beta users
- Enable for opted-in beta users
- Monitor logs/errors
- Gather feedback

**Week 3**: 50% rollout
- Enable for 50% of users (randomized)
- Monitor performance metrics
- A/B test engagement

**Week 4**: 100% rollout
- Enable for all users
- Full monitoring
- Support documentation

### 9.3 Monitoring

**Metrics to track**:
- MCP config generation time (p50, p95, p99)
- MCP tool call latency
- MCP tool success/failure rate
- OAuth token fetch time
- Number of MCP servers per agent

**Alerts**:
- MCP config generation failures > 5%
- MCP tool call latency > 1s
- OAuth token fetch failures > 10%

### 9.4 Rollback Plan

**Criteria for rollback**:
- Config generation failures > 20%
- Agent start failures > 10%
- User-reported issues > 50/day

**Rollback process**:
1. Disable feature flag
2. Restart affected agents
3. Investigate root cause
4. Fix and redeploy

---

## 10. Future Enhancements

### 10.1 Hot-Reload
- Watch Helix app config for changes
- Regenerate Zed config on change
- Signal Zed to reload context servers

### 10.2 MCP Server Marketplace
- Curated list of MCP servers
- One-click install from Helix UI
- Automatic configuration

### 10.3 Custom MCP Development
- Helix SDK for building MCP servers
- Deploy custom MCP servers to Helix cloud
- Share MCP servers with team

### 10.4 Advanced OAuth
- Support OAuth 2.0 PKCE flow
- Hardware security key support
- SSO integration

---

## 11. Success Metrics

### 11.1 Adoption Metrics
- **Target**: 80% of external agents use MCP tools within 1 month
- **Measure**: % of agents with MCP config generated

### 11.2 Functionality Metrics
- **Target**: 95% MCP tool success rate
- **Measure**: Successful tool calls / total tool calls

### 11.3 Performance Metrics
- **Target**: < 200ms config generation time (p95)
- **Target**: < 500ms MCP tool call overhead (p95)
- **Measure**: Latency telemetry

### 11.4 User Satisfaction
- **Target**: 4.5/5 user rating for MCP features
- **Measure**: User surveys, NPS

---

## 12. Appendix

### A. MCP Protocol Reference
- [Model Context Protocol Spec](https://spec.modelcontextprotocol.io/)
- [MCP Go Library](https://github.com/mark3labs/mcp-go)

### B. Existing Helix Patterns
- Tool configuration: `/home/luke/pm/helix/api/pkg/types/types.go:1276-1463`
- MCP client: `/home/luke/pm/helix/api/pkg/agent/skill/mcp/`
- MCP proxy: `/home/luke/pm/helix/api/pkg/cli/mcp/mcp_proxy.go`

### C. Zed Integration
- Context servers: `/home/luke/pm/zed/crates/project/src/context_server_store.rs`
- MCP manager: `/home/luke/pm/zed/crates/external_websocket_sync/src/mcp.rs`
- Settings: `/home/luke/pm/zed/crates/settings/src/settings_content/`
