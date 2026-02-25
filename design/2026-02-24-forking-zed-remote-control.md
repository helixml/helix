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

## Streaming Performance: From O(N²) to O(delta)

The naive streaming path had a quadratic cost profile. On every `message_added` event from Zed (dozens per second during fast token streaming), the API would:

1. Query the database for the session (to find the owner)
2. Query the database for the interaction (to get the current state)
3. Write the updated interaction back to the database
4. Serialize the entire interaction as JSON and publish it to the frontend via pubsub

For a 100KB response, this meant pushing 100KB over the WebSocket to the browser on every token. The frontend would then parse the full JSON, update the React Query cache (allocating new interaction arrays), and re-render. By the end of a long response, the browser was doing megabytes of string copying per second and the UI would visibly lag.

### Caching and throttling (Go side)

The first fix was a `streamingContext` struct that caches the session and interaction across the lifetime of a streaming response. Created on the first `message_added`, cleared on `message_completed`. This eliminates two database round-trips per token.

Database writes are throttled to one every 200ms. The in-memory interaction always has the latest content, but we only flush to Postgres when enough time has passed. Risk: up to 200ms of content lost on a crash. Acceptable because `message_completed` always writes the final state, and Zed retains the full content regardless.

Frontend publishes are throttled to one every 50ms. The frontend batches updates to `requestAnimationFrame` (~16ms), so publishing faster than 50ms is wasted work.

### Patch-based deltas (Go → Frontend)

Instead of sending the full interaction JSON on every update, the API computes a patch: the byte offset of the first change and the new content from that point forward.

```go
func computePatch(previousContent, newContent string) (patchOffset int, patch string, totalLength int)
```

In the common case (pure append), the fast path fires: check that `newContent` starts with `previousContent`, return `offset = utf16Len(previousContent)`, `patch = newContent[len(previousContent):]`. This is a single string prefix comparison — no scanning.

For backwards edits (tool call status changing from "Running" to "Finished"), the slow path finds the first differing rune and returns from there.

The frontend receives `interaction_patch` events and applies them directly to a ref (`patchContentRef`), bypassing React state entirely. Multiple patches between animation frames are coalesced. The React Query cache is not touched during streaming — only on completion.

Wire traffic drops from O(N) per update to O(delta). For a 100KB response where each token adds ~20 bytes, that's a 5000x reduction per update.

### The UTF-16 offset bug

The first deployment of the patch protocol produced garbled text. Users saw `"de Statussktop"` where `"desktop"` should have been. The content in the database was correct — the corruption was purely in frontend rendering.

The root cause: `computePatch` returned byte offsets (Go's `len()` counts bytes), but JavaScript's `string.slice()` operates on UTF-16 code units. The streaming content contained 147 instances of `›` (U+203A, RIGHT SINGLE ANGLE QUOTATION MARK — Zed uses this as a breadcrumb separator in tool call output). Each `›` is 3 bytes in UTF-8 but 1 UTF-16 code unit, creating a cumulative offset divergence of 294 bytes. When a backwards edit occurred (tool call status change), the patch was spliced into the wrong position.

The fix iterates by rune rather than byte and tracks the UTF-16 code unit position:

```go
func utf16RuneLen(r rune) int {
    if r >= 0x10000 {
        return 2 // surrogate pair
    }
    return 1
}
```

The slow path now decodes runes from both strings in lockstep, accumulating `utf16Off` alongside `byteOff`. The returned offset matches what JavaScript expects. Supplementary plane characters (emoji like 📤) correctly count as 2 UTF-16 code units.

### Zed-side throttling

Zed fires an `EntryUpdated` event on every token from the LLM. At high token rates this means hundreds of `message_added` WebSocket messages per second, each carrying the full cumulative content for that entry. Most of these are redundant — the Go side only publishes to the frontend every 50ms anyway.

The fix adds a 100ms throttle in Zed's `thread_service.rs`. A `STREAMING_THROTTLE` static tracks the last send time per entry. If less than 100ms has elapsed, the message is buffered. Before every `message_completed`, all pending buffers are flushed to guarantee no data loss. This cuts Zed→Go wire traffic by roughly 90%.
