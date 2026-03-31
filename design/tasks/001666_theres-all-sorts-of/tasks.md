# Implementation Tasks

## Bug 1: Queue deletion reappearance

- [x] Add `DeletedAt *time.Time` field to `PromptHistoryEntry` in types (GORM AutoMigrate adds column)
- [x] Add `DeletePromptHistoryEntry(ctx, id)` to store interface, postgres impl, mock, and memorystore
- [x] Update `SyncPromptHistory` and `ListPromptHistoryBySpecTask` DB queries to exclude `deleted_at IS NOT NULL` entries
- [x] Update queue-fetching queries (`GetNextPendingPrompt`, `GetAnyPendingPrompt`, `GetNextInterruptPrompt`) to skip deleted entries
- [x] Add `DELETE /api/v1/prompt-history/{id}` endpoint with swagger annotations
- [x] Register the route in server.go
- [x] Add swagger annotations → run `./stack update_openapi` → use generated client method in frontend
- [x] Update `RobustPromptInput.handleRemoveFromQueue` to call the new delete API before removing from localStorage

## Bug 2: Interrupt-mode prompts disappear (double-claim bug)

Root cause: `processAnyPendingPrompt` and `processInterruptPrompt` call `Get*Prompt` (which atomically sets status='sending') followed by `ClaimPromptForSending` (which requires status='pending'/'failed' → always returns false → function bails).

- [x] Fix `processInterruptPrompt`: remove redundant `ClaimPromptForSending` call
- [x] Fix `processAnyPendingPrompt`: remove redundant `ClaimPromptForSending` call
- [x] Update unit tests that expected the now-removed `ClaimPromptForSending` calls

## Bug 3: E2E tests

E2E tests already have Phase 8 (mid-stream interrupt) and Phase 9 (rapid 3-turn cancel). These were flaky because Bug 2 caused interrupt delivery to always fail. Bug 2 fix resolves the E2E flakiness.

- [x] Verify fixes: all 46 unit tests pass (`CGO_ENABLED=1 go test -v -run TestWebSocketSyncSuite ./pkg/server/ -count=1`)
