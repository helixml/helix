# Design: Mid-Session Agent Switching

## Investigation Summary

Investigated three systems: Helix session database, Zed's thread/agent architecture, and the MCP protocol layer. The key finding is that **Zed already has most of the machinery needed** — thread replay, agent profiles, and model switching — but Helix and Zed need coordination to make agent switching work end-to-end.

## Current Architecture (What Exists)

### Agent identity is frozen at session creation
- `Session.Metadata.ZedAgentName` is set once when a thread is created
- `Session.Metadata.CodeAgentRuntime` determines which CLI agent runs in the container
- `getAgentNameForSession()` reads this value and never changes it
- The WebSocket `chat_message` and `open_thread` commands include `agent_name` — this is read-only today

### Zed threads are agent-agnostic
- `Thread.messages` stores `Vec<Message>` (user messages + agent messages)
- Messages contain text, tool calls, and tool results — none are agent-specific
- `Thread.profile_id` (AgentProfileId) and `Thread.model` can be changed independently
- `Thread.replay()` reconstructs the full conversation as a `ThreadEvent` stream — this is the replay mechanism

### Zed supports thread import
- `SharedThread` provides a zstd-compressed JSON wire format
- `to_db_thread()` creates a loadable thread from shared data, setting `imported: true`
- `open_thread()` loads from DB → `Thread::from_db()` → `replay()` → feed to UI

### The container persists across agent switches
- The sandbox (dev container) holds the workspace, git state, installed tools
- Only the agent process inside needs to change — the container stays running
- MCP servers (chrome-devtools, helix-desktop, helix-session, etc.) are container-level, not agent-level

## Proposed Architecture

### Strategy: Zed-side thread replay with Helix-coordinated agent switch

The switch happens in three steps:

```
1. Helix API receives "switch agent" request
2. Helix updates session metadata + sends switch command via WebSocket
3. Zed stops current agent, switches profile/model, replays thread to new agent
```

This leverages Zed's existing `replay()` mechanism and avoids cloning sessions or moving data between systems.

### Component Changes

#### 1. Helix API: New endpoint + WebSocket command

**New endpoint:** `POST /api/v1/sessions/{id}/switch-agent`
```json
{
  "code_agent_runtime": "claude_code"   // or "qwen_code", "zed_agent", etc.
}
```

This endpoint:
- Validates the session exists and belongs to the user
- Validates no interaction is currently in `waiting` state (agent must be idle)
- Updates `Session.Metadata.ZedAgentName` to the new agent name
- Updates `Session.Metadata.CodeAgentRuntime` to the new runtime
- Creates a system interaction marking the switch (for audit trail)
- Sends a new WebSocket command `switch_agent` to the connected Zed instance

**New WebSocket command:** `switch_agent`
```json
{
  "type": "switch_agent",
  "data": {
    "agent_name": "claude",
    "acp_thread_id": "thread_xyz"
  }
}
```

**File:** `api/pkg/server/websocket_external_agent_sync.go` — add handler  
**File:** `api/pkg/types/types.go` — add `ExternalAgentCommandSwitchAgent` constant

#### 2. Zed: Handle `switch_agent` command

When Zed receives `switch_agent` via the external WebSocket sync:

1. **Stop current agent turn** (if running) — call `Thread.cancel()` 
2. **Update thread's agent profile** — `Thread.set_profile(new_profile_id)`
3. **Update thread's model** — `Thread.set_model(new_model)` based on agent's default
4. **Re-register MCP tools** — context server registry stays the same (container-level), but agent profile toggles may differ
5. **No replay needed** — the thread messages are already in memory; only the agent identity changes

Key insight: since Zed threads are agent-agnostic, switching the profile + model is sufficient. The conversation history is already loaded. A full `replay()` is only needed if the thread must be loaded from scratch (e.g., container restart during switch).

**File:** `crates/external_websocket_sync/` — handle new command type  
**File:** `crates/agent/src/thread.rs` — no changes needed (profile/model setters exist)  
**File:** `crates/agent/src/agent.rs` — add `switch_agent_for_session()` method

#### 3. Helix Frontend: Agent switcher UI

Add an agent selector to the session controls (next to the existing model selector). When clicked:
- Shows available agents (filtered by what the org has enabled)
- Calls `POST /sessions/{id}/switch-agent`
- Shows a brief "Switching to {agent}..." indicator
- Displays an "Agent switched" marker in the conversation timeline

#### 4. Session interaction marker

When an agent switch occurs, create a system interaction:
```go
interaction := &types.Interaction{
    SessionID:     sessionID,
    State:         types.InteractionStateComplete,
    DisplayMessage: fmt.Sprintf("Agent switched from %s to %s", oldAgent, newAgent),
    Trigger:       "agent_switch",
}
```

This appears in the conversation timeline and is visible via the Session MCP backend's `get_turn` and `session_toc` tools.

## Key Decisions

### Why not clone/fork the session?
Cloning interactions to a new session would duplicate data, break the single-session UX, and lose the workspace association. The container (and its workspace) is already tied to the current session ID.

### Why not use MCP session tools for replay?
The Session MCP backend (`get_turn`, `session_toc`) lets an agent *read* history, but doesn't give it native conversation context. The new agent would have to process the history as a user message ("here's what happened before...") rather than having it in its actual message thread. Zed's thread replay provides true conversation context.

### Why Zed-side switching instead of stopping/starting the container?
The container holds the workspace state (files, git, running processes). Restarting it for an agent switch would be destructive and slow. The agent process is lightweight — only the Zed agent profile and model need to change.

### What about agent-specific state?
Each agent runtime (Claude Code, Qwen Code) may maintain internal state (memory files, config). This state lives in the container filesystem and persists across switches. The new agent won't read the old agent's internal state files, but this is acceptable — the conversation history provides sufficient context. We mark this as out of scope for v1.

## Codebase Patterns Observed

- Helix uses `ExternalAgentCommand` structs with `Type` string discriminator for WebSocket messages (`types.go:2176`)
- Zed's `AgentConnection` trait has `load_session` and `resume_session` — these could be extended for switch scenarios
- The `ContextServerRegistry` is per-project, not per-agent — MCP tools survive agent switches automatically
- `getAgentNameForSession()` in `zed_config_handlers.go` maps `CodeAgentRuntime` → Zed agent name strings
