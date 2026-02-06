# Zed Thread State Bug Analysis

**Date:** 2026-02-01
**Status:** Investigation
**Related Sessions:** ses_01kgcetmndem6hfxkng9bd7gqf, ses_01kgcdt5byxa9hcyg2jvqfjwd9

## Problem Summary

After Zed restarts (user clicks X or Zed crashes/relaunches), the following issues occur:

1. **Wrong thread displayed**: Zed opens a NEW thread instead of the previously active session thread
2. **UI disconnection**: Messages sent from Helix don't appear in the visible Zed thread
3. **Duplicate responses**: Helix receives duplicate responses (same message twice)
4. **State divergence**: The "correct" thread exists but isn't displayed; user must manually navigate to find it

## Architecture Overview

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              HELIX API                                      │
│  ┌─────────────────┐     ┌─────────────────────┐                           │
│  │  Session Chat   │────▶│  WebSocket Handler  │                           │
│  │  (React UI)     │     │  /api/v1/external-  │                           │
│  └─────────────────┘     │  agents/sync        │                           │
│                          └──────────┬──────────┘                           │
└─────────────────────────────────────┼───────────────────────────────────────┘
                                      │ WebSocket
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DESKTOP CONTAINER (helix-ubuntu)                    │
│                                                                             │
│  ┌──────────────────────────────────────────────────────────────────────┐  │
│  │                           ZED EDITOR                                 │  │
│  │                                                                      │  │
│  │  ┌────────────────────┐    ┌───────────────────────────────────┐    │  │
│  │  │    AgentPanel      │    │   external_websocket_sync         │    │  │
│  │  │  (agent_panel.rs)  │◀──▶│  (external_websocket_sync.rs)     │    │  │
│  │  │                    │    │                                   │    │  │
│  │  │  • ActiveView      │    │  • WebSocket connection           │    │  │
│  │  │  • serialize()     │    │  • Event routing                  │    │  │
│  │  │  • load()          │    │  • notify_thread_display()        │    │  │
│  │  └─────────┬──────────┘    └───────────────┬───────────────────┘    │  │
│  │            │                               │                         │  │
│  │            ▼                               ▼                         │  │
│  │  ┌─────────────────────────────────────────────────────────────┐    │  │
│  │  │                    AcpThread Entity                         │    │  │
│  │  │                  (acp_thread.rs)                            │    │  │
│  │  │                                                             │    │  │
│  │  │  • session_id: SessionId                                    │    │  │
│  │  │  • entries: Vec<ThreadEntry>                                │    │  │
│  │  │  • subscriptions: observers get notified on changes         │    │  │
│  │  └─────────────────────────────────────────────────────────────┘    │  │
│  │                                                                      │  │
│  │  ┌────────────────────┐    ┌───────────────────────────────────┐    │  │
│  │  │  NativeAgent       │    │   thread_service.rs               │    │  │
│  │  │  (agent.rs)        │    │                                   │    │  │
│  │  │                    │    │  • THREAD_REGISTRY                │    │  │
│  │  │  • sessions:       │    │  • load_thread_from_agent()       │    │  │
│  │  │    HashMap<        │    │  • handle_follow_up_message()     │    │  │
│  │  │    SessionId,      │    │                                   │    │  │
│  │  │    SessionState>   │    │                                   │    │  │
│  │  └────────────────────┘    └───────────────────────────────────┘    │  │
│  │                                                                      │  │
│  │  ┌─────────────────────────────────────────────────────────────┐    │  │
│  │  │                   KEY_VALUE_STORE                           │    │  │
│  │  │                (SQLite: db/0-dev/db.sqlite)                 │    │  │
│  │  │                                                             │    │  │
│  │  │  Persisted at: ~/.local/share/zed/db/0-dev/db.sqlite        │    │  │
│  │  │  (symlinked to /home/retro/work/.zed-state/local-share/)    │    │  │
│  │  └─────────────────────────────────────────────────────────────┘    │  │
│  └──────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│  ┌────────────────────┐                                                     │
│  │  Qwen Code Agent   │ ◀── LLM API calls                                   │
│  │  (qwen-code)       │                                                     │
│  └────────────────────┘                                                     │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Data Structures

