# Design: Mid-Session Agent Switching

## Investigation Summary

Investigated three systems: Helix session database, Zed's thread/agent architecture, and the MCP protocol layer. The goal is switching between external code agents (Claude Code, Qwen Code, Codex, Gemini) within a running Zed session inside a Helix container.

## Current Architecture (Constraints)

### One agent per container, configured at startup
- `settings-sync-daemon` calls `generateAgentServerConfig()` which returns config for **one** runtime only (`zed_config.go:81-293`)
- `qwen_code` → injects `agent_servers.qwen` (custom type, stdio)
- `claude_code` → injects `agent_servers.claude-acp` (registry type)
- `zed_agent` → injects nothing (uses built-in agent with env vars)
- The daemon polls `/api/v1/sessions/{id}/zed-config` every 30s and writes to `/home/retro/.config/zed/settings.json`

### AcpThread.connection is immutable
- `AcpThread` stores `connection: Rc<dyn AgentConnection>` set once in `new()` (`acp_thread.rs:1049`)
- There is no `set_connection()` — a thread is permanently bound to the agent that created it
- Each external agent (Claude Code, Qwen Code) is a separate ACP process with its own `AgentConnection`

### But threads are agent-agnostic
- `Thread.messages` stores `Vec<Message>` — user text, agent text, tool calls, tool results
- Nothing in the message format is agent-specific
- `Thread.replay()` can reconstruct the full conversation as a `ThreadEvent` stream
- `SharedThread` can serialize/deserialize threads (zstd-compressed JSON)

### All credentials are already available
- `DesktopAgentAPIEnvVars()` sets `USER_API_TOKEN`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `ZED_HELIX_TOKEN` on every container
- The single ephemeral API key works for all providers (Helix proxies LLM requests)
- No credential changes needed for switching

### MCP servers are container-level, not agent-level
- `context_servers` (helix-native, kodit, helix-desktop, helix-session, chrome-devtools) are configured in settings.json
- `ContextServerRegistry` is per-project, not per-agent
- All MCP tools survive agent switches automatically

## Proposed Architecture

### Strategy: Pre-configure all agents, new thread on switch with replay

Since `AcpThread.connection` is immutable, we cannot swap the agent on an existing thread. Instead:

1. **Configure all agents upfront** in the container's settings.json
2. **On switch**: close the current thread, create a new thread with the target agent, replay conversation history into it
3. **Helix session stays the same** — the Helix session ID, workspace, and container are unchanged

### Flow

```
User clicks "Switch to Qwen" in Helix UI
  ↓
Helix API: POST /sessions/{id}/switch-agent {agent: "qwen_code"}
  ↓
Helix updates Session.Metadata.ZedAgentName + CodeAgentRuntime
  ↓
Helix sends WebSocket command: switch_agent {agent_name: "qwen", acp_thread_id: "thread_xyz"}
  ↓
Zed receives switch_agent:
  1. Saves current thread to DB
  2. Gets connection to target agent (from AgentConnectionStore)
  3. Creates new AcpThread with new connection
  4. Replays old thread's messages into new thread via ThreadEvent stream
  5. Maps new thread ID back to Helix session via WebSocket
  ↓
User sees conversation history, can continue with new agent
```

### Component Changes

#### 1. Settings-sync-daemon: Configure ALL agents

Change `generateAgentServerConfig()` to return configs for **all** available agents simultaneously, not just the selected one.

**Current** (returns one):
```go
switch d.codeAgentConfig.Runtime {
case "qwen_code":
    return map[string]interface{}{"qwen": {...}}
case "claude_code":
    return map[string]interface{}{"claude-acp": {...}}
}
```

**Proposed** (returns all):
```go
func (d *SettingsDaemon) generateAgentServerConfig() map[string]interface{} {
    servers := map[string]interface{}{}

    // Always configure Qwen
    servers["qwen"] = map[string]interface{}{
        "name": "qwen", "type": "custom",
        "command": "qwen",
        "args": []string{"--experimental-acp", "--no-telemetry", "--include-directories", "/home/retro/work"},
        "env": map[string]interface{}{
            "OPENAI_BASE_URL": baseURL,
            "OPENAI_API_KEY":  d.userAPIKey,
            "OPENAI_MODEL":    "nebius/Qwen/Qwen3-Coder",
        },
    }

    // Always configure Claude (registry type)
    servers["claude-acp"] = map[string]interface{}{
        "type": "registry",
        "default_mode": "bypassPermissions",
        "env": map[string]interface{}{...},
    }

    // Always configure Codex, Gemini (when available)
    // servers["codex"] = ...
    // servers["gemini"] = ...

    return servers
}
```

**File:** `api/cmd/settings-sync-daemon/main.go` — modify `generateAgentServerConfig()`

**Tradeoff:** This starts all agent processes at once. Zed lazily connects to agents (only on `connect()`), so idle agents shouldn't consume significant resources. But we should verify that Zed doesn't eagerly spawn all configured agent_servers processes.

#### 2. Helix API: Switch-agent endpoint + WebSocket command

**New endpoint:** `POST /api/v1/sessions/{id}/switch-agent`
```json
{"code_agent_runtime": "qwen_code"}
```

This endpoint:
- Validates the session exists and belongs to the user
- Validates no interaction is currently in `waiting` state
- Updates `Session.Metadata.ZedAgentName` and `CodeAgentRuntime`
- Creates a system interaction marking the switch (audit trail)
- Sends `switch_agent` WebSocket command to Zed

