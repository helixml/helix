# Requirements: Fix Design-Review Comments Falsely Stamped "Agent Did Not Respond"

## Background

On meta (prod, 2026-08-11) a design-review comment permanently displays

```
[Agent did not respond - try sending your comment again]
```

even though the agent answered at length (89 KB). The string is **persisted** in
`spec_task_design_review_comments.agent_response` — this is not a render artifact.

DB evidence:

```
comment stdrc_9bdb66aa4af6a30e1970468117fb5134
  created_at 12:24:58   updated_at 12:26:58   (exactly commentResponseTimeout = 2m later)
  agent_response = '[Agent did not respond - try sending your comment again]'
  request_id = ''  queued_at = NULL  interaction_id = int_01kzrcgzmewg38s7p0ahw7bnm9

interaction int_01kzrcgzmewg38s7p0ahw7bnm9
  created 12:24:59  updated 12:43:04  state = complete  len(response_message) = 89260
```

Session `ses_01kzrbhv3trp330bbb49xmg2yj` had 4 other prompts in flight (12:22:17, 12:22:52,
12:23:27, 12:24:02) that only reached terminal state at 12:41–12:43. The comment's interaction
sat queued behind them, so at the 2-minute mark it had no persisted text yet and was still
`waiting`.

Scale on meta: **112 comments hold the literal timer string; 91 of those have a `complete`
linked interaction with a non-empty response.** 91 real agent answers the user was told never
happened. This is a primary correctness bug in the review loop, not an edge case.

### What the code actually looks like today (verified at `6bbab7b52`)

Some of the fix has already landed and must not be re-implemented:

- `handleCommentTimeout` (`api/pkg/server/spec_task_design_review_handlers.go:939`) **already**
  handles: terminal interaction → finalize; non-terminal **with text** → re-arm timer, or
  finalize partial if `interaction.Updated` is a full window stale.
- `finalizeCommentResponse` (line ~1413) **already** contains the `needsPopulation` /
  `hadStaleTimerError` repair block that overwrites the stale stamp (line ~1429–1445).
- `reconcileStuckInFlightComment` (line ~886) already unblocks terminal-but-unfinalized comments.
- `GetCommentByInteractionID` already exists at `api/pkg/store/spec_task_design_review_store.go:280`.

The remaining defects are exactly the two the incident exposes:

**Bug 1 — a non-terminal interaction with *zero* text falls through to the error stamp**
(line ~1030). That is the normal state of a prompt queued behind other prompts on a busy agent.
"The agent hasn't started yet" is reported as "the agent did not respond".

**Bug 2 — the stamp is unrepairable.** Line ~1033 clears `currentComment.RequestID` when
stamping. Every repair path is keyed on `request_id`:
`updateCommentWithStreamingResponse` → `GetCommentByRequestID`; `finalizeCommentResponse` →
`GetCommentByRequestID`; `websocket_external_agent_sync.go:3262` (message_completed w/
request_id); the `:3290` fallback → `GetPendingCommentByPlanningSessionID`, whose SQL requires
`request_id != ''`. So the existing repair block is **dead code today** — it can only run for a
comment that was never stamped. Reloading the page never fixes it; the comment is orphaned.

**Bug 1b (found during investigation, not in the original report)** — with the prompt-queue path,
`comment.InteractionID` is only backfilled at *dispatch*
(`backfillCommentLinkageForPrompt`, line ~1714). A comment still waiting in the prompt queue has
`InteractionID == ""`, so a naive "no interaction after the window → stamp" rule would fire on
precisely the queued-behind case this task is meant to fix. The prompt's status
(`GetPromptHistoryEntry(comment.PromptID)`) must be consulted before stamping.

## User Stories

### US1 — A reviewer whose comment is queued behind other work

**As a** user commenting on a design review while the agent is busy with earlier prompts,
**I want** my comment to stay in a visible "waiting for agent" state until the agent actually
answers, **so that** I am not told the agent failed when it simply has not started yet.

Acceptance criteria:
- [ ] When the backstop timer fires and the comment's linked interaction exists and is
      **non-terminal** (`waiting`/streaming), no error string is written, regardless of whether
      the interaction has any text yet.
- [ ] When the timer fires and the comment has **no** linked interaction but its prompt is still
      `pending`/`sending` in the prompt queue, no error string is written.
- [ ] In both cases the timer is re-armed so the comment is re-checked.
- [ ] When the agent finally completes, the comment shows the real answer (text **and**
      `agent_response_entries`, so tool calls render).
- [ ] Frontend "in flight" state (`request_id && !agent_response`, `DesignReviewContent.tsx:297`)
      continues to show the pending indicator for the whole wait — it must not flip to
      "answered with an error".

### US2 — The stamp is repairable when it does happen

**As a** user who did hit a genuine timeout stamp, **I want** a late agent completion to replace
the stamp with the real answer, **so that** a real response is never lost to a premature timer.

Acceptance criteria:
- [ ] When `message_completed` arrives and `GetCommentByRequestID` misses, the completing
      interaction's id is used to find the comment (`GetCommentByInteractionID`).
- [ ] The existing `needsPopulation` / `hadStaleTimerError` overwrite logic then applies:
      stale stamp → real `agent_response` + `agent_response_entries` + `agent_response_at`.
