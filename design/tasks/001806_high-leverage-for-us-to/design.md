# Design: Mid-Session Agent Switching via ACP

## Investigation Summary

Investigated three systems: ACP (Agent Communication Protocol) between Zed and external agents, Zed's thread/agent architecture, and the Helix session database. The goal is switching between external code agents (Claude Code, Qwen Code, Codex, Gemini) within a running Zed session.

## Core Insight

Zed's thread format is the consistent interchange format. `Thread.messages` stores user messages, agent responses, tool calls, and tool results in an agent-agnostic `Vec<Message>`. When Agent A runs a session, its output gets captured in this format. That same format can be replayed into Agent B. Agent-specific features that don't translate (e.g., sub-agent runs) degrade into opaque markdown blobs — acceptable for context continuity.

## How ACP Works Today

### Agents are stateful
- `PromptRequest` sends **only the current user message**, not the full history (`acp_thread.rs:2096-2147`)
- The agent process maintains its own session state (message buffer)
- `new_session()` creates a fresh empty session in the agent process

### Session loading replays via SessionUpdate
- `load_session(session_id)` asks the agent to load a session from its own storage
- The agent replays history by streaming `SessionUpdate` messages back to Zed: `UserMessageChunk`, `AgentMessageChunk`, `ToolCall`, `ToolCallUpdate`, etc. (`acp_thread.rs:1383-1434`)
- These populate `AcpThread.entries` for display
- **Limitation:** `load_session` only loads sessions that exist in that specific agent's storage — it can't load another agent's sessions

### AcpThread.connection is immutable
- Each thread is permanently bound to one agent connection (`acp_thread.rs:1049`)
- Switching agents requires a new thread, not modifying the existing one

## Proposed Approach

### The replay path

```
Agent A session running in Zed
  ↓
User requests switch to Agent B
  ↓
Zed already has the full conversation in Thread.messages (agent-agnostic format)
  ↓
Create new AcpThread bound to Agent B's connection
  ↓
Replay old thread's messages into Agent B's session
  ↓
Agent B now has the conversation context, user continues
```

The key question is step 5: how does Agent B's process get the conversation context?

### Getting history into Agent B

The conversation already exists in Zed's thread as `Vec<Message>` — user text, agent text, tool calls with results. To get this into Agent B's stateful session, there are two viable approaches:

**Option A: Extend ACP with `import_session`**

Add a new ACP request that combines `new_session` + pre-populated history:

```
ImportSessionRequest {
    cwd: PathBuf,
    messages: Vec<ImportedMessage>,  // The conversation history from Zed's thread
}
```

The agent process creates a new session, seeds its internal buffer with the messages, and responds. This is a clean, first-class protocol operation. Each agent runtime (Claude Code, Qwen Code, etc.) implements the handler by populating its internal message buffer.

The `ImportedMessage` format maps directly from Zed's thread messages:
- User messages → user role + content blocks
- Agent text responses → agent role + text content
- Tool calls + results → tool_use + tool_result content blocks
- Sub-agent runs, agent-specific features → flattened into text/markdown blobs

**Option B: First-turn history injection**

On the first `PromptRequest` after the switch, include the full conversation history alongside the new user message. The agent treats the history as context for this turn and incorporates it into its session state going forward.

This requires no new ACP message type but may not work well with all agents (some may not handle a massive first message gracefully).

**Recommendation:** Option A. It's cleaner, explicit, and each agent can handle the import in the way that best fits its internal architecture.

### Pre-configure all agents in the container

Today, `generateAgentServerConfig()` in the settings-sync-daemon returns config for **one** agent (`settings-sync-daemon/main.go:102-215`). Change it to return configs for all agents simultaneously:

- `qwen` — custom type, stdio command
- `claude-acp` — registry type
- `codex` — when available
- `gemini` — when available

All credentials already exist in the container (`USER_API_TOKEN` is set as both `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`). Zed lazily connects to agent servers (via `AgentConnectionStore.request_connection()`), so idle agents shouldn't spawn processes until first use. Needs verification.

### Helix coordination

**New endpoint:** `POST /api/v1/sessions/{id}/switch-agent`

Updates `Session.Metadata.ZedAgentName` + `CodeAgentRuntime`, creates a system interaction marker, sends `switch_agent` WebSocket command to Zed.

**Thread mapping update:** When Zed creates the new thread, it sends `thread_switched` back via WebSocket. Helix updates `Session.Metadata.ZedThreadID` and `contextMappings` to point to the new thread.

### Message format translation

Zed's `Message` enum maps to `ImportedMessage` roughly 1:1:

| Zed Thread Message | Imported As |
|---|---|
| `Message::User` (text, mentions, images) | User role, text + image content blocks |
| `Message::Agent` text | Agent role, text content blocks |
| `Message::Agent` tool_use | Agent role, tool_use content block |
| Tool results (in `AgentMessage.tool_results`) | tool_result content block |
| `Message::Agent` thinking | Omitted or flattened to text (agent-specific) |
| `Message::Resume` | Omitted |
| Sub-agent runs | Flattened to markdown text blob |

Agent-specific features that don't have a clean mapping degrade to readable text. The new agent gets the gist even if it can't re-execute sub-agent runs or parse thinking blocks.

## Key Decisions

### Why is this feasible?
Zed's thread format is already the common denominator. Every agent's output gets normalized into `Thread.messages` when captured via `SessionUpdate`. The format translation from thread messages to importable messages is straightforward — it's mostly identity mapping with graceful degradation for agent-specific features.

### Why not just use Zed's built-in model switching?
Model switching (changing the LLM behind Zed's built-in agent) is trivial — `Thread.set_model()`. But switching between external ACP agents (Claude Code ↔ Qwen Code) means switching between entire agent **processes** with different capabilities, tools, and runtimes. These are fundamentally different programs, not just different models.

### Why new thread instead of swapping connection?
`AcpThread.connection` is `Rc<dyn AgentConnection>` — immutable by design. The Rc is shared across multiple owners. Mutating it would require rethinking the ownership model. Creating a new thread with import is the idiomatic path.

## Codebase Patterns

- **Thread messages (agent-agnostic):** `Thread.messages: Vec<Message>` in `agent/src/thread.rs`
- **ACP session creation:** `AcpConnection::new_session()` in `agent_servers/src/acp.rs:556-688`
- **SessionUpdate handling:** `AcpThread::handle_session_update()` in `acp_thread/src/acp_thread.rs:1383-1434`
- **Thread replay:** `Thread.replay()` in `agent/src/thread.rs:1102-1134`
- **AgentConnection trait:** `load_session()`, `resume_session()` in `acp_thread/src/connection.rs:59-107`
- **Settings daemon agent config:** `generateAgentServerConfig()` in `settings-sync-daemon/main.go:102-215`
- **WebSocket commands:** `ExternalAgentCommand` in `types.go:2176`

## Open Questions

1. **Does Zed eagerly spawn all configured agent_servers processes?** Need to verify lazy spawning — if eager, four agents = four processes at boot.
2. **Which agents do we control?** We control Claude Code (via claude-acp) and Qwen Code. Codex and Gemini are third-party — they'd need upstream ACP `import_session` support or a wrapper.
3. **How large is a typical conversation for import?** If conversations are very large, we may need to truncate or summarize older messages to fit within agent context windows.
