# Qwen Code Session Resume Investigation

**Date:** 2025-12-13
**Status:** Fixed

## Problem

Session resume in Qwen Code (running inside Zed in the Helix sandbox) was failing:
- `sessionExists()` returns `true` - the session file exists
- `loadSession()` returns `undefined` - but loading fails silently

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────────────────┐
│ Sandbox Container (helix-sway)                                              │
│                                                                             │
│   ┌─────────────┐    ACP Protocol     ┌─────────────────┐                  │
│   │    Zed      │ ←─────────────────→ │   Qwen Code     │                  │
│   │    (IDE)    │   (stdin/stdout)    │   (Agent)       │                  │
│   └─────────────┘                     └─────────────────┘                  │
│         │                                    │                              │
│         │ Opens folders at:                  │ Stores sessions at:          │
│         │ /home/retro/work/my-repo           │ ~/.qwen/projects/            │
│         │                                    │   -home-retro-work-my-repo/  │
│         │                                    │   chats/<sessionId>.jsonl    │
│         ▼                                    │                              │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │ Workspace mounted at TWO paths (same underlying directory):         │  │
│   │   - /data/workspaces/spec-tasks/{id}/  (for Docker wrapper)         │  │
│   │   - /home/retro/work                    (user-friendly path)        │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────┘
```

## Session Storage

Qwen Code stores sessions in: `~/.qwen/projects/<sanitized-cwd>/chats/<sessionId>.jsonl`

The `<sanitized-cwd>` is computed by `Storage.sanitizeCwd()`:
- Input: `/home/retro/work/my-repo`
- Output: `-home-retro-work-my-repo`

## Session Validation

When loading a session, Qwen Code validates that it belongs to the current project:

1. **Compute current project hash**: `getProjectHash(cwd)` → SHA256 of normalized cwd
2. **Read first record from session file**: Contains `cwd` field from when session was created
3. **Compute record's project hash**: `getProjectHash(firstRecord.cwd)`
4. **Compare hashes**: If mismatch, reject the session

## Initial Theory (DISPROVEN)

We suspected path mismatch between `/data/workspace` and `/home/retro/work` caused hash mismatches.

**Why this was wrong:**
1. Zed ALWAYS opens folders at `/home/retro/work/...` (see `start-zed-helix.sh`)
2. Zed sends this path via ACP `NewSessionRequest.cwd`
3. Qwen Code receives `/home/retro/work/...` consistently
4. The normalization code we added for `/data/workspace` is irrelevant

## Root Cause Found (2025-12-13)

**The issue is NOT in qwen-code's SessionService.** The session loading code works correctly.

**The actual bug is in Zed's `external_websocket_sync/thread_service.rs`:**

When the sandbox container restarts:
1. Zed restarts and the in-memory thread registry is empty
2. Helix sends a follow-up message with an existing `acp_thread_id`
3. `thread_service.rs` checks `get_thread(existing_thread_id)` - returns None (empty registry)
4. Instead of trying to **load** the session from qwen-code, it **creates a new thread**

The flow in `create_new_thread_sync()` at lines 161-198:
```rust
} else if let Some(thread) = get_thread(existing_thread_id) {
    // Send to existing thread (found in registry)
    ...
} else {
    // Thread not in registry -> CREATE NEW instead of LOADING
    eprintln!("⚠️ [THREAD_SERVICE] Thread {} not found, creating new thread", existing_thread_id);
}
```

**The fix:** When `get_thread()` returns None and `acp_thread_id` is provided, call `load_thread_from_agent()` to load the session from qwen-code via ACP `session/load` protocol.

**Fix implemented in:** `zed/crates/external_websocket_sync/src/thread_service.rs`
- Added `load_thread_from_agent()` async function that connects to the agent, loads session, registers thread, subscribes to events
- Modified message handler to try loading before creating new thread

## Verified Working Components

- Session file storage works: `~/.qwen/projects/-home-retro-work-helix-specs/chats/<uuid>.jsonl`
- Session file has correct `cwd="/home/retro/work/helix-specs"`
- Zed's saved session file works: `/home/retro/work/helix-specs/.zed/acp-session-qwen.json`
- qwen-code's SessionService correctly validates and loads sessions
- ACP `session/load` protocol is implemented in qwen-code

## Relevant Code Locations

- **Zed ACP client**: `zed/crates/agent_servers/src/acp.rs`
  - `new_thread()` and `load_thread()` send cwd to qwen-code
  - cwd comes from project worktree: `project.worktrees(cx).next().abs_path()`

- **Zed startup script**: `helix/wolf/sway-config/start-zed-helix.sh`
  - Builds `ZED_FOLDERS` array using `WORK_DIR="$HOME/work"`
  - Zed opens folders at `/home/retro/work/...`

- **Qwen Code ACP handler**: `qwen-code/packages/cli/src/acp-integration/acpAgent.ts`
  - `loadSession()` method handles ACP LoadSessionRequest
  - Creates `SessionService(params.cwd)` to validate/load session

- **Session Service**: `qwen-code/packages/core/src/services/sessionService.ts`
  - `sessionExists()` - checks if session file exists and belongs to current project
  - `loadSession()` - loads full session data
  - Both use `getProjectHash()` for validation

- **Storage**: `qwen-code/packages/core/src/config/storage.ts`
  - `getProjectDir()` - where sessions are stored
  - Uses `sanitizeCwd()` to convert path to directory name

## Existing Debug Logging

Both `acpAgent.ts` and `sessionService.ts` have debug logging:
- `🔄 [ACP SESSION LOAD]` - ACP-level logging
- `🔍 [SESSION LOAD]` - SessionService-level logging

Logs go to stderr (via `console.error`) which Zed captures.

## Additional Fix: ACP Session UI Loading (2025-12-13)

**Problem:** Users couldn't see ACP agent sessions in the thread list when Zed started.

The `list_sessions` ACP call was only made AFTER a new thread was created (in `thread_view.rs`),
not at Zed startup. This meant:
- If you created sessions with Qwen Code standalone, they wouldn't appear in Zed's thread list
- You had to create a new thread first to trigger the session list fetch

**Fix implemented in:** `zed/crates/agent_ui/src/agent_panel.rs`
- Added `load_acp_sessions_from_agents()` method called during `AgentPanel::new()`
- Added `load_sessions_from_agent()` async helper to connect to each agent and fetch sessions
- Sessions are fetched from all configured external agents (from `agent_server_store.external_agents()`)
- Sessions are stored in-memory in `HistoryStore.acp_agent_sessions` (NOT persisted to SQLite)
- Sessions are re-fetched each time Zed starts (as designed - state lives on agent side)

**Key behavior:**
- Sessions created outside Zed (e.g., Qwen Code CLI) appear in Zed's thread list
- Sessions are dynamically loaded, not persisted locally
- Each agent's sessions are fetched in parallel via separate spawned tasks
- Agents that don't support `session/list` are skipped gracefully

**Logging added:**
- `📋 [AGENT_PANEL] Loading ACP sessions from configured agents at startup...`
- `📋 [AGENT_PANEL] Found N external agents: [...]`
- `📋 [AGENT_PANEL] Fetched N sessions from agent X at startup`

## Critical Fix: Missing `type` field in agent_servers config (2025-12-13)

**Problem:** Qwen Code wasn't appearing in `external_agents()` iterator despite being configured.

Zed deserializes `agent_servers` using a tagged enum:
```rust
#[serde(tag = "type", rename_all = "snake_case")]
pub enum CustomAgentServerSettings {
    Custom { command, args, env, ... },
    Extension { ... },
}
```

The `settings-sync-daemon` was generating:
```json
{
  "agent_servers": {
    "qwen": {
      "command": "qwen",
      "args": [...],
      "env": {...}
    }
  }
}
```

Without the required `"type": "custom"` field, serde couldn't deserialize this, so `AllAgentServersSettings.custom` was empty, and `external_agents()` never included Qwen.

**Fix implemented in:** `helix/api/cmd/settings-sync-daemon/main.go`
- Added `"type": "custom"` to the Qwen agent_servers configuration

**Result:** Now the config is:
```json
{
  "agent_servers": {
    "qwen": {
      "type": "custom",
      "command": "qwen",
      ...
    }
  }
}
```

This allows Zed's `AgentServerStore` to properly register Qwen as a custom agent, which enables:
1. `external_agents()` includes "qwen"
2. `load_acp_sessions_from_agents()` queries Qwen at startup
3. Session resume works because the session loading flow finds the agent

## Complete Flow Diagrams

### Zed Startup Flow (Session List Loading)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Zed Startup                                                                  │
│                                                                              │
│  AgentPanel::new()                                                           │
│    │                                                                         │
│    ├── sync_agent_servers_from_extensions()  ← Extension agents             │
│    │                                                                         │
│    └── load_acp_sessions_from_agents()       ← NEW: Fetches sessions        │
│          │                                                                   │
│          └── for each agent in external_agents():                           │
│                ├── Connect to agent                                          │
│                ├── list_sessions(cwd)                                       │
│                └── Store in HistoryStore.acp_agent_sessions                 │
│                                                                              │
│  History View renders:                                                       │
│    - AcpThread entries (NativeAgent, from SQLite)                           │
│    - TextThread entries (from filesystem)                                   │
│    - AcpAgentSession entries (Qwen/external, from memory)                   │
│                                                                              │
│  User clicks AcpAgentSession:                                               │
│    └── load_acp_agent_session(agent_name, session_id)                       │
│          └── AcpThreadView::new_with_acp_session(session_id)                │
│                └── initial_state() → connection.load_thread(session_id)     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Sandbox Restart + Helix Resume Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Sandbox Restart + Helix Resume                                               │
│                                                                              │
│  Helix → WebSocket → create_new_thread_sync(acp_thread_id=existing_id)      │
│    │                                                                         │
│    └── get_thread(existing_id) returns None (registry empty after restart) │
│          │                                                                   │
│          └── load_thread_from_agent(existing_id)  ← FIXED                   │
│                ├── Connect to agent (if not connected)                       │
│                ├── connection.load_thread(existing_id)                       │
│                ├── Register thread in registry                               │
│                └── Continue with resumed session                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Settings Flow (agent_servers Configuration)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Settings Sync Daemon                                                         │
│                                                                              │
│  generateAgentServerConfig()                                                │
│    │                                                                         │
│    └── Returns for qwen_code runtime:                                       │
│          {                                                                   │
│            "qwen": {                                                         │
│              "type": "custom",    ← CRITICAL: Required for serde            │
│              "command": "qwen",                                              │
│              "args": ["--experimental-acp", ...],                           │
│              "env": { "OPENAI_BASE_URL": ..., ... }                         │
│            }                                                                 │
│          }                                                                   │
│                                                                              │
│  Writes to ~/.config/zed/settings.json                                      │
│    │                                                                         │
│    └── Zed's SettingsStore observes changes                                 │
│          │                                                                   │
│          └── AgentServerStore.agent_servers_settings_changed()              │
│                │                                                             │
│                └── reregister_agents()                                       │
│                      │                                                       │
│                      └── external_agents.insert("qwen", LocalCustomAgent)   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Summary of All Fixes

| Issue | Root Cause | Fix Location |
|-------|------------|--------------|
| Session resume fails after restart | thread_service.rs created new thread instead of loading | `zed/crates/external_websocket_sync/src/thread_service.rs` |
| Sessions don't appear at startup | list_sessions only called after thread creation | `zed/crates/agent_ui/src/agent_panel.rs` |
| Qwen not in external_agents() | Missing `"type": "custom"` in settings | `helix/api/cmd/settings-sync-daemon/main.go` |
| Docker wrapper path translation | Bind mount broke symlink resolution | `helix/wolf/*/docker-wrapper.sh` |

## Reverted Changes

Removed unnecessary path normalization in `Storage` class that was added based on incorrect theory:
- `storage.ts` no longer calls `normalizeProjectPath()` in `getProjectDir()`, `getProjectTempDir()`, `getHistoryDir()`
- The `normalizeProjectPath()` function in `paths.ts` still exists (used by `getProjectHash()`)
