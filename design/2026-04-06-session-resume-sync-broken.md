# Session Resume: WebSocket Sync Broken After Container Restart

**Date:** 2026-04-06
**Session:** `ses_01knd14n2f7q7gq2y7d029b29a` (spectask `spt_01knd14jw9707mmet33hmzr850`)
**Task:** "Build a training module for how to use helix"

## Summary

Two independent issues found:

1. **Frontend chat not updating in real-time** (BUG 5) — affects ALL sessions, regression from PR #2146
2. **Zed-to-Helix sync broken after container restart** (BUGs 1-4) — race condition between panel restoration and server's `open_thread` command

## BUG 5: Frontend Chat Not Updating (All Sessions)

### Root Cause

**Regression from PR #2146** ("long sessions get unusably slow"), merged April 5.

PR #2146 switched `EmbeddedSessionView` from `useGetSession` (cache key: `GET_SESSION_QUERY_KEY`) to `useListInteractions` (cache key: `LIST_INTERACTIONS_QUERY_KEY`). But `streaming.tsx` was never updated — it still only invalidated `GET_SESSION_QUERY_KEY`, which nobody reads for rendering anymore.

WebSocket `interaction_update` and `interaction_patch` messages arrive at the browser, but the React Query cache they update is not the one the chat UI reads from. Page refresh works because `useListInteractions` re-fetches from the API.

### Fix (Helix PR)

- Added `["interactions", currentSessionId]` invalidation to the debounced path in `streaming.tsx`
- Only fires for `interaction_update` events (2x per turn), NOT high-frequency `interaction_patch` (already excluded)
- Removed `GET_SESSION_QUERY_KEY` from debounced invalidation (no longer needed for chat, was causing desktop stream "Reconnecting..." flicker via `useSandboxState`)
- Preserved existing session config when `session_update` handler writes to cache (prevents sandbox state stomping)

### Desktop Stream Flicker Fix

The `session_update` WebSocket handler was overwriting the entire session in the React Query cache, including `config.external_agent_status`. When this carried a stale value (e.g. "starting" instead of "running"), `useSandboxState` briefly flipped `isRunning` to false, flashing "Reconnecting..." on the desktop stream. Fixed by preserving the existing config when updating from chat WebSocket events.

## BUGs 1-4: Zed-to-Helix Sync Broken After Container Restart

### What We Observed

After container restart (auto-expiry):
- User chats with claude-acp in Zed agent panel — works locally
- Zero `message_added` WebSocket events sent to Helix server
- Helix chat view shows nothing new
- Sending a message FROM Helix to Zed "fixes" the sync — after that, Zed-side messages work

### Exact Startup Sequence (from logs)

```
13:46:33  Zed starts
13:46:34  Panel restoration: "last active thread 689dbabf not found in database, skipping restoration"
13:46:34  Panel creates rogue thread f9113455 (UserCreatedThread fails - WS not ready)
13:46:34  WebSocket connects
13:46:34  Server sends open_thread for 689dbabf IMMEDIATELY (before agent_ready)
13:46:34  Thread service starts loading 689dbabf from claude-acp
13:46:42  Thread loaded, ensure_thread_subscription called, agent_ready sent
13:46:42  notify_thread_display → from_existing_thread replaces rogue thread
13:46:42  Rogue thread f9113455 auto-generates title "Building Interactive Tower Defense Game App"
13:46:50+ User sends messages in Zed — claude-acp responds, but ZERO sync events
15:40:11  User sends message FROM Helix — sync starts working
```

### Root Cause: Race Condition + Wrong Agent Type Serialization

**Two issues combine to create the race:**

#### 1. `selected_agent_type` not updated on `notify_thread_display`

When the `notify_thread_display` handler in `agent_panel.rs` creates `ConversationView::from_existing_thread`, it correctly sets the `connection_agent` to `Agent::Custom { id: "claude-acp" }`. But it does NOT update `self.selected_agent_type`. When the panel serializes, it saves `agent_type: NativeAgent`.

