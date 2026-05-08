# Production Incidents — 2026-04-02

**Date**: 2026-04-02
**Reporter**: Luke Marsden (investigated via `gcloud compute ssh` and `ssh root@code.helix.ml`)

---

## Incident 1: "Message the Zed Agent" Display Despite Claude Code Configuration

**Affected**: `spt_01kn7sm0g9v0xnbt2saznxqpgy` / `ses_01kn7sm27nh00cfkbbx5kvmj9m`
**Project**: `prj_01kn7skwmgrt7m0f1vfv5sk1jq`

### Symptoms
User reports that when creating a new spectask configured to use Claude Code with the Anthropic API, Zed IDE shows "Message the Zed Agent" instead of "Message Claude Code" in the agent panel.

### Investigation

**DB state is correct:**
- Session `config.zed_agent_name = "claude"` ✓
- App `code_agent_runtime = "claude_code"`, `code_agent_credential_type = "api_key"` ✓
- `settings.json` inside container has `agent_servers.claude-acp` with `type: "registry"` ✓
- Settings-sync-daemon log confirms: `Using claude_code runtime (API key mode): base_url=https://app.helix.ml` ✓

**Claude Code ACP IS running in the production container** (`ubuntu-external-01kn7sm27nh00cfkbbx5kvmj9m` inside `helix-sandbox-app`):
```
ps aux output:
  npm exec @agentclientprotocol/claude-agent-acp@0.24.2
  node .../claude-agent-acp
  claude    ← the actual Claude Code CLI process
```

**Container logs confirm Claude Code created the thread:**
```
[THREAD_SERVICE] Creating ACP thread with agent: Some("claude")
agent stderr: Error handling request { method: 'authenticate', params: { methodId: 'claude-login' } }
  { code: -32603, message: 'Internal error', data: { details: 'Method not implemented.' } }
[THREAD_SERVICE] Created ACP thread: a711b3c3-9bde-434a-87fc-d799b1cd5d2d
```

The `claude-login` authentication error is non-fatal — thread creation succeeds and Claude Code processes messages. Responses include `<thinking>` tags and Claude-style tool calls, confirming it IS Claude Code, not the Zed Agent.

**Env comparison**: Both production (`app.helix.ml`) and local (`meta.helix.ml`) have identical Anthropic/Vertex config. Both route through Vertex AI. The `helix-ubuntu` image is `2.9.18` on both. The Zed registry at `cdn.agentclientprotocol.com` is reachable from inside the container (verified with curl, returns 200).

### Root Cause

**Not yet determined.** Claude Code ACP is confirmed running and processing messages in this container. The "Message the Zed Agent" display text may be:
1. A transient state before the registry fetch completed
2. A UI-only issue where the display label doesn't update after Claude Code connects
3. Something else entirely — needs reproduction with the user watching the Zed UI

### Status
**Needs further investigation** — Claude Code IS running, so the issue is either cosmetic or was transient.

---

## Incident 2: FIFO Queue Desynchronization + PostgreSQL Unicode Error

**Affected**: `spt_01kknzkhfs8zjtkqr35dychcb9` / `ses_01kknzkjvksxnrwwf981dk2g8x`
**Project**: `prj_01khv0gnntbj5xbch709cbmmgs`

### Symptoms
1. Session UI shows "Incomplete interaction — the agent may have disconnected before finishing" with spinner
2. Two interactions stuck in `state=waiting` with empty responses forever
3. User's follow-up message "huh" never reaches the agent
4. Zed thread sqlite shows work completed correctly
5. Spectask status is `pull_request` — the agent actually did the work

### DB State
```
int_01kknzkjvpabrbpbwh2a234gkp | complete | 140521 bytes  (planning phase)
int_01kn833q2trr4we4whkst81tvx | complete | 168284 bytes  (implementation #1)
int_01kn833r9ctxy630r349hr3gwe | complete |   2456 bytes  (implementation #2)
int_01kn8341wt36xrxg1jpn5bd9zs | waiting  |      0 bytes  ← STUCK
int_01kn83cz3hrq9zad6bqhajb89t | waiting  |      0 bytes  ← STUCK
```

