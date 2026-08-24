# Design: Fix False "Agent Did Not Respond" Stamps on Review Comments

## Map of the existing machinery (read this before changing anything)

All paths relative to `/home/retro/work/helix/api/`.

| Piece | Location | Role |
|---|---|---|
| `CommentTimerNoResponseMessage` | `pkg/server/spec_task_design_review_handlers.go:26` | The stamp string |
| `commentResponseTimeout = 2m` | `…:32` | Backstop timer duration |
| `ResumeCommentQueueProcessing` | `…:41` | Startup: reconcile → reset stuck → resume queues |
| `queueCommentForAgent` | `…:719` | Sets `queued_at`, kicks the queue |
| `processNextCommentInQueue` | `…:757` | In-flight guard → send → `armCommentTimer` |
| `armCommentTimer` | `…:861` | One `time.AfterFunc` per **session** |
| `reconcileStuckInFlightComment` | `…:885` | In-flight comment whose interaction is terminal → finalize |
| `handleCommentTimeout` | `…:939` | The decision tree (see below) |
| `sendCommentToAgentNow` | `…:1233` | Enqueues prompt, sets `RequestID = promptID` placeholder |
| `finalizeCommentResponse` | `…:1404` | `message_completed` → copy response onto comment, unblock queue |
| `backfillCommentLinkageForPrompt` | `…:1714` | At dispatch: `RequestID`/`InteractionID` ← real interaction |
| Completion entry point | `pkg/server/websocket_external_agent_sync.go:3431` | Calls `finalizeCommentResponse` |
| Store queries | `pkg/store/spec_task_design_review_store.go:280-425` | `GetCommentByRequestID`, `GetCommentByInteractionID`, `IsCommentBeingProcessedForSession`, `GetNextQueuedCommentForSession`, `ResetStuckComments` |
| Unit tests | `pkg/server/spec_task_design_review_handlers_test.go` (`CommentTimerSuite`) | 9 tests already pin this behaviour |

Key facts established by reading the code:

- On the queue path `comment.RequestID == comment.InteractionID` — both are set from the same
  interaction at dispatch (`backfillCommentLinkageForPrompt`, and
  `requestID = createdInteraction.ExternalAgentRequestID` at
  `websocket_external_agent_sync.go:3844`). Before dispatch, `RequestID` holds the prompt id as
  a placeholder purely so the in-flight guard blocks a concurrent second comment.
- `IsCommentBeingProcessedForSession` = `request_id != ''`. **Clearing `RequestID` is what
  unblocks the session's comment queue.** Any fix that keeps `RequestID` set after stamping
  must also change this predicate, or every later comment for that session is silently never
  delivered — the exact hazard `handleCommentTimeout`'s own comments warn about.
- `updateCommentWithStreamingResponse` writes `agent_response` **during** streaming, so
  "agent_response is non-empty" is *not* a safe proxy for "not in flight".
- `handleCommentTimeout`'s decision tree is good code. Every one of its branches requires the
  interaction to already have content or be terminal. A cold-booting sandbox has neither, so
  control falls straight through to the stamp. That fall-through is the bug, not the tree.

## Decision 1 — Timer semantics: liveness-gated, evidence-based

The timer stops being a **deadline** and becomes a **poll tick** that makes a decision each
time it fires. `commentResponseTimeout` is renamed `commentTimerInterval` (still 2 minutes) to
say what it is.

New tick order in `handleCommentTimeout` — the first four branches are today's tree, unchanged:

1. **Resolved** — `RequestID == ""` or a real `AgentResponse` exists → return.
2. **Terminal interaction** — `complete` / `interrupted` / `error` → `finalizeCommentResponse`.
   Covers "agent finished but `message_completed` never mapped back".
3. **Has content, still moving** (`interactions.updated` within one interval) → re-arm.
4. **Has content, stalled a full interval** → finalize the partial response, unblock the queue.
5. **NEW — agent not reachable.** No external-agent WebSocket connection for the session
   (`s.externalAgentWSManager.getConnection(sessionID)` returns false), **or** the comment has
   no `InteractionID` yet (the prompt has not dispatched — `RequestID` is still the prompt-id
   placeholder). This is the cold-boot state. Re-arm. Log at INFO with a dedicated message
   naming the state. Track the first tick at which we entered this state so it can be bounded
   (see Decision 2). **Never** stamp `CommentTimerNoResponseMessage` here.