On next restart, the restoration code sees a native agent, checks sqlite (which doesn't have ACP threads), fails, and gives up — "not found in database, skipping restoration". This forces the panel to `Uninitialized`, which creates a rogue thread when focused.

**The panel restoration SHOULD load the thread directly from claude-acp's persistent storage** (`.claude-state/`), which it does for non-native agents. But because the agent_type was serialized incorrectly, it takes the wrong path.

#### 2. `open_thread` sent before panel restoration completes

The server sends `open_thread` immediately on WebSocket connect, before `agent_ready`. This was intentional (to avoid a different race with `chat_message`). But it means:

1. Panel restoration starts (or fails and creates rogue thread)
2. `open_thread` arrives simultaneously
3. Two parallel loads of the same thread from different code paths
4. Two entities, two subscriptions, two `register_thread` calls
5. Subscription ends up on wrong entity → sync breaks

#### 3. Panel restoration never sent `agent_ready`

The panel restoration path (`initial_state` → `connection.load_session()`) bypasses `thread_service` entirely. It never calls `ensure_thread_subscription` or `send_agent_ready`. Previously this didn't matter because `open_thread` was sent before `agent_ready` anyway. But with the new protocol, `agent_ready` gates the queue.

### Fixes

#### Zed Changes (helixml/zed PR `fix/agent-type-serialization-and-debug-logging`)

1. **Fix `selected_agent_type` serialization** (`agent_panel.rs`): Update `selected_agent_type` in the `notify_thread_display` handler so serialization correctly saves `Custom("claude-acp")` instead of `NativeAgent`.

2. **Wait for WebSocket before panel restoration** (`agent_panel.rs`): Block panel deserialization until the WebSocket connects (up to 10s timeout). This ensures the `agent_ready` → `open_thread` handshake can complete.

3. **Send `agent_ready` from panel restoration** (`conversation_view.rs`): After `initial_state` completes for a resumed thread, call `ensure_thread_subscription` and `send_agent_ready`. This signals the server to flush its readiness queue.

4. **Share `THREAD_LOAD_IN_PROGRESS` lock** (`conversation_view.rs`, `thread_service.rs`): Panel restoration's `initial_state` now acquires the same lock that `open_existing_thread_sync` uses. Only one thread load can be in progress at a time, preventing duplicate entities. Uses a drop guard for automatic cleanup.

5. **Make `ensure_thread_subscription` public** (`thread_service.rs`): So `conversation_view.rs` can call it from the panel restoration path.

6. **Debug logging**: Entity IDs logged on subscription creation/firing and thread registration, to diagnose any remaining issues.

#### Helix Changes (helixml/helix PR `fix/streaming-query-cache-key-mismatch`)

1. **Queue `open_thread` after `agent_ready`** (`websocket_external_agent_sync.go`): Instead of writing `open_thread` directly to the WebSocket on connect, prepend it to the readiness queue. It's sent after `agent_ready`, when Zed's panel restoration has completed.

2. **`prependToReadinessQueue` method**: Ensures `open_thread` arrives before any queued `chat_message` when the queue is flushed.

### New Startup Protocol

```
1. Container starts → Zed launches
2. WebSocket connects (panel restoration WAITS for this)
3. Server: readiness tracking initialized, open_thread QUEUED (not sent)
4. Panel restoration: agent_type correctly deserialized as Custom("claude-acp")
   → initial_state(resume_session_id=689dbabf)
   → acquires THREAD_LOAD_IN_PROGRESS lock
   → connection.load_session(689dbabf) from claude-acp .claude-state
   → Entity created, registered in THREAD_REGISTRY
   → ensure_thread_subscription(Entity A)
   → send_agent_ready("claude-acp", "689dbabf")
   → releases lock
5. Server receives agent_ready → flushes readiness queue:
   → open_thread(689dbabf) → already in registry → SKIP (no duplicate)
   → chat_message (if any) → uses existing Entity A → subscription fires
```

### What This Eliminates

- No rogue thread (panel restores correctly via ACP path)
- No parallel thread loads (shared lock)
- No `open_thread` racing with restoration (queued after `agent_ready`)
- Subscription set up by panel restoration (not just thread_service)
- `agent_ready` sent from all paths (not just thread_service)

## Files Involved

| File | Repo | Role |
|------|------|------|
| `frontend/src/contexts/streaming.tsx` | helix | WebSocket handler — cache invalidation fix |
| `api/pkg/server/websocket_external_agent_sync.go` | helix | Queue `open_thread` after `agent_ready`, `prependToReadinessQueue` |
| `crates/agent_ui/src/agent_panel.rs` | zed | Fix `selected_agent_type`, wait for WebSocket |
| `crates/agent_ui/src/conversation_view.rs` | zed | `ensure_thread_subscription` + `send_agent_ready` from panel restore, shared lock |
| `crates/external_websocket_sync/src/thread_service.rs` | zed | Public `ensure_thread_subscription`, lock helpers, debug logging |
| `crates/external_websocket_sync/src/websocket_sync.rs` | zed | `wait_for_websocket_connected` |

## PRs

- **Helix**: `fix/streaming-query-cache-key-mismatch` (helixml/helix#2153) — frontend cache fix + `open_thread` after `agent_ready`
- **Zed**: `fix/agent-type-serialization-and-debug-logging` (helixml/zed) — serialization fix + WebSocket wait + shared lock + `agent_ready` from restore path
