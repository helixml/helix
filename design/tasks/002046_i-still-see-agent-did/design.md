# Design: Stop False "[Agent did not respond]" When Agent Streams Past 2-Minute Timer

## TL;DR

Two changes, both in `api/pkg/server/spec_task_design_review_handlers.go`:

1. **Timer must check the interaction, not just the comment row.** During
   streaming, response content lives in `interaction.ResponseMessage` /
   `ResponseEntries`, not `comment.AgentResponse`. Today the timer only looks
   at the comment, which is misleadingly empty during streaming.

2. **`finalizeCommentResponse` must overwrite the error message.** The
   `if comment.AgentResponse == ""` guard makes the error sticky once stamped.
   Drop the guard, OR treat the literal error string as "empty".

## Why this is the right fix (and prior fixes weren't)

| Prior fix | What it did | Why it's not enough |
|---|---|---|
| `ec845a4aa` | Run `finalizeCommentResponse` synchronously **before** publish | Closed a frontend cache race. But the timer can fire BEFORE `message_completed` ever arrives — finalize never runs. |
| `a93665af0` | Frontend keeps `streamingResponse` visible after stream completes | Prevents flicker during the cache-refresh gap. Has nothing to do with the backend timer that stamps an error string into the DB. |
| `1657323d3` | Preserve `Interrupted` state when `message_completed` has empty response | About the cancel/interrupt race for non-comment turns. Doesn't touch the comment timer. |
| `2186abcda` | State-based dedup of `message_completed` | Prevents double-completion clobber. Unrelated to the 2-minute timer. |

None of them touched the central asymmetry: **the timer reads the comment row;
streaming writes to the interaction row.** That's the bug.

## Key Files

| File | Role |
|---|---|
| `api/pkg/server/spec_task_design_review_handlers.go` | `processNextCommentInQueue` (timer setup, line 733; timer body line 818-843), `finalizeCommentResponse` (line 1215), `updateCommentWithStreamingResponse` (line 1180, only called from legacy path) |
| `api/pkg/server/websocket_external_agent_sync.go` | `handleMessageCompleted` (line 2417) — calls `finalizeCommentResponse`; `handleMessageAdded` streaming path that writes to interaction |
| `api/pkg/store/spec_task_design_review_store.go` | `UpdateCommentAgentResponse` (line 85, partial), `UpdateSpecTaskDesignReviewComment` (line 79, full Save) |
| `api/pkg/types/types.go` | `TextFromInteraction` (line 3228) — pulls text from `ResponseEntries`, falls back to `ResponseMessage` |

## Change 1: Timer-side fix (don't stamp error while agent is streaming)

Modify the `time.AfterFunc` body at `spec_task_design_review_handlers.go:818`.
Before stamping the error, load the linked interaction and check whether the
agent has actually been silent.

```go
s.sessionCommentTimeout[sessionID] = time.AfterFunc(commentResponseTimeout, func() {
    currentComment, fetchErr := s.Store.GetSpecTaskDesignReviewComment(ctx, commentID)
    if fetchErr != nil { ...; return }

    // Original quick-exit conditions (unchanged).
    if currentComment.RequestID == "" || currentComment.AgentResponse != "" {
        return
    }

    // NEW: check the linked interaction. During modern streaming, content
    // lands in interaction.ResponseEntries / ResponseMessage, NOT on the
    // comment row. If the interaction has any content OR the interaction
    // is already in a terminal state, the agent IS responding — let
    // finalizeCommentResponse handle it.
    if currentComment.InteractionID != "" {
        interaction, ierr := s.Store.GetInteraction(ctx, currentComment.InteractionID)
        if ierr == nil && interaction != nil {
            agentText := types.TextFromInteraction(interaction)
            terminal := interaction.State == types.InteractionStateComplete ||
                interaction.State == types.InteractionStateInterrupted ||
                interaction.State == types.InteractionStateError
            if agentText != "" || terminal {
                log.Info().
                    Str("comment_id", commentID).
                    Str("interaction_id", interaction.ID).
                    Str("interaction_state", string(interaction.State)).
                    Int("interaction_response_len", len(agentText)).
                    Msg("⏭️  Comment timer: skipping error stamp — interaction has content or is terminal")
                // Reschedule a shorter recheck in case finalize gets stuck.
                // Optional; alternative is to bail entirely and trust message_completed.
                return
            }
        }
    }

    // Genuine no-response: stamp the error (existing behavior).
    currentComment.AgentResponse = "[Agent did not respond - try sending your comment again]"
    currentComment.RequestID = ""
    currentComment.QueuedAt = nil
    if updateErr := s.Store.UpdateSpecTaskDesignReviewComment(ctx, currentComment); updateErr != nil { ... }
    go s.processNextCommentInQueue(ctx, sessionID)
})
```

