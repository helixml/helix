# Auto-Start Dev Container: WebSocket Sync Issues After Reconnect

**Date**: 2026-04-02 (updated 2026-04-02 22:00)
**Context**: Testing auto-start of dev containers when messages are sent to stopped sessions

## Summary of All Issues Found and Fixed

### Issue 1: Message ID Collision After Container Restart (Critical — PR #2127)

**Root cause**: `collectExcludedMessageIDs` collected message IDs from ALL completed interactions in the session. Message IDs are ACP thread entry indices (1, 2, ... 68) that get reused when a thread is reloaded after container restart. The new response's `message_id` would collide with old entries and be **silently dropped** by the accumulator — causing `response_length=0`.

**Fix**: Clear the streaming context (including `excludedMessageIDs`) on WebSocket reconnect in `handleExternalAgentSync` via `flushAndClearStreamingContext`. The excluded IDs are rebuilt fresh from the current session's interactions, which are all from the current Zed session (not historical ones with recycled IDs).

### Issue 2: Connection Replacement Race / Panic (Critical — PR #2127)

**Root cause**: When Zed reconnects, the old connection handler's `defer unregisterConnection` could close the NEW connection's `SendChan`. `unregisterConnection` looked up by session ID, found the new connection (which had overwritten the old one in the map), and closed its channel.

**Fix**: `unregisterConnection` now takes the connection object and only closes/removes if `current == conn`. Also added a deferred `recover` in `sendCommandToExternalAgent` to handle any remaining race where the channel is closed between `getConnection` and the send.

### Issue 3: Entity Released After Zed Restart (Critical — Zed commit c3418cc)

**Root cause**: `AgentConnection::load_session(self: Rc<Self>, ...)` consumes the `Rc`, dropping the only strong reference to `NativeAgent`. The spawned tasks inside `open_thread`/`load_thread` hold `WeakEntity<NativeAgent>` which then fail with "entity released" when they try to upgrade.

**Fix**: Clone the `Rc<dyn AgentConnection>` before passing to `load_session`, keeping the clone alive (`_connection_keepalive`) until `load_task.await` completes. Applied in both `load_thread_from_agent` and `open_existing_thread_sync`.

### Issue 4: Stale Response After Auto-Start (High — PR #2123)

**Root cause**: When `sendCommandToExternalAgent` failed (no connection), `sendQueuedPromptToSession` had already added the interaction to `sessionToWaitingInteraction`. On agent reconnect, a stale `message_completed` from the agent's previous context was popped and matched to the new interaction — wrong response.

**Fix**: On send failure, remove the interaction from `sessionToWaitingInteraction` and `requestToSessionMapping`. `pickupWaitingInteraction` sets them up fresh after reconnect.

### Issue 5: `isSessionReady` Guard Blocked Real Responses (High — PR #2126)

**Root cause**: The `isSessionReady` guard in `handleMessageAdded` was intended to drop history replay events before `agent_ready` fired. But Zed processes `chat_message` and responds BEFORE its thread service sends `agent_ready`. So the guard blocked the agent's actual response, not just history replay.

**Fix**: Removed the guard entirely. History replay adding stale entries to the DB is cosmetic (not data loss). Blocking real responses is data loss.

### Issue 6: Idle Timeout Overwriting Screenshot (Medium — PR #2124)

**Root cause**: `stopIdleDesktop` loaded session metadata before calling `StopDesktop`. `StopDesktop` saves `PausedScreenshotPath` to DB. Then `stopIdleDesktop` overwrote it with the stale pre-stop copy (empty `PausedScreenshotPath`).

**Fix**: Re-fetch the session after `StopDesktop` so the `terminated_idle` status update preserves the screenshot path.

### Issue 7: Double-Load Race on Thread Open (Low — Zed commits 826d32f, 40a88fd)

**Root cause**: When both Zed's workspace restore and Helix's `open_thread` command try to load the same thread concurrently, both pass the registry check (thread not yet registered), both spawn async load tasks, and the second overwrites the first's entity — orphaning subscriptions.

**Fix**: `THREAD_LOAD_IN_PROGRESS: parking_lot::Mutex<Option<String>>` — single mutex guards concurrent loads. If a load is already in progress, the second request is skipped. Drop guard clears on completion (success or error).

### Issue 8: Prompt Text Contaminated with CLI Output (Medium — not yet fixed)

The prompt "carry on 3" was delivered with `<command-name>/model</command-name>...` XML prefix. Frontend prompt history sync captures terminal output along with the user's message.

**Fix needed**: Frontend should strip `<command-name>`, `<local-command-stdout>`, and similar tags from prompt content before syncing.

---

## Approach That Did NOT Work: `isSessionReady` Guard

The initial approach was to gate `message_added` events on `isSessionReady()`:
- Helix sends `open_thread` before the `agent_ready` gate
- Zed delays `agent_ready` until thread is loaded (5s fallback timer)
- API drops `message_added` events while `isSessionReady() == false`

