# Design: Spec Comments Send `interrupt=true`

## Goal

Make every spec-task design-review comment delivered to the agent over WebSocket carry `interrupt=true`, so the agent cancels its current turn and processes the comment immediately. Approval and workflow messages remain non-interrupting.

## Current Code Path

```
createDesignReviewComment (handler)
  → sendCommentToAgent (wrapper)
    → queueCommentForAgent          (sets QueuedAt, kicks queue)
      → processNextCommentInQueue   (serializes per-session)
        → sendCommentToAgentNow
          → sendMessageToSpecTaskAgent(ctx, specTask, prompt, commenterID)
            → sendChatMessageToExternalAgent(sessionID, message, requestID)
              → sendCommandToExternalAgent(... ExternalAgentCommand{Type: "chat_message", Data: {...}})
```

`respondToDesignReview` (the "request changes" path on a review submission) reaches the agent via the same `sendMessageToSpecTaskAgent` — see `spec_task_design_review_handlers.go:378`.

The "interrupt" semantic already exists on the prompt-queue path: `websocket_external_agent_sync.go:2845` includes `"interrupt": prompt.Interrupt` in the `chat_message` data. The agent (Zed / Claude / Qwen) interprets this as "cancel the current turn, then send the new message." The path used by spec comments does NOT include this field today.

## Decision

Plumb an `interrupt` flag through `sendMessageToSpecTaskAgent` → `sendChatMessageToExternalAgent` and have spec comment callers pass `true`. Approval/workflow callers pass `false` (preserving current behaviour).

### Why this approach over alternatives

- **Add a parameter, don't add a side field on `Comment`**: `interrupt` is a transport concern (how the WebSocket command is shaped), not a per-comment property. There is no use case for storing it on the comment row.
- **Don't reroute spec comments through the prompt-history queue**: the comment queue already exists with its own serialization, request-id tracking, response linking, and timeout. Reusing the prompt queue would be a much larger refactor for no functional benefit — `interrupt=true` is the only piece of behaviour the comment path is missing.
- **Don't hardcode `interrupt=true` inside `sendChatMessageToExternalAgent`**: the same function is also used by approval and workflow flows (`sendApprovalInstructionToAgent`, `spec_task_workflow_handlers.go`) where interrupting would be wrong. A parameter at the seam is the correct granularity.

## Changes

### 1. `sendChatMessageToExternalAgent` — add `interrupt` parameter

File: `api/pkg/server/websocket_external_agent_sync.go` (around line 1712).

```go
// Before
func (apiServer *HelixAPIServer) sendChatMessageToExternalAgent(sessionID, message, requestID string) (interactionID string, err error)

// After
func (apiServer *HelixAPIServer) sendChatMessageToExternalAgent(sessionID, message, requestID string, interrupt bool) (interactionID string, err error)
```

Include the field in the command data (mirroring the queue path at line 2845):

```go
command := types.ExternalAgentCommand{
    Type: "chat_message",
    Data: map[string]interface{}{
        "message":       message,
        "request_id":    requestID,
        "acp_thread_id": acpThreadID,
        "agent_name":    agentName,
        "interrupt":     interrupt,
    },
}
```

### 2. `sendMessageToSpecTaskAgent` — add `interrupt` parameter

File: `api/pkg/server/spec_task_design_review_handlers.go` (around line 1470).

```go
// Before
func (s *HelixAPIServer) sendMessageToSpecTaskAgent(
    ctx context.Context,
    specTask *types.SpecTask,
    message string,
    notifyUserID string,
) (string, string, error)

// After
func (s *HelixAPIServer) sendMessageToSpecTaskAgent(
    ctx context.Context,
    specTask *types.SpecTask,
    message string,
    notifyUserID string,
    interrupt bool,
) (string, string, error)
```

Forward `interrupt` to `sendChatMessageToExternalAgent`.

### 3. Update callers

| Caller | File | Pass | Reason |
|---|---|---|---|
| `sendCommentToAgentNow` | `spec_task_design_review_handlers.go:1021` | `interrupt=true` | Comment feedback on live turn |
| `respondToDesignReview` (request-changes branch) | `spec_task_design_review_handlers.go:378` | `interrupt=true` | Reviewer-driven feedback, same semantic as a comment |
| `sendApprovalInstructionToAgent` | `spec_task_design_review_handlers.go:1572` | `interrupt=false` | Sent only when agent is idle; new phase, not feedback |
| Auto-revision / workflow sends | `spec_task_workflow_handlers.go:211, 294` | `interrupt=false` | System-driven, not reactive feedback |

### 4. Server bootstrap binding

`api/pkg/server/server.go:489` assigns `apiServer.sendMessageToSpecTaskAgent` to `specDrivenTaskService.SendMessageToAgent`. The function signature on `SendMessageToAgent` (in `services/spec_driven_task_service.go` or similar) must be updated to match — confirm during implementation, and update the field type.

### 5. Tests

- Update `api/pkg/server/test_helpers.go:74` and any other internal callers to match the new signature.
- Update `websocket_external_agent_sync_test.go` cases that call `sendChatMessageToExternalAgent` directly.
- Add a focused unit test verifying that `sendChatMessageToExternalAgent` includes `"interrupt": true` in the outgoing `chat_message` data when invoked with `interrupt=true`, and `false` (or omitted-as-false) otherwise. Prefer asserting on the captured `ExternalAgentCommand.Data` map via the existing test infrastructure.

## Risks & Edge Cases

- **Agent receives interrupt with no in-flight turn.** Existing agent-side code already tolerates this (a no-op cancel followed by normal message handling). No change needed.
- **Comment queue still serializes.** A reviewer rapidly submitting three comments still gets them processed one at a time per session; each one arrives flagged `interrupt=true`, but only the first actually interrupts in-flight work — the next two arrive while the agent is responding to the previous comment and effectively interrupt that previous comment-response. This matches the user's intent: the latest feedback always takes priority.
- **Race: comment arrives while agent is already cancelling.** The interrupt code path in `websocket_external_agent_sync.go:1427-1454` already handles "interrupt arrives before message_completed" via auto-completion logic. No new handling required.
- **`from_queue` field.** The prompt-history path also sets `"from_queue": true`. The comment path does not need this — it is a separate queue and the field has no consumer for non-prompt-queue messages.

## Verification

- `go build ./api/pkg/server/ ./api/pkg/services/` clean.
- New unit test passes.
- Manual end-to-end in inner Helix per requirements §6: start a long-running agent turn, drop a comment, observe the prior turn cancel and the comment response begin within a few seconds.
- Existing E2E (`./run_docker_e2e.sh`) still passes — it covers interrupt semantics on the prompt-queue path, which this change does not touch.

## Notes for Future Implementers

- `prompt.Interrupt` in the prompt-history flow defaults to `false` (queue mode). The spec-comment flow does not have an analogous defaulting concern: every spec-comment caller will explicitly pass `true`.
- If a future requirement adds a non-interrupting "FYI" comment type, the parameter is already in place — the comment row would gain a column and the comment-handler would forward it.
- Keep the `interrupt` parameter required (no default) on `sendChatMessageToExternalAgent`. Required parameters force every new caller to make an explicit choice instead of accidentally inheriting "non-interrupt" behaviour.
