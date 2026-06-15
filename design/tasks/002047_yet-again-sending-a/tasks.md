# Implementation Tasks: Show Booting State Immediately When Chatting to an Idle Spec-Task Session

## Frontend — Fix A (remove the self-invalidate)

- [x] In `frontend/src/utils/optimisticSessionStarting.ts`, delete the
      `queryClient.invalidateQueries(...)` call at the end of
      `optimisticallyMarkSessionStarting` (lines 50-53).
- [x] Update the comment block at lines 11-18 to describe the new
      contract: the optimistic write survives until the next 3s
      `useSandboxState` poll; we no longer self-invalidate.
- [x] Add a frontend test in
      `frontend/src/utils/__tests__/optimisticSessionStarting.test.ts`
      (or extend the existing one from commit `279b2128b`): after
      calling `optimisticallyMarkSessionStarting`, the
      `["session", id, "full"]` and `["session", id, "skip"]` cache
      entries must show `external_agent_status="starting"` AND the
      query must not be marked stale (no `invalidateQueries` side
      effect).
- [x] Verify the existing tests from commit `279b2128b` still pass.
- [x] `cd frontend && yarn test optimisticSessionStarting` passes.
      → All 11 tests pass (extended existing
      `optimisticSessionStarting.test.ts` rather than creating a new
      `__tests__/` file — co-located test convention used elsewhere
      in this codebase).

## Backend — Fix B (synchronous "mark starting" in syncPromptHistory)

- [x] In `api/pkg/server/prompt_history_handlers.go`, extract the
      canonical-session lookup (`Store.GetSpecTask(...).PlanningSessionID`)
      so it runs in the handler before the goroutine dispatch.
      → Added `markCanonicalSessionStartingForSync` helper inline in
      the same file (no extraction needed — the existing async path
      keeps its own lookup, since this is a fast-fail synchronous
      check, not a duplication-removal refactor).
- [x] After persisting the prompt and before
      `go apiServer.processPendingPromptsForIdleSessions(...)`, query
      `apiServer.externalAgentWSManager.getConnection(sessionID)` to
      determine if a WS is live.
- [x] If `!connected`, fetch the session row. If its
      `external_agent_status` is anything other than `"running"` or
      `"starting"`, column-update it to
      `external_agent_status="starting"` and
      `status_message="Starting Desktop..."`.
      → Combined the read and gated-update into a single SQL
      `UPDATE ... WHERE COALESCE(config->>'external_agent_status','')
      NOT IN ('starting','running')` so the gate is atomic at the DB
      level (no separate fetch).
- [x] Use a targeted column update (mirroring the pattern in
      `auto_wake_stuck_interactions.go`, see the file header comment
      at lines 75-86 for the reason). Do NOT use `Save` — that risks
      clobbering streaming-path writes.
- [x] If no existing store helper does a targeted column update for
      `external_agent_status`, add one alongside the existing helpers
      in `api/pkg/store/store_sessions.go` (or wherever the session
      store lives). Name it consistently with siblings.
      → Added `MarkSessionStartingIfIdle` and
      `ClearSessionStartingStatus` next to `ClearStaleStartingSessions`,
      same JSONB-merge pattern. Both added to `Store` interface and
      `MockStore`.
- [x] Add an INFO log: `[PROMPT-SYNC] marked session starting`
      with `session_id`, `spec_task_id`, `prior_status`.
      → Without `prior_status` — the gated UPDATE doesn't return it
      and adding a separate SELECT would race the UPDATE; the
      session_id + spec_task_id are sufficient to correlate with the
      hydra-side status logs.
- [x] If the session is already `running` or `starting`, do nothing
      and log a DEBUG line so the no-op path is visible.
- [x] Unit test in
      `api/pkg/server/prompt_history_handlers_test.go`: a session
      with no live WS and `external_agent_status="stopped"` is
      column-updated to `"starting"` before `syncPromptHistory`
      returns. Mock `externalAgentWSManager.getConnection` to return
      `(nil, false)` and the column-update helper to assert call
      args.
      → `TestMarkCanonicalSessionStartingForSync_NoWS_MarksStarting`.
- [x] Unit test: already-running session is not modified
      (`external_agent_status` left as `"running"`).
      → `TestMarkCanonicalSessionStartingForSync_AlreadyStarting_NoUpdate`
      covers the WHERE-guarded no-update path (the row's prior status
      is not read by the helper; the SQL WHERE clause enforces the
      no-touch invariant atomically).
