# Implementation Tasks

## Bug 1: Queue deletion reappearance

- [ ] Add `deleted_at *time.Time` field to `PromptHistoryEntry` in store types and write a DB migration
- [ ] Add `DELETE /api/v1/sessions/:sessionID/prompt-queue/:entryID` API endpoint that sets `deleted_at`
- [ ] Update `processPendingPromptsForIdleSessions` (prompt_history_handlers.go) to skip entries where `deleted_at IS NOT NULL`
- [ ] When rebuilding / popping from `sessionToWaitingInteraction`, skip deleted entries
- [ ] Update `RobustPromptInput.handleRemoveFromQueue` to call the new DELETE endpoint before removing from localStorage
- [ ] Write unit tests for the DELETE endpoint and for the queue processor skipping deleted entries

## Bug 2: Interrupt-mode prompts disappear in Zed

- [ ] Trace the exact code path in `websocket_external_agent_sync.go` when `interrupt=true` — document whether cancel is sent before or after the new `chat_message` command
- [ ] Reorder: send `chat_message` command to Zed *before* issuing the ACP cancel, so Zed queues the new message first
- [ ] If Zed's `thread_service.rs` drops buffered commands during cancel teardown, add a pending-command buffer that survives the cancel cycle
- [ ] Verify the fix manually: toggle a queued prompt to interrupt, confirm it appears in Zed

## Bug 3: E2E test coverage for interrupt

- [ ] Add Phase 6 to `zed/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go`:
  - Send a normal message, wait for Zed to start responding (first `message_added`)
  - Send an interrupt message before response completes
  - Assert interrupt message is delivered (new `thread_created` or `message_added` for the interrupt)
  - Assert original response is canceled / no `message_completed` for original interaction
- [ ] Run the full E2E suite (`run_docker_e2e.sh`) and confirm all phases pass including new Phase 6
- [ ] Fix any flakiness in existing phases exposed during the run (timing / missing waits)
