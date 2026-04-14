# Design: Mid-Session Agent Switching via Helix-Level Transcript Injection

## Investigation Summary

Investigated three systems: ACP (Agent Communication Protocol) between Zed and external agents, Zed's thread/agent architecture, and the Helix session database. The goal is switching between external code agents (Claude Code, Qwen Code, Codex, Gemini) within a running Zed session.

## Core Insight

Helix already has the full conversation history as Interactions (PromptMessage, ResponseMessage, ResponseEntries). Instead of having Zed handle transcript serialization and agent switching internally, Helix can serialize the interaction history into a markdown transcript and prepend it to the next `chat_message` sent to Zed with the new agent. Zed sees a normal message with an empty `acp_thread_id`, creates a new thread with the target agent automatically, and the existing `handleThreadCreated` flow stores the new thread ID. Zero Zed-side changes required. This also supports switching to non-Zed agents in the future.

## How ACP Works Today

### Agents are stateful
- `PromptRequest` sends **only the current user message**, not the full history
- The agent process maintains its own session state (message buffer)
- `new_session()` creates a fresh empty session in the agent process

### AcpThread.connection is immutable
- Each thread is permanently bound to one agent connection
- Switching agents requires a new thread, not modifying the existing one

### Zed creates new threads automatically
- When `chat_message` is sent with an empty `acp_thread_id`, Zed creates a new thread
- The `thread_created` event fires back to Helix, which stores the new `ZedThreadID` on the session
- This existing flow handles the new-thread-after-switch case perfectly

## Approach: Helix-Level Transcript Injection

```
User requests switch to Agent B via Helix API
  ↓
Helix updates session metadata (ZedAgentName, CodeAgentRuntime)
Helix clears ZedThreadID, drains old thread
  ↓
On next user message, Helix detects ZedThreadID is empty + completed interactions exist
  ↓
Helix serializes interactions into markdown transcript, prepends to message
  ↓
Helix sends chat_message with new agent_name + empty acp_thread_id + transcript
  ↓
Zed creates new thread with Agent B, processes the large first message
  ↓
Agent B has the conversation context, user continues
```

### Why Helix-level, not Zed-level?

The original design had Zed doing the heavy lifting: serialize `Thread.messages` in Rust, handle a new `switch_agent` WebSocket command, create a new AcpThread, send back a `thread_switched` confirmation. This required:
- New Rust code in Zed (transcript serializer, command handler)
- New WebSocket protocol (switch_agent command, thread_switched confirmation)
- Two-phase thread ID swapping with timeout rollback

The Helix-level approach eliminates all of this. Helix already has the data (interactions), already controls message routing (agent_name in chat_message), and already handles new thread creation (handleThreadCreated). The entire switch is just: clear the thread ID, prepend the history to the next message.

This also supports non-Zed agents in the future — any agent backend that accepts messages can receive the transcript.

### How the switch endpoint works

`POST /api/v1/sessions/{id}/switch-agent` with `{"code_agent_runtime": "claude_code"}`:

1. Validate: session ownership, zed_external type, not already on target, no interactions in "waiting" state
2. Update `ZedAgentName` + `CodeAgentRuntime` on session
3. **Clear `ZedThreadID`** — so next message creates a new thread
4. Clean up old thread from `contextMappings`, add to draining set
5. Create system interaction with trigger `"agent_switch"` as visual marker
6. Return success — the actual switch happens on the next message

### How transcript injection works

When Helix sends a `chat_message`, it checks if `ZedThreadID` is empty AND completed interactions exist. If so:

1. Load all interactions for the session
2. Serialize into markdown transcript (user turns, agent responses with tool calls)
3. Prepend to the user's message with a preamble
4. Send with empty `acp_thread_id` → Zed creates new thread

The `ResponseEntries` JSON gives structured text/tool_call data. For legacy interactions without ResponseEntries, fall back to `ResponseMessage`. System interactions (trigger == "agent_switch") are skipped.

**Transcript format:**

| Interaction Field | Serialized As |
|---|---|
| `PromptMessage` | `**User:** [text]` |
| ResponseEntry type=text | `**Agent:** [text]` |
| ResponseEntry type=tool_call | `**Tool Call: [name]** Status: [status]` with fenced content |
| `ResponseMessage` (fallback) | `**Agent:** [text]` |
| Trigger == "agent_switch" | Omitted |

**Truncation:** When transcript exceeds 100KB, oldest turns are dropped with a notice: `[Earlier conversation history truncated — N turns omitted]`

### Pre-configure all agents in the container

`generateAgentServerConfig()` in the settings-sync-daemon returns configs for all agents simultaneously (qwen + claude-acp). All credentials already exist in the container. Zed lazily connects to agent servers, so idle agents shouldn't spawn processes until first use.

### Old thread cleanup

When `switchAgentSession` clears `ZedThreadID`, it also:
- Removes the old thread from `contextMappings` (in-memory routing map)
- Adds the old thread ID to a 60-second draining set

Events from draining threads are silently dropped at the top of `processExternalAgentSyncMessage`. After the draining TTL expires, the old thread ID is cleaned up. Late events that arrive after TTL expiry will fail both the contextMappings lookup and the DB fallback (no session has that ZedThreadID anymore), so they're safely dropped with a warning.

## Key Decisions

### Why first-turn injection instead of an ACP protocol extension?
An `import_session` ACP message would be cleaner from a protocol standpoint, but it requires forking every agent runtime to implement the handler. First-turn injection works with any ACP agent unmodified — the agent just sees a large first message. We can always add `import_session` to ACP later as an optimization.

### Why not just switch the model behind a single agent?
Each ACP agent has a `SetSessionModelRequest` that can change the LLM mid-session. So switching between Anthropic models (Sonnet ↔ Opus) within Claude Code is trivial.

But switching to a different **provider** (e.g., pointing Claude Code at Qwen) doesn't work — different API formats, different caching strategies. So there are two tiers:
- **Same-provider model switch** (Sonnet ↔ Opus): Use `SetSessionModelRequest`. Already works.
- **Cross-provider agent switch** (Claude Code ↔ Qwen Code): Requires the full agent switch with transcript injection described here.

### Why clear ZedThreadID instead of a flag?
Clearing `ZedThreadID` serves double duty: it signals that the next message should create a new thread (existing behavior when `acp_thread_id` is empty), and it enables transcript injection detection (empty thread ID + completed interactions = post-switch state). No new flag needed.

## Codebase Patterns

- **Switch endpoint:** `switchAgentSession()` in `session_handlers.go`
- **Transcript serializer:** `serializeTranscript()` in `websocket_external_agent_sync.go`
- **Transcript injection:** `maybePrependTranscript()` in `websocket_external_agent_sync.go`
- **Message paths (3):** `NotifyExternalAgentOfNewInteraction`, `pickupWaitingInteraction`, `sendChatMessageToExternalAgent`
- **Thread creation:** `handleThreadCreated()` in `websocket_external_agent_sync.go`
- **Draining set:** `isThreadDraining()`, `addDrainingThread()` in `websocket_external_agent_sync.go`
- **Settings daemon:** `generateAgentServerConfig()` in `settings-sync-daemon/main.go`
- **Runtime mapping:** `agentNameForRuntime()` in `session_handlers.go`

## Open Questions

1. **Does Zed eagerly spawn all configured agent_servers processes?** Need to verify lazy spawning — if eager, four agents = four processes at boot.
2. **How large is a typical conversation transcript?** The 100KB limit should cover most conversations, but very long sessions may lose older context.
