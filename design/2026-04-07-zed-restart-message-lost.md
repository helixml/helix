# Zed Restart: chat_message Sent But message_added Never Received

**Date:** 2026-04-07  
**Session:** `ses_01knmds53hhgbrmx7az8ezm06r`  
**Task:** `spt_01knmdrzrvj775djnc6986hbj4`  
**Interaction stuck:** `int_01knmxs7mspy84ts5r65zh13av` (state: `waiting`, never completed)

---

## Symptom

After the user manually restarted the Zed agent, Helix sent a queued `chat_message`. Zed
made LLM calls and set clipboard content — it was visibly processing — but never emitted a
single `message_added` event back to Helix over the ACP WebSocket. The interaction stayed
stuck in `waiting` state indefinitely, showing a spinner in the UI.

---

## Exact Event Timeline (from API logs)

| Time (UTC) | Event |
|---|---|
| 21:31:28 | Previous interaction `int_01knmxp3jjnbpw6acfbtrqhzs7` completed (message_ids 98–105, `message_completed` received) |
| 21:32:22–26 | Zed WebSocket gone; API retries `open_thread` command 5 times, all fail ("no WebSocket connection") |
| **21:32:28** | Zed ACP WebSocket reconnects |
| 21:32:28 | `pickupWaitingInteraction()` called — **finds zero waiting interactions** (previous was already complete), returns immediately |
| 21:32:28 | `open_thread(acp_thread_id=8d7d839b-7349-4a2b-a448-90f3cca27446, agent_name=claude)` written directly to new WebSocket |
| 21:32:30 | Zed attempts helixos MCP — **fails** (`MCP server 'helixos' not configured for this agent`) × 2 |
| 21:32:33 | `agent_ready` received from Zed |
| **21:32:39** | `processPromptQueue` dispatches queued prompt "Try again now that I've restarted you to use Chrome MCP." |
| 21:32:39 | New interaction `int_01knmxs7mspy84ts5r65zh13av` created in `waiting` state |
| 21:32:39 | `requestToSessionMapping[int_01knmxs7...]` and `requestToInteractionMapping[int_01knmxs7...]` both stored |
| 21:32:39 | `chat_message(acp_thread_id=8d7d839b-..., request_id=int_01knmxs7..., agent_name=claude, first_message=false)` sent |
| 21:32:45–21:33:06 | **6–7 LLM calls via Anthropic proxy** (prompt_tokens 1–3 each — see below) |
| 21:33:03, 21:33:21 | Clipboard set in sandbox via RevDial (Zed using MCP tools) |
| 21:35:38 | Video stream WebSocket closed |
| (ever after) | **Zero `message_added` events** received on ACP WebSocket |

---

## What Helix Sent

### `open_thread` payload (line 390, `websocket_external_agent_sync.go`)
```json
{
  "type": "open_thread",
  "data": {
    "acp_thread_id": "8d7d839b-7349-4a2b-a448-90f3cca27446",
    "agent_name": "claude",
    "session_id": "ses_01knmds53hhgbrmx7az8ezm06r"
  }
}
```

### `chat_message` payload (line 2578, `websocket_external_agent_sync.go`)
```json
{
  "type": "chat_message",
  "data": {
    "acp_thread_id": "8d7d839b-7349-4a2b-a448-90f3cca27446",
    "message": "Try again now that I've restarted you to use Chrome MCP...",
    "request_id": "int_01knmxs7mspy84ts5r65zh13av",
    "agent_name": "claude",
    "from_queue": true
  }
}
```

`first_message=false` because `session.Metadata.ZedThreadID` was already set (line 2593).
`acp_thread_id` is the pre-existing thread from before the restart.

---

## Helix-Side State Was Correct

Confirmed from code + DB:

- `contextMappings[8d7d839b-...]` = `ses_01knmds53hhgbrmx7az8ezm06r` — **restored** at line
  343 during reconnect handler, using `session.Metadata.ZedThreadID` from DB.
- `requestToSessionMapping[int_01knmxs7...]` = `ses_01knmds53hhgbrmx7az8ezm06r` — **set**
  at line 2563 when interaction was created.
- `requestToInteractionMapping[int_01knmxs7...]` = `int_01knmxs7mspy84ts5r65zh13av` — **set**
  at line 2568.
- `int_01knmxs7mspy84ts5r65zh13av` in `waiting` state in DB — fallback lookup in
  `getOrCreateStreamingContext()` (lines 1414–1421) **would have found it** if `message_added`
  had arrived.

**Conclusion: the Helix side is not the bug.** If any `message_added` event had arrived,
it would have been correctly routed and the interaction would have progressed.

---

## Root Cause: Zed Side

Zed is visibly processing (LLM calls, clipboard operations) but not emitting `message_added`
events. There are no "Failed to process sync message" errors in the Helix logs and no silent
drops in `processExternalAgentSyncMessage` — the events simply never arrive.

### Hypothesis: `open_thread` on a restarted Zed doesn't rewire Claude Code's output callback

Zed has three internal components that need to be connected:

1. **ACP WebSocket connection** to Helix (for receiving `chat_message`, sending `message_added`)
2. **Claude Code process/session** (the AI agent doing the work)
3. **Output routing hook**: "when Claude Code produces output → emit `message_added` on ACP WS"

When Zed runs normally, all three are wired together from the start. After a restart:
- (1) reconnects successfully (we see "External agent WebSocket connected")
- (2) restarts and makes LLM calls (confirmed via Anthropic proxy logs)
- **(3) is NOT rewired** — Claude Code's output has nowhere to go

When Zed receives `open_thread` for a thread it no longer has in memory, it likely:
- Creates a new in-memory thread context with the given ID
- Passes the subsequent `chat_message` to a fresh Claude Code session
- Claude Code processes and produces output
- But the subscription/callback that would forward that output as `message_added` to Helix is
  not established for this "reopened" thread