- [ ] The match is on the **exact** interaction id. A comment the user re-sent (new interaction)
      is never clobbered by a late completion for the old one.
- [ ] A comment that already holds a real (non-stamp, non-empty) response is never overwritten.
- [ ] Both `message_completed` branches are wired: the `request_id` branch
      (`websocket_external_agent_sync.go` ~3260) **and** the no-`request_id` session fallback
      (~3285), whose `GetPendingCommentByPlanningSessionID` query cannot see a stamped comment.

### US3 — A genuinely dead agent still terminates the queue

**As an** operator, **I want** an unbounded wait to be impossible, **so that** one dead
interaction cannot block a session's comment queue forever.

Acceptance criteria:
- [ ] Re-arming is bounded. After a hard cap of re-arms with no progress, the comment is stamped,
      logged loudly (error level, with session/comment/interaction/prompt ids and elapsed time),
      and the queue advances.
- [ ] `processNextCommentInQueue` still runs after a stamp — the timer's queue-unblocking role is
      preserved.
- [ ] `GetPendingCommentByPlanningSessionID`, `GetNextQueuedCommentForSession` and
      `IsCommentBeingProcessedForSession` semantics stay mutually consistent; a stamped comment is
      never re-sent to the agent.
- [ ] The existing behaviours are unchanged: terminal → finalize; non-terminal with text and a
      stale `Updated` → finalize partial; already-resolved → no-op.

### US4 — Existing prod damage is documented and repairable

**As** Luke, **I want** a ready-to-run repair statement and an accurate count of affected rows,
**so that** I can decide whether to heal meta's 91 orphaned answers.

Acceptance criteria:
- [ ] The PR description contains the diagnostic `SELECT` and the one-off repair `UPDATE`.
- [ ] The repair is **not** run against meta by this task.
- [ ] The PR states the current meta numbers (112 stamped / 91 healable).

## Testing Requirements

### Unit — `api/pkg/server/spec_task_design_review_handlers_test.go` (extend `CommentTimerSuite`)

- [ ] Timer fires, interaction non-terminal with **empty** text → **no stamp**, timer re-armed.
- [ ] Timer fires, interaction terminal with empty text → stamp, queue advances.
- [ ] Timer fires, comment has no interaction but prompt is still pending → no stamp, re-armed.
- [ ] Re-arm cap exhausted with no progress → stamp, queue advances, loud log.
- [ ] Late `message_completed` for an interaction whose comment was already stamped and has
      `request_id == ''` → stale stamp overwritten with real response **and** entries.
- [ ] A re-sent comment (different interaction) is **not** clobbered by a late completion for the
      old interaction.
- [ ] Existing suite tests still pass unchanged.

Run: `sudo apt-get install -y gcc libc6-dev` then
`cd api && CGO_ENABLED=1 go test -v -run TestCommentTimerSuite ./pkg/server/ -count=1`.

### E2E in the inner Helix (mandatory — this is a lifecycle/seam bug)

- [ ] Create a spec task, reach the design-review page.
- [ ] Post a review comment **while the agent is already busy with another prompt**, so the
      comment's interaction is queued > 2 minutes with no output.
- [ ] Verify the comment shows the agent's real answer and **never** the error string. Check both
      the UI and `SELECT agent_response FROM spec_task_design_review_comments`.
- [ ] Test the next operation: post a second comment afterwards and confirm the queue still drains.
- [ ] Screenshots saved under `screenshots/` in this task directory.

**Reporting this fixed on unit tests alone is not acceptable.**

## Non-Goals

- Bumping `commentResponseTimeout` as the fix. The constant stays a backstop; the state machine is
  what gets fixed.
- Any frontend change beyond confirming the existing pending indicator behaves correctly.
- Running the data repair against meta.

## Open Questions

1. **Hard cap value.** Proposed: 30 re-arms × 2 min = 60 minutes of wall clock before a
   no-progress comment is stamped. Long agent turns on meta ran ~19 minutes; 60 min gives ~3×
   headroom. Is an hour acceptable, or should it be shorter (e.g. 30 min) so users get a
   retry signal sooner?
2. **`request_id` on stamp.** The plan keeps clearing `request_id` (queue-unblocking and store
   query semantics stay untouched, no migration) and makes repair reachable via `interaction_id`
   instead. The alternative in the brief — a `timed_out` column plus updated queries — is
   rejected as more churn for no extra capability. Confirm that trade-off.
3. **Restart behaviour of the re-arm counter.** The counter lives in the timer closure
   (in-memory), so an API restart resets it. `ResumeCommentQueueProcessing` +
   `reconcileStuckInFlightComment` already reconcile on boot, so the practical effect is a fresh
   waiting window rather than an immediate stamp. Acceptable, or should the elapsed time be
   derived from `comment.QueuedAt` so it survives restarts?
4. **Stamp wording.** Should the bounded-cap stamp keep the exact existing string (required for
   the `hadStaleTimerError` repair check and for the meta backfill query to match), or gain a
   distinct message such as "agent did not respond within 60 minutes"? Plan assumes the string
   stays identical.
5. **Streaming repair path.** `updateCommentWithStreamingResponse` is also `request_id`-keyed, so
   a stamped comment shows nothing until `message_completed`. Plan leaves it alone (the final
   completion heals it). Should mid-stream updates also heal a stamped comment?