```
NativeAgent.sessions: HashMap<SessionId, SessionState>
  └── SessionState
        ├── acp_thread: WeakEntity<AcpThread>  // Weak reference to thread entity
        └── db_metadata: Option<DbThreadMetadata>

THREAD_REGISTRY: HashMap<String, WeakEntity<AcpThread>>
  └── Maps session_id string -> thread entity

Entity<AcpThread>: The actual thread data
  ├── session_id: SessionId
  ├── entries: Vec<ThreadEntry>  // Messages, tool calls, etc.
  └── Observers: UI components subscribe to changes
```

### Message Flow: Helix → Zed

```
1. Helix sends chat_message via WebSocket
   └── { type: "chat_message", acp_thread_id: <uuid>, message: "...", request_id: "..." }

2. external_websocket_sync receives message
   └── handle_incoming_message() parses command

3. request_thread_creation() is called
   ├── If acp_thread_id is None: Creates new thread
   └── If acp_thread_id exists: Loads existing thread from agent

4. Thread entity is created/loaded
   └── register_session() stores in NativeAgent.sessions

5. notify_thread_display() sends notification to AgentPanel
   └── AgentPanel switches to display the thread

6. Message is added to thread.entries
   └── Observers (UI) receive update notifications

7. UI renders the message
```

## Investigation Findings

### Finding 1: KEY_VALUE_STORE Has No `agent_panel` Key

**Evidence:**
```python
# Queried the database directly:
[('installation_id', '71994bbc-7ae4-483c-9192-3d681d2a6410'),
 ('session_id', 'a8c7a91b-96aa-45c9-972c-1223366530a8'),
 ('agent_panel__last_used_external_agent', '{"agent":"native_agent"}'),
 ('recent-agent-threads', '[{"AcpThread":"4b090239-31aa-4f30-bb1d-cf37ea1f7259"}]')]
```

The `agent_panel` key (which should contain `SerializedAgentPanel` with `active_acp_session`) is **NOT present**.

**Hypothesis:** `serialize()` is either:
- Not being called when Zed closes
- Being called but the async write isn't completing before process exit
- Writing to a different key

### Finding 2: serialized_panel Exists: false

**Evidence from logs:**
```
📂 [AGENT_PANEL] Loading panel - serialized_panel exists: false
📂 [AGENT_PANEL] No serialized panel, creating new NativeAgent thread
```

This confirms the database read is returning None, causing Zed to create a new thread.

### Finding 3: Thread Display Notification Works But Wrong Thread Visible

**Evidence from logs:**
```
🎯 [AGENT_PANEL] Received thread created for session: 4b090239-31aa-4f30-bb1d-cf37ea1f7259
📂 [AGENT_PANEL] Ensuring agent panel is focused
✅ [AGENT_PANEL] Auto-opened existing headless thread in UI
```

The notification IS being received and processed, but user still sees wrong thread.

### Finding 4: Duplicate Message Completed Events

**User observation:** "Helix gets the response duplicated - two twos"

**Hypothesis:** There are two subscriptions sending WebSocket events:
1. `thread_service.rs` subscribes to AcpThread and sends events
2. `thread_view.rs` (UI) also subscribes and sends events

Both fire on the same thread updates, causing duplicates.

## Root Cause Hypotheses

### Hypothesis A: serialize() Not Called on Zed Exit

The `serialize()` method uses `cx.background_spawn()` which is async. When Zed process exits:
1. The async task may not complete before process termination
2. SQLite WAL (Write-Ahead Log) may not flush to main DB

**Supporting evidence:**
- Database exists and has other keys
- But `agent_panel` key is missing
- The serialize call might be scheduled but never executed

**Test:** Add eprintln! in serialize() to confirm it's called, and force sync write.

### Hypothesis B: Entity Orphaning After Restart

