# Design: Fix In-Memory Mappings Lost After Restart

## Problem Analysis

The Helix API server maintains several in-memory maps for routing messages between HTTP clients and WebSocket-connected agents:

```
┌─────────────────┐      ┌─────────────────────┐      ┌─────────────────┐
│  HTTP Client    │──────│   Helix API         │──────│   Agent (Zed)   │
│  (browser)      │      │   (in-memory maps)  │      │   (WebSocket)   │
└─────────────────┘      └─────────────────────┘      └─────────────────┘
                                   │
                         ┌─────────┴─────────┐
                         │ RESTART = LOST    │
                         │ - contextMappings │
                         │ - responseChannels│
                         │ - sessionToWaiting│
                         └───────────────────┘
```

When the API restarts:
1. WebSocket connections drop (expected - agents reconnect)
2. In-memory maps are empty (problem - routing breaks)
3. Agent reconnects but API can't route messages to it

## Current State

**Partially Working:**
- `contextMappings` (Zed thread → Helix session) is restored from `session.Metadata.ZedThreadID` on reconnect (line 332 of websocket_external_agent_sync.go)

**Not Working:**
- `sessionToWaitingInteraction` - never persisted, never restored
- `requestToSessionMapping` - never persisted, never restored  
- `externalAgentSessionMapping` - initialized empty, never populated (unused?)
- `responseChannels` - HTTP Go channels (cannot be persisted, must timeout gracefully)

## Solution: Persist to Session Metadata

### Key Insight
Session metadata is already persisted in the database and restored when agents reconnect. We should extend this pattern.

### Design

1. **Add Fields to Session Metadata**
```go
type SessionMetadata struct {
    // Existing
    ZedThreadID string
    // New
    WaitingInteractionID string  // The interaction currently waiting for response
    LastRequestID        string  // Most recent request_id (for reconnect routing)
}
```

2. **Persist on Set**
When we set `sessionToWaitingInteraction[sessionID] = interactionID`, also:
```go
session.Metadata.WaitingInteractionID = interactionID
store.UpdateSession(ctx, session)
```

3. **Restore on Reconnect**
In `handleExternalAgentSync`, after restoring `contextMappings`:
```go
if session.Metadata.WaitingInteractionID != "" {
    apiServer.sessionToWaitingInteraction[helixSessionID] = session.Metadata.WaitingInteractionID
}
```

4. **Clear on Complete**
When interaction completes, clear both in-memory and persisted:
```go
delete(apiServer.sessionToWaitingInteraction, sessionID)
session.Metadata.WaitingInteractionID = ""
store.UpdateSession(ctx, session)
```

### Response Channels (Cannot Persist)

Go channels cannot be persisted. When API restarts during streaming:

**Current Behavior:** Request hangs until 90s timeout

**Proposed Behavior:** 
- Add `RequestStartedAt` to session metadata
- On reconnect, if `WaitingInteractionID` exists and `RequestStartedAt > 5 minutes ago`, mark interaction as failed
- HTTP streaming code should check for stale requests and return error immediately

## Affected Files

| File | Change |
|------|--------|
| `api/pkg/types/session.go` | Add `WaitingInteractionID` to SessionMetadata |
| `api/pkg/server/session_handlers.go` | Persist WaitingInteractionID when set |
| `api/pkg/server/websocket_external_agent_sync.go` | Restore mappings on reconnect, clear on complete |

## Decision: Keep externalAgentSessionMapping?

This map is initialized but never written to. Options:
1. **Remove it** - dead code
2. **Implement it** - for non-ses_* agent IDs

Recommendation: Remove it. Current code uses `ses_*` prefixed session IDs directly.

## Testing Strategy

1. Start session, send message, verify response
2. Restart API while session active
3. Wait for agent reconnect
4. Send new message, verify routing works
5. Check logs for "Restored contextMappings" and new "Restored sessionToWaitingInteraction" messages

## Implementation Notes

### Files Modified

1. **`api/pkg/types/types.go`** - Added three fields to `SessionMetadata`:
   - `WaitingInteractionID` - Interaction currently waiting for response
   - `LastRequestID` - Most recent request_id for reconnect routing
   - `RequestStartedAt` - Timestamp for stale detection

2. **`api/pkg/server/session_handlers.go`** - In `streamFromExternalAgent`:
   - Persist all three fields to database when setting in-memory mappings
   - Uses non-fatal logging if persistence fails (to not break existing flow)

