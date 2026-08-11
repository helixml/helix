# Design: Fix Design-Review Comments Falsely Stamped "Agent Did Not Respond"

## Files in play

| File | Role |
|---|---|
| `api/pkg/server/spec_task_design_review_handlers.go` | Timer + finalize + queue. Both bugs live here. |
| `api/pkg/server/websocket_external_agent_sync.go` (~3240–3310) | `message_completed` → finalize wiring. |
| `api/pkg/store/spec_task_design_review_store.go` | Comment queries (`GetCommentByInteractionID:280`, etc.). No change needed. |
| `api/pkg/server/spec_task_design_review_handlers_test.go` | `CommentTimerSuite` — extend. |
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx:297` | Pending indicator keys off `request_id && !agent_response`. Read-only. |

## Current lifecycle (verified)

```
POST comment
  → queued_at set, prompt enqueued (comment.PromptID set)      [interaction_id STILL EMPTY]
  → prompt queue dispatches when the session is idle
      → backfillCommentLinkageForPrompt sets RequestID + InteractionID   (line ~1714)
  → processNextCommentInQueue → sendCommentToAgentNow → armCommentTimer  (line ~848)
  → agent streams  → message_added  → updateCommentWithStreamingResponse (request_id-keyed)
  → agent finishes → message_completed → finalizeCommentResponse (request_id-keyed)
                                       → clears request_id/queued_at, processNextCommentInQueue
  ⏰ 2-min backstop → handleCommentTimeout                                (line ~939)