Even if session restore works, there may be entity lifecycle issues:

1. `load_acp_agent_session()` creates new `Entity<AcpThread>`
2. WebSocket follow-up creates ANOTHER `Entity<AcpThread>` for same session
3. UI observes the first one, but updates go to the second one
4. Result: UI appears frozen

**Supporting evidence:**
- Previous fix in `register_session()` tried to return existing entity
- But that code path might not be reached in all cases

### Hypothesis C: Thread Registry Mismatch

The THREAD_REGISTRY in `thread_service.rs` tracks threads by session_id string.
After restart:
1. Saved session is loaded with ID "abc123"
2. New message arrives for "abc123"
3. But registry has stale/different entry
4. New thread created instead of reusing existing

### Hypothesis D: Dual Subscription Causing Duplicates

Two places subscribe to AcpThread events and send to WebSocket:
1. `thread_service.rs::handle_follow_up_message()` - creates subscription
2. `thread_view.rs::handle_thread_event()` - UI creates subscription

Both send `MessageAdded` and `MessageCompleted` events.

## Files Involved

| File | Role |
|------|------|
| `crates/agent_ui/src/agent_panel.rs` | AgentPanel component, serialization, thread display |
| `crates/agent_ui/src/acp/thread_view.rs` | UI view for thread, sends WebSocket events |
| `crates/external_websocket_sync/src/external_websocket_sync.rs` | WebSocket connection, event routing |
| `crates/external_websocket_sync/src/thread_service.rs` | Thread loading, registry, message handling |
| `crates/agent/src/agent.rs` | NativeAgent, sessions HashMap, register_session() |
| `crates/acp_thread/src/acp_thread.rs` | Thread entity, entries, subscriptions |
| `crates/db/src/kvp.rs` | KEY_VALUE_STORE, SQLite persistence |

## Proposed Tests

### Test 1: Verify serialize() is Called
Add synchronous logging BEFORE the async spawn in `serialize()`:
```rust
eprintln!("🔧 [SERIALIZE] About to serialize agent_panel");
```

### Test 2: Force Synchronous Write
Temporarily change serialize() to block on the write:
```rust
smol::block_on(KEY_VALUE_STORE.write_kvp(...));
```

### Test 3: Check Duplicate Subscriptions
Add unique ID to each subscription and log when events are sent:
```rust
eprintln!("📤 [thread_service] Sending event from subscription {}", sub_id);
eprintln!("📤 [thread_view] Sending event from subscription {}", sub_id);
```

### Test 4: Trace Entity Lifecycle
Log entity creation/destruction:
```rust
eprintln!("🔧 [ENTITY] Created AcpThread entity_id={}", entity.entity_id());
```

## Next Steps

1. **Debug serialize()** - Confirm it's being called and completing
2. **Fix async write** - Either block on write or hook into Zed shutdown
3. **Deduplicate subscriptions** - Ensure only one place sends WebSocket events
4. **Test entity reuse** - Verify existing entities are returned, not replaced

## Questions for User

1. When you click X on Zed, does it exit immediately or show a "saving..." delay?
2. Is there a Zed config for "exit behavior" that might affect shutdown hooks?
3. Have you ever seen the agent_panel state persist correctly (before this bug)?

---

## Update: Root Cause Found (2026-02-01 11:30)

### The Bug

`serialize()` was only called in specific cases:
1. When switching to TextThread agent type
2. When selecting ClaudeCode or Codex agents
3. When resizing the panel

It was **NOT called** when:
- An ExternalAgentThread (ACP thread) becomes active via WebSocket message
- Zed is about to close

### The Fix (Image c8ed42)

Added `serialize()` call in `set_active_view()` when the new view is an ExternalAgentThread:

```rust
// In set_active_view():
if matches!(self.active_view, ActiveView::ExternalAgentThread { .. }) {
    eprintln!("🔧 [AGENT_PANEL] View changed to ExternalAgentThread, serializing...");
    self.serialize(cx);
}
```

