# Fix queue deletion reappearance and interrupt prompt delivery

## Summary

Two bugs causing flakiness in the resilient prompt widget and interrupt mode:

**Bug 1 — Deleted queue items reappear:** Deletion in `RobustPromptInput` only removed the entry from localStorage. The backend DB still had the record, so `SyncPromptHistory` returned it on the next sync and the frontend re-added it. Fix: soft-delete (`deleted_at`) with a backend `DELETE /api/v1/prompt-history/{id}` endpoint. Frontend now calls this before updating localStorage. All queue-processing SQL queries filter `deleted_at IS NULL`.

**Bug 2 — Interrupt-mode prompts disappear:** `processInterruptPrompt` and `processAnyPendingPrompt` called `Get*Prompt()` (which atomically sets `status='sending'` via `UPDATE...RETURNING`) then immediately called `ClaimPromptForSending()` (which requires `status IN ('pending','failed')`). The second call always returned `claimed=false`, causing every interrupt and idle-session prompt to be silently dropped. Fix: remove the redundant `ClaimPromptForSending` calls — the Get functions already provide an atomic claim.

**Bug 3 — Flaky E2E tests (Phase 8/9):** The existing interrupt E2E phases (mid-stream interrupt, rapid 3-turn cancel) were flaky because Bug 2 caused interrupt delivery to always fail. No new test phases needed; fixing Bug 2 resolves the flakiness.

## Changes

- `api/pkg/types/prompt_history.go` — add `DeletedAt *time.Time` field (GORM adds column via AutoMigrate)
- `api/pkg/store/store.go` — add `DeletePromptHistoryEntry` to store interface
- `api/pkg/store/store_prompt_history.go` — implement soft-delete; add `deleted_at IS NULL` filter to all queue-fetch and sync queries
- `api/pkg/store/store_mocks.go` + `memorystore/memorystore.go` — add mock/stub implementations
- `api/pkg/server/prompt_history_handlers.go` — add `DELETE /api/v1/prompt-history/{id}` endpoint; fix `processInterruptPrompt` double-claim
- `api/pkg/server/websocket_external_agent_sync.go` — fix `processAnyPendingPrompt` double-claim
- `api/pkg/server/server.go` — register new DELETE route
- `api/pkg/server/websocket_external_agent_sync_test.go` — remove expectations for the now-removed `ClaimPromptForSending` calls (all 46 tests pass)
- `frontend/src/api/api.ts` — regenerated with `v1PromptHistoryDelete` method
- `frontend/src/components/common/RobustPromptInput.tsx` — call backend delete in `handleRemoveFromQueue`
