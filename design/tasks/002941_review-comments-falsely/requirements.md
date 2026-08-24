# Requirements: Fix False "Agent Did Not Respond" Stamps on Review Comments

## Background

A design-review comment sent to a spec-task agent is guarded by a 2-minute backstop timer
(`commentResponseTimeout`, `api/pkg/server/spec_task_design_review_handlers.go:32`). When the
timer fires and the linked interaction has no content yet, `handleCommentTimeout` stamps
`CommentTimerNoResponseMessage` (`"[Agent did not respond - try sending your comment again]"`)
onto the comment **and clears `RequestID`**.

`RequestID` is the only key `finalizeCommentResponse` can look a comment up by
(`GetCommentByRequestID`). So when the agent's real answer arrives minutes later, the lookup
misses, the function returns early, and the answer is discarded. The miss is logged at DEBUG
with the text *"this is normal for non-comment interactions"*
(`api/pkg/server/websocket_external_agent_sync.go:3435`).

The dominant trigger is a **cold sandbox**: on a reaped session the comment itself is what
boots the desktop container. Boot + Zed startup + thread load can take ~7 minutes; the 2-minute
timer fires ~2 minutes into boot, when the interaction is legitimately empty.

Measured on meta: 112 comments carry the stamp; **94 of them have a real agent response on the
linked interaction that the user never saw** (142 bytes to 677 KB), across 52 reviews, 6 users,
2026-03-09 to 2026-08-11. The remaining 18 have no `interaction_id` and are genuine
no-responses.

Confirmed instance: task `spt_01m0s0vktb0twdtwz7cmk6wgtg`, review
`9dd3b44d-f27d-4c4d-aa12-70ee39e0aea1`, comment `stdrc_c80fa745619d06c68d9516796e30b906`,
interaction `int_01m0srqmdkzhr1g6h234dtvfqm`, session `ses_01m0s0vkvq89563fdkxza4cry6`. The
agent completed at 11:40:08 with 38,340 chars; the comment already carried the stamp from
11:35:16. That single row was hand-repaired on meta; the other 94 are still broken.

## User Stories

### US-1 — A cold sandbox must not produce a false "did not respond"
**As a** reviewer commenting on a spec task whose desktop container has been reaped
**I want** my comment to wait for the sandbox to boot
**So that** I get the agent's answer instead of a false failure message.

Acceptance criteria:
- [ ] Posting a comment to a spec task with **no running desktop container** does not stamp
      `CommentTimerNoResponseMessage` at the 2-minute mark.
- [ ] "The agent is not reachable yet" is a **named, distinguishable branch** in
      `handleCommentTimeout`, not an unlabelled fall-through into the no-response stamp. It logs
      a distinct message naming the state (sandbox starting / agent not connected).
- [ ] Time during which the agent is unreachable does not count toward the no-response budget.
- [ ] If any message is ever written to the comment while in that state, it says the sandbox is
      still starting — never that the agent refused to answer.
- [ ] The fix is not "raise 2 minutes to a bigger number". A 30-minute agent turn on a warm
      sandbox must also not be stamped. Whatever bound remains is justified in the design doc.

### US-2 — A late completion must always find its comment
**As a** reviewer whose comment was stamped
**I want** the agent's late answer to replace the stamp
**So that** no answer is silently discarded.

Acceptance criteria:
- [ ] After a timer stamp, a later `message_completed` for the same interaction repairs the
      comment: `agent_response`, `agent_response_entries`, `agent_response_at` are populated
      from the interaction and the stamp is gone.
- [ ] There is **exactly one** lookup path a late completion goes through to find its comment.
      No second parallel path that can disagree with the first.
- [ ] The existing repair branch in `finalizeCommentResponse` (`needsPopulation` /
      `hadStaleTimerError`, the `🔁 Overwriting stale…` log) is reachable via the normal
      `message_completed` route.
- [ ] The doc comment on `CommentTimerNoResponseMessage` describes what the code actually does.
- [ ] The comment queue is **not** left blocked as a side effect. After a repair or late
      finalize, the next queued comment for that session is delivered and answered.

### US-3 — A lost answer must be loud
**As an** operator
**I want** a completion that cannot be attached to its linked comment to be an error, not
routine debug noise
**So that** the next occurrence is discovered in hours, not five months.

Acceptance criteria:
- [ ] A lookup miss for an interaction that is **not** linked to any comment stays at DEBUG
      (this is the common, genuinely-normal case).
- [ ] A completion whose interaction **is** linked to a comment but which fails to attach logs
      at ERROR with a message that says an agent answer would be lost.