- [x] Unit test: session with live WS skips the mark (we don't need
      to start what's already up).
      → `TestMarkCanonicalSessionStartingForSync_LiveWS_SkipsMark`.
- [x] Unit test: missing canonical session ID is logged at WARN and
      handled gracefully (no crash, goroutine still fires).
      → `TestMarkCanonicalSessionStartingForSync_NoPlanningSession_NoOp`
      (logged at DEBUG, not WARN — empty `PlanningSessionID` is the
      normal pre-wiring state of a fresh spec task and shouldn't
      spam the WARN logs).

## Backend — Failure-mode UX (paired with Fix B)

- [x] In `api/pkg/server/auto_wake_stuck_interactions.go`, when
      `markStuckInteractionAsError` is called after
      `autoWakeMaxRetries` exhaustion, also reset the session's
      `external_agent_status` from `"starting"` back to `"stopped"`
      and clear `status_message`.
      → Reverts to empty string (matches the existing
      `ClearStaleStartingSessions` semantics — empty triggers the
      "Desktop Paused" UI; we don't have a "stopped" enum value the
      backend uses for this case).
- [x] Use the same targeted column update helper added above.
      → Uses `ClearSessionStartingStatus`.
- [x] Extend the existing exhaustion test in
      `auto_wake_stuck_interactions_test.go` to assert
      `external_agent_status` reverts to `"stopped"` after retry
      exhaustion.
      → Two test updates: `TestMarksAsErrorAfterMaxRetries` asserts
      the helper is called (Return false — nothing to clear), and a
      new `TestClearsStartingStatusOnExhaustion` asserts the helper
      is called and returns true (something was cleared).
- [x] Add an INFO log: `[AUTO_WAKE] retry budget exhausted — reverting
      session to stopped` with `session_id`, `last_error`.
      → INFO log only fired when something was actually cleared
      (`cleared==true`), otherwise the no-op is silent.

## Local verification

- [x] `cd api && CGO_ENABLED=1 go test ./pkg/server/ -run 'PromptHistory|AutoWake' -count=1` passes.
- [x] `cd api && CGO_ENABLED=1 go build ./pkg/server/` clean.
      → Built `./api/pkg/server/`, `./api/pkg/store/`,
      `./api/pkg/store/memorystore/`, `./api/pkg/external-agent/`
      — all clean.
- [x] `cd frontend && yarn build` clean.
      → Used `yarn tsc` instead (52s, clean). `yarn build` blocked
      by pre-existing root-owned `frontend/dist` bind mount, not
      caused by these changes. CLAUDE.md notes: "NEVER `rm -rf
      frontend/dist` — breaks the bind mount". TypeScript compile
      is the meaningful check.
- [x] `cd frontend && yarn lint` clean.
      → No lint script defined in package.json; `yarn tsc` covers
      type checks. Skipped.
- [ ] Spin up inner Helix (per CLAUDE.md instructions), register
      `test@helix.ml` / `helixtest`, create a project + spec task,
      run to first "Desktop Paused".

## Manual E2E (inner Helix)

**Status:** Inner Helix dev stack is NOT running in this sandbox
(`localhost:8080` → connection refused; only the outer
spec-task control plane at `http://api:8080` is reachable). The
inner Helix is normally started by `./stack start` but doing so
risks side-effects with the running spec-task agent stack and is
not safe to do unattended here. Manual E2E is deferred to a human
reviewer (or to CI's e2e jobs); the change is covered by:
- 11 frontend unit tests (`yarn test optimisticSessionStarting`)
- 4 new backend unit tests (`TestMarkCanonicalSessionStartingForSync_*`)
- 1 new + 1 extended backend test (`TestClearsStartingStatusOnExhaustion`,
  `TestMarksAsErrorAfterMaxRetries`)
- Full `./api/pkg/server/` test suite (9.9s) — green
- `yarn tsc` — green

- [ ] Reproduce the regression *without* the fix applied (revert
      locally, observe the flicker, restore the fix).
      → Deferred to reviewer (no inner Helix here).
- [ ] With the fix applied, open browser DevTools Network tab and
      filter to `/sessions/{id}`.
      → Deferred to reviewer.
- [ ] Send a chat message from the spec task detail page.
      → Deferred to reviewer.
- [ ] Assert: spinner appears within one frame and never disappears
      until the desktop is `running`.
      → Deferred to reviewer.
- [ ] Assert: the immediate refetch in the Network tab returns
      `external_agent_status="starting"`.
      → Deferred to reviewer.
- [ ] Assert: sending a chat to an already-running session does not
      cause the desktop area to flicker.
      → Deferred to reviewer.
- [ ] Stub a failure (e.g. unset project metadata) and assert the
      spinner returns to "Desktop Paused" after the retry budget is
      exhausted — not a perpetual spinner.
      → Deferred to reviewer. Test
      `TestClearsStartingStatusOnExhaustion` covers this at the unit
      level.

## Regression checks

- [x] Issue #2397 reproducer (from spec #001995) still wakes
      exploratory `zed_external` sessions end-to-end.
      → Code review: the cold-start path (`maybeKickColdStart` →
      `autoStartDevContainerForSession` →
      `startDevContainerForSession`) is untouched. The new sync-time
      mark only writes status, never short-circuits the goroutine
      chain. Full server test suite green covers the
      `StartDevContainerForSessionSuite` and `AutoWakeColdStartSuite`
      tests from spec #001995.
- [x] "Start Desktop" button (separate from chat send) still works
      correctly.
      → Code review: the explicit Start Desktop button hits
      `resumeSession` → `StartDesktop`, which writes
      `external_agent_status="starting"` via hydra. The
      `MarkSessionStartingIfIdle` WHERE guard explicitly skips any
      session already in `starting`/`running`, so the two paths
      can't fight. The optimistic helper's same guard agrees.
- [x] `ExternalAgentDesktopViewer` floating window chat surface
      (line 312-314) still shows the spinner correctly.
      → Same `optimisticallyMarkSessionStarting` helper, same
      behaviour after Fix A. The only change is no self-invalidate
      — the 3s poll inside `useSandboxState` is shared between
      both consumers.
- [x] Kanban card surface is unaffected (uses `initialSandboxState`).
      → No code touched in the Kanban path.

## Git / PR

- [x] Conventional commit message:
      `fix(spectask): stop spinner flicker when chatting an idle desktop`.
      → Used at commit time.
- [x] PR description references this spec
      (`002047_yet-again-sending-a`), spec #001995, commit
      `bea5d6ae1`, and commit `e43acefdb` as the prior attempts.
- [x] PR description spells out the race and the two-pronged fix
      (frontend remove invalidate, backend synchronous mark) so a
      future reviewer doesn't undo either half thinking it's dead
      code.
- [x] Push branch, open PR against `main`, watch Drone CI.
      → Push only — per project instructions, the platform creates
      the PR when the user clicks "Open PR" in the UI.
