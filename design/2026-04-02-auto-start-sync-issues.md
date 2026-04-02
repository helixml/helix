# Auto-Start Dev Container: WebSocket Sync Issues After Reconnect

**Date**: 2026-04-02
**Context**: Testing auto-start of dev containers when messages are sent to stopped sessions (PRs #2113, #2121, #2122, #2123, #2124)

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

When Zed opens an existing thread (`open_thread`), it replays ALL historical messages as `message_added` events. These are indistinguishable from new streaming content. `handleMessageAdded` routes them to the current waiting interaction (`int_01kn7g9849gekkc13jrparx1a2`), overwriting its `response_entries` with empty content from old messages.

The message IDs jump (9, 10, 11, 19, 22, 24, 26, 28, 30, 44, 50...) — these are clearly historical messages being replayed, not a streaming response. But there's no way for the API to distinguish replay from new content.

**Root cause detail**: `pickupWaitingInteraction` queues both `open_thread` AND `chat_message` together (both sent immediately after `agent_ready`). But Zed needs to finish loading the thread and replaying history before the new `chat_message` is processed. The history replay `message_added` events arrive while the interaction is already set up, corrupting it.

**Fix options:**
- a) **Delay `agent_ready` until thread is loaded (recommended)**: Currently `agent_ready` fires when Zed is ready to receive commands, BEFORE the thread is opened. The API then sends `open_thread` + `chat_message` together after `agent_ready`, so the history replay corrupts the chat interaction. Fix: send `open_thread` immediately on WebSocket connect (before `agent_ready` gate), have Zed delay `agent_ready` until the thread is fully loaded and history replayed. The existing `agent_ready` gate then naturally holds `chat_message` until replay is done. No new event types needed — just reordering.
- b) **New `thread_opened` handshake**: Zed sends a `thread_opened` event after it finishes loading and replaying thread history. The API gates `chat_message` delivery on this event. More protocol complexity but more explicit.
- c) **Track last-seen message_id per thread**: Ignore `message_added` events with `message_id <= last_seen_id`. On reconnect, only process messages with higher IDs.
- d) **API-side heuristic**: After sending `open_thread`, ignore `message_added` events for ~2s. Fragile/hacky.

### Issue 2: Prompt Text Contaminated with CLI Output

**Severity: Medium**

The "carry on 3" message was delivered with XML prefix:
```
<command-name>/model</command-name>
<command-message>model</command-message>
<command-args>default</command-args>
<local-command-stdout>Set model to claude-sonnet-4-6</local-command-stdout>carry on 3
```

The prompt history sync captures local CLI command output along with the user's message. This is a frontend bug — the prompt text should be the user's message only, not the full terminal buffer.

**Fix**: Frontend should strip `<command-name>`, `<local-command-stdout>`, and similar tags from the prompt content before syncing to the prompt history API.

### Issue 3: User Messages Typed Directly in Zed Don't Sync to Helix

**Severity: Medium**

When the user typed "you there?" directly in Zed (not via the Helix UI), the agent responded ("Yep, here!") but the interaction was never created in Helix. The Zed-side `message_added` and `message_completed` events were either not sent, or sent with a context/thread that Helix didn't have a mapping for.

This is expected behaviour for now (Zed-side messages bypass the Helix prompt queue), but it means the Helix session view is incomplete. After reconnect, any Zed-side interactions are invisible to Helix.

**Fix options:**
- a) On reconnect, sync the full thread state from Zed → Helix (reconciliation)
- b) Treat any `message_completed` without a matching interaction as a new interaction (auto-create)

### Issue 4: Multiple WebSocket Reconnections

**Severity: Low**

Between 16:27:28 and 16:29:04, three WebSocket connections were established for the same session. Container-side logs show "Connection reset without closing handshake" errors. The reconnection loop causes repeated `open_thread` → history replay cycles, compounding Issue 1.

**Fix**: Investigate why the WebSocket connection is unstable after auto-start. May be related to the sandbox reconnect at 16:27:28 racing with the initial connection at 16:26:52.

## Chosen Fix: Delay `agent_ready` until thread is loaded (Option a)

### Design

The existing `agent_ready` gate already holds `chat_message` delivery. The fix reorders
what happens on each side of the gate:

**Before (broken):**
1. WebSocket connects
2. Zed sends `agent_ready` (immediately, Zed process is ready)
3. `pickupWaitingInteraction` sends `open_thread` + `chat_message` together
4. Zed opens thread → replays history → `message_added` events corrupt the interaction
5. `message_completed` → empty response

**After (fixed):**
1. WebSocket connects
2. API sends `open_thread` immediately (before `agent_ready` gate) — only if session has `ZedThreadID`
3. Zed receives `open_thread`, loads thread, replays history
4. Zed sends `agent_ready` only after thread loading is complete
5. API receives `agent_ready`, `pickupWaitingInteraction` sends `chat_message` only (no `open_thread`)
6. Agent processes `chat_message` → real response streams correctly

**Fresh start (no thread):**
1. WebSocket connects
2. No `open_thread` sent (no `ZedThreadID`)
3. Zed sends `agent_ready` immediately (nothing to load)
4. `pickupWaitingInteraction` sends `chat_message` → triggers `thread_created`

### Changes Required

**Helix API (Go):**
- On WebSocket connect, BEFORE calling `pickupWaitingInteraction`: if session has `ZedThreadID`, send `open_thread` command immediately
- In `pickupWaitingInteraction`: skip sending `open_thread` (it was already sent on connect)
- Ignore `message_added` events that arrive before `agent_ready` (thread history replay)

**Zed (Rust):**
- When `open_thread` arrives: load the thread, replay history, THEN send `agent_ready`
- If no `open_thread` arrives before the agent is otherwise ready: send `agent_ready` as normal
- Requires updating `sandbox-versions.txt` with the new Zed commit

## Priority

Issue 1 (thread history replay) is the most critical — it causes data loss (empty response). Without fixing it, the auto-start feature is unreliable because every reconnect triggers a history replay that corrupts the current interaction.

## Related PRs
- #2113: Backend auto-start for design review comments
- #2121: Auto-start in NotifyExternalAgentOfNewInteraction
- #2122: Auto-start in sendCommandToExternalAgent (consolidated)
- #2123: Fix stale response routing after auto-start
- #2124: Fix idle timeout overwriting paused screenshot
