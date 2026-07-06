# Design: Route Automated interrupt=false Agent Messages Through the Prompt Queue

## Summary

Add a small **enqueue** path that services can call to insert a pending
`prompt_history_entries` row (interrupt=false) and nudge the existing poller —
the same mechanism `syncPromptHistory` uses for user queue-mode messages.
Migrate the four automated `interrupt=false` senders onto it, then delete the
now-dead direct-dispatch duplicate.

## Current architecture (as verified in the tree)

### The GOOD path (keep, reuse)
- `syncPromptHistory` (`prompt_history_handlers.go:66`) writes rows via
  `Store.SyncPromptHistory`, then fires
  `go processPendingPromptsForIdleSessions(ctx, specTaskID)`.
- `processPendingPromptsForIdleSessions` (`prompt_history_handlers.go:165`):
  - filters to the canonical `task.PlanningSessionID`;
  - loads the newest interaction (`Order: id DESC, PerPage: 1`);
  - **idle** (no interaction or latest not `waiting`) → `processInterruptPrompt`
    (if interrupts pending) else `processPromptQueue`;
  - **busy** → defer, except: interrupts fire if thread established (boot
    barrier); and the PR #2808 orphaned-waiting reap guard
    (`isOrphanedWaitingInteraction`) resumes a dead desktop.
- `processPromptQueue` (`websocket_external_agent_sync.go:3067`) re-checks busy,
  claims the next pending non-interrupt prompt, calls
  `sendQueuedPromptToSession`.
- `sendQueuedPromptToSession` (`:3217`) creates the `waiting` interaction with
  `PromptID` set, then `sendCommandToExternalAgent`. On `ErrNoExternalAgentWS`
  it has **already kicked off autostart**; `pickupWaitingInteraction` delivers
  on reconnect. So the queue path already boots stopped desktops — no extra work
  needed for the offline case.

### The POOR path (narrow / delete)
- `sendChatMessageToExternalAgent` (`websocket_external_agent_sync.go:1955`) —
  primitive; creates interaction + emits `chat_message` immediately, no
  busy-check.
- `sendMessageToSession` (`spec_task_design_review_handlers.go:1741`) — wraps it,
  adds requestID + commenter mapping.
- `sendMessageToSpecTaskAgent` (`:1708`) — resolves session, wraps
  `sendMessageToSession`.

### The four automated senders (all interrupt=false today)
1. CI notifier — `services/spec_task_ci_notifier.go:50`
   (`MessageSenderCINotifier` calls `SpecTaskMessageSender(..., false)`; wired in
   `server.go:631`; invoked from `spec_task_orchestrator_ci.go:115`).
2. Post-merge **push** — `server/spec_task_workflow_handlers.go:213`.
3. Post-merge-failure **rebase** — `server/spec_task_workflow_handlers.go:314`.
4. Approval kickoff — `services/agent_instruction_service.go:673`
   (`SendApprovalInstruction` via `s.messageSender(..., false)`).

### Audit findings (the three "confirm" methods)
`SendImplementationReviewRequest` / `SendRevisionInstruction` /
`SendMergeInstruction` (`agent_instruction_service.go:759/779/799`) have **no
production callers** (only tests, if any). They use a **third** path —
`AgentInstructionService.sendMessage` → `store.CreateInteraction` directly (no
WebSocket, no queue). They are already dead code → delete them.
`BuildRevisionInstructionPrompt` is still used by interrupt callers (keep);
`BuildImplementationReviewPrompt` / `BuildMergeInstructionPrompt` become unused
→ delete.

### Constraint discovered: the direct path cannot be fully deleted
`session_handlers.go:2324` (user-send endpoint, on the do-not-touch list) calls
`sendMessageToSession(..., body.Interrupt)` with a **user-controlled** flag, so
`sendMessageToSession` and `sendChatMessageToExternalAgent` must keep their
`interrupt` parameter. Interrupt-true callers (`org_wiring:34`,
`design_review:403,1251`) also keep using `sendMessageToSpecTaskAgent`. So we
**narrow** the direct path (remove its interrupt=false automated reachability),
not delete it wholesale.

## Proposed changes