This is the minimum change needed to fix AC-1 and AC-3.

**Why not just extend the timeout to 10 minutes?** Long agent turns can
exceed any fixed timeout. The right question is "is the agent making
progress?", not "did it finish in N minutes". Looking at the interaction
state is the authoritative answer to that question.

## Change 2: Finalize-side fix (overwrite stale error)

Modify `finalizeCommentResponse` at `spec_task_design_review_handlers.go:1233`:

```go
// Before:
if comment.AgentResponse == "" && comment.InteractionID != "" {
    interaction, interactionErr := s.Store.GetInteraction(ctx, comment.InteractionID)
    if interactionErr == nil {
        text := types.TextFromInteraction(interaction)
        if text != "" {
            comment.AgentResponse = text
            ...

// After: treat the literal timeout-stamped error as "empty" so we overwrite it.
const timerErrorMsg = "[Agent did not respond - try sending your comment again]"
needsPopulation := comment.AgentResponse == "" || comment.AgentResponse == timerErrorMsg
if needsPopulation && comment.InteractionID != "" {
    interaction, interactionErr := s.Store.GetInteraction(ctx, comment.InteractionID)
    if interactionErr == nil {
        text := types.TextFromInteraction(interaction)
        if text != "" {
            if comment.AgentResponse == timerErrorMsg {
                log.Warn().
                    Str("comment_id", comment.ID).
                    Str("interaction_id", comment.InteractionID).
                    Int("response_length", len(text)).
                    Msg("🔁 Overwriting stale 'did not respond' error with real agent response")
            }
            comment.AgentResponse = text
            comment.AgentResponseEntries = interaction.ResponseEntries
            now := time.Now()
            comment.AgentResponseAt = &now
        }
    }
}

// Same idea for populateAgentResponseFromSession fallback — also check error string.
if (comment.AgentResponse == "" || comment.AgentResponse == timerErrorMsg) {
    s.populateAgentResponseFromSession(ctx, comment)
}
```

Also update `populateAgentResponseFromSession`
(`spec_task_design_review_handlers.go:1340`) to mirror the same overwrite
semantics — currently it has no guard but its caller does, so be consistent.

This satisfies AC-2: any late `message_completed` repairs the comment even
if the timer beat it.

## Hoist the constant

Define `const CommentTimerNoResponseMessage = "[Agent did not respond - try sending your comment again]"`
at package scope so the same string is used by:
- the timer (when stamping)
- `finalizeCommentResponse` (when detecting and overwriting)
- the test (when asserting)

No string drift across the three sites.

## Tests

Add to `api/pkg/server/spec_task_design_review_handlers_test.go` (or a new
file if cleaner):

1. **`TestCommentTimer_SkipsErrorWhenInteractionHasContent`** — set up a
   comment with `RequestID` set + empty `AgentResponse`, a linked interaction
   in `Waiting` state with `ResponseMessage="streaming so far..."`. Call the
   timer body directly; assert `UpdateSpecTaskDesignReviewComment` is NOT
   called with the error string.

2. **`TestCommentTimer_StampsErrorWhenInteractionEmpty`** — same setup but
   interaction has empty response and state `Waiting`. Assert error IS
   stamped (regression guard for AC-3).

3. **`TestFinalize_OverwritesStaleTimerError`** — comment has
   `AgentResponse = CommentTimerNoResponseMessage`, interaction has real
   text. Call `finalizeCommentResponse`; assert comment ends with real text,
   not the error.