6. **Connected, no content, non-terminal** — the agent is reachable and thinking. Re-arm. This
   is the "30-minute agent turn" case; absence of tokens is not evidence of refusal.
7. **Ceiling exceeded** (Decision 2) → stamp and unblock.

Branch 5 is the whole point: a cold-booting sandbox is now a *named state*, not an unlabelled
fall-through.

### Why not just raise 2 minutes

Raising the constant trades one wrong fixed guess for another — a 30-minute agent turn on a
warm sandbox is still stamped, and a sandbox that takes 12 minutes to boot is still stamped if
the new guess is 10. The correct predicate is not elapsed wall-clock, it is **observable agent
liveness**. Branches 5 and 6 make the timer count only against evidence it actually has.

## Decision 2 — The bounds we keep, and why we keep any

A tick that only ever re-arms would leave a comment in-flight forever, and `RequestID != ''`
blocks the whole session's comment queue. So two ceilings survive, both far above any
legitimate duration, both producing an **accurate** message:

| State | Ceiling | Message stamped | Rationale |
|---|---|---|---|
| Agent unreachable (branch 5) | 30 min continuous disconnection | `CommentSandboxNotStartedMessage` — *"[Sandbox is still starting - your comment will be answered when it comes up]"* → on ceiling: *"[Sandbox failed to start - try sending your comment again]"* | Observed cold boots run ~7 min. 30 min of continuous disconnection means the sandbox failed, not that it is slow. The message names the real cause. |
| Agent connected, zero content, non-terminal, `interactions.updated` not moving (branch 6) | 60 min | `CommentTimerNoResponseMessage` (unchanged text — here it is *true*) | Longest plausible legitimate agent turn, with headroom. Only reached with the agent demonstrably connected and demonstrably producing nothing. |

Both ceilings measure time **in that state**, not since the comment was queued. Disconnected
time never counts toward the connected ceiling and vice versa. Because the timer is one
`AfterFunc` per session that re-arms, state is derived per tick from `comment.QueuedAt`,
`interactions.updated`, and connection presence — **no new in-memory state and no new column**;
a restart re-derives the same answer, and `ResumeCommentQueueProcessing` re-arms the tick.

The honest case for a ceiling at all: without one, a permanently dead sandbox blocks every
later comment on that review silently. A wrong-but-accurate message after an hour is strictly
better than a silently dead queue.

## Cold-start reproduction — result, and a defect the design missed

Reproduced live in the inner Helix on 2026-08-24 (pre-fix, unmodified code).

Setup: org `testorg`, project `testproj` (`prj_01m0t9b2wrfvqzp660w2k7tscv`), spec task
`spt_01m0t9fw42eck2ycqvbhjxjk4c` driven to `spec_review`, review
`e538f07a-41b6-4cf7-b8ad-69308d935f1e`, planning session `ses_01m0t9fw5611kr6gkd3c2ebsry`.
Desktop stopped via the UI's **Stop desktop** button (container
`ubuntu-external-01m0t9fw5611kr6gkd3c2ebsry` confirmed gone), then a review comment posted so
the comment itself triggered the cold boot.

```
16:30:38  comment stdrc_07b981e5a688d788881ae216ab3b757a posted; queued
16:31:05  request_id = int_01m0t9r4m3hj7whwf18c8v363n, interaction_id = same, agent_response = '' (0)
16:32:38  timer fires → request_id BLANKED, agent_response = the 56-char stamp
16:34:21  agent completes: interaction state=complete, response_message = 6,791 chars
          DBG websocket_external_agent_sync.go:3435
              "No comment found for request_id (this is normal for non-comment interactions)"
              error="no comment found for request req_01m0t9fw599x9sgbhgc6x656pd: record not found"
16:41:07  comment still 56 chars. The 6,791-char answer is discarded.
```

Screenshot: `screenshots/01-before-false-stamp.png` — the comment log shows *"[Agent did not
respond - try sending your comment again] / Worked for 0s"* while acceptance criterion #5 in
the same document has already been rewritten by the agent to say "guideline, not a hard cap",
which is precisely what the comment asked for. The answer is visibly there; the user is told
it isn't.

