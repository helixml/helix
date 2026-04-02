# Auto-Start Dev Container: WebSocket Sync Issues After Reconnect

**Date**: 2026-04-02
**Context**: Testing auto-start of dev containers when messages are sent to stopped sessions (PRs #2113, #2121, #2122, #2123, #2124, #2125)

## Incident Timeline (spt_01kn54z1vm6zd89cdj07fvqnb8)

User sent "carry on 3" to a stopped task. The dev container auto-started correctly, but multiple sync issues occurred.

### What happened (API logs, session ses_01kn54z48b3c6b3kye68ey41y5)

1. **16:26:36** — Prompt queue processes "carry on 3", creates interaction `int_01kn7g9849gekkc13jrparx1a2`, `sendCommandToExternalAgent` fails (no WS), auto-start triggered. In-memory state cleaned up (PR #2123 fix).

2. **16:26:52** — Agent WebSocket connects. `pickupWaitingInteraction` finds `int_01kn7g9849gekkc13jrparx1a2` in waiting state, sets up `requestToSessionMapping` and `sessionToWaitingInteraction`, sends `open_thread` + `chat_message` (queued for agent_ready).

3. **16:26:53** — `agent_ready` received. Queued `chat_message` ("carry on 3") flushed to agent.

4. **16:26:56** — Zed opens existing thread `8d9516e7-...`, **replays entire thread history** as `message_added` events (messages 9-50, all `role=assistant`). Every replayed message hits `handleMessageAdded` which routes it to `int_01kn7g9849gekkc13jrparx1a2` and overwrites its response with `content_length=0`.

5. **16:27:00** — `message_completed` arrives with `request_id=int_01kn7g9849gekkc13jrparx1a2`. Interaction is popped from FIFO queue and marked complete. But `response_length=0` — **the response content was wiped by the thread history replay**.

6. **16:27:00** — Warning: `⚠️ message_completed but response_message is EMPTY — content may have been lost during streaming flush`

7. **16:27:28** — Sandbox reconnects (separate connection). Container recovered.

8. **16:27:47** — User types "you there?" directly in Zed. Anthropic API call happens (3 prompt tokens, 9 completion tokens → "Yep, here!"). **This response never syncs to Helix** — no interaction was created for it, no `message_completed` is processed.

9. **16:28:57** — Another WebSocket connection + open_thread cycle. Multiple reconnects happening.

### Container-side logs

- `open_thread` with `acp_thread_id=8d9516e7-...` received and processed correctly
- `chat_message` "carry on 3" received with correct `request_id`
- WebSocket connection dropped and reconnected multiple times (`Connection reset without closing handshake`)

## Root Cause Analysis

### Issue 1: Thread History Replay Wipes Current Interaction Response

**Severity: Critical**

When Zed opens an existing thread (`open_thread`), it replays ALL historical messages as `message_added` events. These are indistinguishable from new streaming content. `handleMessageAdded` routes them to the current waiting interaction, overwriting its `response_entries` with empty content from old messages.

**Root cause detail**: `pickupWaitingInteraction` queues both `open_thread` AND `chat_message` together (both sent immediately after `agent_ready`). But Zed needs to finish loading the thread and replaying history before the new `chat_message` is processed.

### Issue 2: Prompt Text Contaminated with CLI Output

**Severity: Medium**

The "carry on 3" message was delivered with XML prefix from local CLI command output. The prompt history sync captures terminal buffer content along with the user's message.

**Fix**: Frontend should strip `<command-name>`, `<local-command-stdout>`, and similar tags from the prompt content before syncing.

### Issue 3: User Messages Typed Directly in Zed Don't Sync to Helix

**Severity: Medium**

When the user typed "you there?" directly in Zed (not via the Helix UI), the agent responded but the interaction was never created in Helix.

### Issue 4: Multiple WebSocket Reconnections

**Severity: Low**

Three connections established in 2 minutes, with "Connection reset without closing handshake" errors.

## Chosen Fix: Delay `agent_ready` until thread is loaded (PR #2125)

### Design

**Before (broken):**
1. WebSocket connects
2. Zed sends `agent_ready` (immediately)
3. `pickupWaitingInteraction` sends `open_thread` + `chat_message` together
4. Zed opens thread → replays history → `message_added` events corrupt the interaction

**After (fixed):**
1. WebSocket connects
2. API sends `open_thread` immediately (before `agent_ready` gate) — only if session has `ZedThreadID`
3. Zed receives `open_thread`, loads thread, replays history
4. Zed sends `agent_ready` only after thread loading is complete (thread_service.rs:1253, 1404)
5. API receives `agent_ready`, flushes queued `chat_message`
6. API-side guard: `handleMessageAdded` drops events when `isSessionReady()==false`

**Fresh start (no thread):**
1. WebSocket connects
2. No `open_thread` sent (no `ZedThreadID`)
3. Zed sends `agent_ready` after 5s fallback timer
4. `pickupWaitingInteraction` sends `chat_message` → triggers `thread_created`

---

## Restart Resilience Analysis

### Scenario A: Zed Restart (container stops → auto-start → fresh Zed process)

**What Zed loses:**
- Thread registry (`THREAD_REGISTRY`, `THREAD_KEEP_ALIVE`) — all thread entity references
- Subscriptions (`PERSISTENT_SUBSCRIPTIONS`) — cleared on process restart
- Message ID mappings (`THREAD_REQUEST_MAP`, `THREAD_AGENT_SESSION_MAP`)
- External origin tracking (`EXTERNAL_ORIGINATED_ENTRIES`)
- Streaming throttle state (`STREAMING_THROTTLE`)

**Recovery flow:**
1. Zed starts fresh, workspace restoration loads `SerializedAgentPanel` (agent_panel.rs:854-883)
2. `load_agent_thread()` restores the last active thread from the serialized state
3. WebSocket connects → Helix sends `open_thread` → Zed loads thread from ACP agent DB
4. Thread service sends `agent_ready` after thread is loaded (thread_service.rs:1253)
5. Helix flushes queued `chat_message`

**Collision risk: Zed workspace restore vs. Helix `open_thread`**

Both paths try to load the same thread:
- **Zed's native restore**: `SerializedAgentPanel` → `load_agent_thread()` → loads thread from ACP agent
- **Helix's `open_thread`**: `open_existing_thread_sync()` → loads thread from ACP agent

The collision is mitigated at thread_service.rs:1282:
```rust
if let Some(thread_weak) = get_thread(&request.acp_thread_id) {
    // Thread already loaded — just ensure subscription, skip re-load
    ensure_thread_subscription(&thread_entity, &request.acp_thread_id, cx);
    return Ok(());
}
```

And `ensure_thread_subscription` is idempotent (thread_service.rs:417):
```rust
if has_persistent_subscription(thread_id) {
    return; // already subscribed
}
```

**Remaining risk**: If Helix's `open_thread` arrives BEFORE Zed's workspace restore has registered the thread in the registry, both paths load independently. The second load would create a second `Entity<AcpThread>` for the same thread. However, `register_thread` overwrites the registry entry, so only the latest entity survives. The earlier entity's subscription becomes orphaned (subscribed to a dropped entity → events silently lost).

**Verdict**: **LOW risk** — the registry overwrite prevents corruption, and the thread service always re-subscribes on `open_thread`.

**What happens to in-flight interactions:**
- Interaction stays in `Waiting` state in DB
- `pickupWaitingInteraction` finds it on reconnect (websocket_external_agent_sync.go:399-410)
- Message is re-sent to Zed after `agent_ready`
- **Result**: Message delivery is reliable ✅

**What happens to partial streaming content:**
- `streamingContexts` is in-memory only, lost on Zed disconnect
- The interaction stays in `Waiting` state
- On reconnect, `pickupWaitingInteraction` re-sends the original `chat_message`
- Zed generates a fresh response
- **Result**: Up to 200ms of streamed content lost, but full response regenerated ✅

**Gap: `streamingContexts` not cleaned up on disconnect**
When Zed disconnects mid-stream, `handleExternalAgentReceiver` returns but `streamingContexts[sessionID]` is never deleted. Memory leak + stale context if reused.
**Fix**: Clean up `streamingContexts` in the defer/error handler of `handleExternalAgentReceiver`.

### Scenario B: Helix API Restart (Air hot-reload or crash)

**What Helix loses:**
- `contextMappings` (acp_thread_id → helix_session_id) — **in-memory only**
- `sessionToWaitingInteraction` (session → FIFO queue) — **in-memory only**
- `requestToSessionMapping` (request_id → session_id) — **in-memory only**
- `readinessState` (per-session readiness tracking) — **in-memory only**
- `streamingContexts` (buffered streaming content) — **in-memory only**
- `sessionCommentTimeout` (comment processing timers) — **in-memory only**

**Recovery flow:**
1. Zed detects WebSocket close, enters reconnection loop (exponential backoff: 1s → 2s → ... → 30s)
2. Helix restarts, new process has empty in-memory state
3. Zed reconnects → `handleExternalAgentSync` fires
4. `contextMappings` rebuilt from `session.Metadata.ZedThreadID` (line 338-346) ✅
5. `pickupWaitingInteraction` finds `Waiting` interactions from DB (line 399-410) ✅
6. Readiness state initialized fresh (line 353) ✅
7. `open_thread` sent on connect (line 364+) ✅
8. `ResumeCommentQueueProcessing` runs on startup (server.go:581), resets stuck comments ✅

**What's NOT recovered:**
- **`readinessState.PendingQueue`** — messages queued but not yet sent to Zed. If Helix crashes while messages are in the pending queue (between creation and `agent_ready` flush), those messages are lost. However, if they correspond to DB interactions in `Waiting` state, `pickupWaitingInteraction` will re-discover and re-send them.
- **Prompt history entries in `sending` state** — a prompt marked `sending` but never confirmed as `sent`. `processPendingPromptsForIdleSessions` will re-scan on the next sync, but the `sending` status might prevent re-processing (depends on the store query). **Gap: needs a timeout to revert `sending` → `pending`.**

**Gap: Partial streaming content lost**
If `streamingContext` was mid-flush (200ms throttle interval), up to 200ms of content is lost. However, since Zed has the full response, when it reconnects and the thread is re-opened via `open_thread`, the full history is replayed. With the `isSessionReady` guard, this replay is correctly dropped (it's old content, not new). But the original interaction's response is incomplete.
**Mitigation**: The interaction is in `Waiting` state, so `pickupWaitingInteraction` will re-send the `chat_message` and get a fresh response.

### Scenario C: Simultaneous Restart (both Zed and Helix crash)

**What happens:**
1. Both processes crash — all in-memory state lost on both sides
2. Helix restarts first (typically faster — Go binary)
3. Zed container auto-starts (container creation + Zed startup — slower)
4. Zed connects → `handleExternalAgentSync` → `pickupWaitingInteraction` → re-sends

**Result**: The message is re-delivered. The original partial response is lost, but a fresh response is generated. **Acceptable** — the user didn't see the original response anyway.

---

## ACP Agent Compatibility

Both built-in Zed ACP agent (NativeAgent) and external ACP agents (Claude Code, Qwen Code) go through the same `ThreadService` code paths:

```rust
// thread_service.rs — agent selection
match request.agent_name.as_deref() {
    Some("zed-agent") | Some("") | None => ExternalAgent::NativeAgent,
    Some("claude") => ExternalAgent::Custom { name: CLAUDE_AGENT_ID, ... },
    Some(other) => ExternalAgent::Custom { name: other, ... },
}
```

Both agent types use:
- Same `open_existing_thread_sync` for thread loading (thread_service.rs:1267)
- Same `load_thread_from_agent` for loading from ACP DB (thread_service.rs:1188)
- Same `ensure_thread_subscription` for event subscriptions (thread_service.rs:412)
- Same `agent_ready` signaling after thread load (thread_service.rs:1253, 1404)

The protocol change (delay `agent_ready` until thread loaded) applies uniformly to all agent types. ✅

---

## State Persistence Summary

| State | Persisted? | Zed Restart Recovery | Helix Restart Recovery |
|-------|-----------|---------------------|----------------------|
| Session metadata (ZedThreadID) | **YES** (DB) | Rebuilt on connect ✅ | Rebuilt on connect ✅ |
| Waiting interactions | **YES** (DB) | Re-sent via pickupWaitingInteraction ✅ | Re-sent via pickupWaitingInteraction ✅ |
| contextMappings | **Partial** (in ZedThreadID) | Rebuilt from metadata ✅ | Rebuilt from metadata ✅ |
| sessionToWaitingInteraction | **NO** (memory) | Rebuilt from DB ✅ | Rebuilt from DB ✅ |
| readinessState.PendingQueue | **NO** (memory) | N/A (fresh Zed) | **LOST** ⚠️ (mitigated by pickupWaitingInteraction) |
| streamingContexts | **Partial** (200ms flush) | New context on reconnect ✅ | New context on reconnect ✅ |
| Prompt history entries | **YES** (DB) | Re-processed on sync ✅ | Re-processed on sync ✅ |
| Comment queue (QueuedAt) | **YES** (DB) | ResumeCommentQueueProcessing ✅ | ResumeCommentQueueProcessing ✅ |
| PERSISTENT_SUBSCRIPTIONS (Zed) | **NO** (memory) | Cleared, recreated via open_thread ✅ | N/A |

---

## Remaining Gaps (Priority Order)

### HIGH: `streamingContexts` not cleaned up on disconnect
When Zed disconnects mid-stream, the streaming context leaks. Clean up in the defer of `handleExternalAgentReceiver`.

### HIGH: Prompt history `sending` → `pending` timeout
If a prompt is marked `sending` but Helix crashes before confirming, it's orphaned. Need a background job that reverts `sending` → `pending` after 5 minutes.

### MEDIUM: Workspace restore + `open_thread` timing race
If Helix's `open_thread` arrives before Zed's workspace restore registers the thread, the thread is loaded twice. The second load's entity overwrites the registry, orphaning the first entity's subscriptions. Low probability but possible on slow disk I/O.

### MEDIUM: `sessionToWaitingInteraction` is append-only
The FIFO queue grows unboundedly. Clean up entries when interactions transition to terminal states.

### LOW: No request_id correlation in `message_added`
The protocol doesn't include `request_id` in `message_added` events, so Helix can't verify responses match the right interaction. Currently relies on FIFO ordering which is fragile.

---

## E2E Test Gaps

### Phase 12: Zed restart reconnection test
1. Complete a message (Phase 1-style)
2. Kill the Zed process
3. Start a new Zed process (fresh binary, connects to same test server)
4. Test server sends `open_thread` for existing thread
5. New Zed loads thread, replays history, sends `agent_ready`
6. Test server sends `chat_message`
7. Verify response is correct, `response_length > 0`

Requires test infrastructure to restart the Zed binary mid-test.

### Phase 13: Helix restart reconnection test
1. Complete a message
2. Restart the Go test server (clear in-memory state, keep DB)
3. Zed reconnects to new server
4. Verify contextMappings rebuilt, waiting interaction re-sent
5. Verify response routing correct

Requires test infrastructure to restart the Go test server mid-test while preserving the DB.

## Related PRs
- #2113: Backend auto-start for design review comments
- #2121: Auto-start in NotifyExternalAgentOfNewInteraction
- #2122: Auto-start in sendCommandToExternalAgent (consolidated)
- #2123: Fix stale response routing after auto-start
- #2124: Fix idle timeout overwriting paused screenshot
- #2125: Fix history replay corruption (open_thread before agent_ready)