### Root Cause: FIFO Queue Pops Don't Match request_id

The FIFO queue (`sessionToWaitingInteraction`) pops **blindly from the front** on `message_completed`, ignoring the `request_id` in the completion event. When multiple messages are sent rapidly and the agent processes them out of order, the queue desynchronizes.

**Timeline:**

| Time | Event | Detail |
|------|-------|--------|
| 21:55:47 | Queue position 0 | `int_01kn833r9ctxy630r349hr3gwe` created, request_id mapping set |
| 21:55:49 | Queue position 1 | `int_01kn8341wt36xrxg1jpn5bd9zs` sent (impl prompt, 2s later) |
| 21:57:31 | **message_completed** | `request_id=req_c89ea00b` (for `int_01kn8341wt36xrxg1jpn5bd9zs`) |
| | **BUG →** | Queue pops `int_01kn833q2trr4we4whkst81tvx` (WRONG — popped the first interaction, not the one that completed) |
| 21:57:43 | **message_completed** | `request_id=int_01kn833r9ctxy630r349hr3gwe` |
| | Queue pops | `int_01kn833r9ctxy630r349hr3gwe` (remaining_queue=2) |
| 22:00:41 | Queue position 2 | `int_01kn83cz3hrq9zad6bqhajb89t` sent (approve message) |
| 22:00:43 | **New streaming context** | Created for `int_01kn833r9ctxy630r349hr3gwe` ← **already completed!** |
| 22:00:43-54 | **104× 22P05 errors** | `message_added` events routed to wrong interaction, DB writes fail |
| 22:00:54 | **message_completed** | Pops `int_01kn833r9ctxy630r349hr3gwe` AGAIN (already completed), remaining_queue=2 |

**The FIFO queue assumes messages complete in insertion order.** When the agent completes message #3 before message #1 finishes streaming, the blind pop desynchronizes the entire queue. All subsequent interactions get routed to wrong targets.

### Secondary Issue: PostgreSQL 22P05 Unicode Error

The 104 `unsupported Unicode escape sequence (SQLSTATE 22P05)` errors are a **secondary symptom**, not the root cause. They occurred because:

1. The approve message's response was routed to the already-completed `int_01kn833r9ctxy630r349hr3gwe`
2. A new streaming context was created from DB state, loading the interaction with its existing `response_entries`
3. Zed's flush resent ALL thread entries (including 168K+ of accumulated content from the entire thread history)
4. Somewhere in that accumulated content was a character that PostgreSQL's jsonb parser rejects
5. The specific character was NOT a null byte in the websocket content (verified by scanning Zed logs) — it was likely introduced during JSON re-serialization of the accumulated entries in the `response_entries` jsonb column

**What the character actually was:** Unknown. The content in the Zed websocket log is clean. The content that eventually made it to the DB (2456 bytes for `int_01kn833r9ctxy630r349hr3gwe`) is clean. The problematic content was in the in-memory accumulator state during the misrouted streaming — content that was never successfully written to the DB and is now lost.

### Why the Two Interactions Are Stuck

`int_01kn8341wt36xrxg1jpn5bd9zs` and `int_01kn83cz3hrq9zad6bqhajb89t` are stuck in `waiting` with empty responses because:

1. Their `message_completed` events were consumed by queue pops for DIFFERENT interactions
2. The streaming context was pointing at the wrong interaction, so their response content was never captured
3. No more `message_completed` events will arrive — the agent has moved on

### Fix Required

**Primary fix:** `handleMessageCompleted` must match the `request_id` from the completion event to the correct interaction in the FIFO queue, not blindly pop from the front. The queue should search for the matching interaction and remove it by value, not by position.

**Code location:** `websocket_external_agent_sync.go` around line 2140-2160 where the queue is popped.

