# Design: Chat Session Bug Fixes

## Architecture Overview

### Message paths
```
User queue (Enter)   → RobustPromptInput.handleSend
                     → usePromptHistory.saveToHistory (status=pending, interrupt=false)
                     → syncEntryImmediately → POST /api/v1/prompt-history/sync
                     → processPendingPromptsForIdleSessions (background goroutine)
                          ├─ idle?  → processAnyPendingPrompt → sendQueuedPromptToSession
                          └─ busy + interrupt? → processInterruptPrompt
                     ─ OR ─ after message_completed: processPromptQueue → sendQueuedPromptToSession

Approve action       → approveImplementation (HTTP handler)
                     → sendMessageToSpecTaskAgent
                          → CreateInteraction (I_pre, state=Waiting)
                          → sessionToWaitingInteraction[sessionID] = I_pre.ID
                          → sendChatMessageToExternalAgent → WebSocket → Zed

Zed response         → message_added(role=user) [echo of sent message]
                     → handleMessageAdded (user branch)
                          → CreateInteraction (I_echo) ← DUPLICATE!
                          → sessionToWaitingInteraction[sessionID] = I_echo.ID ← OVERWRITES!
                     → message_added(role=assistant)
                     → handleMessageAdded (assistant branch)
                          → getOrCreateStreamingContext → finds I_echo.ID
                          → response written to I_echo, NOT I_pre ← BUG 1
```

---

## Bug 1 Root Cause

### Double-interaction creation

`sendMessageToSpecTaskAgent` pre-creates interaction `I_pre` and records it in
`sessionToWaitingInteraction`. The Zed agent echoes the sent user message back as
`message_added(role=user)`. `handleMessageAdded`'s user-role branch unconditionally
creates a **second** interaction `I_echo` and **overwrites** the
`sessionToWaitingInteraction` mapping. When the assistant response arrives, it goes into
`I_echo`. `I_pre` is left orphaned in state=Waiting.

Key files:
- `api/pkg/server/spec_task_design_review_handlers.go` – `sendMessageToSpecTaskAgent` (lines ~1276–1298)
- `api/pkg/server/websocket_external_agent_sync.go` – `handleMessageAdded` user branch (lines ~1080–1124)

### Fix: Suppress user-echo interaction when a pre-created one exists

In `handleMessageAdded` (user-message branch), before creating a new interaction, check
whether `sessionToWaitingInteraction[helixSessionID]` already points to an existing
Waiting interaction for this session. If so:
- Do NOT create a new interaction.
- Do NOT overwrite `sessionToWaitingInteraction`.
- Optionally update the existing interaction's `PromptMessage` if it is empty/different.

This way the response for the approval message always lands in `I_pre`.

**Alternative considered**: Remove the pre-creation in `sendMessageToSpecTaskAgent` and let
`handleMessageAdded` create the interaction from the Zed echo. Rejected because:
- The pre-created interaction is necessary for the approve flow's request→interaction
  tracking (used by `finalizeCommentResponse`).
- Other callers of `sendMessageToSpecTaskAgent` rely on the returned interaction ID.

---

## Bug 2 Root Cause

### Eager queue processing on sync/list

`processPendingPromptsForIdleSessions` is called as a background goroutine in **both**:
- `syncPromptHistory` (called immediately when user presses Enter)
- `listPromptHistory` (polled every 2 s by the frontend when pending messages exist)

When the function runs, it checks whether the last session interaction is in state
`InteractionStateWaiting`. If the session is idle (`isIdle=true`), it calls
`processAnyPendingPrompt`, which calls `sendQueuedPromptToSession`. This immediately:
1. Creates an interaction in the DB (message appears in `EmbeddedSessionView`).
2. Sends the chat_message to Zed.

The race condition: Zed is still streaming a long response, so the last DB interaction may
still show state=Waiting. However, there is a timing window between `message_completed`
being processed and the next sync call. If the sync/poll happens right after
`message_completed` marks the interaction as Complete but before the user has seen the
response, the session appears idle and the next queued message is fired immediately,
showing up in the UI before the user has absorbed the previous response.

More importantly: the user's **intent** with queue mode is "buffer this; send it when the
agent is done with what it is currently working on." They expect the message to stay in the
`RobustPromptInput` queue, not appear in the chat, until the agent's response is complete.

### Fix: Remove queue-mode processing from sync/list; process only from message_completed

Queue-mode (`interrupt=false`) messages should be processed **exclusively** from
`handleMessageCompleted` → `processPromptQueue`.

Change `processPendingPromptsForIdleSessions` to only process **interrupt-mode** messages
when the session is idle (or when the session is busy with interrupt pending). Remove the
`processAnyPendingPrompt` call for the idle case.

The corrected logic in `processPendingPromptsForIdleSessions`:
```
if isIdle && pending.interruptCount > 0:
    processInterruptPrompt(sessionID)
elif !isIdle && pending.interruptCount > 0:
    processInterruptPrompt(sessionID)
# Queue-mode messages: do nothing here – message_completed will trigger processPromptQueue
```

For the edge case where a queue-mode message arrives while the session has been idle for a
long time (user hasn't interacted recently), `processPromptQueue` is called from
`handleMessageCompleted`. If there is no in-flight message (truly idle session), we need a
separate trigger. Options:
- **Option A (recommended)**: On `syncPromptHistory` / `listPromptHistory`, if the session
  is idle AND there are only queue-mode messages pending, call `processPromptQueue` (not
  `processAnyPendingPrompt`) so the semantics stay consistent. `processPromptQueue` already
  uses `GetNextPendingPrompt` which filters for `interrupt=false`.
- **Option B**: Keep the idle-send behaviour for queue-mode when truly idle (no Waiting
  interaction at all), but only trigger from `message_completed`. The 2-second poll is the
  fallback for server restarts.

Option A is simpler: just replace `processAnyPendingPrompt` with `processPromptQueue` in
the idle branch of `processPendingPromptsForIdleSessions`. This maintains the correct
semantics: queue-mode messages are always processed by `processPromptQueue`, never by the
"any pending" shortcut.

Key files:
- `api/pkg/server/prompt_history_handlers.go` – `processPendingPromptsForIdleSessions` (lines ~123–176)
- `api/pkg/server/websocket_external_agent_sync.go` – `processPromptQueue` (lines ~2082–2130), `handleMessageCompleted` (line 2075)

---

## Patterns Observed in Codebase

- **Interaction pre-creation pattern**: `sendQueuedPromptToSession` and
  `sendMessageToSpecTaskAgent` both create an interaction before sending the WebSocket
  command, then store the mapping in `sessionToWaitingInteraction`. This is the intended
  way to associate a response with an interaction.
- **Streaming context cache**: `getOrCreateStreamingContext` (websocket_external_agent_sync.go
  ~line 1137) detects interaction transitions via the `sessionToWaitingInteraction` mapping
  and resets the cache automatically. This logic is correct but depends on the mapping not
  being overwritten by the Zed echo.
- **Interrupt vs queue**: `interrupt=true` → immediate send (bypasses Waiting check),
  `interrupt=false` → wait for `message_completed` before sending. This is stored in the
  `prompt_history_entries` table and used by `GetNextInterruptPrompt` vs `GetNextPendingPrompt`.
- **Frontend queue display**: `usePromptHistory` shows entries with `status=pending` in the
  `RobustPromptInput` queue UI. Entries are marked `sent` by the backend via
  `MarkPromptAsSent` → polled by `listPromptHistory` every 2 s. Messages should only leave
  the queue UI after they are sent (interaction created and chat_message dispatched to Zed).