**New WebSocket command:** `switch_agent`
```json
{
  "type": "switch_agent",
  "data": {
    "agent_name": "qwen",
    "acp_thread_id": "thread_xyz"
  }
}
```

**Files:**
- `api/pkg/server/websocket_external_agent_sync.go` — send command
- `api/pkg/types/types.go` — add command constant

#### 3. Zed: Handle switch_agent command

This is the core change. When Zed receives `switch_agent`:

```rust
// In external_websocket_sync handler:
fn handle_switch_agent(agent_name: &str, old_thread_id: &str) {
    // 1. Save current thread
    let old_thread = agent.sessions.get(old_thread_id);
    old_thread.save_to_db();

    // 2. Get connection to new agent
    let new_agent_key = map_agent_name_to_key(agent_name); // "qwen" → Agent::Custom("qwen")
    let new_connection = connection_store.request_connection(new_agent_key);

    // 3. Create new session with new agent
    let new_acp_thread = new_connection.new_session(project, work_dirs);

    // 4. Replay old messages into new thread
    let events = old_thread.replay(); // ThreadEvent stream
    NativeAgentConnection::handle_thread_events(events, new_acp_thread);

    // 5. Map new thread back to same Helix session
    send_ws_event("thread_switched", {
        old_thread_id,
        new_thread_id: new_acp_thread.session_id,
    });
}
```

**Key question: Does the new external agent (Claude Code, Qwen) receive the replayed history?**

No — `replay()` feeds events to the `AcpThread` (Zed's UI layer), not to the external agent process. The external agent starts fresh. On the next user prompt, the external agent will see the full message history because `AcpThread` includes all messages when calling `run_turn()`. This is how external agents already work — they're stateless per-turn; the full conversation is sent each time.

**Important:** Verify that external agent `run_turn()` sends the full `AcpThread.entries` (including replayed history) to the agent process. If the ACP protocol supports this, no additional work is needed. If external agents maintain their own state and ignore the thread history, we'd need a different approach (e.g., system prompt injection).

**Files:**
- `crates/external_websocket_sync/` — handle new command
- `crates/agent/src/agent.rs` — add `switch_agent()` method

#### 4. Helix Frontend: Agent selector

Add agent selector dropdown to session controls:
- Shows all available agents (Claude Code, Qwen Code, Codex, Gemini)
- Indicates which is currently active
- Calls `POST /sessions/{id}/switch-agent`
- Shows "Switching to {agent}..." loading state
- Renders "Agent switched" marker in conversation timeline

#### 5. Thread-to-session mapping update

When the Zed thread changes (new thread_id after switch), Helix needs to update its mapping:
- `Session.Metadata.ZedThreadID` → new thread ID
- `apiServer.contextMappings[newThreadID]` → sessionID
- Remove old mapping

## Key Decisions

### Why pre-configure all agents instead of hot-injecting one?
The settings-sync-daemon already writes settings.json every 30s. We could update it to inject a different agent on switch. But Zed watches settings.json for changes and would need to handle agent_servers additions gracefully (spawn new process, register connection). Pre-configuring all agents avoids this timing-sensitive dance and makes switching instant.

### Why new thread instead of swapping connection?
`AcpThread.connection` is `Rc<dyn AgentConnection>` — immutable by design. Adding a `set_connection()` would require rethinking the ownership model (Rc references are shared). Creating a new thread with replay is the idiomatic Zed approach and uses existing, tested code paths.

### What about conversation context for the new agent?
External ACP agents (Claude Code, Qwen Code) are invoked per-turn with the full message history. When `run_turn()` is called on the new thread, the ACP protocol sends all prior messages (including replayed ones) to the agent process. The new agent doesn't need to "know" about the switch — it just sees a conversation history and continues from there.

### What about agent-specific internal state?
Claude Code maintains `~/.claude/` memory files. Qwen Code has its own state. These persist on the container filesystem and aren't affected by the switch. The new agent won't read the old agent's state, but the conversation history (tool calls, file edits, etc.) provides sufficient context. Out of scope for v1.

## Codebase Patterns

- `generateAgentServerConfig()` in `settings-sync-daemon/main.go:102-215` — currently returns one agent config
- `AgentConnectionStore` in `agent_ui/src/agent_connection_store.rs` — caches one connection per Agent key, lazy-connects on `request_connection()`
- `Thread.replay()` in `agent/src/thread.rs:1102-1134` — emits ThreadEvent stream from stored messages
- `NativeAgentConnection::handle_thread_events()` in `agent/src/agent.rs:1164-1287` — feeds events to AcpThread
- `ExternalAgentCommand` in `types.go:2176` — WebSocket command struct with `Type` string discriminator
- `getAgentNameForSession()` in `zed_config_handlers.go` — maps CodeAgentRuntime → Zed agent name

## Open Questions

1. **Does Zed eagerly spawn all configured agent_servers processes?** If so, pre-configuring all four agents would start four processes at container boot. Need to verify lazy spawning behavior.
2. **Does the ACP `run_turn()` protocol send full message history to external agents?** If external agents are truly stateless per-turn, replay works seamlessly. If they maintain internal session state, we may need to call `load_session()` or inject a system prompt.
3. **Resource impact of multiple idle agent processes.** Claude Code and Qwen Code CLIs may have non-trivial idle memory usage. Need to measure.