### The missed defect: the completion's id is not the id the comment stores

The design assumed `comment.RequestID == comment.InteractionID` and that the only reason the
lookup failed was the timer blanking `RequestID`. The repro disproves the second half:

| id | value |
|---|---|
| `comments.request_id` at dispatch | `int_01m0t9r4m3hj7whwf18c8v363n` (= the interaction id) |
| `comments.interaction_id` | `int_01m0t9r4m3hj7whwf18c8v363n` |
| `interactions.external_agent_request_id` at completion | **`req_01m0t9fw599x9sgbhgc6x656pd`** |
| id carried on `message_completed` | **`req_01m0t9fw599x9sgbhgc6x656pd`** |

`backfillCommentLinkageForPrompt` logged `request_id=int_01m0t9r4m3hj7whwf18c8v363n` at
dispatch, but by completion the interaction's `ExternalAgentRequestID` had been **rebound** to
Zed's own `req_…` id by `MarkInteractionExternalAgentDispatched`
(`websocket_external_agent_sync.go:1995-2005`) when Zed streamed its first message with a
request id of its own.

So `GetCommentByRequestID(req_…)` **misses even when `request_id` is intact**. There are two
independent breaks, not one:

1. the timer blanks `request_id` (Defect 2 as briefed), and
2. the id the completion arrives with is not the id the comment ever stored.

Fix (2) alone would have left the bug alive on any turn where Zed rebinds the request id; fix
(1) alone would have left it alive on this very reproduction. Decision 3 below is amended to
cover both.

## Decision 3 — Exactly one lookup path for a late completion

Two candidates were considered.

**(a) Stop clearing `RequestID` when stamping.** Rejected. `IsCommentBeingProcessedForSession`
keys off `request_id != ''`, so the stamped comment would block the session's queue forever.
Fixing that means changing the in-flight predicate, and the obvious predicate
(`request_id != '' AND agent_response = ''`) is wrong because streaming writes `agent_response`
mid-flight. The alternative — baking the literal stamp string into store SQL — puts a magic
string in the persistence layer. Too much blast radius on the queue for the benefit.

**(b) Make the resolver stable, keep queue semantics untouched.** Chosen, and amended after the
reproduction to also normalise the incoming id.

One resolver, `resolveCommentForAgentRequest(ctx, requestID)`, is the **only** way the
completion path finds a comment. It does two things in a fixed order:

1. **Normalise the id.** The id on `message_completed` is the interaction's *current*
   `ExternalAgentRequestID`, which Zed rebinds mid-turn. `GetInteractionByExternalAgentRequestID`
   (already on the store interface, `store.go:330`) maps it back to the interaction id — the one
   durable key the comment stores and never loses.
2. **One query over the durable keys.**

```go
// GetCommentByAgentRequestIDs resolves the design-review comment for a completed
// agent turn from the candidate ids the completion could be keyed by.
// WHERE request_id IN (?) OR interaction_id IN (?)
func (s *PostgresStore) GetCommentByAgentRequestIDs(ctx context.Context, ids []string) (*types.SpecTaskDesignReviewComment, error)
```

`finalizeCommentResponse` calls only the resolver; `GetCommentByRequestID` is no longer used on
the completion path. This is one resolver, not a fallback chain: a single function, a single
query, a single result — it cannot disagree with itself. `interaction_id` is unique per comment
so the OR cannot be ambiguous. The existing `needsPopulation` / `hadStaleTimerError` repair
branch becomes reachable exactly as its doc comment already claims.

Guard against double-finalize: if the resolved comment already has a real `AgentResponse` and
an empty `RequestID`, it was finalized already — log at DEBUG and return without re-triggering
the queue.

`GetCommentByRequestID` stays for `updateCommentWithStreamingResponse` (streaming always runs
while `RequestID` is set) unless the migration proves trivial; do not leave two resolvers on
the *completion* path.

**Fix the lying doc comment** on `CommentTimerNoResponseMessage` (`…:21-25`). It currently
asserts a guarantee the code does not provide. Rewrite it to state that the stamp is keyed by
`interaction_id` via `GetCommentForAgentRequest`, and that `request_id` is deliberately cleared
to unblock the queue.

