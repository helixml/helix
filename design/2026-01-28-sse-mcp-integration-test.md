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

**Key insight:** The Helix API proxies MCP connections. Zed doesn't connect directly to MCP servers - it connects to the API's MCP proxy endpoint, which forwards requests to the actual MCP server. This means:
- Only the API needs network access to the MCP server
- The desktop container (inside sandbox) doesn't need to reach external MCP servers
- Both API and SSE test server are on `helix_default` network, so hostname resolution works

```
┌─────────────────────────────────────────────────────────────────────┐
│ Host                                                                │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │ Docker Compose Network (helix_default)                        │  │
│  │                                                               │  │
│  │  ┌─────────────┐         ┌─────────────┐                     │  │
│  │  │   Helix     │  HTTP   │  SSE MCP    │                     │  │
│  │  │    API      │────────►│  Test       │                     │  │
│  │  │             │   SSE   │  Server     │                     │  │
│  │  │  MCP Proxy  │◄────────│ (get_secret)│                     │  │
│  │  │  Endpoint   │         │             │                     │  │
│  │  └──────▲──────┘         └─────────────┘                     │  │
│  │         │                sse-mcp-test:3333                   │  │
│  │         │ WebSocket                                          │  │
│  │         │ (MCP over WS)                                      │  │
│  │  ┌──────┴────────────────────────────────────────────────┐   │  │
│  │  │    Sandbox Container (DinD)                            │   │  │
│  │  │                                                        │   │  │
│  │  │  ┌────────────────────────────────────────────────┐   │   │  │
│  │  │  │  Desktop Container (helix-ubuntu)               │   │   │  │
│  │  │  │                                                 │   │   │  │
│  │  │  │  ┌─────────────┐      ┌─────────────────────┐  │   │   │  │
│  │  │  │  │    Zed      │      │    Qwen Code        │  │   │   │  │
│  │  │  │  │   Editor    │◄────►│    Agent            │  │   │   │  │
│  │  │  │  │             │ ACP  │                     │  │   │   │  │
│  │  │  │  │  context_   │      │  Uses MCP tools     │  │   │   │  │
│  │  │  │  │  servers    │      │  via Zed            │  │   │   │  │
│  │  │  │  └─────────────┘      └─────────────────────┘  │   │   │  │
│  │  │  └────────────────────────────────────────────────┘   │   │  │
│  │  └───────────────────────────────────────────────────────┘   │  │
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

### MCP Connection Flow

1. Agent config specifies MCP server URL: `http://sse-mcp-test:3333/sse`
2. Helix API receives agent config when starting session
3. API generates `settings.json` for Zed with MCP proxy URL pointing back to API
4. Zed's `context_servers` connects to API's MCP proxy endpoint via WebSocket
5. API's MCP proxy connects to actual SSE server (`sse-mcp-test:3333`)
6. MCP messages flow: Zed ↔ API Proxy ↔ SSE Server

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

### 2. Test SSE Server Container

The test script starts the SSE server as a standalone container (not in docker-compose):

```bash
docker run -d --name sse-mcp-test --network helix_default -p 3333:3333 \
    -v "$DIR/../../zed/script/test_sse_mcp_server.py:/app/server.py:ro" \
    python:3.11-slim python /app/server.py 3333
```

The service is accessible from:
- Host: `http://localhost:3333/sse` (for debugging)
- Helix API: `http://sse-mcp-test:3333/sse` (Docker DNS on helix_default)

**Important:** The container name `sse-mcp-test` must match the hostname in the agent config URL.

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
2. [x] Create test agent YAML with MCP config
3. [x] Write integration test script with video capture
4. [ ] Test locally with real Helix session
5. [ ] Verify secret appears in agent response
6. [ ] Debug SSE transport if needed (check Zed logs, API proxy logs)

## Debugging

### Check SSE server is receiving connections
```bash
docker logs sse-mcp-test
```

### Check API MCP proxy logs
```bash
docker compose logs --tail 50 api 2>&1 | grep -i mcp
```

### Check Zed logs inside desktop container
```bash
# Find desktop container
docker compose exec -T sandbox-nvidia docker ps --format "{{.Names}}" | grep ubuntu

# View Zed logs
docker compose exec -T sandbox-nvidia docker exec <container> cat ~/.local/share/zed/logs/Zed.log | grep -i "sse\|mcp\|context"
```

### Video capture for debugging
The test script captures video to `/tmp/sse-mcp-test/`. Convert to playable format:
```bash
ffmpeg -i /tmp/sse-mcp-test/sse-mcp-test-*.h264 -c copy output.mp4
```