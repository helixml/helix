# How We Forked Zed and Added Remote Control for Agent Fleet Orchestration

## Why We Forked Zed

Zed is a fast, GPU-accelerated code editor built in Rust. It has excellent language server support, a growing agent panel for AI-assisted coding, and a clean architecture. It has no concept of external orchestration.

Helix runs fleets of coding agents. Each agent is a headless Zed instance running inside a Docker container, connected to an LLM via the Agent Control Protocol (ACP). A central API dispatches tasks, monitors progress, manages thread lifecycles, and streams results back to users in real time. None of this is possible with stock Zed.

We needed three capabilities that required forking:

1. **Remote command injection** -- the API must be able to send chat messages, simulate user input, and query UI state in a running Zed instance, without any human at the keyboard.
2. **Event exfiltration** -- Zed must report back when an agent thread is created, when messages stream in, when the agent finishes, and when errors occur.
3. **Multi-thread lifecycle management** -- when a thread exhausts its context window, Helix starts a new one on the same WebSocket connection. Zed must handle multiple concurrent ACP threads per connection.

## The WebSocket Sync Protocol

The control plane is a single bidirectional WebSocket between the Helix API and each Zed instance. The API side lives in `websocket_external_agent_sync.go`; the Zed side in `crates/external_websocket_sync/`.

**Server to Zed (commands):**

| Command | Purpose |
|---------|---------|
| `chat_message` | Start a new thread or send a follow-up (carries `acp_thread_id`, `message`, `request_id`, `agent_name`) |
| `simulate_user_input` | Inject text into an existing thread as if the user typed it |
| `query_ui_state` | Request a snapshot of Zed's current agent panel state |

**Zed to Server (events):**

| Event | Purpose |
|-------|---------|
| `agent_ready` | Zed is initialized and accepting commands |
| `thread_created` | A new ACP thread was created (carries `acp_thread_id`, `request_id`) |
| `message_added` | Content update -- streaming or complete (carries `acp_thread_id`, `message_id`, `content`, `role`) |
| `message_completed` | The agent finished responding (carries `acp_thread_id`, `request_id`) |
| `thread_load_error` | Thread failed to load |
| `ui_state_response` | Response to a `query_ui_state` query |

Every message that relates to a thread carries `acp_thread_id` for correlation. The `request_id` field ties a command to its eventual `thread_created` and `message_completed` events, enabling the API to track which user request produced which response.

## Architecture

```
Helix Frontend
      |
      | HTTP POST /api/v1/sessions/chat
      v
Helix API  ----WebSocket----> Zed (headless, in container) ---ACP---> LLM
      |                              |
      | pubsub (session_update,      | thread events
      | interaction_update)          | (message_added, etc.)
      v                              |
Helix Frontend <----WebSocket--------+
```