## Decision 4 — Make the loss loud (Defect 3)

`finalizeCommentResponse` returns a sentinel on a resolution miss:

```go
var ErrNoCommentForAgentRequest = errors.New("no design-review comment linked to agent request")
```

At `websocket_external_agent_sync.go:3435`:

- `errors.Is(err, ErrNoCommentForAgentRequest)` → DEBUG, current wording kept. This really is
  the common case (most interactions are not comments).
- Any other error → **ERROR**, with `request_id`, `interaction_id`, `comment_id`, and a message
  stating that an agent answer could not be attached to its review comment and would be lost.

Add a WARN inside `GetCommentForAgentRequest`'s caller when a comment is resolved via
`interaction_id` while `request_id` was empty — the canary that the primary key path was broken
for that turn. If that WARN ever fires after this ships, the timer stamped something it should
not have.

## Decision 5 — Backfill: reconciliation pass, not a migration

Chosen: a new `reconcileTimerStampedComments(ctx)` called from `ResumeCommentQueueProcessing`
as **step 0**, before the existing reconcile/reset steps.

Why not a SQL migration:

- The correct response text is `types.TextFromInteraction(i)` =
  `TextFromEntries(i.ResponseEntries, i.ResponseMessage)` (`pkg/types/types.go:3809`). A
  migration would have to reimplement that precedence in SQL and would drift from the Go
  version. Reusing the Go helper is the correctness argument.
- The brief explicitly prefers extending the existing machinery over a parallel mechanism, and
  `ResumeCommentQueueProcessing` is already exactly this: startup repair of comments whose
  completion never mapped back.
- It runs on every deployment, so it repairs both the historical 94 rows and any residual row
  stranded by a race the forward fix does not cover.
- It is trivially idempotent — the predicate is `agent_response = CommentTimerNoResponseMessage`,
  which stops matching after repair.

Implementation:

```go
// pkg/store — new
ListTimerStampedCommentsWithResponses(ctx context.Context, stamp string, limit int) ([]TimerStampedCommentRepair, error)
```

implementing the brief's SQL verbatim:

```sql
SELECT c.id, c.interaction_id, i.state, i.response_message, i.response_entries, i.updated
FROM spec_task_design_review_comments c
JOIN interactions i ON i.id = c.interaction_id
WHERE c.agent_response = @stamp
  AND i.state IN ('complete','error')
  AND length(i.response_message) > 0
ORDER BY c.created_at
LIMIT @limit
```

The `JOIN` alone excludes the 18 rows with no `interaction_id` — no extra guard needed, but
assert it in a test. For each row set `agent_response`, `agent_response_entries`, and
`agent_response_at = interactions.updated` (not `time.Now()` — the answer is historical).
Batch with a limit (1000) in a loop; log `Int("repaired", n)` per batch and a total at the end.
Run it in a goroutine so a large backfill never delays API startup, and never let it clear or
set `request_id` (those rows already have `request_id = ''`).

Not covered by this query and deliberately left alone: rows whose interaction is terminal but
`response_message` is empty while `response_entries` holds the text. If the meta count with the
Go-side `TextFromInteraction` differs from 94, report the discrepancy rather than widening the
predicate silently.

## Decision 6 — Queue-unblock invariant

Every branch that writes a terminal outcome to a comment must clear `RequestID` and
`QueuedAt` and call `processNextCommentInQueue`. The verification requirement "post a second
comment on the same review and confirm it is answered" exists to catch a regression here. The
new branches 5 and 6 do **not** write terminal outcomes — they re-arm, so the comment stays
legitimately in flight and the queue stays legitimately blocked for that session (correct: the
agent is working on that comment). Only the ceilings in Decision 2 terminate.

## Testing strategy

**Cold-start reproduction (mandatory, cannot be substituted by a unit test).** In the inner
Helix at `http://localhost:8080`: register `test@helix.ml` / `helixtest`, complete onboarding,
create a spec task, drive it to `spec_review`, then **stop/remove its desktop container**
(`docker compose exec -T sandbox-nvidia docker ps | grep ubuntu-external-<sid>`, then `docker
stop`). Post a review comment — the comment itself triggers the cold boot. Watch
`spec_task_design_review_comments.agent_response` and the API logs. Before the fix the stamp
appears at ~2 minutes; after the fix the real answer lands and no stamp appears. Then post a
**second** comment on the same review and confirm it is delivered and answered.

