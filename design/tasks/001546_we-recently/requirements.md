# Requirements: Chat Session Bug Fixes (Interaction Association & Queue Mode)

## Context

The spec task detail page has a chat panel backed by `EmbeddedSessionView` (which shows
Helix session interactions) and `RobustPromptInput` (which manages a local frontend queue).
Messages flow through `usePromptHistory` → backend `SyncPromptHistory` → `processPromptQueue`
or `processAnyPendingPrompt` → `sendQueuedPromptToSession` → Zed agent via WebSocket.

System-generated messages (e.g. "Your implementation has been approved") go through
`sendMessageToSpecTaskAgent` in `spec_task_design_review_handlers.go`.

---

## Bug 1: Response Goes to Wrong Interaction After Approval

### Observed Behaviour

When the user clicks "Approve SPEC" (or similar approve actions), the backend sends
"Your implementation has been approved" to the Zed agent. The agent's response is
displayed in the **previous** interaction in the chat UI instead of the **new** interaction
that represents the approval message.

### Expected Behaviour

The agent's response to the approval message must appear in the newly created interaction
that contains the prompt "Your implementation has been approved".

### Acceptance Criteria

- AC1: Clicking "Approve SPEC" creates exactly **one** visible interaction in the chat containing
  the "Your implementation has been approved" prompt.
- AC2: The agent's reply appears inside that same interaction, not in any previous interaction.
- AC3: No orphaned Waiting interactions remain after the response is received.
- AC4: The fix must not break the normal user-typed message flow (messages sent via
  `RobustPromptInput`).

---

## Bug 2: Queue-Mode Messages Appear in Helix Chat Before Agent is Ready

### Observed Behaviour

When the user sends a message in queue mode (default, Enter key, no Ctrl), the message
immediately appears in the Helix chat interface (session interaction list) even though the
agent has not yet finished its current task. The agent does eventually wait before
responding, but the interaction is visible in the chat from the moment the user presses Enter.

The user expects queue-mode messages to remain in the **frontend queue** (the "Message
queue" chip in `RobustPromptInput`) until the agent is done and ready to receive the
next message. Only at that point should the interaction appear in the session chat.

### Acceptance Criteria

- AC1: While the agent has a Waiting interaction, queue-mode (`interrupt=false`) messages
  must NOT appear as interactions in the `EmbeddedSessionView`.
- AC2: Queue-mode messages stay visible in the `RobustPromptInput` queue UI (status=pending)
  until the agent has finished the current task.
- AC3: Once the agent completes its current task (`message_completed`), the next queued
  message is sent, its interaction appears, and its status changes to `sent` in the queue UI.
- AC4: Interrupt-mode messages (`interrupt=true`, Ctrl+Enter) are unaffected – they continue
  to be sent immediately even while the agent is busy.
- AC5: If the session is idle (no current Waiting interaction) when a queue-mode message is
  synced, the message is sent immediately (unchanged behaviour for the idle case).
