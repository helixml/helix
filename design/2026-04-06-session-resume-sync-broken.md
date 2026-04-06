# Session Resume: WebSocket Sync Broken After Container Restart

**Date:** 2026-04-06
**Session:** `ses_01knd14n2f7q7gq2y7d029b29a` (spectask `spt_01knd14jw9707mmet33hmzr850`)
**Task:** "Build a training module for how to use helix"

## Summary

After a container restart (auto-expiry), the Zed-to-Helix WebSocket sync completely breaks. The user can chat with claude-acp in Zed's agent panel normally, but **zero `message_added` events** are sent to the Helix server. The Helix chat view appears frozen/disconnected from the Zed thread.

Additionally, a rogue thread is auto-created on startup that runs on Zed's built-in agent instead of claude-acp.

## Exact Timeline (all times UTC unless noted)

### Previous run (Apr 4-5) — working normally

- **Apr 4 19:57** — Session created, thread `689dbabf-b498-41c0-9a1c-b834ddb59f84` created in claude-acp
- **Apr 4-5** — 671 `message_added` events sent from Zed to Helix server, all syncing correctly
- **Apr 5 17:29** — Last interaction recorded in Helix DB (`int_01knfas67tqhepjjswg7ahp6s8`)
- Session auto-expires, container stopped

### Container restart (Apr 6) — sync breaks

**13:46:18** — User clicks Resume in Helix UI
```
Resume session request — session_id=ses_01knd14n2f7q7gq2y7d029b29a
```

**13:46:19** — New container created: `ubuntu-external-01knd14n2f7q7gq2y7d029b29a`

**13:46:33** — Zed starts inside container

**13:46:34** — **BUG 1: Thread restoration fails**
```
WARN  [agent_ui::agent_panel] last active thread 689dbabf not found in database, skipping restoration
```
Thread `689dbabf` was never saved to Zed's sqlite threads.db. Only claude-acp's `.claude-state` has the conversation.

**13:46:34** — **BUG 2: Rogue thread created before WebSocket ready**
```
ERROR [agent_ui::conversation_view] Failed to send UserCreatedThread WebSocket event: WebSocket service not initialized
```
AgentPanel auto-creates new thread `f9113455-8961-4c92-a0f3-8b04c080ac19` using Zed's **built-in** agent (not claude-acp). The creation event fails because WebSocket isn't connected yet. Server never learns about this thread.

**13:46:34** — WebSocket connects to `ws://api:8080/api/v1/external-agents/sync?session_id=ses_01knd14n2f7q7gq2y7d029b29a`

**13:46:34** — Server sends `open_thread` for `689dbabf` (sent twice due to race, second deduplicated by load lock)
```
[CONNECT] Sending open_thread directly on new connection before agent_ready gate
```

**13:46:34** — Thread service begins loading thread from claude-acp agent
```
Selected agent: Custom { name: "claude-acp", command: AgentServerCommand { path: "", args: [], env: None } }
```

**13:46:36** — Connected to claude-acp agent server, calling `load_session()`

**13:46:42** — Thread loaded from claude-acp's persistent storage (`689dbabf.jsonl`, 13.8MB)
```
✅ Loaded ACP thread from agent: 689dbabf (session_id)
📋 Registered thread: 689dbabf → agent session: 689dbabf
```

**13:46:42** — `ensure_thread_subscription()` called — subscribes to `Entity<AcpThread>` for `NewEntry`/`EntryUpdated`/`Stopped` events. `.detach()` called on subscription.

**13:46:42** — `agent_ready` sent to server (this is the LAST successful sync event)
```
📤 Sending JSON: {"event_type":"agent_ready","data":{"agent_name":"claude","thread_id":"689dbabf"}}
```

**13:46:42** — `notify_thread_display()` called — AgentPanel creates `ConversationView::from_existing_thread` using the same `Entity<AcpThread>`. The user now sees the claude-acp thread in Zed's agent panel.

**13:46:42** — **BUG 3: Rogue thread receives prompt and title-generates**
```
📤 Sending JSON: {"event_type":"thread_title_changed","data":{"acp_thread_id":"f9113455","title":"show me in chrome"}}
[agent] Received prompt request for session: f9113455
```
The rogue thread `f9113455` auto-ran a prompt and generated title "Building Interactive Tower Defense Game App" — completely unrelated to the actual task.

**13:46:42** — Server correctly warns:
```
⚠️ Thread title changed but no Helix session found for thread f9113455
```

### User interaction (Apr 6, 13:46:50+) — messages flow but don't sync

**13:46:50** — User sends "." in Zed agent panel (testing)

**13:46:56** — claude-acp responds: "The Learn module system is now fully functional..."
- This is a **stale response** from the previous session's context. When `load_session()` loaded the 13.8MB JSONL, it restored the full conversation history. Claude's first response after resumption summarized where it left off, NOT acknowledging the "." input.
- **This is the "swallowed message"** the user reported — the "." was consumed as a prompt but the response was contextually from the previous session.

**13:47:14** — User sends "open it in chrome"

**13:47:18-25** — claude-acp responds normally (takes screenshot, opens Chrome)

**13:47:46** — User sends "the startup script is still loading, can you see it?"

**13:49:18** — User sends "log in using test creds"

**13:49:44** — claude-acp responds with credentials (last activity in JSONL)

### What the server sees

- **Zero `message_added` events** received after `agent_ready`
- **Zero new interactions** created in Postgres for this session today
- The Helix chat view shows the last interaction from Apr 5 17:29
- PR creation loop runs every 30s, failing because no commits on `feature/001711-build-a-training-module`

