# Stop discarding agent answers behind false "[Agent did not respond]" stamps

## Summary

Design-review comments were stamped `[Agent did not respond - try sending your comment again]`
while the agent's real answer sat unread on the linked interaction. On meta: **112 comments
carried the stamp and 94 of them had a real response** (142 bytes to 677 KB), across 52 reviews
and 6 users, from 2026-03-09 to 2026-08-11. Five months of quietly eaten answers.

The trigger is a cold sandbox. On a reaped session **the comment itself is what boots the
desktop container**, and boot + Zed startup + thread load takes far longer than the 2-minute
backstop timer. Every branch of `handleCommentTimeout`'s decision tree required the interaction
to already have content, so a cold-booting sandbox fell straight through to the "genuine
no-response" stamp — and stamping cleared `request_id`, the only key
`finalizeCommentResponse` could look a comment up by. The late completion missed, returned
early, and the answer was dropped with a DEBUG line asserting this was normal.

Full write-up: `design/2026-08-24-review-comment-timer-false-negative.md`.

## A third defect, found by reproducing rather than reading

The brief described two defects. Reproducing the cold start live surfaced a third:

| id | value |
|---|---|
| `comments.request_id` at dispatch | `int_01m0t9r4m3hj7whwf18c8v363n` |
| `interactions.external_agent_request_id` at completion | **`req_01m0t9fw599x9sgbhgc6x656pd`** |
| id carried on `message_completed` | **`req_01m0t9fw599x9sgbhgc6x656pd`** |

The agent rebinds the interaction's `ExternalAgentRequestID` mid-turn
(`MarkInteractionExternalAgentDispatched`), so the completion arrives under an id the comment
never stored. `GetCommentByRequestID` missed **even with `request_id` intact**. Fixing only the
timer's `RequestID = ""` would have left the bug alive on any turn where the agent rebinds.

## Changes

**Timer is a poll, not a deadline** (`commentResponseTimeout` → `commentTimerInterval`). Two
named branches ahead of the old fall-through: *agent not reachable* (no external-agent
WebSocket, or the prompt has not dispatched) and *connected but no content yet*. Both re-arm.
Time while the agent is unreachable never counts against the agent.

**Bounds kept, and argued.** Not "raise 2 minutes to a bigger number" — that trades one wrong
guess for another and still stamps a 30-minute turn. Ceilings exist only because `request_id`
stays set while in flight and an in-flight comment blocks every later comment for the session:
30 min of continuous disconnection stamps a message naming the *sandbox*; 60 min connected with
zero content and no movement stamps the no-response message, which is the one place it is true.
Both derived from DB state — no new in-memory state, no new column.

**One lookup path.** `resolveCommentForAgentRequest` normalises the incoming id to its
interaction, then runs a single query over both durable columns
(`request_id IN ? OR interaction_id IN ?`). One function, one query, one result — it cannot
disagree with itself. `request_id` is still cleared when stamping, deliberately, because it is
the queue's in-flight marker; keying the resolver on `interaction_id` is what makes that safe.
The lying doc comment on `CommentTimerNoResponseMessage` is rewritten to describe what the code
actually does.

**Loud failures.** `ErrNoCommentForAgentRequest` distinguishes a genuinely non-comment
interaction (stays DEBUG) from a comment that was found and failed to attach (now ERROR, "this
answer would be lost"). A WARN fires whenever a comment resolves via `interaction_id` — a
standing canary for the rebind path.

**Errored turns explain themselves.** A turn that fails with no output now surfaces
`interactions.error` instead of leaving a blank response box.

**Backfill** via `reconcileTimerStampedComments` at startup, not a migration: the response text
must come from `types.TextFromInteraction` (entries take precedence over the flat message), and
SQL reproducing that would drift from the live path — measured on the recovered row,
`response_message` was 6,791 chars while `TextFromInteraction` yields 2,870. Idempotent, batched,
logged; rows with no `interaction_id` excluded by the `JOIN`.

## Verification

Reproduced and verified live in the inner Helix, cold-start — not a unit test.

**Before:** comment posted 16:30:38 → stamp at 16:32:38 (`request_id` blanked) → agent completed
16:34:21 with 6,791 chars → `DBG No comment found for request_id … record not found` → comment
still 56 chars at 16:41.

**After:**
- **Backfill on real stranded data** — first boot recovered that exact comment:
  `✅ Recovered … repaired=1`, `agent_response_at=16:34:21` from the interaction, not "now".
- **Cold start past 2 minutes** — no stamp; the timer re-armed through the boot and acted only
  at ~6 min when the interaction reached a terminal error, which then surfaced its reason.
- **Cold start happy path** — real 1,655-char answer, no stamp.
- **Queue not blocked** — a further comment on the same review was delivered and answered in
  ~30s.
- **20 unit tests** in `CommentTimerSuite`, including both re-arm branches, both ceilings, repair
  after a stamp cleared `request_id`, rebound-request-id resolution, the sentinel, error
  surfacing, and backfill idempotency.

**Not exercised live:** the 30/60-minute ceilings and the sandbox-failure message (unit tests
only — staging a dead sandbox for 30 minutes was not attempted), and the error-surfacing path,
which was added after the cold start that motivated it.

`go build ./pkg/...` passes. The frontend bundle compiles (22,300 modules); the in-place
`yarn build` cannot write to `frontend/dist` in this sandbox because it is a root-owned
read-only bind mount, so it was verified with `vite build --outDir /tmp/fe-dist`. No frontend
files are touched by this change.

## Screenshots

Same comment, before and after — note that in the "before" the agent had *already* rewritten
acceptance criterion #5 to say "guideline, not a hard cap" exactly as asked, while the comment
claimed it never responded.

![Before: false stamp](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002941_review-comments-falsely/screenshots/01-before-false-stamp.png)
![After: recovered answer](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002941_review-comments-falsely/screenshots/02-after-real-answers.png)
