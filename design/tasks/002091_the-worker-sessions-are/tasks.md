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

### E2E verification in inner Helix — DONE

User overrode the earlier deferral ("you will not stop this from working"); restarted the api + frontend containers to pick up the changes, then drove the inner Helix at http://localhost:8080 via chrome-devtools.

- [x] **API rebuilt and confirmed running new code.** Startup log: `🚀 [AUTO_WAKE] Started auto-wake worker for stuck waiting interactions  max_retries=2 scan_interval=10000 stuck_threshold=180000` — `stuck_threshold=180000` (180s) proves Phase 1 is live in production.
- [x] **Registered test user, created testorg + testproj, created spec task #000001** with the prompt: "Run this command in bash and report the result back to me: `sleep 200 && echo \"tool finished\"`."
- [x] **Started planning** — agent session `ses_01kttyvw9p3z6we0xs6r450hcq` spawned, ran the 200s sleep tool call, completed successfully.
- [x] **DB confirms no decoy, no auto-wake firing** over the full ~10 minutes the session was tracked:
  - At age 60s: 1 row, state=waiting, auto_wake_count=0
  - At age 187s (past 180s threshold): 1 row, state=waiting, auto_wake_count=0
  - At age 315s (past old 5-min ActivationTimeout boundary): 1 row, state=complete, auto_wake_count=0
  - At age 555s: still 1 row, state=complete, auto_wake_count=0
  - **Throughout: exactly one interaction. No decoy ever spawned.**
- [x] **Zero `AUTO_WAKE` log entries** during the 12-minute test window — `docker compose logs api | grep AUTO_WAKE` matched only the startup banner.
- [x] **UI confirms no error banner** — agent's final message: "The command completed after the 200-second sleep. It printed: tool finished". "Agent finished working" notification, no "↻ Retried", no "Incomplete interaction".
- [x] Screenshots saved under `screenshots/` — `01-agent-running-sleep-tool.png` (mid-run), `02-session-completed-no-banner.png` (completion).
- [x] Design notes in `design/2026-06-11-auto-wake-tool-call-fix.md` in the Helix repo per CLAUDE.md convention.

## Phase 4 — CI and ship

- [x] Write per-repo PR description (`pull_request_helix.md`).
- [x] Merge `origin/main` into `feature/002091-stop-auto-wake-from` — clean merge, no conflicts. Build + tests still green.
- [x] Push `feature/002091-stop-auto-wake-from` to origin. (Helix platform auto-creates the GitHub PR.)
- [ ] On merge, post in the relevant ops channel: operators can drop custom `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=600+` overrides — 180s is now the safe default, and the decoy-spawning bug is gone.
