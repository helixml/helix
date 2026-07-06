# Design: Unify All Agent Message Sending on the Session-Scoped Prompt Queue

## Summary

Generalise the prompt queue from **spec-task-scoped** to **session-scoped**, then
route **every** agent-message sender through it and delete the direct-dispatch
path. `interrupt` becomes the single knob: `false` = defer until idle, `true` =
cancel-then-send (boot-safe). This fixes the CI incident, gives org bots /
general session sends the busy-defer discipline they lack today, and removes the
duplicate.

## Current architecture (verified in tree)

- **Direct path** (poor): `sendChatMessageToExternalAgent`
  (`websocket_external_agent_sync.go:1955`) → `sendMessageToSession`
  (`spec_task_design_review_handlers.go:1741`) → `sendMessageToSpecTaskAgent`
  (`:1708`). No busy-check. Used by: the 4 automated spec-task senders
  (interrupt=false), the interrupt=true spec-task senders (comment reply, org
  transition, revision), **and** `sendSessionMessage` (`session_handlers.go:2324`)
  — which org bots call via `inProcHelixClient.SendMessage`
  (`helix_org_inproc.go:504`), always interrupt=false.
- **Queue path** (good, reliable): rows in `prompt_history_entries` →
  `processPendingPromptsForIdleSessions(specTaskID)`
  (`prompt_history_handlers.go:165`) → per-session `processPromptQueue` /
  `processInterruptPrompt` → `sendQueuedPromptToSession`
  (`websocket_external_agent_sync.go:3217`). Carries busy-defer, the
  thread-establishment boot barrier, and the PR #2808 orphaned-waiting reap.
- **Key enabler:** the *per-session* logic inside
  `processPendingPromptsForIdleSessions` already uses session-scoped store calls
  (`GetSession`, `ListInteractions` DESC/1, `GetNextPendingPrompt(sessionID)`,
  `processPromptQueue(sessionID)`). Only the *entry* (`ListPromptHistoryBySpecTask`
  + canonical-session filter) is spec-task-specific. So generalising is mostly an
  extraction, not a rewrite.
- **Offline boot:** `sendQueuedPromptToSession` → `sendCommandToExternalAgent`
  already triggers autostart on `ErrNoExternalAgentWS`; `pickupWaitingInteraction`
  delivers on reconnect. General sessions get this for free.

## Proposed changes

### 1. Schema — make the queue session-scoped
Make `prompt_history_entries.SpecTaskID` **nullable** (general/bot rows omit it).
The row's delivery unit is `SessionID` (already present). GORM AutoMigrate may not
relax `NOT NULL` reliably — include an explicit nullable-column migration if
needed (see Open Q1). No other consumer requires `SpecTaskID` non-null
(`SyncPromptHistory`/`ListPromptHistory` always set it from the frontend).

### 2. Store — single-row create
Add to `store.Store` + `PostgresStore` + regenerate `MockStore`:
```go
CreatePromptHistoryEntry(ctx context.Context, entry *types.PromptHistoryEntry) error
```
Session-scoped selectors (`GetNextPendingPrompt`, `GetNextInterruptPrompt`,
`GetAnyPendingPrompt`) already exist and need no change.

### 3. Extract a session-scoped poller
Refactor the per-session body of `processPendingPromptsForIdleSessions` into:
```go
func (apiServer *HelixAPIServer) processPendingPromptsForSession(ctx, sessionID string)
```
which does the busy-check → idle: `processPromptQueue`/`processInterruptPrompt`;
busy: boot barrier + PR #2808 reap. The existing
`processPendingPromptsForIdleSessions(specTaskID)` keeps the spec-task listing +
canonical filter, then calls `processPendingPromptsForSession` for the canonical
session. The new enqueue path calls it directly by session id.

### 4. Enqueue entry point + callback
API-server method:
```go
func (apiServer *HelixAPIServer) enqueueAgentMessage(
    ctx, sessionID, message string, interrupt bool, notifyUserID, specTaskID string) error
```
Resolves the session (owner, project), inserts a pending
`prompt_history_entries` row (`Interrupt`, `SessionID`, optional `SpecTaskID`,
`NotifyUserID`, user = spec-task `CreatedBy`/`Owner` or `session.Owner`), then
`go processPendingPromptsForSession(ctx, sessionID)`.

For `pkg/services` (store-decoupled) keep the callback pattern; a spec-task-shaped
enqueuer wraps it:
```go
type SpecTaskMessageEnqueuer func(ctx, task *types.SpecTask, message string, interrupt bool) error
```

### 5. Carry the commenter link on the row (for interrupt=true comment replies)
Add `NotifyUserID` to `prompt_history_entries`. At dispatch,
`sendQueuedPromptToSession` sets `requestToCommenterMapping[requestID]` /
`sessionToCommenterMapping[sessionID]` from the row (today `sendMessageToSession`
does this synchronously). This preserves design-review response streaming.