## Root Cause Analysis

### BUG 1: Thread not in Zed's sqlite DB

Thread `689dbabf` exists in claude-acp's persistent storage (`.claude-state/projects/-home-retro-work/689dbabf.jsonl`) but NOT in Zed's threads.db (`.zed-state/local-share/threads/threads.db`).

**Evidence:** `sqlite3 threads.db "SELECT id FROM threads"` returns only `f9113455` (the rogue thread).

**Likely cause:** The `load_session()` → `load_thread_from_agent()` code path creates an `Entity<AcpThread>` in memory but never persists it to Zed's sqlite ThreadStore. Only threads created through the normal UI flow get saved to the DB.

### BUG 2: Rogue thread created before WebSocket

The AgentPanel's startup sequence tries to restore the last active thread. When it fails (BUG 1), it creates a new default thread. This happens before the WebSocket service is initialized, so the `UserCreatedThread` event is lost.

**Impact:** Rogue thread `f9113455` receives a prompt from somewhere (possibly auto-prompt from startup script context or stale state) and runs on Zed's built-in Claude agent, producing nonsensical content.

### BUG 3: Subscription doesn't fire after `load_session()`

This is the **critical bug**. `ensure_thread_subscription()` at `thread_service.rs:426` subscribes to the thread entity for `AcpThreadEvent::NewEntry/EntryUpdated/Stopped`. The subscription IS created (verified: no "already has persistent subscription" skip message in logs). But it **never fires**.

**Evidence:**
- 0 `message_added` events sent after startup (only `agent_ready` + `thread_title_changed` for rogue thread)
- 0 `send_websocket_event()` calls after startup (its eprintln logging never appears)
- User's messages DO reach claude-acp and get responses (verified in 689dbabf.jsonl)
- The `Entity<AcpThread>` IS the same one used by ConversationView (verified: `from_existing_thread` receives `notification.thread_entity.clone()`)

**Hypotheses:**

1. **Subscription scope issue**: `ensure_thread_subscription` is called inside `cx.update(|cx| { ... })` from an async context in `load_thread_from_agent`. The `cx.subscribe().detach()` pattern may not work correctly when `cx` is an `&mut App` obtained via `AsyncApp::update()` — the subscription might be scoped to the update closure and dropped when it returns.

2. **Entity not emitting events**: The `Entity<AcpThread>` loaded via `load_session()` might not emit GPUI events (`cx.emit(AcpThreadEvent::NewEntry)`) the same way as a freshly created thread. The ACP thread module (`acp_thread.rs:2147`) is already erroring with "failed to get old checkpoint — No such file or directory" which suggests the thread is in a partially broken state.

3. **Race between subscription and first event**: The subscription is created at the same time as the thread is being displayed and the user can start typing. If an event fires before the subscription is fully registered in the GPUI event system, it would be lost.

## State at Investigation Time

```
threads.db:     f9113455 only (rogue thread, "Building Interactive Tower Defense Game App")
claude-acp:     689dbabf.jsonl (13.8MB, correct thread, last activity 13:49 UTC today)
DB interactions: last from Apr 5 17:29 (nothing synced today)
WS events sent:  agent_ready (1), thread_title_changed for f9113455 (2)
WS events NOT sent: message_added (0), message_completed (0)
Helix task status: pull_request (stuck in 30s retry loop, no commits on branch)
```

## BUG 4: Thread goes completely dead after initial load

After the initial `load_session()` and the first few messages (which DID work — ".", "open it in chrome", etc.), the thread stops processing entirely. The user sent a "." message much later and it was **completely swallowed** — never reached claude-acp's JSONL, never generated a Zed log entry. The Zed log stopped writing at 14:49:18 and never resumed despite the Zed process still running (PID active, 4.9% CPU).

The `failed to get old checkpoint` errors at `acp_thread.rs:2147` correlate exactly with the timestamps of user messages:
- 14:47:14 — matches "open it in chrome" timing
- 14:47:46 — matches "startup script still loading" timing  
- 14:49:18 — matches "log in using test creds" timing

After that, silence. The checkpoint errors suggest the AcpThread can't save state properly after `load_session()`, and eventually this causes the thread to stop processing entirely.

## Impact

- User's work in Zed is completely invisible in the Helix chat view
- Session appears stuck/dead from the Helix UI perspective
- Any session that gets auto-expired and resumed will hit this bug
- This affects ALL spectask sessions that use container restart/resume

## Files Involved

| File | Role |
|------|------|
| `zed/crates/external_websocket_sync/src/thread_service.rs:426` | `ensure_thread_subscription()` — creates the subscription that doesn't fire |
| `zed/crates/external_websocket_sync/src/thread_service.rs:1270` | `load_thread_from_agent()` — loads thread from claude-acp, calls ensure_thread_subscription |
| `zed/crates/external_websocket_sync/src/thread_service.rs:1322` | `open_existing_thread_sync()` — entry point for open_thread handling |
| `zed/crates/agent_ui/src/agent_panel.rs:1046` | Thread display callback — creates ConversationView from existing entity |
| `zed/crates/acp_thread/src/acp_thread.rs:2147` | Checkpoint error — "No such file or directory" |
| `helix/api/pkg/server/websocket_external_agent_sync.go:338` | Server-side reconnect handling |
| `helix/api/pkg/server/session_handlers.go:2017` | Server sends open_thread on resume |
