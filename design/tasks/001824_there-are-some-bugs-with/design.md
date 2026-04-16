# Design: Fix Interrupt Message Sending from Robust Prompt Input

## Architecture Overview

The interrupt message flow involves three layers:

1. **Frontend** (`RobustPromptInput.tsx` → `usePromptHistory.ts` → `promptHistoryService.ts`): Captures user intent, creates a `PromptHistoryEntry` with `interrupt: true/false`, syncs to backend.
2. **Backend API** (`prompt_history_handlers.go`): Receives sync, stores entry in DB, triggers `processPendingPromptsForIdleSessions` in a goroutine.
3. **Backend Processing** (`websocket_external_agent_sync.go`): `processInterruptPrompt` / `processPromptQueue` claims the entry, creates an interaction, and sends a `chat_message` command to Zed with `"interrupt": true/false`.

## Code Analysis & Findings

### Key Files
| Layer | File | Purpose |
|-------|------|---------|
| Frontend Component | `frontend/src/components/common/RobustPromptInput.tsx` | Input widget, keyboard handlers, send buttons |
| Frontend Hook | `frontend/src/hooks/usePromptHistory.ts` | State management, localStorage, backend sync |
| Frontend Service | `frontend/src/services/promptHistoryService.ts` | API calls to backend |
| Backend Handler | `api/pkg/server/prompt_history_handlers.go` | HTTP endpoints, triggers processing |
| Backend Store | `api/pkg/store/store_prompt_history.go` | Database CRUD, atomic claim queries |
| Backend Processing | `api/pkg/server/websocket_external_agent_sync.go` | `processInterruptPrompt`, `processPromptQueue`, `sendQueuedPromptToSession` |
| Types | `api/pkg/types/prompt_history.go` | `PromptHistoryEntry`, `PromptHistoryEntrySync` |

### Three Send Paths Compared

#### Path A: Ctrl+Enter (BROKEN)
```
handleKeyDown → useInterrupt = e.ctrlKey || e.metaKey → saveToHistory(content, true)
→ syncEntryImmediately(entry) → POST /api/v1/prompt-history/sync
→ SyncPromptHistory creates entry → go processPendingPromptsForIdleSessions
```

#### Path B: Toggle Mode + Send (BROKEN)
```
setInterruptMode(true) → handleSend() → saveToHistory(content, interruptMode=true)
→ syncEntryImmediately(entry) → POST /api/v1/prompt-history/sync
→ SyncPromptHistory creates entry → go processPendingPromptsForIdleSessions
```

#### Path C: Toggle on Queued Message (WORKS)
```
updateInterrupt(entryId, true) → marks syncedToBackend=false
→ debounced syncToBackend() → POST /api/v1/prompt-history/sync
→ SyncPromptHistory UPDATES existing entry → go processPendingPromptsForIdleSessions
```

#### Path D: Empty Enter promotes oldest queue message (NEW FEATURE)
```
handleKeyDown → Enter with empty draft, no Ctrl →
  find oldest pending entry with interrupt=false →
  updateInterrupt(entryId, true) → marks syncedToBackend=false
→ debounced syncToBackend() → same flow as Path C
```

This reuses the existing `updateInterrupt` from the hook (same mechanism as Path C / the "switch to interrupt" button on queued messages). The only new code is the conditional logic in `handleKeyDown` to detect empty-input Enter and find the right entry to promote.

### Root Cause Analysis

After tracing all three paths through the entire stack, the data flow for setting `interrupt: true` appears correct at every layer — the frontend correctly passes the flag, the service correctly serializes it, the backend correctly deserializes and stores it, and the processing logic correctly distinguishes interrupt from queue prompts.

**The critical difference between working (Path C) and broken (Paths A/B) is: Path C updates an EXISTING entry, while Paths A/B create NEW entries.** This points to a potential issue in either:

1. **The CREATE path in `SyncPromptHistory`** — some edge case where the `interrupt` flag is lost during insert (GORM boolean handling, JSON deserialization of `*bool` with `omitempty`)
2. **Timing/race between `syncEntryImmediately` and the debounced `syncToBackend`** — both can fire for the same new entry within the 100ms debounce window, potentially causing conflicting updates

**Confirmed secondary bug:** `processInterruptPrompt` (prompt_history_handlers.go:208-253) does NOT call `MarkPromptAsSent` after successful delivery. The prompt stays in 'sending' status. Compare with `processPromptQueue` (websocket_external_agent_sync.go:2540-2543) which correctly calls `MarkPromptAsSent`. While this doesn't prevent delivery, it means interrupt prompts are never cleaned up properly.

### Investigation Strategy for Implementation

Since the exact root cause requires runtime debugging, the implementation should:

1. **Add logging** to the frontend `syncEntryImmediately` and backend `SyncPromptHistory` CREATE path to verify the interrupt flag value at each step
2. **Add a database query** after create to confirm the interrupt flag was persisted
3. **Fix the `MarkPromptAsSent` bug** in `processInterruptPrompt` regardless
4. **Test each path** end-to-end with browser devtools network tab open to verify the JSON payload

## Key Decisions

- **Fix approach**: Debug the CREATE path with logging first, then fix whatever is found. The code structure is sound — this is a data-flow bug, not an architecture issue.
- **No refactoring**: The three paths should remain separate (they serve different UX purposes). Just fix the interrupt flag handling.
- **Testing**: Both manual testing (Ctrl+Enter, toggle+send) and checking API logs/DB state are needed since the bug crosses frontend/backend.

## Codebase Patterns Discovered

- **Prompt history uses a dual-sync model**: `syncEntryImmediately` for new prompts (no debounce), `syncToBackend` for updates (100ms debounce). Both call the same backend endpoint.
- **Atomic claim pattern**: `GetNextInterruptPrompt` / `GetNextPendingPrompt` use PostgreSQL `UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED) RETURNING *` to prevent race conditions between concurrent goroutines.
- **Frontend uses localStorage + backend sync**: Entries are persisted in localStorage first, then synced to the backend. The backend response is merged back into local state.
- **`PromptHistoryEntrySync.Interrupt` is `*bool` with `omitempty`**: This means a missing `interrupt` field in JSON defaults to `false` (not `true`) in the backend store. The frontend must always explicitly send the field.
- **Backend-owned vs frontend-owned fields**: The sync UPDATE path only touches `interrupt`, `queue_position`, `content`, `updated_at` — it preserves `status`, `retry_count`, `next_retry_at` which are backend-owned.
