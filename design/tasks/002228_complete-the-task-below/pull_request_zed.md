# Add e2e phases for the production prompt-queue path

## Summary

The WebSocket-sync e2e drove every phase through the low-level `SendChatMessage`
primitive. Since Helix now sends all agent messages through the session-scoped
prompt queue (see the companion helix PR), that primitive is no longer the
production send path. This adds two phases that exercise the real production
queue path (`EnqueueQueuedPrompt` → `processPendingPromptsForSession` →
`processPromptQueue` / `processInterruptPrompt` → `sendQueuedPromptToSession`)
against a real Zed binary.

## Changes

- **Phase 16 — queue busy-defer:** start a turn, then enqueue an `interrupt=false`
  message while it streams; assert it is HELD (no concurrent interaction) and
  delivered as the next turn once idle (the concurrent-mid-turn incident, fixed).
- **Phase 17 — queue interrupt:** enqueue an `interrupt=true` message while a turn
  streams; assert it cancels the running turn (interaction → interrupted) and its
  own message is delivered.
- Phases poll the store (queue prompts get generated request_ids that don't flow
  through the completion switch) and wait for the first turn to stream before the
  second enqueue.
- go.mod/go.sum: re-synced with the current helix dependency graph.

Runs green locally (`run_docker_e2e.sh`, zed-agent): all 17 phases + store
validations PASSED.

Depends on the helix PR (test helper `EnqueueQueuedPrompt` + in-memory prompt
queue in `memorystore`); `sandbox-versions.txt` `ZED_COMMIT` is bumped there to
this commit.

Release Notes:

- N/A