4. **`TestFinalize_PreservesUserVisibleResponse`** — comment already has a
   real response from streaming (not the error string). Call finalize with
   an interaction that has a different/newer response. Decide and pin the
   semantic — recommend: prefer interaction's content as authoritative
   (matches AC-2 spirit). If we keep the existing "don't overwrite real
   text" semantic, document why.

To extract the timer body for testability, refactor the closure into a
named method `(s *HelixAPIServer) handleCommentTimeout(ctx, sessionID, commentID)`
that the closure calls. This is a small refactor with no behavior change but
unlocks direct unit testing.

## E2E verification (REQUIRED — per CLAUDE.md "PREFER end-to-end testing")

In the inner Helix at `http://localhost:8080`:

1. Register `test@helix.ml` / `helixtest`, complete onboarding (testorg →
   testproj → claude-opus-4-6 auto-selects), create a spec task, drive it to
   the design-review page.
2. Add a comment that will elicit a long response, e.g. "Walk me through how
   you'd refactor the entire authentication system, step by step with
   reasoning for each step." — something that streams for 3+ minutes.
3. Verify in the UI:
   - During streaming: comment shows the agent's streaming content (not the
     error).
   - At t=2:00 (when timer would fire): NO error message appears.
   - When agent finishes: comment shows the real response.
4. Cross-check the DB:
   `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, request_id, length(agent_response), substring(agent_response, 1, 120) FROM spec_task_design_review_comments WHERE review_id = '<id>' ORDER BY created_at;"`
5. Force the failure scenario for AC-2: while the agent is responding,
   manually run
   `UPDATE spec_task_design_review_comments SET agent_response = '[Agent did not respond - try sending your comment again]' WHERE id = '<comment_id>';`
   then wait for `message_completed`. Verify the row is updated to the real
   response.

## Notes for the implementer

- **Don't extend the 2-minute timeout instead of fixing the check.** That
  just shifts the problem. The check itself is wrong.
- **Don't change `UpdateCommentAgentResponse` semantics.** It's used by
  `updateCommentWithStreamingResponse` (legacy path) and the streaming-time
  partial-write protection is correct. The bug is in the timer + finalize.
- **`GetSpecTaskDesignReviewComment` and `GetInteraction` are both safe
  to call from inside the timer closure** — they hit Postgres directly, no
  WebSocket dependency, fine on a goroutine.
- **The timer closure currently captures `ctx` from
  `processNextCommentInQueue`, which is `context.Background()` (line 725
  via goroutine spawn) or the HTTP request context (other call sites).
  HTTP ctx may be cancelled by then.** Recommend the closure use
  `context.Background()` explicitly to avoid using a stale request context
  for a 2-minute-deferred DB call. (Small additional fix — flag it in
  the PR even if the main bug doesn't depend on it.)
- The `sessionCommentMutex` only protects the timer map, not concurrent
  comment writes. Optimistic locking on the comment row is not in scope —
  prior fixes have lived with last-write-wins on `Save()`.

## Patterns Learned (for future agents)

- **Comment data lives in two places during the response lifecycle.** While
  the agent streams, content is on the `interactions` row
  (`response_message`, `response_entries`); only at `message_completed` does
  it get copied to `spec_task_design_review_comments.agent_response`. Any
  check that reasons about "has the agent responded" must look at the
  interaction, not just the comment.
- **The 2-minute comment timer is keyed by session, but its closure captures
  a specific `commentID`.** Map overwrites on subsequent comments don't
  cancel the in-flight closure for the prior comment — `time.Timer.Stop()`
  is best-effort once the closure has begun executing. The check inside
  the closure must therefore be idempotent against state changes that
  happened between scheduling and firing.
- **`UpdateSpecTaskDesignReviewComment` uses GORM's `Save()` (full-row
  upsert) — last write wins with no version column.** The
  `UpdateCommentAgentResponse` partial-update method was introduced
  specifically to avoid streaming updates clobbering resolution status.
  Use the partial-update path when you only mean to touch agent_response.
- **`TextFromInteraction` prefers `ResponseEntries` (structured) over
  `ResponseMessage` (flat string).** Both can be present; entries is
  authoritative. Use the helper, never read the fields directly.