**Defense-in-depth:** Also sanitize content before DB writes (the `sanitizeForPostgres` change already made in `accumulator.go`) to prevent 22P05 errors from making debugging harder in future incidents.

### How Multiple Messages Got Queued So Fast

Three implementation-phase prompts were sent within 2 seconds (21:55:47 to 21:55:49). This likely happened because:
- The spectask was resumed/restarted
- The `sendQueuedPromptToSession` or `spec_task_design_review_handlers` sent multiple messages in rapid succession
- Each created a separate interaction and queued it

The agent processed the later message first (perhaps because it was simpler or because ACP threading caused reordering), sending its `message_completed` before the first message's streaming had finished.

### Status
**Root cause identified.** Fix needed in `handleMessageCompleted` to match by `request_id` instead of blind FIFO pop.

---

## Incident 3: "Poisoned Exclusion Set" — All Responses content_length=0

**Affected**: `spt_01kkpk3x3ch61yvzdf0yv2xd9e` / `ses_01kkr75mrdb1s0mvysgc8rm3m6`
**Date**: 2026-04-02 (discovered during spectask debugging)

### Symptoms

All `message_added` events for the session show `content_length=0` in the streaming context. Final response is empty. This affects EVERY interaction in the session, not just one.

### Root Cause: Poisoned ExcludedMessageIDs from History-Replay Accumulation

The session had 10+ interactions across multiple container restarts. One interaction (`int_01kn34yx22t8sycygmw8hd9bzw`, ran 2026-03-31) ran BEFORE the clear-on-reconnect fix (PR #2127) was merged. Without that fix, Zed replayed ALL thread history (entries 1-274) as `message_added` events during reconnect. Since no streaming context was cleared, those 274 entries accumulated into `int_01kn34yx22t8sycygmw8hd9bzw`'s `response_entries`.

This poisoned the `ExcludedMessageIDs` set for all future interactions:
- `collectExcludedMessageIDs` collects message_ids from ALL completed interactions
- `int_01kn34yx22t8sycygmw8hd9bzw`'s bloated `response_entries` covers IDs 1–274
- The union of all historical IDs covers essentially every possible entry index
- New responses with IDs 143–148 were silently dropped (all in excluded set)

### The Fundamental Design Flaw

Using `response_entries` (which can be corrupted by history-replay accumulation) to build the exclusion set is fragile. If ANY historical interaction has over-accumulated entries, it permanently poisons the session.

The `flushAndClearStreamingContext` on reconnect (PR #2127) clears the old streaming context, but a NEW context is immediately built from DB data that includes the poisoned `response_entries`.

### Fix: Numeric Threshold Instead of ID Set

**Changed `collectExcludedMessageIDs` → `collectExcludedMessageIDThreshold`** (`websocket_external_agent_sync.go`):

Old: Build `map[string]bool` from response_entries of ALL completed interactions
New: Get `last_zed_message_id` (integer) of the IMMEDIATELY PRECEDING interaction and use it as a numeric threshold

**Changed `MessageAccumulator`** (`accumulator.go`):
Added `MaxExcludedMessageID int` field. In `AddMessageWithToolInfo`: if messageID (parsed as int) ≤ threshold → drop silently.

**Why this works**:
- `int_01kn438sx96mp1pdagmw7gakmd` (preceding interaction): `last_zed_message_id = 102`
- Threshold = 102
- History replay 1–102: excluded ✓
- New response 143–148: 143 > 102, NOT excluded ✓
- Immune to poisoning from `int_01kn34yx22t8sycygmw8hd9bzw`'s bloated response_entries

**Trade-off**: Entries 103–142 (if they exist in the ACP thread between the threshold and the new response) would be accepted. In practice these are rarely present and represent at most cosmetic stale content, not data loss.

### Status
**Fixed** in `accumulator.go` + `websocket_external_agent_sync.go`. All wsprotocol tests pass.