### 1. New store method — create a single pending prompt row
Add to the `store.Store` interface + `PostgresStore` + regenerate `MockStore`:

```go
CreatePromptHistoryEntry(ctx context.Context, entry *types.PromptHistoryEntry) error
```

Thin `gdb.Create` wrapper (the `BeforeCreate` hook already stamps timestamps).
For CI coalescing (optional, see §5), also add:

```go
// Appends newLine to an existing pending, not-yet-claimed CI entry for the
// session; returns true if it coalesced, false if none was found.
CoalescePendingCINotification(ctx context.Context, sessionID, newLine string) (bool, error)
```

### 2. New enqueue callback (keep `pkg/services` decoupled from the store)
Mirror the existing `SpecTaskMessageSender` callback pattern. In
`services/git_http_server.go` add:

```go
// SpecTaskMessageEnqueuer inserts an interrupt=false prompt into the queue for a
// spec task's canonical planning session and nudges the poller. Delivery is
// deferred until the agent is idle. No IDs are returned — dispatch is async.
type SpecTaskMessageEnqueuer func(ctx context.Context, task *types.SpecTask, message string) error
```

Implement on the API server (e.g. in `prompt_history_handlers.go`):

```go
func (apiServer *HelixAPIServer) enqueueSpecTaskAgentMessage(ctx, task, message) error {
    sessionID := task.PlanningSessionID           // canonical target (poller filter)
    userID := task.CreatedBy; if "" { task.Owner }
    entry := &types.PromptHistoryEntry{
        ID: system.Generate...(), UserID: userID, ProjectID: task.ProjectID,
        SpecTaskID: task.ID, SessionID: sessionID, Content: message,
        Status: "pending", Interrupt: false,
    }
    if err := apiServer.Store.CreatePromptHistoryEntry(ctx, entry); err != nil { return err }
    go apiServer.processPendingPromptsForIdleSessions(context.Background(), task.ID)
    return nil
}
```

Guard: if `sessionID == ""` return a clear error (nothing to target).

### 3. Wire the callback (server.go)
- Add `EnqueueMessageToAgent SpecTaskMessageEnqueuer` to `SpecDrivenTaskService`;
  set `apiServer.specDrivenTaskService.EnqueueMessageToAgent =
  apiServer.enqueueSpecTaskAgentMessage`.
- Replace `NewMessageSenderCINotifier(apiServer.sendMessageToSpecTaskAgent)` with
  an enqueue-based notifier constructed from `enqueueSpecTaskAgentMessage`.
- `NewAgentInstructionService(...)` gets the enqueuer instead of `messageSender`.

### 4. Migrate the four senders
1. **CI notifier**: replace `MessageSenderCINotifier` with a notifier that holds
   a `SpecTaskMessageEnqueuer` and calls it in `NotifyCIResult`. Delete
   `MessageSenderCINotifier` / `NewMessageSenderCINotifier`.
2. **Push** (`spec_task_workflow_handlers.go:213`): call
   `s.enqueueSpecTaskAgentMessage(ctx, specTask, message)`.
3. **Rebase** (`:314`): same.
4. **Approval** (`agent_instruction_service.go:673`): call the enqueuer; drop the
   `s.messageSender` field once unused.

### 5. CI coalescing (design decision — see Open Questions)
**Recommended:** coalesce consecutive *pending, not-yet-dispatched* CI entries
for the same session so N CI transitions during one long turn drain as one
"here's what happened while you were working" message. Push/rebase/approval do
**not** coalesce (distinct, ordered).

Mechanism: mark CI entries with a sentinel in `Tags` (e.g. `["__ci__"]`, no
schema change). The CI enqueue path calls `CoalescePendingCINotification`; on
`false` it creates a fresh tagged entry. Only coalesce `status='pending'` rows
(never one already `sending`/`sent`) so we never mutate an in-flight turn.

If coalescing is deferred (answer to Open Q1 = in-order), skip §5 entirely and
the CI notifier just enqueues each result as its own row — still correct, just
chattier.

### 6. Delete dead code
- `SendImplementationReviewRequest`, `SendRevisionInstruction`,
  `SendMergeInstruction`, `AgentInstructionService.sendMessage` (now unused),
  `BuildImplementationReviewPrompt`, `BuildMergeInstructionPrompt` (+ templates).
