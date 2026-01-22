# Roo Code + Cursor Agent Integration

> **IMPORTANT: READ THIS FIRST AFTER CONTEXT COMPACTION**
>
> **Worktree:** `/prod/home/luke/pm/helix.2` (NOT `/prod/home/luke/pm/helix`)
> **Branch:** `feat/roo-code-agent`
> **Date:** 2026-01-22
> **Status:** In Progress - Adding Cursor support

## Overview

Add alternative code agents alongside Zed:
1. **Zed Agent** - Zed's built-in agent panel
2. **Qwen Code** - Qwen Code agent in Zed via ACP
3. **VS Code + Roo Code** - VS Code with Roo Code extension
4. **Cursor** - Cursor IDE with built-in AI agent (in progress)

This allows users to choose their preferred IDE + agent combination for desktop coding sessions.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│ Helix API                                                           │
│  └─ websocket_external_agent_sync.go                                │
│      └─ Sends ExternalAgentCommand (chat_message, etc.)             │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ WebSocket (/api/v1/external-agents/sync)
                               ▼
┌─────────────────────────────────────────────────────────────────────┐
│ Desktop Container                                                   │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │ desktop-bridge (existing, extended)                             ││
│  │  api/cmd/desktop-bridge/main.go                                 ││
│  │  api/pkg/desktop/agent_client.go (new)                          ││
│  │  api/pkg/desktop/roocode.go (new)                               ││
│  │                                                                 ││
│  │  When HELIX_AGENT_HOST_TYPE=vscode:                             ││
│  │  - AgentClient: WS client to Helix API (receives commands)      ││
│  │  - RooCodeBridge: Socket.IO SERVER (Roo Code connects to us)    ││
│  │  - Serves /api/extension/bridge/config for Roo Code             ││
│  └─────────────────────────────┬───────────────────────────────────┘│
│                                │ Socket.IO SERVER (localhost:9879)  │
│                                │ (Roo Code connects TO this server) │
│                                ▼                                    │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │ VS Code + Roo Code Extension                                    ││
│  │  - Pre-installed in container                                   ││
│  │  - ROO_CODE_API_URL=http://localhost:9879 (our bridge)          ││
│  │  - Extension fetches config, then connects via Socket.IO        ││
│  └─────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────┘
```

**Key insight:** Roo Code extension is a Socket.IO CLIENT that connects OUTBOUND.
We run a Socket.IO SERVER that it connects to. The `ROO_CODE_API_URL` env var
tells the extension where to fetch the bridge config (which returns our local URL).

## Key Design Decisions

### 1. AgentHostType vs CodeAgentRuntime

Current `CodeAgentRuntime` is Zed-specific (which agent inside Zed). We need a higher-level concept:

```go
// AgentHostType specifies which code editor to use in the desktop container
type AgentHostType string

const (
    AgentHostTypeZed    AgentHostType = "zed"     // Zed editor (default)
    AgentHostTypeVSCode AgentHostType = "vscode"  // VS Code with Roo Code
)
```

When `AgentHostType == "vscode"`:
- Start VS Code instead of Zed
- Start roocode-bridge daemon
- Ignore CodeAgentRuntime (that's Zed-specific)

### 2. Command Translation

| Helix Command | Roo Code Equivalent |
|---------------|---------------------|
| `chat_message` (new) | `ExtensionBridgeCommand.StartTask` |
| `chat_message` (existing thread) | `TaskBridgeCommand.Message` |
| N/A | `TaskBridgeCommand.ApproveAsk` (auto-approve) |
| N/A | `TaskBridgeCommand.DenyAsk` |

### 3. Event Translation

| Roo Code Event | Helix Event |
|----------------|-------------|
| `TaskCreated` + `TaskInteractive` | `agent_ready` |
| `TaskBridgeEvent.Message` | `message_added` |
| `TaskCompleted` | `message_completed` |
| `TaskAborted` | `message_completed` (error) |

### 4. Auto-Approval Mode

Roo Code has an "ask" system for tool approvals. For headless/automated use, we'll:
- Configure Roo Code in auto-approve mode if available
- Or have roocode-bridge auto-respond to ApproveAsk for all tool calls

### 5. Editor Switching (Container Startup)

The container startup determines which editor to launch based on `HELIX_AGENT_HOST_TYPE`:

```bash
# In startup-app.sh
case "${HELIX_AGENT_HOST_TYPE:-zed}" in
  vscode)
    # Launch VS Code with Roo Code extension
    code --disable-workspace-trust /home/retro/work &
    ;;
  zed|*)
    # Launch Zed (existing behavior)
    zed /home/retro/work &
    ;;
