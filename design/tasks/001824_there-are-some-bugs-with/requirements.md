# Requirements: Fix Interrupt Message Sending from Robust Prompt Input

## Problem Statement

Sending interrupt messages from the Robust Prompt input widget is broken for two of the three send methods. Only the post-queue "switch to interrupt" button works.

## User Stories

### US-1: Ctrl+Enter sends with interrupt
**As a** user with a running agent session,
**I want to** press Ctrl+Enter (or Cmd+Enter on Mac) in the prompt input to send my message as an interrupt,
**So that** I can quickly interrupt the agent without toggling UI mode.

**Acceptance Criteria:**
- Pressing Ctrl+Enter in the textarea sends the message with `interrupt: true`
- The message appears in the queue with the interrupt (zap) icon
- The backend receives and processes it as an interrupt prompt
- The agent's current turn is interrupted and the new message is processed

### US-2: Toggle to interrupt mode + send
**As a** user,
**I want to** toggle the send mode to "Interrupt Mode" via the UI button, then send my message (via Enter or clicking send),
**So that** my message interrupts the current agent conversation.

**Acceptance Criteria:**
- Clicking the mode toggle button switches to "Interrupt Mode" (zap icon, warning color)
- Pressing Enter or clicking Send while in interrupt mode sends with `interrupt: true`
- The message is processed as an interrupt by the backend
- The agent's current turn is interrupted and the new message is processed

### US-3: Existing "switch to interrupt" on queued message continues to work
**As a** user,
**I want** the existing toggle-interrupt button on a queued message to continue working,
**So that** I can change a queued message to interrupt after the fact.

**Acceptance Criteria:**
- This path already works — ensure no regressions