- `MessageSenderCINotifier` / `NewMessageSenderCINotifier`.
- If Open Q5 = yes: drop the `interrupt` param from `sendMessageToSpecTaskAgent`
  (hardcode cancel-first for its interrupt-only callers). Keep the param on
  `sendMessageToSession` / `sendChatMessageToExternalAgent` (user-send endpoint).

## Key decisions & rationale

- **Reuse the queue, don't add a second deferral mechanism.** The poller already
  encodes busy-defer, the boot barrier, and the PR #2808 reap guard. Enqueue +
  nudge inherits all of it. This is the design doc's preferred fix.
- **Callback, not direct store access, from `pkg/services`.** Matches the
  existing `SpecTaskMessageSender` wiring and keeps services store-agnostic.
- **Narrow, don't delete, the direct path.** The user-send endpoint and
  interrupt-true comment flows genuinely need immediate/synchronous dispatch.
- **Target the canonical planning session.** The poller ignores non-canonical
  sessions, so enqueue must match or messages would be silently dropped.

## Testing strategy

- **Build:** `CGO_ENABLED=0 go build ./...`.
- **Unit (gomock store, suite pattern):**
  - enqueue creates a pending interrupt=false row for the canonical session and
    nudges the poller;
  - busy session (latest interaction `waiting`) → poller defers (no interaction
    created);
  - idle session → dispatched via `processPromptQueue`;
  - CI coalescing (if built): second CI enqueue while a pending CI row exists
    appends instead of creating a second row; push/rebase never coalesce;
  - the three deleted methods are gone (compile-time).
- **Live E2E in the inner Helix (mandatory — lifecycle code):** create a spec
  task, drive it to a live established thread, start a long turn, simulate a CI
  transition, confirm the message is **held** and delivered only after the turn
  completes (reproduce the incident shape and show it no longer occurs). Verify
  the four `interrupt=true` paths still interrupt.
- **CI:** push, then check Drone via MCP tools; fix forward.

## Files touched (estimate)

| File | Change |
|---|---|
| `api/pkg/types/prompt_history.go` | (only if adding a Kind column — prefer not) |
| `api/pkg/store/store.go` | +`CreatePromptHistoryEntry` (+`CoalescePendingCINotification`) |
| `api/pkg/store/store_prompt_history.go` | impl |
| `api/pkg/store/store_mocks.go` | regen |
| `api/pkg/services/git_http_server.go` | +`SpecTaskMessageEnqueuer` type |
| `api/pkg/services/spec_task_ci_notifier.go` | enqueue-based notifier; delete old |
| `api/pkg/services/agent_instruction_service.go` | approval→enqueue; delete dead methods/builders |
| `api/pkg/services/spec_driven_task_service.go` | +`EnqueueMessageToAgent`; pass enqueuer to instruction service |
| `api/pkg/server/prompt_history_handlers.go` | +`enqueueSpecTaskAgentMessage` |
| `api/pkg/server/spec_task_workflow_handlers.go` | push/rebase → enqueue |
| `api/pkg/server/server.go` | wire enqueuer + new CI notifier |
| `api/pkg/server/spec_task_design_review_handlers.go` | (only if dropping interrupt param) |
| `*_test.go` | new suite + update helpers referencing deleted symbols |

## Risks

- **Silent drop if `SessionID` mismatches canonical.** Mitigate: enqueue targets
  `PlanningSessionID`; error on empty.
- **Latency change for the idle case.** Enqueue + async poller vs. direct send —
  the poller runs immediately (nudged) and on every sync/list poll; acceptable.
- **Deleting methods used only by tests.** Update/remove those tests in the same
  PR.
- **Coalescing edge:** never touch a `sending`/`sent` row (would corrupt an
  in-flight turn) — enforced by the `status='pending'` filter.

## References
- Incident: attachment `2026-07-06-ci-notification-concurrent-turn-mid-turn.md`
- Composes with reap fix: https://github.com/helixml/helix/pull/2808
- Boot-race hazard to preserve:
  `design/2026-06-19-incident-interrupt-during-boot-context-loss.md`