esac
```

### 6. Settings Sync Strategy

**Current state:** `settings-sync-daemon` syncs Zed-specific settings (themes, keybindings).

**VS Code approach (phased):**

**Phase 1 (Initial):** Pre-configure in Docker image
- Install VS Code + Roo Code extension during image build
- Pre-configure settings.json with sane defaults
- Roo Code API configuration via env vars at runtime:
  - `ANTHROPIC_API_KEY` / `OPENAI_API_KEY` (already passed)
  - `ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL` (already passed)

**Phase 2 (Later):** Extend settings-sync-daemon
- Add VS Code settings format support
- Sync user preferences (theme, font size) to both Zed and VS Code
- Abstract settings into editor-agnostic format

**VS Code settings locations:**
```
~/.config/Code/User/settings.json    # VS Code settings
~/.vscode/extensions/               # Extensions (pre-installed)
~/.config/Code/User/globalStorage/  # Roo Code extension state
```

**Roo Code API configuration (settings.json):**
```json
{
  "roo-code.apiProvider": "openai-compatible",
  "roo-code.openAiBaseUrl": "${env:OPENAI_BASE_URL}",
  "roo-code.openAiApiKey": "${env:OPENAI_API_KEY}",
  "roo-code.openAiModelId": "${env:OPENAI_MODEL}"
}
```

Note: VS Code supports `${env:VAR}` syntax in settings.json for environment variable substitution.

## Implementation Plan

### Phase 1: Types and Configuration
- [x] Create design doc
- [x] Add `AgentHostType` to types (task_management.go)
- [x] Add `AgentHostType` to ExternalAgentConfig (types.go)
- [x] Add `HELIX_AGENT_HOST_TYPE` env var in hydra_executor.go

### Phase 2: Desktop Bridge Extension
- [x] Add Socket.IO server dependency to go.mod (note: server not client - we are the server)
- [x] Create `api/pkg/desktop/roocode.go` with RooCodeBridge (Socket.IO server)
- [x] Implement command translation (Helix → Roo Code)
- [x] Implement event translation (Roo Code → Helix)
- [x] Create `api/pkg/desktop/agent_client.go` for Helix API WebSocket client
- [x] Update `api/cmd/desktop-bridge/main.go` to start AgentClient when HELIX_AGENT_HOST_TYPE=vscode

### Phase 3: Container Integration
- [x] Add VS Code installation to helix-ubuntu Dockerfile
- [x] Add Roo Code extension pre-installation
- [x] Create default VS Code settings.json (inline in Dockerfile)
- [x] Update startup-app.sh for editor selection (zed vs code vs headless)

### Phase 4: Testing & Refinement
- [ ] Build and test Zed mode still works (regression)
- [ ] Build and test VS Code + Roo Code mode
- [ ] Add auto-approve handling for Roo Code asks (implemented in roocode.go)
- [ ] Add frontend support for editor selection (optional)

## Licensing

Roo Code extension is Apache 2.0 licensed. We are:
- Using the open-source extension (permitted)
- Building our own local bridge (permitted)
- NOT using Roo Code Cloud service
- NOT bypassing any paid features (Roomote is their cloud feature)

## Files Modified

All paths are relative to `/prod/home/luke/pm/helix.2`:

**Go Types:**
- `api/pkg/types/task_management.go` - Added AgentHostType enum ✓
- `api/pkg/types/types.go` - Added AgentHostType to ExternalAgentConfig, DesktopAgent ✓

**Desktop Bridge:**
- `api/pkg/desktop/roocode.go` - New file: RooCodeBridge (Socket.IO server) ✓
- `api/pkg/desktop/agent_client.go` - New file: Helix API WebSocket client ✓
- `api/cmd/desktop-bridge/main.go` - Start AgentClient when HELIX_AGENT_HOST_TYPE=vscode ✓
- `api/go.mod` - Added Socket.IO server dependency ✓

**Hydra/Container Config:**
- `api/pkg/external-agent/hydra_executor.go` - Set HELIX_AGENT_HOST_TYPE + ROO_CODE_API_URL env vars ✓

**Container Image:**
- `Dockerfile.ubuntu-helix` - Install VS Code + Roo Code extension ✓
- `desktop/ubuntu-config/startup-app.sh` - Editor selection logic (zed/vscode/headless) ✓
- `desktop/ubuntu-config/start-vscode-helix.sh` - Minimal VS Code startup (14 lines) ✓
- `desktop/shared/start-agenthost-core.sh` - Extended with `start_vscode_helix()` and generic `run_editor_restart_loop()` ✓

## Phase 5: Cursor IDE Integration (In Progress)

### Cursor Architecture

Unlike Roo Code (Socket.IO server ↔ extension), Cursor uses a subprocess-based CLI:

```
[Helix API] <--(WebSocket)--> [AgentClient] <--(subprocess)--> [cursor-agent CLI]
```

**Key differences from Roo Code:**
- Cursor CLI is a subprocess, not a persistent connection
- Each task spawns a new `cursor-agent -p "prompt" --output-format stream-json` process
- No persistent session state between tasks (unlike Roo Code's Socket.IO)
- MCP servers configured via `.cursor/mcp.json` (same pattern as Roo Code)

### Cursor CLI Modes

| Flag | Description |
|------|-------------|
| `-p` / `--print` | Non-interactive (print) mode for automation |
| `--output-format json` | Single JSON result object |
| `--output-format stream-json` | NDJSON events (system, delta, tool_call, result) |
| `--force` | Allow file changes without confirmation |

### Known Cursor CLI Issues

1. **Terminal hang bug**: CLI may not release terminal even with `-p` flag
   - Workaround: CursorBridge uses context timeout + process.Kill()

2. **Initial trust required**: CLI may need interactive session first to trust directory
   - Workaround: Pre-trust `/home/retro/work` during container build

### Files Added for Cursor

**Desktop Bridge:**
- `api/pkg/desktop/cursor.go` - CursorBridge (subprocess management)
- `api/pkg/desktop/agent_client.go` - Updated with Cursor support

**Container Image:**
- `Dockerfile.ubuntu-helix` - Install Cursor IDE + CLI
- `desktop/ubuntu-config/start-cursor-helix.sh` - Cursor startup script
- `desktop/ubuntu-config/startup-app.sh` - Added cursor case to editor selection

**Frontend:**
- `frontend/src/contexts/apps.tsx` - Added `cursor_agent` to AgentConfiguration
- `frontend/src/components/agent/AgentConfigurationSelector.tsx` - Added Cursor option

### MCP Server Configuration

Both Roo Code and Cursor support MCP servers:

| Tool | Global Config | Project Config |
|------|--------------|----------------|
| Roo Code | `mcp_settings.json` | `.roo/mcp.json` |
| Cursor | `~/.cursor/mcp.json` | `.cursor/mcp.json` |

### WebSocket Sync Protocol Compatibility

| Feature | Roo Code | Cursor CLI |
|---------|----------|------------|
| Persistent session | Yes (Socket.IO) | No (subprocess) |
| Real-time streaming | Yes | Partial (NDJSON) |
| Context between messages | Yes | Limited (working dir only) |
| Stop mid-task | Yes | Yes (process.Kill) |

### Licensing

Cursor is proprietary software. Users must have their own Cursor account/license.
- Cursor IDE: Proprietary (user must agree to terms)
- cursor-agent CLI: Same license as IDE
- We're using the CLI in headless mode, not bypassing licensing