- [ ] The two cases are distinguishable by log level and message, not only by reading the
      error string.

### US-4 — The 94 stranded answers are recovered
**As a** reviewer whose answer was discarded months ago
**I want** the answer surfaced on my comment
**So that** I can read it without a DBA hand-repairing rows.

Acceptance criteria:
- [ ] For every comment where `agent_response = CommentTimerNoResponseMessage` AND the linked
      interaction is `complete` or `error` AND `length(response_message) > 0`:
      `response_message → agent_response`, `response_entries → agent_response_entries`,
      `agent_response_at = interactions.updated`.
- [ ] Rows with no `interaction_id` (the 18 genuine no-responses) are left untouched.
- [ ] The repair is idempotent — running it twice changes nothing the second time.
- [ ] It logs how many rows it repaired.
- [ ] The chosen mechanism (migration vs. reconciliation pass) is justified in the design doc,
      and extends the existing `ResumeCommentQueueProcessing` /
      `reconcileStuckInFlightComment` machinery rather than adding a parallel one.

### US-5 — Verified live, not just in unit tests
**As a** maintainer
**I want** the cold-start case reproduced in the inner Helix before and after the fix
**So that** the fix is known to work in the seam between the timer and container boot.

Acceptance criteria:
- [ ] **Before the fix**: in the inner Helix at `http://localhost:8080`, a spec task in
      `spec_review` with its desktop container **gone**, plus a posted review comment, produces
      the stamp at the 2-minute mark. Reproduction captured (screenshot and/or DB row).
- [ ] **After the fix**: the same sequence ends with the real agent answer on the comment and
      no false stamp.
- [ ] A **second** comment posted on the same review afterwards is delivered and answered —
      proving the queue is not blocked.
- [ ] Go unit coverage added for: (a) late `message_completed` after a timer stamp repairs the
      comment; (b) a cold/unreachable agent is not stamped as "did not respond".
- [ ] `go build ./pkg/...` and `cd frontend && yarn build` pass before committing.
- [ ] CI checked after push with `gh pr checks`; failures fixed unprompted.

## Out of Scope

- Any change to `spt_01m0sh2mqx8491eg62fkp9qrap` (sandbox defaults + stream init replay).
- Speeding up desktop container cold boot.
- Frontend rendering changes to the comment thread (the fix is server-side; the stamp is plain
  text in `agent_response` and there is no frontend special-casing of the string —
  `grep "Agent did not respond" frontend/src` returns nothing).

## Deliverable

One PR, plus `design/2026-08-24-review-comment-timer-false-negative.md` in the helix repo
recording the chosen timer semantics, the single lookup path, and the backfill strategy.
Use full URLs for PRs/issues (`https://github.com/helixml/helix/pull/NNN`).

## Open Questions

1. **Absolute ceiling for an unreachable sandbox.** The design proposes never stamping
   "did not respond" while the agent is unreachable, but keeping a long safety ceiling
   (proposed: 30 minutes of continuous disconnection) after which a *different*, accurate
   message is stamped so the queue cannot block forever. Is 30 minutes acceptable, or should an
   unreachable sandbox re-arm indefinitely and rely solely on the startup reconciliation pass?
2. **Ceiling for a connected-but-silent agent.** Proposed: never stamp on absence of evidence;
   stamp only when the interaction reaches a terminal state with empty text, or after a long
   no-progress ceiling (proposed: 60 minutes with the agent connected, interaction non-terminal,
   zero content, `interactions.updated` not moving). Is 60 minutes the right order of magnitude
   for the longest legitimate agent turn?
3. **Backfill `agent_response_at`.** The brief says set it from `interactions.updated`. That is
   the time of the *last* interaction write, which for repaired rows is the completion time —
   assumed correct. Confirm this rather than, say, leaving it null to mark the row as repaired.
4. **Should repaired rows be marked?** No column currently distinguishes "answer recovered by
   backfill" from "answered normally". The plan is not to add one (no schema change), relying on
   log output for the audit trail. Confirm that is enough.
5. **Backfill scope in the inner Helix.** The inner Helix DB will have few or no affected rows,
   so the backfill can only be verified against synthetic rows inserted for the test. The 94
   real rows live on meta and will be repaired when this ships there. Confirm that meta is the
   intended deployment target for the recovery.
6. **Existing unit test `TestHandleCommentTimeout_StampsErrorWhenInteractionEmpty`** asserts the
   stamp is written for an empty, non-terminal interaction on the first tick. Under the new
   semantics that is exactly the false positive being fixed, so this test must be rewritten to
   assert a re-arm instead. Flagging because it changes an existing pinned behaviour.
