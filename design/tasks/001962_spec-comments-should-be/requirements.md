# Requirements: Spec Comments Should Be `interrupt=true`

## Context

When a reviewer leaves a comment on a spec task design document (requirements.md / design.md / tasks.md), the comment is sent to the planning agent via WebSocket. Today the comment is delivered as a normal `chat_message` (no `interrupt` flag). If the agent is mid-turn — for example, still streaming a response to a previous comment, or working on auto-revisions — the new comment sits in the per-comment queue and waits for the current turn to complete.

This delays feedback substantially, especially when reviewers want to redirect the agent's work in flight. The prompt-history flow already supports an `interrupt=true` semantic that cancels the agent's current turn before sending the new prompt. Spec comments should use the same semantic.

## User Story

**As a** spec task reviewer leaving inline or general comments on a design document,
**I want** my comment to interrupt the agent if it is currently working,
**So that** the agent stops, processes my feedback, and reorients its next turn around it instead of finishing a now-stale response first.

## Acceptance Criteria

1. When a design review comment is dispatched to the agent (via `sendCommentToAgentNow` → `sendMessageToSpecTaskAgent` → `sendChatMessageToExternalAgent`), the resulting `chat_message` WebSocket command MUST include `"interrupt": true` in its `Data` payload.
2. The behaviour MUST apply to all comment types currently routed through the comment queue: inline comments, general comments, and the "request changes" overall feedback message sent from `respondToDesignReview`.
3. Approval-flow messages sent via `sendApprovalInstructionToAgent` (post-approval implementation kickoff) MUST NOT be flagged `interrupt=true` — they are sent only when the agent is idle and represent a new phase, not feedback on the current turn.
4. Workflow messages sent from `spec_task_workflow_handlers.go` (auto-revision, status-change notifications) MUST NOT be flagged `interrupt=true` unless explicitly required by their callers — they are not user feedback on a live turn.
5. The existing per-comment serialization (one comment processed per session at a time, tracked via `RequestID`/`QueuedAt`) MUST be preserved. Adding `interrupt=true` changes how Zed handles the message on receipt; it does NOT change how Helix queues comments behind one another.
6. End-to-end behaviour, verified manually in the inner Helix:
   - Start a spec task, wait for the agent to begin streaming a long response.
   - Leave a new inline comment on the design doc.
   - The agent's current turn cancels promptly (visible in the chat as an interrupted message), and the agent begins responding to the new comment within seconds rather than after the previous turn would have completed.
7. The Zed/Qwen/Claude agent side already understands `interrupt=true` on `chat_message` (it's the same field consumed today from `prompt.Interrupt` at `websocket_external_agent_sync.go:2845`). No agent-side change is required as part of this task.

## Out of Scope

- Adding any UI control for the reviewer to choose between "interrupt" and "queue". All comments default to interrupt.
- Changing the comment queue serialization model.
- Changing how the agent acknowledges or recovers from an interrupted turn (already handled by existing interrupt code paths in `websocket_external_agent_sync.go`).
- Any change to the prompt-history queue for normal user chat messages.
