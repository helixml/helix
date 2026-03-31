# Design: Queue Deletion Flakiness & Interrupt Mode

## Problem Summary

Two distinct but related bugs in the prompt queue system:

### Bug 1: Deleted items reappear

**Root cause:** Deletion is frontend-only. `RobustPromptInput.handleRemoveFromQueue` removes the entry from localStorage via `usePromptHistory.removeFromQueue`, but never notifies the backend. The backend's async queue processor (`processPendingPromptsForIdleSessions` in `prompt_history_handlers.go`) independently reads from the DB, which still has the record, and re-queues it.

On API restart, the in-memory `sessionToWaitingInteraction map[string][]string` (server.go:98) is reinitialized empty, and DB records without `deleted_at` are candidates for re-processing.

There is no `deleted_at` / soft-delete field on `PromptHistoryEntry`.

**Key files:**
- `frontend/src/components/common/RobustPromptInput.tsx:573` — frontend delete, no API call
- `frontend/src/hooks/usePromptHistory.ts` — localStorage-only
- `api/pkg/server/prompt_history_handlers.go` — async processor, reads DB unconditionally

### Bug 2: Interrupt-mode prompts disappear in Zed

**Likely root cause:** When `interrupt=true`, the backend sends the message and calls `cancel()` on the current ACP turn. The race between cancel and the new message delivery can cause the new message to be dropped if the cancel reaches Zed first and Zed tears down the active thread before routing the new message. The E2E tests don't cover this path, so the failure mode is undetected.

**Key files:**
- `api/pkg/server/websocket_external_agent_sync.go` — interrupt routing logic
- `zed/crates/external_websocket_sync/src/thread_service.rs` — receives commands, feeds to ACP
- `zed/crates/external_websocket_sync/e2e-test/` — existing E2E (no interrupt phase)

---

## Approach

### Fix 1: Backend soft-delete for queue items

Add `deleted_at *time.Time` to `PromptHistoryEntry` (or equivalent store type). Expose a `DELETE /api/v1/sessions/:sessionID/prompt-queue/:entryID` endpoint that sets `deleted_at`.

Frontend calls this endpoint in `handleRemoveFromQueue` before updating localStorage.

The async queue processor and any in-memory queue rebuild skip entries where `deleted_at IS NOT NULL`.

If the item was already dispatched (i.e., it's in `sessionToWaitingInteraction`), remove it from the slice too.

### Fix 2: Interrupt-mode delivery race

Investigate the exact sequence in `websocket_external_agent_sync.go` when `interrupt=true`:
- Is the cancel sent before or after the new `chat_message` command?
- Does Zed handle receiving a new command while a cancel is in flight?

The fix is likely to send the new `chat_message` command *before* sending cancel, so Zed has the new message queued before tearing down the current turn. Alternatively, Zed's `thread_service.rs` should buffer incoming commands during a cancel cycle.

### Fix 3: E2E test — interrupt phase

Add a Phase 6 to the existing E2E test in `zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go`:
- Send a regular message, wait for Zed to start responding
- Send an interrupt message before the first response completes
- Assert that Zed emits `thread_created` or `message_added` for the interrupt message
- Assert the original response is canceled

---

## Key Decisions

- **Soft-delete over hard-delete**: A hard delete while an item is in the FIFO slice requires locking and slice surgery. Soft-delete is simpler and auditable.
- **Send command before cancel**: Guarantees Zed has the new message queued before the ACP cancel signal arrives.
- **E2E test is a new phase in existing harness**: Avoids a separate test binary; keeps infrastructure simple.

---

## Patterns Found in This Codebase

- The WebSocket sync handlers are all in one large file (`websocket_external_agent_sync.go`, ~2700 lines). New handler logic goes there.
- Queue state is in-memory (`sessionToWaitingInteraction`); DB is the source of truth for rebuilds. Any fix to deletion must touch both.
- E2E tests use a Go test server + real Zed binary in headless Docker. Test phases are sequential and hard-coded in `main.go`.
- `PromptHistoryEntry` lives in the store layer; schema changes require a migration file under `api/pkg/store/migrations/`.
