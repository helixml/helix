# Bug: Queue/Interrupt Message Delivery Hangs Zed Thread

**Date**: 2026-03-19
**Session**: spt_01km320kmz9x3b1w1z92y8cmmd / ses_01km321cskpdehma4nj8537cnz

## Three Bugs

### Bug 1: Edits to queued messages never reach the backend

**Root cause**: Two related problems:

**1a. Frontend doesn't re-sync edited entries.** Both `updateInterrupt()` (usePromptHistory.ts:630) and `updateContent()` (usePromptHistory.ts:619) update local state but do NOT reset `syncedToBackend` to `false`. The `syncToBackend()` function (line 267) only syncs entries where `!h.syncedToBackend`. Since the entry was already synced on creation, subsequent edits are never sent to the backend.

**1b. Backend update path ignores content changes.** Even if the frontend re-synced, the backend's `SyncPromptHistory` update path (store_prompt_history.go:68-72) only updates `interrupt`, `queue_position`, and `updated_at`. It does not update `content`. So edited prompt text would be silently dropped.

**Fix (frontend)**: In both `updateInterrupt` and `updateContent`, also set `syncedToBackend: false`.

**Fix (backend)**: Add `content` to the update fields in `SyncPromptHistory`.

**Files**:
- `frontend/src/hooks/usePromptHistory.ts:619-638`
- `api/pkg/store/store_prompt_history.go:66-81`

### Bug 2: Helix sends queue-mode prompt while Zed is already busy

**What happened**: User typed a message in Zed (Path 2, local). Meanwhile the same message was in Helix's prompt queue (Path 1). After `message_completed`, `processPromptQueue` fired and sent the queued prompt to Zed while Zed was already processing the user's local message. The content being identical was coincidental (user copy-pasted because Bug 1 prevented the interrupt toggle from working) — the bug would occur with any two messages.

**Root cause**: `processPromptQueue` runs as a goroutine after `message_completed`. It doesn't check whether Zed already has a message in flight. Queue-mode (non-interrupt) messages should NEVER be sent while the session is busy — they should wait for idle. Only interrupt messages should be sent while busy.

**Fix**: In `processPromptQueue` (which only handles non-interrupt/queue-mode prompts), check the DB interaction state before sending. Query the session's last interaction — if it's `waiting`, the session is busy, skip. This is race-free because `handleMessageAdded` for Zed's `message_added(role=user)` creates the interaction synchronously before returning.

No Zed-side changes needed for Bug 2. Queue-mode messages simply never reach Zed while it's busy. Interrupt messages already work correctly — `thread.send()` cancels the current turn, which is the intended behavior for interrupts.

**Files**:
- `api/pkg/server/websocket_external_agent_sync.go:2170` (`processPromptQueue`)

### Bug 3: Rapid cancel sequence drops GPUI Task, leaving thread permanently stuck

**Root cause**: GPUI Tasks are cancel-on-drop ("If you drop a task it will be cancelled immediately" — scheduler/src/executor.rs:233). The `run_turn()` cancel chain involves `cancel()` doing `background_spawn(old_send_task)`, moving Tasks through multiple ownership levels. The rapid 3-turn cancel sequence caused a Task in the chain to be dropped, cancelling its async future.

When a `send_task` is cancelled, its `tx` oneshot sender is dropped. The outer future's `rx.await` returns `Err`. The outer future hits `let Ok(response) = response else { return Ok(None) }` and returns **WITHOUT** clearing `running_turn` and **WITHOUT** emitting `Stopped`.

**Result**: `running_turn` permanently set, thread shows `Generating`, spinner forever, no input accepted. Agent is idle (JSONL confirms turn 3's prompt was never sent).

**Fix**: In the outer future (acp_thread.rs:1986-1991), when `rx.await` returns `Err` (tx dropped), check `is_same_turn` and take `running_turn` if it still belongs to this turn, then emit `Stopped`. This ensures the thread recovers from a dropped Task.

```rust
let Ok(response) = response else {
    let is_same_turn = this.running_turn.as_ref()
        .is_some_and(|turn| turn_id == turn.id);
    if is_same_turn {
        this.running_turn.take();
    }
    cx.emit(AcpThreadEvent::Stopped);
    return Ok(None);
};
```

**File**: `zed/crates/acp_thread/src/acp_thread.rs:1986-1991` (outer future in `run_turn`)

## Implementation Plan

### Bug 1 (frontend + backend)

1. `frontend/src/hooks/usePromptHistory.ts:622` — add `syncedToBackend: false` in `updateContent`
2. `frontend/src/hooks/usePromptHistory.ts:633` — add `syncedToBackend: false` in `updateInterrupt`
3. `api/pkg/store/store_prompt_history.go:68` — add `"content": entry.Content` to update fields

### Bug 2 (Helix only)

1. `api/pkg/server/websocket_external_agent_sync.go` — in `processPromptQueue`, before sending, query the session's interactions. If the last interaction is `waiting`, skip (session is busy, queue will retry on next `message_completed`).

### Bug 3 (Zed only)

1. `zed/crates/acp_thread/src/acp_thread.rs:1989-1991` — handle the `Err` case from `rx.await`: take `running_turn` if same turn, emit `Stopped`.
