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

The conversation already exists in Zed's thread as `Vec<Message>` — user text, agent text, tool calls with results. To get this into Agent B's stateful session:

**Approach: First-turn history injection (Zed-side, no agent forks)**

On the first `PromptRequest` after the switch, Zed serializes the old thread's conversation into the message content and prepends it to the user's next message. The agent process sees a single large first message containing the conversation transcript, treats it as context, and incorporates it into its session state going forward.

This works with **any ACP agent unmodified** — no forks, no protocol extension, no `import_session` handler to implement per-agent. The agent just sees a big first message.

**How Zed constructs the first-turn message:**

1. Extract messages from old thread's `Vec<Message>`
2. Serialize to a readable transcript format (user/agent turns, tool calls with results, sub-agent runs as markdown)
3. Wrap in a context preamble, e.g.: `"The following is the conversation history from a previous agent session. Continue from where it left off.\n\n[transcript]"`
4. Prepend to the user's actual new message in the `PromptRequest`

The agent processes this as a normal (large) user message. On subsequent turns, the agent has the transcript in its session buffer and `PromptRequest` sends only the new message as usual.

**Format mapping:**

| Zed Thread Message | Serialized As |
|---|---|
| `Message::User` (text, mentions, images) | `**User:** [text]` |
| `Message::Agent` text | `**Agent:** [text]` |
| `Message::Agent` tool_use + result | `**Agent used tool** [name]: [input]\n**Result:** [output]` |
| `Message::Agent` thinking | Omitted |
| `Message::Resume` | Omitted |
| Sub-agent runs | Flattened to markdown summary |

**Tradeoffs:**
- The first turn after a switch uses more tokens (full transcript + new message). For long conversations, we may need to truncate or summarize older turns.
- The agent doesn't have "native" history — it has a text transcript. It can't re-execute old tool calls, but it understands what happened. This is acceptable.
- Works with every ACP agent today — Claude Code, Qwen Code, Codex, Gemini — without forking any of them.

### Pre-configure all agents in the container

Today, `generateAgentServerConfig()` in the settings-sync-daemon returns config for **one** agent (`settings-sync-daemon/main.go:102-215`). Change it to return configs for all agents simultaneously:

- `qwen` — custom type, stdio command
- `claude-acp` — registry type
- `codex` — when available
- `gemini` — when available

All credentials already exist in the container (`USER_API_TOKEN` is set as both `ANTHROPIC_API_KEY` and `OPENAI_API_KEY`). Zed lazily connects to agent servers (via `AgentConnectionStore.request_connection()`), so idle agents shouldn't spawn processes until first use. Needs verification.

### Helix coordination and thread ID mapping (critical risk area)

An agent switch produces a **new Zed thread ID** while keeping the **same Helix session ID**. This is the most fragile part of the design. Multiple places in the codebase maintain the Helix session ↔ Zed thread mapping, and all must be updated atomically:

1. **`Session.Metadata.ZedThreadID`** — persisted in Postgres, used to route `open_thread` commands on reconnect
2. **`apiServer.contextMappings[threadID] → sessionID`** — in-memory map used to route incoming Zed WebSocket events (`message_added`, `message_completed`) to the correct Helix session
3. **`apiServer.requestToSessionMapping`** — maps in-flight request IDs to sessions
4. **Old thread ID cleanup** — the old mapping must be removed or subsequent events from the old thread (e.g., late-arriving completions) would route to the wrong session

**Race conditions to guard against:**
- A `message_completed` event from Agent A arrives *after* the switch but *before* the mapping is updated → would be attributed to Agent B's thread or dropped
- The `switch_agent` command is sent while an interaction is still in `waiting` state → Agent A responds after the switch, corrupting the timeline
- Helix API updates session metadata but Zed fails to create the new thread → session is in a broken state with a stale `ZedThreadID`

**Proposed safeguards:**
- Reject the switch if any interaction is in `waiting` state (agent must be idle)
- Use a two-phase update: Helix sends the switch command, then waits for `thread_switched` confirmation from Zed before updating mappings. If Zed fails, the switch is rolled back.
- Add the old thread ID to a short-lived "draining" set — events from it are silently dropped rather than routed to the new thread
- The `thread_switched` WebSocket event must include both old and new thread IDs so Helix can atomically swap the mappings

**New endpoint:** `POST /api/v1/sessions/{id}/switch-agent`

Updates `Session.Metadata.ZedAgentName` + `CodeAgentRuntime`, creates a system interaction marker, sends `switch_agent` WebSocket command to Zed. Does NOT update `ZedThreadID` yet — waits for confirmation.

**Confirmation event:** `thread_switched` from Zed → Helix atomically updates `Session.Metadata.ZedThreadID`, swaps `contextMappings`, drains old thread ID.

## Key Decisions

### Why first-turn injection instead of an ACP protocol extension?
An `import_session` ACP message would be cleaner from a protocol standpoint, but it requires forking every agent runtime to implement the handler. We control Claude Code and Qwen Code, but Codex and Gemini are third-party. First-turn injection works with any ACP agent unmodified — the agent just sees a large first message. We can always add `import_session` to ACP later as an optimization for agents we control, but the injection approach ships without blocking on agent forks.

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
2. **How large is a typical conversation transcript?** Long conversations may exceed the agent's context window on the first turn after a switch. May need to truncate or summarize older turns.
3. **How should the transcript be formatted?** The serialization format (markdown vs structured text) affects how well the new agent can parse the history. Needs experimentation with each agent to find what works best.
