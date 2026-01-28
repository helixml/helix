# SSE MCP Integration Test

**Date:** 2026-01-28
**Status:** In Progress
**Branch (Helix):** `feat/sse-mcp-integration-test`
**Branch (Zed):** `feat/legacy-sse`

## Overview

Test the Zed SSE MCP transport implementation end-to-end through Helix infrastructure. The SSE transport implements the legacy MCP HTTP+SSE protocol (2024-11-05 spec) which is still used by enterprise MCP servers like Atlassian.

## Test Strategy

A "secret server" MCP provides a `get_secret` tool that returns a hard-coded secret value (`HELIX-SSE-MCP-SECRET-7f3a9b2c`). The test:

1. Starts the SSE secret server as a Docker container
2. Creates a Helix agent configured to use that MCP server
3. Creates a spec task and asks "What is the secret?"
4. Asserts the agent's response contains `HELIX-SSE-MCP-SECRET-7f3a9b2c`

If the secret appears in the response, we know the entire SSE MCP flow worked:
- Zed connected to the SSE endpoint
- Received the `endpoint` event
- Called `tools/list` and discovered `get_secret`
- Called `tools/call` with `get_secret`
- Received the response via SSE `message` event
- Incorporated the result into the agent's response

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│ Host                                                                │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Docker Compose Network                                        │  │
│  │                                                               │  │
│  │  ┌─────────────┐    ┌─────────────┐    ┌─────────────────┐   │  │
│  │  │   Helix     │    │  SSE MCP    │    │    Sandbox      │   │  │
│  │  │    API      │    │  Test       │    │   Container     │   │  │
│  │  │             │    │  Server     │    │                 │   │  │
│  │  │             │    │             │    │  ┌───────────┐  │   │  │
│  │  │  Configures │    │  Provides   │    │  │    Zed    │  │   │  │
│  │  │  agent with ├───►│  "echo"     │◄───┤  │  (Agent)  │  │   │  │
│  │  │  MCP server │    │  tool via   │    │  │           │  │   │  │
│  │  │             │    │  SSE        │    │  └───────────┘  │   │  │
│  │  └─────────────┘    └─────────────┘    └─────────────────┘   │  │
│  │        │                   ▲                    ▲            │  │
│  │        │                   │                    │            │  │
│  │        └───────────────────┴────────────────────┘            │  │
│  │              Docker internal network                         │  │
│  │              (sse-mcp-test:3333)                             │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ┌─────────────────┐                                               │
│  │  Test Script    │  Uses Helix CLI to:                           │
│  │                 │  1. Start SSE server container                │
│  │                 │  2. Create agent with MCP config              │
│  │                 │  3. Create spec task                          │
│  │                 │  4. Send prompt requiring MCP tool            │
│  │                 │  5. Assert response contains expected data    │
│  └─────────────────┘                                               │
└─────────────────────────────────────────────────────────────────────┘
```

## Components

### 1. SSE MCP Secret Server (Python)

Location: `zed/script/test_sse_mcp_server.py`

A minimal Python HTTP server implementing the MCP 2024-11-05 SSE protocol:
- GET `/sse` - SSE endpoint, sends `endpoint` event with POST URL
- POST `/message` - Receives JSON-RPC requests, sends responses via SSE
- GET `/health` - Health check endpoint
- GET `/secret` - Debug endpoint returning the secret directly

Tools provided:
- `get_secret` - Returns `"The secret is: HELIX-SSE-MCP-SECRET-7f3a9b2c"`

The secret value is hard-coded in the Python file as `SECRET_VALUE`.

### 2. Docker Compose Service

Add to `docker-compose.dev.yaml`:
```yaml
sse-mcp-secret:
  image: python:3.11-slim
  command: python /app/test_sse_mcp_server.py 3333
  volumes:
    - ../zed/script/test_sse_mcp_server.py:/app/test_sse_mcp_server.py:ro
  ports:
    - "3333:3333"
  healthcheck:
    test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:3333/health')"]
    interval: 5s
    timeout: 3s
    retries: 3
```

The service is accessible from:
- Host: `http://localhost:3333/sse`
- Other containers: `http://sse-mcp-secret:3333/sse`