**Go unit tests** — extend `CommentTimerSuite`:

- `TestHandleCommentTimeout_DoesNotStampWhenAgentUnreachable` — no WS connection, interaction
  empty and non-terminal → re-arm, no `UpdateSpecTaskDesignReviewComment` with the stamp.
- `TestHandleCommentTimeout_DoesNotStampWhenCommentNotYetDispatched` — `InteractionID == ""`.
- `TestFinalizeCommentResponse_RepairsAfterTimerStampClearedRequestID` — comment with
  `RequestID = ""`, `InteractionID` set, `AgentResponse = CommentTimerNoResponseMessage`;
  `finalizeCommentResponse(interactionID)` resolves it and overwrites the stamp.
- `TestReconcileTimerStampedComments_*` — repairs matching rows, skips rows without an
  interaction, idempotent on a second run.
- **Rewrite** `TestHandleCommentTimeout_StampsErrorWhenInteractionEmpty` — under the new
  semantics an empty non-terminal interaction re-arms on the first tick. Retarget the test at
  the ceiling case so the stamp remains covered.

Local run needs CGo: `sudo apt-get install -y gcc libc6-dev` then
`CGO_ENABLED=1 go test -run TestCommentTimerSuite ./pkg/server/ -count=1`.

## Environment setup for the inner Helix (needed before any spec task can be created)

A fresh inner Helix has no users and no configuration. In order, via the browser at
`http://localhost:8080`:

1. Register `test@helix.ml` / `helixtest` (first account is admin in dev), create org
   `testorg`, choose "Continue with Helix credits", create project `testproj`.
2. **System settings** — project creation fails with *"default new project agent provider and
   model are not configured in Admin > System Settings"* until this is set. The route is
   `PUT /api/v1/system/settings` (note: **not** `/api/v1/admin/system/settings` — `adminRouter`
   is a `MatcherFunc` subrouter of `authRouter`, so it shares the same path prefix):
   `{default_new_project_agent_provider:'openai', default_new_project_agent_model:'zai-org/GLM-5.2', default_new_project_agent_reasoning_effort:'none'}`.
3. **Org harness allow-list** — next failure is *"provider \"openai\" is not enabled for
   coding-agent harness \"zed_agent\" in this organization"*. `AllowsProvider` requires the
   provider to be listed explicitly; an empty `provider_refs` allows nothing.
   `PUT /api/v1/organizations/testorg/code-agent-harnesses` with
   `{harnesses:[{runtime:'zed_agent', enabled:true, provider_refs:['openai']}]}` (the body is an
   object with a `harnesses` key, not a bare array).
4. In the composer, click the mode button (defaults to **Build** = straight to implementation)
   and pick **Plan**, otherwise the task never enters `spec_review`.

Model choice: both global providers proxy to the outer Helix. The `anthropic` one is unusable
here (`/v1/messages` returns *"code-agent task has no API provider selected"*), so no Claude
models are available. The `openai` one serves 394 models; `zai-org/GLM-5.2` was probed working
and is fast enough to plan a small task in ~2 minutes.

UI note: `fill` on the composer sets the DOM value but does not update React state, leaving
**Send** disabled. Click the textbox and use `type_text` instead.

## Gotchas discovered

- The stack in this sandbox can take 5–10 minutes to come up; `helix-postgres-1` not existing
  yet is not a broken stack. Poll before concluding anything.
- `frontend/src` has **no** reference to the stamp string — it renders as plain markdown in the
  comment thread. No frontend change is needed, but `yarn build` is still required by CLAUDE.md.
- `armCommentTimer` is keyed by **session**, not comment. A re-arm for a different comment in
  the same session replaces the timer. Any new state must be derived from the DB, not held in
  the timer closure, or it is lost on re-arm and on restart.
- Do not touch `spt_01m0sh2mqx8491eg62fkp9qrap` (sandbox defaults + stream init replay).