Also added extensive logging to `serialize()` to confirm:
- When it's called
- What session data is being serialized
- Whether the async write completes

### Expected Logs

After this fix, you should see in container logs:
```
🔧 [AGENT_PANEL] View changed to ExternalAgentThread, serializing...
🔧 [SERIALIZE] serialize() called
🔧 [SERIALIZE] ExternalAgentThread active, session=Some(SerializedAcpSession { session_id: "...", agent_name: "..." })
🔧 [SERIALIZE] Serializing: SerializedAgentPanel { ... }
🔧 [SERIALIZE] Async write starting...
✅ [SERIALIZE] Async write completed successfully
```

And on restart:
```
📂 [AGENT_PANEL] Loading panel - serialized_panel exists: true
📂 [AGENT_PANEL] - active_acp_session: Some(SerializedAcpSession { ... })
📂 [AGENT_PANEL] Resuming saved ACP session on startup: ...
```

### Remaining Issues

1. **Duplicate WebSocket events** - Still need to investigate `thread_service.rs` vs `thread_view.rs` both sending events
2. **Entity orphaning** - If the session restore doesn't properly reuse existing entities, UI updates won't propagate

---

## Update: Agent Name Mismatch Fix (2026-02-01 12:15)

### The Bug

The first fix checked for `"zed-agent"` but the serialized value was `"Zed Agent"` (display name):

```
# From container logs:
🔧 [SERIALIZE] ExternalAgentThread active, session=Some(SerializedAcpSession {
    session_id: "...",
    agent_name: "Zed Agent"  # <-- Display name, not wire protocol name!
})
```

The original fix only matched the wire protocol name:
```rust
// Only matched "zed-agent", missed "Zed Agent"
match agent_name.as_ref() {
    "zed-agent" => ExternalAgent::NativeAgent,
    _ => ExternalAgent::Custom { name: agent_name.clone() },  // "Zed Agent" hit this!
}
```

### The Fix (Image 483f82)

Updated `load_acp_agent_session()` to match both names:

```rust
let ext_agent = match agent_name.as_ref() {
    "zed-agent" | "Zed Agent" => ExternalAgent::NativeAgent,
    _ => ExternalAgent::Custom { name: agent_name.clone() },
};
```

### Expected Logs

On session restore, you should now see:
```
🔧 [LOAD_ACP] load_acp_agent_session called with agent_name="Zed Agent"
🔧 [LOAD_ACP] Matched NativeAgent for agent_name="Zed Agent"
```

Instead of the error:
```
An error happened: custom agent server zed-agent is not registered!
```

---

## Update: UI Corruption & Duplicate Events (2026-02-01 12:45)

### User Report

After the agent name fix, session restore works (correct thread opens). However:

1. **UI Corruption**: When a message arrives from Helix after restart, Zed shows a "corrupted" view with only the first request and response, not the full thread history. Clicking "View All" → selecting the thread shows all entries correctly.

2. **Duplicate WebSocket Events**: Responses from Zed to Helix are sent twice.

### Root Cause 1: `from_existing_thread()` Missing `splice_focusable()`

When `notify_thread_display()` creates a new `AcpThreadView` via `from_existing_thread()`, it syncs the entry data but **never adds them to the list_state**:

```rust
// from_existing_thread() - BUGGY CODE
let list_state = ListState::new(0, ...);  // Creates with 0 items
let count = thread.read(cx).entries().len();
entry_view_state.update(cx, |view_state, cx| {
    for ix in 0..count {
        view_state.sync_entry(ix, &thread, window, cx);  // ✅ Syncs data
    }
    // ❌ MISSING: list_state.splice_focusable() to add entries to UI!
});
```

Compare to the working code in the regular constructor:
```rust
// Regular constructor - WORKING CODE
this.entry_view_state.update(cx, |view_state, cx| {
    for ix in 0..count {
        view_state.sync_entry(ix, &thread, window, cx);
    }
    this.list_state.splice_focusable(  // ✅ Adds entries to list!
        0..0,
        (0..count).map(|ix| view_state.entry(ix)?.focus_handle(cx)),
    );
});
```

