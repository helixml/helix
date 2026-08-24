# Implementation Tasks: Fix False "Agent Did Not Respond" Stamps on Review Comments

## 0. Reproduce first (do not skip)

- [x] Bring up the inner Helix at `http://localhost:8080`; poll until `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` are `Up` and `8080` returns 200 (allow 5–10 min)
- [x] Register `test@helix.ml` / `helixtest`, complete onboarding (org → project)
- [x] Configure prerequisites discovered during setup (see design.md "Environment setup"): system-settings default project agent, org harness provider allow-list
- [x] Create a spec task and drive it to `spec_review` with a design review present
- [x] Confirm its desktop container is gone (UI "Stop desktop"; container `ubuntu-external-<sid>` verified gone)
- [x] Post a review comment so the comment itself triggers cold boot; watch `docker logs helix-api-1` and the comment row
- [x] **Capture the pre-fix reproduction**: stamp at 16:32:38 (2 min), agent completed 16:34:21 with 6,791 chars, comment still 56 chars — `screenshots/01-before-false-stamp.png`
- [x] **New finding**: the completion's `request_id` (`req_…`) is not the id the comment stores (`int_…`) — Zed rebinds it mid-turn. Recorded in design.md; Decision 3 amended.

## 1. Defect 1 — timer must not count time while the agent is unreachable

- [x] Rename `commentResponseTimeout` → `commentTimerInterval` (still 2m) and update its doc comment to say it is a poll interval, not a deadline
- [x] Add branch: **agent not reachable** (`externalAgentWSManager.getConnection(sessionID)` false, or `comment.InteractionID == ""`) → re-arm, log at INFO with a dedicated "sandbox starting / agent not connected" message, never stamp
- [x] Add branch: **connected, non-terminal, zero content** → re-arm (the long-agent-turn case)
- [x] Add the two ceilings from the design: 30 min continuous disconnection → `CommentSandboxNotStartedMessage`; 60 min connected-with-no-progress → `CommentTimerNoResponseMessage`
- [x] Derive both elapsed values from DB state (`QueuedAt`, `interactions.updated`, connection presence) — no new in-memory state, no new column
- [x] Add `CommentSandboxNotStartedMessage` const with an accurate doc comment
- [x] Verify the four existing branches (resolved / terminal / streaming / stalled) are unchanged

## 2. Defect 2 — one stable lookup path for a late completion

- [x] Add `GetCommentByAgentRequestIDs(ctx, ids)` to the store (`WHERE request_id IN ? OR interaction_id IN ?`) + mockgen regeneration
- [x] Add `resolveCommentForAgentRequest` which normalises the completion id via `GetInteractionByExternalAgentRequestID` before querying (covers the request-id rebind found in the repro)
- [x] Switch `finalizeCommentResponse` to use it as its only lookup; keep `RequestID` clearing on stamp so the queue still unblocks
- [x] Add the already-finalized guard (real `AgentResponse` + empty `RequestID` → DEBUG, return, do not re-trigger the queue)
- [x] Add a WARN when a comment resolves via `interaction_id` with `request_id` empty (canary that the primary path was broken)
- [x] **Rewrite the `CommentTimerNoResponseMessage` doc comment** to describe what the code actually does
- [x] Confirm the existing `needsPopulation` / `hadStaleTimerError` repair branch is now reachable via `message_completed`

## 3. Defect 3 — make a lost answer loud

- [x] Add `ErrNoCommentForAgentRequest` sentinel; return it from `finalizeCommentResponse` on a resolution miss
- [x] At `websocket_external_agent_sync.go:3435`: DEBUG only for the sentinel; **ERROR** for anything else, with `request_id`/`interaction_id`/`comment_id` and a message stating an agent answer would be lost
- [x] Apply the same treatment to the session-based fallback branch below it

## 4. Recover the 94 stranded answers

- [x] Add `ListTimerStampedCommentsWithResponses(ctx, stamp, limit)` to the store using the brief's SQL (the `JOIN` excludes the 18 no-interaction rows)
- [x] Add `reconcileTimerStampedComments(ctx)` using `types.TextFromInteraction`; set `agent_response`, `agent_response_entries`, `agent_response_at = interactions.updated`
- [x] Call it as step 0 of `ResumeCommentQueueProcessing`, in a goroutine, batched (limit 1000) with per-batch and total repaired counts logged
- [x] Never touch `request_id` / `queued_at` in the repair path
- [x] Confirm idempotency: a second run repairs 0 rows

## 5. Unit tests

- [x] `TestHandleCommentTimeout_DoesNotStampWhenAgentUnreachable`
- [x] `TestHandleCommentTimeout_DoesNotStampWhenCommentNotYetDispatched`
- [x] `TestFinalizeCommentResponse_RepairsAfterTimerStampClearedRequestID`
- [x] `TestReconcileTimerStampedComments_RepairsMatchingRows` / `_NoopWhenNothingStranded` / `_LeavesStampWhenInteractionYieldsNoText`
- [x] Replaced `TestHandleCommentTimeout_StampsErrorWhenInteractionEmpty` with `_DoesNotStampWhenAgentUnreachable`; stamp coverage retargeted at `_StampsSandboxFailureAfterCeiling` and `_StampsNoResponseAfterSilentCeiling`
- [x] Added `TestFinalizeCommentResponse_ResolvesWhenAgentRebindsRequestID` and `_ReturnsSentinelWhenNoComment` for the repro findings
- [x] **19/19 tests pass** (`CGO_ENABLED=1 go test -run TestCommentTimerSuite ./pkg/server/ -count=1`)
- [x] Run: `sudo apt-get install -y gcc libc6-dev && CGO_ENABLED=1 go test -run TestCommentTimerSuite ./pkg/server/ -count=1`

## 6. Verify live (mandatory)

- [x] Re-run the §0 cold-start sequence **after** the fix: no false stamp, real answer lands on the comment
- [x] Post a **second** comment on the same review — delivered and answered in ~30s (queue not blocked)
- [x] Backfill verified against **real** stranded data: the pre-fix repro comment was recovered on first boot (`repaired=1`, `agent_response_at=16:34:21` from the interaction, not now)
- [x] Cold-start round 2 (hard `docker stop`): **no false stamp at the 2-min mark**; the timer waited and only acted when the interaction reached a terminal error state
- [x] Cold-start round 3 (graceful stop, agent reconnects): real 1,655-char answer, no stamp
- [x] Capture screenshots of before/after comment threads into `screenshots/`

## 7. Ship

- [x] `cd api && go build ./pkg/...`
- [x] `cd frontend && yarn build` — bundle compiles (22,300 modules); in-place write blocked by the root-owned read-only `dist` bind mount, verified via `vite build --outDir /tmp/fe-dist`
- [x] Write `design/2026-08-24-review-comment-timer-false-negative.md` in the helix repo: chosen timer semantics, the single lookup path, the backfill strategy, and the ceilings' justification
- [x] Commit with conventional-commit messages; merge `origin/main`; push `feature/002941-fix-false-agent-did-not`
- [x] Write `pull_request_helix.md` (the platform opens the PR)
- [~] Check CI after the PR is opened; fix failures without being asked
- [x] Did not touch `spt_01m0sh2mqx8491eg62fkp9qrap` (sandbox defaults + stream init replay)
