# Requirements: Queue Deletion Flakiness & Interrupt Mode Disappearance

## User Stories

**US1: Queue deletion sticks**
As a user, when I delete an item from the prompt queue in the resilient prompt widget (`RobustPromptInput`), the item should not reappear after deletion. This includes surviving page reloads, API restarts, and async queue processing cycles.

**US2: Interrupt mode toggle is stable**
As a user, when I toggle a queued prompt to interrupt mode, that prompt should appear in Zed with its interrupt flag respected. It should not silently disappear (lost in transit) or appear in a different mode than intended.

**US3: E2E tests cover the interruption scenario**
As a developer, the WebSocket sync E2E test suite should include a test case that exercises the interrupt path end-to-end (Helix → Zed) so regressions are caught automatically.

## Acceptance Criteria

**Deletion:**
- AC1.1: Deleting a queued item in the frontend calls the backend to mark it deleted.
- AC1.2: A deleted item is never delivered to Zed, even if the API restarts between deletion and delivery.
- AC1.3: `processPendingPromptsForIdleSessions` skips items with a `deleted_at` timestamp.
- AC1.4: If an item is deleted after it was already sent to Zed (race), a cancellation is issued.

**Interrupt mode:**
- AC2.1: When a prompt with `interrupt=true` is sent, Zed receives and processes it as an interrupt.
- AC2.2: Toggling interrupt mode on an already-queued item updates the in-flight state consistently.
- AC2.3: The interrupt prompt appears in Zed (not silently dropped).

**E2E tests:**
- AC3.1: There is at least one E2E test phase that sends an interrupt message and verifies it arrives in Zed.
- AC3.2: The existing deletion/interrupt unit tests in `websocket_external_agent_sync_test.go` remain green.