3. **`api/pkg/server/websocket_external_agent_sync.go`**:
   - In `handleExternalAgentSync`: Restore mappings from session metadata on reconnect
   - Added stale request detection (>5 min threshold) - marks interaction as error and clears mappings
   - In `handleMessageCompleted`: Clear both in-memory and persisted mappings when done
   - Removed dead code for `externalAgentSessionMapping` and `externalAgentUserMapping`

4. **`api/pkg/server/server.go`** - Removed unused fields from `HelixAPIServer` struct

### Key Discoveries

- **`externalAgentSessionMapping` was dead code** - Declared and initialized but never written to
- **`externalAgentUserMapping` was also dead code** - Same issue, always fell back to "external-agent-user"
- **Existing partial fix** - `contextMappings` was already being restored from `ZedThreadID` (line 332), but `sessionToWaitingInteraction` and `requestToSessionMapping` were not
- **Stale threshold of 5 minutes** - Chosen because HTTP streaming requests timeout at 90s, so anything older than 5 min is definitely stale

### Log Messages to Watch

- `💾 [HELIX] Persisted session mappings for restart recovery` - Mappings saved to DB
- `🔧 [HELIX] Restored sessionToWaitingInteraction from session metadata` - Mapping restored on reconnect
- `🔧 [HELIX] Restored requestToSessionMapping from session metadata` - Mapping restored on reconnect
- `⚠️ [HELIX] Detected stale waiting interaction on reconnect` - Old request detected and cleaned up
- `🧹 [HELIX] Cleared session restart recovery metadata` - Cleanup on interaction complete

## CRITICAL: Zed-Side Bug Discovered

### The Real Root Cause

After extensive debugging, we discovered that the "entity released" error after Zed restart is actually a **Zed-side bug**, not a Helix issue.

**Error observed:** `Thread load failed: Failed to load thread: entity released`

### Root Cause Analysis

When Helix sends a `chat_message` with an existing `acp_thread_id` after Zed restarts:

1. WebSocket code checks `THREAD_REGISTRY` → not found (Zed restarted, registry empty)
2. Calls `load_thread_from_agent()` → creates new `Rc<NativeAgentConnection>`
3. Calls `connection.load_thread()` which takes `self: Rc<Self>` (consuming the Rc)
4. Inside `load_thread`, it calls `self.0.update(cx, |agent, cx| agent.open_thread(...))`
5. `open_thread` spawns an async task with `cx.spawn(async move |this, cx| ...)` where `this` is a **WeakEntity**
6. After `load_thread` returns, the `Rc<NativeAgentConnection>` is dropped
7. This drops the only strong reference to `Entity<NativeAgent>`
8. The async task runs, calls `this.update()` on the dead weak reference → **"entity released"**

### Why It Works On First Run

In `new_thread` (used for creating new threads), the code does:
```rust
fn new_thread(self: Rc<Self>, ...) -> Task<...> {
    let agent = self.0.clone();  // <-- CLONES the Entity (strong ref)
    cx.spawn(async move |cx| {
        agent.update(cx, ...)?;  // Uses strong reference
    })
}
```

But in `load_thread`:
```rust
fn load_thread(self: Rc<Self>, ...) -> Task<...> {
    self.0.update(cx, |agent, cx| agent.open_thread(...))
    // <-- Does NOT clone! open_thread captures weak ref internally
}
```

### The Fix (Zed-side)

In `zed/crates/agent/src/agent.rs`, `NativeAgentConnection::load_thread` needs to clone the entity:

```rust
fn load_thread(
    self: Rc<Self>,
    session_id: acp::SessionId,
    _project: Entity<Project>,
    _cwd: &Path,
    cx: &mut App,
) -> Task<Result<Entity<AcpThread>>> {
    // Clone the Entity<NativeAgent> to keep it alive for the duration of the async task.
    let agent = self.0.clone();
    cx.spawn(async move |_cx| {
        let task = agent.update(_cx, |a, cx| a.open_thread(session_id, cx))?;
        task.await
    })
}
```

### Helix-Side Safety Net

Added code in `handleThreadLoadError` to clear `ZedThreadID` when receiving "entity released" error. This allows recovery by creating a new thread on retry, even if the Zed fix isn't deployed yet.