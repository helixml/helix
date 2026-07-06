# Requirements: Route Automated interrupt=false Agent Messages Through the Prompt Queue

## Background

Helix has **two parallel implementations** of "create a waiting interaction and
send a `chat_message` to the Zed/ACP agent." Both bottom out on the same
`sendCommandToExternalAgent` primitive, but only one carries the busy/idle
discipline.

- **GOOD — the prompt-queue path** (`prompt_history_entries` rows →
  `processPendingPromptsForIdleSessions` → `processPromptQueue` /
  `processInterruptPrompt` → `sendQueuedPromptToSession`). Honors `interrupt`
  as first-class: `interrupt=true` cancels the current turn and sends;
  `interrupt=false` is **held until the latest interaction is not `waiting`**
  (deferred until idle). Also carries the thread-establishment boot barrier and
  the orphaned-waiting reap guard (PR #2808).
- **POOR — the direct path** (`sendChatMessageToExternalAgent` →
  `sendMessageToSession` → `sendMessageToSpecTaskAgent`). Creates the waiting
  interaction and emits the `chat_message` **immediately, with no busy-check and
  no deferral**. `interrupt` here only chooses cancel-first (true) vs send-now
  (false); **neither value defers until idle**.

Four automated, non-urgent senders pass `interrupt=false` believing it queues
behind the in-flight turn. It does not — they fire a concurrent `session/prompt`
mid-turn, creating a second empty `waiting` interaction that leans on Zed's
fragile ACP message-pump. This caused a production incident (see attachment
`2026-07-06-ci-notification-concurrent-turn-mid-turn.md`).

## Goal

Converge the automated `interrupt=false` senders onto the queue path so
`interrupt=false` finally means the same thing everywhere — "defer until the
agent is idle" — then delete the now-dead poor-duplicate code.

## User Stories

### US1 — CI results never barge into a running turn
As a user watching a spec-task agent work a long turn, when CI passes/fails on a
PR, I want the CI notification to be **held and delivered only after the current
turn completes**, not injected as a second empty `waiting` card while the real
work streams into the previous one.

**Acceptance criteria**
- A CI transition fired while the latest interaction is `waiting` creates **no**
  new interaction until the running turn completes.
- Once the agent goes idle, the queued CI message is delivered as the next turn.
- If the agent is already idle, the CI message is delivered promptly (queue
  dispatch, no regression vs today's latency in the common case).
- If the desktop is stopped/offline, the queued CI message boots it cleanly (via
  the existing queue → `sendCommandToExternalAgent` → autostart path) and is
  delivered on reconnect — never stranded as a concurrent dispatch.

### US2 — Post-merge push/rebase instructions respect the queue
As a user who approves an implementation while the agent is mid-turn, I want the
post-merge **push** instruction (and the post-merge-failure **rebase**
instruction) to queue behind the in-flight turn and deliver when idle.

**Acceptance criteria**
- Both instructions are enqueued (interrupt=false) and deferred while busy.
- They are delivered in order once idle; they are **not** coalesced with each
  other or with CI results.

### US3 — Approval kickoff respects the queue
As the system starting the implementation phase, I want the approval kickoff
instruction to be enqueued so that on the rare occasion the agent is not idle,
it defers instead of dispatching concurrently.

**Acceptance criteria**
- `SendApprovalInstruction` enqueues (interrupt=false) rather than dispatching
  directly.
- Delivered as the next turn when the agent is idle.

### US4 — Deliberate interrupts still interrupt
As a reviewer submitting revision feedback / a design-review comment / an org
status transition, I want my message to interrupt the current turn immediately
(these are `interrupt=true` today and must stay so).

**Acceptance criteria**
- `spec_driven_task_service.go:1457`, `spec_tasks_org_wiring.go:34`,
  `spec_task_design_review_handlers.go:403` & `:1251`, and the user-send endpoint
  `session_handlers.go:2324` are **unchanged in behaviour**.
- The interrupt-during-boot barrier
  (`design/2026-06-19-incident-interrupt-during-boot-context-loss.md`) is not
  regressed.

### US5 — No dead code / no duplicate deferral mechanism
As a maintainer, I want the poor-duplicate direct-dispatch path removed once no
non-interrupt caller reaches it, per repo rules (no fallbacks, clean up dead
code).

**Acceptance criteria**
- The three unused instruction methods
  (`SendImplementationReviewRequest`, `SendRevisionInstruction`,
  `SendMergeInstruction`) and their now-unused helpers/prompt builders are
  deleted.
- `MessageSenderCINotifier` (the direct-sender CI notifier) is replaced by an
  enqueue-based notifier; the old one is deleted.
- The direct path is narrowed so `interrupt=false` is no longer reachable from
  automated senders. What remains of the direct path exists only for genuine
  `interrupt=true` callers and the user-controlled send endpoint.
- `CGO_ENABLED=0 go build ./...` passes; no unused symbols remain.

## Non-Goals

- Rewriting response routing for the `interrupt=true` comment-reply paths (they
  need a synchronous interactionID + commenter mapping the async queue can't
  provide). The direct path stays for them.
- Changing the user-send endpoint `session_handlers.go:2324` (it forwards a
  user-controlled `interrupt` flag — must be preserved).
- Changing the offline/reconnect delivery semantics (queue path already handles
  no-WS via autostart + `pickupWaitingInteraction`).

## Open Questions

1. **Coalescing CI results.** The brief prefers coalescing multiple CI
   transitions that stack during one long turn into a single "here's what
   happened while you were working" message, keeping push/rebase distinct. Do
   you want coalescing implemented in this PR, or is strict in-order delivery of
   separate CI messages acceptable for v1? (Design proposes lightweight
   coalescing of consecutive *pending, not-yet-dispatched* CI entries for the
   same session; recommend this but it adds a store method + a way to mark CI
   entries.)
2. **How to mark CI entries for coalescing.** Proposed: a sentinel value in the
   existing `Tags` column (no schema change) vs. adding a dedicated
   `Kind`/`Source` column to `prompt_history_entries`. Recommend the Tags
   sentinel to avoid a migration — acceptable?
3. **Enqueue target session.** The queue only delivers to the canonical
   `task.PlanningSessionID` (poller filters non-canonical sessions). The enqueue
   path will therefore target `PlanningSessionID`, not
   `findConnectedSessionForSpecTask`. For approval kickoff the reused session is
   the planning session, so this matches — please confirm no automated sender
   needs to target a non-planning session.
4. **User attribution on enqueued rows.** `prompt_history_entries.user_id` is
   `NOT NULL`. Plan: use `task.CreatedBy` (fallback `task.Owner`). CI results
   aren't tied to a commenter; is attributing them to the task creator fine?
5. **Dropping the `interrupt` param from `sendMessageToSpecTaskAgent`.** After
   migration all its remaining callers pass `true`. Drop the param (hardcode
   true) for cleanliness, or leave it? (`sendMessageToSession` /
   `sendChatMessageToExternalAgent` must keep the param for the user-send
   endpoint.)
