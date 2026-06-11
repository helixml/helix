# Implementation Tasks: Stop Auto-Wake From Cutting Short Long Tool-Call Sessions

## Phase 1 — Session-layer threshold bump (ships first, quick mitigation)

- [~] Change `defaultAutoWakeStuckThreshold` in `api/pkg/server/auto_wake_stuck_interactions.go` from `60 * time.Second` to `180 * time.Second`.
- [ ] Rewrite the comment block above the constant (lines 190-209) to explain the new value: 180s covers realistic synchronous tool-call durations (`git push`, `npm install`, `gh pr view`, `find /`), with Decision 1 (org-layer timeout removal) as the real protection and this threshold as defence-in-depth.
- [ ] Update the comment at `auto_wake_stuck_interactions.go:396-415` to correct the empirical claim: tool-call **cascades** touch `lastPublish` on every event, but a **single long-running** tool produces no events during execution, so the gate can decay during it. Document this limitation honestly.
- [ ] Update any test in `auto_wake_stuck_interactions_test.go` that depends on the literal 60s value. Verify `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` override still works (add a test case if not covered).
- [ ] `go build ./api/pkg/server/...` to confirm compile.
- [ ] Commit: `fix(api): raise auto-wake stuck threshold to 180s`.

## Phase 2 — Org-layer: remove artificial poll-loop deadline

- [ ] In `api/pkg/org/infrastructure/runtime/helix/spawner.go`:
  - Rename `SpawnerConfig.ActivationTimeout` → `SessionStartupTimeout` (default `5 * time.Minute`).
  - Add `SpawnerConfig.ActivationRunawayGuard` (default `24 * time.Hour`).
  - Update the default-population block at lines 146-148.
- [ ] In `Spawner` body (lines 159+):
  - Build a `startupCtx` from `ctx` with `SessionStartupTimeout`, use it for `cfg.ensureSession(...)`.
  - Build a separate `pollCtx` from `ctx` with `ActivationRunawayGuard`, use it for `cfg.pollUntilDone(...)`.
  - Cancel each at the appropriate point (defer cancel for each).
- [ ] `grep -rn "ActivationTimeout" /home/retro/work/helix/api/` — rename every reference. Likely sites: test wirings, dev wirings, any config-parsing code, any documentation comments inside the package.
- [ ] Audit callers of `Spawner` / `SpawnerConfig` in `api/pkg/server/helix_org.go` (or wherever the host wires the spawner) and update field names.
- [ ] Add or update a unit test on `Spawner` covering: (a) `ensureSession` honours `SessionStartupTimeout` (fails if startup hangs past it); (b) `pollUntilDone` does NOT terminate at the 5-minute mark when the session is still being polled (lets it run past the old deadline); (c) `pollUntilDone` does eventually terminate at `ActivationRunawayGuard` if the session truly never reports terminal.
- [ ] `go build ./api/pkg/org/... ./api/pkg/server/...`.
- [ ] Commit: `refactor(org): remove poll-loop deadline; rename ActivationTimeout`.

## Phase 3 — End-to-end verification in inner Helix

- [ ] Register `test@helix.ml` / `helixtest` in the inner Helix at `http://localhost:8080`. Complete onboarding (testorg → testproj).
- [ ] Reproduce the failure mode against an un-fixed build (or revert Phases 1+2 locally first): create a spec task and ask the agent to run `sleep 120 && echo done` inside a tool call. Confirm the "↻ Retried" / "Incomplete interaction" banner appears on a decoy interaction.
- [ ] Re-apply Phases 1+2. Repeat the same scenario. Confirm: the session completes without a banner, no decoy row exists in the DB, no `auto_wake_count > 0` on the interaction.
- [ ] DB check after a long session:
  `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, session_id, state, response_message, auto_wake_count, created FROM interactions WHERE session_id = '<session>' ORDER BY created;"`
  Expect exactly one row per turn, none with empty `response_message` while the session is still alive.
- [ ] Verify the auto-wake worker still fires when genuinely needed: manually insert a `state=waiting` row with empty response and aged-out `created` (or wait for an organic ACP-buffer-drop case). Confirm wake-up runs on the new 180s schedule.
- [ ] Verify `ActivationRunawayGuard` does fire on a session that genuinely never terminates (mock / staging environment): the spawner should return after 24h, not earlier. Long-running but acceptable to skip in interactive verification — covered by unit test.
- [ ] Document results in `design/2026-06-11-auto-wake-tool-call-fix.md` in the Helix repo (per CLAUDE.md convention).

## Phase 4 — CI and ship

- [ ] Push branch. Open Helix PR titled `fix(api,org): stop auto-wake cutting short long tool-call sessions`. Body links to this spec task and summarises both decisions.
- [ ] Watch CI via `gh pr checks <num>` or the Drone MCP tools. Fix any failures before requesting review.
- [ ] On merge, post in the relevant ops channel: operators can drop custom `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=600+` overrides — 180s is now the safe default, and the decoy-spawning bug is gone.
