# Review comment timer false negatives, and the answers they discarded

Design-review comments were being stamped `[Agent did not respond - try sending your
comment again]` while the agent's real answer sat on the linked interaction, unread. On meta
at the time of writing: 112 comments carried the stamp, **94 of them had a real response on
the linked interaction** (142 bytes to 677 KB), across 52 reviews and 6 users, from
2026-03-09 to 2026-08-11. Five months.

This records the timer semantics chosen, the single lookup path settled on, and the backfill
strategy.

## What was actually broken

Three defects, two of which were only visible once the cold-start case was reproduced live.

### 1. The timer could not tell "still booting" from "refused to answer"

`commentResponseTimeout = 2 * time.Minute` started counting when the comment was queued. On a
reaped session **the comment itself is what boots the desktop container**, and boot + Zed
startup + thread load routinely takes far longer than two minutes.

`handleCommentTimeout`'s decision tree was careful — terminal interaction → finalize; has text
and still moving → re-arm; has text but stalled → finalize partial — but **every branch
required the interaction to already have content**. A cold-booting sandbox has none, so control
fell straight through to the "genuine no-response" stamp. Two minutes was not a backstop there;
it was a near-guaranteed false positive.

### 2. Stamping destroyed the key the repair path needed

The stamp branch did:

```go
currentComment.AgentResponse = CommentTimerNoResponseMessage
currentComment.RequestID = ""      // the only key finalize could look up by
```

