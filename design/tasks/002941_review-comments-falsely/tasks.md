# Implementation Tasks: Fix False "Agent Did Not Respond" Stamps on Review Comments

## 0. Reproduce first (do not skip)

- [ ] Bring up the inner Helix at `http://localhost:8080`; poll until `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` are `Up` and `8080` returns 200 (allow 5–10 min)
- [ ] Register `test@helix.ml` / `helixtest`, complete onboarding (org → project)
- [ ] Create a spec task and drive it to `spec_review` with a design review present
- [ ] Confirm its desktop container is gone: `docker compose exec -T sandbox-nvidia docker ps | grep ubuntu-external-<sid>`; stop it if still running
- [ ] Post a review comment so the comment itself triggers cold boot; watch `docker logs helix-api-1` and the comment row
- [ ] **Capture the pre-fix reproduction**: stamp appears at ~2 min while the agent later completes with real text (screenshot to `screenshots/` + DB row)

## 1. Defect 1 — timer must not count time while the agent is unreachable

- [ ] Rename `commentResponseTimeout` → `commentTimerInterval` (still 2m) and update its doc comment to say it is a poll interval, not a deadline
- [ ] Add branch: **agent not reachable** (`externalAgentWSManager.getConnection(sessionID)` false, or `comment.InteractionID == ""`) → re-arm, log at INFO with a dedicated "sandbox starting / agent not connected" message, never stamp
- [ ] Add branch: **connected, non-terminal, zero content** → re-arm (the long-agent-turn case)
- [ ] Add the two ceilings from the design: 30 min continuous disconnection → `CommentSandboxNotStartedMessage`; 60 min connected-with-no-progress → `CommentTimerNoResponseMessage`
- [ ] Derive both elapsed values from DB state (`QueuedAt`, `interactions.updated`, connection presence) — no new in-memory state, no new column
- [ ] Add `CommentSandboxNotStartedMessage` const with an accurate doc comment
- [ ] Verify the four existing branches (resolved / terminal / streaming / stalled) are unchanged

## 2. Defect 2 — one stable lookup path for a late completion

- [ ] Add `GetCommentForAgentRequest(ctx, id)` to the store (`WHERE request_id = ? OR interaction_id = ?`) + mockgen regeneration
- [ ] Switch `finalizeCommentResponse` to use it as its only lookup; keep `RequestID` clearing on stamp so the queue still unblocks
- [ ] Add the already-finalized guard (real `AgentResponse` + empty `RequestID` → DEBUG, return, do not re-trigger the queue)
- [ ] Add a WARN when a comment resolves via `interaction_id` with `request_id` empty (canary that the primary path was broken)
- [ ] **Rewrite the `CommentTimerNoResponseMessage` doc comment** to describe what the code actually does
- [ ] Confirm the existing `needsPopulation` / `hadStaleTimerError` repair branch is now reachable via `message_completed`

## 3. Defect 3 — make a lost answer loud

- [ ] Add `ErrNoCommentForAgentRequest` sentinel; return it from `finalizeCommentResponse` on a resolution miss
- [ ] At `websocket_external_agent_sync.go:3435`: DEBUG only for the sentinel; **ERROR** for anything else, with `request_id`/`interaction_id`/`comment_id` and a message stating an agent answer would be lost
- [ ] Apply the same treatment to the session-based fallback branch below it

## 4. Recover the 94 stranded answers

- [ ] Add `ListTimerStampedCommentsWithResponses(ctx, stamp, limit)` to the store using the brief's SQL (the `JOIN` excludes the 18 no-interaction rows)
- [ ] Add `reconcileTimerStampedComments(ctx)` using `types.TextFromInteraction`; set `agent_response`, `agent_response_entries`, `agent_response_at = interactions.updated`
- [ ] Call it as step 0 of `ResumeCommentQueueProcessing`, in a goroutine, batched (limit 1000) with per-batch and total repaired counts logged
- [ ] Never touch `request_id` / `queued_at` in the repair path
- [ ] Confirm idempotency: a second run repairs 0 rows

## 5. Unit tests

- [ ] `TestHandleCommentTimeout_DoesNotStampWhenAgentUnreachable`
- [ ] `TestHandleCommentTimeout_DoesNotStampWhenCommentNotYetDispatched`
- [ ] `TestFinalizeCommentResponse_RepairsAfterTimerStampClearedRequestID`
- [ ] `TestReconcileTimerStampedComments_RepairsMatchingRows` / `_SkipsRowsWithoutInteraction` / `_IsIdempotent`
- [ ] Rewrite `TestHandleCommentTimeout_StampsErrorWhenInteractionEmpty` to assert a re-arm on the first tick, and retarget stamp coverage at the ceiling case
- [ ] Run: `sudo apt-get install -y gcc libc6-dev && CGO_ENABLED=1 go test -run TestCommentTimerSuite ./pkg/server/ -count=1`

## 6. Verify live (mandatory)

- [ ] Re-run the §0 cold-start sequence **after** the fix: no false stamp, real answer lands on the comment
- [ ] Post a **second** comment on the same review — confirm it is delivered and answered (queue not blocked)
- [ ] Insert a synthetic stamped comment + completed interaction, restart the API, confirm the reconciliation pass repairs it and logs the count
- [ ] Capture screenshots of before/after comment threads into `screenshots/`

## 7. Ship

- [ ] `cd api && go build ./pkg/...`
- [ ] `cd frontend && yarn build`
- [ ] Write `design/2026-08-24-review-comment-timer-false-negative.md` in the helix repo: chosen timer semantics, the single lookup path, the backfill strategy, and the ceilings' justification
- [ ] Commit with conventional-commit messages; open one PR; report the full URL (`https://github.com/helixml/helix/pull/NNN`)
- [ ] `gh pr checks <num>` — investigate and fix any CI failure without being asked
- [ ] Do not touch `spt_01m0sh2mqx8491eg62fkp9qrap` (sandbox defaults + stream init replay)
