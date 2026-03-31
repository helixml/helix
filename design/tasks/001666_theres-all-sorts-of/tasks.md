# Implementation Tasks

## Bug 1: Queue deletion reappearance

- [~] Add `DeletedAt *time.Time` field to `PromptHistoryEntry` in types (GORM AutoMigrate adds column)
- [ ] Add `DeletePromptHistoryEntry(ctx, id)` to store interface, postgres impl, mock, and memorystore
- [ ] Update `SyncPromptHistory` and `ListPromptHistory` DB queries to exclude `deleted_at IS NOT NULL` entries
- [ ] Update queue-fetching queries (`GetNextPendingPrompt`, `GetAnyPendingPrompt`, `GetNextInterruptPrompt`) to skip deleted entries
- [ ] Add `DELETE /api/v1/prompt-history/{id}` endpoint with swagger annotations
- [ ] Register the route in server.go
- [ ] Add swagger annotations → run `./stack update_openapi` → use generated client method in frontend
- [ ] Update `RobustPromptInput.handleRemoveFromQueue` to call the new delete API before removing from localStorage

## Bug 2: Interrupt-mode prompts disappear (double-claim bug)

Root cause found: `processAnyPendingPrompt` and `processInterruptPrompt` call `Get*Prompt` (which atomically sets status='sending') followed by `ClaimPromptForSending` (which requires status='pending'/'failed' → always returns false → function bails).

- [~] Fix `processInterruptPrompt`: remove redundant `ClaimPromptForSending` call (prompt is already claimed by `GetNextInterruptPrompt`)
- [ ] Fix `processAnyPendingPrompt`: remove redundant `ClaimPromptForSending` call (prompt is already claimed by `GetAnyPendingPrompt`)

## Bug 3: E2E tests

E2E tests already have Phase 8 (mid-stream interrupt) and Phase 9 (rapid 3-turn cancel). These are flaky because Bug 2 causes interrupt delivery to always fail. Fixing Bug 2 should fix the E2E flakiness.

- [ ] Verify fixes by checking unit tests pass: `CGO_ENABLED=1 go test -v -run TestWebSocketSyncSuite ./pkg/server/ -count=1`
