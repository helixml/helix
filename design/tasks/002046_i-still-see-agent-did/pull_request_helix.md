# fix(api): stop false "agent did not respond" while agent is streaming

## Summary

Resolves the long-standing complaint that the comment view shows
`[Agent did not respond - try sending your comment again]` even after
the agent successfully replies. Prior attempts (PRs around
`ec845a4aa`, `a93665af0`, `1657323d3`) closed adjacent races but never
fixed the root cause.

**Root cause:** the 2-minute comment-response timer in
`processNextCommentInQueue` decides whether the agent responded by
reading `comment.AgentResponse`. But the modern streaming path writes
to `interaction.ResponseMessage` / `ResponseEntries`; the comment row
is only populated by `finalizeCommentResponse` when
`message_completed` fires. So any agent reply that takes more than 2
minutes to finish (long answers, multi-tool turns, thinking) trips the
timer and stamps the error onto the comment — even though the agent is
actively streaming.

Worse, the error was sticky: `finalizeCommentResponse` had a guard
`if comment.AgentResponse == ""` that skipped repopulation when the
timer had already stamped the error string.

## Changes

- `api/pkg/server/spec_task_design_review_handlers.go`
  - Hoist the error message into
    `const CommentTimerNoResponseMessage` so timer + finalize agree
    on the exact string.
  - Refactor the `time.AfterFunc` closure body into a named method
    `handleCommentTimeout(ctx, sessionID, commentID)` so it can be
    unit-tested directly without spinning a real timer.
  - In `handleCommentTimeout`, after the existing early-exit checks,
    load the linked interaction and call `types.TextFromInteraction`.
    If the interaction has any text content OR is in a terminal state
    (`complete` / `interrupted` / `error`), log a skip and return
    without stamping the error.
  - In `finalizeCommentResponse`, treat
    `AgentResponse == CommentTimerNoResponseMessage` as "needs
    population" so a late `message_completed` repairs the comment
    with the real response. Log at `warn` when overwriting a stale
    timer-stamp so operators can see it happen.
  - Use `context.Background()` for the deferred timer body so it
    never reads from a request-scoped context that may already be
    cancelled when the timer fires 2 minutes later.

- `api/pkg/server/spec_task_design_review_handlers_test.go` (new file)
  - `CommentTimerSuite` with six tests covering:
    - Timer skips error when interaction has content (streaming).
    - Timer skips error when interaction is in a terminal state.
    - Timer stamps error when interaction is empty + waiting
      (regression guard for the legitimate no-response case).
    - Timer is a no-op when the comment is already resolved.
    - Finalize overwrites the stale timer-stamped error with the
      real interaction text.
    - Finalize populates an empty comment from the interaction
      (happy path regression guard).

## Test plan

- [x] `CGO_ENABLED=1 go test -v -run TestCommentTimerSuite ./api/pkg/server/ -count=1`
      — all 6 new tests pass.
- [x] `CGO_ENABLED=1 go test -run WebSocketSync ./api/pkg/server/ -count=1`
      — full pre-existing suite still green.
- [x] `CGO_ENABLED=0 go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
      — clean.
- [ ] **E2E manual verification (reviewer)** — could not run in the
      spec-task sandbox (no inner Helix running, no docker). Suggested
      check:
  1. Create a spec task and drive it to the design-review page.
  2. Add a comment that elicits a 3+ minute streaming response
     (e.g. "walk me through how you'd refactor the auth system step
     by step with reasoning"). Watch the comment as the timer would
     fire at t=2:00 — confirm no error string appears.
  3. For the sticky-error repair: while agent is streaming, run
     `UPDATE spec_task_design_review_comments SET agent_response =
     '[Agent did not respond - try sending your comment again]' WHERE
     id = '<id>';` then wait for `message_completed`. Confirm the
     row is overwritten with the real response.

## Linked design

`helix-specs` branch — task `002046_i-still-see-agent-did/` —
requirements.md / design.md / tasks.md describe the full diagnosis.
