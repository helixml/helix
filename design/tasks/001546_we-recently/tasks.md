# Implementation Tasks

## Bug 1: Response goes to wrong interaction after approval

- [ ] In `handleMessageAdded` (user-message branch, `websocket_external_agent_sync.go` ~line 1080), before creating a new interaction, check whether `sessionToWaitingInteraction[helixSessionID]` already holds an existing Waiting interaction ID for this session
- [ ] If a pre-created Waiting interaction exists, skip creating a new interaction and skip overwriting the `sessionToWaitingInteraction` mapping (reuse the pre-created one)
- [ ] Add a unit test in `websocket_external_agent_sync_test.go` covering the scenario: `sendMessageToSpecTaskAgent` sets a mapping → Zed echoes user message → assert mapping is NOT overwritten and only one interaction exists
- [ ] Verify the fix end-to-end: click Approve SPEC, confirm the agent response appears in the new interaction (not the previous one), and no orphaned Waiting interactions remain

## Bug 2: Queue-mode messages appear in Helix chat before agent is ready

- [ ] In `processPendingPromptsForIdleSessions` (`prompt_history_handlers.go` ~line 155), replace the `processAnyPendingPrompt` call in the idle branch with `processPromptQueue` so queue-mode messages use the same code path as the post-`message_completed` trigger
- [ ] Verify that `processPromptQueue` correctly handles the idle case (session idle, queue message pending) by calling `sendQueuedPromptToSession` and marking the prompt as sent
- [ ] Confirm that interrupt-mode messages are still processed eagerly (via `processInterruptPrompt`) even when the session is busy
- [ ] Add/update a unit test confirming that when the session is idle and only queue-mode prompts are pending, `processPendingPromptsForIdleSessions` dispatches via `processPromptQueue` (not `processAnyPendingPrompt`)
- [ ] Manual test: send a queue-mode message while the agent is streaming a long response; confirm the message stays in the `RobustPromptInput` queue until the agent completes, then moves to the chat