`finalizeCommentResponse` began with `GetCommentByRequestID(ctx, requestID)`, so the late
completion missed, returned early, and the answer was dropped. The well-commented repair branch
inside that function (`hadStaleTimerError` → *"Overwriting stale 'agent did not respond'
timer-stamp"*) was unreachable via the normal completion route, and the const's doc comment
asserted the opposite guarantee.

### 3. The completion's id is not the id the comment stores

**Found by reproducing the bug, not by reading the code.** In the reproduction:

| id | value |
|---|---|
| `comments.request_id` at dispatch | `int_01m0t9r4m3hj7whwf18c8v363n` |
| `comments.interaction_id` | `int_01m0t9r4m3hj7whwf18c8v363n` |
| `interactions.external_agent_request_id` at completion | **`req_01m0t9fw599x9sgbhgc6x656pd`** |
| id carried on `message_completed` | **`req_01m0t9fw599x9sgbhgc6x656pd`** |

`backfillCommentLinkageForPrompt` stores the dispatch-time id, but by completion the
interaction's `ExternalAgentRequestID` has been **rebound** to the agent's own request id by
`MarkInteractionExternalAgentDispatched` (`websocket_external_agent_sync.go:1995-2005`) once Zed
streams a message with an id of its own.

So `GetCommentByRequestID` missed **even when `request_id` was intact**. Fixing only the timer's
`RequestID = ""` would have left the bug alive on any turn where the agent rebinds. This is why
the reproduction was worth doing.

### 4. The loss was logged as routine

```
DBG No comment found for request_id (this is normal for non-comment interactions)
    error="no comment found for request …: record not found"
```

A lost answer was indistinguishable from noise, at DEBUG, with a message asserting it was fine.

## Chosen timer semantics

**The timer is a poll, not a deadline.** `commentResponseTimeout` is renamed
`commentTimerInterval` to say so. Each tick evaluates observable state and either acts on
evidence or re-arms. Elapsed wall-clock alone is never read as "the agent refused to answer".

Order of branches in `handleCommentTimeout` (the first four are unchanged):

1. Resolved (`RequestID` cleared or a real response landed) → return.
2. Interaction terminal → `finalizeCommentResponse`.
3. Interaction has content and `Updated` is moving → re-arm.
4. Interaction has content but has not moved for a full interval → finalize the partial.
5. **Agent not reachable** — no external-agent WebSocket for the session, or no `InteractionID`
   yet (the prompt has not dispatched) → re-arm, logged as its own state.
6. **Connected but no content yet** — the agent is thinking → re-arm.

Branches 5 and 6 are the fix. A cold-booting sandbox is now a *named state*, not an unlabelled
fall-through.

### Why not just raise the two minutes

That trades one wrong fixed guess for another. A 30-minute agent turn on a warm sandbox would
still be stamped, and a sandbox that takes 12 minutes to boot would still be stamped if the new
guess were 10. The correct predicate is **observable agent liveness**, not elapsed time.

### The bounds that remain, and why any remain at all

A tick that only ever re-armed would leave `request_id` set forever, and `request_id != ''` is
what `IsCommentBeingProcessedForSession` reads as "in flight" — so a permanently dead sandbox
would silently block every later comment on that review. That is strictly worse than a
wrong-but-honest message. Hence two ceilings, both far above any legitimate duration, both
measuring time *in that state*, both producing an accurate message:

| State | Ceiling | Stamped | Why this number |
|---|---|---|---|
| Agent unreachable | `commentSandboxStartCeiling` = 30 min | `CommentSandboxNotStartedMessage` | Observed cold boots run ~7 min. 30 min of continuous disconnection means the sandbox failed, not that it is slow — and the message says so instead of blaming the agent. |
| Connected, zero content, non-terminal, `interactions.updated` not moving | `commentSilentAgentCeiling` = 60 min | `CommentTimerNoResponseMessage` | Above the longest plausible legitimate turn. A long answer bumps `updated` and is caught by branch 3 first. This is the only path on which that message is true. |

Both elapsed values are derived from DB state (`QueuedAt`, `interactions.updated`, connection
presence). **No new in-memory state and no new column** — the timer is one `time.AfterFunc` per
*session* and gets replaced on every re-arm, so anything held in the closure would be lost on
re-arm and on restart.

## The single lookup path

One resolver, `resolveCommentForAgentRequest`, is the only way a completion finds its comment:

1. **Normalise the id.** `GetInteractionByExternalAgentRequestID` maps the incoming id back to
   its interaction id — the durable key the comment stores and never loses. This is what covers
   defect 3.
2. **One query over both durable columns.** `GetCommentByAgentRequestIDs` does
   `WHERE request_id IN ? OR interaction_id IN ?`. `interaction_id` is unique per comment, so
   the OR cannot resolve ambiguously.

This is one resolver, not a fallback chain: a single function, a single query, a single result.
It cannot disagree with itself.

**`RequestID` is still cleared when stamping**, deliberately. It is the queue's in-flight
marker; keeping it set would reintroduce exactly the "one stuck comment silently blocks every
later comment" hazard that `handleCommentTimeout`'s own comments warn about. Keeping the
resolver keyed on `interaction_id` is what makes clearing it safe.

Both placeholder strings are treated as "no real response yet" (`isTimerPlaceholderResponse`),
so a late completion overwrites either. The doc comment on `CommentTimerNoResponseMessage` now
describes what the code does rather than a guarantee it did not provide.

## Logging

`ErrNoCommentForAgentRequest` is returned when a completion is genuinely not linked to a
comment. Callers log **only that** at DEBUG; every other failure is an ERROR saying the answer
would be lost. A WARN fires whenever a comment resolves via `interaction_id` while the
completion's `request_id` did not match — a standing canary for the rebind path.

A turn that reaches a terminal error state with no output now surfaces
`interactions.error` on the comment instead of leaving a blank response box. The reproduction
produced exactly this ("Agent never connected after auto-wake cold-start retries") and showed
the user nothing.

## Backfill: reconciliation pass, not a migration

`reconcileTimerStampedComments` runs as step 0 of `ResumeCommentQueueProcessing`, in a goroutine,
batched at 1000 rows, logging per-row and total counts.

Why not a migration:

- The correct response text is `types.TextFromInteraction` — entries take precedence over the
  legacy flat message. SQL would have to reimplement that precedence and would then drift from
  the Go version every live path uses. (This is not hypothetical: the recovered row's
  `response_message` was 6,791 chars while `TextFromInteraction` yields 2,870 — the latter is
  what the live path writes.)
- The brief's own preference: extend the existing machinery. `ResumeCommentQueueProcessing`
  already *is* startup repair of comments whose completion never mapped back.
- It runs on every boot, so it repairs the historical backlog *and* anything stranded later by a
  residual race. A migration runs once and then protects nothing.
- Idempotent by construction: the predicate is "agent_response is a placeholder", which stops
  matching once repaired.

Rows with no `interaction_id` — the genuine no-responses — are excluded by the `JOIN`, not by a
separate guard. A row whose interaction yields no text is left stamped rather than blanked.

## Verification

Reproduced and verified live in the inner Helix, cold-start, not a unit test.

**Before the fix** (task `spt_01m0t9fw42eck2ycqvbhjxjk4c`, desktop stopped so the comment itself
triggers the boot):

```
16:30:38  comment posted
16:32:38  timer fires -> request_id BLANKED, 56-char stamp written
16:34:21  agent completes, 6,791 chars on the interaction
          DBG "No comment found for request_id (this is normal…)"
              error="…request req_01m0t9fw599x9sgbhgc6x656pd: record not found"
16:41:07  comment still 56 chars. Answer discarded.
```

**After the fix:**

- **Backfill, on real stranded data.** First boot after the change:
  `🔁 Recovered agent response stranded behind a stale timer stamp … response_length=2870`,
  `✅ Recovered … repaired=1`, with `agent_response_at=16:34:21` taken from the interaction
  rather than "now". That is the very comment the reproduction stranded.
- **Cold start past the two-minute mark.** A second cold start whose boot exceeded two minutes
  produced **no stamp at all**; the timer re-armed through the boot and acted only at ~6 minutes
  when the interaction reached a terminal error state — which then surfaced its reason.
- **Cold start, happy path.** A third cold start (graceful stop, container gone, comment boots
  it) delivered the real 1,655-char answer with no stamp.
- **Queue not blocked.** A further comment on the same review after the repaired and finalized
  ones was delivered and answered in ~30 seconds.
- 20 unit tests in `CommentTimerSuite` pass, covering both new re-arm branches, both ceilings,
  repair after a stamp cleared `request_id`, the rebound-request-id resolution, the sentinel,
  the error surfacing, and the backfill including idempotency.

**Not exercised live:** the two ceilings (30 min / 60 min) and the sandbox-failure message —
unit tests only; staging a genuinely dead sandbox for 30 minutes was not attempted. The
error-surfacing path was added after the cold start that motivated it, so it is covered by a
unit test rather than a live run.