```

Two seams break under load:

1. Between "comment queued" and "prompt dispatched" the comment has **no interaction id at all**.
2. Between "prompt dispatched" and "agent starts emitting" the interaction is `waiting` with
   **zero bytes**. On meta this gap was ~18 minutes because four earlier prompts were in flight.

Both look identical to "the agent is silent" under today's rules.

## Decision 1 — Fix the state machine, keep `commentResponseTimeout = 2m`

The timer stays a 2-minute *check* interval; it is the *decision* it makes that changes. Bumping
the constant would only move the false positive later and would delay the genuine-failure signal.
Rewrite `handleCommentTimeout` as:

```
0. comment resolved (RequestID == "" || AgentResponse is real)     → return
1. InteractionID == "":
     PromptID != "" && prompt status ∈ {pending, sending}          → re-arm (agent hasn't got it)
     otherwise                                                      → stamp
2. InteractionID != "", interaction load fails                     → re-arm (transient DB error;
                                                                      today this stamps — wrong)
3. interaction terminal (complete/interrupted/error)               → finalizeCommentResponse  [existing]
4. non-terminal, has text, Updated stale > 1 window                → finalize partial          [existing]
5. non-terminal, has text, Updated fresh                           → re-arm                    [existing]
6. non-terminal, EMPTY text                                        → re-arm                    [NEW — Bug 1]
7. any re-arm path with attempt >= maxCommentTimerRearms           → stamp + ERROR log          [NEW — US3]
```

Only branches 1/2/6/7 are new. Everything else is preserved byte-for-byte so the existing suite
keeps passing.

**Bounding the re-arm.** `armCommentTimer(sessionID, commentID)` gains an `attempt int`
parameter carried in the `time.AfterFunc` closure; `processNextCommentInQueue` passes `0`, the
timer passes `attempt+1`. New const:

```go
// maxCommentTimerRearms bounds the backstop timer's re-arm loop. 30 × 2m = 60 minutes
// of no observable progress before we stamp. Long agent turns on meta run ~19 minutes,
// so this is ~3× headroom. Exhausting it means something is genuinely dead.
const maxCommentTimerRearms = 30
```

On exhaustion: stamp, `log.Error()` with `session_id`, `comment_id`, `interaction_id`,
`prompt_id`, `interaction_state`, `attempts`, `elapsed`, then `processNextCommentInQueue`.

**Why an in-memory counter and not a DB column.** The timer is already in-memory
(`s.sessionCommentTimeout[sessionID]`), and a restart is already handled by
`ResumeCommentQueueProcessing` → `reconcileStuckInFlightComment`, which finalizes any
terminal-but-unfinalized comment. A restart therefore grants a fresh waiting window — strictly
safer than stamping. No migration, no new column.

## Decision 2 — Repair keyed on `interaction_id`, not a new `timed_out` column

Keep `request_id = ""` on stamp. Rationale:

- It is what unblocks the queue (`IsCommentBeingProcessedForSession` counts `request_id != ''`).
- `GetNextQueuedCommentForSession` requires `agent_response` empty, so a stamped comment is never
  re-sent to the agent even though `request_id` is now clear. Semantics stay consistent.
- The frontend's in-flight test is `request_id && !agent_response` — unchanged.
- A `timed_out` column would need an AutoMigrate plus edits to three queries and buys nothing the
  interaction-id key does not already give.

Make the repair reachable instead:

```go
// finalizeCommentResponse keeps its signature for existing callers.
func (s *HelixAPIServer) finalizeCommentResponse(ctx context.Context, requestID string) error {
	return s.finalizeCommentResponseForInteraction(ctx, requestID, "")
}

// finalizeCommentResponseForInteraction is the real body. interactionID is the
// completing interaction; it is used as a fallback lookup key when the comment's
// request_id has already been cleared (e.g. by a premature timer stamp).
func (s *HelixAPIServer) finalizeCommentResponseForInteraction(ctx context.Context, requestID, interactionID string) error
```

Lookup order inside:

1. `GetCommentByRequestID(requestID)` when `requestID != ""`.
2. On miss (or empty `requestID`) and `interactionID != ""`:
   `GetCommentByInteractionID(interactionID)` — then **guard**:
   - `comment.InteractionID == interactionID` exactly (never a fuzzy/session match), and
   - `comment.AgentResponse == "" || comment.AgentResponse == CommentTimerNoResponseMessage`
     — a comment holding a real answer is left alone.
   Otherwise return the existing "no comment found" error (callers log it at debug; it is normal).
3. From there the code is unchanged — the existing `needsPopulation` / `hadStaleTimerError` block
   copies `ResponseMessage` **and** `ResponseEntries` and logs 🔁. That block stops being dead code.

**Why exact-id matching prevents clobbering a re-send.** A re-sent comment is a *new row* with a
*new* `interaction_id`. `GetCommentByInteractionID` matches on the column, so the late completion
for the old interaction resolves to the old row only. The explicit equality check is belt-and-braces
against a future store change.

## Decision 3 — Wire the interaction id through both `message_completed` branches

`targetInteraction` is already in scope at `websocket_external_agent_sync.go:~3230`.

- `request_id` branch (~3260): `finalizeCommentResponseForInteraction(ctx, messageRequestID, targetInteraction.ID)`.
- No-`request_id` fallback (~3285): `GetPendingCommentByPlanningSessionID` requires
  `request_id != ''` and therefore **cannot** see a stamped comment. Add an interaction-id attempt
  before/after it: if the pending lookup misses, call
  `finalizeCommentResponseForInteraction(ctx, "", targetInteraction.ID)`. Keep the existing
  `requestToCommenterMapping` cleanup on the path that has a request id.

`linkAgentResponseToComment` (line ~1342) already looks up by interaction id but overwrites
unconditionally — add the same "don't clobber a real response" guard there for consistency.

## What is deliberately not changed

- `commentResponseTimeout = 2 * time.Minute` — stays (Open Question 1 covers the cap value).
- `CommentTimerNoResponseMessage` string — must stay byte-identical: `hadStaleTimerError` keys off
  it and the meta backfill query matches on it.
- `updateCommentWithStreamingResponse` — still `request_id`-keyed. A stamped comment shows no
  mid-stream text but is healed at completion. Flagged as Open Question 5.
- Store schema and all four comment queries.

## Data repair (PR description only — do NOT run against meta)

Diagnostic (returns 112 rows on meta; 91 with a `complete` interaction and non-empty response):

```sql
SELECT c.id, c.updated_at, i.state, length(i.response_message)
FROM spec_task_design_review_comments c
JOIN interactions i ON i.id = c.interaction_id
WHERE c.agent_response = '[Agent did not respond - try sending your comment again]';
```

One-off repair:

```sql
UPDATE spec_task_design_review_comments c
SET agent_response         = i.response_message,
    agent_response_entries = i.response_entries,
    agent_response_at      = COALESCE(c.agent_response_at, i.updated),
    updated_at             = NOW()
FROM interactions i
WHERE i.id = c.interaction_id
  AND c.agent_response = '[Agent did not respond - try sending your comment again]'
  AND i.state = 'complete'
  AND length(i.response_message) > 0;
```

Includes the single row from the incident (`stdrc_9bdb66aa4af6a30e1970468117fb5134` ←
`int_01kzrcgzmewg38s7p0ahw7bnm9`). Snapshot first; Luke decides when to run it.

## Test strategy

**Unit** — extend `CommentTimerSuite` (`spec_task_design_review_handlers_test.go`). The suite
already models the pattern: `gomock` strict mode means "no stamp" is asserted simply by *not*
expecting `UpdateSpecTaskDesignReviewComment`, and re-arm tests must `t.Stop()` the timer they
leave behind (see `TestHandleCommentTimeout_SkipsErrorWhenInteractionHasContent`). Empty-queue
lookups return `errNotFound{}`, not `(nil, nil)` — a nil comment nil-derefs.

**E2E in the inner Helix (mandatory).** Register `test@helix.ml` / `helixtest`, onboard, create a
spec task (a spec task is required — a bare `zed_external` chat session never connects because
there is no repo). Then:

1. Send a long prompt to the agent so it is busy.
2. Immediately post a design-review comment → its prompt queues behind.
3. Watch for > 2 minutes (past the first timer fire). Assert in the DB that `agent_response` is
   never the stamp, and in the UI that the pending indicator persists.
4. When the agent finishes, assert the comment renders the real answer.
5. Post a second comment and confirm the queue drains — the timer's unblocking role must survive.

Screenshots at each step into `screenshots/`. API hot-reloads via Air; no rebuild needed for
`api/pkg/server` changes.

## Notes for future agents

- **`comment.InteractionID` is empty until prompt dispatch.** Any "does this comment have an
  interaction?" logic must consider `comment.PromptID` + prompt status first, or it will
  mis-handle every comment queued behind other work. This is the non-obvious part of this bug.
- **Comment text lives on the interaction during streaming**, not on the comment.
  `comment.AgentResponse` is only written at `message_completed`. An empty `AgentResponse` proves
  nothing about agent progress — always consult `types.TextFromInteraction(interaction)`.
- **`request_id` is the whole repair keyring.** Anything that clears it orphans the row. If you
  clear it, provide another key (here: `interaction_id`).
- **Always carry `ResponseEntries` alongside `ResponseMessage`** when copying a response onto a
  comment, or tool calls stop rendering in `CommentLogSidebar.tsx`.
- Three independent healers already exist and should be kept coherent, not duplicated:
  `handleCommentTimeout` (terminal → finalize), `reconcileStuckInFlightComment` (startup +
  pre-dispatch), and `finalizeCommentResponse`'s `hadStaleTimerError` block.