The API manages a map of `acp_thread_id` to Helix session IDs. When a user sends a message, the API creates an Interaction record (with the user's prompt), then dispatches a `chat_message` command over the WebSocket. Zed creates or reuses an ACP thread, the LLM streams its response, and Zed relays each chunk back as `message_added` events. The API accumulates these into the Interaction's response and publishes real-time updates to the frontend.

When context exhausts, Helix sends a new `chat_message` without an `acp_thread_id`, prompting Zed to create a fresh thread. The new `thread_created` event maps the new thread back to the same Helix session. The same WebSocket connection manages the full lifecycle.

## The Multi-Message Accumulation Bug

This was the subtlest bug in the protocol layer. Zed's agent panel produces multiple distinct entries per response turn: an assistant message, one or more tool calls, and a follow-up message. Each entry has its own `message_id`. Within a single entry, Zed streams cumulative content updates (the full content so far for that entry, not deltas).

The old code stored the response as a single `ResponseMessage` string. On each `message_added` event, it overwrote the entire string:

```go
// Old code (simplified) -- the bug
interaction.ResponseMessage = content
```

This worked fine when there was only one `message_id`. But when the response contained multiple entries:

1. `message_added(id="msg-1", content="I'll help you with that.")` -- response = `"I'll help you with that."`
2. `message_added(id="msg-2", content="```tool\nedit")` -- response = `"```tool\nedit"` (msg-1 destroyed)
3. `message_added(id="msg-2", content="```tool\nedit file.py\n```")` -- response = `"```tool\nedit file.py\n```"` (correct overwrite of msg-2, but msg-1 is still gone)

The fix tracks the byte offset where each `message_id`'s content begins. Same ID = replace from offset. New ID = append with separator, record new offset.

```go
type MessageAccumulator struct {
    Content       string
    LastMessageID string
    Offset        int // byte offset where current message_id starts
}

func (a *MessageAccumulator) AddMessage(messageID, content string) {
    if a.LastMessageID == "" {
        a.Content = content
        a.Offset = 0
        a.LastMessageID = messageID
        return
    }

    if a.LastMessageID == messageID {
        // Same message streaming -- replace from offset, keep prefix
        a.Content = a.Content[:a.Offset] + content
        return
    }

    // New distinct message -- record offset, append with separator
    a.Offset = len(a.Content) + 2 // account for "\n\n"
    a.Content = a.Content + "\n\n" + content
    a.LastMessageID = messageID
}
```

Zed sends cumulative content per `message_id` (overwrite semantics), but the overall response is an append-only sequence of distinct message IDs. The accumulator handles both cases with a single offset tracker.

## The Completion Hang Bug

When the agent finished, Zed sent `message_completed`. The handler marked the Interaction as complete in the database and published a `session_update` event to the frontend. Users reported that responses would appear to stream correctly but never show as "complete" -- the loading spinner hung indefinitely.

The root cause was in the frontend's event handling. During streaming, the API published `interaction_update` events. The frontend's `useLiveInteraction` hook consumed these and rendered content in real time. But `handleMessageCompleted` only published `session_update` events.

The frontend's `session_update` handler has rejection logic -- it checks whether the incoming session has the expected number of interactions and silently drops events that fail validation. This was a safeguard against stale data from out-of-order WebSocket messages, but it meant completion events were intermittently discarded.

The fix was to publish through both channels:

```go
// 1. interaction_update -- same channel used during streaming
//    ensures useLiveInteraction sees state=complete
err = apiServer.publishInteractionUpdateToFrontend(
    helixSessionID, helixSession.Owner, targetInteraction, messageRequestID)

// 2. session_update -- full session for React Query cache consistency
err = apiServer.publishSessionUpdateToFrontend(
    reloadedSession, targetInteraction, messageRequestID)
```

The `interaction_update` path bypasses the rejection logic entirely because it targets a specific interaction, not the full session. This is the reliable path for completion signals.

## Testing Strategy: Shared Protocol Code

The original E2E tests used a Python mock WebSocket server that reimplemented the sync protocol. This created a class of bugs where the test and production code diverged silently. The Python mock would not exhibit the accumulation bug because it had its own (simpler) message handling. Tests passed. Production broke.

The solution was to extract a shared `wsprotocol` Go package that both the production Helix server and the Go test server import. Same parsing code, same accumulation logic, same event dispatch. If the accumulator has a bug, the test catches it because it runs the same code path.

## The wsprotocol Package

The package lives at `api/pkg/server/wsprotocol/` and contains four components:

**MessageAccumulator** -- the append/overwrite logic described above. Stateless struct with `AddMessage(messageID, content)`. Serializable for persistence across API restarts.

**Protocol** -- manages the WebSocket lifecycle. Upgrades HTTP connections, reads messages in a loop, parses JSON into `SyncMessage` structs, dispatches to the appropriate handler method. Owns a map of `MessageAccumulator` instances keyed by `acp_thread_id`.

**EventHandler interface** -- the seam between shared protocol code and environment-specific behavior:

```go
type EventHandler interface {
    OnAgentReady(conn *Conn, sessionID string) error
    OnThreadCreated(conn *Conn, sessionID string, evt *ThreadCreatedEvent) error
    OnMessageAdded(conn *Conn, sessionID string, evt *MessageAddedEvent, accumulated string) error
    OnMessageCompleted(conn *Conn, sessionID string, evt *MessageCompletedEvent) error
    OnUIStateResponse(conn *Conn, sessionID string, evt *UIStateResponseEvent) error
    OnThreadLoadError(conn *Conn, sessionID string, evt *ThreadLoadErrorEvent) error
    OnRawEvent(conn *Conn, sessionID string, msg *SyncMessage) error
}
```

Production implements this with database writes and pubsub. Tests implement it with in-memory tracking and assertions. The `OnRawEvent` escape hatch handles Helix-specific events (`chat_response`, `context_title_changed`, etc.) without bloating the shared interface.

**Conn** -- thread-safe wrapper around `gorilla/websocket` with convenience methods: `SendChatMessage`, `SendSimulateUserInput`, `SendQueryUIState`. Mutex-protected writes prevent interleaved frames when multiple goroutines send commands concurrently.

The dispatch logic in `Protocol` handles the parsing once:

```go
func (p *Protocol) dispatch(conn *Conn, sessionID string, msg *SyncMessage) error {
    switch msg.EventType {
    case "message_added":
        evt, err := parseMessageAdded(msg)
        if err != nil {
            return err
        }
        var accumulated string
        if evt.Role == "assistant" && evt.MessageID != "" {
            acc := p.GetAccumulator(evt.ACPThreadID)
            acc.AddMessage(evt.MessageID, evt.Content)
            accumulated = acc.Content
        } else {
            accumulated = evt.Content
        }
        return p.handler.OnMessageAdded(conn, sessionID, evt, accumulated)
    // ... other cases
    }
}
```

The handler receives both the raw event (individual `message_id` content) and the accumulated full response. Production uses the accumulated string for storage; the raw event is available for logging and debugging.

Adding a new event type means: (1) add a struct to `types.go`, (2) add a case to `dispatch`, (3) add a method to `EventHandler`. Both production and test code get the change, or neither does. No more protocol drift.
