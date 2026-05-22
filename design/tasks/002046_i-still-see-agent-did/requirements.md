# Requirements: Stop False "[Agent did not respond]" When Agent Streams Past 2-Minute Timer

## Background

The "[Agent did not respond - try sending your comment again]" error has been
"fixed" multiple times — see prior specs `001523_in-the-comment-queue`,
`001569_still-seeing-agent-did`, `001640_commenting-on-a-spec` and commits
`ec845a4aa` (finalize before publish), `a93665af0` (frontend isComplete flag),
`2186abcda` (dedup message_completed), `1657323d3` (preserve interrupted state).
The user still sees it. None of the prior fixes addressed the actual root cause.

## Root Cause (verified against current code)

The 2-minute timeout in `processNextCommentInQueue`
(`api/pkg/server/spec_task_design_review_handlers.go:818`) checks the **comment
row's `AgentResponse`** field to decide whether the agent responded:

```go
// line 832
if currentComment.RequestID != "" && currentComment.AgentResponse == "" {
    currentComment.AgentResponse = "[Agent did not respond - try sending your comment again]"
    ...
}
```

**But the comment's `AgentResponse` is NEVER populated during streaming on the
modern code path.** Verified by grep: the only caller of
`UpdateCommentAgentResponse` is `updateCommentWithStreamingResponse`, which is
only invoked from the legacy `handleChatResponse` (single-chunk path) at
`websocket_external_agent_sync.go:2328`. The modern path
(`message_added` → `handleMessageAdded` → updates
`targetInteraction.ResponseMessage` and `ResponseEntries`) writes streaming
content to the **interaction**, not the comment. The comment's `AgentResponse`
stays empty until `finalizeCommentResponse` runs on `message_completed`.

So whenever an agent takes longer than 2 minutes to emit `message_completed`
(common for: long answers, multi-tool-call turns, thinking-heavy responses,
slow models, or the agent reconnecting after a stream supersede), the timer
fires with `AgentResponse == ""` even though the agent IS actively streaming
content into the interaction row. The error message gets stamped onto the
comment.

**The error then becomes sticky** because `finalizeCommentResponse`
(`spec_task_design_review_handlers.go:1233`) has this guard:

```go
if comment.AgentResponse == "" && comment.InteractionID != "" {
    // populate from interaction
}
```

If the timer already stamped the error string, `AgentResponse != ""` so the
real response from the interaction is never copied across. The error sticks
forever, even though `message_completed` arrived 10 seconds later with the
real response in the interaction.

## User Stories

**US-1: Long agent responses don't get falsely flagged**
> As a reviewer commenting on a spec, when the agent's reply takes more than
> 2 minutes (long answer, tool calls, thinking), I should see the agent's real
> response when it arrives — not the "did not respond" error.

**US-2: Late `message_completed` overwrites a stale error**
> As a reviewer, if the timer fired prematurely while the agent was still
> working, when `message_completed` eventually arrives with a real response,
> the comment should display the real response, not the leftover error.

**US-3: Genuine no-response still shows the error**
> As a reviewer, if the agent truly did not respond (desktop dead, no
> streaming, no `message_completed`), I should still see the "did not respond"
> error so I know to resend. The fix must not silence the legitimate case.

## Acceptance Criteria

- **AC-1:** When the agent is actively streaming response content into the
  interaction (`interaction.ResponseMessage` or `ResponseEntries` is non-empty
  and `interaction.State == waiting`), the 2-minute timer must NOT stamp the
  error message on the comment. The timer should either extend, skip, or
  recheck on a longer schedule.

- **AC-2:** When `finalizeCommentResponse` runs and the interaction has real
  response content, the comment's `AgentResponse` must be replaced with the
  real content even if `AgentResponse` currently contains the error string
  `"[Agent did not respond - try sending your comment again]"`.

- **AC-3:** The error message must still appear on a comment when ALL of the
  following are true at timeout: comment still has `RequestID` set, comment's
  `AgentResponse` is empty, **AND** the linked interaction (if any) has empty
  `ResponseMessage` and `ResponseEntries` AND is not in state
  `complete`/`interrupted`/`error`. (i.e. truly nothing happened)

- **AC-4:** Tested end-to-end in the inner Helix at `http://localhost:8080`:
  send a comment that elicits a 3+ minute streaming response (e.g. "implement
  X feature step by step with detailed explanation"), confirm no error
  message appears at any point and the final comment shows the agent's
  actual response.

- **AC-5:** Regression test for the sticky case: simulate timer firing
  (manually set `AgentResponse` to the error string), then trigger
  `finalizeCommentResponse` with an interaction that has real content;
  comment must end up with real content, not the error string.

- **AC-6:** Existing Go test coverage in
  `api/pkg/server/spec_task_design_review_handlers_test.go` and
  `websocket_external_agent_sync_test.go` continues to pass; new test
  pins down the "interaction has content while comment is empty" timer skip.

## Out of Scope

- The frontend transition flicker between comments (already fixed by
  `a93665af0` via `isComplete`).
- Order of `finalizeCommentResponse` vs `publishInteractionUpdateToFrontend`
  (already fixed by `ec845a4aa`).
- Empty-response `message_completed` re-queueing for non-comment paths
  (already fixed by `1657323d3`).
