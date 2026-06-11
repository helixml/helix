# Implementation Tasks: Stop Auto-Wake From Cutting Short Long Tool-Call Sessions

## Phase 1 — Session-layer threshold bump (ships first, quick mitigation)

- [x] Change `defaultAutoWakeStuckThreshold` in `api/pkg/server/auto_wake_stuck_interactions.go` from `60 * time.Second` to `180 * time.Second`.
- [x] Rewrite the comment block above the constant (lines 190-209) to explain the new value: 180s covers realistic synchronous tool-call durations (`git push`, `npm install`, `gh pr view`, `find /`), with Decision 1 (org-layer timeout removal) as the real protection and this threshold as defence-in-depth.
- [x] Update the comment at `auto_wake_stuck_interactions.go:396-415` to correct the empirical claim: tool-call **cascades** touch `lastPublish` on every event, but a **single long-running** tool produces no events during execution, so the gate can decay during it. Document this limitation honestly.
- [x] Update any test in `auto_wake_stuck_interactions_test.go` that depends on the literal 60s value. Verify `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` override still works (add a test case if not covered). _Done: bumped fixture `Created` from -90s to -4m on the two cold-start tests that needed to clear the new threshold; added `TestAutoWakeStuckThresholdDefault` and `TestAutoWakeStuckThresholdOverride`._
- [x] `go build ./api/pkg/server/...` to confirm compile.
- [x] Commit: `fix(api): raise auto-wake stuck threshold to 180s`.

## Phase 2 — Org-layer: remove artificial poll-loop deadline

- [x] In `api/pkg/org/infrastructure/runtime/helix/spawner.go`:
  - Rename `SpawnerConfig.ActivationTimeout` → `SessionStartupTimeout` (default `5 * time.Minute`).
  - Add `SpawnerConfig.ActivationRunawayGuard` (default `24 * time.Hour`).
  - Update the default-population block.
- [x] In `Spawner` body: build a shared `parentCtx` (with bearer token attached), derive `startupCtx` with `SessionStartupTimeout` for all pre-session work + `ensureSession`, derive `pollCtx` with `ActivationRunawayGuard` for `pollUntilDone`. Each has its own `defer cancel()`.
- [x] `grep -rn "ActivationTimeout"` — all remaining references are in explanatory comments (the rename rationale); no live code refs.
- [x] Production wiring at `api/pkg/server/helix_org.go:885` doesn't set `ActivationTimeout` — falls through to the new defaults. No edits required there.
- [x] Add unit tests on `Spawner`: `TestSpawnerSessionStartupTimeoutBoundsStartup` (hanging StartSession fires SessionStartupTimeout before runaway guard) and `TestSpawnerPollPhaseNotBoundedBySessionStartupTimeout` (poll loop runs past SessionStartupTimeout boundary, terminates at ActivationRunawayGuard). Existing `TestSpawnerTimeoutEmitsExitError` now exercises ActivationRunawayGuard.
- [x] `go build ./api/pkg/org/... ./api/pkg/server/...` — clean.
- [x] All 15 `TestSpawner*` tests pass; all auto-wake tests pass.
- [x] Commit: `refactor(org): split ActivationTimeout into startup + runaway guard`.

## Phase 3 — Verification

### Unit-test coverage (done in this session)

- [x] **Phase 1 (threshold):** `TestAutoWakeStuckThresholdDefault` pins the new 180s default; `TestAutoWakeStuckThresholdOverride` covers env-var override, garbage fallback, zero fallback. Two existing cold-start tests (`TestSkipsBudgetWhileStartDesktopInFlight`, `TestSkipsBudgetWhileRunningButNoWS`) updated to use `-4 * time.Minute` Created times so they still clear the new threshold.
- [x] **Phase 2 (split contexts):** `TestSpawnerSessionStartupTimeoutBoundsStartup` proves a hanging `StartSession` is bounded by `SessionStartupTimeout`, NOT by the much larger `ActivationRunawayGuard`. `TestSpawnerPollPhaseNotBoundedBySessionStartupTimeout` proves the poll loop runs past the SessionStartupTimeout boundary and terminates only at ActivationRunawayGuard — direct regression test for the decoy-spawning bug. `TestSpawnerTimeoutEmitsExitError` retargeted to exercise `ActivationRunawayGuard`.
- [x] All 15 `TestSpawner*` tests pass; all auto-wake tests pass; whole api package builds clean.

### E2E in inner Helix — DEFERRED to operator after merge

E2E reproduction was not attempted in this session because:

- The inner Helix `api` container is the very runtime hosting this spec-task agent. A full restart to pick up the Phase 2 changes (Air's inotify watcher doesn't fire reliably through the bind mount for `api/pkg/org/infrastructure/runtime/helix/`) would interrupt this and any other concurrent worker sessions on the same instance.
- The bug repro requires a 2–5 minute synchronous tool call followed by observing for several minutes whether a decoy `state=waiting` row is created and whether the auto-wake worker fires. This is operator-friendly to script but very slow inside an active agent session.

What the operator should run post-merge to confirm:

- [ ] Reproduce pre-change (against a build without these commits, or with `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=60` env-var to keep just the threshold half): create a spec task, ask the agent to run `sleep 120 && echo done` inside a tool call. Confirm the "↻ Retried" / "Incomplete interaction" banner appears on a decoy interaction.
- [ ] Post-change: repeat the same scenario. Confirm: the session completes without a banner; no decoy row exists in the DB; no `auto_wake_count > 0` on the interaction.
- [ ] DB check:
  `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, session_id, state, response_message, auto_wake_count, created FROM interactions WHERE session_id = '<session>' ORDER BY created;"`
  Expect exactly one row per turn, none with empty `response_message` while the session is still alive.
- [ ] Verify auto-wake still fires when genuinely needed: simulate a stuck `state=waiting` row with empty response and aged-out `created`. Confirm wake-up runs on the new 180s schedule (slower than before — that is the design).
- [ ] `ActivationRunawayGuard` (24h) is covered by unit test (`TestSpawnerTimeoutEmitsExitError` with `ActivationRunawayGuard = 30ms`); production behaviour at 24h does not need interactive verification.

- [x] Design notes captured in `design/2026-06-11-auto-wake-tool-call-fix.md` in the Helix repo per CLAUDE.md convention.

## Phase 4 — CI and ship

- [~] Write per-repo PR description (`pull_request_helix.md`).
- [ ] Merge `origin/main` into `feature/002091-stop-auto-wake-from` and resolve any conflicts.
- [ ] Push `feature/002091-stop-auto-wake-from` to origin. (Helix platform auto-creates the GitHub PR.)
- [ ] On merge, post in the relevant ops channel: operators can drop custom `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=600+` overrides — 180s is now the safe default, and the decoy-spawning bug is gone.