**Why it failed**: Zed processes the `chat_message` and generates a response BEFORE its thread service finishes and sends `agent_ready`. The real response arrives while `isSessionReady()` is still `false`, so it gets dropped. The guard can't distinguish between history replay and real responses because both arrive before `agent_ready`.

The Zed-side delayed `agent_ready` (5s timer, suppressed on `open_thread`) remains in place as it doesn't cause harm, but the API-side guard was removed.

---

## What Actually Fixed the Response Issue

The real reason responses were empty after restart was **NOT** history replay corruption. It was the `excludedMessageIDs` mechanism:

1. Previous completed interactions had entries with `message_id` values like "1", "2", ... "68"
2. After Zed restart, ACP thread entry indices reset and reuse the same numbers
3. The accumulator's `ExcludedMessageIDs` set contained "68" from a previous interaction
4. The new response also had `message_id=68` → silently dropped
5. All `message_added` events showed `content_length=0`
6. `message_completed` arrived with `response_length=0`

The fix: `flushAndClearStreamingContext` on reconnect wipes the stale excluded IDs.

---

## Restart Resilience

### Zed Restart (container stops → auto-start → fresh process)

**What Zed loses**: Thread registry, subscriptions, message ID mappings, streaming throttle state — all process-local statics.

**Recovery**: Helix sends `open_thread` on connect → Zed loads thread from ACP agent DB → `pickupWaitingInteraction` finds waiting interactions → message delivered. The `_connection_keepalive` fix ensures the NativeAgent entity stays alive during `load_session`.

### Helix API Restart (Air hot-reload or crash)

**What Helix loses**: `contextMappings`, `sessionToWaitingInteraction`, `requestToSessionMapping`, `readinessState`, `streamingContexts`.

**Recovery**: `contextMappings` rebuilt from `session.Metadata.ZedThreadID`. `pickupWaitingInteraction` finds waiting interactions from DB. `ResumeCommentQueueProcessing` handles stuck comments.

### Both Restart Simultaneously

Message is re-delivered from DB. Original partial response lost but fresh response generated. Acceptable — user didn't see the original.

---

## State Persistence Summary

| State | Persisted? | Zed Restart | Helix Restart |
|-------|-----------|------------|--------------|
| Session metadata (ZedThreadID) | **YES** (DB) | Rebuilt ✅ | Rebuilt ✅ |
| Waiting interactions | **YES** (DB) | Re-sent ✅ | Re-sent ✅ |
| contextMappings | **Partial** (ZedThreadID) | Rebuilt ✅ | Rebuilt ✅ |
| sessionToWaitingInteraction | **NO** (memory) | Rebuilt from DB ✅ | Rebuilt from DB ✅ |
| streamingContexts | **NO** (memory) | Cleared on reconnect ✅ | Fresh ✅ |
| excludedMessageIDs | **NO** (memory) | Cleared on reconnect ✅ | Fresh ✅ |
| Prompt history entries | **YES** (DB) | Re-processed ✅ | Re-processed ✅ |
| Comment queue (QueuedAt) | **YES** (DB) | Resumed ✅ | Resumed ✅ |

---

## E2E Test Coverage

**Phase 12 (added in PR #2127)**: Kills the Zed process, waits for reconnection, sends `open_thread` + `chat_message` to an existing thread, verifies `message_completed` arrives with non-empty response content in the store. This catches:
- Entity released after restart
- Message ID collision (excludedMessageIDs)
- Connection replacement race
- Thread loading failures

All 12 phases pass with both `zed-agent` and `claude` agents.

---

## Remaining Gaps

### MEDIUM: Prompt history `sending` → `pending` timeout
If a prompt is marked `sending` but Helix crashes before confirming, it's orphaned.

### MEDIUM: `sessionToWaitingInteraction` is append-only
The FIFO queue grows unboundedly. Should clean up on interaction completion.

### LOW: No request_id correlation in `message_added`
Protocol doesn't include `request_id` in `message_added`, so Helix relies on FIFO ordering.

### LOW: Thread history replay adds stale entries to DB
After reconnect, Zed replays old entries which get accumulated. Cosmetic — doesn't cause data loss but bloats `response_entries`.

---

## Related PRs
- #2113: Backend auto-start for design review comments
- #2121: Auto-start in NotifyExternalAgentOfNewInteraction
- #2122: Auto-start in sendCommandToExternalAgent (consolidated)
- #2123: Fix stale response routing after auto-start
- #2124: Fix idle timeout overwriting paused screenshot
- #2125: open_thread before agent_ready, delayed agent_ready, thread load guard
- #2126: Remove isSessionReady guard (blocked real responses)
- #2127: Connection replacement race, entity released fix, Phase 12 E2E test