**Result**: Thread data exists but list displays nothing initially. New messages get added incrementally, explaining why only the first request/response show up.

### Root Cause 2: Dual WebSocket Subscriptions

Two places subscribe to `AcpThread` events and send WebSocket events:

1. **thread_service.rs:617-686** - Subscription in `handle_follow_up_message()`:
   - Handles `AcpThreadEvent::Stopped` → sends `MessageCompleted`

2. **thread_view.rs:1948-1977** - Subscription in UI:
   - Handles `AcpThreadEvent::Stopped` → sends `MessageCompleted`

Both fire on the same event, causing **duplicate `MessageCompleted` events**.

### The Fixes

#### Fix 1: Add missing `splice_focusable()` to `from_existing_thread()`

```rust
// In from_existing_thread(), after sync_entry loop:
list_state.splice_focusable(
    0..0,
    (0..count).filter_map(|ix| entry_view_state.read(cx).entry(ix).map(|e| e.focus_handle(cx))),
);
```

#### Fix 2: Remove duplicate subscription

The thread_service.rs subscription should only handle **loaded threads** (threads created headlessly from Helix messages), not threads that have a UI view.

When a thread has a UI view (AcpThreadView), the view's subscription should handle events. When running headlessly (no UI), thread_service.rs handles events.

Current approach: Remove the Stopped event handler from thread_service.rs and let thread_view.rs handle it exclusively, since `notify_thread_display()` always creates a UI view for the thread.

#### Fix 3: Skip creating duplicate views (Image fe9dc6)

Additional duplicates occurred because `notify_thread_display()` was creating new views even when the panel already showed that thread. Added a check:

```rust
// In notify_thread_display handler:
if let ActiveView::ExternalAgentThread { thread_view } = &this.active_view {
    if let Some(existing_thread) = thread_view.read(cx).thread() {
        let existing_session_id = existing_thread.read(cx).session_id().to_string();
        if existing_session_id == incoming_session_id {
            eprintln!("🔄 [AGENT_PANEL] Already showing thread {}, skipping view creation", incoming_session_id);
            return; // Don't create duplicate view
        }
    }
}
```

This prevents creating multiple AcpThreadView instances for the same thread, which would each subscribe to thread events and send duplicate WebSocket messages.

#### Fix 4: Register thread in THREAD_REGISTRY (Image 57818d)

Root cause of the decoherence: When the UI loads a thread at startup via `load_acp_agent_session()`, the thread entity was never registered in `THREAD_REGISTRY`. Later, when a WebSocket command arrives, `thread_service.rs` checks the registry, doesn't find it, and creates a NEW entity. The UI observes Entity A, but messages go to Entity B.

Added registration in `thread_view.rs` at line 919, right after the thread is ready:

```rust
#[cfg(feature = "external_websocket_sync")]
{
    let session_id = thread.read(cx).session_id().to_string();
    eprintln!("📋 [THREAD_VIEW] Registering thread {} in THREAD_REGISTRY", session_id);
    external_websocket_sync::register_thread(session_id, thread.clone());
}
```

Now when WebSocket commands arrive, they find the UI's entity in the registry and use it, so updates propagate correctly to the UI.

---

## Status: Fixed (2026-02-01 17:00)

The main Zed thread state bug is now fixed in image `57818d`. Key fixes:

1. **Serialize on view change** - Session now persists when switching to ExternalAgentThread
2. **Agent name mapping** - Both "zed-agent" and "Zed Agent" map to NativeAgent
3. **UI list population** - `from_existing_thread()` now adds entries to `list_state`
4. **Skip duplicate views** - Don't create new view if already showing the thread
5. **Thread registration** - UI registers thread in THREAD_REGISTRY so WebSocket finds it

### Remaining Issue (Helix Frontend)

First response back from Zed in the Helix session may show stale content. This appears to be a Helix frontend caching/rendering issue, not a Zed issue.