The `open_thread` command may be designed for the case where Zed is still running and just
needs to switch thread context — not for the case of a full restart where the entire
Claude Code ↔ ACP event pipeline needs rebuilding from scratch.

### Why the LLM Calls Are Tiny (1–3 prompt tokens)

The prompt token counts of 1–3 are physically impossible for a real conversation turn. These
are almost certainly **tool result continuation calls** — short messages to the LLM like
`[tool result: ...]` with minimal context, or possibly health-check/echo calls. This suggests
Zed is running Claude Code but in a degraded mode, possibly with a truncated or empty context,
rather than a full conversation with thread history.

This is consistent with the `open_thread` → fresh Claude Code session hypothesis: Zed starts
Claude Code fresh with no history, Claude Code gets a very short message and produces output,
but the output routing back through ACP is broken.

### Alternative Hypothesis: helixos MCP failure broke thread loading

At 21:32:30 (2 seconds after reconnect, before `agent_ready`), Zed tried to use the
`helixos` MCP server and failed twice. If `open_thread` processing depends on an MCP call
to load thread state, a failure here could leave the thread in a partially-initialised state
where Zed believes it's ready (`agent_ready` at 21:32:33) but the thread context is broken.

---

## What We Need to Diagnose Further

1. **Zed process logs** from inside the container during 21:32:28–21:33:06:
   ```bash
   docker compose exec -T sandbox-nvidia docker logs ubuntu-external-01knmds53hhgbrmx7az8ezm06r 2>&1 | grep -v "desktop-bridge\|PIPEWIRE\|FRAME\|SHARED_VIDEO"
   ```
   We need to see Zed's own stdout/stderr — did it log an error processing `open_thread`
   or `chat_message`? Did it see Claude Code's output?

2. **Zed source code** (`~/pm/zed`): how does `open_thread` behave when Zed has restarted
   and the thread is not in memory? Specifically:
   - `crates/external_websocket_sync/` — the ACP protocol handler
   - Does `open_thread` establish a new Claude Code session and wire up `message_added`
     emission, or does it assume the session already exists?

3. **Add logging to Helix** on ACP WebSocket receive:
   - Log every incoming event at DEBUG level (currently only logged at TRACE)
   - Specifically: confirm whether ANY bytes arrive from Zed after 21:32:39

4. **Does Zed send `thread_created` after receiving `open_thread`?**  
   If yes, we should see it in the logs (handled by `handleThreadCreated`). We don't. This
   means Zed is NOT sending `thread_created` — either it accepted `open_thread` silently
   and started working, or it failed silently.

---

## Impact

Any time a user manually restarts the Zed agent while the session has an existing thread
(`ZedThreadID` is set), the next message after reconnect will be silently lost. Zed processes
it but the response never reaches Helix. The interaction stays in `waiting` state forever.

This is **distinct** from the case where Zed restarts with no existing thread (first message
after a session start), which works correctly because `acp_thread_id` is empty, forcing Zed
to create a fresh thread with full event wiring.

---

## Fix Applied

**File**: `~/pm/zed/crates/external_websocket_sync/src/thread_service.rs`  
**Commit**: `47950a9cf8` on branch `fix/agent-type-serialization-and-debug-logging`

Added `ensure_thread_subscription()` call in `open_existing_thread_sync()` immediately
before `send_agent_ready()`, mirroring what `create_new_thread_sync()` does at line 1218.

```rust
// CRITICAL: Subscribe to thread events so streaming responses sync to Helix.
cx.update(|cx| {
    ensure_thread_subscription(&thread_entity, &acp_thread_id, cx);
});
```

This rewires the `NewEntry` / `EntryUpdated` / `Stopped` event subscriptions on the loaded
thread entity so Claude Code's output is forwarded as `message_added` events to Helix.

---

## Potential Fixes (historical — superseded by fix above)

### Option A: Helix — Force fresh thread on post-restart send (defensive)

When Helix sends `chat_message` after a reconnect where `open_thread` was used, clear
`session.Metadata.ZedThreadID` from the command so `acp_thread_id` is empty. This forces
Zed to create a new thread with proper event wiring, at the cost of losing thread history
in Zed (Helix has the full history so this is recoverable).

Risk: Zed creates a new thread, sends `thread_created` with a new ID, Helix updates the
`ZedThreadID` — this should work but loses thread continuity in Zed.

### Option B: Zed — Fix `open_thread` to properly rewire Claude Code output

In Zed's ACP handler, when `open_thread` is received for a thread not in memory (post-restart),
ensure a new Claude Code session is created AND its output is properly subscribed to emit
`message_added` events. This is the correct fix but requires Zed changes.

### Option C: Helix — Timeout + retry with fresh thread

If no `message_added` arrives within N seconds of sending a `chat_message` with an existing
`acp_thread_id`, Helix closes the interaction, clears `ZedThreadID`, and re-queues the
message. This causes a retry with `acp_thread_id=nil` (fresh thread), which should work.

### Option D: Helix — Send fresh-thread chat_message on reconnect

In the reconnect path (line 363+), after `pickupWaitingInteraction`, always clear
`ZedThreadID` from the local command payload (not from DB) when sending post-reconnect.
Helix keeps the mapping for routing but Zed gets a clean slate.

---

## Files

- `api/pkg/server/websocket_external_agent_sync.go` — reconnect handler (line 328), `pickupWaitingInteraction` (line 432), `processPromptQueue` (line ~2520), `open_thread` send (line 390), `handleMessageAdded` (line 969)
- `api/pkg/server/session_handlers.go` — `open_thread` retry loop (line 2028)
