# Zed Multi-Thread Sync — Design & Fix Plan

**Date**: 2026-03-21
**Status**: Investigation complete, fix in progress
**Branch**: `fix/zed-multi-thread-sync`

## Problem

When using zed-agent runtime, the agent creates new threads manually (Zed doesn't auto-compact — user must create a new thread for context management). New threads don't sync to Helix:

1. Activity in new threads is invisible to Helix
2. Interacting from Helix UI sends messages to the old thread → out-of-context errors
3. The session dropdown in the frontend only shows the original thread

## Existing Infrastructure

There's a full multi-session model already built and in use (250 work sessions, 250 zed threads in the DB):

```
SpecTask
  └── SpecTaskWorkSession (maps 1:1 to Helix Session)
       └── SpecTaskZedThread (maps 1:1 to Zed thread ID)
```

Tables: `spec_task_work_sessions`, `spec_task_zed_threads`

For spectask `spt_01kj8pxf9w49ek6zcdgdckvspe`:
- 1 work session: `stws_43685ada...` → `ses_01kj8t3c...`
- 1 zed thread: `8b975c94-...` (the original)
- User created a new thread in Zed, but it was never registered

## Root Causes

### 1. Zed: `user_created_thread` event suppressed on empty threads

**File**: `zed/crates/agent_ui/src/acp/thread_view.rs:991-1007`

```rust
if !is_resume {
    let entry_count = thread_entity.read(cx).entries().len();
    if entry_count > 0 {  // ← BLOCKS empty new threads
        // send UserCreatedThread event
    }
}
```

When the user clicks "New Thread" in Zed, the thread starts empty (`entry_count == 0`). The event never fires. The first message to the thread doesn't re-trigger this code path.

**Fix**: Send the event unconditionally for non-resume thread switches. Remove the `entry_count > 0` guard. The event includes the `acp_thread_id` and `title` — Helix can handle empty threads.

### 2. Helix: `handleUserCreatedThread` bypasses work session model

**File**: `api/pkg/server/websocket_external_agent_sync.go:3091-3129`

When `user_created_thread` IS received, the handler creates a raw `Session` directly instead of going through the `SpecTaskWorkSession` + `SpecTaskZedThread` model. The new session is missing:

- `SpecTaskID` in metadata
- `ProjectID` on session
- `CodeAgentRuntime` in metadata
- No `SpecTaskWorkSession` row created
- No `SpecTaskZedThread` row created

**Fix**: Create proper work session + zed thread records alongside the session:

```go
// 1. Create new Helix Session (copy metadata from parent)
session := &types.Session{
    // ... copy all fields from existingSession
    // Including SpecTaskID, ProjectID, CodeAgentRuntime
}

// 2. Create SpecTaskWorkSession
workSession := &types.SpecTaskWorkSession{
    SpecTaskID:     existingSession.Metadata.SpecTaskID,
    HelixSessionID: session.ID,
    Phase:          existingWorkSession.Phase,  // inherit from parent
    Status:         types.SpecTaskWorkSessionStatusActive,
}

// 3. Create SpecTaskZedThread
zedThread := &types.SpecTaskZedThread{
    WorkSessionID: workSession.ID,
    SpecTaskID:    existingSession.Metadata.SpecTaskID,
    ZedThreadID:   acpThreadID,
    Status:        types.SpecTaskZedStatusActive,
}
```

### 3. Helix: `open_thread` on reconnect uses stale thread ID

**File**: `api/pkg/server/websocket_external_agent_sync.go:2676-2691`

When the desktop restarts and reconnects, Helix sends `open_thread` using the `ZedThreadID` from the planning session — which is the FIRST thread, not the current one. This jumps the user back to the old thread.

**Fix**: On reconnect, find the LATEST `SpecTaskZedThread` for this spectask (by `LastActivityAt` or creation time) and use that thread ID instead of the one stored on the session.

### 4. Frontend: Session dropdown exists but needs data

The user says there IS a session dropdown in the UI. The issue is that the new session/thread is never created (issues 1+2), so the dropdown only shows one entry. Once issues 1+2 are fixed, the dropdown should populate automatically.

### 5. No E2E test for multi-thread flow

**File**: `zed/crates/external_websocket_sync/e2e-test/`

The E2E test has 9 phases but none test thread creation. Need a new phase:
- Create initial thread, send message
- Create new thread (simulate user click)
- Verify `user_created_thread` event received by server
- Send message on new thread
- Verify message routed to new session (not original)
- Verify original session still accessible

## Fix Order

1. **Zed: Remove entry_count guard** (enables the event to fire)
2. **Helix: Fix handleUserCreatedThread** (creates proper work session + zed thread)
3. **Helix: Fix open_thread reconnect** (uses latest thread, not first)
4. **E2E test: Add multi-thread phase** (prevents regression)

## Observed Data

- Spectask: `spt_01kj8pxf9w49ek6zcdgdckvspe`
- Session: `ses_01kj8t3c38jd7sx8k4faws26ek`
- Thread: `8b975c94-9916-47ca-90ed-511ed8d63dbd`
- Agent: `zed_agent` runtime
- Work session: `stws_43685adab96a87afa6b9ff65efde26f9` (active)
- No `user_created_thread` events ever received in API logs
- User created new thread in Zed UI but Helix never learned about it
