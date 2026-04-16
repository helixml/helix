# Implementation Tasks

## Debugging & Root Cause

- [ ] Add `console.log` in `handleKeyDown` (RobustPromptInput.tsx:676) to log `e.ctrlKey`, `e.metaKey`, and `useInterrupt` value when Enter is pressed
- [ ] Add `console.log` in `handleSend` (RobustPromptInput.tsx:559) to log `interruptMode` value
- [ ] Add `console.log` in `saveToHistory` (usePromptHistory.ts:549) to log the `interrupt` parameter value
- [ ] Add `console.log` in `syncEntryImmediately` (usePromptHistory.ts:240) to log the entry's `interrupt` field before sending
- [ ] Add backend logging in `SyncPromptHistory` (store_prompt_history.go:19-82) to log the `interrupt` value received in the sync request and the value written to DB
- [ ] Test Ctrl+Enter: check browser devtools network tab for the sync request payload — verify `interrupt: true` is in the JSON body
- [ ] Test toggle+send: check the same way
- [ ] Query the database after each test to verify the entry's `interrupt` column value: `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, interrupt, status FROM prompt_history_entries ORDER BY created_at DESC LIMIT 5;"`

## Fixes

- [ ] Fix the root cause identified during debugging (most likely in the CREATE path of `SyncPromptHistory` or in the frontend sync flow)
- [ ] Fix `processInterruptPrompt` (prompt_history_handlers.go:208-253) to call `MarkPromptAsSent` after successful delivery — matching the pattern used by `processPromptQueue` (websocket_external_agent_sync.go:2540-2543) and `processAnyPendingPrompt` (websocket_external_agent_sync.go:2593-2596)

## New Feature: Empty Enter promotes oldest queue message

- [ ] In `handleKeyDown` (RobustPromptInput.tsx), when Enter is pressed with empty draft, no attachments, and no Ctrl/Cmd key: find the oldest pending entry with `interrupt === false` from `pendingPrompts`, and call `updateInterrupt(entryId, true)` to promote it — this reuses the existing working Path C mechanism
- [ ] Skip promotion if Ctrl/Cmd is held (Ctrl+Enter with empty input should do nothing)
- [ ] Skip promotion if there are no queue-mode (`interrupt: false`) pending messages

## Testing

- [ ] Test Ctrl+Enter sends as interrupt (message interrupts current agent turn)
- [ ] Test toggle to interrupt mode + Enter sends as interrupt
- [ ] Test toggle to interrupt mode + click Send button sends as interrupt
- [ ] Test existing "switch to interrupt" button on queued message still works (regression check)
- [ ] Test that normal Enter (no Ctrl) with text sends as queue mode (interrupt=false)
- [ ] Test that Enter with empty input promotes the oldest queue-mode message to interrupt
- [ ] Test that Enter with empty input does nothing when there are no queue-mode pending messages
- [ ] Test that Ctrl+Enter with empty input does nothing (no promotion)
- [ ] Remove debug logging added during investigation