### 6. Re-plumb comment-reply finalization to resolve via PromptID
Today `sendCommentToAgentNow` stores `comment.RequestID`/`InteractionID`
synchronously and `finalizeCommentResponse(requestID)` →
`GetCommentByRequestID`. In the queue path `requestID == interaction.ID`, minted
at dispatch, and the interaction already persists `PromptID`. Change:
- enqueue the comment (interrupt=true) and link comment → prompt (store the
  prompt id on the comment);
- at `message_completed`, resolve interaction → `PromptID` → comment and finalize
  by copying `interaction.ResponseMessage`.
This removes the need for a synchronously-returned request/interaction id and is
what lets the direct path die. **Highest-risk change** — the finalize logic has
several safety nets (`auto_wake`, terminal-without-finalize fallbacks) that must
be kept working. (See Open Q3 on sequencing.)

### 7. Migrate every sender onto enqueue
| Sender | interrupt | notes |
|---|---|---|
| CI notifier (`spec_task_ci_notifier.go`) | true | replace `MessageSenderCINotifier` with enqueuer-backed notifier |
| Push (`spec_task_workflow_handlers.go:213`) | false | |
| Rebase (`:314`) | false | |
| Approval (`agent_instruction_service.go:673`) | false | enqueuer replaces `messageSender` field |
| Comment reply (`spec_task_design_review_handlers.go:1251`) | true | + §5/§6 |
| Design-review submit (`:403`) | true | + §5/§6 |
| Org transition (`spec_tasks_org_wiring.go:34`) | true | |
| Reviewer revision (`spec_driven_task_service.go:1457`) | true | |
| **User/bot send** (`session_handlers.go:2324`) | from body | enqueue; org bots inherit the fix |

### 8. Public endpoint becomes async
`sendSessionMessage` enqueues and returns the queue-entry id (async handle)
rather than `{request_id, interaction_id}`. Regenerate the API client
(`./stack update_openapi`) and update the CLI. (Open Q2.)

### 9. Delete dead code
- `sendChatMessageToExternalAgent`, `sendMessageToSession`,
  `sendMessageToSpecTaskAgent`.
- `MessageSenderCINotifier` / `NewMessageSenderCINotifier`.
- `SendImplementationReviewRequest`, `SendRevisionInstruction`,
  `SendMergeInstruction`, `AgentInstructionService.sendMessage`,
  `BuildImplementationReviewPrompt`, `BuildMergeInstructionPrompt` (unused;
  `BuildRevisionInstructionPrompt` stays).
- The `messageSender` field on `AgentInstructionService`.

## Key decisions & rationale

- **One disciplined path, keyed on the session.** The delivery unit was always
  the session; spec-task scoping was incidental. Generalising it gives bots and
  general sends the exact busy-defer, boot barrier, and desktop-resume reap that
  make the spec-task queue reliable — the root-cause fix, not a per-caller patch.
- **Bots need deferral, not interrupt.** A Worker's activations must complete in
  order; `interrupt=true` would cancel a Worker's own in-progress turn. So bots
  enqueue `interrupt=false`. (CI is the opposite — a human wants to know now —
  hence `interrupt=true`. Same path, different flag.)
- **Delete, don't narrow.** With the general endpoint on the queue too, no
  production caller needs the direct path; per repo rules it goes.

## Testing strategy

- **Build:** `CGO_ENABLED=0 go build ./...`.
- **Unit (gomock, suite pattern):** `processPendingPromptsForSession` idle→dispatch,
  busy→defer, interrupt=true busy+established→interrupt, busy+not-established→defer;
  enqueue writes the right row (interrupt, session, optional spec task, notify
  user); deleted symbols gone (compile-time).
- **Live E2E in inner Helix — the seam bots use:** create a session, start a long
  turn, then `POST /sessions/{id}/messages` with `interrupt=false` and confirm it
  is **held** and delivered only after completion — NOT a concurrent empty
  interaction. This exercises the exact path org bots depend on **without needing
  the org runtime**.
- **Live E2E spec task:** CI transition mid-turn cancels + delivers as one turn
  (not concurrent); push/rebase/approval held until idle; design-review comment
  reply still interrupts AND its response finalizes onto the comment.
- **Bot causation (Open Q4):** attempt a real org bot mid-turn overlap and show
  the empty concurrent interaction before, gone after.
- **CI:** push, check Drone via MCP tools, fix forward.

## Risks

- **Comment-reply reroute (§6)** is delicate — response streaming + finalize have
  safety nets; must be tested live end-to-end. Sequencing is Open Q3.
- **Public API contract change (§8)** — coordinate client + CLI regen.
- **Nullable migration (§1)** — verify AutoMigrate behaviour; add explicit alter
  if needed.
- **Silent drop if session id is wrong** — enqueue errors on empty session;
  spec-task canonical filtering preserved.

## References
- Incident: attachment `2026-07-06-ci-notification-concurrent-turn-mid-turn.md`
- Composes with: https://github.com/helixml/helix/pull/2808
- Boot-race hazard to preserve:
  `design/2026-06-19-incident-interrupt-during-boot-context-loss.md`