### 3. Helix Agent Configuration

The agent is configured via the Helix API with an MCP server pointing to the SSE secret server. The `AssistantMCP` type supports HTTP URLs which get passed through to Zed's context_servers config:

```json
{
  "name": "SSE MCP Test Agent",
  "config": {
    "helix": {
      "assistants": [{
        "name": "default",
        "model": "meta-llama/Llama-3.3-70B-Instruct",
        "system_prompt": "You have access to MCP tools. When asked about secrets, use the get_secret tool.",
        "mcps": [{
          "name": "secret-server",
          "description": "SSE MCP server that provides secrets",
          "url": "http://sse-mcp-secret:3333/sse"
        }]
      }]
    }
  }
}
```

The `url` field with `http://` prefix triggers HTTP transport in `mcpToContextServer()`. Zed's transport selection then determines whether to use Streamable HTTP or legacy SSE based on the endpoint behavior.

**Note:** Zed currently auto-detects transport. For explicit SSE, we may need to add a `transport` field to `AssistantMCP` and propagate it through `ContextServerConfig`.

### 4. Test Script

Location: `helix/scripts/test-zed-sse-mcp.sh`

```bash
#!/bin/bash
set -e

SECRET="HELIX-SSE-MCP-SECRET-7f3a9b2c"

# 1. Verify SSE server is running
curl -sf http://localhost:3333/health || { echo "SSE server not running"; exit 1; }

# 2. Create test agent with MCP config (or use existing)
AGENT_ID=$(helix app create --name "sse-mcp-test" --config '...' | jq -r '.id')

# 3. Start spec task
SESSION_ID=$(helix spectask start --agent $AGENT_ID --project $PROJECT_ID -n "SSE MCP Test" | jq -r '.session_id')

# 4. Wait for session to be ready
sleep 30

# 5. Send prompt asking for the secret
helix spectask send $SESSION_ID "What is the secret? Use the get_secret tool to find out." --wait

# 6. Get the response and check for secret
RESPONSE=$(helix spectask interact $SESSION_ID --history | tail -1)
if echo "$RESPONSE" | grep -q "$SECRET"; then
    echo "✓ SUCCESS: Agent learned the secret via SSE MCP"
else
    echo "✗ FAILURE: Secret not found in response"
    exit 1
fi

# 7. Cleanup
helix spectask stop $SESSION_ID
```

## CLI Commands Added

```bash
# Execute command in session container
helix spectask exec <session-id> <command> [args...]

# Copy file into session container
helix spectask copy <session-id> <local-file> [--dest <path>]
```

## Files

| File | Description |
|------|-------------|
| `zed/script/test_sse_mcp_server.py` | Python SSE MCP test server |
| `zed/crates/context_server/src/transport/sse.rs` | Zed SSE transport implementation |
| `helix/api/pkg/cli/spectask/exec_cmd.go` | CLI exec command |
| `helix/api/pkg/cli/spectask/copy_cmd.go` | CLI copy command |
| `helix/scripts/test-zed-sse-mcp.sh` | Integration test script |
| `helix/design/2026-01-28-sse-mcp-integration-test.md` | This document |

## Transport Detection

Currently, Zed auto-detects transport based on server behavior:
- Streamable HTTP (2025-03-26): Server accepts POST with `Accept: application/json, text/event-stream`
- Legacy SSE (2024-11-05): Server returns `text/event-stream` on GET with `endpoint` event

The secret server implements the legacy SSE protocol, so Zed should auto-detect it. If auto-detection fails, we may need to:
1. Add explicit `transport: "sse"` field to `AssistantMCP` type
2. Propagate through `ContextServerConfig` 
3. Update Zed settings sync to include transport type

## Open Questions

1. Does Zed's auto-detection work reliably, or do we need explicit transport config?
2. How does the sandbox container resolve `sse-mcp-secret` hostname? (Docker network)
3. Should we add this as a CI integration test?

## Next Steps

1. [x] Simplify test server to just `get_secret` tool
2. [ ] Add SSE server to docker-compose.dev.yaml
3. [ ] Create test agent via API with MCP config
4. [ ] Write integration test script
5. [ ] Test locally with real Helix session
6. [ ] Verify secret appears in agent response