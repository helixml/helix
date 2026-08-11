# Implementation Tasks: Fix Design-Review Comments Falsely Stamped "Agent Did Not Respond"

## Backend — timer state machine (Bug 1)

- [ ] Add `maxCommentTimerRearms = 30` const next to `commentResponseTimeout` with a comment explaining the 60-minute bound
- [ ] Add an `attempt int` parameter to `armCommentTimer` and thread it through the `time.AfterFunc` closure into `handleCommentTimeout`
- [ ] Update the `armCommentTimer` call in `processNextCommentInQueue` (line ~848) to pass `0`
- [ ] In `handleCommentTimeout`, re-arm (do not stamp) when the linked interaction is non-terminal with empty text
- [ ] In `handleCommentTimeout`, re-arm (do not stamp) when `InteractionID == ""` but the comment's `PromptID` is still `pending`/`sending` per `GetPromptHistoryEntry`
- [ ] In `handleCommentTimeout`, re-arm (do not stamp) when the interaction load returns an error — today this falls through to the stamp
- [ ] Stamp + `log.Error()` (session/comment/interaction/prompt ids, state, attempts, elapsed) once `attempt >= maxCommentTimerRearms`, then call `processNextCommentInQueue`
- [ ] Verify the preserved branches are untouched: terminal → finalize, non-terminal + text + stale `Updated` → finalize partial, non-terminal + text + fresh → re-arm, already-resolved → no-op
- [ ] Update the `handleCommentTimeout` doc comment to describe the new decision tree

## Backend — reachable repair path (Bug 2)

- [ ] Extract the body of `finalizeCommentResponse` into `finalizeCommentResponseForInteraction(ctx, requestID, interactionID string)`; keep `finalizeCommentResponse` as a wrapper passing `""`
- [ ] Add the `GetCommentByInteractionID` fallback when the `request_id` lookup misses or `requestID` is empty
- [ ] Guard the fallback: require `comment.InteractionID == interactionID` exactly, and require `AgentResponse` to be empty or exactly `CommentTimerNoResponseMessage`
- [ ] Confirm the existing `needsPopulation` / `hadStaleTimerError` block copies both `ResponseMessage` and `ResponseEntries` and sets `AgentResponseAt`
- [ ] Add the same "don't clobber a real response" guard to `linkAgentResponseToComment` (line ~1342)

## Backend — websocket wiring

- [ ] In `websocket_external_agent_sync.go` (~3260), pass `targetInteraction.ID` on the `request_id` branch
- [ ] In the no-`request_id` fallback (~3285), attempt `finalizeCommentResponseForInteraction(ctx, "", targetInteraction.ID)` when `GetPendingCommentByPlanningSessionID` misses
- [ ] Keep `requestToCommenterMapping` cleanup intact on the request-id path

## Unit tests (`spec_task_design_review_handlers_test.go`, extend `CommentTimerSuite`)

- [ ] Timer fires, interaction non-terminal with empty text → no `UpdateSpecTaskDesignReviewComment`, timer re-armed (stop the timer in the test)
- [ ] Timer fires, interaction terminal with empty text → stamp written, queue advances
- [ ] Timer fires, no interaction but prompt still pending → no stamp, timer re-armed
- [ ] Re-arm cap exhausted → stamp written, queue advances
- [ ] Late `message_completed` for a stamped comment with `request_id == ''` → stale stamp overwritten with real response and entries via the interaction-id fallback
- [ ] Late completion for an old interaction does not clobber a re-sent comment on a different interaction
- [ ] `CGO_ENABLED=1 go test -v -run TestCommentTimerSuite ./pkg/server/ -count=1` passes, existing suite tests included
- [ ] `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` clean

## E2E in the inner Helix (mandatory — do not report fixed without this)

- [ ] Confirm the stack is up (`docker compose -f docker-compose.dev.yaml ps`, `curl localhost:8080` → 200); wait several minutes if still booting
- [ ] Register `test@helix.ml` / `helixtest`, complete onboarding (org → project)
- [ ] Create a spec task and reach its design-review page
- [ ] Make the agent busy with a long-running prompt, then post a review comment so its prompt queues behind
- [ ] Wait past the 2-minute mark and confirm via `docker exec helix-postgres-1 psql` that `agent_response` is never the timer string
- [ ] Confirm the UI keeps the pending indicator, then shows the agent's real answer with tool-call entries rendered
- [ ] Post a second comment afterwards and confirm the queue still drains
- [ ] Save screenshots to `screenshots/` in this task directory
- [ ] Check API logs for the re-arm log lines and absence of unexpected stamps

## Delivery

- [ ] Commit with conventional commits, e.g. `fix(api): stop stamping design-review comments while the agent is still queued`
- [ ] Open the PR with the full URL; include in the description the diagnostic `SELECT`, the one-off repair `UPDATE`, and the meta counts (112 stamped / 91 healable)
- [ ] State explicitly in the PR that the repair was NOT run against meta — Luke decides
- [ ] Check CI (`gh pr checks` / Drone MCP tools) and fix failures without being asked
